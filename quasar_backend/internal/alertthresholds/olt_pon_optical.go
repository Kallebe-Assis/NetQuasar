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

func evaluateOltPonMetric(
	ctx context.Context,
	pool *pgxpool.Pool,
	log *zerolog.Logger,
	deviceID uuid.UUID,
	deviceDesc, deviceIP, ponKey, metricID, alertType, headline string,
	value float64,
	unit string,
) {
	th, label, ok := LoadGlobalGteMetricForDevice(ctx, pool, metricID, "olt")
	if !ok {
		return
	}
	sev := severityGteMetric(value, th)
	if metricID == "olt_pon_rx_dbm" {
		sev = capRxToWarning(sev)
	}
	metaKey := metricID + ":" + ponKey
	if sev == "ok" {
		closeOltPonOpticalAlert(ctx, pool, log, deviceID, alertType, metaKey)
		return
	}
	if alertignore.IsMuted(ctx, pool, deviceID, alertType, metaKey) {
		return
	}
	msg := fmt.Sprintf("%s (%s): PON %s — %s em %.2f %s (severidade: %s).",
		descOrEmpty(strings.TrimSpace(deviceDesc), "?"),
		addrOrEmpty(strings.TrimSpace(deviceIP), "?"),
		ponKey, label, value, unit, sev)
	base := map[string]any{
		"source":     "monitor_worker_olt",
		"key":        metaKey,
		"metric_id":  metricID,
		"pon":        ponKey,
		"value":      value,
		"value_text": fmt.Sprintf("%.2f %s", value, unit),
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

func closeOltPonOpticalAlert(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, deviceID uuid.UUID, alertType, key string) {
	_, _, _ = alertstore.Close(ctx, pool, log, alertstore.CloseSpec{
		DeviceID: deviceID, AlertType: alertType,
		Match: alertstore.Match{Kind: alertstore.MatchMetaKey, MetaKey: key},
		Resolved: map[string]any{
			"resolved": "pon_metric_within_limits", "source": "monitor_worker_olt", "key": key,
		},
	})
}
