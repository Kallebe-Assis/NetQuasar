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
	defaultMapCtoColor        = "#0d0663"
	defaultMapSpliceColor     = "#d97706"
	defaultMapEquipmentIcon   = "pin"
	defaultMapConnectionIcon  = "user"
	defaultMapCtoIcon         = "pin"
	defaultMapSpliceIcon      = "rocket"
)

var hexColorRe = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

var validMapIcons = map[string]map[string]bool{
	"equipment":  {"pin": true, "server": true, "radio": true, "chip": true, "building": true},
	"connection": {"user": true, "home": true, "wifi": true, "key": true, "signal": true},
	"cto":        {"pin": true, "cabinet": true, "hub": true, "drop": true, "ring": true},
	"splice":     {"rocket": true, "joint": true, "bolt": true, "diamond": true, "hex": true},
}

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
	MapCtoColor        string
	MapSpliceColor     string
	MapEquipmentIcon   string
	MapConnectionIcon  string
	MapCtoIcon         string
	MapSpliceIcon      string
	Updated            time.Time
}

func normalizeHexColor(v, fallback string) string {
	v = strings.TrimSpace(v)
	if hexColorRe.MatchString(v) {
		return strings.ToLower(v)
	}
	return fallback
}

func normalizeMapIcon(kind, v, fallback string) string {
	v = strings.TrimSpace(strings.ToLower(v))
	if allowed, ok := validMapIcons[kind]; ok && allowed[v] {
		return v
	}
	return fallback
}

func loadUIAppearance(ctx context.Context, pool *pgxpool.Pool) (uiAppearanceRow, error) {
	var row uiAppearanceRow
	row.Theme = uiThemeDark
	row.MapEquipmentColor = defaultMapEquipmentColor
	row.MapConnectionColor = defaultMapConnectionColor
	row.MapCtoColor = defaultMapCtoColor
	row.MapSpliceColor = defaultMapSpliceColor
	row.MapEquipmentIcon = defaultMapEquipmentIcon
	row.MapConnectionIcon = defaultMapConnectionIcon
	row.MapCtoIcon = defaultMapCtoIcon
	row.MapSpliceIcon = defaultMapSpliceIcon
	err := pool.QueryRow(ctx, `
		SELECT theme, map_equipment_color, map_connection_color,
			map_cto_color, map_splice_color,
			map_equipment_icon, map_connection_icon, map_cto_icon, map_splice_icon,
			updated_at
		FROM settings_ui WHERE id = 1
	`).Scan(
		&row.Theme, &row.MapEquipmentColor, &row.MapConnectionColor,
		&row.MapCtoColor, &row.MapSpliceColor,
		&row.MapEquipmentIcon, &row.MapConnectionIcon, &row.MapCtoIcon, &row.MapSpliceIcon,
		&row.Updated,
	)
	if err != nil {
		return uiAppearanceRow{
			Theme: uiThemeDark, MapEquipmentColor: defaultMapEquipmentColor, MapConnectionColor: defaultMapConnectionColor,
			MapCtoColor: defaultMapCtoColor, MapSpliceColor: defaultMapSpliceColor,
			MapEquipmentIcon: defaultMapEquipmentIcon, MapConnectionIcon: defaultMapConnectionIcon,
			MapCtoIcon: defaultMapCtoIcon, MapSpliceIcon: defaultMapSpliceIcon,
		}, err
	}
	if t, ok := normalizeUITheme(row.Theme); ok {
		row.Theme = t
	}
	row.MapEquipmentColor = normalizeHexColor(row.MapEquipmentColor, defaultMapEquipmentColor)
	row.MapConnectionColor = normalizeHexColor(row.MapConnectionColor, defaultMapConnectionColor)
	row.MapCtoColor = normalizeHexColor(row.MapCtoColor, defaultMapCtoColor)
	row.MapSpliceColor = normalizeHexColor(row.MapSpliceColor, defaultMapSpliceColor)
	row.MapEquipmentIcon = normalizeMapIcon("equipment", row.MapEquipmentIcon, defaultMapEquipmentIcon)
	row.MapConnectionIcon = normalizeMapIcon("connection", row.MapConnectionIcon, defaultMapConnectionIcon)
	row.MapCtoIcon = normalizeMapIcon("cto", row.MapCtoIcon, defaultMapCtoIcon)
	row.MapSpliceIcon = normalizeMapIcon("splice", row.MapSpliceIcon, defaultMapSpliceIcon)
	return row, nil
}

func uiAppearanceJSON(row uiAppearanceRow, source string) map[string]any {
	out := map[string]any{
		"theme":                 row.Theme,
		"map_equipment_color":   row.MapEquipmentColor,
		"map_connection_color":  row.MapConnectionColor,
		"map_cto_color":         row.MapCtoColor,
		"map_splice_color":      row.MapSpliceColor,
		"map_equipment_icon":    row.MapEquipmentIcon,
		"map_connection_icon":   row.MapConnectionIcon,
		"map_cto_icon":          row.MapCtoIcon,
		"map_splice_icon":       row.MapSpliceIcon,
		"updated_at":            row.Updated,
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
			MapCtoColor:        defaultMapCtoColor,
			MapSpliceColor:     defaultMapSpliceColor,
			MapEquipmentIcon:   defaultMapEquipmentIcon,
			MapConnectionIcon:  defaultMapConnectionIcon,
			MapCtoIcon:         defaultMapCtoIcon,
			MapSpliceIcon:      defaultMapSpliceIcon,
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
		MapCtoColor        *string `json:"map_cto_color"`
		MapSpliceColor     *string `json:"map_splice_color"`
		MapEquipmentIcon   *string `json:"map_equipment_icon"`
		MapConnectionIcon  *string `json:"map_connection_icon"`
		MapCtoIcon         *string `json:"map_cto_icon"`
		MapSpliceIcon      *string `json:"map_splice_icon"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	if body.Theme == nil && body.MapEquipmentColor == nil && body.MapConnectionColor == nil &&
		body.MapCtoColor == nil && body.MapSpliceColor == nil &&
		body.MapEquipmentIcon == nil && body.MapConnectionIcon == nil &&
		body.MapCtoIcon == nil && body.MapSpliceIcon == nil {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "informe theme, cores e/ou ícones do mapa", nil)
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
	setColor := func(raw *string, dest *string, field, fallback string) bool {
		if raw == nil {
			return true
		}
		if !hexColorRe.MatchString(strings.TrimSpace(*raw)) {
			writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", field+" deve ser #RRGGBB", nil)
			return false
		}
		*dest = normalizeHexColor(*raw, fallback)
		audit[field] = *dest
		return true
	}
	if !setColor(body.MapEquipmentColor, &cur.MapEquipmentColor, "map_equipment_color", defaultMapEquipmentColor) {
		return
	}
	if !setColor(body.MapConnectionColor, &cur.MapConnectionColor, "map_connection_color", defaultMapConnectionColor) {
		return
	}
	if !setColor(body.MapCtoColor, &cur.MapCtoColor, "map_cto_color", defaultMapCtoColor) {
		return
	}
	if !setColor(body.MapSpliceColor, &cur.MapSpliceColor, "map_splice_color", defaultMapSpliceColor) {
		return
	}
	setIcon := func(raw *string, dest *string, kind, field, fallback string) bool {
		if raw == nil {
			return true
		}
		next := normalizeMapIcon(kind, *raw, "")
		if next == "" {
			writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", field+" inválido", nil)
			return false
		}
		*dest = next
		audit[field] = next
		return true
	}
	if !setIcon(body.MapEquipmentIcon, &cur.MapEquipmentIcon, "equipment", "map_equipment_icon", defaultMapEquipmentIcon) {
		return
	}
	if !setIcon(body.MapConnectionIcon, &cur.MapConnectionIcon, "connection", "map_connection_icon", defaultMapConnectionIcon) {
		return
	}
	if !setIcon(body.MapCtoIcon, &cur.MapCtoIcon, "cto", "map_cto_icon", defaultMapCtoIcon) {
		return
	}
	if !setIcon(body.MapSpliceIcon, &cur.MapSpliceIcon, "splice", "map_splice_icon", defaultMapSpliceIcon) {
		return
	}
	err = p.QueryRow(r.Context(), `
		UPDATE settings_ui
		SET theme = $1,
			map_equipment_color = $2, map_connection_color = $3,
			map_cto_color = $4, map_splice_color = $5,
			map_equipment_icon = $6, map_connection_icon = $7,
			map_cto_icon = $8, map_splice_icon = $9,
			updated_at = now()
		WHERE id = 1
		RETURNING updated_at
	`, cur.Theme, cur.MapEquipmentColor, cur.MapConnectionColor, cur.MapCtoColor, cur.MapSpliceColor,
		cur.MapEquipmentIcon, cur.MapConnectionIcon, cur.MapCtoIcon, cur.MapSpliceIcon,
	).Scan(&cur.Updated)
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
		"map_cto_color":        cur.MapCtoColor,
		"map_splice_color":     cur.MapSpliceColor,
		"map_equipment_icon":   cur.MapEquipmentIcon,
		"map_connection_icon":  cur.MapConnectionIcon,
		"map_cto_icon":         cur.MapCtoIcon,
		"map_splice_icon":      cur.MapSpliceIcon,
		"updated_at":           cur.Updated,
	})
}
