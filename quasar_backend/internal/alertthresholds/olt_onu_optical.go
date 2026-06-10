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
	"github.com/netquasar/netquasar/quasar_backend/internal/alertignore"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertnotify"
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
		"source":            "monitor_worker_olt",
		"key":               key,
		"metric_id":         metricID,
		"pon":               ponKey,
		"value":             value,
		"streak":            streak,
		"required_streak":   onuOpticalCycles,
		"threshold_severity": sev,
	}, "optical_normal", "threshold_"+sev, nil)
	metaRaw, _ := json.Marshal(meta)
	var newID uuid.UUID
	err := pool.QueryRow(ctx, `
		INSERT INTO alert_instances (device_id, severity, alert_type, message, ip, device_name, meta)
		SELECT $1::uuid, $2::text, $3::text, $4, NULLIF(trim($5),''), NULLIF(trim($6),''), COALESCE($7::jsonb,'{}'::jsonb)
		WHERE NOT EXISTS (
			SELECT 1 FROM alert_instances ai
			WHERE ai.device_id = $1::uuid AND ai.alert_type = $3::text AND ai.closed_at IS NULL
			  AND (ai.meta->>'key') = ($7::jsonb->>'key')
		)
		RETURNING id
	`, deviceID, sev, alertType, msg, deviceIP, deviceDesc, metaRaw).Scan(&newID)
	if err == nil {
		alertnotify.SendMonitoringTelegramAndPatchMeta(ctx, pool, log, newID, strings.ToUpper(sev), title, msg)
		return
	}
	if err != pgx.ErrNoRows {
		return
	}
	patch, _ := json.Marshal(map[string]any{"value": value, "streak": streak, "source": "monitor_worker_olt"})
	_, _ = pool.Exec(ctx, `
		UPDATE alert_instances SET
			severity = $4::text,
			message = $5,
			meta = COALESCE(meta, '{}'::jsonb) || $6::jsonb
		WHERE device_id = $1::uuid AND alert_type = $2::text AND closed_at IS NULL
		  AND (meta->>'key') = $3
	`, deviceID, alertType, key, sev, msg, patch)
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
	metaPatch, _ := json.Marshal(map[string]any{"resolved": "optical_within_limits", "source": "monitor_worker_olt"})
	var aid uuid.UUID
	var msg string
	err := pool.QueryRow(ctx, `
		UPDATE alert_instances SET
			closed_at = now(),
			meta = COALESCE(meta, '{}'::jsonb) || $3::jsonb
		WHERE device_id = $1::uuid AND alert_type = $2::text AND closed_at IS NULL
		  AND (meta->>'key') = $4
		RETURNING id, message
	`, deviceID, alertType, metaPatch, key).Scan(&aid, &msg)
	if err != nil {
		return
	}
	alertnotify.SendResolutionTelegramAndPatchMeta(ctx, pool, log, aid, alertnotify.ResolutionHeadlineForAlertType(alertType), msg)
}

