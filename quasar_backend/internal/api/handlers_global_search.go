package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/netquasar/netquasar/quasar_backend/internal/oltcollect"
	"github.com/netquasar/netquasar/quasar_backend/internal/vsolparse"
)

// globalSearch é a pesquisa central do Dashboard: um único campo que varre equipamentos,
// logins/clientes, infraestrutura de rede (CTOs, cabos, postes, emendas/foguetes,
// projectos, POPs) e ONUs (por serial, ao vivo nos snapshots das OLTs, ou pelo nome do
// cliente vinculado — ver handlers_olt_onu_client_links.go). Só corre com pedido
// explícito do utilizador (Enter/botão) — não em cada tecla — por ser uma consulta
// larga sobre várias tabelas/snapshots.
func (s *Server) globalSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) < 2 {
		writeJSON(w, http.StatusOK, map[string]any{"results": []any{}, "q": q})
		return
	}
	pattern := "%" + q + "%"
	ctx := r.Context()
	const perKind = 8
	const totalCap = 60

	var out []map[string]any
	appendIfRoom := func(row map[string]any) bool {
		if len(out) >= totalCap {
			return false
		}
		out = append(out, row)
		return true
	}

	// Equipamentos.
	if rows, err := s.DB().Query(ctx, `
		SELECT id::text, COALESCE(NULLIF(trim(description),''), '?'), COALESCE(category,''),
			COALESCE(host(ip)::text, ''), COALESCE(mac,''), COALESCE(serial_number,''), COALESCE(network_status,'')
		FROM devices
		WHERE description ILIKE $1 OR host(ip)::text ILIKE $1 OR mac ILIKE $1 OR serial_number ILIKE $1
		ORDER BY description
		LIMIT $2
	`, pattern, perKind); err == nil {
		func() {
			defer rows.Close()
			for rows.Next() {
				var id, desc, cat, ip, mac, serial, status string
				if rows.Scan(&id, &desc, &cat, &ip, &mac, &serial, &status) != nil {
					continue
				}
				parts := []string{}
				if cat != "" {
					parts = append(parts, cat)
				}
				if ip != "" {
					parts = append(parts, ip)
				}
				if mac != "" {
					parts = append(parts, "MAC "+mac)
				}
				if serial != "" {
					parts = append(parts, "S/N "+serial)
				}
				appendIfRoom(map[string]any{
					"kind": "device", "label": desc, "subtitle": strings.Join(parts, " · "),
					"status": status, "href": "/devices?focus=" + id,
				})
			}
		}()
	}

	// Logins / clientes.
	if rows, err := s.DB().Query(ctx, `
		SELECT id::text, COALESCE(NULLIF(trim(client_name),''), '?'), COALESCE(login,''),
			COALESCE(ip_address,''), COALESCE(cto,''), COALESCE(onu_mac_sn,'')
		FROM client_connections
		WHERE client_name ILIKE $1 OR login ILIKE $1 OR ip_address ILIKE $1 OR onu_mac_sn ILIKE $1 OR cto ILIKE $1
		ORDER BY client_name
		LIMIT $2
	`, pattern, perKind); err == nil {
		func() {
			defer rows.Close()
			for rows.Next() {
				var id, name, login, ip, cto, macSn string
				if rows.Scan(&id, &name, &login, &ip, &cto, &macSn) != nil {
					continue
				}
				parts := []string{}
				if login != "" {
					parts = append(parts, "login "+login)
				}
				if ip != "" {
					parts = append(parts, ip)
				}
				if cto != "" {
					parts = append(parts, "CTO "+cto)
				}
				if macSn != "" {
					parts = append(parts, macSn)
				}
				appendIfRoom(map[string]any{
					"kind": "connection", "label": name, "subtitle": strings.Join(parts, " · "),
					"href": "/connections",
				})
			}
		}()
	}

	// Infraestrutura de rede (CTOs, postes, emendas/foguetes, cabos, projectos).
	infraSpecs := []struct{ table, kind, label string }{
		{"network_ctos", "cto", "CTO"},
		{"network_poles", "pole", "Poste"},
		{"network_splice_boxes", "splice_box", "Emenda/foguete"},
		{"network_cables", "cable", "Cabo"},
		{"network_projects", "project", "Projeto"},
	}
	for _, spec := range infraSpecs {
		rows, err := s.DB().Query(ctx, `
			SELECT id::text, description, display_number
			FROM `+spec.table+`
			WHERE description ILIKE $1
			ORDER BY display_number
			LIMIT $2
		`, pattern, perKind)
		if err != nil {
			continue
		}
		func() {
			defer rows.Close()
			for rows.Next() {
				var id, desc string
				var num int
				if rows.Scan(&id, &desc, &num) != nil {
					continue
				}
				appendIfRoom(map[string]any{
					"kind": spec.kind, "label": desc,
					"subtitle": spec.label + " #" + strconv.Itoa(num),
					"href":     "/map",
				})
			}
		}()
	}

	// POPs.
	if rows, err := s.DB().Query(ctx, `
		SELECT id::text, description FROM pops WHERE description ILIKE $1 ORDER BY description LIMIT $2
	`, pattern, perKind); err == nil {
		func() {
			defer rows.Close()
			for rows.Next() {
				var id, desc string
				if rows.Scan(&id, &desc) != nil {
					continue
				}
				appendIfRoom(map[string]any{"kind": "pop", "label": desc, "subtitle": "POP", "href": "/map"})
			}
		}()
	}

	// ONUs — ao vivo nos snapshots das OLTs (serial) + nome do cliente vinculado.
	if clientNames, err := s.onuClientNameMap(ctx); err == nil {
		if rows, err := s.DB().Query(ctx, `
			SELECT d.description, COALESCE(o.summary::text, '{}')
			FROM devices d
			JOIN olt_snapshots o ON o.device_id = d.id
			WHERE lower(trim(d.category)) = 'olt'
		`); err == nil {
			func() {
				defer rows.Close()
				qLower := strings.ToLower(q)
				onuHits := 0
				for rows.Next() && onuHits < perKind {
					var oltDesc, sum string
					if rows.Scan(&oltDesc, &sum) != nil {
						continue
					}
					for _, raw := range vsolparse.VsolOnuRowsFromSummaryBlob([]byte(sum)) {
						if onuHits >= perKind {
							break
						}
						row, ok := raw.(map[string]any)
						if !ok {
							continue
						}
						serial := strings.TrimSpace(stringFromAny(row["serial"]))
						clientName := clientNames[strings.ToUpper(serial)]
						if !oltcollect.SerialPartialMatch(serial, q) &&
							!(clientName != "" && strings.Contains(strings.ToLower(clientName), qLower)) {
							continue
						}
						label := serial
						if clientName != "" {
							label = clientName + " — " + serial
						}
						parts := []string{oltDesc}
						if pon := intFromOnuSearchRow(row, "pon"); pon > 0 {
							parts = append(parts, "PON "+strconv.Itoa(pon))
						}
						if on, ok := row["online"].(bool); ok {
							if on {
								parts = append(parts, "online")
							} else {
								parts = append(parts, "offline")
							}
						}
						if appendIfRoom(map[string]any{
							"kind": "onu", "label": label, "subtitle": strings.Join(parts, " · "),
							"href": "/olt",
						}) {
							onuHits++
						}
					}
				}
			}()
		}
	}

	if out == nil {
		out = []map[string]any{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": out, "q": q, "total": len(out)})
}
