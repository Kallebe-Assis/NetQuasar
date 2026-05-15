package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	uiThemeDark  = "dark"
	uiThemeLight = "light"
)

func normalizeUITheme(v string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case uiThemeDark, "":
		return uiThemeDark, true
	case uiThemeLight:
		return uiThemeLight, true
	default:
		return "", false
	}
}

func loadUITheme(ctx context.Context, pool *pgxpool.Pool) (theme string, updated time.Time, err error) {
	theme = uiThemeDark
	err = pool.QueryRow(ctx, `SELECT theme, updated_at FROM settings_ui WHERE id = 1`).Scan(&theme, &updated)
	if err != nil {
		return uiThemeDark, time.Time{}, err
	}
	if t, ok := normalizeUITheme(theme); ok {
		theme = t
	}
	return theme, updated, nil
}

func (s *Server) getUIAppearance(w http.ResponseWriter, r *http.Request) {
	p := s.DB()
	if p == nil {
		writeJSON(w, http.StatusOK, map[string]any{"theme": uiThemeDark, "source": "default_no_db"})
		return
	}
	theme, updated, err := loadUITheme(r.Context(), p)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"theme":      theme,
		"updated_at": updated,
	})
}

func (s *Server) patchUIAppearance(w http.ResponseWriter, r *http.Request) {
	p := s.DB()
	if p == nil {
		writeErr(w, http.StatusServiceUnavailable, "NO_DB", "base de dados não configurada", nil)
		return
	}
	var body struct {
		Theme *string `json:"theme"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	if body.Theme == nil {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "informe theme (dark ou light)", nil)
		return
	}
	theme, ok := normalizeUITheme(*body.Theme)
	if !ok {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "theme deve ser dark ou light", map[string]any{"theme": *body.Theme})
		return
	}
	var updated time.Time
	err := p.QueryRow(r.Context(), `
		UPDATE settings_ui SET theme = $1, updated_at = now() WHERE id = 1
		RETURNING updated_at
	`, theme).Scan(&updated)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "theme": theme, "updated_at": updated})
}
