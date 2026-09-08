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
	uiThemeDark                       = "dark"
	uiThemeLight                      = "light"
	defaultMapEquipmentColor          = "#3388ff"
	defaultMapConnectionColor         = "#3b82f6"
	defaultMapCtoColor                = "#0d0663"
	defaultMapSpliceColor             = "#d97706"
	defaultMapEquipmentIcon           = "pin"
	defaultMapConnectionIcon          = "user"
	defaultMapCtoIcon                 = "pin"
	defaultMapSpliceIcon              = "rocket"
	defaultMapSpliceEmendaColor       = "#d97706"
	defaultMapSpliceDistribuicaoColor = "#7c3aed"
	defaultMapSpliceEmendaIcon        = "joint"
	defaultMapSpliceDistribuicaoIcon  = "rocket"
)

// Cor padrão por função de cabo quando settings_ui.map_cable_funcao_colors não tem a chave —
// mesma lista de funcao em handlers_network_infrastructure.go (networkCableFuncoes).
var defaultMapCableFuncaoColors = map[string]string{
	"backbone_link": "#0891b2",
	"transporte":    "#16a34a",
	"backbone_ftth": "#2563eb",
	"cto":           "#eab308",
	"multipla":      "#dc2626",
	"outro":         "#64748b",
}

// Mesmas 9 categorias de DevicesPage.tsx / MapFilterModal.tsx (MAP_DEVICE_CATEGORIES).
var mapEquipmentCategories = map[string]bool{
	"Concentrador": true, "Energia": true, "Mikrotik": true, "Switch": true, "OLT": true,
	"Rádio": true, "Servidor": true, "Máquina Virtual": true, "Outros": true,
}

// Chaves aceites em settings_ui.map_icon_image_urls — as mesmas 5 "funções" de ícone que já têm
// cor/estilo próprios (equipment é o padrão geral, categorias individuais ficam em
// map_equipment_categories).
var mapIconImageURLRoles = map[string]bool{
	"equipment": true, "connection": true, "cto": true, "splice_emenda": true, "splice_distribuicao": true,
}

// Ícone/imagem/visibilidade de uma categoria de equipamento no mapa — chave ausente do mapa usa
// o padrão (map_equipment_icon, visível). Campos vazios/false equivalem a "sem override".
type equipmentCategoryConfig struct {
	Icon     string `json:"icon,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
	Hidden   bool   `json:"hidden,omitempty"`
}

// Ponteiros nulos = campo não incluído no patch (mantém o valor já gravado dessa categoria).
type equipmentCategoryPatch struct {
	Icon     *string `json:"icon"`
	ImageURL *string `json:"image_url"`
	Hidden   *bool   `json:"hidden"`
}

var hexColorRe = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

var validMapIcons = map[string]map[string]bool{
	"equipment": {
		"pin": true, "server": true, "radio": true, "chip": true, "building": true,
		"antenna": true, "router": true, "battery": true, "box": true, "tower": true,
	},
	"connection": {
		"user": true, "home": true, "wifi": true, "key": true, "signal": true,
		"smartphone": true, "laptop": true, "star": true, "flag": true, "tag": true,
	},
	"cto": {
		"pin": true, "cabinet": true, "hub": true, "drop": true, "ring": true,
		"layers": true, "target": true, "square": true, "shield": true, "bookmark": true,
	},
	"splice": {
		"rocket": true, "joint": true, "bolt": true, "diamond": true, "hex": true,
		"flame": true, "link": true, "circle-dot": true, "anchor": true, "triangle": true,
	},
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
	Theme                      string
	MapEquipmentColor          string
	MapConnectionColor         string
	MapCtoColor                string
	MapSpliceColor             string
	MapEquipmentIcon           string
	MapConnectionIcon          string
	MapCtoIcon                 string
	MapSpliceIcon              string
	MapSpliceEmendaColor       string
	MapSpliceDistribuicaoColor string
	MapSpliceEmendaIcon        string
	MapSpliceDistribuicaoIcon  string
	MapCableFuncaoColors       map[string]string
	MapEquipmentCategories     map[string]equipmentCategoryConfig
	MapIconImageURLs           map[string]string
	Updated                    time.Time
}

// mergeCableFuncaoColors preenche as funções sem cor personalizada com o padrão — o frontend
// recebe sempre um mapa completo (6 chaves), nunca precisa de tratar chave ausente.
func mergeCableFuncaoColors(stored map[string]string) map[string]string {
	out := make(map[string]string, len(defaultMapCableFuncaoColors))
	for k, v := range defaultMapCableFuncaoColors {
		out[k] = v
	}
	for k, v := range stored {
		if _, known := defaultMapCableFuncaoColors[k]; !known {
			continue
		}
		if hexColorRe.MatchString(strings.TrimSpace(v)) {
			out[k] = strings.ToLower(strings.TrimSpace(v))
		}
	}
	return out
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
	row := defaultUIAppearanceRow()
	var funcaoColorsRaw, equipmentCategoriesRaw, iconImageURLsRaw []byte
	err := pool.QueryRow(ctx, `
		SELECT theme, map_equipment_color, map_connection_color,
			map_cto_color, map_splice_color,
			map_equipment_icon, map_connection_icon, map_cto_icon, map_splice_icon,
			map_splice_emenda_color, map_splice_distribuicao_color,
			map_splice_emenda_icon, map_splice_distribuicao_icon,
			map_cable_funcao_colors, map_equipment_categories, map_icon_image_urls,
			updated_at
		FROM settings_ui WHERE id = 1
	`).Scan(
		&row.Theme, &row.MapEquipmentColor, &row.MapConnectionColor,
		&row.MapCtoColor, &row.MapSpliceColor,
		&row.MapEquipmentIcon, &row.MapConnectionIcon, &row.MapCtoIcon, &row.MapSpliceIcon,
		&row.MapSpliceEmendaColor, &row.MapSpliceDistribuicaoColor,
		&row.MapSpliceEmendaIcon, &row.MapSpliceDistribuicaoIcon,
		&funcaoColorsRaw, &equipmentCategoriesRaw, &iconImageURLsRaw,
		&row.Updated,
	)
	if err != nil {
		return defaultUIAppearanceRow(), err
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
	row.MapSpliceEmendaColor = normalizeHexColor(row.MapSpliceEmendaColor, defaultMapSpliceEmendaColor)
	row.MapSpliceDistribuicaoColor = normalizeHexColor(row.MapSpliceDistribuicaoColor, defaultMapSpliceDistribuicaoColor)
	row.MapSpliceEmendaIcon = normalizeMapIcon("splice", row.MapSpliceEmendaIcon, defaultMapSpliceEmendaIcon)
	row.MapSpliceDistribuicaoIcon = normalizeMapIcon("splice", row.MapSpliceDistribuicaoIcon, defaultMapSpliceDistribuicaoIcon)
	var storedFuncaoColors map[string]string
	if len(funcaoColorsRaw) > 0 {
		_ = json.Unmarshal(funcaoColorsRaw, &storedFuncaoColors)
	}
	row.MapCableFuncaoColors = mergeCableFuncaoColors(storedFuncaoColors)

	var storedCategories map[string]equipmentCategoryConfig
	if len(equipmentCategoriesRaw) > 0 {
		_ = json.Unmarshal(equipmentCategoriesRaw, &storedCategories)
	}
	row.MapEquipmentCategories = map[string]equipmentCategoryConfig{}
	for cat, cfg := range storedCategories {
		if !mapEquipmentCategories[cat] {
			continue
		}
		if cfg.Icon != "" {
			cfg.Icon = normalizeMapIcon("equipment", cfg.Icon, "")
		}
		if cfg.ImageURL != "" && !looksLikeHTTPURL(cfg.ImageURL) {
			cfg.ImageURL = ""
		}
		row.MapEquipmentCategories[cat] = cfg
	}

	var storedIconURLs map[string]string
	if len(iconImageURLsRaw) > 0 {
		_ = json.Unmarshal(iconImageURLsRaw, &storedIconURLs)
	}
	row.MapIconImageURLs = map[string]string{}
	for role, u := range storedIconURLs {
		if !mapIconImageURLRoles[role] {
			continue
		}
		if looksLikeHTTPURL(u) {
			row.MapIconImageURLs[role] = u
		}
	}
	return row, nil
}

func defaultUIAppearanceRow() uiAppearanceRow {
	return uiAppearanceRow{
		Theme: uiThemeDark, MapEquipmentColor: defaultMapEquipmentColor, MapConnectionColor: defaultMapConnectionColor,
		MapCtoColor: defaultMapCtoColor, MapSpliceColor: defaultMapSpliceColor,
		MapEquipmentIcon: defaultMapEquipmentIcon, MapConnectionIcon: defaultMapConnectionIcon,
		MapCtoIcon: defaultMapCtoIcon, MapSpliceIcon: defaultMapSpliceIcon,
		MapSpliceEmendaColor: defaultMapSpliceEmendaColor, MapSpliceDistribuicaoColor: defaultMapSpliceDistribuicaoColor,
		MapSpliceEmendaIcon: defaultMapSpliceEmendaIcon, MapSpliceDistribuicaoIcon: defaultMapSpliceDistribuicaoIcon,
		MapCableFuncaoColors:   mergeCableFuncaoColors(nil),
		MapEquipmentCategories: map[string]equipmentCategoryConfig{},
		MapIconImageURLs:       map[string]string{},
	}
}

func uiAppearanceJSON(row uiAppearanceRow, source string) map[string]any {
	out := map[string]any{
		"theme":                         row.Theme,
		"map_equipment_color":           row.MapEquipmentColor,
		"map_connection_color":          row.MapConnectionColor,
		"map_cto_color":                 row.MapCtoColor,
		"map_splice_color":              row.MapSpliceColor,
		"map_equipment_icon":            row.MapEquipmentIcon,
		"map_connection_icon":           row.MapConnectionIcon,
		"map_cto_icon":                  row.MapCtoIcon,
		"map_splice_icon":               row.MapSpliceIcon,
		"map_splice_emenda_color":       row.MapSpliceEmendaColor,
		"map_splice_distribuicao_color": row.MapSpliceDistribuicaoColor,
		"map_splice_emenda_icon":        row.MapSpliceEmendaIcon,
		"map_splice_distribuicao_icon":  row.MapSpliceDistribuicaoIcon,
		"map_cable_funcao_colors":       row.MapCableFuncaoColors,
		"map_equipment_categories":      row.MapEquipmentCategories,
		"map_icon_image_urls":           row.MapIconImageURLs,
		"updated_at":                    row.Updated,
	}
	if source != "" {
		out["source"] = source
	}
	return out
}

func (s *Server) getUIAppearance(w http.ResponseWriter, r *http.Request) {
	p := s.DB()
	if p == nil {
		writeJSON(w, http.StatusOK, uiAppearanceJSON(defaultUIAppearanceRow(), "default_no_db"))
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
		Theme                      *string `json:"theme"`
		MapEquipmentColor          *string `json:"map_equipment_color"`
		MapConnectionColor         *string `json:"map_connection_color"`
		MapCtoColor                *string `json:"map_cto_color"`
		MapSpliceColor             *string `json:"map_splice_color"`
		MapEquipmentIcon           *string `json:"map_equipment_icon"`
		MapConnectionIcon          *string `json:"map_connection_icon"`
		MapCtoIcon                 *string `json:"map_cto_icon"`
		MapSpliceIcon              *string `json:"map_splice_icon"`
		MapSpliceEmendaColor       *string `json:"map_splice_emenda_color"`
		MapSpliceDistribuicaoColor *string `json:"map_splice_distribuicao_color"`
		MapSpliceEmendaIcon        *string `json:"map_splice_emenda_icon"`
		MapSpliceDistribuicaoIcon  *string `json:"map_splice_distribuicao_icon"`
		// Patch parcial: só as funções presentes no objeto são alteradas, as restantes mantêm o
		// valor já gravado (ou o padrão, se nunca foi personalizado).
		MapCableFuncaoColors map[string]string `json:"map_cable_funcao_colors"`
		// Idem — patch parcial por categoria, e dentro de cada categoria só os campos presentes
		// (ponteiro não-nulo) são alterados (ver equipmentCategoryPatch).
		MapEquipmentCategories map[string]equipmentCategoryPatch `json:"map_equipment_categories"`
		// "" apaga o override (volta a usar o ícone/estilo do catálogo).
		MapIconImageURLs map[string]string `json:"map_icon_image_urls"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	if body.Theme == nil && body.MapEquipmentColor == nil && body.MapConnectionColor == nil &&
		body.MapCtoColor == nil && body.MapSpliceColor == nil &&
		body.MapEquipmentIcon == nil && body.MapConnectionIcon == nil &&
		body.MapCtoIcon == nil && body.MapSpliceIcon == nil &&
		body.MapSpliceEmendaColor == nil && body.MapSpliceDistribuicaoColor == nil &&
		body.MapSpliceEmendaIcon == nil && body.MapSpliceDistribuicaoIcon == nil &&
		len(body.MapCableFuncaoColors) == 0 && len(body.MapEquipmentCategories) == 0 && len(body.MapIconImageURLs) == 0 {
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
	if !setColor(body.MapSpliceEmendaColor, &cur.MapSpliceEmendaColor, "map_splice_emenda_color", defaultMapSpliceEmendaColor) {
		return
	}
	if !setColor(body.MapSpliceDistribuicaoColor, &cur.MapSpliceDistribuicaoColor, "map_splice_distribuicao_color", defaultMapSpliceDistribuicaoColor) {
		return
	}
	if !setIcon(body.MapSpliceEmendaIcon, &cur.MapSpliceEmendaIcon, "splice", "map_splice_emenda_icon", defaultMapSpliceEmendaIcon) {
		return
	}
	if !setIcon(body.MapSpliceDistribuicaoIcon, &cur.MapSpliceDistribuicaoIcon, "splice", "map_splice_distribuicao_icon", defaultMapSpliceDistribuicaoIcon) {
		return
	}
	if len(body.MapCableFuncaoColors) > 0 {
		next := map[string]string{}
		for k, v := range cur.MapCableFuncaoColors {
			next[k] = v
		}
		for funcao, hex := range body.MapCableFuncaoColors {
			if _, known := defaultMapCableFuncaoColors[funcao]; !known {
				writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "map_cable_funcao_colors: função desconhecida: "+funcao, nil)
				return
			}
			if !hexColorRe.MatchString(strings.TrimSpace(hex)) {
				writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "map_cable_funcao_colors["+funcao+"] deve ser #RRGGBB", nil)
				return
			}
			next[funcao] = strings.ToLower(strings.TrimSpace(hex))
		}
		cur.MapCableFuncaoColors = next
		audit["map_cable_funcao_colors"] = next
	}
	if len(body.MapEquipmentCategories) > 0 {
		next := map[string]equipmentCategoryConfig{}
		for k, v := range cur.MapEquipmentCategories {
			next[k] = v
		}
		for cat, patch := range body.MapEquipmentCategories {
			if !mapEquipmentCategories[cat] {
				writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "map_equipment_categories: categoria desconhecida: "+cat, nil)
				return
			}
			existing := next[cat]
			if patch.Icon != nil {
				icon := strings.TrimSpace(*patch.Icon)
				if icon == "" {
					existing.Icon = ""
				} else {
					normalized := normalizeMapIcon("equipment", icon, "")
					if normalized == "" {
						writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "map_equipment_categories["+cat+"].icon inválido", nil)
						return
					}
					existing.Icon = normalized
				}
			}
			if patch.ImageURL != nil {
				u := strings.TrimSpace(*patch.ImageURL)
				if u != "" && !looksLikeHTTPURL(u) {
					writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "map_equipment_categories["+cat+"].image_url deve ser http(s)", nil)
					return
				}
				existing.ImageURL = u
			}
			if patch.Hidden != nil {
				existing.Hidden = *patch.Hidden
			}
			next[cat] = existing
		}
		cur.MapEquipmentCategories = next
		audit["map_equipment_categories"] = next
	}
	if len(body.MapIconImageURLs) > 0 {
		next := map[string]string{}
		for k, v := range cur.MapIconImageURLs {
			next[k] = v
		}
		for role, raw := range body.MapIconImageURLs {
			if !mapIconImageURLRoles[role] {
				writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "map_icon_image_urls: tipo desconhecido: "+role, nil)
				return
			}
			u := strings.TrimSpace(raw)
			if u == "" {
				delete(next, role)
				continue
			}
			if !looksLikeHTTPURL(u) {
				writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "map_icon_image_urls["+role+"] deve ser http(s)", nil)
				return
			}
			next[role] = u
		}
		cur.MapIconImageURLs = next
		audit["map_icon_image_urls"] = next
	}
	funcaoColorsJSON, err := json.Marshal(cur.MapCableFuncaoColors)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	equipmentCategoriesJSON, err := json.Marshal(cur.MapEquipmentCategories)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	iconImageURLsJSON, err := json.Marshal(cur.MapIconImageURLs)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	err = p.QueryRow(r.Context(), `
		UPDATE settings_ui
		SET theme = $1,
			map_equipment_color = $2, map_connection_color = $3,
			map_cto_color = $4, map_splice_color = $5,
			map_equipment_icon = $6, map_connection_icon = $7,
			map_cto_icon = $8, map_splice_icon = $9,
			map_splice_emenda_color = $10, map_splice_distribuicao_color = $11,
			map_splice_emenda_icon = $12, map_splice_distribuicao_icon = $13,
			map_cable_funcao_colors = $14::jsonb,
			map_equipment_categories = $15::jsonb, map_icon_image_urls = $16::jsonb,
			updated_at = now()
		WHERE id = 1
		RETURNING updated_at
	`, cur.Theme, cur.MapEquipmentColor, cur.MapConnectionColor, cur.MapCtoColor, cur.MapSpliceColor,
		cur.MapEquipmentIcon, cur.MapConnectionIcon, cur.MapCtoIcon, cur.MapSpliceIcon,
		cur.MapSpliceEmendaColor, cur.MapSpliceDistribuicaoColor, cur.MapSpliceEmendaIcon, cur.MapSpliceDistribuicaoIcon,
		string(funcaoColorsJSON), string(equipmentCategoriesJSON), string(iconImageURLsJSON),
	).Scan(&cur.Updated)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "settings_ui", "1", "patch", s.actorFromRequest(r), nil, audit)
	resp := uiAppearanceJSON(cur, "")
	resp["ok"] = true
	writeJSON(w, http.StatusOK, resp)
}
