package api

import (
	"encoding/json"
	"io"
	"net/http"
)

// handlers_topology.go — documento único (singleton) do diagrama livre de topologia
// (menu Mapa → Topologia, ver topology_canvas em internal/db/migrations/122_topology_canvas.sql).
// O JSON gravado (nodes/edges/groups) é opaco para o backend — só o frontend interpreta a
// estrutura; aqui só se garante que é JSON válido antes de gravar.

func (s *Server) getTopologyCanvas(w http.ResponseWriter, r *http.Request) {
	var raw []byte
	err := s.DB().QueryRow(r.Context(), `SELECT canvas::text FROM topology_canvas WHERE id=1`).Scan(&raw)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw)
}

func (s *Server) putTopologyCanvas(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20)) // 8MB é generoso para um diagrama desenhado à mão
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_BODY", err.Error(), nil)
		return
	}
	if !json.Valid(body) {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "corpo não é JSON válido", nil)
		return
	}
	_, err = s.DB().Exec(r.Context(), `
		UPDATE topology_canvas SET canvas=$1::jsonb, updated_at=now() WHERE id=1
	`, body)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "topology_canvas", "1", "update", s.actorFromRequest(r), nil, nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
