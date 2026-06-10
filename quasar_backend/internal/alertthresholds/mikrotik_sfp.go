package alertthresholds

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertignore"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertnotify"
	"github.com/rs/zerolog"
)

const (
	globalThresholdRuleName = "Limiar global de alertas"
	alertSchemaV1           = "netquasar.alert_thresholds.v1"
	alertTypeSfpTx          = "mikrotik_sfp_tx"
	alertTypeSfpRx          = "mikrotik_sfp_rx"
)

// SfpInterfaceRow dados por interface após colheita SNMP.
type SfpInterfaceRow struct {
	IfIndex     int
	DisplayName string
	Sfp         bool
	TxDBm       *float64
	RxDBm       *float64
}

type thresholdMetric struct {
	ID          string
	Operator    string
	GreenMin    float64
	WarningMin  float64
	CriticalMin float64
	HasGreen    bool
	HasWarning  bool
	HasCritical bool
}

func parseFloatPtr(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	f, err := strconv.ParseFloat(strings.Replace(s, ",", ".", -1), 64)
	if err != nil || math.IsNaN(f) {
		return 0, false
	}
	return f, true
}

func loadGlobalSfpThresholds(ctx context.Context, pool *pgxpool.Pool) (tx, rx thresholdMetric, ruleEnabled bool, ok bool) {
	if pool == nil {
		return tx, rx, false, false
	}
	var en bool
	var raw []byte
	err := pool.QueryRow(ctx, `
		SELECT enabled, condition_json::text FROM alert_rules
		WHERE name = $1 LIMIT 1
	`, globalThresholdRuleName).Scan(&en, &raw)
	if err != nil || !en || len(raw) == 0 {
		return tx, rx, false, false
	}
	var root struct {
		Schema  string `json:"schema"`
		Metrics []struct {
			ID          string `json:"id"`
			Enabled     *bool  `json:"enabled"`
			Operator    string `json:"operator"`
			GreenMin    string `json:"green_min"`
			WarningMin  string `json:"warning_min"`
			CriticalMin string `json:"critical_min"`
		} `json:"metrics"`
	}
	if json.Unmarshal(raw, &root) != nil {
		return tx, rx, false, false
	}
	if root.Schema != "" && root.Schema != alertSchemaV1 {
		return tx, rx, false, false
	}

	fill := func(id string, target *thresholdMetric) {
		for _, m := range root.Metrics {
			if strings.TrimSpace(m.ID) != id {
				continue
			}
			if m.Enabled != nil && !*m.Enabled {
				return
			}
			target.ID = id
			target.Operator = strings.TrimSpace(strings.ToLower(m.Operator))
			if target.Operator == "" {
				target.Operator = "lte"
			}
			if f, ok := parseFloatPtr(m.GreenMin); ok {
				target.GreenMin, target.HasGreen = f, true
			}
			if f, ok := parseFloatPtr(m.WarningMin); ok {
				target.WarningMin, target.HasWarning = f, true
			}
			if f, ok := parseFloatPtr(m.CriticalMin); ok {
				target.CriticalMin, target.HasCritical = f, true
			}
			return
		}
	}
	fill("mikrotik_sfp_tx_dbm", &tx)
	fill("mikrotik_sfp_rx_dbm", &rx)
	if !tx.HasWarning && !tx.HasCritical && !rx.HasWarning && !rx.HasCritical {
		return tx, rx, true, false
	}
	return tx, rx, true, true
}

func severityLTE(v float64, m thresholdMetric) string {
	if m.HasCritical && v <= m.CriticalMin {
		return "critical"
	}
	if m.HasWarning && v <= m.WarningMin {
		return "warning"
	}
	return "ok"
}

func severityGTE(v float64, m thresholdMetric) string {
	if m.HasCritical && v >= m.CriticalMin {
		return "critical"
	}
	if m.HasWarning && v >= m.WarningMin {
		return "warning"
	}
	return "ok"
}

func evalOne(v float64, m thresholdMetric) string {
	if m.Operator == "gte" {
		return severityGTE(v, m)
	}
	return severityLTE(v, m)
}

// EvaluateMikrotikSFPAfterSnapshot abre ou fecha alertas conforme limiares globais (regra «Limiar global de alertas»).
func EvaluateMikrotikSFPAfterSnapshot(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, deviceID uuid.UUID, deviceDesc, deviceIP string, rows []SfpInterfaceRow) {
	txRule, rxRule, enabled, hasLimits := loadGlobalSfpThresholds(ctx, pool)
	if !enabled || !hasLimits {
		return
	}
	ip := strings.TrimSpace(deviceIP)
	desc := strings.TrimSpace(deviceDesc)

	for _, row := range rows {
		if !row.Sfp {
			closeSfpAlert(ctx, pool, log, deviceID, alertTypeSfpTx, row.IfIndex)
			closeSfpAlert(ctx, pool, log, deviceID, alertTypeSfpRx, row.IfIndex)
			continue
		}
		if row.TxDBm != nil && (txRule.HasWarning || txRule.HasCritical) {
			syncSfpAlert(ctx, pool, log, deviceID, desc, ip, alertTypeSfpTx, row.IfIndex, row.DisplayName, "TX", *row.TxDBm, txRule)
		} else {
			closeSfpAlert(ctx, pool, log, deviceID, alertTypeSfpTx, row.IfIndex)
		}
		if row.RxDBm != nil && (rxRule.HasWarning || rxRule.HasCritical) {
			syncSfpAlert(ctx, pool, log, deviceID, desc, ip, alertTypeSfpRx, row.IfIndex, row.DisplayName, "RX", *row.RxDBm, rxRule)
		} else {
			closeSfpAlert(ctx, pool, log, deviceID, alertTypeSfpRx, row.IfIndex)
		}
	}
}

func syncSfpAlert(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, deviceID uuid.UUID, desc, ip, alertType string, ifIndex int, ifName, label string, v float64, rule thresholdMetric) {
	sev := evalOne(v, rule)
	ifLabel := strings.TrimSpace(ifName)
	if ifLabel == "" {
		ifLabel = fmt.Sprintf("if%d", ifIndex)
	}
	base := map[string]any{
		"source":       "interface_snmp_refresh",
		"if_index":     ifIndex,
		"display_name": ifLabel,
		"if_name":      ifLabel,
		"key":          ifLabel,
		"metric":       label,
		"dbm":          v,
	}
	if sev == "ok" {
		closeSfpAlert(ctx, pool, log, deviceID, alertType, ifIndex)
		return
	}
	if alertignore.IsMuted(ctx, pool, deviceID, alertType, ifLabel) {
		return
	}
	msg := fmt.Sprintf("%s (%s): interface %s — potência SFP %s %.3f dBm (severidade: %s).", descOr(desc, "?"), addrOr(ip, "?"), ifLabel, label, v, sev)
	insertMeta, _ := json.Marshal(alertnotify.WithStatusTransition(base, "optical_within_limits", "threshold_"+sev, nil))
	var newID uuid.UUID
	err := pool.QueryRow(ctx, `
		INSERT INTO alert_instances (device_id, severity, alert_type, message, ip, device_name, meta)
		SELECT $1::uuid, $2::text, $3::text, $4, NULLIF(trim($5),''), NULLIF(trim($6),''), COALESCE($7::jsonb,'{}'::jsonb)
		WHERE NOT EXISTS (
			SELECT 1 FROM alert_instances ai
			WHERE ai.device_id = $1::uuid AND ai.alert_type = $3::text AND ai.closed_at IS NULL
			  AND (ai.meta->>'if_index')::int = $8
		)
		RETURNING id
	`, deviceID, sev, alertType, msg, ip, desc, insertMeta, ifIndex).Scan(&newID)
	if err == nil {
		if log != nil {
			log.Warn().Str("device", deviceID.String()).Str("alert_type", alertType).Int("if_index", ifIndex).Msg("alerta SFP: mudança de estado (limiar)")
		}
		alertnotify.SendMonitoringTelegramAndPatchMeta(ctx, pool, log, newID, strings.ToUpper(sev), "Potência óptica SFP", msg)
		return
	}
	if err != pgx.ErrNoRows {
		if log != nil {
			log.Error().Err(err).Str("device", deviceID.String()).Msg("alert_instances insert mikrotik_sfp")
		}
		return
	}
	updateMeta, _ := json.Marshal(base)
	_, err = pool.Exec(ctx, `
		UPDATE alert_instances SET
			severity = $4::text,
			message = $5,
			meta = COALESCE(meta, '{}'::jsonb) || $6::jsonb
		WHERE device_id = $1::uuid AND alert_type = $2::text AND closed_at IS NULL
		  AND (meta->>'if_index')::int = $7
	`, deviceID, alertType, sev, msg, updateMeta, ifIndex)
	if err != nil && log != nil {
		log.Error().Err(err).Str("device", deviceID.String()).Msg("alert_instances update mikrotik_sfp valores")
	}
}

func closeSfpAlert(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, deviceID uuid.UUID, alertType string, ifIndex int) {
	metaPatch, _ := json.Marshal(map[string]any{"resolved": "sfp_threshold_ok", "source": "interface_snmp_refresh"})
	var aid uuid.UUID
	var msg string
	err := pool.QueryRow(ctx, `
		UPDATE alert_instances SET
			closed_at = now(),
			meta = COALESCE(meta, '{}'::jsonb) || $4::jsonb
		WHERE device_id = $1::uuid AND alert_type = $2::text AND closed_at IS NULL
		  AND (meta->>'if_index')::int = $3
		RETURNING id, message
	`, deviceID, alertType, ifIndex, metaPatch).Scan(&aid, &msg)
	if err != nil {
		return
	}
	head := alertnotify.ResolutionHeadlineForAlertType(alertType)
	alertnotify.SendResolutionTelegramAndPatchMeta(ctx, pool, log, aid, head, msg)
}

func descOr(s, fb string) string {
	if strings.TrimSpace(s) == "" {
		return fb
	}
	return s
}

func addrOr(s, fb string) string {
	if strings.TrimSpace(s) == "" {
		return fb
	}
	return s
}
