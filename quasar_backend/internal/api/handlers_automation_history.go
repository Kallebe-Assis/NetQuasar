package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) getAutomationExecutionHistory(w http.ResponseWriter, r *http.Request) {
	pool := s.DB()
	if pool == nil {
		writeErr(w, http.StatusServiceUnavailable, "DB", "base indisponível", nil)
		return
	}
	q := r.URL.Query()
	jobType := q.Get("job_type")
	search := q.Get("q")
	limit := 200
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			if n > 500 {
				n = 500
			}
			limit = n
		}
	}
	var from, to *time.Time
	if v := q.Get("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			from = &t
		}
	}
	if v := q.Get("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			to = &t
		}
	}
	items, err := s.listAutomationExecutionHistory(r.Context(), pool, jobType, search, from, to, limit)
	if err != nil {
		msg := err.Error()
		if strings.Contains(strings.ToLower(msg), "automation_execution_log") && strings.Contains(strings.ToLower(msg), "does not exist") {
			msg = "tabela automation_execution_log inexistente — reinicie o backend após migração 035 ou execute db/migrate"
		}
		writeErr(w, http.StatusInternalServerError, "DB", msg, nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
