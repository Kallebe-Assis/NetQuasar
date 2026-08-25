package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

var validInfraMapKinds = map[string]string{
	"ctos":         "cto",
	"splice_boxes": "splice_box",
	"cables":       "cable",
	"poles":        "pole",
	"projects":     "project",
	"pops":         "pop",
}

func parseInfraMapKindsQuery(r *http.Request) []string {
	raw := strings.TrimSpace(r.URL.Query().Get("kinds"))
	if raw == "" {
		return []string{"ctos", "splice_boxes", "cables", "poles"}
	}
	var out []string
	for _, p := range strings.Split(raw, ",") {
		k := strings.TrimSpace(p)
		if k == "" {
			continue
		}
		if _, ok := validInfraMapKinds[k]; ok {
			out = append(out, k)
		}
	}
	if len(out) == 0 {
		return []string{"ctos", "splice_boxes", "cables", "poles"}
	}
	return out
}

func infraMapBBoxSQL(hasBBox bool, n *int, args *[]any, minLat, maxLat, minLng, maxLng float64) string {
	if !hasBBox {
		return ""
	}
	clause := fmt.Sprintf(` AND latitude >= $%d AND latitude <= $%d AND longitude >= $%d AND longitude <= $%d`, *n, *n+1, *n+2, *n+3)
	*args = append(*args, minLat, maxLat, minLng, maxLng)
	*n += 4
	return clause
}

func infraMapProjectSQL(projectID *uuid.UUID, n *int, args *[]any) string {
	if projectID == nil {
		return ""
	}
	clause := fmt.Sprintf(` AND project_id = $%d`, *n)
	*args = append(*args, *projectID)
	*n++
	return clause
}

// Projetos inativos e os respectivos elementos não aparecem no mapa.
func infraMapHideInactiveSQL(table string) string {
	if table == "network_projects" {
		return ` AND status <> 'inativo'`
	}
	return ` AND (project_id IS NULL OR EXISTS (SELECT 1 FROM network_projects np WHERE np.id = project_id AND np.status <> 'inativo'))`
}

func parseInfraMapProjectID(r *http.Request) (*uuid.UUID, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("project_id"))
	if raw == "" {
		return nil, nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil, errors.New("project_id inválido")
	}
	return &id, nil
}

func parseInfraMapLocalityID(r *http.Request) (*uuid.UUID, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("locality_id"))
	if raw == "" {
		return nil, nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil, errors.New("locality_id inválido")
	}
	return &id, nil
}

// Filtra por localidade directa ou pelo projecto associado (cabos/emendas só têm project_id).
func infraMapLocalitySQL(table string, localityID *uuid.UUID, n *int, args *[]any) string {
	if localityID == nil {
		return ""
	}
	switch table {
	case "network_projects", "pops":
		clause := fmt.Sprintf(` AND locality_id = $%d`, *n)
		*args = append(*args, *localityID)
		*n++
		return clause
	case "network_ctos", "network_poles":
		clause := fmt.Sprintf(` AND (
			locality_id = $%d
			OR EXISTS (SELECT 1 FROM network_projects np WHERE np.id = %s.project_id AND np.locality_id = $%d)
		)`, *n, table, *n)
		*args = append(*args, *localityID)
		*n++
		return clause
	default:
		// network_cables, network_splice_boxes, etc.
		clause := fmt.Sprintf(` AND EXISTS (
			SELECT 1 FROM network_projects np WHERE np.id = %s.project_id AND np.locality_id = $%d
		)`, table, *n)
		*args = append(*args, *localityID)
		*n++
		return clause
	}
}

func (s *Server) mapInfrastructurePoints(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	kinds := parseInfraMapKindsQuery(r)
	minLat, maxLat, minLng, maxLng, hasBBox := parseMapBBoxQuery(r)
	projectID, perr := parseInfraMapProjectID(r)
	if perr != nil {
		writeErr(w, http.StatusBadRequest, "VALIDATION", perr.Error(), nil)
		return
	}
	localityID, lerr := parseInfraMapLocalityID(r)
	if lerr != nil {
		writeErr(w, http.StatusBadRequest, "VALIDATION", lerr.Error(), nil)
		return
	}
	zoom := parseMapZoomQuery(r)
	scoped := projectID != nil || localityID != nil
	limit := mapInfrastructureLimit(zoom, hasBBox, scoped)

	kindSet := map[string]bool{}
	for _, k := range kinds {
		kindSet[k] = true
	}

	var pts []map[string]any
	remaining := limit
	hitCap := false
	centerLat := (minLat + maxLat) / 2
	centerLng := (minLng + maxLng) / 2

	orderNearCenter := func(n *int, args *[]any) string {
		if !hasBBox {
			return ` ORDER BY display_number`
		}
		clause := fmt.Sprintf(` ORDER BY ((latitude - $%d)^2 + (longitude - $%d)^2) ASC, display_number`, *n, *n+1)
		*args = append(*args, centerLat, centerLng)
		*n += 2
		return clause
	}

	take := func(kind string) int {
		return mapInfraKindCap(kind, zoom, remaining, scoped)
	}

	appendRows := func(table, kind, idPrefix string, extraSelect string) error {
		capN := take(kind)
		if capN <= 0 {
			return nil
		}
		q := `SELECT id, description, display_number, latitude, longitude` + extraSelect + `
			FROM ` + table + `
			WHERE latitude IS NOT NULL AND longitude IS NOT NULL`
		args := []any{}
		n := 1
		q += infraMapBBoxSQL(hasBBox, &n, &args, minLat, maxLat, minLng, maxLng)
		q += infraMapProjectSQL(projectID, &n, &args)
		q += infraMapLocalitySQL(table, localityID, &n, &args)
		q += infraMapHideInactiveSQL(table)
		q += orderNearCenter(&n, &args)
		q += fmt.Sprintf(` LIMIT $%d`, n)
		args = append(args, capN)

		rows, err := s.DB().Query(ctx, q, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		fetched := 0
		for rows.Next() {
			var id uuid.UUID
			var desc string
			var displayNum int
			var lat, lon float64
			scanArgs := []any{&id, &desc, &displayNum, &lat, &lon}
			var color *string
			if extraSelect != "" {
				scanArgs = append(scanArgs, &color)
			}
			if err := rows.Scan(scanArgs...); err != nil {
				return err
			}
			pt := map[string]any{
				"id":             id.String(),
				"description":    desc,
				"display_number": displayNum,
				"lat":            lat,
				"lng":            lon,
				"point_type":     kind,
				"id_prefix":      idPrefix,
			}
			if color != nil && strings.TrimSpace(*color) != "" {
				pt["color"] = strings.TrimSpace(*color)
			}
			pts = append(pts, pt)
			fetched++
			remaining--
			if remaining <= 0 {
				hitCap = true
				break
			}
		}
		if fetched >= capN {
			hitCap = true
		}
		return nil
	}

	if kindSet["ctos"] {
		capN := take("cto")
		if capN > 0 {
			q := `SELECT id, description, display_number, latitude, longitude, splitter, fiber_color
				FROM network_ctos
				WHERE latitude IS NOT NULL AND longitude IS NOT NULL`
			args := []any{}
			n := 1
			q += infraMapBBoxSQL(hasBBox, &n, &args, minLat, maxLat, minLng, maxLng)
			q += infraMapProjectSQL(projectID, &n, &args)
			q += infraMapLocalitySQL("network_ctos", localityID, &n, &args)
			q += infraMapHideInactiveSQL("network_ctos")
			q += orderNearCenter(&n, &args)
			q += fmt.Sprintf(` LIMIT $%d`, n)
			args = append(args, capN)
			rows, err := s.DB().Query(ctx, q, args...)
			if err != nil {
				writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
				return
			}
			fetched := 0
			for rows.Next() {
				var id uuid.UUID
				var desc string
				var displayNum int
				var lat, lon float64
				var splitter, fiberColor *string
				if err := rows.Scan(&id, &desc, &displayNum, &lat, &lon, &splitter, &fiberColor); err != nil {
					rows.Close()
					writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
					return
				}
				pt := map[string]any{
					"id":             id.String(),
					"description":    desc,
					"display_number": displayNum,
					"lat":            lat,
					"lng":            lon,
					"point_type":     "cto",
					"id_prefix":      "CTO",
				}
				if splitter != nil && strings.TrimSpace(*splitter) != "" {
					pt["splitter"] = strings.TrimSpace(*splitter)
				}
				if fiberColor != nil && strings.TrimSpace(*fiberColor) != "" {
					pt["fiber_color"] = strings.TrimSpace(*fiberColor)
				}
				pts = append(pts, pt)
				fetched++
				remaining--
				if remaining <= 0 {
					hitCap = true
					break
				}
			}
			rows.Close()
			if fetched >= capN {
				hitCap = true
			}
		}
	}
	if kindSet["splice_boxes"] {
		if err := appendRows("network_splice_boxes", "splice_box", "Emenda", ""); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
	}
	if kindSet["cables"] {
		capN := take("cable")
		if capN > 0 {
			q := `SELECT id, description, display_number, latitude, longitude, path
				FROM network_cables
				WHERE latitude IS NOT NULL AND longitude IS NOT NULL`
			args := []any{}
			n := 1
			q += infraMapBBoxSQL(hasBBox, &n, &args, minLat, maxLat, minLng, maxLng)
			q += infraMapProjectSQL(projectID, &n, &args)
			q += infraMapLocalitySQL("network_cables", localityID, &n, &args)
			q += infraMapHideInactiveSQL("network_cables")
			q += orderNearCenter(&n, &args)
			q += fmt.Sprintf(` LIMIT $%d`, n)
			args = append(args, capN)
			rows, err := s.DB().Query(ctx, q, args...)
			if err != nil {
				writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
				return
			}
			fetched := 0
			for rows.Next() {
				var id uuid.UUID
				var desc string
				var displayNum int
				var lat, lon float64
				var pathRaw []byte
				if err := rows.Scan(&id, &desc, &displayNum, &lat, &lon, &pathRaw); err != nil {
					rows.Close()
					writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
					return
				}
				pt := map[string]any{
					"id":             id.String(),
					"description":    desc,
					"display_number": displayNum,
					"lat":            lat,
					"lng":            lon,
					"point_type":     "cable",
					"id_prefix":      "Cabo",
				}
				if len(pathRaw) > 0 && string(pathRaw) != "null" {
					var path any
					if err := json.Unmarshal(pathRaw, &path); err == nil {
						pt["path"] = path
					}
				}
				pts = append(pts, pt)
				fetched++
				remaining--
				if remaining <= 0 {
					hitCap = true
					break
				}
			}
			rows.Close()
			if fetched >= capN {
				hitCap = true
			}
		}
	}
	if kindSet["poles"] {
		if err := appendRows("network_poles", "pole", "Poste", ""); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
	}
	if kindSet["projects"] {
		if projectID != nil {
			capN := take("project")
			if capN > 0 {
				q := `SELECT id, description, display_number, latitude, longitude, color
					FROM network_projects
					WHERE latitude IS NOT NULL AND longitude IS NOT NULL AND id = $1 AND status <> 'inativo'`
				args := []any{*projectID}
				n := 2
				q += infraMapBBoxSQL(hasBBox, &n, &args, minLat, maxLat, minLng, maxLng)
				q += fmt.Sprintf(` ORDER BY display_number LIMIT $%d`, n)
				args = append(args, capN)
				rows, err := s.DB().Query(ctx, q, args...)
				if err != nil {
					writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
					return
				}
				for rows.Next() {
					var id uuid.UUID
					var desc string
					var displayNum int
					var lat, lon float64
					var color *string
					if err := rows.Scan(&id, &desc, &displayNum, &lat, &lon, &color); err != nil {
						rows.Close()
						writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
						return
					}
					pt := map[string]any{
						"id":             id.String(),
						"description":    desc,
						"display_number": displayNum,
						"lat":            lat,
						"lng":            lon,
						"point_type":     "project",
						"id_prefix":      "Projeto",
					}
					if color != nil && strings.TrimSpace(*color) != "" {
						pt["color"] = strings.TrimSpace(*color)
					}
					pts = append(pts, pt)
					remaining--
				}
				rows.Close()
			}
		} else if err := appendRows("network_projects", "project", "Projeto", `, color`); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
	}
	if kindSet["pops"] {
		capN := take("pop")
		if capN > 0 {
			q := `SELECT id, description, latitude, longitude
				FROM pops
				WHERE latitude IS NOT NULL AND longitude IS NOT NULL`
			args := []any{}
			n := 1
			q += infraMapBBoxSQL(hasBBox, &n, &args, minLat, maxLat, minLng, maxLng)
			q += infraMapLocalitySQL("pops", localityID, &n, &args)
			q += orderNearCenter(&n, &args)
			q += fmt.Sprintf(` LIMIT $%d`, n)
			args = append(args, capN)
			rows, err := s.DB().Query(ctx, q, args...)
			if err != nil {
				writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
				return
			}
			fetched := 0
			for rows.Next() {
				var id uuid.UUID
				var desc string
				var lat, lon float64
				if err := rows.Scan(&id, &desc, &lat, &lon); err != nil {
					rows.Close()
					writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
					return
				}
				pts = append(pts, map[string]any{
					"id":             id.String(),
					"description":    desc,
					"display_number": 0,
					"lat":            lat,
					"lng":            lon,
					"point_type":     "pop",
					"id_prefix":      "POP",
				})
				fetched++
				remaining--
				if remaining <= 0 {
					hitCap = true
					break
				}
			}
			rows.Close()
			if fetched >= capN {
				hitCap = true
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"points":    pts,
		"total":     len(pts),
		"truncated": hitCap || remaining <= 0,
		"limit":     limit,
		"zoom":      zoom,
		"scoped":    scoped,
	})
}

// mapInfrastructureLimit limita pins de infra no mapa. Com filtro de projecto/localidade o tecto sobe.
// Sem filtro (todos os projectos) o tecto também é generoso — CTOs espalhadas por várias localidades.
func mapInfrastructureLimit(zoom float64, hasBBox bool, scoped bool) int {
	if scoped {
		if !hasBBox {
			return 10000
		}
		switch {
		case zoom < 11:
			return 4000
		case zoom < 13:
			return 7000
		case zoom < 15:
			return 10000
		default:
			return 15000
		}
	}
	if !hasBBox {
		return 4000
	}
	switch {
	case zoom < 10:
		return 2500
	case zoom < 12:
		return 5000
	case zoom < 14:
		return 8000
	case zoom < 16:
		return 12000
	default:
		return 15000
	}
}

// mapInfraKindCap evita que um único tipo ocupe todo o orçamento (relaxado com filtro scoped).
func mapInfraKindCap(kind string, zoom float64, remaining int, scoped bool) int {
	if remaining <= 0 {
		return 0
	}
	if scoped {
		return remaining
	}
	capN := remaining
	switch kind {
	case "cto":
		// Prioridade às CTOs: tecto alto para cobrir várias localidades no viewport.
		switch {
		case zoom < 11:
			if capN > 2000 {
				capN = 2000
			}
		case zoom < 13:
			if capN > 4500 {
				capN = 4500
			}
		case zoom < 15:
			if capN > 9000 {
				capN = 9000
			}
		default:
			if capN > 12000 {
				capN = 12000
			}
		}
	case "cable":
		switch {
		case zoom < 12:
			if capN > 80 {
				capN = 80
			}
		case zoom < 14:
			if capN > 200 {
				capN = 200
			}
		default:
			if capN > 400 {
				capN = 400
			}
		}
	case "splice_box", "pole":
		if zoom < 12 && capN > 80 {
			capN = 80
		} else if zoom < 15 && capN > 400 {
			capN = 400
		} else if capN > 1000 {
			capN = 1000
		}
	case "project", "pop":
		if capN > 120 {
			capN = 120
		}
	}
	return capN
}
