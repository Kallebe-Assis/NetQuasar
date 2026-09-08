package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

const (
	mapRadiusDefaultM = 250.0
	mapRadiusMinM     = 10.0
	mapRadiusMaxM     = 5000.0
)

// haversineExprM expressão SQL de distância em metros entre ($1,$2) e (latitude,longitude) —
// mesmo modelo geodésico de mapNearestCtos (handlers_map_nearest.go), sem depender de PostGIS.
const haversineExprM = `(6371000.0 * 2.0 * asin(sqrt(
	power(sin(radians(($1::float8 - latitude) / 2.0)), 2) +
	cos(radians(latitude)) * cos(radians($1::float8)) *
	power(sin(radians(($2::float8 - longitude) / 2.0)), 2)
)))`

// mapRadiusSearch — "Raio de Atendimento": localiza elementos (CTO/foguete/poste/equipamento)
// dentro de um raio (metros) de um ponto qualquer do mapa (clique do utilizador, não só GPS —
// ver mapNearestCtos, que é só CTO e sempre GPS/top-N). Para CTOs devolve também a contagem de
// portas disponíveis (ports_free, ver ctoPortCounts) — pedido explícito do utilizador para
// avaliar se dá para atender um novo cliente na área.
func (s *Server) mapRadiusSearch(w http.ResponseWriter, r *http.Request) {
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
	radius := mapRadiusDefaultM
	if raw := strings.TrimSpace(r.URL.Query().Get("radius_m")); raw != "" {
		v, perr := strconv.ParseFloat(raw, 64)
		if perr != nil || v < mapRadiusMinM || v > mapRadiusMaxM {
			writeErr(w, http.StatusBadRequest, "VALIDATION", "radius_m deve ser entre 10 e 5000 metros", nil)
			return
		}
		radius = v
	}
	kindSet := map[string]bool{"cto": true, "splice_box": true, "pole": true, "equipment": true}
	if raw := strings.TrimSpace(r.URL.Query().Get("kinds")); raw != "" {
		kindSet = map[string]bool{}
		for _, k := range strings.Split(raw, ",") {
			k = strings.ToLower(strings.TrimSpace(k))
			if k != "" {
				kindSet[k] = true
			}
		}
	}
	const maxPerKind = 200
	ctx := r.Context()
	var points []map[string]any

	if kindSet["cto"] {
		q := `
			SELECT id, description, display_number, latitude, longitude, splitter, fiber_color, splitter_ports,
				` + haversineExprM + ` AS distance_m
			FROM network_ctos
			WHERE latitude IS NOT NULL AND longitude IS NOT NULL AND ` + haversineExprM + ` <= $3
			ORDER BY distance_m ASC LIMIT ` + strconv.Itoa(maxPerKind)
		rows, err := s.DB().Query(ctx, q, lat, lng, radius)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		for rows.Next() {
			var id uuid.UUID
			var desc string
			var displayNum int
			var clat, clng, dist float64
			var splitter, fiberColor *string
			var splitterPorts []byte
			if err := rows.Scan(&id, &desc, &displayNum, &clat, &clng, &splitter, &fiberColor, &splitterPorts, &dist); err != nil {
				rows.Close()
				writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
				return
			}
			pt := map[string]any{
				"point_type": "cto", "id": id.String(), "map_id": "infra-cto-" + id.String(),
				"description": desc, "display_number": displayNum, "lat": clat, "lng": clng, "distance_m": dist,
			}
			if splitter != nil && strings.TrimSpace(*splitter) != "" {
				pt["splitter"] = strings.TrimSpace(*splitter)
			}
			ratio := ""
			if splitter != nil {
				ratio = *splitter
			}
			if pTot, pUsed, pFree := ctoPortCounts(splitterPorts, ratio); pTot > 0 {
				pt["ports_total"] = pTot
				pt["ports_used"] = pUsed
				pt["ports_free"] = pFree
			}
			points = append(points, pt)
		}
		rows.Close()
	}

	if kindSet["splice_box"] {
		q := `
			SELECT id, description, display_number, latitude, longitude, box_model,
				` + haversineExprM + ` AS distance_m
			FROM network_splice_boxes
			WHERE latitude IS NOT NULL AND longitude IS NOT NULL AND ` + haversineExprM + ` <= $3
			ORDER BY distance_m ASC LIMIT ` + strconv.Itoa(maxPerKind)
		rows, err := s.DB().Query(ctx, q, lat, lng, radius)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		for rows.Next() {
			var id uuid.UUID
			var desc string
			var displayNum int
			var clat, clng, dist float64
			var boxModel *string
			if err := rows.Scan(&id, &desc, &displayNum, &clat, &clng, &boxModel, &dist); err != nil {
				rows.Close()
				writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
				return
			}
			pt := map[string]any{
				"point_type": "splice_box", "id": id.String(), "map_id": "infra-splice_box-" + id.String(),
				"description": desc, "display_number": displayNum, "lat": clat, "lng": clng, "distance_m": dist,
			}
			if boxModel != nil && strings.TrimSpace(*boxModel) != "" {
				pt["box_model"] = strings.TrimSpace(*boxModel)
			}
			points = append(points, pt)
		}
		rows.Close()
	}

	if kindSet["pole"] {
		q := `
			SELECT id, description, display_number, latitude, longitude, pole_type, height_m, has_transformer, material,
				` + haversineExprM + ` AS distance_m
			FROM network_poles
			WHERE latitude IS NOT NULL AND longitude IS NOT NULL AND ` + haversineExprM + ` <= $3
			ORDER BY distance_m ASC LIMIT ` + strconv.Itoa(maxPerKind)
		rows, err := s.DB().Query(ctx, q, lat, lng, radius)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		for rows.Next() {
			var id uuid.UUID
			var desc string
			var displayNum int
			var clat, clng, dist float64
			var poleType, material *string
			var heightM *float64
			var hasTransformer bool
			if err := rows.Scan(&id, &desc, &displayNum, &clat, &clng, &poleType, &heightM, &hasTransformer, &material, &dist); err != nil {
				rows.Close()
				writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
				return
			}
			pt := map[string]any{
				"point_type": "pole", "id": id.String(), "map_id": "infra-pole-" + id.String(),
				"description": desc, "display_number": displayNum, "lat": clat, "lng": clng, "distance_m": dist,
				"has_transformer": hasTransformer,
			}
			if poleType != nil && strings.TrimSpace(*poleType) != "" {
				pt["pole_type"] = strings.TrimSpace(*poleType)
			}
			if heightM != nil {
				pt["height_m"] = *heightM
			}
			if material != nil && strings.TrimSpace(*material) != "" {
				pt["material"] = strings.TrimSpace(*material)
			}
			points = append(points, pt)
		}
		rows.Close()
	}

	if kindSet["equipment"] {
		// Coordenada do equipamento (própria, ou herdada do POP) — não pode usar haversineExprM
		// directamente (esse assume colunas "latitude"/"longitude" simples); substitui-as pela
		// mesma expressão COALESCE usada no SELECT.
		const devHaversineExprM = `(6371000.0 * 2.0 * asin(sqrt(
			power(sin(radians(($1::float8 - COALESCE(d.latitude, p.latitude)) / 2.0)), 2) +
			cos(radians(COALESCE(d.latitude, p.latitude))) * cos(radians($1::float8)) *
			power(sin(radians(($2::float8 - COALESCE(d.longitude, p.longitude)) / 2.0)), 2)
		)))`
		q := `
			SELECT d.id, d.description, d.category, host(d.ip)::text,
				COALESCE(d.latitude, p.latitude) AS lat_out,
				COALESCE(d.longitude, p.longitude) AS lng_out,
				` + devHaversineExprM + ` AS distance_m
			FROM devices d
			LEFT JOIN pops p ON p.id = d.pop_id
			WHERE COALESCE(d.latitude, p.latitude) IS NOT NULL AND COALESCE(d.longitude, p.longitude) IS NOT NULL
			  AND ` + devHaversineExprM + ` <= $3
			ORDER BY distance_m ASC LIMIT ` + strconv.Itoa(maxPerKind)
		rows, err := s.DB().Query(ctx, q, lat, lng, radius)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		for rows.Next() {
			var id uuid.UUID
			var desc, category string
			var ip *string
			var clat, clng, dist float64
			if err := rows.Scan(&id, &desc, &category, &ip, &clat, &clng, &dist); err != nil {
				rows.Close()
				writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
				return
			}
			pt := map[string]any{
				"point_type": "equipment", "id": id.String(), "map_id": id.String(),
				"description": desc, "category": category, "lat": clat, "lng": clng, "distance_m": dist,
			}
			if ip != nil && strings.TrimSpace(*ip) != "" {
				pt["ip"] = strings.TrimSpace(*ip)
			}
			points = append(points, pt)
		}
		rows.Close()
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"origin":   map[string]float64{"lat": lat, "lng": lng},
		"radius_m": radius,
		"points":   points,
		"count":    len(points),
	})
}
