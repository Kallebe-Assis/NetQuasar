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

func (s *Server) mapInfrastructurePoints(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	kinds := parseInfraMapKindsQuery(r)
	minLat, maxLat, minLng, maxLng, hasBBox := parseMapBBoxQuery(r)
	projectID, perr := parseInfraMapProjectID(r)
	if perr != nil {
		writeErr(w, http.StatusBadRequest, "VALIDATION", perr.Error(), nil)
		return
	}
	// Com filtro de projeto: carregar tudo do projeto (sem bbox do viewport).
	if projectID != nil {
		hasBBox = false
	}
	zoom := parseMapZoomQuery(r)
	limit := mapConnectionLimit(zoom, hasBBox)
	if !hasBBox {
		limit = 2500
	} else if limit <= 0 {
		limit = 2500
	}

	kindSet := map[string]bool{}
	for _, k := range kinds {
		kindSet[k] = true
	}

	var pts []map[string]any
	remaining := limit

	appendRows := func(table, kind, idPrefix string, extraSelect string) error {
		if remaining <= 0 {
			return nil
		}
		q := `SELECT id, description, display_number, latitude, longitude` + extraSelect + `
			FROM ` + table + `
			WHERE latitude IS NOT NULL AND longitude IS NOT NULL`
		args := []any{}
		n := 1
		q += infraMapBBoxSQL(hasBBox, &n, &args, minLat, maxLat, minLng, maxLng)
		q += infraMapProjectSQL(projectID, &n, &args)
		q += fmt.Sprintf(` ORDER BY display_number LIMIT $%d`, n)
		args = append(args, remaining)

		rows, err := s.DB().Query(ctx, q, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

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
			remaining--
			if remaining <= 0 {
				break
			}
		}
		return nil
	}

	if kindSet["ctos"] {
		if remaining > 0 {
			q := `SELECT id, description, display_number, latitude, longitude, splitter, fiber_color
				FROM network_ctos
				WHERE latitude IS NOT NULL AND longitude IS NOT NULL`
			args := []any{}
			n := 1
			q += infraMapBBoxSQL(hasBBox, &n, &args, minLat, maxLat, minLng, maxLng)
			q += infraMapProjectSQL(projectID, &n, &args)
			q += fmt.Sprintf(` ORDER BY display_number LIMIT $%d`, n)
			args = append(args, remaining)
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
				remaining--
				if remaining <= 0 {
					break
				}
			}
			rows.Close()
		}
	}
	if kindSet["splice_boxes"] {
		if err := appendRows("network_splice_boxes", "splice_box", "Emenda", ""); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
	}
	if kindSet["cables"] && remaining > 0 {
		q := `SELECT id, description, display_number, latitude, longitude, path
			FROM network_cables
			WHERE latitude IS NOT NULL AND longitude IS NOT NULL`
		args := []any{}
		n := 1
		q += infraMapBBoxSQL(hasBBox, &n, &args, minLat, maxLat, minLng, maxLng)
		q += infraMapProjectSQL(projectID, &n, &args)
		q += fmt.Sprintf(` ORDER BY display_number LIMIT $%d`, n)
		args = append(args, remaining)
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
			remaining--
			if remaining <= 0 {
				break
			}
		}
		rows.Close()
	}
	if kindSet["poles"] {
		if err := appendRows("network_poles", "pole", "Poste", ""); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
	}
	if kindSet["projects"] {
		if projectID != nil {
			if remaining > 0 {
				q := `SELECT id, description, display_number, latitude, longitude, color
					FROM network_projects
					WHERE latitude IS NOT NULL AND longitude IS NOT NULL AND id = $1`
				args := []any{*projectID}
				n := 2
				q += infraMapBBoxSQL(hasBBox, &n, &args, minLat, maxLat, minLng, maxLng)
				q += fmt.Sprintf(` ORDER BY display_number LIMIT $%d`, n)
				args = append(args, remaining)
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

	writeJSON(w, http.StatusOK, map[string]any{
		"points":    pts,
		"total":     len(pts),
		"truncated": remaining <= 0,
		"limit":     limit,
	})
}
