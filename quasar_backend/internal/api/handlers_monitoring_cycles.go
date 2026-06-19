package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/netquasar/netquasar/quasar_backend/internal/monitorworker"
)

type monitoringCycleRequest struct {
	DeviceID *uuid.UUID `json:"device_id"`
	Force    bool       `json:"force"`
}

func decodeMonitoringCycleBody(r *http.Request) (monitoringCycleRequest, error) {
	var req monitoringCycleRequest
	if r.Body == nil {
		return req, nil
	}
	b, err := io.ReadAll(r.Body)
	if err != nil {
		return req, err
	}
	if len(b) == 0 {
		return req, nil
	}
	if err := json.Unmarshal(b, &req); err != nil {
		return req, err
	}
	return req, nil
}

func (s *Server) monitoringRuntimeMode(ctx context.Context) (string, error) {
	var mode string
	err := s.DB().QueryRow(ctx, `
		SELECT COALESCE(NULLIF(TRIM(monitoring_mode), ''), 'off')
		FROM monitoring_runtime WHERE id=1`).Scan(&mode)
	return mode, err
}

// monitoringCycleKinds lista ciclos suportados e intervalos actuais (para UI ou integrações).
func (s *Server) monitoringCycleKinds(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var ps, tm, pto int
	var telSecRaw, ifaceSec, oltDerivedSec int
	var icmpPB, offTh, uptimeRestart int
	if err := s.DB().QueryRow(ctx, `
		SELECT ping_seconds, telemetry_minutes, ping_timeout_ms,
			telemetry_seconds,
			interface_snapshot_seconds, olt_if_derived_pon_seconds,
			icmp_payload_bytes, offline_ping_fail_threshold,
			COALESCE(uptime_restart_alert_minutes, 0)
		FROM monitoring_intervals WHERE id=1`).Scan(&ps, &tm, &pto, &telSecRaw, &ifaceSec, &oltDerivedSec, &icmpPB, &offTh, &uptimeRestart); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	telSec := monitorworker.ResolveTelemetrySeconds(telSecRaw, tm)
	kinds := monitorworker.ListCycleKinds()
	writeJSON(w, http.StatusOK, map[string]any{
		"kinds": kinds,
		"intervals_seconds": map[string]int{
			"ping":                      ps,
			"telemetry":                 telSec,
			"interface_snapshot":        ifaceSec,
			"olt_if_derived_pon":        oltDerivedSec,
		},
		"ping_timeout_ms":               pto,
		"icmp_payload_bytes":            icmpPB,
		"offline_ping_fail_threshold":   offTh,
		"uptime_restart_alert_minutes": uptimeRestart,
		"telemetry_minutes":             tm,
		"post_cycle":                    "POST /api/v1/monitoring/cycles/{cycle} com JSON opcional {\"device_id\":\"uuid\",\"force\":true}",
		"aliases":                       "latency: ping, icmp | telemetry: snmp, metrics | interfaces: iface | olt-if-derived: pon, olt_pon",
	})
}

// monitoringCycleRun despacha POST /api/v1/monitoring/cycles/{cycle} (slug dinâmico + aliases).
func (s *Server) monitoringCycleRun(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	raw := strings.TrimSpace(chi.URLParam(r, "cycle"))
	slug, err := monitorworker.NormalizeCycleSlug(raw)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_CYCLE", err.Error(), map[string]any{
			"kinds_endpoint": "/api/v1/monitoring/cycles/kinds",
		})
		return
	}

	mode, err := s.monitoringRuntimeMode(ctx)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}

	body, err := decodeMonitoringCycleBody(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	opts := monitorworker.SweepOpts{Force: body.Force, Source: "api", DeviceID: body.DeviceID}

	if slug != monitorworker.CycleSlugLatency {
		if mode != monitorworker.ModeFull {
			writeErr(w, http.StatusUnprocessableEntity, "MODE", "Este ciclo requer monitoring_mode=full.", map[string]any{"cycle": slug, "current": mode})
			return
		}
	}

	effectiveMode := mode
	if slug == monitorworker.CycleSlugLatency && (effectiveMode == monitorworker.ModeOff || effectiveMode == "") {
		effectiveMode = monitorworker.ModeSimplePing
	}

	if err := monitorworker.EnforceAPICycleInterval(ctx, s.DB(), slug, opts); err != nil {
		if errors.Is(err, monitorworker.ErrCycleIntervalNotElapsed) {
			writeErr(w, http.StatusTooManyRequests, "INTERVAL", err.Error(), map[string]any{"cycle": slug, "hint": "use force=true para ignorar o intervalo"})
			return
		}
		writeErr(w, http.StatusBadRequest, "GUARD", err.Error(), nil)
		return
	}

	if slug == monitorworker.CycleSlugLatency {
		if !monitorworker.TryLockLatencyCycle() {
			writeErr(w, http.StatusConflict, "BUSY", monitorworker.ErrLatencyCycleBusy.Error(), map[string]any{"cycle": slug})
			return
		}
		defer monitorworker.UnlockLatencyCycle()
	} else if slug == monitorworker.CycleSlugTelemetry {
		if !monitorworker.TryLockTelemetryCycle() {
			writeErr(w, http.StatusConflict, "BUSY", monitorworker.ErrTelemetryCycleBusy.Error(), map[string]any{"cycle": slug})
			return
		}
		defer monitorworker.UnlockTelemetryCycle()
	} else if slug == monitorworker.CycleSlugOltIfDerived {
		if !monitorworker.TryLockOltPonCycle() {
			writeErr(w, http.StatusConflict, "BUSY", monitorworker.ErrOltPonCycleBusy.Error(), map[string]any{"cycle": slug})
			return
		}
		defer monitorworker.UnlockOltPonCycle()
	} else {
		if !monitorworker.TryLockMonitoringPipeline() {
			writeErr(w, http.StatusConflict, "BUSY", monitorworker.ErrPipelineBusy.Error(), map[string]any{"cycle": slug})
			return
		}
		defer monitorworker.UnlockMonitoringPipeline()
	}

	l := s.Log.With().Str("route", "monitoring/cycles").Str("cycle", slug).Logger()
	sweepErr := monitorworker.RunMonitorCycleBySlug(ctx, s.DB(), &l, slug, effectiveMode, opts)
	if sweepErr != nil {
		writeErr(w, http.StatusInternalServerError, "SWEEP", sweepErr.Error(), nil)
		return
	}

	s.appendAuditLog(r.Context(), "monitoring_cycle", slug, "run", s.actorFromRequest(r), nil, map[string]any{
		"cycle":                       slug,
		"requested_slug":              raw,
		"source":                      "api",
		"monitoring_mode_requested":   mode,
		"monitoring_mode_effective":   effectiveMode,
		"device_id":                   body.DeviceID,
		"force":                       body.Force,
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"cycle":                       slug,
		"requested_slug":              raw,
		"accepted":                    true,
		"monitoring_mode_requested":     mode,
		"monitoring_mode_effective":   effectiveMode,
		"device_id":                   body.DeviceID,
		"force":                       body.Force,
	})
}
