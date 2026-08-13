package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type networkVLANRow struct {
	ID           string  `json:"id,omitempty"`
	VLANID       string  `json:"vlan_id"`
	Name         string  `json:"name"`
	Description  string  `json:"description"`
	Kind         string  `json:"kind"`
	Status       string  `json:"status"`
	Capacity     *int    `json:"capacity,omitempty"`
	Connections  int     `json:"connections"`
	Equipment    int     `json:"equipment"`
	Utilization  *int    `json:"utilization,omitempty"`
	Catalogued   bool    `json:"catalogued"`
	UpdatedAt    *string `json:"updated_at,omitempty"`
}

func normalizeVLANKind(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "gerencia", "gerência", "mgmt", "management":
		return "gerencia"
	case "transporte", "transport", "core":
		return "transporte"
	default:
		return "pppoe"
	}
}

func normalizeVLANStatus(s string) string {
	if strings.ToLower(strings.TrimSpace(s)) == "inactive" {
		return "inactive"
	}
	return "active"
}

func normalizeCatalogVLANID(raw string) (string, string) {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(strings.ToLower(s), "vlan")
	s = strings.TrimSpace(s)
	if s == "" || s == "0" {
		return "", "informe o número da VLAN"
	}
	if n, err := strconv.Atoi(s); err == nil {
		if n < 1 || n > 4094 {
			return "", "VLAN deve estar entre 1 e 4094"
		}
		return strconv.Itoa(n), ""
	}
	if len(s) > 32 {
		return "", "identificador demasiado longo"
	}
	return s, ""
}

func (s *Server) listNetworkVLANs(w http.ResponseWriter, r *http.Request) {
	if s.DB() == nil {
		writeJSON(w, http.StatusOK, map[string]any{"vlans": []any{}, "summary": map[string]any{}})
		return
	}
	deviceFilter := ""
	var args []any
	if idStr := strings.TrimSpace(r.URL.Query().Get("device_id")); idStr != "" {
		id, err := uuid.Parse(idStr)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "BAD_ID", "device_id inválido", nil)
			return
		}
		deviceFilter = " AND device_id = $1"
		args = append(args, id)
	}

	statsQ := `
		SELECT trim(vlan) AS vlan,
			COUNT(*) FILTER (WHERE is_online)::int AS connections,
			COUNT(DISTINCT device_id)::int AS equipment
		FROM bng_known_logins
		WHERE vlan IS NOT NULL AND trim(vlan) <> '' AND trim(vlan) <> '0'` + deviceFilter + `
		GROUP BY 1`

	type stat struct{ conn, equip int }
	stats := map[string]stat{}
	if rows, err := s.DB().Query(r.Context(), statsQ, args...); err == nil {
		defer rows.Close()
		for rows.Next() {
			var vid string
			var st stat
			if rows.Scan(&vid, &st.conn, &st.equip) == nil && vid != "" {
				stats[vid] = st
			}
		}
	}

	discovered := map[string]struct{}{}
	for v := range stats {
		discovered[v] = struct{}{}
	}
	if locRows, err := s.DB().Query(r.Context(), `
		SELECT DISTINCT trim(vlan) FROM locality_vlans
		WHERE vlan IS NOT NULL AND trim(vlan) <> ''`); err == nil {
		defer locRows.Close()
		for locRows.Next() {
			var v string
			if locRows.Scan(&v) == nil && v != "" {
				discovered[v] = struct{}{}
			}
		}
	}

	catalog := map[string]networkVLANRow{}
	if rows, err := s.DB().Query(r.Context(), `
		SELECT id::text, vlan_id, name, description, kind, status, capacity, updated_at::text
		FROM network_vlans ORDER BY vlan_id`); err == nil {
		defer rows.Close()
		for rows.Next() {
			var row networkVLANRow
			var cap *int
			var updated *string
			if err := rows.Scan(&row.ID, &row.VLANID, &row.Name, &row.Description, &row.Kind, &row.Status, &cap, &updated); err != nil {
				continue
			}
			row.Capacity = cap
			row.UpdatedAt = updated
			row.Catalogued = true
			catalog[row.VLANID] = row
		}
	}

	keys := map[string]struct{}{}
	for k := range catalog {
		keys[k] = struct{}{}
	}
	for k := range discovered {
		keys[k] = struct{}{}
	}

	list := make([]networkVLANRow, 0, len(keys))
	var totalConn, activeN, critN, equipN int
	equipSeen := map[string]struct{}{}
	for vid := range keys {
		row, ok := catalog[vid]
		if !ok {
			row = networkVLANRow{VLANID: vid, Kind: "pppoe", Status: "active", Catalogued: false}
		}
		if st, ok := stats[vid]; ok {
			row.Connections = st.conn
			row.Equipment = st.equip
		}
		if row.Capacity != nil && *row.Capacity > 0 {
			pct := int(float64(row.Connections) * 100 / float64(*row.Capacity))
			if pct > 100 {
				pct = 100
			}
			row.Utilization = &pct
			if pct >= 80 && row.Status == "active" {
				critN++
			}
		}
		if row.Status == "active" {
			activeN++
		}
		totalConn += row.Connections
		if row.Equipment > 0 {
			equipSeen[vid] = struct{}{}
		}
		list = append(list, row)
	}
	equipN = len(equipSeen)
	if n, err := s.countBNGDevicesUsingVLANs(r, args, deviceFilter); err == nil && n > equipN {
		equipN = n
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"vlans": list,
		"summary": map[string]any{
			"total":       len(list),
			"active":      activeN,
			"connections": totalConn,
			"equipment":   equipN,
			"critical":    critN,
		},
	})
}

func (s *Server) countBNGDevicesUsingVLANs(r *http.Request, args []any, deviceFilter string) (int, error) {
	var n int
	err := s.DB().QueryRow(r.Context(), `
		SELECT COUNT(DISTINCT device_id)::int FROM bng_known_logins
		WHERE vlan IS NOT NULL AND trim(vlan) <> '' AND trim(vlan) <> '0'`+deviceFilter, args...).Scan(&n)
	return n, err
}

func (s *Server) createNetworkVLAN(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, "bng.collect", "devices.manage", "*") {
		return
	}
	var body struct {
		VLANID      string `json:"vlan_id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Kind        string `json:"kind"`
		Status      string `json:"status"`
		Capacity    *int   `json:"capacity"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	vid, msg := normalizeCatalogVLANID(body.VLANID)
	if msg != "" {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", msg, nil)
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = "VLAN " + vid
	}
	if len(name) > 80 {
		name = name[:80]
	}
	desc := strings.TrimSpace(body.Description)
	if len(desc) > 240 {
		desc = desc[:240]
	}
	kind := normalizeVLANKind(body.Kind)
	status := normalizeVLANStatus(body.Status)
	var cap any
	if body.Capacity != nil && *body.Capacity > 0 {
		cap = *body.Capacity
	}
	var id uuid.UUID
	err := s.DB().QueryRow(r.Context(), `
		INSERT INTO network_vlans (vlan_id, name, description, kind, status, capacity)
		VALUES ($1,$2,$3,$4,$5,$6)
		RETURNING id`, vid, name, desc, kind, status, cap).Scan(&id)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") || strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			writeErr(w, http.StatusConflict, "DUPLICATE", "já existe uma VLAN com este número", nil)
			return
		}
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "network_vlan", id.String(), "create", s.actorFromRequest(r), nil, map[string]any{
		"vlan_id": vid, "kind": kind,
	})
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "vlan_id": vid})
}

func (s *Server) patchNetworkVLAN(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, "bng.collect", "devices.manage", "*") {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	var body struct {
		VLANID      *string `json:"vlan_id"`
		Name        *string `json:"name"`
		Description *string `json:"description"`
		Kind        *string `json:"kind"`
		Status      *string `json:"status"`
		Capacity    *int    `json:"capacity"`
		ClearCap    bool    `json:"clear_capacity"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	var curVID, curName, curDesc, curKind, curStatus string
	var curCap *int
	err = s.DB().QueryRow(r.Context(), `
		SELECT vlan_id, name, description, kind, status, capacity FROM network_vlans WHERE id=$1`, id).
		Scan(&curVID, &curName, &curDesc, &curKind, &curStatus, &curCap)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeErr(w, http.StatusNotFound, "NOT_FOUND", "VLAN não encontrada", nil)
			return
		}
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	vid := curVID
	if body.VLANID != nil {
		n, msg := normalizeCatalogVLANID(*body.VLANID)
		if msg != "" {
			writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", msg, nil)
			return
		}
		vid = n
	}
	name := curName
	if body.Name != nil {
		name = strings.TrimSpace(*body.Name)
		if name == "" {
			name = "VLAN " + vid
		}
	}
	desc := curDesc
	if body.Description != nil {
		desc = strings.TrimSpace(*body.Description)
	}
	kind := curKind
	if body.Kind != nil {
		kind = normalizeVLANKind(*body.Kind)
	}
	status := curStatus
	if body.Status != nil {
		status = normalizeVLANStatus(*body.Status)
	}
	var cap any
	if body.ClearCap {
		cap = nil
	} else if body.Capacity != nil && *body.Capacity > 0 {
		cap = *body.Capacity
	} else {
		cap = curCap
	}
	_, err = s.DB().Exec(r.Context(), `
		UPDATE network_vlans
		SET vlan_id=$2, name=$3, description=$4, kind=$5, status=$6, capacity=$7, updated_at=now()
		WHERE id=$1`, id, vid, name, desc, kind, status, cap)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			writeErr(w, http.StatusConflict, "DUPLICATE", "já existe uma VLAN com este número", nil)
			return
		}
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "network_vlan", id.String(), "update", s.actorFromRequest(r), nil, map[string]any{
		"vlan_id": vid, "kind": kind, "status": status,
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})
}

func (s *Server) upsertNetworkVLAN(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, "bng.collect", "devices.manage", "*") {
		return
	}
	var body struct {
		VLANID      string `json:"vlan_id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Kind        string `json:"kind"`
		Status      string `json:"status"`
		Capacity    *int   `json:"capacity"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	vid, msg := normalizeCatalogVLANID(body.VLANID)
	if msg != "" {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", msg, nil)
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = "VLAN " + vid
	}
	desc := strings.TrimSpace(body.Description)
	kind := normalizeVLANKind(body.Kind)
	status := normalizeVLANStatus(body.Status)
	var cap any
	if body.Capacity != nil && *body.Capacity > 0 {
		cap = *body.Capacity
	}
	var id uuid.UUID
	err := s.DB().QueryRow(r.Context(), `
		INSERT INTO network_vlans (vlan_id, name, description, kind, status, capacity)
		VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (vlan_id) DO UPDATE SET
			name=EXCLUDED.name, description=EXCLUDED.description, kind=EXCLUDED.kind,
			status=EXCLUDED.status, capacity=EXCLUDED.capacity, updated_at=now()
		RETURNING id`, vid, name, desc, kind, status, cap).Scan(&id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "network_vlan", id.String(), "upsert", s.actorFromRequest(r), nil, map[string]any{
		"vlan_id": vid, "kind": kind,
	})
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "vlan_id": vid})
}

func (s *Server) deleteNetworkVLAN(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, "bng.collect", "devices.manage", "*") {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	ct, err := s.DB().Exec(r.Context(), `DELETE FROM network_vlans WHERE id=$1`, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if ct.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "VLAN não encontrada", nil)
		return
	}
	s.appendAuditLog(r.Context(), "network_vlan", id.String(), "delete", s.actorFromRequest(r), nil, nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
