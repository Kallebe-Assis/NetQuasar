package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

// mapNearestCtos devolve as CTOs mais próximas de um ponto GPS (lat/lng do browser).
// Distância em metros via Haversine sobre DOUBLE PRECISION (sem depender de PostGIS).
func (s *Server) mapNearestCtos(w http.ResponseWriter, r *http.Request) {
	if s.DB() == nil {
		writeErr(w, http.StatusServiceUnavailable, "NO_DB", "base de dados indisponível", nil)
		return
	}
	lat, err := strconv.ParseFloat(strings.TrimSpace(r.URL.Query().Get("lat")), 64)
	if err != nil || lat < -90 || lat > 90 {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "lat inválida", nil)
		return
	}
	lng, err := strconv.ParseFloat(strings.TrimSpace(r.URL.Query().Get("lng")), 64)
	if err != nil || lng < -180 || lng > 180 {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "lng inválida", nil)
		return
	}
	limit := 3
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		n, perr := strconv.Atoi(raw)
		if perr != nil || n < 1 || n > 20 {
			writeErr(w, http.StatusBadRequest, "VALIDATION", "limit deve ser entre 1 e 20", nil)
			return
		}
		limit = n
	}
	projectID, perr := parseInfraMapProjectID(r)
	if perr != nil {
		writeErr(w, http.StatusBadRequest, "VALIDATION", perr.Error(), nil)
		return
	}

	// Haversine (metros) — mesmo modelo geodésico usado no frontend.
	const distExpr = `(6371000.0 * 2.0 * asin(sqrt(
		power(sin(radians(($1::float8 - latitude) / 2.0)), 2) +
		cos(radians(latitude)) * cos(radians($1::float8)) *
		power(sin(radians(($2::float8 - longitude) / 2.0)), 2)
	)))`

	q := `
		SELECT id, description, display_number, latitude, longitude,
			splitter, fiber_color, ` + distExpr + ` AS distance_m
		FROM network_ctos
		WHERE latitude IS NOT NULL AND longitude IS NOT NULL`
	args := []any{lat, lng}
	n := 3
	if projectID != nil {
		q += ` AND project_id = $` + strconv.Itoa(n)
		args = append(args, *projectID)
		n++
	}
	q += ` ORDER BY distance_m ASC NULLS LAST LIMIT $` + strconv.Itoa(n)
	args = append(args, limit)

	rows, err := s.DB().Query(r.Context(), q, args...)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()

	type item struct {
		ID            string  `json:"id"`
		MapID         string  `json:"map_id"`
		Description   string  `json:"description"`
		DisplayNumber int     `json:"display_number"`
		Lat           float64 `json:"lat"`
		Lng           float64 `json:"lng"`
		DistanceM     float64 `json:"distance_m"`
		Splitter      *string `json:"splitter,omitempty"`
		FiberColor    *string `json:"fiber_color,omitempty"`
	}
	out := make([]item, 0, limit)
	for rows.Next() {
		var id uuid.UUID
		var desc string
		var displayNum int
		var clat, clng, dist float64
		var splitter, fiberColor *string
		if err := rows.Scan(&id, &desc, &displayNum, &clat, &clng, &splitter, &fiberColor, &dist); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		it := item{
			ID:            id.String(),
			MapID:         "infra-cto-" + id.String(),
			Description:   desc,
			DisplayNumber: displayNum,
			Lat:           clat,
			Lng:           clng,
			DistanceM:     dist,
		}
		if splitter != nil && strings.TrimSpace(*splitter) != "" {
			s := strings.TrimSpace(*splitter)
			it.Splitter = &s
		}
		if fiberColor != nil && strings.TrimSpace(*fiberColor) != "" {
			c := strings.TrimSpace(*fiberColor)
			it.FiberColor = &c
		}
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"origin": map[string]float64{"lat": lat, "lng": lng},
		"ctos":   out,
		"count":  len(out),
	})
}
