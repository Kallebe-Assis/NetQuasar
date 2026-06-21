package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

func (s *Server) mapEquipmentPoints(w http.ResponseWriter, r *http.Request) {
	q := `
		SELECT d.id, d.description, d.category,
			COALESCE(d.latitude, p.latitude) AS latitude,
			COALESCE(d.longitude, p.longitude) AS longitude,
			host(d.ip)::text, d.pop_id, d.operational_mode,
			d.ping_enabled,
			COALESCE(c.ok, false) AS probe_ok,
			c.reach_ok,
			c.checked_at,
			CASE
				WHEN d.latitude IS NOT NULL AND d.longitude IS NOT NULL THEN 'device'
				WHEN p.latitude IS NOT NULL AND p.longitude IS NOT NULL THEN 'pop'
				ELSE 'none'
			END AS coord_source
		FROM devices d
		LEFT JOIN pops p ON p.id = d.pop_id
		LEFT JOIN device_probe_cache c ON c.device_id = d.id
		WHERE COALESCE(d.latitude, p.latitude) IS NOT NULL
		  AND COALESCE(d.longitude, p.longitude) IS NOT NULL
	`
	args := []any{}
	n := 1
	if v := r.URL.Query().Get("pop_id"); v != "" {
		if id, err := uuid.Parse(v); err == nil {
			q += ` AND d.pop_id = $` + strconv.Itoa(n)
			args = append(args, id)
			n++
		}
	}
	if v := strings.TrimSpace(r.URL.Query().Get("category")); v != "" {
		q += ` AND lower(d.category) = lower($` + strconv.Itoa(n) + `)`
		args = append(args, v)
		n++
	}
	if v := strings.TrimSpace(r.URL.Query().Get("q")); v != "" {
		pat := "%" + v + "%"
		q += ` AND (d.description ILIKE $` + strconv.Itoa(n) + ` OR host(d.ip)::text ILIKE $` + strconv.Itoa(n+1) + `)`
		args = append(args, pat, pat)
		n += 2
	}
	if mn, err := strconv.ParseFloat(r.URL.Query().Get("min_lat"), 64); err == nil {
		q += ` AND COALESCE(d.latitude, p.latitude) >= $` + strconv.Itoa(n)
		args = append(args, mn)
		n++
	}
	if mx, err := strconv.ParseFloat(r.URL.Query().Get("max_lat"), 64); err == nil {
		q += ` AND COALESCE(d.latitude, p.latitude) <= $` + strconv.Itoa(n)
		args = append(args, mx)
		n++
	}
	if mn, err := strconv.ParseFloat(r.URL.Query().Get("min_lon"), 64); err == nil {
		q += ` AND COALESCE(d.longitude, p.longitude) >= $` + strconv.Itoa(n)
		args = append(args, mn)
		n++
	}
	if mx, err := strconv.ParseFloat(r.URL.Query().Get("max_lon"), 64); err == nil {
		q += ` AND COALESCE(d.longitude, p.longitude) <= $` + strconv.Itoa(n)
		args = append(args, mx)
		n++
	}
	q += ` ORDER BY d.description LIMIT 2000`
	rows, err := s.DB().Query(r.Context(), q, args...)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	var pts []map[string]any
	for rows.Next() {
		var id uuid.UUID
		var desc, cat, op, coordSource string
		var lat, lon float64
		var ip *string
		var popID *uuid.UUID
		var pingEnabled bool
		var probeOK bool
		var reachOK *bool
		var checked *time.Time
		if err := rows.Scan(&id, &desc, &cat, &lat, &lon, &ip, &popID, &op, &pingEnabled, &probeOK, &reachOK, &checked, &coordSource); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		st := mapDeviceReachabilityStatus(pingEnabled, checked, probeOK, reachOK)
		pts = append(pts, map[string]any{
			"id": id, "description": desc, "category": cat, "lat": lat, "lng": lon,
			"ip": ip, "pop_id": popID, "operational_mode": op, "status": st, "last_check_at": checked,
			"coord_source": coordSource,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"points": pts})
}
