package monitorworker

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertnotify"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertstore"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertthresholds"
	"github.com/rs/zerolog"
)

const alertTypeLatencyHigh = "latency_high"

// Limites heurísticos (ms) quando não existir critério global latency_ms.
const latencyHealthyMaxMS = int64(120)
const latencyDegradedMinMS = int64(280)

var latencyHighMatch = alertstore.Match{Kind: alertstore.MatchDeviceOnly}

func latencyReadingIsHigh(ctx context.Context, pool *pgxpool.Pool, deviceCategory string, reachOK bool, lat int64) bool {
	if !reachOK || lat <= 0 {
		return false
	}
	th, _, ok := alertthresholds.LoadGlobalGteMetricForDevice(ctx, pool, "latency_ms", deviceCategory)
	if ok {
		return alertthresholds.EvalMetricSeverity(float64(lat), th) != "ok"
	}
	return lat >= latencyDegradedMinMS
}

func latencyIsCalm(ctx context.Context, pool *pgxpool.Pool, deviceCategory string, lat int64) bool {
	if lat <= 0 {
		return false
	}
	th, _, ok := alertthresholds.LoadGlobalGteMetricForDevice(ctx, pool, "latency_ms", deviceCategory)
	if ok {
		return alertthresholds.EvalMetricSeverity(float64(lat), th) == "ok"
	}
	return lat < latencyHealthyMaxMS+40
}

func latencyHighStreakAfter(prev int, isHigh bool) int {
	if isHigh {
		return prev + 1
	}
	return 0
}

// EvaluateLatencyHighAlerts actualiza streak e gere alerta latency_high (abrir/atualizar/fechar).
// prevSampleLatMs: latência da coleta anterior (para média quando confirma na 2.ª leitura alta).
func EvaluateLatencyHighAlerts(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, deviceID uuid.UUID, deviceCategory, description, ip string, reachOK bool, currLat, prevSampleLatMs int64, prevHighStreak, requiredConsecutive int) int {
	requiredConsecutive = consecutiveLatencyRequired(requiredConsecutive)
	isHigh := latencyReadingIsHigh(ctx, pool, deviceCategory, reachOK, currLat)
	streakAfter := latencyHighStreakAfter(prevHighStreak, isHigh)

	if reachOK {
		refreshOpenLatencyHigh(ctx, pool, deviceID, description, ip, currLat)
	}

	if streakAfter >= requiredConsecutive && isHigh {
		reportLat := currLat
		if prevHighStreak > 0 && prevSampleLatMs > 0 && currLat > 0 {
			reportLat = (prevSampleLatMs + currLat) / 2
		}
		openOrUpdateLatencyHigh(ctx, pool, log, deviceID, deviceCategory, description, ip, reportLat, currLat, prevSampleLatMs, requiredConsecutive)
	} else if streakAfter == 0 && reachOK {
		closeLatencyHighIfCalm(ctx, pool, log, deviceID, deviceCategory, currLat)
	}
	return streakAfter
}

type latencyAlertContent struct {
	Severity string
	Message  string
	Meta     map[string]any
}

func buildLatencyHighContent(ctx context.Context, pool *pgxpool.Pool, deviceCategory, description, ip string, reportLat, currLat, prevSampleLatMs int64, requiredConsecutive int) (latencyAlertContent, bool) {
	requiredConsecutive = consecutiveLatencyRequired(requiredConsecutive)
	desc := strings.TrimSpace(description)
	addr := strings.TrimSpace(ip)
	baseMeta := map[string]any{
		"source":                    "monitor_worker",
		"metric_id":                 "latency_ms",
		"value":                     reportLat,
		"value_text":                strconv.FormatInt(reportLat, 10),
		"curr_latency_ms":           currLat,
		"probe_latency_ms":          currLat,
		"avg_latency_ms":            reportLat,
		"latency_high_streak":       requiredConsecutive,
		"consecutive_high_required": requiredConsecutive,
	}
	if prevSampleLatMs > 0 {
		baseMeta["prev_latency_ms"] = prevSampleLatMs
		baseMeta["latency_samples_ms"] = []int64{prevSampleLatMs, currLat}
	}

	th, label, hasGlobal := alertthresholds.LoadGlobalGteMetricForDevice(ctx, pool, "latency_ms", deviceCategory)
	if hasGlobal {
		sev := alertthresholds.EvalMetricSeverity(float64(reportLat), th)
		if sev == "ok" {
			return latencyAlertContent{}, false
		}
		msg := fmt.Sprintf("%s (%s): %s em %d ms (média de %d coletas) — %s.",
			descOr(desc, "?"), addrOr(addr, "?"), label, reportLat, requiredConsecutive, sev)
		meta := alertnotify.WithStatusTransition(baseMeta, "latency_normal", "threshold_"+sev, nil)
		return latencyAlertContent{Severity: sev, Message: msg, Meta: meta}, true
	}

	if reportLat < latencyDegradedMinMS {
		return latencyAlertContent{}, false
	}
	msg := fmt.Sprintf("%s (%s): latência ICMP/TCP em %d ms (média de %d coletas ≥ %d ms).",
		descOr(desc, "?"), addrOr(addr, "?"), reportLat, requiredConsecutive, latencyDegradedMinMS)
	meta := alertnotify.WithStatusTransition(baseMeta, "latency_normal", "latency_degraded", nil)
	return latencyAlertContent{Severity: "warning", Message: msg, Meta: meta}, true
}

func openOrUpdateLatencyHigh(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, deviceID uuid.UUID, deviceCategory, description, ip string, reportLat, currLat, prevSampleLatMs int64, requiredConsecutive int) {
	content, ok := buildLatencyHighContent(ctx, pool, deviceCategory, description, ip, reportLat, currLat, prevSampleLatMs, requiredConsecutive)
	if !ok {
		return
	}
	res, err := alertstore.OpenOrUpdate(ctx, pool, alertstore.OpenSpec{
		DeviceID: deviceID, Severity: content.Severity, AlertType: alertTypeLatencyHigh,
		Message: content.Message, IP: strings.TrimSpace(ip), DeviceName: strings.TrimSpace(description),
		Meta: content.Meta, Match: latencyHighMatch,
	}, &alertstore.NotifyCreate{
		Log: log, Level: strings.ToUpper(content.Severity), Headline: "Latência elevada",
	})
	if err != nil && log != nil {
		log.Error().Err(err).Str("device", deviceID.String()).Msg("alertstore latency_high")
		return
	}
	if res.Created && log != nil {
		log.Warn().Str("device", deviceID.String()).Int64("ms_avg", reportLat).Int64("ms_curr", currLat).Msg("alerta criado: latência elevada")
	}
}

func refreshOpenLatencyHigh(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID, description, ip string, currLat int64) {
	if currLat <= 0 {
		return
	}
	msg := fmt.Sprintf("%s (%s): latência ICMP/TCP em %d ms (acima do limiar).",
		descOr(strings.TrimSpace(description), "?"), addrOr(strings.TrimSpace(ip), "?"), currLat)
	patch := map[string]any{
		"source":           "monitor_worker",
		"curr_latency_ms":  currLat,
		"value":            currLat,
		"value_text":       strconv.FormatInt(currLat, 10),
		"probe_latency_ms": currLat,
	}
	_ = alertstore.PatchOpenMeta(ctx, pool, alertstore.OpenSpec{
		DeviceID: deviceID, AlertType: alertTypeLatencyHigh,
		Message: msg, Meta: patch, Match: latencyHighMatch,
	})
}

func closeLatencyHighIfCalm(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, deviceID uuid.UUID, deviceCategory string, currLat int64) {
	if !latencyIsCalm(ctx, pool, deviceCategory, currLat) {
		return
	}
	resolved := map[string]any{"resolved": "latency_normalized", "source": "monitor_worker"}
	if currLat > 0 {
		resolved["curr_latency_ms"] = currLat
		resolved["resolved_value"] = fmt.Sprintf("%d ms", currLat)
	}
	closed, _, err := alertstore.Close(ctx, pool, log, alertstore.CloseSpec{
		DeviceID: deviceID, AlertType: alertTypeLatencyHigh, Match: latencyHighMatch,
		Resolved: resolved,
	})
	if err != nil && log != nil {
		log.Error().Err(err).Str("device", deviceID.String()).Msg("fechar latency_high")
		return
	}
	if closed && log != nil {
		log.Info().Str("device", deviceID.String()).Msg("alerta latência encerrado — valores normalizados")
	}
}
