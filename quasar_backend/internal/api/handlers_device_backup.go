package api

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s *Server) getDeviceConfigBackup(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	var content string
	var updatedAt time.Time
	err = s.DB().QueryRow(r.Context(), `
		SELECT content, updated_at FROM device_config_backups WHERE device_id = $1
	`, id).Scan(&content, &updatedAt)
	if err == pgx.ErrNoRows {
		writeJSON(w, http.StatusOK, map[string]any{
			"device_id":  id,
			"content":    "",
			"updated_at": nil,
		})
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"device_id":  id,
		"content":    content,
		"updated_at": updatedAt,
	})
}

func (s *Server) putDeviceConfigBackup(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	var body struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	var exists bool
	if err := s.DB().QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM devices WHERE id=$1)`, id).Scan(&exists); err != nil || !exists {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "equipamento não encontrado", nil)
		return
	}
	_, err = s.DB().Exec(r.Context(), `
		INSERT INTO device_config_backups (device_id, content, updated_at)
		VALUES ($1, $2, now())
		ON CONFLICT (device_id) DO UPDATE SET content = EXCLUDED.content, updated_at = now()
	`, id, body.Content)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "device_config_backup", id.String(), "put", actorFromRequest(r), nil, map[string]any{
		"bytes": len(body.Content),
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "device_id": id})
}

func (s *Server) exportDeviceConfigBackup(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	var desc, ip string
	err = s.DB().QueryRow(r.Context(), `
		SELECT COALESCE(TRIM(description), ''), COALESCE(host(ip)::text, '')
		FROM devices WHERE id = $1
	`, id).Scan(&desc, &ip)
	if err != nil {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "", nil)
		return
	}
	var content string
	var updatedAt *time.Time
	_ = s.DB().QueryRow(r.Context(), `
		SELECT content, updated_at FROM device_config_backups WHERE device_id = $1
	`, id).Scan(&content, &updatedAt)

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	safe := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, desc)
	if safe == "" {
		safe = id.String()[:8]
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="backup_%s.csv"`, safe))

	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"device_id", "description", "ip", "updated_at", "config_backup"})
	updated := ""
	if updatedAt != nil {
		updated = updatedAt.Format(time.RFC3339)
	}
	_ = cw.Write([]string{id.String(), desc, ip, updated, content})
	cw.Flush()
}
