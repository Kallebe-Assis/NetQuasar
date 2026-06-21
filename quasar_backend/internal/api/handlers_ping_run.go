package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertnotify"
	"github.com/netquasar/netquasar/quasar_backend/internal/monitorworker"
	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

func (s *Server) pingRunDevice(w http.ResponseWriter, r *http.Request) {
	ctxBase := r.Context()
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	port := strings.TrimSpace(r.URL.Query().Get("port"))
	if port == "" {
		port = strings.TrimSpace(r.URL.Query().Get("tcp_port"))
	}
	if port == "" {
		port = "443"
	}
	icmpOnly := queryTruthy(r.URL.Query().Get("icmp_only"))

	var pingTimeoutMs, icmpPayloadBytes, offlineThreshold int
	if err := s.DB().QueryRow(ctxBase, `
		SELECT ping_timeout_ms, icmp_payload_bytes, offline_ping_fail_threshold
		FROM monitoring_intervals WHERE id=1`).Scan(&pingTimeoutMs, &icmpPayloadBytes, &offlineThreshold); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if pingTimeoutMs < 1000 {
		pingTimeoutMs = 1000
	}
	if pingTimeoutMs > 30000 {
		pingTimeoutMs = 30000
	}
	if offlineThreshold < 1 {
		offlineThreshold = 3
	}
	if offlineThreshold > 50 {
		offlineThreshold = 50
	}
	icmpPayloadBytes = probing.ClampICMPPayloadBytes(icmpPayloadBytes)

	to := time.Duration(pingTimeoutMs) * time.Millisecond
	var icmpPart, tcpPart time.Duration
	if icmpOnly {
		icmpPart = to
		tcpPart = 0
	} else {
		icmpPart = to * 2 / 3
		if icmpPart < 500*time.Millisecond {
			icmpPart = 500 * time.Millisecond
		}
		tcpPart = to - icmpPart
		if tcpPart < time.Second {
			tcpPart = time.Second
		}
	}

	var ipStr *string
	var deviceDesc string
	err = s.DB().QueryRow(ctxBase, `SELECT host(ip)::text, COALESCE(trim(description),'') FROM devices WHERE id=$1`, id).Scan(&ipStr, &deviceDesc)
	if err == pgx.ErrNoRows {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "equipamento não encontrado", nil)
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if ipStr == nil || *ipStr == "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "device_id": id, "error": "equipamento sem IP"})
		return
	}
	host := strings.TrimSpace(*ipStr)

	force := queryTruthy(r.URL.Query().Get("force"))
	if err := monitorworker.EnforceAPICycleInterval(ctxBase, s.DB(), monitorworker.CycleSlugLatency, monitorworker.SweepOpts{
		Source: "api", DeviceID: &id, Force: force,
	}); err != nil {
		if errors.Is(err, monitorworker.ErrCycleIntervalNotElapsed) {
			writeErr(w, http.StatusTooManyRequests, "INTERVAL", err.Error(), map[string]any{"hint": "adicione ?force=true ou aguarde ping_seconds"})
			return
		}
		writeErr(w, http.StatusBadRequest, "GUARD", err.Error(), nil)
		return
	}

	var probe map[string]any
	attemptsUsed := 0
	for attempt := 1; attempt <= offlineThreshold; attempt++ {
		perCtx, cancel := context.WithTimeout(ctxBase, to+500*time.Millisecond)
		if icmpOnly {
			probe = probing.HostReachabilityICMPOnly(perCtx, host, icmpPart, icmpPayloadBytes)
		} else {
			probe = probing.HostReachability(perCtx, host, port, icmpPart, tcpPart, icmpPayloadBytes)
		}
		cancel()
		attemptsUsed = attempt
		if ok, _ := probe["ok"].(bool); ok {
			break
		}
	}

	resp := map[string]any{
		"device_id":                  id,
		"source":                     "live_api",
		"icmp_only":                  icmpOnly,
		"host":                       probe["host"],
		"ok":                         probe["ok"],
		"method":                     probe["method"],
		"latency_ms":                 probe["latency_ms"],
		"icmp":                       probe["icmp"],
		"ping_timeout_ms":            pingTimeoutMs,
		"icmp_payload_bytes":         icmpPayloadBytes,
		"offline_ping_fail_threshold": offlineThreshold,
		"ping_attempts_used":         attemptsUsed,
	}
	if !icmpOnly {
		resp["tcp_fallback"] = probe["tcp_fallback"]
	}

	okb, _ := probe["ok"].(bool)

	monMode := "simple_ping"
	_ = s.DB().QueryRow(ctxBase, `SELECT COALESCE(NULLIF(TRIM(monitoring_mode), ''), 'simple_ping') FROM monitoring_runtime WHERE id=1`).Scan(&monMode)
	if monMode == "" {
		monMode = "simple_ping"
	}

	var prevStreak int
	var eligibleForAlerts bool
	_ = s.DB().QueryRow(ctxBase, `
		SELECT COALESCE(c.ping_fail_streak, 0),
			(d.ping_enabled = true
			 AND TRIM(BOTH FROM COALESCE(d.operational_mode, '')) = 'Ativo'
			 AND TRIM(BOTH FROM COALESCE(d.network_status, '')) = 'Normal'
			 AND d.ip IS NOT NULL
			 AND TRIM(BOTH FROM host(d.ip)::text) <> '')
		FROM devices d
		LEFT JOIN device_probe_cache c ON c.device_id = d.id
		WHERE d.id = $1
	`, id).Scan(&prevStreak, &eligibleForAlerts)
	streakAfter := 0
	if !okb && eligibleForAlerts {
		streakAfter = prevStreak + 1
	}

	pingMeta := map[string]any{
		"ping_run": map[string]any{
			"attempts_used":        attemptsUsed,
			"max_attempts":         offlineThreshold,
			"ping_timeout_ms":      pingTimeoutMs,
			"icmp_payload_bytes":   icmpPayloadBytes,
			"icmp_only":            icmpOnly,
		},
	}
	lProbe := s.Log.With().Str("route", "ping_run").Logger()
	if err := monitorworker.UpsertSingleDeviceLatencyProbe(ctxBase, s.DB(), &lProbe, id, monMode, probe, streakAfter, "api_ping", pingMeta); err != nil {
		resp["cache_update_error"] = err.Error()
	}

	if okb {
		var aid uuid.UUID
		var msg, at string
		closeMeta := map[string]any{"resolved": "reach_ok", "source": "api_ping"}
		if lat, ok := probe["latency_ms"]; ok {
			switch n := lat.(type) {
			case int64:
				if n > 0 {
					closeMeta["curr_latency_ms"] = n
					closeMeta["resolved_value"] = fmt.Sprintf("%d ms", n)
				}
			case float64:
				if n > 0 {
					closeMeta["curr_latency_ms"] = int64(n)
					closeMeta["resolved_value"] = fmt.Sprintf("%d ms", int64(n))
				}
			case int:
				if n > 0 {
					closeMeta["curr_latency_ms"] = int64(n)
					closeMeta["resolved_value"] = fmt.Sprintf("%d ms", n)
				}
			}
		}
		metaClose, _ := json.Marshal(closeMeta)
		if err := s.DB().QueryRow(ctxBase, `
			UPDATE alert_instances SET
				closed_at = now(),
				meta = COALESCE(meta, '{}'::jsonb) || $2::jsonb
			WHERE alert_type = 'ping_unreachable'
			  AND closed_at IS NULL
			  AND device_id = $1::uuid
			RETURNING id, message, alert_type
		`, id, metaClose).Scan(&aid, &msg, &at); err == nil {
			alertnotify.SendResolutionTelegramAndPatchMeta(ctxBase, s.DB(), &s.Log, aid, alertnotify.ResolutionHeadlineForAlertType(at), msg)
		}
	} else if monitorworker.ShouldOpenPingUnreachableAlert(okb, streakAfter, monitorworker.ConsecutivePingsRequired(offlineThreshold)) && eligibleForAlerts {
		l := s.Log.With().Str("route", "ping_run").Logger()
		monitorworker.InsertPingUnreachableIfNewForMonitoredDevice(ctxBase, s.DB(), &l, id, deviceDesc, host, probe, "api_ping")
	}

	s.auditDeviceAction(ctxBase, r, id, "ping_run", map[string]any{
		"description": deviceDesc,
		"host":        host,
		"ok":          okb,
		"latency_ms":  probe["latency_ms"],
		"method":      probe["method"],
	})
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) overviewSummaryStub(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var devices, pops int64
	var clients int64
	var monRunning bool
	_ = s.DB().QueryRow(ctx, `SELECT COUNT(*) FROM devices`).Scan(&devices)
	_ = s.DB().QueryRow(ctx, `SELECT COUNT(*) FROM pops`).Scan(&pops)
	_ = s.DB().QueryRow(ctx, `
		SELECT COALESCE(SUM(client_count), 0)::bigint FROM commercial_monthly_records
		WHERE year_month = to_char((CURRENT_TIMESTAMP AT TIME ZONE 'UTC'), 'YYYY-MM')
	`).Scan(&clients)
	_ = s.DB().QueryRow(ctx, `SELECT is_running FROM monitoring_runtime WHERE id=1`).Scan(&monRunning)

	writeJSON(w, http.StatusOK, map[string]any{
		"devices":              devices,
		"pops":                 pops,
		"commercial_clients_sum": clients,
		"monitoring_running":   monRunning,
	})
}

