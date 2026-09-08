package alertthresholds

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertignore"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertnotify"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertstore"
	"github.com/netquasar/netquasar/quasar_backend/internal/oltifderive"
	"github.com/rs/zerolog"
)

// Alertas de limiar óptico/térmico ao nível da PON (métricas olt_pon_* na UI de Alertas).
// Distintos de olt_onu_*: usam os mesmos campos do snapshot da PON, mas limiares e tipos próprios.
const (
	alertTypeOltPonTx   = "olt_pon_tx"
	alertTypeOltPonRx   = "olt_pon_rx"
	alertTypeOltPonTemp = "olt_pon_temp"
	// ponOpticalCycles — mesma cadência de confirmação já usada pelo alerta de ONU
	// (onuOpticalCycles, olt_onu_optical.go): só abre alerta depois de N leituras ruins
	// SEGUIDAS, para não disparar por um timeout SNMP pontual (alta latência até a OLT) que
	// uma única leitura ruim, isolada, não distingue de uma falha óptica real.
	ponOpticalCycles = 3
)

// EvaluateOltPonOpticalFromPons avalia TX/RX dBm e temperatura da PON face aos limiares
// «olt_pon_tx_dbm», «olt_pon_rx_dbm» e «olt_pon_temp_c» da regra global.
func EvaluateOltPonOpticalFromPons(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, deviceID uuid.UUID, desc, ip string, pons []map[string]any) {
	if pool == nil || len(pons) == 0 {
		return
	}
	for _, p := range pons {
		key := oltifderive.StablePonRowKey(p)
		if key == "" {
			continue
		}
		if tx, ok := parseNum(p["tx_dbm"]); ok {
			evaluateOltPonMetric(ctx, pool, log, deviceID, desc, ip, key, "olt_pon_tx_dbm", alertTypeOltPonTx, "PON TX fora do limiar", tx, "dBm")
		}
		if rx, ok := parseNum(p["rx_dbm"]); ok {
			evaluateOltPonMetric(ctx, pool, log, deviceID, desc, ip, key, "olt_pon_rx_dbm", alertTypeOltPonRx, "PON RX fora do limiar", rx, "dBm")
		}
		if temp, ok := parseNum(p["temperature"]); ok {
			evaluateOltPonMetric(ctx, pool, log, deviceID, desc, ip, key, "olt_pon_temp_c", alertTypeOltPonTemp, "Temperatura da PON fora do limiar", temp, "°C")
		}
	}
}

// isPlausibleOltPonOptical filtra leituras que quase certamente não são uma medição real, mas
// sim um timeout/latência alta que o parser SNMP não conseguiu distinguir de um valor genuíno
// (ver collectSessionsByIndex/parseOpticalDbm — devolvem "", não "0", numa leitura vazia, mas
// alguns caminhos de IF-MIB/merge podem produzir um zero-value quando a leitura falha a meio).
// TX óptico de uma PON activa nunca é exactamente 0.00 dBm num transceiver real — é o padrão
// clássico de leitura falhada. Devolve false = ignorar esta leitura por completo (não conta como
// boa nem má, não fecha nem abre alerta) em vez de tratar como uma leitura má genuína.
func isPlausibleOltPonOptical(metricID string, value float64) bool {
	if metricID == "olt_pon_tx_dbm" && value == 0 {
		return false
	}
	return true
}

func evaluateOltPonMetric(
	ctx context.Context,
	pool *pgxpool.Pool,
	log *zerolog.Logger,
	deviceID uuid.UUID,
	deviceDesc, deviceIP, ponKey, metricID, alertType, headline string,
	value float64,
	unit string,
) {
	if !isPlausibleOltPonOptical(metricID, value) {
		return
	}
	th, label, ok := LoadGlobalGteMetricForDevice(ctx, pool, metricID, "olt")
	if !ok {
		return
	}
	sev := severityGteMetric(value, th)
	if metricID == "olt_pon_rx_dbm" {
		sev = capRxToWarning(sev)
	}
	metaKey := metricID + ":" + ponKey
	streak := updatePonOpticalStreak(ctx, pool, deviceID, ponKey, metricID, sev != "ok", value)
	if sev == "ok" {
		closeOltPonOpticalAlert(ctx, pool, log, deviceID, alertType, metaKey)
		return
	}
	// Só confirma o alerta depois de N ciclos seguidos ruins — ver ponOpticalCycles.
	if streak < ponOpticalCycles {
		return
	}
	if alertignore.IsMuted(ctx, pool, deviceID, alertType, metaKey) {
		return
	}
	msg := fmt.Sprintf("%s (%s): PON %s — %s em %.2f %s (severidade: %s, confirmado por %d ciclos).",
		descOrEmpty(strings.TrimSpace(deviceDesc), "?"),
		addrOrEmpty(strings.TrimSpace(deviceIP), "?"),
		ponKey, label, value, unit, sev, ponOpticalCycles)
	base := map[string]any{
		"source":          "monitor_worker_olt",
		"key":             metaKey,
		"metric_id":       metricID,
		"pon":             ponKey,
		"value":           value,
		"value_text":      fmt.Sprintf("%.2f %s", value, unit),
		"streak":          streak,
		"required_streak": ponOpticalCycles,
	}
	if unit == "dBm" {
		base["dbm"] = value
	}
	if unit == "°C" {
		base["temperature_c"] = value
	}
	meta := alertnotify.WithStatusTransition(base, "pon_metric_normal", "threshold_"+sev, nil)
	_, err := alertstore.OpenOrUpdate(ctx, pool, alertstore.OpenSpec{
		DeviceID: deviceID, Severity: sev, AlertType: alertType,
		Message: msg, IP: deviceIP, DeviceName: deviceDesc, Meta: meta,
		Match: alertstore.Match{Kind: alertstore.MatchMetaKey, MetaKey: metaKey},
	}, &alertstore.NotifyCreate{
		Log: log, Level: strings.ToUpper(sev), Headline: headline,
	})
	if err != nil && log != nil {
		log.Error().Err(err).Str("device", deviceID.String()).Str("alert_type", alertType).Msg("alertstore olt_pon_optical")
	}
}

// updatePonOpticalStreak incrementa (leitura ruim) ou zera (leitura ok) a contagem de ciclos
// seguidos ruins para esta PON+métrica — mesma técnica de updateOnuOpticalStreak (olt_onu_optical.go).
func updatePonOpticalStreak(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID, ponKey, metricID string, bad bool, value float64) int {
	next := 0
	if bad {
		next = 1
	}
	_ = pool.QueryRow(ctx, `
		INSERT INTO olt_pon_optical_streak (device_id, pon_key, metric_id, streak, last_value, updated_at)
		VALUES ($1::uuid, $2::text, $3::text, $4::int, $5::float8, now())
		ON CONFLICT (device_id, pon_key, metric_id)
		DO UPDATE SET
			streak = CASE WHEN $4::int > 0 THEN olt_pon_optical_streak.streak + 1 ELSE 0 END,
			last_value = $5::float8,
			updated_at = now()
		RETURNING streak
	`, deviceID, strings.TrimSpace(ponKey), strings.TrimSpace(metricID), next, value).Scan(&next)
	return next
}

func closeOltPonOpticalAlert(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, deviceID uuid.UUID, alertType, key string) {
	_, _, _ = alertstore.Close(ctx, pool, log, alertstore.CloseSpec{
		DeviceID: deviceID, AlertType: alertType,
		Match: alertstore.Match{Kind: alertstore.MatchMetaKey, MetaKey: key},
		Resolved: map[string]any{
			"resolved": "pon_metric_within_limits", "source": "monitor_worker_olt", "key": key,
		},
	})
}
