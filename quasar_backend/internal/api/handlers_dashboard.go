package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Filtro alinhado ao monitoramento: só equipamentos em operação «Ativo».
const sqlDeviceOperationalAtivo = `TRIM(BOTH FROM COALESCE(operational_mode, '')) = 'Ativo'`
const sqlDeviceOperationalAtivoD = `TRIM(BOTH FROM COALESCE(d.operational_mode, '')) = 'Ativo'`

// dashboardAnalytics agrega leituras materializadas (sem ping/SNMP inline).
func (s *Server) dashboardAnalytics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days <= 0 || days > 90 {
		days = 30
	}
	if s.rt != nil && s.rt.redis != nil {
		cacheKey := "netquasar:dashboard:analytics:" + strconv.Itoa(days)
		if txt, err := s.rt.redis.Get(ctx, cacheKey).Result(); err == nil && strings.TrimSpace(txt) != "" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(txt))
			return
		}
	}
	since := time.Now().UTC().AddDate(0, 0, -days)

	var nDev, nPops, nClients, telDev, pingDev int64
	var monRunning bool
	_ = s.DB().QueryRow(ctx, `SELECT COUNT(*) FROM devices`).Scan(&nDev)
	_ = s.DB().QueryRow(ctx, `SELECT COUNT(*) FROM pops`).Scan(&nPops)
	_ = s.DB().QueryRow(ctx, `
		SELECT COALESCE(SUM(client_count), 0)::bigint FROM commercial_monthly_records
		WHERE year_month = to_char((CURRENT_TIMESTAMP AT TIME ZONE 'UTC'), 'YYYY-MM')
	`).Scan(&nClients)
	_ = s.DB().QueryRow(ctx, `SELECT is_running FROM monitoring_runtime WHERE id=1`).Scan(&monRunning)
	_ = s.DB().QueryRow(ctx, `
		SELECT COUNT(*) FROM devices WHERE telemetry_enabled = true AND `+sqlDeviceOperationalAtivo).Scan(&telDev)
	_ = s.DB().QueryRow(ctx, `
		SELECT COUNT(*) FROM devices WHERE ping_enabled = true AND `+sqlDeviceOperationalAtivo).Scan(&pingDev)
	totals := map[string]any{
		"devices":                   nDev,
		"pops":                      nPops,
		"commercial_clients_sum":    nClients,
		"monitoring_running":        monRunning,
		"telemetry_enabled_devices": telDev,
		"ping_enabled_devices":      pingDev,
	}

	out := map[string]any{
		"generated_at": time.Now().UTC().Format(time.RFC3339),
		"days":         days,
		"since":        since.Format(time.RFC3339),
		"totals":       totals,
	}

	if rows, err := s.DB().Query(ctx, `
		SELECT COALESCE(NULLIF(trim(category), ''), '(sem categoria)'), COUNT(*)::bigint
		FROM devices GROUP BY 1 ORDER BY 2 DESC, 1`); err == nil {
		var byCat []map[string]any
		for rows.Next() {
			var c string
			var n int64
			if rows.Scan(&c, &n) == nil {
				byCat = append(byCat, map[string]any{"category": c, "count": n})
			}
		}
		rows.Close()
		out["devices_by_category"] = byCat
	} else {
		out["devices_by_category"] = []any{}
	}

	if rows, err := s.DB().Query(ctx, `
		SELECT COALESCE(NULLIF(trim(network_status), ''), '—'), COUNT(*)::bigint
		FROM devices GROUP BY 1 ORDER BY 2 DESC`); err == nil {
		var byNs []map[string]any
		for rows.Next() {
			var ns string
			var n int64
			if rows.Scan(&ns, &n) == nil {
				byNs = append(byNs, map[string]any{"network_status": ns, "count": n})
			}
		}
		rows.Close()
		out["devices_by_network_status"] = byNs
	} else {
		out["devices_by_network_status"] = []any{}
	}

	if rows, err := s.DB().Query(ctx, `
		SELECT COALESCE(NULLIF(trim(operational_mode), ''), '—'), COUNT(*)::bigint
		FROM devices GROUP BY 1 ORDER BY 2 DESC`); err == nil {
		var byOp []map[string]any
		for rows.Next() {
			var op string
			var n int64
			if rows.Scan(&op, &n) == nil {
				byOp = append(byOp, map[string]any{"operational_mode": op, "count": n})
			}
		}
		rows.Close()
		out["devices_by_operational_mode"] = byOp
	} else {
		out["devices_by_operational_mode"] = []any{}
	}

	if rows, err := s.DB().Query(ctx, `
		SELECT p.id::text, p.description, COUNT(d.id)::bigint
		FROM pops p
		LEFT JOIN devices d ON d.pop_id = p.id
		GROUP BY p.id, p.description
		ORDER BY 3 DESC, p.description`); err == nil {
		var byPop []map[string]any
		for rows.Next() {
			var pid, desc string
			var n int64
			if rows.Scan(&pid, &desc, &n) == nil {
				byPop = append(byPop, map[string]any{"pop_id": pid, "pop_name": desc, "count": n})
			}
		}
		rows.Close()
		out["devices_by_pop"] = byPop
	} else {
		out["devices_by_pop"] = []any{}
	}

	if rows, err := s.DB().Query(ctx, `
		SELECT l.id::text, l.name, COUNT(d.id)::bigint
		FROM commercial_localities l
		LEFT JOIN devices d ON d.locality_id = l.id
		GROUP BY l.id, l.name
		ORDER BY 3 DESC, l.name`); err == nil {
		var byLoc []map[string]any
		for rows.Next() {
			var lid, name string
			var n int64
			if rows.Scan(&lid, &name, &n) == nil {
				byLoc = append(byLoc, map[string]any{"locality_id": lid, "locality_name": name, "count": n})
			}
		}
		rows.Close()
		out["devices_by_locality"] = byLoc
	} else {
		out["devices_by_locality"] = []any{}
	}

	var pingN, pingOk int64
	var pingAvg *float64
	_ = s.DB().QueryRow(ctx, `
		SELECT COUNT(*)::bigint,
			COUNT(*) FILTER (WHERE ph.ok)::bigint,
			AVG(ph.latency_ms)::float8 FILTER (WHERE ph.ok AND ph.latency_ms IS NOT NULL)
		FROM ping_history ph
		JOIN devices d ON d.id = ph.device_id AND `+sqlDeviceOperationalAtivoD+`
		WHERE ph.checked_at >= $1`, since).Scan(&pingN, &pingOk, &pingAvg)
	pingRatio := float64(0)
	if pingN > 0 {
		pingRatio = float64(pingOk) / float64(pingN) * 100
	}
	out["ping_window"] = map[string]any{
		"samples":        pingN,
		"ok_samples":     pingOk,
		"ok_percent":     pingRatio,
		"avg_latency_ms": pingAvg,
	}

	if rows, err := s.DB().Query(ctx, `
		SELECT d.id::text, d.description, AVG(ph.latency_ms)::float8 AS avg_ms, COUNT(*)::bigint AS n
		FROM ping_history ph
		JOIN devices d ON d.id = ph.device_id AND `+sqlDeviceOperationalAtivoD+`
		WHERE ph.checked_at >= $1 AND ph.ok = true AND ph.latency_ms IS NOT NULL
		GROUP BY d.id, d.description
		HAVING COUNT(*) >= 3
		ORDER BY avg_ms DESC NULLS LAST
		LIMIT 12`, since); err == nil {
		var worst []map[string]any
		for rows.Next() {
			var id, desc string
			var avg float64
			var n int64
			if rows.Scan(&id, &desc, &avg, &n) == nil {
				worst = append(worst, map[string]any{"device_id": id, "description": desc, "avg_latency_ms": avg, "samples": n})
			}
		}
		rows.Close()
		out["ping_ranking_worst_latency"] = worst
	} else {
		out["ping_ranking_worst_latency"] = []any{}
	}

	if rows, err := s.DB().Query(ctx, `
		SELECT d.id::text, d.description, AVG(ph.latency_ms)::float8 AS avg_ms, COUNT(*)::bigint AS n
		FROM ping_history ph
		JOIN devices d ON d.id = ph.device_id AND `+sqlDeviceOperationalAtivoD+`
		WHERE ph.checked_at >= $1 AND ph.ok = true AND ph.latency_ms IS NOT NULL
		GROUP BY d.id, d.description
		HAVING COUNT(*) >= 3
		ORDER BY avg_ms ASC NULLS LAST
		LIMIT 12`, since); err == nil {
		var best []map[string]any
		for rows.Next() {
			var id, desc string
			var avg float64
			var n int64
			if rows.Scan(&id, &desc, &avg, &n) == nil {
				best = append(best, map[string]any{"device_id": id, "description": desc, "avg_latency_ms": avg, "samples": n})
			}
		}
		rows.Close()
		out["ping_ranking_best_latency"] = best
	} else {
		out["ping_ranking_best_latency"] = []any{}
	}

	var telN int64
	_ = s.DB().QueryRow(ctx, `
		SELECT COUNT(*)::bigint
		FROM telemetry_samples ts
		JOIN devices d ON d.id = ts.device_id AND `+sqlDeviceOperationalAtivoD+`
		WHERE ts.collected_at >= $1`, since).Scan(&telN)
	out["telemetry_window"] = map[string]any{"samples": telN}

	if rows, err := s.DB().Query(ctx, `
		SELECT ai.alert_type, COUNT(*)::bigint
		FROM alert_instances ai
		JOIN devices d ON d.id = ai.device_id AND `+sqlDeviceOperationalAtivoD+`
		WHERE ai.active_since >= $1
		GROUP BY ai.alert_type ORDER BY 2 DESC`, since); err == nil {
		var at []map[string]any
		for rows.Next() {
			var typ string
			var n int64
			if rows.Scan(&typ, &n) == nil {
				at = append(at, map[string]any{"alert_type": typ, "count": n})
			}
		}
		rows.Close()
		out["alerts_by_type_30d"] = at
	} else {
		out["alerts_by_type_30d"] = []any{}
	}

	var openAlerts int64
	_ = s.DB().QueryRow(ctx, `
		SELECT COUNT(*)::bigint
		FROM alert_instances ai
		JOIN devices d ON d.id = ai.device_id AND `+sqlDeviceOperationalAtivoD+`
		WHERE ai.closed_at IS NULL`).Scan(&openAlerts)
	out["alerts_open"] = openAlerts

	if rows, err := s.DB().Query(ctx, `
		SELECT d.id::text, d.description, d.brand,
			COALESCE((
				SELECT SUM(COALESCE((NULLIF(trim(e->>'onu_total'), ''))::bigint, 0))
				FROM jsonb_array_elements(CASE WHEN jsonb_typeof(o.pons) = 'array' THEN o.pons ELSE '[]'::jsonb END) e
			), 0)::bigint AS onu_total,
			COALESCE((
				SELECT SUM(COALESCE((NULLIF(trim(e->>'onu_online'), ''))::bigint, 0))
				FROM jsonb_array_elements(CASE WHEN jsonb_typeof(o.pons) = 'array' THEN o.pons ELSE '[]'::jsonb END) e
			), 0)::bigint AS onu_online,
			COALESCE((
				SELECT SUM(COALESCE((NULLIF(trim(e->>'onu_offline'), ''))::bigint, 0))
				FROM jsonb_array_elements(CASE WHEN jsonb_typeof(o.pons) = 'array' THEN o.pons ELSE '[]'::jsonb END) e
			), 0)::bigint AS onu_offline,
			o.updated_at
		FROM devices d
		JOIN olt_snapshots o ON o.device_id = d.id
		WHERE lower(trim(d.category)) = 'olt' AND `+sqlDeviceOperationalAtivoD+`
		ORDER BY onu_total DESC, d.description
		LIMIT 24`); err == nil {
		var olts []map[string]any
		for rows.Next() {
			var id, desc string
			var brand *string
			var onuTotal, onuOn, onuOff int64
			var upd time.Time
			if rows.Scan(&id, &desc, &brand, &onuTotal, &onuOn, &onuOff, &upd) == nil {
				m := map[string]any{
					"device_id":   id,
					"description": desc,
					"onu_count":   onuTotal,
					"onu_online":  onuOn,
					"onu_offline": onuOff,
					"snapshot_at": upd.Format(time.RFC3339),
				}
				if brand != nil {
					m["brand"] = *brand
				}
				olts = append(olts, m)
			}
		}
		rows.Close()
		out["olt_onu_by_device"] = olts
		var fleetTotal, fleetOn, fleetOff int64
		for _, o := range olts {
			fleetTotal += o["onu_count"].(int64)
			fleetOn += o["onu_online"].(int64)
			fleetOff += o["onu_offline"].(int64)
		}
		out["olt_onu_fleet_totals"] = map[string]any{
			"onu_count":   fleetTotal,
			"onu_online":  fleetOn,
			"onu_offline": fleetOff,
		}
	} else {
		out["olt_onu_by_device"] = []any{}
		out["olt_onu_fleet_totals"] = map[string]any{"onu_count": int64(0), "onu_online": int64(0), "onu_offline": int64(0)}
	}

	if rows, err := s.DB().Query(ctx, `
		SELECT DISTINCT ON (i.device_id) i.device_id, d.description, i.collected_at, i.interfaces::text
		FROM interface_snapshots i
		JOIN devices d ON d.id = i.device_id AND `+sqlDeviceOperationalAtivoD+`
		WHERE i.collected_at >= $1
		  AND (
			lower(trim(d.category)) LIKE '%mikrotik%'
			OR lower(coalesce(d.brand, '')) LIKE '%mikrotik%'
		  )
		ORDER BY i.device_id, i.collected_at DESC
		LIMIT 16`, since); err == nil {
		var mk []map[string]any
		for rows.Next() {
			var did uuid.UUID
			var desc string
			var ts time.Time
			var raw string
			if rows.Scan(&did, &desc, &ts, &raw) != nil {
				continue
			}
			inO, outO := parseIfOctetsFromSnapshotJSON(raw)
			mk = append(mk, map[string]any{
				"device_id":     did.String(),
				"description":   desc,
				"collected_at":  ts.Format(time.RFC3339),
				"if_in_octets":  inO,
				"if_out_octets": outO,
				"note":          "Soma ifInOctets / ifOutOctets (IF-MIB) na última amostra por equipamento Mikrotik (Ativo).",
			})
		}
		rows.Close()
		out["mikrotik_interface_traffic_latest"] = mk
	} else {
		out["mikrotik_interface_traffic_latest"] = []any{}
	}

	if s.rt != nil && s.rt.redis != nil {
		if raw, err := json.Marshal(out); err == nil {
			cacheKey := "netquasar:dashboard:analytics:" + strconv.Itoa(days)
			_ = s.rt.redis.Set(ctx, cacheKey, string(raw), 45*time.Second).Err()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(raw)
			return
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func parseIfOctetsFromSnapshotJSON(raw string) (inSum, outSum *int64) {
	var arr []struct {
		OID   string `json:"oid"`
		Value string `json:"value"`
	}
	if json.Unmarshal([]byte(raw), &arr) != nil {
		return nil, nil
	}
	var inAcc, outAcc int64
	var hasIn, hasOut bool
	for _, v := range arr {
		oid := strings.TrimSpace(v.OID)
		oid = strings.TrimPrefix(oid, ".")
		val := strings.TrimSpace(v.Value)
		if oid == "" || val == "" {
			continue
		}
		if strings.Contains(oid, "1.3.6.1.2.1.2.2.1.10.") {
			if n, err := strconv.ParseInt(val, 10, 64); err == nil {
				inAcc += n
				hasIn = true
			}
		}
		if strings.Contains(oid, "1.3.6.1.2.1.2.2.1.16.") {
			if n, err := strconv.ParseInt(val, 10, 64); err == nil {
				outAcc += n
				hasOut = true
			}
		}
	}
	if hasIn {
		inSum = &inAcc
	}
	if hasOut {
		outSum = &outAcc
	}
	return
}
