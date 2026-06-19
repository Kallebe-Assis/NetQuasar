package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type popRow struct {
	ID          uuid.UUID `json:"id"`
	Description string    `json:"description"`
	Address     *string   `json:"address"`
	Latitude    *float64  `json:"latitude"`
	Longitude   *float64  `json:"longitude"`
	DeviceCount int64     `json:"device_count"`
}

func (s *Server) listPops(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB().Query(r.Context(), `
		SELECT p.id, p.description, p.address, p.latitude, p.longitude,
			(SELECT COUNT(*) FROM devices d WHERE d.pop_id = p.id)
		FROM pops p ORDER BY p.description
	`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	var list []popRow
	for rows.Next() {
		var pr popRow
		if err := rows.Scan(&pr.ID, &pr.Description, &pr.Address, &pr.Latitude, &pr.Longitude, &pr.DeviceCount); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		list = append(list, pr)
	}
	writeJSON(w, http.StatusOK, map[string]any{"pops": list})
}

func (s *Server) createPop(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Description string   `json:"description"`
		Address     *string  `json:"address"`
		Latitude    *float64 `json:"latitude"`
		Longitude   *float64 `json:"longitude"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Description == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "description obrigatória", nil)
		return
	}
	var id uuid.UUID
	err := s.DB().QueryRow(r.Context(), `
		INSERT INTO pops (description, address, latitude, longitude) VALUES ($1,$2,$3,$4) RETURNING id
	`, body.Description, body.Address, body.Latitude, body.Longitude).Scan(&id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "pop", id.String(), "create", s.actorFromRequest(r), nil, body)
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (s *Server) getPop(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "UUID inválido", nil)
		return
	}
	var pr popRow
	err = s.DB().QueryRow(r.Context(), `
		SELECT p.id, p.description, p.address, p.latitude, p.longitude,
			(SELECT COUNT(*) FROM devices d WHERE d.pop_id = p.id)
		FROM pops p WHERE p.id=$1
	`, id).Scan(&pr.ID, &pr.Description, &pr.Address, &pr.Latitude, &pr.Longitude, &pr.DeviceCount)
	if err == pgx.ErrNoRows {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "POP não encontrado", nil)
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, pr)
}

func (s *Server) patchPop(w http.ResponseWriter, r *http.Request) {
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
	var desc string
	var addr *string
	var lat, lon *float64
	if err := s.DB().QueryRow(r.Context(), `SELECT description, address, latitude, longitude FROM pops WHERE id=$1`, id).Scan(&desc, &addr, &lat, &lon); err == pgx.ErrNoRows {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "POP não encontrado", nil)
		return
	} else if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if v, ok := body["description"]; ok {
		_ = json.Unmarshal(v, &desc)
	}
	if v, ok := body["address"]; ok {
		_ = json.Unmarshal(v, &addr)
	}
	if v, ok := body["latitude"]; ok {
		_ = json.Unmarshal(v, &lat)
	}
	if v, ok := body["longitude"]; ok {
		_ = json.Unmarshal(v, &lon)
	}
	before := map[string]any{"description": desc, "address": addr, "latitude": lat, "longitude": lon}
	_, err = s.DB().Exec(r.Context(), `UPDATE pops SET description=$2, address=$3, latitude=$4, longitude=$5, updated_at=now() WHERE id=$1`, id, desc, addr, lat, lon)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	after := map[string]any{"description": desc, "address": addr, "latitude": lat, "longitude": lon}
	s.appendAuditLog(r.Context(), "pop", id.String(), "patch", s.actorFromRequest(r), before, after)
	s.getPop(w, r)
}

func (s *Server) deletePop(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "UUID inválido", nil)
		return
	}
	ct, err := s.DB().Exec(r.Context(), `DELETE FROM pops WHERE id=$1`, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if ct.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "POP não encontrado", nil)
		return
	}
	s.appendAuditLog(r.Context(), "pop", id.String(), "delete", s.actorFromRequest(r), nil, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) bulkAttachDevices(w http.ResponseWriter, r *http.Request) {
	popID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "UUID inválido", nil)
		return
	}
	var body struct {
		DeviceIDs []uuid.UUID `json:"device_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.DeviceIDs) == 0 {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "device_ids não vazio", nil)
		return
	}
	tx, err := s.DB().Begin(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	for _, did := range body.DeviceIDs {
		if _, err := tx.Exec(r.Context(), `UPDATE devices SET pop_id=$1, updated_at=now() WHERE id=$2`, popID, did); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"updated": len(body.DeviceIDs)})
}
