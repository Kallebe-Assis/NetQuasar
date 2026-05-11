package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertnotify"
	"github.com/netquasar/netquasar/quasar_backend/internal/monitorworker"
	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

func queryTruthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func (s *Server) createSuppression(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ScopeType string     `json:"scope_type"`
		ScopeRef  string     `json:"scope_ref"`
		Reason    string     `json:"reason"`
		CreatedBy *uuid.UUID `json:"created_by"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	if body.ScopeType == "" || body.ScopeRef == "" || body.Reason == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "scope_type, scope_ref e reason obrigatórios", nil)
		return
	}
	var id uuid.UUID
	err := s.DB().QueryRow(r.Context(), `
		INSERT INTO alert_suppressions (scope_type, scope_ref, reason, created_by) VALUES ($1,$2,$3,$4) RETURNING id
	`, body.ScopeType, body.ScopeRef, body.Reason, body.CreatedBy).Scan(&id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (s *Server) listSuppressions(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB().Query(r.Context(), `
		SELECT id, scope_type, scope_ref, reason, created_by, created_at FROM alert_suppressions ORDER BY created_at DESC LIMIT 500
	`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	var list []map[string]any
	for rows.Next() {
		var id uuid.UUID
		var st, sr, reason string
		var cb *uuid.UUID
		var created time.Time
		if err := rows.Scan(&id, &st, &sr, &reason, &cb, &created); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		list = append(list, map[string]any{
			"id": id, "scope_type": st, "scope_ref": sr, "reason": reason, "created_by": cb, "created_at": created,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"suppressions": list})
}

func (s *Server) getSuppression(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	var st, sr, reason string
	var cb *uuid.UUID
	var created time.Time
	err = s.DB().QueryRow(r.Context(), `
		SELECT scope_type, scope_ref, reason, created_by, created_at FROM alert_suppressions WHERE id=$1
	`, id).Scan(&st, &sr, &reason, &cb, &created)
	if err == pgx.ErrNoRows {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "", nil)
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id": id, "scope_type": st, "scope_ref": sr, "reason": reason, "created_by": cb, "created_at": created,
	})
}

func (s *Server) patchSuppression(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	var body struct {
		ScopeType *string `json:"scope_type"`
		ScopeRef  *string `json:"scope_ref"`
		Reason    *string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	ct, err := s.DB().Exec(r.Context(), `
		UPDATE alert_suppressions SET
			scope_type = COALESCE($2, scope_type),
			scope_ref = COALESCE($3, scope_ref),
			reason = COALESCE($4, reason)
		WHERE id=$1
	`, id, body.ScopeType, body.ScopeRef, body.Reason)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if ct.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "", nil)
		return
	}
	s.getSuppression(w, r)
}

func (s *Server) deleteSuppression(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	ct, err := s.DB().Exec(r.Context(), `DELETE FROM alert_suppressions WHERE id=$1`, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if ct.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "", nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) mapEquipmentPoints(w http.ResponseWriter, r *http.Request) {
	q := `
		SELECT d.id, d.description, d.category, d.latitude, d.longitude, host(d.ip)::text, d.pop_id, d.operational_mode,
			COALESCE(c.ok, false) AS up, c.checked_at
		FROM devices d
		LEFT JOIN device_probe_cache c ON c.device_id = d.id
		WHERE d.latitude IS NOT NULL AND d.longitude IS NOT NULL
	`
	args := []any{}
	n := 1
	if v := r.URL.Query().Get("pop_id"); v != "" {
		if id, err := uuid.Parse(v); err == nil {
			q += ` AND d.pop_id = $` + strconv.Itoa(n)
			args = append(args, id)
			n++
		}
	}
	if v := strings.TrimSpace(r.URL.Query().Get("category")); v != "" {
		q += ` AND lower(d.category) = lower($` + strconv.Itoa(n) + `)`
		args = append(args, v)
		n++
	}
	if v := strings.TrimSpace(r.URL.Query().Get("q")); v != "" {
		pat := "%" + v + "%"
		q += ` AND (d.description ILIKE $` + strconv.Itoa(n) + ` OR host(d.ip)::text ILIKE $` + strconv.Itoa(n+1) + `)`
		args = append(args, pat, pat)
		n += 2
	}
	if mn, err := strconv.ParseFloat(r.URL.Query().Get("min_lat"), 64); err == nil {
		q += ` AND d.latitude >= $` + strconv.Itoa(n)
		args = append(args, mn)
		n++
	}
	if mx, err := strconv.ParseFloat(r.URL.Query().Get("max_lat"), 64); err == nil {
		q += ` AND d.latitude <= $` + strconv.Itoa(n)
		args = append(args, mx)
		n++
	}
	if mn, err := strconv.ParseFloat(r.URL.Query().Get("min_lon"), 64); err == nil {
		q += ` AND d.longitude >= $` + strconv.Itoa(n)
		args = append(args, mn)
		n++
	}
	if mx, err := strconv.ParseFloat(r.URL.Query().Get("max_lon"), 64); err == nil {
		q += ` AND d.longitude <= $` + strconv.Itoa(n)
		args = append(args, mx)
		n++
	}
	q += ` ORDER BY d.description LIMIT 2000`
	rows, err := s.DB().Query(r.Context(), q, args...)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	var pts []map[string]any
	for rows.Next() {
		var id uuid.UUID
		var desc, cat, op string
		var lat, lon float64
		var ip *string
		var popID *uuid.UUID
		var up bool
		var checked *time.Time
		if err := rows.Scan(&id, &desc, &cat, &lat, &lon, &ip, &popID, &op, &up, &checked); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		st := "unknown"
		if checked != nil {
			if up {
				st = "online"
			} else {
				st = "offline"
			}
		}
		pts = append(pts, map[string]any{
			"id": id, "description": desc, "category": cat, "lat": lat, "lng": lon,
			"ip": ip, "pop_id": popID, "operational_mode": op, "status": st, "last_check_at": checked,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"points": pts})
}

func (s *Server) pingRunStub(w http.ResponseWriter, r *http.Request) {
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
	_ = s.DB().QueryRow(ctxBase, `SELECT COALESCE(ping_fail_streak, 0) FROM device_probe_cache WHERE device_id=$1`, id).Scan(&prevStreak)
	streakAfter := 0
	if !okb {
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
		if err := s.DB().QueryRow(ctxBase, `
			UPDATE alert_instances SET
				closed_at = now(),
				meta = COALESCE(meta, '{}'::jsonb) || '{"resolved":"reach_ok","source":"api_ping"}'::jsonb
			WHERE alert_type = 'ping_unreachable'
			  AND closed_at IS NULL
			  AND device_id = $1::uuid
			RETURNING id, message, alert_type
		`, id).Scan(&aid, &msg, &at); err == nil {
			alertnotify.SendResolutionTelegramAndPatchMeta(ctxBase, s.DB(), &s.Log, aid, alertnotify.ResolutionHeadlineForAlertType(at), msg)
		}
	} else if attemptsUsed >= offlineThreshold {
		l := s.Log.With().Str("route", "ping_run").Logger()
		monitorworker.InsertPingUnreachableIfNew(ctxBase, s.DB(), &l, id, deviceDesc, host, probe, "api_ping")
	}

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
