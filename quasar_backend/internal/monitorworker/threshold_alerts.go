package monitorworker

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/netquasar/netquasar/quasar_backend/internal/alertthresholds"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertignore"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertnotify"
	"github.com/rs/zerolog"
)

const alertTypeLatencyHigh = "latency_high"

// Limites heurísticos (ms): fallback quando não existir critério global para latência.
const latencyHealthyMaxMS = int64(120)
const latencyDegradedMinMS = int64(280)

// latencyHighConsecutiveRequired — alerta só após N leituras consecutivas acima do limiar.
const latencyHighConsecutiveRequired = 3

// patchOpenLatencyHighMeta grava a latência mais recente do probe em meta (curr_latency_ms / value)
// para alertas latency_high abertos acompanharem device_probe_cache.
func patchOpenLatencyHighMeta(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID, currLat int64) {
	if pool == nil {
		return
	}
	patch, _ := json.Marshal(map[string]any{
		"source":           "monitor_worker",
		"curr_latency_ms":  currLat,
		"value":            currLat,
		"value_text":       strconv.FormatInt(currLat, 10),
		"probe_latency_ms": currLat,
	})
	_, _ = pool.Exec(ctx, `
		UPDATE alert_instances SET
			meta = COALESCE(meta, '{}'::jsonb) || $3::jsonb
		WHERE device_id = $1::uuid AND alert_type = $2::text AND closed_at IS NULL
	`, deviceID, alertTypeLatencyHigh, patch)
}

func latencyReadingIsHigh(ctx context.Context, pool *pgxpool.Pool, deviceCategory string, reachOK bool, lat int64) bool {
	if !reachOK || lat <= 0 {
		return false
	}
	th, _, ok := alertthresholds.LoadGlobalGteMetricForDevice(ctx, pool, "latency_ms", deviceCategory)
	if ok {
		return thresholdSeverity(float64(lat), th) != "ok"
	}
	return lat >= latencyDegradedMinMS
}

func latencyHighStreakAfter(prev int, isHigh bool) int {
	if isHigh {
		return prev + 1
	}
	return 0
}

// EvaluateLatencyHighAlerts actualiza o streak de latência alta e só abre alerta após N leituras consecutivas.
func EvaluateLatencyHighAlerts(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, deviceID uuid.UUID, deviceCategory, description, ip string, reachOK bool, currLat int64, prevHighStreak int) int {
	streakAfter := latencyHighStreakAfter(prevHighStreak, latencyReadingIsHigh(ctx, pool, deviceCategory, reachOK, currLat))

	if reachOK {
		patchOpenLatencyHighMeta(ctx, pool, deviceID, currLat)
	}

	if streakAfter >= latencyHighConsecutiveRequired {
		if syncLatencyAlertByGlobalThreshold(ctx, pool, log, deviceID, deviceCategory, description, ip, reachOK, currLat) {
			return streakAfter
		}
		openLatencyHighAfterStreak(ctx, pool, log, deviceID, description, ip, currLat)
	} else if streakAfter == 0 {
		resolveLatencyHighIfCalm(ctx, pool, log, deviceID, reachOK, currLat)
	}
	return streakAfter
}

func openLatencyHighAfterStreak(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, deviceID uuid.UUID, description, ip string, currLat int64) {
	if currLat < latencyDegradedMinMS {
		return
	}
	desc := strings.TrimSpace(description)
	addr := strings.TrimSpace(ip)
	if alertignore.IsMuted(ctx, pool, deviceID, alertTypeLatencyHigh, "") {
		return
	}
	msg := fmt.Sprintf("%s (%s): latência ICMP/TCP em %d ms (≥ %d ms em %d leituras consecutivas).",
		descOr(desc, "?"), addrOr(addr, "?"), currLat, latencyDegradedMinMS, latencyHighConsecutiveRequired)
	meta := alertnotify.WithStatusTransition(map[string]any{
		"source":                    "monitor_worker",
		"curr_latency_ms":           currLat,
		"latency_high_streak":       latencyHighConsecutiveRequired,
		"consecutive_high_required": latencyHighConsecutiveRequired,
	}, "latency_normal", "latency_degraded", nil)
	metaRaw, _ := json.Marshal(meta)
	var alertID uuid.UUID
	err := pool.QueryRow(ctx, `
		INSERT INTO alert_instances (device_id, severity, alert_type, message, ip, device_name, meta)
		SELECT $1::uuid, 'warning', $2::text, $3, NULLIF(trim($4), ''), NULLIF(trim($5), ''),
			COALESCE($6::jsonb, '{}'::jsonb)
		WHERE NOT EXISTS (
			SELECT 1 FROM alert_instances ai
			WHERE ai.device_id = $1::uuid AND ai.alert_type = $2::text AND ai.closed_at IS NULL
		)
		RETURNING id
	`, deviceID, alertTypeLatencyHigh, msg, addr, desc, metaRaw).Scan(&alertID)
	if err != nil {
		if err == pgx.ErrNoRows {
			updMsg := fmt.Sprintf("%s (%s): latência ICMP/TCP em %d ms (acima do limiar).", descOr(desc, "?"), addrOr(addr, "?"), currLat)
			patch, _ := json.Marshal(map[string]any{
				"source":          "monitor_worker",
				"curr_latency_ms": currLat,
				"value":           currLat,
				"value_text":      strconv.FormatInt(currLat, 10),
			})
			_, _ = pool.Exec(ctx, `
				UPDATE alert_instances SET
					message = $3,
					meta = COALESCE(meta, '{}'::jsonb) || $4::jsonb
				WHERE device_id = $1::uuid AND alert_type = $2::text AND closed_at IS NULL
			`, deviceID, alertTypeLatencyHigh, updMsg, patch)
			return
		}
		log.Error().Err(err).Str("device", deviceID.String()).Msg("alert_instances insert latency_high")
		return
	}
	log.Warn().Str("device", deviceID.String()).Int64("ms", currLat).Int("streak", latencyHighConsecutiveRequired).Msg("alerta criado: latência elevada")
	alertnotify.SendMonitoringTelegramAndPatchMeta(ctx, pool, log, alertID, "WARNING", "Latência elevada", msg)
}

func resolveLatencyHighIfCalm(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, deviceID uuid.UUID, reachOK bool, currLat int64) {
	if !reachOK {
		return
	}
	if currLat >= latencyHealthyMaxMS+40 { // histerese
		return
	}
	var aid uuid.UUID
	var msg, dname, ip string
	err := pool.QueryRow(ctx, `
		UPDATE alert_instances SET
			closed_at = now(),
			meta = COALESCE(meta, '{}'::jsonb) || '{"resolved":"latency_normalized","source":"monitor_worker"}'::jsonb
		WHERE alert_type = $1
		  AND closed_at IS NULL
		  AND device_id = $2::uuid
		RETURNING id, message, device_name, ip
	`, alertTypeLatencyHigh, deviceID).Scan(&aid, &msg, &dname, &ip)
	if err != nil {
		if err == pgx.ErrNoRows {
			return
		}
		log.Error().Err(err).Msg("fechar alertas latency_high")
		return
	}
	log.Info().Str("device", deviceID.String()).Msg("alerta latência encerrado — valores voltaram ao normal")
	detail := msg
	if strings.TrimSpace(dname) != "" || strings.TrimSpace(ip) != "" {
		detail = fmt.Sprintf("%s — %s (%s)", msg, strings.TrimSpace(dname), strings.TrimSpace(ip))
	}
	alertnotify.SendResolutionTelegramAndPatchMeta(ctx, pool, log, aid, alertnotify.ResolutionHeadlineForAlertType(alertTypeLatencyHigh), detail)
}

func thresholdSeverity(v float64, t alertthresholds.GteMetricThreshold) string {
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

// syncLatencyAlertByGlobalThreshold usa a regra global (latency_ms). Se não existir, retorna false para fallback legado.
func syncLatencyAlertByGlobalThreshold(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, deviceID uuid.UUID, deviceCategory, description, ip string, reachOK bool, currLat int64) bool {
	if !reachOK {
		return false
	}
	th, label, ok := alertthresholds.LoadGlobalGteMetricForDevice(ctx, pool, "latency_ms", deviceCategory)
	if !ok {
		return false
	}
	sev := thresholdSeverity(float64(currLat), th)
	if sev == "ok" {
		resolveLatencyHighIfCalm(ctx, pool, log, deviceID, reachOK, 0)
		return true
	}
	if alertignore.IsMuted(ctx, pool, deviceID, alertTypeLatencyHigh, "") {
		return true
	}
	desc := strings.TrimSpace(description)
	addr := strings.TrimSpace(ip)
	msg := fmt.Sprintf("%s (%s): %s em %d ms (limiar %s; %d leituras consecutivas).",
		descOr(desc, "?"), addrOr(addr, "?"), label, currLat, sev, latencyHighConsecutiveRequired)
	meta := alertnotify.WithStatusTransition(map[string]any{
		"source":                    "monitor_worker",
		"metric_id":                 "latency_ms",
		"value":                     currLat,
		"value_text":                strconv.FormatInt(currLat, 10),
		"curr_latency_ms":           currLat,
		"probe_latency_ms":          currLat,
		"latency_high_streak":       latencyHighConsecutiveRequired,
		"consecutive_high_required": latencyHighConsecutiveRequired,
	}, "latency_normal", "threshold_"+sev, nil)
	metaRaw, _ := json.Marshal(meta)
	var aid uuid.UUID
	err := pool.QueryRow(ctx, `
		INSERT INTO alert_instances (device_id, severity, alert_type, message, ip, device_name, meta)
		SELECT $1::uuid, $2::text, $3::text, $4, NULLIF(trim($5),''), NULLIF(trim($6),''), COALESCE($7::jsonb, '{}'::jsonb)
		WHERE NOT EXISTS (
			SELECT 1 FROM alert_instances ai
			WHERE ai.device_id = $1::uuid AND ai.alert_type = $3::text AND ai.closed_at IS NULL
		)
		RETURNING id
	`, deviceID, sev, alertTypeLatencyHigh, msg, addr, desc, metaRaw).Scan(&aid)
	if err == nil {
		alertnotify.SendMonitoringTelegramAndPatchMeta(ctx, pool, log, aid, strings.ToUpper(sev), "Latência elevada", msg)
		return true
	}
	if err != pgx.ErrNoRows {
		return true
	}
	metaPatch := map[string]any{
		"source":           "monitor_worker",
		"metric_id":        "latency_ms",
		"value":            currLat,
		"value_text":       strconv.FormatInt(currLat, 10),
		"curr_latency_ms":  currLat,
		"probe_latency_ms": currLat,
	}
	patchRaw, _ := json.Marshal(metaPatch)
	_, _ = pool.Exec(ctx, `
		UPDATE alert_instances SET
			severity = $4::text,
			message = $5,
			meta = COALESCE(meta, '{}'::jsonb) || $6::jsonb
		WHERE device_id = $1::uuid AND alert_type = $2::text AND closed_at IS NULL
	`, deviceID, alertTypeLatencyHigh, sev, msg, patchRaw)
	return true
}
