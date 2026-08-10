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
)

func fleetQ(r *http.Request) string {
	return strings.TrimSpace(r.URL.Query().Get("q"))
}

func fleetStatusFilter(r *http.Request) string {
	return strings.TrimSpace(r.URL.Query().Get("status"))
}

func fleetLimitOffset(r *http.Request) (limit, offset int) {
	limit = 200
	offset = 0
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v <= 1000 {
		limit = v
	}
	if v, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil && v >= 0 {
		offset = v
	}
	return
}

func ptrTrim(s *string) *string {
	if s == nil {
		return nil
	}
	t := strings.TrimSpace(*s)
	if t == "" {
		return nil
	}
	return &t
}

func emptyToNil(s string) *string {
	t := strings.TrimSpace(s)
	if t == "" {
		return nil
	}
	return &t
}

// --- Cost centers ---

type fleetCostCenter struct {
	ID          uuid.UUID  `json:"id"`
	Code        string     `json:"code"`
	Description string     `json:"description"`
	ParentID    *uuid.UUID `json:"parent_id"`
	Status      string     `json:"status"`
	Notes       *string    `json:"notes"`
}

func (s *Server) listFleetCostCenters(w http.ResponseWriter, r *http.Request) {
	q := fleetQ(r)
	st := fleetStatusFilter(r)
	rows, err := s.DB().Query(r.Context(), `
		SELECT id, code, description, parent_id, status, notes
		FROM fleet_cost_centers
		WHERE ($1 = '' OR code ILIKE '%'||$1||'%' OR description ILIKE '%'||$1||'%')
		  AND ($2 = '' OR status = $2)
		ORDER BY code
	`, q, st)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	list := []fleetCostCenter{}
	for rows.Next() {
		var it fleetCostCenter
		if err := rows.Scan(&it.ID, &it.Code, &it.Description, &it.ParentID, &it.Status, &it.Notes); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		list = append(list, it)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list})
}

func (s *Server) createFleetCostCenter(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code        string     `json:"code"`
		Description string     `json:"description"`
		ParentID    *uuid.UUID `json:"parent_id"`
		Status      string     `json:"status"`
		Notes       *string    `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Code) == "" || strings.TrimSpace(body.Description) == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "code e description obrigatórios", nil)
		return
	}
	if body.Status == "" {
		body.Status = "active"
	}
	uid := s.userIDFromRequest(r)
	var id uuid.UUID
	err := s.DB().QueryRow(r.Context(), `
		INSERT INTO fleet_cost_centers (code, description, parent_id, status, notes, created_by, updated_by)
		VALUES ($1,$2,$3,$4,$5,$6,$6) RETURNING id
	`, strings.TrimSpace(body.Code), strings.TrimSpace(body.Description), body.ParentID, body.Status, ptrTrim(body.Notes), uid).Scan(&id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (s *Server) patchFleetCostCenter(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "UUID inválido", nil)
		return
	}
	var body map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	var cur fleetCostCenter
	if err := s.DB().QueryRow(r.Context(), `SELECT id, code, description, parent_id, status, notes FROM fleet_cost_centers WHERE id=$1`, id).
		Scan(&cur.ID, &cur.Code, &cur.Description, &cur.ParentID, &cur.Status, &cur.Notes); err == pgx.ErrNoRows {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "centro de custo não encontrado", nil)
		return
	} else if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if v, ok := body["code"]; ok {
		_ = json.Unmarshal(v, &cur.Code)
	}
	if v, ok := body["description"]; ok {
		_ = json.Unmarshal(v, &cur.Description)
	}
	if v, ok := body["parent_id"]; ok {
		_ = json.Unmarshal(v, &cur.ParentID)
	}
	if v, ok := body["status"]; ok {
		_ = json.Unmarshal(v, &cur.Status)
	}
	if v, ok := body["notes"]; ok {
		_ = json.Unmarshal(v, &cur.Notes)
	}
	_, err = s.DB().Exec(r.Context(), `
		UPDATE fleet_cost_centers SET code=$2, description=$3, parent_id=$4, status=$5, notes=$6, updated_at=now(), updated_by=$7 WHERE id=$1
	`, id, strings.TrimSpace(cur.Code), strings.TrimSpace(cur.Description), cur.ParentID, cur.Status, ptrTrim(cur.Notes), s.userIDFromRequest(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// --- Fuels ---

type fleetFuel struct {
	ID          uuid.UUID `json:"id"`
	Description string    `json:"description"`
	Code        *string   `json:"code"`
	FuelType    *string   `json:"fuel_type"`
	Unit        string    `json:"unit"`
	Active      bool      `json:"active"`
	Notes       *string   `json:"notes"`
}

func (s *Server) listFleetFuels(w http.ResponseWriter, r *http.Request) {
	q := fleetQ(r)
	onlyActive := r.URL.Query().Get("active") == "1" || r.URL.Query().Get("active") == "true"
	rows, err := s.DB().Query(r.Context(), `
		SELECT id, description, code, fuel_type, unit, active, notes
		FROM fleet_fuels
		WHERE ($1 = '' OR description ILIKE '%'||$1||'%' OR COALESCE(code,'') ILIKE '%'||$1||'%')
		  AND (NOT $2 OR active)
		ORDER BY description
	`, q, onlyActive)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	list := []fleetFuel{}
	for rows.Next() {
		var it fleetFuel
		if err := rows.Scan(&it.ID, &it.Description, &it.Code, &it.FuelType, &it.Unit, &it.Active, &it.Notes); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		list = append(list, it)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list})
}

func (s *Server) createFleetFuel(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Description string  `json:"description"`
		Code        *string `json:"code"`
		FuelType    *string `json:"fuel_type"`
		Unit        string  `json:"unit"`
		Active      *bool   `json:"active"`
		Notes       *string `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Description) == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "description obrigatória", nil)
		return
	}
	if body.Unit == "" {
		body.Unit = "L"
	}
	active := true
	if body.Active != nil {
		active = *body.Active
	}
	var id uuid.UUID
	err := s.DB().QueryRow(r.Context(), `
		INSERT INTO fleet_fuels (description, code, fuel_type, unit, active, notes, created_by, updated_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$7) RETURNING id
	`, strings.TrimSpace(body.Description), ptrTrim(body.Code), ptrTrim(body.FuelType), body.Unit, active, ptrTrim(body.Notes), s.userIDFromRequest(r)).Scan(&id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (s *Server) patchFleetFuel(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "UUID inválido", nil)
		return
	}
	var body map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	var cur fleetFuel
	if err := s.DB().QueryRow(r.Context(), `SELECT id, description, code, fuel_type, unit, active, notes FROM fleet_fuels WHERE id=$1`, id).
		Scan(&cur.ID, &cur.Description, &cur.Code, &cur.FuelType, &cur.Unit, &cur.Active, &cur.Notes); err == pgx.ErrNoRows {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "combustível não encontrado", nil)
		return
	} else if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if v, ok := body["description"]; ok {
		_ = json.Unmarshal(v, &cur.Description)
	}
	if v, ok := body["code"]; ok {
		_ = json.Unmarshal(v, &cur.Code)
	}
	if v, ok := body["fuel_type"]; ok {
		_ = json.Unmarshal(v, &cur.FuelType)
	}
	if v, ok := body["unit"]; ok {
		_ = json.Unmarshal(v, &cur.Unit)
	}
	if v, ok := body["active"]; ok {
		_ = json.Unmarshal(v, &cur.Active)
	}
	if v, ok := body["notes"]; ok {
		_ = json.Unmarshal(v, &cur.Notes)
	}
	_, err = s.DB().Exec(r.Context(), `
		UPDATE fleet_fuels SET description=$2, code=$3, fuel_type=$4, unit=$5, active=$6, notes=$7, updated_at=now(), updated_by=$8 WHERE id=$1
	`, id, strings.TrimSpace(cur.Description), ptrTrim(cur.Code), ptrTrim(cur.FuelType), cur.Unit, cur.Active, ptrTrim(cur.Notes), s.userIDFromRequest(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// --- Stations ---

type fleetStation struct {
	ID           uuid.UUID  `json:"id"`
	Description  string     `json:"description"`
	LegalName    *string    `json:"legal_name"`
	TradeName    *string    `json:"trade_name"`
	CNPJ         *string    `json:"cnpj"`
	Phone        *string    `json:"phone"`
	Email        *string    `json:"email"`
	Zip          *string    `json:"zip"`
	Address      *string    `json:"address"`
	Number       *string    `json:"number"`
	Complement   *string    `json:"complement"`
	Neighborhood *string    `json:"neighborhood"`
	City         *string    `json:"city"`
	UF           *string    `json:"uf"`
	Latitude     *float64   `json:"latitude"`
	Longitude    *float64   `json:"longitude"`
	StationKind  string     `json:"station_kind"`
	Status       string     `json:"status"`
	Notes        *string    `json:"notes"`
	FuelIDs      []uuid.UUID `json:"fuel_ids,omitempty"`
}

func (s *Server) listFleetStations(w http.ResponseWriter, r *http.Request) {
	q := fleetQ(r)
	st := fleetStatusFilter(r)
	rows, err := s.DB().Query(r.Context(), `
		SELECT id, description, legal_name, trade_name, cnpj, phone, email, zip, address, number, complement,
			neighborhood, city, uf, latitude, longitude, station_kind, status, notes
		FROM fleet_stations
		WHERE ($1 = '' OR description ILIKE '%'||$1||'%' OR COALESCE(trade_name,'') ILIKE '%'||$1||'%' OR COALESCE(cnpj,'') ILIKE '%'||$1||'%')
		  AND ($2 = '' OR status = $2)
		ORDER BY description
	`, q, st)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	list := []fleetStation{}
	for rows.Next() {
		var it fleetStation
		if err := rows.Scan(&it.ID, &it.Description, &it.LegalName, &it.TradeName, &it.CNPJ, &it.Phone, &it.Email, &it.Zip,
			&it.Address, &it.Number, &it.Complement, &it.Neighborhood, &it.City, &it.UF, &it.Latitude, &it.Longitude,
			&it.StationKind, &it.Status, &it.Notes); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		list = append(list, it)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list})
}

func (s *Server) getFleetStation(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "UUID inválido", nil)
		return
	}
	var it fleetStation
	err = s.DB().QueryRow(r.Context(), `
		SELECT id, description, legal_name, trade_name, cnpj, phone, email, zip, address, number, complement,
			neighborhood, city, uf, latitude, longitude, station_kind, status, notes
		FROM fleet_stations WHERE id=$1
	`, id).Scan(&it.ID, &it.Description, &it.LegalName, &it.TradeName, &it.CNPJ, &it.Phone, &it.Email, &it.Zip,
		&it.Address, &it.Number, &it.Complement, &it.Neighborhood, &it.City, &it.UF, &it.Latitude, &it.Longitude,
		&it.StationKind, &it.Status, &it.Notes)
	if err == pgx.ErrNoRows {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "posto não encontrado", nil)
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	frows, err := s.DB().Query(r.Context(), `SELECT fuel_id FROM fleet_station_fuels WHERE station_id=$1`, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer frows.Close()
	it.FuelIDs = []uuid.UUID{}
	for frows.Next() {
		var fid uuid.UUID
		if err := frows.Scan(&fid); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		it.FuelIDs = append(it.FuelIDs, fid)
	}
	writeJSON(w, http.StatusOK, it)
}

func (s *Server) createFleetStation(w http.ResponseWriter, r *http.Request) {
	var body fleetStation
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Description) == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "description obrigatória", nil)
		return
	}
	if body.StationKind == "" {
		body.StationKind = "other"
	}
	if body.Status == "" {
		body.Status = "active"
	}
	uid := s.userIDFromRequest(r)
	tx, err := s.DB().Begin(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer tx.Rollback(r.Context())
	var id uuid.UUID
	err = tx.QueryRow(r.Context(), `
		INSERT INTO fleet_stations (
			description, legal_name, trade_name, cnpj, phone, email, zip, address, number, complement,
			neighborhood, city, uf, latitude, longitude, station_kind, status, notes, created_by, updated_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$19) RETURNING id
	`, strings.TrimSpace(body.Description), ptrTrim(body.LegalName), ptrTrim(body.TradeName), ptrTrim(body.CNPJ),
		ptrTrim(body.Phone), ptrTrim(body.Email), ptrTrim(body.Zip), ptrTrim(body.Address), ptrTrim(body.Number),
		ptrTrim(body.Complement), ptrTrim(body.Neighborhood), ptrTrim(body.City), ptrTrim(body.UF),
		body.Latitude, body.Longitude, body.StationKind, body.Status, ptrTrim(body.Notes), uid).Scan(&id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	for _, fid := range body.FuelIDs {
		if _, err := tx.Exec(r.Context(), `INSERT INTO fleet_station_fuels (station_id, fuel_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, id, fid); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (s *Server) patchFleetStation(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "UUID inválido", nil)
		return
	}
	var body fleetStation
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	tx, err := s.DB().Begin(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer tx.Rollback(r.Context())
	tag, err := tx.Exec(r.Context(), `
		UPDATE fleet_stations SET
			description=COALESCE(NULLIF(trim($2),''), description),
			legal_name=$3, trade_name=$4, cnpj=$5, phone=$6, email=$7, zip=$8, address=$9, number=$10, complement=$11,
			neighborhood=$12, city=$13, uf=$14, latitude=$15, longitude=$16,
			station_kind=COALESCE(NULLIF($17,''), station_kind),
			status=COALESCE(NULLIF($18,''), status),
			notes=$19, updated_at=now(), updated_by=$20
		WHERE id=$1
	`, id, body.Description, ptrTrim(body.LegalName), ptrTrim(body.TradeName), ptrTrim(body.CNPJ),
		ptrTrim(body.Phone), ptrTrim(body.Email), ptrTrim(body.Zip), ptrTrim(body.Address), ptrTrim(body.Number),
		ptrTrim(body.Complement), ptrTrim(body.Neighborhood), ptrTrim(body.City), ptrTrim(body.UF),
		body.Latitude, body.Longitude, body.StationKind, body.Status, ptrTrim(body.Notes), s.userIDFromRequest(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "posto não encontrado", nil)
		return
	}
	if body.FuelIDs != nil {
		if _, err := tx.Exec(r.Context(), `DELETE FROM fleet_station_fuels WHERE station_id=$1`, id); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		for _, fid := range body.FuelIDs {
			if _, err := tx.Exec(r.Context(), `INSERT INTO fleet_station_fuels (station_id, fuel_id) VALUES ($1,$2)`, id, fid); err != nil {
				writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
				return
			}
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func parseTimeFlexible(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, nil
	}
	layouts := []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02"}
	for _, l := range layouts {
		if t, err := time.ParseInLocation(l, s, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, strconv.ErrSyntax
}
