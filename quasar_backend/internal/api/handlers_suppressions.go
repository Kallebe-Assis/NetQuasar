package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

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
	s.appendAuditLog(r.Context(), "alert_suppression", id.String(), "create", s.actorFromRequest(r), nil, body)
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
	s.appendAuditLog(r.Context(), "alert_suppression", id.String(), "patch", s.actorFromRequest(r), nil, body)
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
	s.appendAuditLog(r.Context(), "alert_suppression", id.String(), "delete", s.actorFromRequest(r), nil, nil)
	w.WriteHeader(http.StatusNoContent)
}
