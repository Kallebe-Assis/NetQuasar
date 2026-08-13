package api

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/netquasar/netquasar/quasar_backend/internal/networkevents"
)

type networkEventRow struct {
	ID              uuid.UUID  `json:"id"`
	OccurredAt      time.Time  `json:"occurred_at"`
	CategoryCode    string     `json:"category_code"`
	CategoryLabel   string     `json:"category_label"`
	TypeCode        string     `json:"type_code"`
	TypeLabel       string     `json:"type_label"`
	Impact          string     `json:"impact"`
	Notes           *string    `json:"notes"`
	PopID           *uuid.UUID `json:"pop_id"`
	PopName         *string    `json:"pop_name,omitempty"`
	DeviceID        *uuid.UUID `json:"device_id"`
	DeviceName      *string    `json:"device_name,omitempty"`
	TechnicianID    *uuid.UUID `json:"technician_id"`
	TechnicianName  *string    `json:"technician_name,omitempty"`
	ProjectID       *uuid.UUID `json:"project_id"`
	ProjectName     *string    `json:"project_name,omitempty"`
	CtoID           *uuid.UUID `json:"cto_id"`
	CtoName         *string    `json:"cto_name,omitempty"`
	CableID         *uuid.UUID `json:"cable_id"`
	CableName       *string    `json:"cable_name,omitempty"`
	SpliceBoxID     *uuid.UUID `json:"splice_box_id"`
	SpliceBoxName   *string    `json:"splice_box_name,omitempty"`
	PoleID          *uuid.UUID `json:"pole_id"`
	PoleName        *string    `json:"pole_name,omitempty"`
	InterfaceName   *string    `json:"interface_name"`
	VLAN            *string    `json:"vlan"`
	CreatedBy       *uuid.UUID `json:"created_by"`
	CreatedByName   *string    `json:"created_by_name,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type networkEventBody struct {
	OccurredAt    string  `json:"occurred_at"`
	TypeCode      string  `json:"type_code"`
	Impact        string  `json:"impact"`
	Notes         *string `json:"notes"`
	PopID         *string `json:"pop_id"`
	DeviceID      *string `json:"device_id"`
	TechnicianID  *string `json:"technician_id"`
	ProjectID     *string `json:"project_id"`
	CtoID         *string `json:"cto_id"`
	CableID       *string `json:"cable_id"`
	SpliceBoxID   *string `json:"splice_box_id"`
	PoleID        *string `json:"pole_id"`
	InterfaceName *string `json:"interface_name"`
	VLAN          *string `json:"vlan"`
}

func (s *Server) networkEventsCatalog(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, "network_events.view", "network_events.manage", "*") {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"categories": networkevents.Categories,
		"types":      networkevents.Types,
		"impacts": []map[string]string{
			{"code": "none", "label": "Nenhum"},
			{"code": "low", "label": "Baixo"},
			{"code": "medium", "label": "Médio"},
			{"code": "high", "label": "Alto"},
			{"code": "critical", "label": "Crítico"},
		},
	})
}

func (s *Server) networkEventsLookups(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, "network_events.view", "network_events.manage", "*") {
		return
	}
	ctx := r.Context()
	type lite struct {
		ID          uuid.UUID  `json:"id"`
		Description string     `json:"description"`
		PopID       *uuid.UUID `json:"pop_id,omitempty"`
		ProjectID   *uuid.UUID `json:"project_id,omitempty"`
	}
	queryLite := func(q string) []lite {
		rows, err := s.DB().Query(ctx, q)
		if err != nil {
			return []lite{}
		}
		defer rows.Close()
		out := []lite{}
		for rows.Next() {
			var it lite
			_ = rows.Scan(&it.ID, &it.Description, &it.PopID, &it.ProjectID)
			out = append(out, it)
		}
		return out
	}
	pops := queryLite(`SELECT id, description, NULL::uuid, NULL::uuid FROM pops ORDER BY description`)
	devices := queryLite(`SELECT id, description, pop_id, NULL::uuid FROM devices ORDER BY description LIMIT 2000`)
	projects := queryLite(`SELECT id, description, NULL::uuid, NULL::uuid FROM network_projects WHERE COALESCE(status,'') <> 'inativo' ORDER BY description`)
	ctos := queryLite(`SELECT id, description, NULL::uuid, project_id FROM network_ctos ORDER BY description LIMIT 3000`)
	cables := queryLite(`SELECT id, COALESCE(NULLIF(trim(description),''), 'Cabo #'||display_number::text), NULL::uuid, project_id FROM network_cables ORDER BY display_number LIMIT 3000`)
	splices := queryLite(`SELECT id, description, NULL::uuid, project_id FROM network_splice_boxes ORDER BY description LIMIT 3000`)
	poles := queryLite(`SELECT id, COALESCE(NULLIF(trim(description),''), 'Poste #'||display_number::text), NULL::uuid, project_id FROM network_poles ORDER BY display_number LIMIT 3000`)

	type userLite struct {
		ID    uuid.UUID `json:"id"`
		Label string    `json:"label"`
	}
	users := []userLite{}
	urows, err := s.DB().Query(ctx, `
		SELECT id, COALESCE(NULLIF(trim(display_name), ''), email)
		FROM users
		WHERE COALESCE(is_active, true)
		ORDER BY 2
	`)
	if err == nil {
		defer urows.Close()
		for urows.Next() {
			var u userLite
			if urows.Scan(&u.ID, &u.Label) == nil {
				users = append(users, u)
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"pops": pops, "devices": devices, "projects": projects,
		"ctos": ctos, "cables": cables, "splice_boxes": splices, "poles": poles,
		"technicians": users,
	})
}

func (s *Server) listNetworkEvents(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, "network_events.view", "network_events.manage", "*") {
		return
	}
	list, total, err := s.queryNetworkEvents(r, false)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list, "total": total})
}

func (s *Server) exportNetworkEventsCSV(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, "network_events.view", "network_events.manage", "*") {
		return
	}
	list, _, err := s.queryNetworkEvents(r, true)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="eventos-rede.csv"`)
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{
		"data", "categoria", "tipo", "codigo_tipo", "impacto", "pop", "equipamento",
		"projeto", "cto", "cabo", "emenda", "poste", "interface", "vlan", "tecnico", "notas",
	})
	for _, it := range list {
		_ = cw.Write([]string{
			it.OccurredAt.In(time.Local).Format("2006-01-02 15:04"),
			it.CategoryLabel, it.TypeLabel, it.TypeCode, impactLabel(it.Impact),
			netevText(it.PopName), netevText(it.DeviceName), netevText(it.ProjectName),
			netevText(it.CtoName), netevText(it.CableName), netevText(it.SpliceBoxName),
			netevText(it.PoleName), netevText(it.InterfaceName), netevText(it.VLAN),
			netevText(it.TechnicianName), netevText(it.Notes),
		})
	}
	cw.Flush()
}

func (s *Server) networkEventsSummary(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, "network_events.view", "network_events.manage", "*") {
		return
	}
	from, to := r.URL.Query().Get("from"), r.URL.Query().Get("to")
	var total, monthN, incidentN int
	_ = s.DB().QueryRow(r.Context(), `SELECT COUNT(*) FROM network_events`).Scan(&total)
	_ = s.DB().QueryRow(r.Context(), `
		SELECT COUNT(*) FROM network_events
		WHERE occurred_at >= date_trunc('month', now())
	`).Scan(&monthN)
	_ = s.DB().QueryRow(r.Context(), `
		SELECT COUNT(*) FROM network_events WHERE category_code = 'incident'
	`).Scan(&incidentN)

	type pair struct {
		Code  string `json:"code"`
		Label string `json:"label"`
		Count int    `json:"count"`
	}
	byCat := []pair{}
	rows, err := s.DB().Query(r.Context(), `
		SELECT category_code, COUNT(*) FROM network_events
		WHERE ($1 = '' OR occurred_at >= $1::timestamptz)
		  AND ($2 = '' OR occurred_at < $2::timestamptz + interval '1 day')
		GROUP BY category_code ORDER BY COUNT(*) DESC
	`, from, to)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var p pair
			if rows.Scan(&p.Code, &p.Count) == nil {
				if c := networkevents.CategoryByCode(p.Code); c != nil {
					p.Label = c.Label
				} else {
					p.Label = p.Code
				}
				byCat = append(byCat, p)
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"total": total, "this_month": monthN, "incidents": incidentN, "by_category": byCat,
	})
}

func (s *Server) createNetworkEvent(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, "network_events.manage", "*") {
		return
	}
	var body networkEventBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "JSON inválido", nil)
		return
	}
	vals, err := validateNetworkEventBody(body)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "VALIDATION", err.Error(), nil)
		return
	}
	actor := s.requestAuthContext(r).UserID
	var actorPtr *uuid.UUID
	if actor != uuid.Nil {
		actorPtr = &actor
	}
	var id uuid.UUID
	err = s.DB().QueryRow(r.Context(), `
		INSERT INTO network_events (
			occurred_at, category_code, type_code, impact, notes,
			pop_id, device_id, technician_id, project_id, cto_id, cable_id, splice_box_id, pole_id,
			interface_name, vlan, created_by, updated_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$16)
		RETURNING id
	`, vals.occurred, vals.typ.CategoryCode, vals.typ.Code, vals.impact, netevTrimPtr(body.Notes),
		vals.pop, vals.device, vals.tech, vals.project, vals.cto, vals.cable, vals.splice, vals.pole,
		netevTrimPtr(body.InterfaceName), netevTrimPtr(body.VLAN), actorPtr,
	).Scan(&id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "network_event", id.String(), "create", s.actorFromRequest(r), nil, body)
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (s *Server) patchNetworkEvent(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, "network_events.manage", "*") {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "UUID inválido", nil)
		return
	}
	var body networkEventBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "JSON inválido", nil)
		return
	}
	vals, err := validateNetworkEventBody(body)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "VALIDATION", err.Error(), nil)
		return
	}
	actor := s.requestAuthContext(r).UserID
	var actorPtr *uuid.UUID
	if actor != uuid.Nil {
		actorPtr = &actor
	}
	tag, err := s.DB().Exec(r.Context(), `
		UPDATE network_events SET
			occurred_at=$2, category_code=$3, type_code=$4, impact=$5, notes=$6,
			pop_id=$7, device_id=$8, technician_id=$9, project_id=$10, cto_id=$11, cable_id=$12,
			splice_box_id=$13, pole_id=$14, interface_name=$15, vlan=$16,
			updated_by=$17, updated_at=now()
		WHERE id=$1
	`, id, vals.occurred, vals.typ.CategoryCode, vals.typ.Code, vals.impact, netevTrimPtr(body.Notes),
		vals.pop, vals.device, vals.tech, vals.project, vals.cto, vals.cable, vals.splice, vals.pole,
		netevTrimPtr(body.InterfaceName), netevTrimPtr(body.VLAN), actorPtr)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "evento não encontrado", nil)
		return
	}
	s.appendAuditLog(r.Context(), "network_event", id.String(), "update", s.actorFromRequest(r), nil, body)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) deleteNetworkEvent(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, "network_events.manage", "*") {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "UUID inválido", nil)
		return
	}
	tag, err := s.DB().Exec(r.Context(), `DELETE FROM network_events WHERE id=$1`, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "evento não encontrado", nil)
		return
	}
	s.appendAuditLog(r.Context(), "network_event", id.String(), "delete", s.actorFromRequest(r), nil, nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

type netEvVals struct {
	occurred time.Time
	typ      *networkevents.Type
	impact   string
	pop, device, tech, project, cto, cable, splice, pole *uuid.UUID
}

func validateNetworkEventBody(body networkEventBody) (netEvVals, error) {
	var out netEvVals
	code := strings.TrimSpace(body.TypeCode)
	out.typ = networkevents.TypeByCode(code)
	if out.typ == nil {
		return out, fmt.Errorf("tipo de evento inválido")
	}
	out.impact = strings.TrimSpace(body.Impact)
	if out.impact == "" {
		out.impact = "none"
	}
	if !networkevents.ValidImpact(out.impact) {
		return out, fmt.Errorf("impacto inválido")
	}
	if strings.TrimSpace(body.OccurredAt) == "" {
		out.occurred = time.Now()
	} else {
		t, err := parseFlexibleTime(body.OccurredAt)
		if err != nil {
			return out, fmt.Errorf("data/hora inválida")
		}
		out.occurred = t
	}
	out.pop = parseOptUUID(body.PopID)
	out.device = parseOptUUID(body.DeviceID)
	out.tech = parseOptUUID(body.TechnicianID)
	out.project = parseOptUUID(body.ProjectID)
	out.cto = parseOptUUID(body.CtoID)
	out.cable = parseOptUUID(body.CableID)
	out.splice = parseOptUUID(body.SpliceBoxID)
	out.pole = parseOptUUID(body.PoleID)
	return out, nil
}

func (s *Server) queryNetworkEvents(r *http.Request, all bool) ([]networkEventRow, int, error) {
	q := r.URL.Query()
	search := strings.TrimSpace(q.Get("q"))
	cat := strings.TrimSpace(q.Get("category"))
	typ := strings.TrimSpace(q.Get("type"))
	impact := strings.TrimSpace(q.Get("impact"))
	from, to := strings.TrimSpace(q.Get("from")), strings.TrimSpace(q.Get("to"))
	popID, deviceID, techID := strings.TrimSpace(q.Get("pop_id")), strings.TrimSpace(q.Get("device_id")), strings.TrimSpace(q.Get("technician_id"))
	projectID := strings.TrimSpace(q.Get("project_id"))
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	if all {
		limit = 10000
		offset = 0
	} else {
		if limit <= 0 || limit > 500 {
			limit = 200
		}
		if offset < 0 {
			offset = 0
		}
	}

	args := []any{search, cat, typ, impact, from, to, popID, deviceID, techID, projectID}
	where := `
		WHERE ($1 = '' OR e.notes ILIKE '%'||$1||'%' OR e.type_code ILIKE '%'||$1||'%'
			OR e.interface_name ILIKE '%'||$1||'%' OR e.vlan ILIKE '%'||$1||'%'
			OR COALESCE(p.description,'') ILIKE '%'||$1||'%'
			OR COALESCE(d.description,'') ILIKE '%'||$1||'%')
		  AND ($2 = '' OR e.category_code = $2)
		  AND ($3 = '' OR e.type_code = $3)
		  AND ($4 = '' OR e.impact = $4)
		  AND ($5 = '' OR e.occurred_at >= $5::timestamptz)
		  AND ($6 = '' OR e.occurred_at < $6::timestamptz + interval '1 day')
		  AND ($7 = '' OR e.pop_id::text = $7)
		  AND ($8 = '' OR e.device_id::text = $8)
		  AND ($9 = '' OR e.technician_id::text = $9)
		  AND ($10 = '' OR e.project_id::text = $10)
	`
	var total int
	if err := s.DB().QueryRow(r.Context(), `
		SELECT COUNT(*) FROM network_events e
		LEFT JOIN pops p ON p.id = e.pop_id
		LEFT JOIN devices d ON d.id = e.device_id
		`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, limit, offset)
	rows, err := s.DB().Query(r.Context(), `
		SELECT e.id, e.occurred_at, e.category_code, e.type_code, e.impact, e.notes,
			e.pop_id, p.description, e.device_id, d.description,
			e.technician_id, COALESCE(NULLIF(trim(tu.display_name),''), tu.email),
			e.project_id, np.description, e.cto_id, cto.description,
			e.cable_id, COALESCE(NULLIF(trim(cab.description),''), 'Cabo #'||cab.display_number::text),
			e.splice_box_id, sp.description,
			e.pole_id, COALESCE(NULLIF(trim(pol.description),''), 'Poste #'||pol.display_number::text),
			e.interface_name, e.vlan, e.created_by,
			COALESCE(NULLIF(trim(cu.display_name),''), cu.email),
			e.created_at, e.updated_at
		FROM network_events e
		LEFT JOIN pops p ON p.id = e.pop_id
		LEFT JOIN devices d ON d.id = e.device_id
		LEFT JOIN users tu ON tu.id = e.technician_id
		LEFT JOIN network_projects np ON np.id = e.project_id
		LEFT JOIN network_ctos cto ON cto.id = e.cto_id
		LEFT JOIN network_cables cab ON cab.id = e.cable_id
		LEFT JOIN network_splice_boxes sp ON sp.id = e.splice_box_id
		LEFT JOIN network_poles pol ON pol.id = e.pole_id
		LEFT JOIN users cu ON cu.id = e.created_by
		`+where+`
		ORDER BY e.occurred_at DESC, e.created_at DESC
		LIMIT $11 OFFSET $12
	`, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	list := []networkEventRow{}
	for rows.Next() {
		var it networkEventRow
		if err := rows.Scan(
			&it.ID, &it.OccurredAt, &it.CategoryCode, &it.TypeCode, &it.Impact, &it.Notes,
			&it.PopID, &it.PopName, &it.DeviceID, &it.DeviceName,
			&it.TechnicianID, &it.TechnicianName,
			&it.ProjectID, &it.ProjectName, &it.CtoID, &it.CtoName,
			&it.CableID, &it.CableName, &it.SpliceBoxID, &it.SpliceBoxName,
			&it.PoleID, &it.PoleName, &it.InterfaceName, &it.VLAN, &it.CreatedBy, &it.CreatedByName,
			&it.CreatedAt, &it.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		if c := networkevents.CategoryByCode(it.CategoryCode); c != nil {
			it.CategoryLabel = c.Label
		} else {
			it.CategoryLabel = it.CategoryCode
		}
		if t := networkevents.TypeByCode(it.TypeCode); t != nil {
			it.TypeLabel = t.Label
		} else {
			it.TypeLabel = it.TypeCode
		}
		list = append(list, it)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

func parseOptUUID(s *string) *uuid.UUID {
	if s == nil {
		return nil
	}
	raw := strings.TrimSpace(*s)
	if raw == "" {
		return nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil
	}
	return &id
}

func netevTrimPtr(s *string) *string {
	if s == nil {
		return nil
	}
	return emptyToNil(*s)
}

func netevText(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func impactLabel(code string) string {
	switch code {
	case "low":
		return "Baixo"
	case "medium":
		return "Médio"
	case "high":
		return "Alto"
	case "critical":
		return "Crítico"
	default:
		return "Nenhum"
	}
}

func parseFlexibleTime(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	layouts := []string{
		time.RFC3339, "2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02 15:04:05", "2006-01-02 15:04", "2006-01-02",
	}
	for _, l := range layouts {
		if t, err := time.ParseInLocation(l, raw, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid time")
}
