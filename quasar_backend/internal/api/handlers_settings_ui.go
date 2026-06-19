package api

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	uiThemeDark               = "dark"
	uiThemeLight              = "light"
	defaultMapEquipmentColor  = "#3388ff"
	defaultMapConnectionColor = "#3b82f6"
)

var hexColorRe = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

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

type uiAppearanceRow struct {
	Theme              string
	MapEquipmentColor  string
	MapConnectionColor string
	Updated            time.Time
}

func normalizeHexColor(v, fallback string) string {
	v = strings.TrimSpace(v)
	if hexColorRe.MatchString(v) {
		return strings.ToLower(v)
	}
	return fallback
}

func loadUIAppearance(ctx context.Context, pool *pgxpool.Pool) (uiAppearanceRow, error) {
	var row uiAppearanceRow
	row.Theme = uiThemeDark
	row.MapEquipmentColor = defaultMapEquipmentColor
	row.MapConnectionColor = defaultMapConnectionColor
	err := pool.QueryRow(ctx, `
		SELECT theme, map_equipment_color, map_connection_color, updated_at
		FROM settings_ui WHERE id = 1
	`).Scan(&row.Theme, &row.MapEquipmentColor, &row.MapConnectionColor, &row.Updated)
	if err != nil {
		return uiAppearanceRow{Theme: uiThemeDark, MapEquipmentColor: defaultMapEquipmentColor, MapConnectionColor: defaultMapConnectionColor}, err
	}
	if t, ok := normalizeUITheme(row.Theme); ok {
		row.Theme = t
	}
	row.MapEquipmentColor = normalizeHexColor(row.MapEquipmentColor, defaultMapEquipmentColor)
	row.MapConnectionColor = normalizeHexColor(row.MapConnectionColor, defaultMapConnectionColor)
	return row, nil
}

func uiAppearanceJSON(row uiAppearanceRow, source string) map[string]any {
	out := map[string]any{
		"theme":                row.Theme,
		"map_equipment_color":  row.MapEquipmentColor,
		"map_connection_color": row.MapConnectionColor,
		"updated_at":           row.Updated,
	}
	if source != "" {
		out["source"] = source
	}
	return out
}

func (s *Server) getUIAppearance(w http.ResponseWriter, r *http.Request) {
	p := s.DB()
	if p == nil {
		writeJSON(w, http.StatusOK, uiAppearanceJSON(uiAppearanceRow{
			Theme:              uiThemeDark,
			MapEquipmentColor:  defaultMapEquipmentColor,
			MapConnectionColor: defaultMapConnectionColor,
		}, "default_no_db"))
		return
	}
	row, err := loadUIAppearance(r.Context(), p)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, uiAppearanceJSON(row, ""))
}

func (s *Server) patchUIAppearance(w http.ResponseWriter, r *http.Request) {
	p := s.DB()
	if p == nil {
		writeErr(w, http.StatusServiceUnavailable, "NO_DB", "base de dados não configurada", nil)
		return
	}
	var body struct {
		Theme              *string `json:"theme"`
		MapEquipmentColor  *string `json:"map_equipment_color"`
		MapConnectionColor *string `json:"map_connection_color"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	if body.Theme == nil && body.MapEquipmentColor == nil && body.MapConnectionColor == nil {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "informe theme e/ou cores do mapa", nil)
		return
	}
	cur, err := loadUIAppearance(r.Context(), p)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	audit := map[string]any{}
	if body.Theme != nil {
		theme, ok := normalizeUITheme(*body.Theme)
		if !ok {
			writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "theme deve ser dark ou light", map[string]any{"theme": *body.Theme})
			return
		}
		cur.Theme = theme
		audit["theme"] = theme
	}
	if body.MapEquipmentColor != nil {
		if !hexColorRe.MatchString(strings.TrimSpace(*body.MapEquipmentColor)) {
			writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "map_equipment_color deve ser #RRGGBB", nil)
			return
		}
		cur.MapEquipmentColor = normalizeHexColor(*body.MapEquipmentColor, defaultMapEquipmentColor)
		audit["map_equipment_color"] = cur.MapEquipmentColor
	}
	if body.MapConnectionColor != nil {
		if !hexColorRe.MatchString(strings.TrimSpace(*body.MapConnectionColor)) {
			writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "map_connection_color deve ser #RRGGBB", nil)
			return
		}
		cur.MapConnectionColor = normalizeHexColor(*body.MapConnectionColor, defaultMapConnectionColor)
		audit["map_connection_color"] = cur.MapConnectionColor
	}
	err = p.QueryRow(r.Context(), `
		UPDATE settings_ui
		SET theme = $1, map_equipment_color = $2, map_connection_color = $3, updated_at = now()
		WHERE id = 1
		RETURNING updated_at
	`, cur.Theme, cur.MapEquipmentColor, cur.MapConnectionColor).Scan(&cur.Updated)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "settings_ui", "1", "patch", s.actorFromRequest(r), nil, audit)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":                   true,
		"theme":                cur.Theme,
		"map_equipment_color":  cur.MapEquipmentColor,
		"map_connection_color": cur.MapConnectionColor,
		"updated_at":           cur.Updated,
	})
}
