package alertthresholds

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertignore"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertnotify"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertstore"
	"github.com/rs/zerolog"
)

const (
	alertTypeOltOnuRx = "olt_onu_rx"
	alertTypeOltOnuTx = "olt_onu_tx"
	onuOpticalCycles  = 3
)

func EvaluateOltOnuOpticalThreshold(
	ctx context.Context,
	pool *pgxpool.Pool,
	log *zerolog.Logger,
	deviceID uuid.UUID,
	deviceDesc, deviceIP, ponKey, metricID string,
	value float64,
) {
	th, label, ok := LoadGlobalGteMetricForDevice(ctx, pool, metricID, "olt")
	if !ok {
		return
	}
	sev := severityGteMetric(value, th)
	streak := updateOnuOpticalStreak(ctx, pool, deviceID, ponKey, metricID, sev != "ok", value)
	alertType := alertTypeOltOnuRx
	title := "ONU RX abaixo do limiar"
	if metricID == "olt_onu_tx_dbm" {
		alertType = alertTypeOltOnuTx
		title = "ONU TX abaixo do limiar"
	}
	key := metricID + ":" + ponKey
	if sev == "ok" {
		closeOltOnuOpticalAlert(ctx, pool, log, deviceID, alertType, key)
		return
	}
	if streak < onuOpticalCycles {
		return
	}
	if alertignore.IsMuted(ctx, pool, deviceID, alertType, key) {
		return
	}
	msg := fmt.Sprintf("%s (%s): PON %s — %s em %.2f dBm (%s por %d ciclos).",
		descOrEmpty(strings.TrimSpace(deviceDesc), "?"),
		addrOrEmpty(strings.TrimSpace(deviceIP), "?"),
		ponKey, label, value, sev, onuOpticalCycles)
	meta := alertnotify.WithStatusTransition(map[string]any{
		"source":             "monitor_worker_olt",
		"key":                key,
		"metric_id":          metricID,
		"pon":                ponKey,
		"value":              value,
		"value_text":         fmt.Sprintf("%.2f dBm", value),
		"streak":             streak,
		"required_streak":    onuOpticalCycles,
		"threshold_severity": sev,
	}, "optical_normal", "threshold_"+sev, nil)
	_, err := alertstore.OpenOrUpdate(ctx, pool, alertstore.OpenSpec{
		DeviceID: deviceID, Severity: sev, AlertType: alertType,
		Message: msg, IP: deviceIP, DeviceName: deviceDesc, Meta: meta,
		Match: alertstore.Match{Kind: alertstore.MatchMetaKey, MetaKey: key},
	}, &alertstore.NotifyCreate{
		Log: log, Level: strings.ToUpper(sev), Headline: title,
	})
	if err != nil && log != nil {
		log.Error().Err(err).Str("device", deviceID.String()).Msg("alertstore olt_onu_optical")
	}
}

func updateOnuOpticalStreak(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID, ponKey, metricID string, bad bool, value float64) int {
	next := 0
	if bad {
		next = 1
	}
	_ = pool.QueryRow(ctx, `
		INSERT INTO olt_onu_optical_streak (device_id, pon_key, metric_id, streak, last_value, updated_at)
		VALUES ($1::uuid, $2::text, $3::text, $4::int, $5::float8, now())
		ON CONFLICT (device_id, pon_key, metric_id)
		DO UPDATE SET
			streak = CASE WHEN $4::int > 0 THEN olt_onu_optical_streak.streak + 1 ELSE 0 END,
			last_value = $5::float8,
			updated_at = now()
		RETURNING streak
	`, deviceID, strings.TrimSpace(ponKey), strings.TrimSpace(metricID), next, value).Scan(&next)
	return next
}

func EvaluateOltOnuOpticalFromPons(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, deviceID uuid.UUID, desc, ip string, pons []map[string]any) {
	for _, p := range pons {
		key := strings.TrimSpace(fmt.Sprint(p["pon_compact"]))
		if key == "" {
			key = strings.TrimSpace(fmt.Sprint(p["id"]))
		}
		if key == "" {
			key = strings.TrimSpace(fmt.Sprint(p["name"]))
		}
		if key == "" {
			continue
		}
		if rx, ok := parseNum(p["rx_dbm"]); ok {
			EvaluateOltOnuOpticalThreshold(ctx, pool, log, deviceID, desc, ip, key, "olt_onu_rx_dbm", rx)
		}
		if tx, ok := parseNum(p["tx_dbm"]); ok {
			EvaluateOltOnuOpticalThreshold(ctx, pool, log, deviceID, desc, ip, key, "olt_onu_tx_dbm", tx)
		}
	}
}

func parseNum(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case float32:
		return float64(t), true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(strings.ReplaceAll(t, ",", ".")), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func closeOltOnuOpticalAlert(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, deviceID uuid.UUID, alertType, key string) {
	_, _, _ = alertstore.Close(ctx, pool, log, alertstore.CloseSpec{
		DeviceID: deviceID, AlertType: alertType,
		Match: alertstore.Match{Kind: alertstore.MatchMetaKey, MetaKey: key},
		Resolved: map[string]any{
			"resolved": "optical_within_limits", "source": "monitor_worker_olt", "key": key,
		},
	})
}

