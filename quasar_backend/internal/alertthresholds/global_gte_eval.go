package alertthresholds

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertnotify"
	"github.com/rs/zerolog"
)

// GteMetricThreshold limiares estilo «warning_min» / «critical_min» com operador gte (métricas de equipamento).
type GteMetricThreshold struct {
	Operator string
	Warning  float64
	Critical float64
	HasWarn  bool
	HasCrit  bool
}

func metricApplyCategoriesAcceptDevice(categories []string, deviceCategory string) bool {
	dev := strings.ToLower(strings.TrimSpace(deviceCategory))
	if len(categories) == 0 {
		return true
	}
	for _, c := range categories {
		cc := strings.ToLower(strings.TrimSpace(c))
		if cc == "" || cc == "*" || cc == "all" || cc == "todos" {
			return true
		}
		if dev != "" && cc == dev {
			return true
		}
		// OLT pode vir como "OLT" ou "olt"
		if dev != "" && strings.EqualFold(strings.TrimSpace(c), deviceCategory) {
			return true
		}
	}
	return dev == ""
}

type globalMetricRow struct {
	ID                string   `json:"id"`
	Label             string   `json:"label"`
	Unit              string   `json:"unit"`
	Enabled           *bool    `json:"enabled"`
	Operator          string   `json:"operator"`
	GreenMin          string   `json:"green_min"`
	WarningMin        string   `json:"warning_min"`
	CriticalMin       string   `json:"critical_min"`
	ApplyCategories   []string `json:"apply_categories"`
}

// GlobalThresholdRuleName é o nome da regra em `alert_rules` usada pela UI de limiares globais.
func GlobalThresholdRuleName() string { return globalThresholdRuleName }

// OltOnuDropPercentAlertsEnabled indica se o limiar «Queda de ONUs online (%)» está activo para OLT
// (regra global activa, métrica presente e `enabled` da métrica).
func OltOnuDropPercentAlertsEnabled(ctx context.Context, pool *pgxpool.Pool) bool {
	_, _, ok := LoadGlobalGteMetricForDevice(ctx, pool, "olt_onu_drop_percent", "olt")
	return ok
}

// OltOnuQuantityAlertsEnabled indica se existe algum limiar activo relacionado com contagem/queda de ONUs na OLT
// (percentual e/ou queda absoluta por PON). O worker de monitorização só deve colectar PON derivado IF-MIB quando isto for verdadeiro.
func OltOnuQuantityAlertsEnabled(ctx context.Context, pool *pgxpool.Pool) bool {
	if OltOnuDropPercentAlertsEnabled(ctx, pool) {
		return true
	}
	_, _, okCnt := LoadGlobalGteMetricForDevice(ctx, pool, "olt_onu_drop_count", "olt")
	return okCnt
}

// LoadGlobalGteMetricForDevice carrega o primeiro métrico da regra global que coincide com metricID e aplica ao tipo de equipamento.
func LoadGlobalGteMetricForDevice(ctx context.Context, pool *pgxpool.Pool, metricID, deviceCategory string) (GteMetricThreshold, string, bool) {
	var out GteMetricThreshold
	if pool == nil {
		return out, "", false
	}
	metricID = strings.TrimSpace(metricID)
	if metricID == "" {
		return out, "", false
	}
	var enabled bool
	var raw []byte
	err := pool.QueryRow(ctx, `
		SELECT enabled, condition_json::text FROM alert_rules
		WHERE name = $1 LIMIT 1
	`, globalThresholdRuleName).Scan(&enabled, &raw)
	if err != nil || !enabled || len(raw) == 0 {
		return out, "", false
	}
	var root struct {
		Schema  string           `json:"schema"`
		Metrics []globalMetricRow `json:"metrics"`
	}
	if json.Unmarshal(raw, &root) != nil {
		return out, "", false
	}
	if root.Schema != "" && root.Schema != alertSchemaV1 {
		return out, "", false
	}
	for _, m := range root.Metrics {
		if strings.TrimSpace(m.ID) != metricID {
			continue
		}
		if !metricApplyCategoriesAcceptDevice(m.ApplyCategories, deviceCategory) {
			continue
		}
		if m.Enabled != nil && !*m.Enabled {
			continue
		}
		op := strings.ToLower(strings.TrimSpace(m.Operator))
		if op == "" {
			op = "gte"
		}
		out.Operator = op
		if f, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(m.WarningMin), ",", "."), 64); err == nil {
			out.Warning, out.HasWarn = f, true
		}
		if f, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(m.CriticalMin), ",", "."), 64); err == nil {
			out.Critical, out.HasCrit = f, true
		}
		label := strings.TrimSpace(m.Label)
		if label == "" {
			label = metricID
		}
		return out, label, out.HasWarn || out.HasCrit
	}
	return out, "", false
}

func severityGteMetric(v float64, t GteMetricThreshold) string {
	if t.Operator == "lte" {
		if t.HasCrit && v <= t.Critical {
			return "critical"
		}
		if t.HasWarn && v <= t.Warning {
			return "warning"
		}
		return "ok"
	}
	if t.HasCrit && v >= t.Critical {
		return "critical"
	}
	if t.HasWarn && v >= t.Warning {
		return "warning"
	}
	return "ok"
}

const alertTypeTelemetryThreshold = "telemetry_threshold"

// EvaluateGlobalGteMetric abre/atualiza ou fecha alerta conforme limiar global (ex.: temperature_c).
func EvaluateGlobalGteMetric(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, deviceID uuid.UUID, deviceDesc, deviceIP, metricID string, value float64) {
	var devCat string
	_ = pool.QueryRow(ctx, `SELECT COALESCE(lower(trim(category)), '') FROM devices WHERE id=$1`, deviceID).Scan(&devCat)
	th, metricLabel, ok := LoadGlobalGteMetricForDevice(ctx, pool, metricID, devCat)
	if !ok {
		return
	}
	sev := severityGteMetric(value, th)
	sevPt := strings.ToUpper(sev)
	if sev == "critical" {
		sevPt = "Crítico"
	} else if sev == "warning" {
		sevPt = "Atenção"
	}
	desc := strings.TrimSpace(deviceDesc)
	ip := strings.TrimSpace(deviceIP)
	key := "telemetry:" + metricID

	if sev == "ok" {
		closeTelemetryThresholdAlert(ctx, pool, log, deviceID, key)
		return
	}
	msg := fmt.Sprintf("%s (%s): %s está em %.2f — estado %s segundo os seus limiares de alerta.", descOrEmpty(desc, "?"), addrOrEmpty(ip, "?"), metricLabel, value, sevPt)
	meta := alertnotify.WithStatusTransition(map[string]any{
		"source":    "monitoring_telemetry",
		"key":       key,
		"metric_id": metricID,
		"value":     value,
	}, "metric_normal", "threshold_"+sev, nil)
	metaRaw, _ := json.Marshal(meta)
	var newID uuid.UUID
	err := pool.QueryRow(ctx, `
		INSERT INTO alert_instances (device_id, severity, alert_type, message, ip, device_name, meta)
		SELECT $1::uuid, $2::text, $3::text, $4, NULLIF(trim($5), ''), NULLIF(trim($6), ''),
			COALESCE($7::jsonb, '{}'::jsonb)
		WHERE NOT EXISTS (
			SELECT 1 FROM alert_instances ai
			WHERE ai.device_id = $1::uuid
			  AND ai.alert_type = $3::text
			  AND ai.closed_at IS NULL
			  AND (ai.meta->>'key') = ($8::jsonb->>'key')
		)
		RETURNING id
	`, deviceID, sev, alertTypeTelemetryThreshold, msg, ip, desc, metaRaw, metaRaw).Scan(&newID)
	if err == nil {
		if log != nil {
			log.Warn().Str("device", deviceID.String()).Str("metric", metricID).Str("sev", sev).Msg("alerta telemetria: limiar global")
		}
		alertnotify.SendMonitoringTelegramAndPatchMeta(ctx, pool, log, newID, strings.ToUpper(sev), "Telemetria — limiar global", msg)
		return
	}
	if err != pgx.ErrNoRows {
		if log != nil {
			log.Error().Err(err).Str("device", deviceID.String()).Msg("alert_instances insert telemetry_threshold")
		}
		return
	}
	// Já existe: atualiza valor / texto / severidade na UI sem novo Telegram.
	updateMeta, _ := json.Marshal(map[string]any{
		"source":    "monitoring_telemetry",
		"metric_id": metricID,
		"value":     value,
	})
	_, _ = pool.Exec(ctx, `
		UPDATE alert_instances SET
			severity = $4::text,
			message = $5,
			meta = COALESCE(meta, '{}'::jsonb) || $6::jsonb
		WHERE device_id = $1::uuid AND alert_type = $2::text AND closed_at IS NULL
		  AND (meta->>'key') = $3
	`, deviceID, alertTypeTelemetryThreshold, key, sev, msg, updateMeta)
}

func closeTelemetryThresholdAlert(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, deviceID uuid.UUID, key string) {
	metaPatch, _ := json.Marshal(map[string]any{"resolved": "metric_within_limits", "source": "monitoring_telemetry"})
	var aid uuid.UUID
	var msg string
	err := pool.QueryRow(ctx, `
		UPDATE alert_instances SET
			closed_at = now(),
			meta = COALESCE(meta, '{}'::jsonb) || $3::jsonb
		WHERE device_id = $1::uuid AND alert_type = $2::text AND closed_at IS NULL
		  AND (meta->>'key') = $4
		RETURNING id, message
	`, deviceID, alertTypeTelemetryThreshold, metaPatch, key).Scan(&aid, &msg)
	if err != nil {
		return
	}
	head := alertnotify.ResolutionHeadlineForAlertType(alertTypeTelemetryThreshold)
	alertnotify.SendResolutionTelegramAndPatchMeta(ctx, pool, log, aid, head, msg)
}

func descOrEmpty(s, fb string) string {
	if strings.TrimSpace(s) == "" {
		return fb
	}
	return s
}

func addrOrEmpty(s, fb string) string {
	if strings.TrimSpace(s) == "" {
		return fb
	}
	return s
}
