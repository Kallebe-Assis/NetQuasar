package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertignore"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertnotify"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertthresholds"
	"github.com/netquasar/netquasar/quasar_backend/internal/monitorworker"
)

func (s *Server) alertsActive(w http.ResponseWriter, r *http.Request) {
	sev := r.URL.Query().Get("severity")
	typ := r.URL.Query().Get("type")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 5000 {
		limit = 5000
	}
	suppressOltOnuDrop := !alertthresholds.OltOnuQuantityAlertsEnabled(r.Context(), s.DB())
	if suppressOltOnuDrop && (typ == "olt_onu_drop" || typ == "olt_onu_rise") {
		writeJSON(w, http.StatusOK, map[string]any{"alerts": []any{}})
		return
	}
	// Abertos + instâncias fechadas há pouco (grace de 1 min na UI antes de sumirem desta lista).
	q := `
		SELECT u.id, u.device_id, u.severity, u.alert_type, u.message, u.ip, u.device_name,
			u.active_since, u.closed_at, u.meta::text, u.incident_id,
			COALESCE(NULLIF(trim(p.description), ''), '') AS pop_name
		FROM (
			SELECT a.id, a.device_id, a.severity, a.alert_type, a.message, a.ip,
				COALESCE(NULLIF(trim(a.device_name), ''), NULLIF(trim(d.description), '')) AS device_name,
				a.active_since, a.closed_at, a.meta, a.incident_id
			FROM alert_instances a
			LEFT JOIN devices d ON d.id = a.device_id
			WHERE a.closed_at IS NULL
			AND NOT EXISTS (
				SELECT 1 FROM alert_suppressions s
				WHERE (s.scope_type = 'device' AND s.scope_ref = a.device_id::text)
				   OR (s.scope_type = 'pop' AND s.scope_ref = 'all')
				   OR (s.scope_type = '*' AND s.scope_ref = '*')
			)
			AND NOT EXISTS (
				SELECT 1 FROM maintenance_windows m
				WHERE now() BETWEEN m.starts_at AND m.ends_at
				  AND (
						m.scope_type = 'global'
						OR (m.scope_type = 'device' AND m.device_id = a.device_id)
						OR (m.scope_type = 'pop' AND m.pop_id IN (SELECT d.pop_id FROM devices d WHERE d.id = a.device_id))
				  )
				  AND m.status IN ('planned', 'running')
			)
			AND (
				a.alert_type <> 'ping_unreachable'
				OR a.device_id IS NULL
				OR (d.id IS NOT NULL AND `+monitorworker.SQLDeviceEligibleForPingAlerts+`)
			)
			`+alertignore.SQLActiveIgnoreNotExists+`
			UNION ALL
			SELECT a.id, a.device_id, a.severity, a.alert_type, a.message, a.ip, a.device_name, a.active_since, a.closed_at, a.meta, a.incident_id
			FROM alert_instances a
			WHERE a.closed_at IS NOT NULL AND a.closed_at >= now() - interval '1 minute'
		) u
		LEFT JOIN devices d ON d.id = u.device_id
		LEFT JOIN pops p ON p.id = d.pop_id
		WHERE 1=1
	`
	args := []any{}
	n := 1
	if sev != "" {
		q += ` AND u.severity = $` + strconv.Itoa(n)
		args = append(args, sev)
		n++
	}
	if typ != "" {
		q += ` AND u.alert_type = $` + strconv.Itoa(n)
		args = append(args, typ)
		n++
	}
	q += ` ORDER BY COALESCE(u.closed_at, u.active_since) DESC LIMIT ` + strconv.Itoa(limit)
	rows, err := s.DB().Query(r.Context(), q, args...)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	var list []map[string]any
	for rows.Next() {
		var id, devID uuid.UUID
		var sev, typ, msg, ip, dname, popName string
		var since time.Time
		var closed *time.Time
		var meta []byte
		var incidentID *uuid.UUID
		if err := rows.Scan(&id, &devID, &sev, &typ, &msg, &ip, &dname, &since, &closed, &meta, &incidentID, &popName); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		if suppressOltOnuDrop && (typ == "olt_onu_drop" || typ == "olt_onu_rise") {
			continue
		}
		item := map[string]any{
			"id": id, "device_id": devID, "severity": sev, "type": typ, "message": msg,
			"ip": ip, "device_name": dname, "active_since": since, "meta": json.RawMessage(meta),
			"pop_name": popName,
		}
		if incidentID != nil {
			item["incident_id"] = *incidentID
		}
		if closed != nil {
			item["closed_at"] = *closed
		}
		list = append(list, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"alerts": list})
}

func (s *Server) alertsRevalidate(w http.ResponseWriter, r *http.Request) {
	// ping_unreachable: fecha se o probe está OK ou se o equipamento não entra na monitorização activa
	// (ex.: inativo, ping desligado, rede Bridge) — evita alertas por ping manual acidental.
	rows, err := s.DB().Query(r.Context(), `
		UPDATE alert_instances a SET
			closed_at = now(),
			meta = COALESCE(a.meta, '{}'::jsonb) || CASE
				WHEN COALESCE(c.reach_ok, false) THEN '{"resolved":"revalidate_probe_ok","source":"alerts_revalidate"}'::jsonb
				ELSE '{"resolved":"revalidate_device_not_monitored","source":"alerts_revalidate"}'::jsonb
			END
		FROM devices d
		LEFT JOIN device_probe_cache c ON c.device_id = d.id
		WHERE a.device_id = d.id
		AND a.closed_at IS NULL
		AND a.alert_type = 'ping_unreachable'
		AND (
			COALESCE(c.reach_ok, false)
			OR `+monitorworker.SQLDeviceEligibleForPingAlertsNotMet+`
		)
		RETURNING a.id, a.alert_type, a.message,
			(COALESCE(c.reach_ok, false)) AS probe_ok
	`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	var n int
	for rows.Next() {
		var id uuid.UUID
		var atype, msg string
		var probeOK bool
		if err := rows.Scan(&id, &atype, &msg, &probeOK); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		n++
		if !probeOK {
			continue
		}
		head := alertnotify.ResolutionHeadlineForAlertType(atype)
		alertnotify.SendResolutionTelegramAndPatchMeta(r.Context(), s.DB(), &s.Log, id, head, msg)
	}
	if err := rows.Err(); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":           true,
		"closed_count": n,
		"note":         "Fecha alertas ping_unreachable quando o probe está OK ou quando o equipamento não está em monitorização ativa (Ativo, ping ligado, rede Normal). Outros tipos de alerta não são alterados.",
	})
}

func (s *Server) alertsHistory(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 5000 {
		limit = 5000
	}
	deviceID := strings.TrimSpace(r.URL.Query().Get("device_id"))
	from := strings.TrimSpace(r.URL.Query().Get("from"))
	to := strings.TrimSpace(r.URL.Query().Get("to"))
	q := `
		SELECT a.id, a.device_id, a.severity, a.alert_type, a.message, a.ip,
			COALESCE(NULLIF(trim(a.device_name), ''), NULLIF(trim(d.description), '')) AS device_name,
			a.active_since, a.closed_at, a.meta::text,
			COALESCE(NULLIF(trim(p.description), ''), '') AS pop_name
		FROM alert_instances a
		LEFT JOIN devices d ON d.id = a.device_id
		LEFT JOIN pops p ON p.id = d.pop_id
		WHERE 1=1`
	args := []any{}
	n := 1
	if deviceID != "" {
		did, err := uuid.Parse(deviceID)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "BAD_QUERY", "device_id inválido", nil)
			return
		}
		q += ` AND a.device_id = $` + strconv.Itoa(n)
		args = append(args, did)
		n++
	}
	if from != "" && to != "" {
		q += ` AND a.active_since <= $` + strconv.Itoa(n+1) + ` AND (a.closed_at IS NULL OR a.closed_at >= $` + strconv.Itoa(n) + `)`
		args = append(args, from, to)
		n += 2
	}
	q += ` ORDER BY a.active_since DESC LIMIT ` + strconv.Itoa(limit)
	rows, err := s.DB().Query(r.Context(), q, args...)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	var list []map[string]any
	for rows.Next() {
		var id uuid.UUID
		var did *uuid.UUID
		var sev, typ, msg string
		var ip, dname, popName *string
		var since time.Time
		var closed *time.Time
		var meta []byte
		if err := rows.Scan(&id, &did, &sev, &typ, &msg, &ip, &dname, &since, &closed, &meta, &popName); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		item := map[string]any{
			"id": id, "device_id": did, "severity": sev, "type": typ, "message": msg,
			"ip": ip, "device_name": dname, "active_since": since, "closed_at": closed, "meta": json.RawMessage(meta),
			"pop_name": ptrStr(popName),
		}
		list = append(list, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": list})
}

func (s *Server) listAlertRules(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB().Query(r.Context(), `SELECT id, name, enabled, condition_json::text, channels_json::text, created_at FROM alert_rules ORDER BY name`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	var list []map[string]any
	for rows.Next() {
		var id uuid.UUID
		var name string
		var en bool
		var cj, ch []byte
		var cr time.Time
		if err := rows.Scan(&id, &name, &en, &cj, &ch, &cr); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		list = append(list, map[string]any{
			"id": id, "name": name, "enabled": en,
			"condition": json.RawMessage(cj), "channels": json.RawMessage(ch), "created_at": cr,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"rules": list})
}

func (s *Server) createAlertRule(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name          string          `json:"name"`
		Enabled       *bool           `json:"enabled"`
		ConditionJSON json.RawMessage `json:"condition"`
		ChannelsJSON  json.RawMessage `json:"channels"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Name) == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "name obrigatório", nil)
		return
	}
	en := true
	if body.Enabled != nil {
		en = *body.Enabled
	}
	cj := body.ConditionJSON
	if len(cj) == 0 {
		cj = json.RawMessage(`{}`)
	}
	ch := body.ChannelsJSON
	if len(ch) == 0 {
		ch = json.RawMessage(`{}`)
	}
	var id uuid.UUID
	err := s.DB().QueryRow(r.Context(), `
		INSERT INTO alert_rules (name, enabled, condition_json, channels_json) VALUES ($1,$2,$3::jsonb,$4::jsonb) RETURNING id
	`, body.Name, en, cj, ch).Scan(&id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "alert_rule", id.String(), "create", s.actorFromRequest(r), nil, body)
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (s *Server) getAlertRule(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	var name string
	var en bool
	var cj, ch []byte
	var cr, up time.Time
	err = s.DB().QueryRow(r.Context(), `
		SELECT name, enabled, condition_json::text, channels_json::text, created_at, updated_at FROM alert_rules WHERE id=$1
	`, id).Scan(&name, &en, &cj, &ch, &cr, &up)
	if err == pgx.ErrNoRows {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "", nil)
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id": id, "name": name, "enabled": en,
		"condition": json.RawMessage(cj), "channels": json.RawMessage(ch), "created_at": cr, "updated_at": up,
	})
}

func (s *Server) patchAlertRule(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	var body struct {
		Name          *string         `json:"name"`
		Enabled       *bool           `json:"enabled"`
		ConditionJSON json.RawMessage `json:"condition"`
		ChannelsJSON  json.RawMessage `json:"channels"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	_, err = s.DB().Exec(r.Context(), `
		UPDATE alert_rules SET
			name = COALESCE($2, name),
			enabled = COALESCE($3, enabled),
			updated_at = now()
		WHERE id=$1
	`, id, body.Name, body.Enabled)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if len(body.ConditionJSON) > 0 {
		_, err = s.DB().Exec(r.Context(), `UPDATE alert_rules SET condition_json=$2::jsonb, updated_at=now() WHERE id=$1`, id, body.ConditionJSON)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
	}
	if len(body.ChannelsJSON) > 0 {
		_, err = s.DB().Exec(r.Context(), `UPDATE alert_rules SET channels_json=$2::jsonb, updated_at=now() WHERE id=$1`, id, body.ChannelsJSON)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
	}
	var ruleName string
	_ = s.DB().QueryRow(r.Context(), `SELECT trim(name) FROM alert_rules WHERE id=$1`, id).Scan(&ruleName)
	if ruleName == alertthresholds.GlobalThresholdRuleName() && !alertthresholds.OltOnuQuantityAlertsEnabled(r.Context(), s.DB()) {
		_, cerr := s.DB().Exec(r.Context(), `
			UPDATE alert_instances SET
				closed_at = now(),
				meta = COALESCE(meta, '{}'::jsonb) || '{"resolved":"olt_onu_threshold_disabled","source":"alert_rules"}'::jsonb
			WHERE alert_type IN ('olt_onu_drop', 'olt_onu_rise') AND closed_at IS NULL
		`)
		if cerr != nil {
			s.Log.Warn().Err(cerr).Msg("fechar olt_onu_drop após desactivar limiar")
		}
	}
	if ruleName == alertthresholds.GlobalThresholdRuleName() && !alertthresholds.BngSubscriberDropAlertsEnabled(r.Context(), s.DB()) {
		_, cerr := s.DB().Exec(r.Context(), `
			UPDATE alert_instances SET
				closed_at = now(),
				meta = COALESCE(meta, '{}'::jsonb) || '{"resolved":"bng_drop_threshold_disabled","source":"alert_rules"}'::jsonb
			WHERE alert_type = 'bng_subscriber_drop' AND closed_at IS NULL
		`)
		if cerr != nil {
			s.Log.Warn().Err(cerr).Msg("fechar bng_subscriber_drop após desactivar limiar")
		}
	}
	s.appendAuditLog(r.Context(), "alert_rule", id.String(), "patch", s.actorFromRequest(r), nil, body)
	s.getAlertRule(w, r)
}

func (s *Server) deleteAlertRule(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	ct, err := s.DB().Exec(r.Context(), `DELETE FROM alert_rules WHERE id=$1`, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if ct.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "", nil)
		return
	}
	s.appendAuditLog(r.Context(), "alert_rule", id.String(), "delete", s.actorFromRequest(r), nil, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) testAlertRule(w http.ResponseWriter, r *http.Request) {
	writeErr(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "Teste sandbox de regra não está disponível em produção.", nil)
}

func (s *Server) realtimePing(w http.ResponseWriter, r *http.Request) {
	ids := r.URL.Query().Get("device_ids")
	if ids == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "device_ids (csv, máx 3)", nil)
		return
	}
	parts := strings.Split(ids, ",")
	if len(parts) > 3 {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "máximo 3 device_ids", nil)
		return
	}
	out := []map[string]any{}
	for _, p := range parts {
		id, err := uuid.Parse(strings.TrimSpace(p))
		if err != nil {
			continue
		}
		var ok bool
		var lat *int64
		var method *string
		var checked *time.Time
		_ = s.DB().QueryRow(r.Context(), `
			SELECT ok, latency_ms, method, checked_at FROM device_probe_cache WHERE device_id=$1
		`, id).Scan(&ok, &lat, &method, &checked)
		row := map[string]any{"device_id": id, "ok": ok, "checked_at": checked}
		if lat != nil {
			row["latency_ms"] = *lat
		}
		if method != nil {
			row["method"] = *method
		}
		out = append(out, row)
	}
	if s.rt != nil {
		s.rt.publish(r.Context(), "realtime.ping.samples", map[string]any{"samples": out})
	}
	writeJSON(w, http.StatusOK, map[string]any{"samples": out, "note": "Leitura do cache do worker; WebSocket pode ser adicionado depois."})
}

func (s *Server) listEvents(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	deviceID := strings.TrimSpace(r.URL.Query().Get("device_id"))
	q := `
		SELECT e.id, e.created_at, e.event_type, e.severity, e.device_id, e.payload::text, d.pop_id::text
		FROM events e
		LEFT JOIN devices d ON d.id = e.device_id
		WHERE 1=1`
	args := []any{}
	n := 1
	if deviceID != "" {
		did, err := uuid.Parse(deviceID)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "BAD_QUERY", "device_id inválido", nil)
			return
		}
		q += ` AND e.device_id = $` + strconv.Itoa(n)
		args = append(args, did)
		n++
	}
	q += ` ORDER BY e.created_at DESC LIMIT ` + strconv.Itoa(limit)
	rows, err := s.DB().Query(r.Context(), q, args...)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	var list []map[string]any
	type baseEv struct {
		row map[string]any
		ts  time.Time
		pop string
	}
	raw := []baseEv{}
	for rows.Next() {
		var id uuid.UUID
		var ts time.Time
		var et string
		var sev *string
		var did *uuid.UUID
		var pl []byte
		var popID *string
		if err := rows.Scan(&id, &ts, &et, &sev, &did, &pl, &popID); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		ev := map[string]any{"id": id, "created_at": ts, "event_type": et, "device_id": did, "payload": json.RawMessage(pl)}
		if sev != nil {
			ev["severity"] = *sev
		}
		pop := ""
		if popID != nil {
			pop = *popID
		}
		raw = append(raw, baseEv{row: ev, ts: ts, pop: pop})
	}
	for i, e := range raw {
		cause := ""
		if e.pop != "" {
			n := 0
			for _, other := range raw {
				if other.pop == e.pop {
					d := other.ts.Sub(e.ts)
					if d < 0 {
						d = -d
					}
					if d <= 5*time.Minute {
						n++
					}
				}
			}
			if n >= 4 {
				cause = "possível falha de uplink/energia no POP (múltiplos eventos próximos)"
			}
		}
		if cause == "" {
			if strings.Contains(strings.ToLower(toString(e.row["event_type"])), "ping") {
				cause = "instabilidade de conectividade/rota no equipamento"
			}
		}
		if cause != "" {
			e.row["probable_cause"] = cause
		}
		list = append(list, raw[i].row)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"events": list,
		"note":   "Tabela events é somente leitura nesta versão — sem writers automáticos; dados históricos/legado ou inserções manuais.",
	})
}

func (s *Server) metricsSeries(w http.ResponseWriter, r *http.Request) {
	writeErr(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "Use endpoints específicos de histórico (ping/telemetria/interfaces).", nil)
}

func (s *Server) prometheusMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("# HELP netquasar_up Backend liveness\nnetquasar_up 1\n"))
}
