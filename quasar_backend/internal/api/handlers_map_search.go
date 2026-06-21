package api

import (
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s *Server) mapLocalityCenter(w http.ResponseWriter, r *http.Request) {
	raw := strings.TrimSpace(r.URL.Query().Get("locality_id"))
	id, err := uuid.Parse(raw)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "locality_id inválido", nil)
		return
	}
	ctx := r.Context()
	var name string
	err = s.DB().QueryRow(ctx, `SELECT name FROM commercial_localities WHERE id=$1`, id).Scan(&name)
	if err == pgx.ErrNoRows {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "localidade não encontrada", nil)
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	var lat, lng *float64
	err = s.DB().QueryRow(ctx, `
		SELECT AVG(t.lat), AVG(t.lng)
		FROM (
			SELECT latitude AS lat, longitude AS lng FROM devices
			 WHERE locality_id = $1::uuid AND latitude IS NOT NULL AND longitude IS NOT NULL
			UNION ALL
			SELECT latitude, longitude FROM network_ctos
			 WHERE locality_id = $1::uuid AND latitude IS NOT NULL AND longitude IS NOT NULL
			UNION ALL
			SELECT latitude, longitude FROM network_poles
			 WHERE locality_id = $1::uuid AND latitude IS NOT NULL AND longitude IS NOT NULL
			UNION ALL
			SELECT latitude, longitude FROM network_projects
			 WHERE locality_id = $1::uuid AND latitude IS NOT NULL AND longitude IS NOT NULL
		) t
	`, id).Scan(&lat, &lng)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if lat == nil || lng == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"locality_id": id.String(),
			"name":        name,
			"found":       false,
			"note":        "Nenhum equipamento ou infraestrutura com coordenadas nesta localidade.",
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"locality_id": id.String(),
		"name":        name,
		"found":       true,
		"lat":         *lat,
		"lng":         *lng,
	})
}

func (s *Server) mapSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	typeFilter := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("type")))
	if len(q) < 2 {
		writeJSON(w, http.StatusOK, map[string]any{"results": []any{}})
		return
	}
	pattern := "%" + q + "%"
	ctx := r.Context()
	var out []map[string]any
	const perKind = 12

	appendIfRoom := func(row map[string]any) {
		if len(out) >= 30 {
			return
		}
		out = append(out, row)
	}

	if typeFilter == "" || typeFilter == "equipment" || typeFilter == "equipamento" {
		rows, err := s.DB().Query(ctx, `
			SELECT d.id::text, COALESCE(NULLIF(trim(d.description), ''), '?'), COALESCE(NULLIF(trim(d.category), ''), ''),
			       COALESCE(d.latitude, p.latitude), COALESCE(d.longitude, p.longitude)
			FROM devices d
			LEFT JOIN pops p ON p.id = d.pop_id
			WHERE COALESCE(d.latitude, p.latitude) IS NOT NULL
			  AND COALESCE(d.longitude, p.longitude) IS NOT NULL
			  AND (
			    d.description ILIKE $1 OR d.category ILIKE $1
			    OR host(d.ip)::text ILIKE $1
			  )
			ORDER BY d.description
			LIMIT $2
		`, pattern, perKind)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var id, desc, cat string
				var lat, lng float64
				if rows.Scan(&id, &desc, &cat, &lat, &lng) == nil {
					appendIfRoom(map[string]any{
						"id": id, "label": desc, "kind": "equipment", "category": cat,
						"lat": lat, "lng": lng, "map_id": id,
					})
				}
			}
		}
	}

	if typeFilter == "" || typeFilter == "login" || typeFilter == "logins" || typeFilter == "connection" {
		rows, err := s.DB().Query(ctx, `
			SELECT id::text, client_name, login, latitude, longitude
			FROM client_connections
			WHERE latitude IS NOT NULL AND longitude IS NOT NULL
			  AND (client_name ILIKE $1 OR login ILIKE $1 OR COALESCE(address, '') ILIKE $1)
			ORDER BY client_name
			LIMIT $2
		`, pattern, perKind)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var id, name, login string
				var lat, lng float64
				if rows.Scan(&id, &name, &login, &lat, &lng) == nil {
					appendIfRoom(map[string]any{
						"id": id, "label": name + " (" + login + ")", "kind": "login",
						"lat": lat, "lng": lng, "map_id": "conn-" + id,
					})
				}
			}
		}
	}

	infraKinds := []struct {
		filter, table, kind, prefix string
	}{
		{"cto", "network_ctos", "cto", "CTO"},
		{"poste", "network_poles", "pole", "Poste"},
		{"pole", "network_poles", "pole", "Poste"},
		{"emenda", "network_splice_boxes", "splice_box", "Emenda"},
		{"cabo", "network_cables", "cable", "Cabo"},
		{"projeto", "network_projects", "project", "Projeto"},
	}
	for _, ik := range infraKinds {
		if typeFilter != "" && typeFilter != "infra" && typeFilter != "infrastructure" && typeFilter != ik.filter && typeFilter != ik.kind {
			continue
		}
		rows, err := s.DB().Query(ctx, `
			SELECT id::text, description, display_number, latitude, longitude
			FROM `+ik.table+`
			WHERE latitude IS NOT NULL AND longitude IS NOT NULL
			  AND description ILIKE $1
			ORDER BY display_number
			LIMIT $2
		`, pattern, perKind)
		if err == nil {
			func() {
				defer rows.Close()
				for rows.Next() {
					var id, desc string
					var num int
					var lat, lng float64
					if rows.Scan(&id, &desc, &num, &lat, &lng) == nil {
						appendIfRoom(map[string]any{
							"id": id, "label": desc, "kind": ik.kind, "category": ik.prefix,
							"lat": lat, "lng": lng, "map_id": "infra-" + ik.kind + "-" + id,
						})
					}
				}
			}()
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"results": out, "q": q})
}
