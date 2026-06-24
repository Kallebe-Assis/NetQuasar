package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (s *Server) maybeKickNightlyCollection(ctx context.Context) {
	var enabled bool
	var hhmm, tz string
	var lastRun *time.Time
	if err := s.DB().QueryRow(ctx, `
		SELECT enabled, run_time_hhmm, timezone, last_run_at
		FROM nightly_collection_settings WHERE id=1
	`).Scan(&enabled, &hhmm, &tz, &lastRun); err != nil || !enabled {
		return
	}
	loc, err := time.LoadLocation(strings.TrimSpace(tz))
	if err != nil || loc == nil {
		loc = time.FixedZone("BRT", -3*3600)
	}
	now := time.Now().In(loc)
	cur := now.Format("15:04")
	if cur != strings.TrimSpace(hhmm) {
		return
	}
	if lastRun != nil {
		lr := lastRun.In(loc)
		if lr.Format("2006-01-02") == now.Format("2006-01-02") {
			return
		}
	}
	ct, err := s.DB().Exec(ctx, `
		UPDATE nightly_collection_settings
		SET last_status='running', updated_at=now()
		WHERE id=1 AND COALESCE(last_status,'') <> 'running'
	`)
	if err != nil || ct.RowsAffected() == 0 {
		return
	}
	go func() {
		sum, runErr := s.executeNightlyCollection(context.Background(), auditActorSistema)
		if runErr != nil {
			_, _ = s.DB().Exec(context.Background(), `
				UPDATE nightly_collection_settings
				SET last_run_at=now(), last_status='error', last_summary=$1::jsonb, updated_at=now()
				WHERE id=1
			`, []byte(`{"error":"`+strings.ReplaceAll(runErr.Error(), `"`, `'`)+`"}`))
			return
		}
		sb, _ := json.Marshal(sum)
		_, _ = s.DB().Exec(context.Background(), `
			UPDATE nightly_collection_settings
			SET last_run_at=now(), last_status='ok', last_summary=$1::jsonb, updated_at=now()
			WHERE id=1
		`, sb)
	}()
}

const auditActorSistema = "SISTEMA"

func (s *Server) actorFromRequest(r *http.Request) string {
	if r == nil {
		return auditActorSistema
	}
	bearer := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if bearer != "" && s.Cfg != nil {
		uid, email, _, err := parseUserJWT(s.Cfg, bearer)
		if err == nil && s.DB() != nil {
			var displayName string
			qerr := s.DB().QueryRow(r.Context(), `
				SELECT COALESCE(NULLIF(trim(display_name), ''), trim(email))
				FROM users WHERE id=$1`, uid).Scan(&displayName)
			if qerr == nil && strings.TrimSpace(displayName) != "" {
				return strings.TrimSpace(displayName)
			}
			if em := strings.TrimSpace(email); em != "" {
				return em
			}
		}
	}
	if strings.TrimSpace(r.Header.Get("X-API-Key")) != "" {
		return "Chave API"
	}
	return auditActorSistema
}

func normalizeAuditActor(actor string) string {
	a := strings.TrimSpace(actor)
	if a == "" {
		return auditActorSistema
	}
	lower := strings.ToLower(a)
	switch lower {
	case "anonymous", "scheduler", "worker", "automation", "system:monitor_worker":
		return auditActorSistema
	}
	if strings.HasPrefix(lower, "system:") {
		return auditActorSistema
	}
	return a
}

func (s *Server) appendAuditLog(ctx context.Context, entityType, entityID, action, actor string, beforeData, afterData any) {
	if s.DB() == nil {
		return
	}
	actor = normalizeAuditActor(actor)
	bb, _ := json.Marshal(beforeData)
	ab, _ := json.Marshal(afterData)
	if _, err := s.DB().Exec(ctx, `
		INSERT INTO ops_audit_log (entity_type, entity_id, action, actor, before_data, after_data)
		VALUES ($1, $2, $3, NULLIF($4,''), $5::jsonb, $6::jsonb)
	`, entityType, entityID, action, strings.TrimSpace(actor), bb, ab); err != nil {
		s.Log.Warn().Err(err).
			Str("entity_type", entityType).
			Str("entity_id", entityID).
			Str("action", action).
			Msg("falha ao gravar ops_audit_log")
	}
}

func (s *Server) auditDeviceAction(ctx context.Context, r *http.Request, deviceID uuid.UUID, action string, detail map[string]any) {
	if detail == nil {
		detail = map[string]any{}
	}
	s.appendAuditLog(ctx, "device", deviceID.String(), action, s.actorFromRequest(r), nil, detail)
}

func (s *Server) auditNetworkTool(ctx context.Context, r *http.Request, tool string, detail map[string]any) {
	if detail == nil {
		detail = map[string]any{}
	}
	detail["tool"] = tool
	s.appendAuditLog(ctx, "network_tool", tool, "executed", s.actorFromRequest(r), nil, detail)
}

func (s *Server) listOpsAudit(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 2000 {
		limit = 150
	}
	todayOnly := queryTruthy(r.URL.Query().Get("today"))
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	entityType := strings.TrimSpace(r.URL.Query().Get("entity_type"))
	actionFilter := strings.TrimSpace(r.URL.Query().Get("action"))
	actorFilter := strings.TrimSpace(r.URL.Query().Get("actor"))

	args := []any{}
	n := 1
	sqlQ := `
		SELECT a.id, a.entity_type, a.entity_id, a.action, a.actor,
			a.before_data::text, a.after_data::text, a.created_at,
			COALESCE(
				NULLIF(trim(d.description), ''),
				NULLIF(trim(p.description), ''),
				NULLIF(trim(cl.name), ''),
				NULLIF(trim(cc.client_name), ''),
				NULLIF(trim(i.name), ''),
				NULLIF(trim(a.after_data->>'description'), ''),
				NULLIF(trim(a.before_data->>'description'), ''),
				NULLIF(trim(a.after_data->>'name'), ''),
				CASE a.entity_type
					WHEN 'monitoring_runtime' THEN 'Sistema — Monitoramento'
					WHEN 'monitoring_intervals' THEN 'Intervalos de monitoramento'
					WHEN 'monitoring_settings' THEN 'Definições de monitoramento'
					WHEN 'monitoring_cycle' THEN
						CASE a.entity_id
							WHEN 'latency' THEN 'Ciclo — latência (ping)'
							WHEN 'telemetry' THEN 'Ciclo — telemetria SNMP'
							WHEN 'interfaces' THEN 'Ciclo — interfaces SNMP'
							WHEN 'olt-if-derived' THEN 'Ciclo — ONU/PON OLT'
							ELSE 'Ciclo de monitoramento'
						END
					WHEN 'nightly_collection' THEN 'Automação — coleta noturna'
					WHEN 'network_tool' THEN 'Ferramenta de rede'
					WHEN 'settings_mikrotik_collection' THEN 'Coleta Mikrotik'
					WHEN 'settings_bng_collection' THEN 'Coleta BNG'
					ELSE NULL
				END,
				a.entity_id
			) AS entity_label
		FROM ops_audit_log a
		LEFT JOIN devices d ON a.entity_type IN ('device', 'device_config_backup')
			AND d.id::text = a.entity_id
		LEFT JOIN pops p ON a.entity_type = 'pop' AND p.id::text = a.entity_id
		LEFT JOIN commercial_localities cl ON a.entity_type = 'commercial_locality' AND cl.id::text = a.entity_id
		LEFT JOIN client_connections cc ON a.entity_type = 'client_connection' AND cc.id::text = a.entity_id
		LEFT JOIN integrations i ON a.entity_type = 'integration' AND i.id::text = a.entity_id
		WHERE 1=1
	`
	if todayOnly {
		sqlQ += ` AND a.created_at >= date_trunc('day', now())`
	}
	if entityType != "" {
		sqlQ += ` AND a.entity_type = $` + strconv.Itoa(n)
		args = append(args, entityType)
		n++
	}
	if actionFilter != "" {
		sqlQ += ` AND a.action = $` + strconv.Itoa(n)
		args = append(args, actionFilter)
		n++
	}
	if actorFilter != "" {
		pat := "%" + actorFilter + "%"
		sqlQ += ` AND COALESCE(a.actor, '') ILIKE $` + strconv.Itoa(n)
		args = append(args, pat)
		n++
	}
	if q != "" {
		pat := "%" + q + "%"
		sqlQ += ` AND (
			COALESCE(a.actor, '') ILIKE $` + strconv.Itoa(n) + `
			OR a.action ILIKE $` + strconv.Itoa(n) + `
			OR a.entity_type ILIKE $` + strconv.Itoa(n) + `
			OR a.entity_id ILIKE $` + strconv.Itoa(n) + `
			OR COALESCE(d.description, '') ILIKE $` + strconv.Itoa(n) + `
			OR COALESCE(p.description, '') ILIKE $` + strconv.Itoa(n) + `
			OR COALESCE(cl.name, '') ILIKE $` + strconv.Itoa(n) + `
			OR COALESCE(cc.client_name, '') ILIKE $` + strconv.Itoa(n) + `
			OR COALESCE(i.name, '') ILIKE $` + strconv.Itoa(n) + `
			OR a.after_data::text ILIKE $` + strconv.Itoa(n) + `
			OR a.before_data::text ILIKE $` + strconv.Itoa(n) + `
		)`
		args = append(args, pat)
		n++
	}
	sqlQ += ` ORDER BY a.id DESC LIMIT $` + strconv.Itoa(n)
	args = append(args, limit)

	rows, err := s.DB().Query(r.Context(), sqlQ, args...)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var et, eid, action, entityLabel string
		var actor *string
		var before, after []byte
		var created time.Time
		if err := rows.Scan(&id, &et, &eid, &action, &actor, &before, &after, &created, &entityLabel); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		out = append(out, map[string]any{
			"id":           id,
			"entity_type":  et,
			"entity_id":    eid,
			"entity_label": entityLabel,
			"action":       action,
			"actor":        actor,
			"before_data":  json.RawMessage(before),
			"after_data":   json.RawMessage(after),
			"created_at":   created,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (s *Server) listMaintenanceWindows(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB().Query(r.Context(), `
		SELECT id, title, scope_type, pop_id, device_id, starts_at, ends_at, checklist::text, notes, status, created_at, updated_at
		FROM maintenance_windows
		ORDER BY starts_at DESC
		LIMIT 500
	`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id uuid.UUID
		var title, scopeType, status string
		var popID, devID *uuid.UUID
		var startsAt, endsAt, createdAt, updatedAt time.Time
		var checklist []byte
		var notes *string
		if err := rows.Scan(&id, &title, &scopeType, &popID, &devID, &startsAt, &endsAt, &checklist, &notes, &status, &createdAt, &updatedAt); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		out = append(out, map[string]any{
			"id":         id,
			"title":      title,
			"scope_type": scopeType,
			"pop_id":     popID,
			"device_id":  devID,
			"starts_at":  startsAt,
			"ends_at":    endsAt,
			"checklist":  json.RawMessage(checklist),
			"notes":      notes,
			"status":     status,
			"created_at": createdAt,
			"updated_at": updatedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (s *Server) createMaintenanceWindow(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title     string          `json:"title"`
		ScopeType string          `json:"scope_type"`
		PopID     *uuid.UUID      `json:"pop_id"`
		DeviceID  *uuid.UUID      `json:"device_id"`
		StartsAt  string          `json:"starts_at"`
		EndsAt    string          `json:"ends_at"`
		Checklist json.RawMessage `json:"checklist"`
		Notes     *string         `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	st := strings.TrimSpace(strings.ToLower(body.ScopeType))
	if body.Title == "" || (st != "global" && st != "pop" && st != "device") {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "title e scope_type (global|pop|device) são obrigatórios", nil)
		return
	}
	startsAt, err := time.Parse(time.RFC3339, strings.TrimSpace(body.StartsAt))
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "starts_at inválido (RFC3339)", nil)
		return
	}
	endsAt, err := time.Parse(time.RFC3339, strings.TrimSpace(body.EndsAt))
	if err != nil || !endsAt.After(startsAt) {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "ends_at inválido/deve ser maior que starts_at", nil)
		return
	}
	if len(body.Checklist) == 0 {
		body.Checklist = json.RawMessage(`[]`)
	}
	var id uuid.UUID
	err = s.DB().QueryRow(r.Context(), `
		INSERT INTO maintenance_windows (title, scope_type, pop_id, device_id, starts_at, ends_at, checklist, notes)
		VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb,$8)
		RETURNING id
	`, body.Title, st, body.PopID, body.DeviceID, startsAt.UTC(), endsAt.UTC(), body.Checklist, body.Notes).Scan(&id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "maintenance_window", id.String(), "create", s.actorFromRequest(r), nil, body)
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (s *Server) patchMaintenanceWindow(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	// patch simples
	_, err = s.DB().Exec(r.Context(), `
		UPDATE maintenance_windows
		SET title = COALESCE($2, title),
			notes = COALESCE($3, notes),
			status = COALESCE($4, status),
			updated_at = now()
		WHERE id = $1
	`, id, asStringPtr(body["title"]), asStringPtr(body["notes"]), asStringPtr(body["status"]))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if v, ok := body["checklist"]; ok {
		b, _ := json.Marshal(v)
		_, _ = s.DB().Exec(r.Context(), `UPDATE maintenance_windows SET checklist=$2::jsonb, updated_at=now() WHERE id=$1`, id, b)
	}
	s.appendAuditLog(r.Context(), "maintenance_window", id.String(), "patch", s.actorFromRequest(r), nil, body)
	s.listMaintenanceWindows(w, r)
}

func asStringPtr(v any) *string {
	if v == nil {
		return nil
	}
	s := strings.TrimSpace(strings.TrimSpace(strings.Trim(strings.TrimSpace(strings.ReplaceAll(strings.TrimSpace(strings.TrimSpace(strings.TrimSpace(toString(v)))), "\x00", "")), `"`)))
	if s == "" {
		return nil
	}
	return &s
}

func toString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	default:
		b, _ := json.Marshal(x)
		return string(b)
	}
}

func (s *Server) listPopContacts(w http.ResponseWriter, r *http.Request) {
	popID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	rows, err := s.DB().Query(r.Context(), `
		SELECT id, name, contact, shift_label, is_primary, notes, created_at, updated_at
		FROM pop_contacts WHERE pop_id = $1
		ORDER BY is_primary DESC, name
	`, popID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id uuid.UUID
		var name, contact string
		var shift, notes *string
		var isPrimary bool
		var created, updated time.Time
		if err := rows.Scan(&id, &name, &contact, &shift, &isPrimary, &notes, &created, &updated); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		out = append(out, map[string]any{
			"id":         id,
			"pop_id":     popID,
			"name":       name,
			"contact":    contact,
			"shift_label": shift,
			"is_primary": isPrimary,
			"notes":      notes,
			"created_at": created,
			"updated_at": updated,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (s *Server) createPopContact(w http.ResponseWriter, r *http.Request) {
	popID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	var body struct {
		Name      string  `json:"name"`
		Contact   string  `json:"contact"`
		ShiftLabel *string `json:"shift_label"`
		IsPrimary bool    `json:"is_primary"`
		Notes     *string `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Name) == "" || strings.TrimSpace(body.Contact) == "" {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "name e contact obrigatórios", nil)
		return
	}
	var id uuid.UUID
	err = s.DB().QueryRow(r.Context(), `
		INSERT INTO pop_contacts (pop_id, name, contact, shift_label, is_primary, notes)
		VALUES ($1,$2,$3,$4,$5,$6) RETURNING id
	`, popID, body.Name, body.Contact, body.ShiftLabel, body.IsPrimary, body.Notes).Scan(&id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "pop_contact", id.String(), "create", s.actorFromRequest(r), nil, body)
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (s *Server) deletePopContact(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "contactId"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	_, err = s.DB().Exec(r.Context(), `DELETE FROM pop_contacts WHERE id=$1`, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "pop_contact", id.String(), "delete", s.actorFromRequest(r), nil, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) commercialMonthComparison(w http.ResponseWriter, r *http.Request) {
	month := strings.TrimSpace(r.URL.Query().Get("month"))
	if month == "" {
		now := time.Now()
		month = now.Format("2006-01")
	}
	t, err := time.Parse("2006-01", month)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "month inválido (YYYY-MM)", nil)
		return
	}
	prev := t.AddDate(0, -1, 0).Format("2006-01")
	rows, err := s.DB().Query(r.Context(), `
		SELECT l.id::text, l.name,
			COALESCE(SUM(CASE WHEN r.year_month = $1 THEN r.client_count ELSE 0 END),0)::bigint AS curr,
			COALESCE(SUM(CASE WHEN r.year_month = $2 THEN r.client_count ELSE 0 END),0)::bigint AS prev
		FROM commercial_localities l
		LEFT JOIN commercial_monthly_records r ON r.locality_id = l.id AND r.year_month IN ($1, $2)
		GROUP BY l.id, l.name
		ORDER BY l.name
	`, month, prev)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, name string
		var curN, prevN int64
		if err := rows.Scan(&id, &name, &curN, &prevN); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		delta := curN - prevN
		pct := 0.0
		if prevN > 0 {
			pct = float64(delta) / float64(prevN) * 100
		}
		out = append(out, map[string]any{
			"locality_id": id, "locality_name": name, "current": curN, "previous": prevN,
			"delta": delta, "delta_percent": pct,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"month": month, "previous_month": prev, "rows": out})
}

func (s *Server) dashboardDataGaps(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB().Query(r.Context(), `
		SELECT id::text, description, category, host(ip)::text, locality_id::text, snmp_community, latitude, longitude, telemetry_enabled
		FROM devices
		ORDER BY description
	`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	summary := map[string]int{
		"without_locality": 0, "without_ip": 0, "without_snmp_community": 0, "without_coordinates": 0, "without_telemetry": 0,
	}
	for rows.Next() {
		var id, desc, cat string
		var ip, localityID, comm *string
		var lat, lon *float64
		var tel bool
		if err := rows.Scan(&id, &desc, &cat, &ip, &localityID, &comm, &lat, &lon, &tel); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		flags := []string{}
		if localityID == nil {
			flags = append(flags, "without_locality")
			summary["without_locality"]++
		}
		if ip == nil || strings.TrimSpace(*ip) == "" {
			flags = append(flags, "without_ip")
			summary["without_ip"]++
		}
		if comm == nil || strings.TrimSpace(*comm) == "" {
			flags = append(flags, "without_snmp_community")
			summary["without_snmp_community"]++
		}
		if lat == nil || lon == nil {
			flags = append(flags, "without_coordinates")
			summary["without_coordinates"]++
		}
		if !tel {
			flags = append(flags, "without_telemetry")
			summary["without_telemetry"]++
		}
		if len(flags) == 0 {
			continue
		}
		out = append(out, map[string]any{
			"id": id, "description": desc, "category": cat, "ip": ip, "locality_id": localityID, "gaps": flags,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"summary": summary, "devices": out})
}

func (s *Server) dashboardOltCapacity(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB().Query(r.Context(), `
		SELECT d.id::text, d.description, o.updated_at, o.pons::text
		FROM devices d
		JOIN olt_snapshots o ON o.device_id = d.id
		WHERE lower(trim(d.category))='olt'
		ORDER BY d.description
	`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	type ponRow struct {
		OLTID       string  `json:"olt_id"`
		OLT         string  `json:"olt"`
		PonID       string  `json:"pon_id"`
		OnuTotal    int     `json:"onu_total"`
		OnuOnline   int     `json:"onu_online"`
		OnuOffline  int     `json:"onu_offline"`
		UsagePct    float64 `json:"usage_percent"`
		Saturated   bool    `json:"near_saturation"`
		SnapshotAt  string  `json:"snapshot_at"`
	}
	pRows := []ponRow{}
	oltRows := []map[string]any{}
	for rows.Next() {
		var id, desc string
		var updated time.Time
		var ponsRaw string
		if err := rows.Scan(&id, &desc, &updated, &ponsRaw); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		var pons []map[string]any
		_ = json.Unmarshal([]byte(ponsRaw), &pons)
		oltTotal := 0
		near := 0
		for _, p := range pons {
			total := toInt(p["onu_total"])
			on := toInt(p["onu_online"])
			off := toInt(p["onu_offline"])
			usage := float64(total) / 128.0 * 100.0
			nearSat := usage >= 80
			if nearSat {
				near++
			}
			oltTotal += total
			pRows = append(pRows, ponRow{
				OLTID: id, OLT: desc, PonID: strings.TrimSpace(toString(p["id"])),
				OnuTotal: total, OnuOnline: on, OnuOffline: off, UsagePct: usage, Saturated: nearSat,
				SnapshotAt: updated.UTC().Format(time.RFC3339),
			})
		}
		oltRows = append(oltRows, map[string]any{
			"olt_id": id, "olt": desc, "onu_total": oltTotal, "pon_count": len(pons), "near_saturation_pons": near, "snapshot_at": updated.UTC().Format(time.RFC3339),
		})
	}
	sort.Slice(pRows, func(i, j int) bool { return pRows[i].UsagePct > pRows[j].UsagePct })
	trendRows := []map[string]any{}
	dayRows, err := s.DB().Query(r.Context(), `
		SELECT (updated_at AT TIME ZONE 'UTC')::date::text AS day,
			COALESCE(SUM((
				SELECT SUM(COALESCE((NULLIF(trim(e->>'onu_total'),''))::bigint,0))
				FROM jsonb_array_elements(CASE WHEN jsonb_typeof(pons)='array' THEN pons ELSE '[]'::jsonb END) e
			)),0)::bigint AS onu_total
		FROM olt_snapshots
		WHERE updated_at >= now() - interval '7 day'
		GROUP BY 1 ORDER BY 1
	`)
	if err == nil {
		defer dayRows.Close()
		for dayRows.Next() {
			var day string
			var n int64
			if dayRows.Scan(&day, &n) == nil {
				trendRows = append(trendRows, map[string]any{"day": day, "onu_total": n})
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"olt_rows": oltRows, "pon_rows": pRows, "trend_7d": trendRows})
}

func toInt(v any) int {
	if v == nil {
		return 0
	}
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(x))
		return n
	default:
		return 0
	}
}

func (s *Server) getNightlyCollectionSettings(w http.ResponseWriter, r *http.Request) {
	var enabled bool
	var hhmm, tz string
	var lastAt *time.Time
	var lastStatus *string
	var summary []byte
	err := s.DB().QueryRow(r.Context(), `
		SELECT enabled, run_time_hhmm, timezone, last_run_at, last_status, COALESCE(last_summary, '{}'::jsonb)::text
		FROM nightly_collection_settings WHERE id = 1
	`).Scan(&enabled, &hhmm, &tz, &lastAt, &lastStatus, &summary)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled": enabled, "run_time_hhmm": hhmm, "timezone": tz,
		"last_run_at": lastAt, "last_status": lastStatus, "last_summary": json.RawMessage(summary),
	})
}

func (s *Server) patchNightlyCollectionSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Enabled      *bool   `json:"enabled"`
		RunTimeHHMM  *string `json:"run_time_hhmm"`
		Timezone     *string `json:"timezone"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	_, err := s.DB().Exec(r.Context(), `
		UPDATE nightly_collection_settings
		SET enabled = COALESCE($1, enabled),
			run_time_hhmm = COALESCE($2, run_time_hhmm),
			timezone = COALESCE($3, timezone),
			updated_at = now()
		WHERE id = 1
	`, body.Enabled, body.RunTimeHHMM, body.Timezone)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	s.getNightlyCollectionSettings(w, r)
}

func (s *Server) runNightlyCollectionNow(w http.ResponseWriter, r *http.Request) {
	sum, err := s.executeNightlyCollection(r.Context(), s.actorFromRequest(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "RUN", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, sum)
}

func (s *Server) executeNightlyCollection(ctx context.Context, actor string) (map[string]any, error) {
	s.setMonitoringActivity(ctx, "Coleta completa noturna")
	defer s.setMonitoringActivity(ctx, "")
	type dev struct {
		ID       uuid.UUID
		IP       *string
		Category string
	}
	rows, err := s.DB().Query(ctx, `SELECT id, host(ip)::text, category FROM devices ORDER BY description`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var all []dev
	for rows.Next() {
		var d dev
		if err := rows.Scan(&d.ID, &d.IP, &d.Category); err != nil {
			return nil, err
		}
		all = append(all, d)
	}
	oltOK, ifOK, telOK, telSkip := 0, 0, 0, 0
	for _, d := range all {
		if strings.EqualFold(strings.TrimSpace(d.Category), "olt") {
			if callInternalDevicePost(ctx, s.refreshOLTDevice, "id", d.ID) == nil {
				oltOK++
			}
			if callInternalDevicePost(ctx, s.refreshDeviceInterfaces, "id", d.ID) == nil {
				ifOK++
			}
		}
		if d.IP == nil || strings.TrimSpace(*d.IP) == "" {
			telSkip++
			continue
		}
		if callInternalDevicePost(ctx, s.telemetryCollect, "id", d.ID) == nil {
			telOK++
		}
	}
	sum := map[string]any{
		"status": "done",
		"olts_snapshot_ok": oltOK,
		"interfaces_refresh_ok": ifOK,
		"telemetry_collect_ok": telOK,
		"telemetry_skipped_no_ip": telSkip,
		"total_devices": len(all),
		"finished_at": time.Now().UTC().Format(time.RFC3339),
	}
	sb, _ := json.Marshal(sum)
	_, _ = s.DB().Exec(ctx, `
		UPDATE nightly_collection_settings
		SET last_run_at = now(), last_status = 'ok', last_summary = $1::jsonb, updated_at = now()
		WHERE id = 1
	`, sb)
	s.appendAuditLog(ctx, "nightly_collection", "1", "run", actor, nil, sum)
	return sum, nil
}

func callInternalDevicePost(ctx context.Context, h func(http.ResponseWriter, *http.Request), param string, id uuid.UUID) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "/internal", bytes.NewReader([]byte(`{}`)))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(param, id.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code >= 200 && rec.Code < 300 {
		return nil
	}
	return context.Canceled
}

