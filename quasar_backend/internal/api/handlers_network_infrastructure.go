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
)

var networkFiberColors = []string{
	"Verde", "Amarelo", "Branco", "Azul", "Vermelho", "Violeta",
	"Marrom", "Rosa", "Preto", "Cinza", "Laranja", "Aqua (Turquesa)",
}

var networkProjectStatuses = []string{
	"planejamento", "em_andamento", "concluido", "pausado", "cancelado",
}

var networkCableStatuses = []string{
	"ativo", "planejado", "inativo", "manutencao",
}

func normalizeFiberColor(v string) (string, bool) {
	s := strings.TrimSpace(v)
	if s == "" {
		return "", true
	}
	for _, c := range networkFiberColors {
		if strings.EqualFold(c, s) {
			return c, true
		}
	}
	return "", false
}

func normalizeProjectStatus(v string) (string, bool) {
	s := strings.ToLower(strings.TrimSpace(v))
	if s == "" {
		return "planejamento", true
	}
	for _, st := range networkProjectStatuses {
		if st == s {
			return st, true
		}
	}
	return "", false
}

func normalizeCableStatus(v string) (string, bool) {
	s := strings.ToLower(strings.TrimSpace(v))
	if s == "" {
		return "ativo", true
	}
	for _, st := range networkCableStatuses {
		if st == s {
			return st, true
		}
	}
	return "", false
}

func validateCoords(lat, lon *float64) error {
	if (lat == nil) != (lon == nil) {
		return errors.New("latitude e longitude devem ser preenchidas juntas")
	}
	if lat != nil {
		if *lat < -90 || *lat > 90 || *lon < -180 || *lon > 180 {
			return errors.New("coordenadas fora do intervalo")
		}
	}
	return nil
}

func optionalUUIDFromString(s string) (*uuid.UUID, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return nil, errors.New("uuid inválido")
	}
	return &id, nil
}

func localityNameByID(ctx context.Context, s *Server, id *uuid.UUID) *string {
	if s.DB() == nil || id == nil || *id == uuid.Nil {
		return nil
	}
	var name string
	if err := s.DB().QueryRow(ctx, `SELECT name FROM commercial_localities WHERE id=$1`, *id).Scan(&name); err != nil {
		return nil
	}
	return &name
}

func projectLabelByID(ctx context.Context, s *Server, id *uuid.UUID) *string {
	if s.DB() == nil || id == nil || *id == uuid.Nil {
		return nil
	}
	var num int
	var desc string
	if err := s.DB().QueryRow(ctx, `SELECT display_number, description FROM network_projects WHERE id=$1`, *id).Scan(&num, &desc); err != nil {
		return nil
	}
	lbl := strconv.Itoa(num) + " — " + strings.TrimSpace(desc)
	return &lbl
}

func networkListQuery(r *http.Request) (q string, projectID, localityID *uuid.UUID, maintenanceOnly bool, err error) {
	q = strings.TrimSpace(r.URL.Query().Get("q"))
	if pid := strings.TrimSpace(r.URL.Query().Get("project_id")); pid != "" {
		projectID, err = optionalUUIDFromString(pid)
		if err != nil {
			return
		}
	}
	if lid := strings.TrimSpace(r.URL.Query().Get("locality_id")); lid != "" {
		localityID, err = optionalUUIDFromString(lid)
		if err != nil {
			return
		}
	}
	maintenanceOnly = queryTruthy(r.URL.Query().Get("needs_maintenance"))
	return
}

// --- Projects ---

type networkProjectInput struct {
	Description string   `json:"description"`
	LocalityID  *string  `json:"locality_id"`
	Color       *string  `json:"color"`
	Status      string   `json:"status"`
	Latitude    *float64 `json:"latitude"`
	Longitude   *float64 `json:"longitude"`
}

func (in *networkProjectInput) validate() error {
	if strings.TrimSpace(in.Description) == "" {
		return errors.New("description obrigatória")
	}
	st, ok := normalizeProjectStatus(in.Status)
	if !ok {
		return errors.New("status inválido")
	}
	in.Status = st
	return validateCoords(in.Latitude, in.Longitude)
}

func scanNetworkProject(s *Server, ctx context.Context, rows interface{ Scan(dest ...any) error }) (map[string]any, error) {
	var id uuid.UUID
	var displayNumber int
	var description, status string
	var localityID *uuid.UUID
	var color *string
	var lat, lon *float64
	var created, updated time.Time
	err := rows.Scan(&id, &displayNumber, &description, &localityID, &color, &status, &lat, &lon, &created, &updated)
	if err != nil {
		return nil, err
	}
	m := map[string]any{
		"id": id, "display_number": displayNumber, "description": description,
		"status": status, "created_at": created, "updated_at": updated,
	}
	if localityID != nil {
		m["locality_id"] = *localityID
		if n := localityNameByID(ctx, s, localityID); n != nil {
			m["locality_name"] = *n
		}
	}
	if color != nil {
		m["color"] = *color
	}
	if lat != nil {
		m["latitude"] = *lat
	}
	if lon != nil {
		m["longitude"] = *lon
	}
	return m, nil
}

func (s *Server) listNetworkProjects(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q, projectID, localityID, _, qerr := networkListQuery(r)
	if qerr != nil {
		writeErr(w, http.StatusBadRequest, "VALIDATION", qerr.Error(), nil)
		return
	}
	_ = projectID
	statusFilter := strings.TrimSpace(r.URL.Query().Get("status"))
	args := []any{}
	sqlQ := `
		SELECT id, display_number, description, locality_id, color, status, latitude, longitude, created_at, updated_at
		FROM network_projects WHERE 1=1`
	n := 1
	if q != "" {
		sqlQ += ` AND (description ILIKE $` + strconv.Itoa(n) + ` OR display_number::text = $` + strconv.Itoa(n+1) + `)`
		args = append(args, "%"+q+"%", q)
		n += 2
	}
	if localityID != nil {
		sqlQ += ` AND locality_id = $` + strconv.Itoa(n)
		args = append(args, *localityID)
		n++
	}
	if statusFilter != "" {
		if st, ok := normalizeProjectStatus(statusFilter); ok {
			sqlQ += ` AND status = $` + strconv.Itoa(n)
			args = append(args, st)
			n++
		}
	}
	sqlQ += ` ORDER BY display_number ASC LIMIT 5000`
	rows, err := s.DB().Query(ctx, sqlQ, args...)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	var list []map[string]any
	for rows.Next() {
		item, err := scanNetworkProject(s, ctx, rows)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		list = append(list, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": list})
}

func (s *Server) loadProjectElements(ctx context.Context, projectID uuid.UUID) map[string]any {
	out := map[string]any{
		"ctos":         []map[string]any{},
		"splice_boxes": []map[string]any{},
		"cables":       []map[string]any{},
		"poles":        []map[string]any{},
	}
	loadSimple := func(table, kind string) {
		rows, err := s.DB().Query(ctx, `
			SELECT display_number, description FROM `+table+` WHERE project_id=$1 ORDER BY display_number`, projectID)
		if err != nil {
			return
		}
		defer rows.Close()
		var items []map[string]any
		for rows.Next() {
			var num int
			var desc string
			if err := rows.Scan(&num, &desc); err != nil {
				continue
			}
			items = append(items, map[string]any{"display_number": num, "description": desc, "kind": kind})
		}
		out[kind] = items
	}
	loadSimple("network_ctos", "ctos")
	loadSimple("network_splice_boxes", "splice_boxes")
	loadSimple("network_cables", "cables")
	loadSimple("network_poles", "poles")
	return out
}

func (s *Server) getNetworkProject(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	row := s.DB().QueryRow(ctx, `
		SELECT id, display_number, description, locality_id, color, status, latitude, longitude, created_at, updated_at
		FROM network_projects WHERE id=$1`, id)
	item, err := scanNetworkProject(s, ctx, row)
	if err == pgx.ErrNoRows {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "", nil)
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	item["elements"] = s.loadProjectElements(ctx, id)
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) createNetworkProject(w http.ResponseWriter, r *http.Request) {
	var body networkProjectInput
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", "", nil)
		return
	}
	if err := body.validate(); err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", err.Error(), nil)
		return
	}
	locID, err := optionalUUIDFromString(networkStrPtr(body.LocalityID))
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", err.Error(), nil)
		return
	}
	var id uuid.UUID
	var displayNumber int
	err = s.DB().QueryRow(r.Context(), `
		INSERT INTO network_projects (description, locality_id, color, status, latitude, longitude)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, display_number`,
		strings.TrimSpace(body.Description), locID, trimPtr(body.Color), body.Status, body.Latitude, body.Longitude,
	).Scan(&id, &displayNumber)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "network_project", id.String(), "create", s.actorFromRequest(r), nil, body)
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "display_number": displayNumber})
}

func (s *Server) patchNetworkProject(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	var body map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", "", nil)
		return
	}
	sets := []string{"updated_at = now()"}
	args := []any{id}
	n := 2
	if raw, ok := body["description"]; ok {
		var v string
		_ = json.Unmarshal(raw, &v)
		if strings.TrimSpace(v) == "" {
			writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "description obrigatória", nil)
			return
		}
		sets = append(sets, "description = $"+strconv.Itoa(n))
		args = append(args, strings.TrimSpace(v))
		n++
	}
	if raw, ok := body["locality_id"]; ok {
		var v *string
		_ = json.Unmarshal(raw, &v)
		locID, err := optionalUUIDFromString(networkStrPtr(v))
		if err != nil {
			writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", err.Error(), nil)
			return
		}
		sets = append(sets, "locality_id = $"+strconv.Itoa(n))
		args = append(args, locID)
		n++
	}
	if raw, ok := body["color"]; ok {
		var v *string
		_ = json.Unmarshal(raw, &v)
		sets = append(sets, "color = $"+strconv.Itoa(n))
		args = append(args, trimPtr(v))
		n++
	}
	if raw, ok := body["status"]; ok {
		var v string
		_ = json.Unmarshal(raw, &v)
		st, ok := normalizeProjectStatus(v)
		if !ok {
			writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "status inválido", nil)
			return
		}
		sets = append(sets, "status = $"+strconv.Itoa(n))
		args = append(args, st)
		n++
	}
	if raw, ok := body["latitude"]; ok {
		var lat *float64
		_ = json.Unmarshal(raw, &lat)
		var lon *float64
		if raw2, ok2 := body["longitude"]; ok2 {
			_ = json.Unmarshal(raw2, &lon)
		}
		if err := validateCoords(lat, lon); err != nil {
			writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", err.Error(), nil)
			return
		}
		sets = append(sets, "latitude = $"+strconv.Itoa(n), "longitude = $"+strconv.Itoa(n+1))
		args = append(args, lat, lon)
		n += 2
	} else if raw, ok := body["longitude"]; ok {
		var lon *float64
		_ = json.Unmarshal(raw, &lon)
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "latitude e longitude devem ser enviadas juntas", nil)
		return
	}
	if len(sets) == 1 {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "nenhum campo para actualizar", nil)
		return
	}
	tag, err := s.DB().Exec(ctx, `UPDATE network_projects SET `+strings.Join(sets, ", ")+` WHERE id=$1`, args...)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "", nil)
		return
	}
	s.appendAuditLog(ctx, "network_project", id.String(), "patch", s.actorFromRequest(r), nil, body)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) deleteNetworkProject(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	tag, err := s.DB().Exec(ctx, `DELETE FROM network_projects WHERE id=$1`, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "", nil)
		return
	}
	s.appendAuditLog(ctx, "network_project", id.String(), "delete", s.actorFromRequest(r), nil, nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func networkStrPtr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func trimPtr(p *string) *string {
	if p == nil {
		return nil
	}
	s := strings.TrimSpace(*p)
	if s == "" {
		return nil
	}
	return &s
}

// --- CTOs ---

type networkCtoInput struct {
	Description      string   `json:"description"`
	Latitude         *float64 `json:"latitude"`
	Longitude        *float64 `json:"longitude"`
	Splitter         *string  `json:"splitter"`
	FiberColor       *string  `json:"fiber_color"`
	Notes            *string  `json:"notes"`
	NeedsMaintenance *bool    `json:"needs_maintenance"`
	ProjectID        *string  `json:"project_id"`
	LocalityID       *string  `json:"locality_id"`
}

func (in *networkCtoInput) validate() error {
	if strings.TrimSpace(in.Description) == "" {
		return errors.New("description obrigatória")
	}
	if in.FiberColor != nil && strings.TrimSpace(*in.FiberColor) != "" {
		c, ok := normalizeFiberColor(*in.FiberColor)
		if !ok {
			return errors.New("fiber_color inválida")
		}
		in.FiberColor = &c
	}
	return validateCoords(in.Latitude, in.Longitude)
}

func scanNetworkCto(s *Server, ctx context.Context, rows interface{ Scan(dest ...any) error }) (map[string]any, error) {
	var id uuid.UUID
	var displayNumber int
	var description string
	var splitter, fiberColor, notes *string
	var projectID, localityID *uuid.UUID
	var needsMaintenance bool
	var lat, lon *float64
	var created, updated time.Time
	err := rows.Scan(&id, &displayNumber, &description, &lat, &lon, &splitter, &fiberColor, &notes,
		&needsMaintenance, &projectID, &localityID, &created, &updated)
	if err != nil {
		return nil, err
	}
	m := map[string]any{
		"id": id, "display_number": displayNumber, "description": description,
		"needs_maintenance": needsMaintenance, "created_at": created, "updated_at": updated,
	}
	setOptionalStr(m, "splitter", splitter)
	setOptionalStr(m, "fiber_color", fiberColor)
	setOptionalStr(m, "notes", notes)
	if lat != nil {
		m["latitude"] = *lat
	}
	if lon != nil {
		m["longitude"] = *lon
	}
	if projectID != nil {
		m["project_id"] = *projectID
		if lbl := projectLabelByID(ctx, s, projectID); lbl != nil {
			m["project_label"] = *lbl
		}
	}
	if localityID != nil {
		m["locality_id"] = *localityID
		if n := localityNameByID(ctx, s, localityID); n != nil {
			m["locality_name"] = *n
		}
	}
	return m, nil
}

const networkCtoSelect = `id, display_number, description, latitude, longitude, splitter, fiber_color, notes,
	needs_maintenance, project_id, locality_id, created_at, updated_at`

func (s *Server) listNetworkCtos(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q, projectID, localityID, maintenanceOnly, qerr := networkListQuery(r)
	if qerr != nil {
		writeErr(w, http.StatusBadRequest, "VALIDATION", qerr.Error(), nil)
		return
	}
	fiberColor := strings.TrimSpace(r.URL.Query().Get("fiber_color"))
	splitter := strings.TrimSpace(r.URL.Query().Get("splitter"))
	args := []any{}
	sqlQ := `SELECT ` + networkCtoSelect + ` FROM network_ctos WHERE 1=1`
	n := 1
	if q != "" {
		sqlQ += ` AND (description ILIKE $` + strconv.Itoa(n) + ` OR display_number::text = $` + strconv.Itoa(n+1) + ` OR COALESCE(splitter,'') ILIKE $` + strconv.Itoa(n) + `)`
		args = append(args, "%"+q+"%", q)
		n += 2
	}
	if projectID != nil {
		sqlQ += ` AND project_id = $` + strconv.Itoa(n)
		args = append(args, *projectID)
		n++
	}
	if localityID != nil {
		sqlQ += ` AND locality_id = $` + strconv.Itoa(n)
		args = append(args, *localityID)
		n++
	}
	if maintenanceOnly {
		sqlQ += ` AND needs_maintenance = true`
	}
	if fiberColor != "" {
		if c, ok := normalizeFiberColor(fiberColor); ok && c != "" {
			sqlQ += ` AND fiber_color = $` + strconv.Itoa(n)
			args = append(args, c)
			n++
		}
	}
	if splitter != "" {
		sqlQ += ` AND COALESCE(splitter,'') ILIKE $` + strconv.Itoa(n)
		args = append(args, "%"+splitter+"%")
		n++
	}
	sqlQ += ` ORDER BY display_number ASC LIMIT 5000`
	rows, err := s.DB().Query(ctx, sqlQ, args...)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	var list []map[string]any
	for rows.Next() {
		item, err := scanNetworkCto(s, ctx, rows)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		list = append(list, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ctos": list})
}

func (s *Server) getNetworkCto(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	row := s.DB().QueryRow(ctx, `SELECT `+networkCtoSelect+` FROM network_ctos WHERE id=$1`, id)
	item, err := scanNetworkCto(s, ctx, row)
	if err == pgx.ErrNoRows {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "", nil)
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) createNetworkCto(w http.ResponseWriter, r *http.Request) {
	var body networkCtoInput
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", "", nil)
		return
	}
	if err := body.validate(); err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", err.Error(), nil)
		return
	}
	projectID, err := optionalUUIDFromString(networkStrPtr(body.ProjectID))
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", err.Error(), nil)
		return
	}
	localityID, err := optionalUUIDFromString(networkStrPtr(body.LocalityID))
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", err.Error(), nil)
		return
	}
	needsMaint := false
	if body.NeedsMaintenance != nil {
		needsMaint = *body.NeedsMaintenance
	}
	var id uuid.UUID
	var displayNumber int
	err = s.DB().QueryRow(r.Context(), `
		INSERT INTO network_ctos (description, latitude, longitude, splitter, fiber_color, notes, needs_maintenance, project_id, locality_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id, display_number`,
		strings.TrimSpace(body.Description), body.Latitude, body.Longitude, trimPtr(body.Splitter), trimPtr(body.FiberColor),
		trimPtr(body.Notes), needsMaint, projectID, localityID,
	).Scan(&id, &displayNumber)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "network_cto", id.String(), "create", s.actorFromRequest(r), nil, body)
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "display_number": displayNumber})
}

func (s *Server) patchNetworkCto(w http.ResponseWriter, r *http.Request) {
	s.patchNetworkRow(w, r, "network_ctos", "network_cto", networkCtoPatch)
}

func networkCtoPatch(body map[string]json.RawMessage) ([]string, []any, int, error) {
	sets := []string{}
	args := []any{}
	n := 2
	if raw, ok := body["description"]; ok {
		var v string
		_ = json.Unmarshal(raw, &v)
		if strings.TrimSpace(v) == "" {
			return nil, nil, 0, errors.New("description obrigatória")
		}
		sets = append(sets, "description = $"+strconv.Itoa(n))
		args = append(args, strings.TrimSpace(v))
		n++
	}
	if raw, ok := body["fiber_color"]; ok {
		var v *string
		_ = json.Unmarshal(raw, &v)
		if v != nil && strings.TrimSpace(*v) != "" {
			c, ok := normalizeFiberColor(*v)
			if !ok {
				return nil, nil, 0, errors.New("fiber_color inválida")
			}
			sets = append(sets, "fiber_color = $"+strconv.Itoa(n))
			args = append(args, c)
			n++
		} else {
			sets = append(sets, "fiber_color = $"+strconv.Itoa(n))
			args = append(args, nil)
			n++
		}
	}
	for _, fld := range []struct {
		key string
		col string
	}{
		{"splitter", "splitter"}, {"notes", "notes"},
	} {
		if raw, ok := body[fld.key]; ok {
			var v *string
			_ = json.Unmarshal(raw, &v)
			sets = append(sets, fld.col+" = $"+strconv.Itoa(n))
			args = append(args, trimPtr(v))
			n++
		}
	}
	if raw, ok := body["needs_maintenance"]; ok {
		var v bool
		_ = json.Unmarshal(raw, &v)
		sets = append(sets, "needs_maintenance = $"+strconv.Itoa(n))
		args = append(args, v)
		n++
	}
	if raw, ok := body["project_id"]; ok {
		var v *string
		_ = json.Unmarshal(raw, &v)
		pid, err := optionalUUIDFromString(networkStrPtr(v))
		if err != nil {
			return nil, nil, 0, err
		}
		sets = append(sets, "project_id = $"+strconv.Itoa(n))
		args = append(args, pid)
		n++
	}
	if raw, ok := body["locality_id"]; ok {
		var v *string
		_ = json.Unmarshal(raw, &v)
		lid, err := optionalUUIDFromString(networkStrPtr(v))
		if err != nil {
			return nil, nil, 0, err
		}
		sets = append(sets, "locality_id = $"+strconv.Itoa(n))
		args = append(args, lid)
		n++
	}
	if raw, ok := body["latitude"]; ok {
		var lat *float64
		_ = json.Unmarshal(raw, &lat)
		var lon *float64
		if raw2, ok2 := body["longitude"]; ok2 {
			_ = json.Unmarshal(raw2, &lon)
		}
		if err := validateCoords(lat, lon); err != nil {
			return nil, nil, 0, err
		}
		sets = append(sets, "latitude = $"+strconv.Itoa(n), "longitude = $"+strconv.Itoa(n+1))
		args = append(args, lat, lon)
		n += 2
	}
	return sets, args, n, nil
}

func (s *Server) deleteNetworkCto(w http.ResponseWriter, r *http.Request) {
	s.deleteNetworkRow(w, r, "network_ctos", "network_cto")
}

func (s *Server) patchNetworkRow(w http.ResponseWriter, r *http.Request, table, auditEntity string, patchFn func(map[string]json.RawMessage) ([]string, []any, int, error)) {
	ctx := r.Context()
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	var body map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", "", nil)
		return
	}
	sets, args, _, perr := patchFn(body)
	if perr != nil {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", perr.Error(), nil)
		return
	}
	if len(sets) == 0 {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "nenhum campo para actualizar", nil)
		return
	}
	sets = append(sets, "updated_at = now()")
	allArgs := append([]any{id}, args...)
	tag, err := s.DB().Exec(ctx, `UPDATE `+table+` SET `+strings.Join(sets, ", ")+` WHERE id=$1`, allArgs...)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "", nil)
		return
	}
	s.appendAuditLog(ctx, auditEntity, id.String(), "patch", s.actorFromRequest(r), nil, body)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) deleteNetworkRow(w http.ResponseWriter, r *http.Request, table, auditEntity string) {
	ctx := r.Context()
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	tag, err := s.DB().Exec(ctx, `DELETE FROM `+table+` WHERE id=$1`, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "", nil)
		return
	}
	s.appendAuditLog(ctx, auditEntity, id.String(), "delete", s.actorFromRequest(r), nil, nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// --- Splice boxes ---

type networkSpliceBoxInput struct {
	Description      string   `json:"description"`
	Latitude         *float64 `json:"latitude"`
	Longitude        *float64 `json:"longitude"`
	FiberCount       *int     `json:"fiber_count"`
	NeedsMaintenance *bool    `json:"needs_maintenance"`
	Notes            *string  `json:"notes"`
	ProjectID        *string  `json:"project_id"`
}

func (in *networkSpliceBoxInput) validate() error {
	if strings.TrimSpace(in.Description) == "" {
		return errors.New("description obrigatória")
	}
	if in.FiberCount != nil && *in.FiberCount < 0 {
		return errors.New("fiber_count inválida")
	}
	return validateCoords(in.Latitude, in.Longitude)
}

const networkSpliceSelect = `id, display_number, description, latitude, longitude, fiber_count, needs_maintenance, notes, project_id, created_at, updated_at`

func scanNetworkSpliceBox(s *Server, ctx context.Context, rows interface{ Scan(dest ...any) error }) (map[string]any, error) {
	var id uuid.UUID
	var displayNumber int
	var description string
	var notes *string
	var fiberCount *int
	var projectID *uuid.UUID
	var needsMaintenance bool
	var lat, lon *float64
	var created, updated time.Time
	err := rows.Scan(&id, &displayNumber, &description, &lat, &lon, &fiberCount, &needsMaintenance, &notes, &projectID, &created, &updated)
	if err != nil {
		return nil, err
	}
	m := map[string]any{
		"id": id, "display_number": displayNumber, "description": description,
		"needs_maintenance": needsMaintenance, "created_at": created, "updated_at": updated,
	}
	setOptionalStr(m, "notes", notes)
	if fiberCount != nil {
		m["fiber_count"] = *fiberCount
	}
	if lat != nil {
		m["latitude"] = *lat
	}
	if lon != nil {
		m["longitude"] = *lon
	}
	if projectID != nil {
		m["project_id"] = *projectID
		if lbl := projectLabelByID(ctx, s, projectID); lbl != nil {
			m["project_label"] = *lbl
		}
	}
	return m, nil
}

func (s *Server) listNetworkSpliceBoxes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q, projectID, _, maintenanceOnly, qerr := networkListQuery(r)
	if qerr != nil {
		writeErr(w, http.StatusBadRequest, "VALIDATION", qerr.Error(), nil)
		return
	}
	args := []any{}
	sqlQ := `SELECT ` + networkSpliceSelect + ` FROM network_splice_boxes WHERE 1=1`
	n := 1
	if q != "" {
		sqlQ += ` AND (description ILIKE $` + strconv.Itoa(n) + ` OR display_number::text = $` + strconv.Itoa(n+1) + `)`
		args = append(args, "%"+q+"%", q)
		n += 2
	}
	if projectID != nil {
		sqlQ += ` AND project_id = $` + strconv.Itoa(n)
		args = append(args, *projectID)
		n++
	}
	if maintenanceOnly {
		sqlQ += ` AND needs_maintenance = true`
	}
	sqlQ += ` ORDER BY display_number ASC LIMIT 5000`
	rows, err := s.DB().Query(ctx, sqlQ, args...)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	var list []map[string]any
	for rows.Next() {
		item, err := scanNetworkSpliceBox(s, ctx, rows)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		list = append(list, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"splice_boxes": list})
}

func (s *Server) getNetworkSpliceBox(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	row := s.DB().QueryRow(ctx, `SELECT `+networkSpliceSelect+` FROM network_splice_boxes WHERE id=$1`, id)
	item, err := scanNetworkSpliceBox(s, ctx, row)
	if err == pgx.ErrNoRows {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "", nil)
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) createNetworkSpliceBox(w http.ResponseWriter, r *http.Request) {
	var body networkSpliceBoxInput
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", "", nil)
		return
	}
	if err := body.validate(); err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", err.Error(), nil)
		return
	}
	projectID, err := optionalUUIDFromString(networkStrPtr(body.ProjectID))
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", err.Error(), nil)
		return
	}
	needsMaint := false
	if body.NeedsMaintenance != nil {
		needsMaint = *body.NeedsMaintenance
	}
	var id uuid.UUID
	var displayNumber int
	err = s.DB().QueryRow(r.Context(), `
		INSERT INTO network_splice_boxes (description, latitude, longitude, fiber_count, needs_maintenance, notes, project_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id, display_number`,
		strings.TrimSpace(body.Description), body.Latitude, body.Longitude, body.FiberCount, needsMaint, trimPtr(body.Notes), projectID,
	).Scan(&id, &displayNumber)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "network_splice_box", id.String(), "create", s.actorFromRequest(r), nil, body)
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "display_number": displayNumber})
}

func (s *Server) patchNetworkSpliceBox(w http.ResponseWriter, r *http.Request) {
	s.patchNetworkRow(w, r, "network_splice_boxes", "network_splice_box", networkSplicePatch)
}

func networkSplicePatch(body map[string]json.RawMessage) ([]string, []any, int, error) {
	sets := []string{}
	args := []any{}
	n := 2
	if raw, ok := body["description"]; ok {
		var v string
		_ = json.Unmarshal(raw, &v)
		if strings.TrimSpace(v) == "" {
			return nil, nil, 0, errors.New("description obrigatória")
		}
		sets = append(sets, "description = $"+strconv.Itoa(n))
		args = append(args, strings.TrimSpace(v))
		n++
	}
	if raw, ok := body["notes"]; ok {
		var v *string
		_ = json.Unmarshal(raw, &v)
		sets = append(sets, "notes = $"+strconv.Itoa(n))
		args = append(args, trimPtr(v))
		n++
	}
	if raw, ok := body["fiber_count"]; ok {
		var v *int
		_ = json.Unmarshal(raw, &v)
		if v != nil && *v < 0 {
			return nil, nil, 0, errors.New("fiber_count inválida")
		}
		sets = append(sets, "fiber_count = $"+strconv.Itoa(n))
		args = append(args, v)
		n++
	}
	if raw, ok := body["needs_maintenance"]; ok {
		var v bool
		_ = json.Unmarshal(raw, &v)
		sets = append(sets, "needs_maintenance = $"+strconv.Itoa(n))
		args = append(args, v)
		n++
	}
	if raw, ok := body["project_id"]; ok {
		var v *string
		_ = json.Unmarshal(raw, &v)
		pid, err := optionalUUIDFromString(networkStrPtr(v))
		if err != nil {
			return nil, nil, 0, err
		}
		sets = append(sets, "project_id = $"+strconv.Itoa(n))
		args = append(args, pid)
		n++
	}
	if raw, ok := body["latitude"]; ok {
		var lat *float64
		_ = json.Unmarshal(raw, &lat)
		var lon *float64
		if raw2, ok2 := body["longitude"]; ok2 {
			_ = json.Unmarshal(raw2, &lon)
		}
		if err := validateCoords(lat, lon); err != nil {
			return nil, nil, 0, err
		}
		sets = append(sets, "latitude = $"+strconv.Itoa(n), "longitude = $"+strconv.Itoa(n+1))
		args = append(args, lat, lon)
		n += 2
	}
	return sets, args, n, nil
}

func (s *Server) deleteNetworkSpliceBox(w http.ResponseWriter, r *http.Request) {
	s.deleteNetworkRow(w, r, "network_splice_boxes", "network_splice_box")
}

// --- Cables (estrutura inicial) ---

type networkCableInput struct {
	Description string   `json:"description"`
	CableType   *string  `json:"cable_type"`
	FiberCount  *int     `json:"fiber_count"`
	Status      string   `json:"status"`
	ProjectID   *string  `json:"project_id"`
	Latitude    *float64 `json:"latitude"`
	Longitude   *float64 `json:"longitude"`
}

func (in *networkCableInput) validate() error {
	st, ok := normalizeCableStatus(in.Status)
	if !ok {
		return errors.New("status inválido")
	}
	in.Status = st
	if in.FiberCount != nil && *in.FiberCount < 0 {
		return errors.New("fiber_count inválida")
	}
	return validateCoords(in.Latitude, in.Longitude)
}

const networkCableSelect = `id, display_number, description, cable_type, fiber_count, status, project_id, latitude, longitude, created_at, updated_at`

func scanNetworkCable(s *Server, ctx context.Context, rows interface{ Scan(dest ...any) error }) (map[string]any, error) {
	var id uuid.UUID
	var displayNumber int
	var description, status string
	var cableType *string
	var fiberCount *int
	var projectID *uuid.UUID
	var lat, lon *float64
	var created, updated time.Time
	err := rows.Scan(&id, &displayNumber, &description, &cableType, &fiberCount, &status, &projectID, &lat, &lon, &created, &updated)
	if err != nil {
		return nil, err
	}
	m := map[string]any{"id": id, "display_number": displayNumber, "description": description, "status": status, "created_at": created, "updated_at": updated}
	setOptionalStr(m, "cable_type", cableType)
	if fiberCount != nil {
		m["fiber_count"] = *fiberCount
	}
	if lat != nil {
		m["latitude"] = *lat
	}
	if lon != nil {
		m["longitude"] = *lon
	}
	if projectID != nil {
		m["project_id"] = *projectID
		if lbl := projectLabelByID(ctx, s, projectID); lbl != nil {
			m["project_label"] = *lbl
		}
	}
	return m, nil
}

func (s *Server) listNetworkCables(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q, projectID, _, _, qerr := networkListQuery(r)
	if qerr != nil {
		writeErr(w, http.StatusBadRequest, "VALIDATION", qerr.Error(), nil)
		return
	}
	statusFilter := strings.TrimSpace(r.URL.Query().Get("status"))
	cableType := strings.TrimSpace(r.URL.Query().Get("cable_type"))
	args := []any{}
	sqlQ := `SELECT ` + networkCableSelect + ` FROM network_cables WHERE 1=1`
	n := 1
	if q != "" {
		sqlQ += ` AND (description ILIKE $` + strconv.Itoa(n) + ` OR display_number::text = $` + strconv.Itoa(n+1) + `)`
		args = append(args, "%"+q+"%", q)
		n += 2
	}
	if projectID != nil {
		sqlQ += ` AND project_id = $` + strconv.Itoa(n)
		args = append(args, *projectID)
		n++
	}
	if statusFilter != "" {
		if st, ok := normalizeCableStatus(statusFilter); ok {
			sqlQ += ` AND status = $` + strconv.Itoa(n)
			args = append(args, st)
			n++
		}
	}
	if cableType != "" {
		sqlQ += ` AND COALESCE(cable_type,'') ILIKE $` + strconv.Itoa(n)
		args = append(args, "%"+cableType+"%")
		n++
	}
	sqlQ += ` ORDER BY display_number ASC LIMIT 5000`
	rows, err := s.DB().Query(ctx, sqlQ, args...)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	var list []map[string]any
	for rows.Next() {
		item, err := scanNetworkCable(s, ctx, rows)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		list = append(list, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"cables": list})
}

func (s *Server) createNetworkCable(w http.ResponseWriter, r *http.Request) {
	var body networkCableInput
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", "", nil)
		return
	}
	if err := body.validate(); err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", err.Error(), nil)
		return
	}
	projectID, err := optionalUUIDFromString(networkStrPtr(body.ProjectID))
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", err.Error(), nil)
		return
	}
	var id uuid.UUID
	var displayNumber int
	err = s.DB().QueryRow(r.Context(), `
		INSERT INTO network_cables (description, cable_type, fiber_count, status, project_id, latitude, longitude)
		VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id, display_number`,
		strings.TrimSpace(body.Description), trimPtr(body.CableType), body.FiberCount, body.Status, projectID, body.Latitude, body.Longitude,
	).Scan(&id, &displayNumber)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "network_cable", id.String(), "create", s.actorFromRequest(r), nil, body)
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "display_number": displayNumber})
}

func (s *Server) patchNetworkCable(w http.ResponseWriter, r *http.Request) {
	s.patchNetworkRow(w, r, "network_cables", "network_cable", networkCablePatch)
}

func networkCablePatch(body map[string]json.RawMessage) ([]string, []any, int, error) {
	sets := []string{}
	args := []any{}
	n := 2
	if raw, ok := body["description"]; ok {
		var v string
		_ = json.Unmarshal(raw, &v)
		sets = append(sets, "description = $"+strconv.Itoa(n))
		args = append(args, strings.TrimSpace(v))
		n++
	}
	if raw, ok := body["cable_type"]; ok {
		var v *string
		_ = json.Unmarshal(raw, &v)
		sets = append(sets, "cable_type = $"+strconv.Itoa(n))
		args = append(args, trimPtr(v))
		n++
	}
	if raw, ok := body["fiber_count"]; ok {
		var v *int
		_ = json.Unmarshal(raw, &v)
		sets = append(sets, "fiber_count = $"+strconv.Itoa(n))
		args = append(args, v)
		n++
	}
	if raw, ok := body["status"]; ok {
		var v string
		_ = json.Unmarshal(raw, &v)
		st, ok := normalizeCableStatus(v)
		if !ok {
			return nil, nil, 0, errors.New("status inválido")
		}
		sets = append(sets, "status = $"+strconv.Itoa(n))
		args = append(args, st)
		n++
	}
	if raw, ok := body["project_id"]; ok {
		var v *string
		_ = json.Unmarshal(raw, &v)
		pid, err := optionalUUIDFromString(networkStrPtr(v))
		if err != nil {
			return nil, nil, 0, err
		}
		sets = append(sets, "project_id = $"+strconv.Itoa(n))
		args = append(args, pid)
		n++
	}
	if raw, ok := body["latitude"]; ok {
		var lat *float64
		_ = json.Unmarshal(raw, &lat)
		var lon *float64
		if raw2, ok2 := body["longitude"]; ok2 {
			_ = json.Unmarshal(raw2, &lon)
		}
		if err := validateCoords(lat, lon); err != nil {
			return nil, nil, 0, err
		}
		sets = append(sets, "latitude = $"+strconv.Itoa(n), "longitude = $"+strconv.Itoa(n+1))
		args = append(args, lat, lon)
		n += 2
	}
	return sets, args, n, nil
}

func (s *Server) deleteNetworkCable(w http.ResponseWriter, r *http.Request) {
	s.deleteNetworkRow(w, r, "network_cables", "network_cable")
}

// --- Poles ---

type networkPoleInput struct {
	Description string   `json:"description"`
	PoleType    *string  `json:"pole_type"`
	ProjectID   *string  `json:"project_id"`
	LocalityID  *string  `json:"locality_id"`
	Latitude    *float64 `json:"latitude"`
	Longitude   *float64 `json:"longitude"`
}

func (in *networkPoleInput) validate() error {
	return validateCoords(in.Latitude, in.Longitude)
}

const networkPoleSelect = `id, display_number, description, pole_type, project_id, locality_id, latitude, longitude, created_at, updated_at`

func scanNetworkPole(s *Server, ctx context.Context, rows interface{ Scan(dest ...any) error }) (map[string]any, error) {
	var id uuid.UUID
	var displayNumber int
	var description string
	var poleType *string
	var projectID, localityID *uuid.UUID
	var lat, lon *float64
	var created, updated time.Time
	err := rows.Scan(&id, &displayNumber, &description, &poleType, &projectID, &localityID, &lat, &lon, &created, &updated)
	if err != nil {
		return nil, err
	}
	m := map[string]any{"id": id, "display_number": displayNumber, "description": description, "created_at": created, "updated_at": updated}
	setOptionalStr(m, "pole_type", poleType)
	if lat != nil {
		m["latitude"] = *lat
	}
	if lon != nil {
		m["longitude"] = *lon
	}
	if projectID != nil {
		m["project_id"] = *projectID
		if lbl := projectLabelByID(ctx, s, projectID); lbl != nil {
			m["project_label"] = *lbl
		}
	}
	if localityID != nil {
		m["locality_id"] = *localityID
		if n := localityNameByID(ctx, s, localityID); n != nil {
			m["locality_name"] = *n
		}
	}
	return m, nil
}

func (s *Server) listNetworkPoles(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q, projectID, localityID, _, qerr := networkListQuery(r)
	if qerr != nil {
		writeErr(w, http.StatusBadRequest, "VALIDATION", qerr.Error(), nil)
		return
	}
	poleType := strings.TrimSpace(r.URL.Query().Get("pole_type"))
	args := []any{}
	sqlQ := `SELECT ` + networkPoleSelect + ` FROM network_poles WHERE 1=1`
	n := 1
	if q != "" {
		sqlQ += ` AND (description ILIKE $` + strconv.Itoa(n) + ` OR display_number::text = $` + strconv.Itoa(n+1) + `)`
		args = append(args, "%"+q+"%", q)
		n += 2
	}
	if projectID != nil {
		sqlQ += ` AND project_id = $` + strconv.Itoa(n)
		args = append(args, *projectID)
		n++
	}
	if localityID != nil {
		sqlQ += ` AND locality_id = $` + strconv.Itoa(n)
		args = append(args, *localityID)
		n++
	}
	if poleType != "" {
		sqlQ += ` AND COALESCE(pole_type,'') ILIKE $` + strconv.Itoa(n)
		args = append(args, "%"+poleType+"%")
		n++
	}
	sqlQ += ` ORDER BY display_number ASC LIMIT 5000`
	rows, err := s.DB().Query(ctx, sqlQ, args...)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	var list []map[string]any
	for rows.Next() {
		item, err := scanNetworkPole(s, ctx, rows)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		list = append(list, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"poles": list})
}

func (s *Server) createNetworkPole(w http.ResponseWriter, r *http.Request) {
	var body networkPoleInput
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", "", nil)
		return
	}
	if err := body.validate(); err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", err.Error(), nil)
		return
	}
	projectID, err := optionalUUIDFromString(networkStrPtr(body.ProjectID))
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", err.Error(), nil)
		return
	}
	localityID, err := optionalUUIDFromString(networkStrPtr(body.LocalityID))
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", err.Error(), nil)
		return
	}
	var id uuid.UUID
	var displayNumber int
	err = s.DB().QueryRow(r.Context(), `
		INSERT INTO network_poles (description, pole_type, project_id, locality_id, latitude, longitude)
		VALUES ($1,$2,$3,$4,$5,$6) RETURNING id, display_number`,
		strings.TrimSpace(body.Description), trimPtr(body.PoleType), projectID, localityID, body.Latitude, body.Longitude,
	).Scan(&id, &displayNumber)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "network_pole", id.String(), "create", s.actorFromRequest(r), nil, body)
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "display_number": displayNumber})
}

func (s *Server) patchNetworkPole(w http.ResponseWriter, r *http.Request) {
	s.patchNetworkRow(w, r, "network_poles", "network_pole", networkPolePatch)
}

func networkPolePatch(body map[string]json.RawMessage) ([]string, []any, int, error) {
	sets := []string{}
	args := []any{}
	n := 2
	if raw, ok := body["description"]; ok {
		var v string
		_ = json.Unmarshal(raw, &v)
		sets = append(sets, "description = $"+strconv.Itoa(n))
		args = append(args, strings.TrimSpace(v))
		n++
	}
	if raw, ok := body["pole_type"]; ok {
		var v *string
		_ = json.Unmarshal(raw, &v)
		sets = append(sets, "pole_type = $"+strconv.Itoa(n))
		args = append(args, trimPtr(v))
		n++
	}
	if raw, ok := body["project_id"]; ok {
		var v *string
		_ = json.Unmarshal(raw, &v)
		pid, err := optionalUUIDFromString(networkStrPtr(v))
		if err != nil {
			return nil, nil, 0, err
		}
		sets = append(sets, "project_id = $"+strconv.Itoa(n))
		args = append(args, pid)
		n++
	}
	if raw, ok := body["locality_id"]; ok {
		var v *string
		_ = json.Unmarshal(raw, &v)
		lid, err := optionalUUIDFromString(networkStrPtr(v))
		if err != nil {
			return nil, nil, 0, err
		}
		sets = append(sets, "locality_id = $"+strconv.Itoa(n))
		args = append(args, lid)
		n++
	}
	if raw, ok := body["latitude"]; ok {
		var lat *float64
		_ = json.Unmarshal(raw, &lat)
		var lon *float64
		if raw2, ok2 := body["longitude"]; ok2 {
			_ = json.Unmarshal(raw2, &lon)
		}
		if err := validateCoords(lat, lon); err != nil {
			return nil, nil, 0, err
		}
		sets = append(sets, "latitude = $"+strconv.Itoa(n), "longitude = $"+strconv.Itoa(n+1))
		args = append(args, lat, lon)
		n += 2
	}
	return sets, args, n, nil
}

func (s *Server) deleteNetworkPole(w http.ResponseWriter, r *http.Request) {
	s.deleteNetworkRow(w, r, "network_poles", "network_pole")
}

func (s *Server) listNetworkFiberColors(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"fiber_colors": networkFiberColors})
}
