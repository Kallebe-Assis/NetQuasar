package alertthresholds

// Limiares SFP MikroTik (TX/RX dBm e temperatura do módulo) avaliados após snapshot de interfaces.

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertnotify"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertstore"
	"github.com/rs/zerolog"
)

const (
	alertTypeSfpTx   = "mikrotik_sfp_tx"
	alertTypeSfpRx   = "mikrotik_sfp_rx"
	alertTypeSfpTemp = "mikrotik_sfp_temp"
)

// SfpInterfaceRow dados por interface após colheita SNMP/óptica.
type SfpInterfaceRow struct {
	IfIndex           int
	DisplayName       string
	IfName            string
	IfAlias           string
	CustomDescription string
	Sfp               bool
	TxDBm             *float64
	RxDBm             *float64
	TemperatureC      *float64 // temperatura do módulo SFP (°C), quando disponível
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

func loadGlobalSfpThresholds(ctx context.Context, pool *pgxpool.Pool) (tx, rx, temp thresholdMetric, ruleEnabled bool, ok bool) {
	if pool == nil {
		return tx, rx, temp, false, false
	}
	var en bool
	var raw []byte
	err := pool.QueryRow(ctx, `
		SELECT enabled, condition_json::text FROM alert_rules
		WHERE name = $1 LIMIT 1
	`, globalThresholdRuleName).Scan(&en, &raw)
	if err != nil || !en || len(raw) == 0 {
		return tx, rx, temp, false, false
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
		return tx, rx, temp, false, false
	}
	if root.Schema != "" && root.Schema != alertSchemaV1 {
		return tx, rx, temp, false, false
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
				if strings.Contains(id, "temp") {
					target.Operator = "gte"
				} else {
					target.Operator = "lte"
				}
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
	fill("mikrotik_sfp_temp_c", &temp)
	if !tx.HasWarning && !tx.HasCritical && !rx.HasWarning && !rx.HasCritical && !temp.HasWarning && !temp.HasCritical {
		return tx, rx, temp, true, false
	}
	return tx, rx, temp, true, true
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

// Histerese e tolerância a leituras ausentes. A potência óptica de um SFP oscila na casa dos centésimos de dB em
// torno de um valor praticamente fixo (ex.: -19,0 / -18,98): sem folga, o valor cruza o limiar a cada coleta e o
// alerta abre → «resolvido» → abre de novo. E uma coleta sem leitura óptica (walk incompleto, módulo que não
// respondeu) também não pode «resolver» o alerta: ausência de leitura não é leitura boa.
const (
	sfpClearMarginDBm  = 0.5 // dB de folga para considerar a potência normalizada
	sfpClearMarginTemp = 2.0 // °C de folga para a temperatura
	sfpMissesToClose   = 12  // coletas seguidas sem leitura antes de fechar o alerta
)

var (
	sfpMissMu sync.Mutex
	sfpMisses = map[string]int{}
)

func sfpMissKey(deviceID uuid.UUID, alertType string, ifIndex int) string {
	return fmt.Sprintf("%s|%s|%d", deviceID, alertType, ifIndex)
}

func sfpResetMiss(deviceID uuid.UUID, alertType string, ifIndex int) {
	sfpMissMu.Lock()
	delete(sfpMisses, sfpMissKey(deviceID, alertType, ifIndex))
	sfpMissMu.Unlock()
}

// sfpMissedTooLong conta uma coleta sem leitura e diz se já passou do limite para fechar o alerta.
func sfpMissedTooLong(deviceID uuid.UUID, alertType string, ifIndex int) bool {
	k := sfpMissKey(deviceID, alertType, ifIndex)
	sfpMissMu.Lock()
	defer sfpMissMu.Unlock()
	sfpMisses[k]++
	if sfpMisses[k] >= sfpMissesToClose {
		delete(sfpMisses, k)
		return true
	}
	return false
}

// closeSfpIfMissingLong: sem leitura (ou sem SFP) → só fecha depois de várias coletas seguidas assim.
func closeSfpIfMissingLong(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, deviceID uuid.UUID, alertType string, ifIndex int) {
	if sfpMissedTooLong(deviceID, alertType, ifIndex) {
		closeSfpAlert(ctx, pool, log, deviceID, alertType, ifIndex)
	}
}

// withinClearBand: o valor já está «ok», mas ainda dentro da folga do limiar — mantém o alerta aberto.
func withinClearBand(v float64, m thresholdMetric, margin float64) bool {
	shifted := v - margin // métricas «lte» (potência): precisa estar acima do limiar por margin
	if m.Operator == "gte" {
		shifted = v + margin // métricas «gte» (temperatura): precisa estar abaixo do limiar por margin
	}
	return evalOne(shifted, m) != "ok"
}

// SfpClearMargin devolve a folga de histerese (dB ou °C) usada para «resolver» um alerta SFP.
func SfpClearMargin(temperature bool) float64 {
	if temperature {
		return sfpClearMarginTemp
	}
	return sfpClearMarginDBm
}

// SfpWithinClearBand — versão exportada de withinClearBand para a verificação manual/automática de alertas.
func SfpWithinClearBand(v float64, t GteMetricThreshold, margin float64) bool {
	shifted := v - margin
	if t.Operator != "lte" {
		shifted = v + margin
	}
	return severityGteMetric(shifted, t) != "ok"
}

// EvaluateMikrotikSFPAfterSnapshot abre ou fecha alertas conforme limiares globais
// (TX/RX dBm e temperatura do módulo SFP na regra «Limiar global de alertas»).
func EvaluateMikrotikSFPAfterSnapshot(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, deviceID uuid.UUID, deviceDesc, deviceIP string, rows []SfpInterfaceRow) {
	txRule, rxRule, tempRule, enabled, hasLimits := loadGlobalSfpThresholds(ctx, pool)
	if !enabled || !hasLimits {
		return
	}
	ip := strings.TrimSpace(deviceIP)
	desc := strings.TrimSpace(deviceDesc)

	for _, row := range rows {
		if !row.Sfp {
			closeSfpIfMissingLong(ctx, pool, log, deviceID, alertTypeSfpTx, row.IfIndex)
			closeSfpIfMissingLong(ctx, pool, log, deviceID, alertTypeSfpRx, row.IfIndex)
			closeSfpIfMissingLong(ctx, pool, log, deviceID, alertTypeSfpTemp, row.IfIndex)
			continue
		}
		if txRule.HasWarning || txRule.HasCritical {
			if row.TxDBm != nil {
				sfpResetMiss(deviceID, alertTypeSfpTx, row.IfIndex)
				syncSfpAlert(ctx, pool, log, deviceID, desc, ip, alertTypeSfpTx, row, "TX", *row.TxDBm, "dBm", txRule)
			} else {
				closeSfpIfMissingLong(ctx, pool, log, deviceID, alertTypeSfpTx, row.IfIndex)
			}
		} else {
			closeSfpAlert(ctx, pool, log, deviceID, alertTypeSfpTx, row.IfIndex)
		}
		if rxRule.HasWarning || rxRule.HasCritical {
			if row.RxDBm != nil {
				sfpResetMiss(deviceID, alertTypeSfpRx, row.IfIndex)
				syncSfpAlert(ctx, pool, log, deviceID, desc, ip, alertTypeSfpRx, row, "RX", *row.RxDBm, "dBm", rxRule)
			} else {
				closeSfpIfMissingLong(ctx, pool, log, deviceID, alertTypeSfpRx, row.IfIndex)
			}
		} else {
			closeSfpAlert(ctx, pool, log, deviceID, alertTypeSfpRx, row.IfIndex)
		}
		if tempRule.HasWarning || tempRule.HasCritical {
			if row.TemperatureC != nil {
				sfpResetMiss(deviceID, alertTypeSfpTemp, row.IfIndex)
				syncSfpAlert(ctx, pool, log, deviceID, desc, ip, alertTypeSfpTemp, row, "TEMP", *row.TemperatureC, "°C", tempRule)
			} else {
				closeSfpIfMissingLong(ctx, pool, log, deviceID, alertTypeSfpTemp, row.IfIndex)
			}
		} else {
			closeSfpAlert(ctx, pool, log, deviceID, alertTypeSfpTemp, row.IfIndex)
		}
	}
}

func syncSfpAlert(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, deviceID uuid.UUID, desc, ip, alertType string, row SfpInterfaceRow, label string, v float64, unit string, rule thresholdMetric) {
	sev := evalOne(v, rule)
	if alertType == alertTypeSfpRx {
		sev = capRxToWarning(sev)
	}
	ifLabel := strings.TrimSpace(row.DisplayName)
	if ifLabel == "" {
		ifLabel = fmt.Sprintf("if%d", row.IfIndex)
	}
	baseName := strings.TrimSpace(row.IfName)
	if baseName == "" {
		baseName = ifLabel
	}
	base := map[string]any{
		"source":             "interface_snapshot",
		"if_index":           row.IfIndex,
		"display_name":       ifLabel,
		"if_name":            baseName,
		"if_alias":           strings.TrimSpace(row.IfAlias),
		"custom_description": strings.TrimSpace(row.CustomDescription),
		"key":                ifLabel,
		"metric":             label,
		"value":              v,
		"value_text":         fmt.Sprintf("%.3f %s", v, unit),
	}
	if unit == "dBm" {
		base["dbm"] = v
	}
	if unit == "°C" {
		base["temperature_c"] = v
	}
	if sev == "ok" {
		margin := sfpClearMarginDBm
		if unit == "°C" {
			margin = sfpClearMarginTemp
		}
		if withinClearBand(v, rule, margin) {
			return // oscilando junto do limiar: mantém o alerta como está, sem «resolver» e reabrir
		}
		closeSfpAlert(ctx, pool, log, deviceID, alertType, row.IfIndex)
		return
	}
	what := "potência SFP " + label
	if label == "TEMP" {
		what = "temperatura SFP"
	}
	msg := fmt.Sprintf("%s (%s): interface %s — %s %.3f %s (severidade: %s).",
		descOr(desc, "?"), addrOr(ip, "?"), ifLabel, what, v, unit, sev)
	meta := alertnotify.WithStatusTransition(base, "optical_within_limits", "threshold_"+sev, nil)
	headline := "Potência óptica SFP"
	if label == "TEMP" {
		headline = "Temperatura módulo SFP"
	}
	res, err := alertstore.OpenOrUpdate(ctx, pool, alertstore.OpenSpec{
		DeviceID: deviceID, Severity: sev, AlertType: alertType,
		Message: msg, IP: ip, DeviceName: desc, Meta: meta,
		Match: alertstore.Match{Kind: alertstore.MatchIfIndex, IfIndex: row.IfIndex},
	}, &alertstore.NotifyCreate{
		Log: log, Level: strings.ToUpper(sev), Headline: headline,
	})
	if err != nil && log != nil {
		log.Error().Err(err).Str("device", deviceID.String()).Str("alert_type", alertType).Msg("alertstore SFP")
	} else if res.Created && log != nil {
		log.Warn().Str("device", deviceID.String()).Str("alert_type", alertType).Int("if_index", row.IfIndex).Msg("alerta SFP: aberto")
	}
}

func closeSfpAlert(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, deviceID uuid.UUID, alertType string, ifIndex int) {
	_, _, _ = alertstore.Close(ctx, pool, log, alertstore.CloseSpec{
		DeviceID: deviceID, AlertType: alertType,
		Match: alertstore.Match{Kind: alertstore.MatchIfIndex, IfIndex: ifIndex},
		Resolved: map[string]any{
			"resolved": "sfp_threshold_ok", "source": "interface_snapshot",
		},
	})
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
