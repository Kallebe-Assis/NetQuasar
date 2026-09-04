package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// handlers_topology.go — N projectos nomeados de diagrama livre de topologia (menu Mapa →
// Topologia), ver topology_projects em internal/db/migrations/132_topology_projects.sql
// (substituiu o documento único "topology_canvas" que existia antes). O JSON de cada projecto
// (nodes/edges/groups/settings) é opaco para o backend — só o frontend interpreta a estrutura;
// aqui só se garante que é JSON válido antes de gravar.

type topologyProjectSummary struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (s *Server) listTopologyProjects(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB().Query(r.Context(), `SELECT id, name, updated_at FROM topology_projects ORDER BY name`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	list := []topologyProjectSummary{}
	for rows.Next() {
		var it topologyProjectSummary
		if err := rows.Scan(&it.ID, &it.Name, &it.UpdatedAt); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		list = append(list, it)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list})
}

func (s *Server) createTopologyProject(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "nome obrigatório", nil)
		return
	}
	var id uuid.UUID
	err := s.DB().QueryRow(r.Context(), `
		INSERT INTO topology_projects (name) VALUES ($1) RETURNING id
	`, name).Scan(&id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "topology_project", id.String(), "create", s.actorFromRequest(r), nil, map[string]any{"name": name})
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "name": name})
}

func (s *Server) renameTopologyProject(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "nome obrigatório", nil)
		return
	}
	tag, err := s.DB().Exec(r.Context(), `UPDATE topology_projects SET name=$2, updated_at=now() WHERE id=$1`, id, name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "", nil)
		return
	}
	s.appendAuditLog(r.Context(), "topology_project", id.String(), "rename", s.actorFromRequest(r), nil, map[string]any{"name": name})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) deleteTopologyProject(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	var total int
	if err := s.DB().QueryRow(r.Context(), `SELECT count(*) FROM topology_projects`).Scan(&total); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if total <= 1 {
		writeErr(w, http.StatusConflict, "LAST_PROJECT", "não é possível remover o último projecto de topologia — crie outro antes de remover este", nil)
		return
	}
	tag, err := s.DB().Exec(r.Context(), `DELETE FROM topology_projects WHERE id=$1`, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "", nil)
		return
	}
	s.appendAuditLog(r.Context(), "topology_project", id.String(), "delete", s.actorFromRequest(r), nil, nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) getTopologyProjectCanvas(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	var raw []byte
	err = s.DB().QueryRow(r.Context(), `SELECT canvas::text FROM topology_projects WHERE id=$1`, id).Scan(&raw)
	if err == pgx.ErrNoRows {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "", nil)
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw)
}

func (s *Server) putTopologyProjectCanvas(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20)) // 8MB é generoso para um diagrama desenhado à mão
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_BODY", err.Error(), nil)
		return
	}
	if !json.Valid(body) {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "corpo não é JSON válido", nil)
		return
	}
	tag, err := s.DB().Exec(r.Context(), `
		UPDATE topology_projects SET canvas=$2::jsonb, updated_at=now() WHERE id=$1
	`, id, body)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "", nil)
		return
	}
	s.appendAuditLog(r.Context(), "topology_project", id.String(), "update", s.actorFromRequest(r), nil, nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
