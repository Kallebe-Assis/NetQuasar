package api

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// handlers_pop_rack.go — diagrama 2D da topologia do POP (rectângulo por porta de OLT/mikrotik/
// switch/DIO, ligações de fibra coloridas entre portas — ver pop_rack_diagrams,
// 139_pop_rack_diagram.sql). Mesmo padrão de documento opaco em JSONB que handlers_topology.go
// já usa para os projectos de Topologia geral: o backend só garante que é JSON válido, quem
// interpreta nodes/edges é o frontend (PopRackTopologyPage.tsx). Um diagrama por POP.

func (s *Server) getPopRackDiagram(w http.ResponseWriter, r *http.Request) {
	popID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	var exists bool
	if err := s.DB().QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM pops WHERE id=$1)`, popID).Scan(&exists); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if !exists {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "POP não encontrado", nil)
		return
	}
	var raw []byte
	err = s.DB().QueryRow(r.Context(), `
		INSERT INTO pop_rack_diagrams (pop_id) VALUES ($1)
		ON CONFLICT (pop_id) DO UPDATE SET pop_id = EXCLUDED.pop_id
		RETURNING canvas::text
	`, popID).Scan(&raw)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw)
}

func (s *Server) putPopRackDiagram(w http.ResponseWriter, r *http.Request) {
	popID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_BODY", err.Error(), nil)
		return
	}
	if !json.Valid(body) {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "corpo não é JSON válido", nil)
		return
	}
	var exists bool
	if err := s.DB().QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM pops WHERE id=$1)`, popID).Scan(&exists); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if !exists {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "POP não encontrado", nil)
		return
	}
	_, err = s.DB().Exec(r.Context(), `
		INSERT INTO pop_rack_diagrams (pop_id, canvas, updated_at) VALUES ($1, $2::jsonb, now())
		ON CONFLICT (pop_id) DO UPDATE SET canvas = EXCLUDED.canvas, updated_at = now()
	`, popID, body)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "pop_rack_diagram", popID.String(), "update", s.actorFromRequest(r), nil, nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
