package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// getOLTOnuHistory devolve as últimas colectas de UMA ONU (pon+onu) — botão "Histórico" (3
// pontinhos) na aba de ONUs. "row" é o mesmo objecto já usado pela tabela ao vivo (ver
// internal/oltsamples/oltsamples.go, RecordOnuHistory), gravado a cada colecta e podado para as
// últimas 10 por ONU.
func (s *Server) getOLTOnuHistory(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "identificador inválido", nil)
		return
	}
	pon, errPon := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("pon")))
	onu, errOnu := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("onu")))
	if errPon != nil || errOnu != nil || pon < 1 || onu < 1 {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "parâmetros pon/onu obrigatórios (inteiros ≥ 1)", nil)
		return
	}
	rows, err := s.DB().Query(r.Context(), `
		SELECT collected_at, row::text
		FROM olt_onu_history
		WHERE device_id = $1 AND pon = $2 AND onu = $3
		ORDER BY collected_at DESC
		LIMIT 10
	`, id, pon, onu)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var collectedAt any
		var raw []byte
		if err := rows.Scan(&collectedAt, &raw); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		var row map[string]any
		_ = json.Unmarshal(raw, &row)
		out = append(out, map[string]any{"collected_at": collectedAt, "row": row})
	}
	if out == nil {
		out = []map[string]any{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"pon": pon, "onu": onu, "history": out})
}
