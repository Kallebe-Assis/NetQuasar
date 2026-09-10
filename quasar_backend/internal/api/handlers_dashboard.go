package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"
)

// Filtro alinhado ao monitoramento: só equipamentos em operação «Ativo».
const sqlDeviceOperationalAtivo = `TRIM(BOTH FROM COALESCE(operational_mode, '')) = 'Ativo'`
const sqlDeviceOperationalAtivoD = `TRIM(BOTH FROM COALESCE(d.operational_mode, '')) = 'Ativo'`

const dashboardRedisTTL = 5 * time.Minute

func (s *Server) dashboardCacheGet(ctx context.Context, key string) []byte {
	if b := dashboardMemGet(key); len(b) > 0 {
		return b
	}
	if s.rt != nil && s.rt.redis != nil {
		if txt, err := s.rt.redis.Get(ctx, key).Result(); err == nil && strings.TrimSpace(txt) != "" {
			b := []byte(txt)
			dashboardMemSet(key, b)
			return b
		}
	}
	return nil
}

func (s *Server) dashboardCacheSet(ctx context.Context, key string, body []byte) {
	dashboardMemSet(key, body)
	if s.rt != nil && s.rt.redis != nil {
		_ = s.rt.redis.Set(ctx, key, string(body), dashboardRedisTTL).Err()
	}
}

func (s *Server) dashboardCacheBust(ctx context.Context) {
	dashboardMemDeletePrefix("netquasar:dashboard:")
	if s.rt != nil && s.rt.redis != nil {
		// Best-effort: chaves conhecidas.
		for _, d := range []int{7, 14, 30, 60, 90} {
			_ = s.rt.redis.Del(ctx, "netquasar:dashboard:analytics:"+strconv.Itoa(d)).Err()
		}
		_ = s.rt.redis.Del(ctx, "netquasar:dashboard:olt-capacity").Err()
	}
}

func writeDashboardCachedJSON(w http.ResponseWriter, body []byte, fromCache bool) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, max-age=60")
	if fromCache {
		w.Header().Set("X-NetQuasar-Cache", "HIT")
	} else {
		w.Header().Set("X-NetQuasar-Cache", "MISS")
	}
	_, _ = w.Write(body)
}

// dashboardAnalytics agrega leituras materializadas (sem ping/SNMP inline).
func (s *Server) dashboardAnalytics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days <= 0 || days > 90 {
		days = 7
	}
	refresh := strings.TrimSpace(r.URL.Query().Get("refresh")) == "1"
	cacheKey := "netquasar:dashboard:analytics:" + strconv.Itoa(days)

	if !refresh {
		if cached := s.dashboardCacheGet(ctx, cacheKey); len(cached) > 0 {
			writeDashboardCachedJSON(w, cached, true)
			return
		}
	} else {
		s.dashboardCacheBust(ctx)
	}

	pool := s.DB()
	if pool == nil {
		writeErr(w, http.StatusServiceUnavailable, "NO_DB", "base de dados indisponível", nil)
		return
	}

	since := time.Now().UTC().AddDate(0, 0, -days)
	out, err := s.buildDashboardAnalytics(ctx, pool, days, since)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	raw, err := json.Marshal(out)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "JSON", err.Error(), nil)
		return
	}
	s.dashboardCacheSet(ctx, cacheKey, raw)
	writeDashboardCachedJSON(w, raw, false)
}

func (s *Server) buildDashboardAnalytics(ctx context.Context, pool *pgxpool.Pool, days int, since time.Time) (map[string]any, error) {
	var (
		mu sync.Mutex
		out = map[string]any{
			"generated_at": time.Now().UTC().Format(time.RFC3339),
			"days":         days,
			"since":        since.Format(time.RFC3339),
		}
	)
	set := func(key string, val any) {
		mu.Lock()
		out[key] = val
		mu.Unlock()
	}

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(8)

	g.Go(func() error {
		var nDev, nPops, nClients, telDev, pingDev int64
		var monRunning bool
		_ = pool.QueryRow(gctx, `SELECT COUNT(*) FROM devices`).Scan(&nDev)
		_ = pool.QueryRow(gctx, `SELECT COUNT(*) FROM pops`).Scan(&nPops)
		_ = pool.QueryRow(gctx, `
			SELECT COALESCE(SUM(client_count), 0)::bigint FROM commercial_monthly_records
			WHERE year_month = to_char((CURRENT_TIMESTAMP AT TIME ZONE 'UTC'), 'YYYY-MM')
		`).Scan(&nClients)
		_ = pool.QueryRow(gctx, `SELECT is_running FROM monitoring_runtime WHERE id=1`).Scan(&monRunning)
		_ = pool.QueryRow(gctx, `
			SELECT COUNT(*) FROM devices WHERE telemetry_enabled = true AND `+sqlDeviceOperationalAtivo).Scan(&telDev)
		_ = pool.QueryRow(gctx, `
			SELECT COUNT(*) FROM devices WHERE ping_enabled = true AND `+sqlDeviceOperationalAtivo).Scan(&pingDev)
		set("totals", map[string]any{
			"devices":                   nDev,
			"pops":                      nPops,
			"commercial_clients_sum":    nClients,
			"monitoring_running":        monRunning,
			"telemetry_enabled_devices": telDev,
			"ping_enabled_devices":      pingDev,
		})
		return nil
	})

	g.Go(func() error {
		rows, err := pool.Query(gctx, `
			SELECT COALESCE(NULLIF(trim(category), ''), '(sem categoria)'), COUNT(*)::bigint
			FROM devices GROUP BY 1 ORDER BY 2 DESC, 1`)
		if err != nil {
			set("devices_by_category", []any{})
			return nil
		}
		defer rows.Close()
		var byCat []map[string]any
		for rows.Next() {
			var c string
			var n int64
			if rows.Scan(&c, &n) == nil {
				byCat = append(byCat, map[string]any{"category": c, "count": n})
			}
		}
		set("devices_by_category", byCat)
		return nil
	})

	g.Go(func() error {
		rows, err := pool.Query(gctx, `
			SELECT COALESCE(NULLIF(trim(network_status), ''), '—'), COUNT(*)::bigint
			FROM devices GROUP BY 1 ORDER BY 2 DESC`)
		if err != nil {
			set("devices_by_network_status", []any{})
			return nil
		}
		defer rows.Close()
		var byNs []map[string]any
		for rows.Next() {
			var ns string
			var n int64
			if rows.Scan(&ns, &n) == nil {
				byNs = append(byNs, map[string]any{"network_status": ns, "count": n})
			}
		}
		set("devices_by_network_status", byNs)
		return nil
	})

	g.Go(func() error {
		rows, err := pool.Query(gctx, `
			SELECT COALESCE(NULLIF(trim(operational_mode), ''), '—'), COUNT(*)::bigint
			FROM devices GROUP BY 1 ORDER BY 2 DESC`)
		if err != nil {
			set("devices_by_operational_mode", []any{})
			return nil
		}
		defer rows.Close()
		var byOp []map[string]any
		for rows.Next() {
			var op string
			var n int64
			if rows.Scan(&op, &n) == nil {
				byOp = append(byOp, map[string]any{"operational_mode": op, "count": n})
			}
		}
		set("devices_by_operational_mode", byOp)
		return nil
	})

	g.Go(func() error {
		// Ocupação de portas das CTOs (splitter_ports JSONB — ver ctoPortCounts em
		// handlers_network_infrastructure.go, mesma definição de "ocupada"/"livre"). Card
		// "Portas de CTO" no dashboard + escala de cores no mapa usam este mesmo critério.
		var totalCtos, ctosWithPorts, portsTotal, portsUsed, portsFree int64
		_ = pool.QueryRow(gctx, `SELECT COUNT(*) FROM network_ctos`).Scan(&totalCtos)
		_ = pool.QueryRow(gctx, `
			SELECT
				COUNT(*),
				COALESCE(SUM(jsonb_array_length(splitter_ports)), 0),
				COALESCE(SUM((SELECT COUNT(*) FROM jsonb_array_elements(splitter_ports) e WHERE e->>'status' = 'ocupada')), 0),
				COALESCE(SUM((SELECT COUNT(*) FROM jsonb_array_elements(splitter_ports) e WHERE e->>'status' = 'livre')), 0)
			FROM network_ctos
			WHERE splitter_ports IS NOT NULL AND jsonb_typeof(splitter_ports) = 'array' AND jsonb_array_length(splitter_ports) > 0
		`).Scan(&ctosWithPorts, &portsTotal, &portsUsed, &portsFree)

		var topFull []map[string]any
		rows, err := pool.Query(gctx, `
			SELECT id::text, display_number, description,
				jsonb_array_length(splitter_ports) AS total,
				(SELECT COUNT(*) FROM jsonb_array_elements(splitter_ports) e WHERE e->>'status' = 'ocupada') AS used
			FROM network_ctos
			WHERE splitter_ports IS NOT NULL AND jsonb_typeof(splitter_ports) = 'array' AND jsonb_array_length(splitter_ports) > 0
			ORDER BY used::float8 / jsonb_array_length(splitter_ports) DESC, total DESC
			LIMIT 10
		`)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var id, desc string
				var dispN, total, used int
				if rows.Scan(&id, &dispN, &desc, &total, &used) == nil {
					topFull = append(topFull, map[string]any{
						"id": id, "display_number": dispN, "description": desc,
						"ports_total": total, "ports_used": used, "ports_free": total - used,
					})
				}
			}
		}
		set("cto_ports", map[string]any{
			"total_ctos":      totalCtos,
			"ctos_with_ports": ctosWithPorts,
			"ports_total":     portsTotal,
			"ports_used":      portsUsed,
			"ports_free":      portsFree,
			"top_occupied":    topFull,
		})
		return nil
	})

	g.Go(func() error {
		rows, err := pool.Query(gctx, `
			SELECT p.id::text, p.description, COUNT(d.id)::bigint
			FROM pops p
			LEFT JOIN devices d ON d.pop_id = p.id
			GROUP BY p.id, p.description
			ORDER BY 3 DESC, p.description`)
		if err != nil {
			set("devices_by_pop", []any{})
			return nil
		}
		defer rows.Close()
		var byPop []map[string]any
		for rows.Next() {
			var pid, desc string
			var n int64
			if rows.Scan(&pid, &desc, &n) == nil {
				byPop = append(byPop, map[string]any{"pop_id": pid, "pop_name": desc, "count": n})
			}
		}
		set("devices_by_pop", byPop)
		return nil
	})

	g.Go(func() error {
		rows, err := pool.Query(gctx, `
			SELECT l.id::text, l.name, COUNT(d.id)::bigint
			FROM commercial_localities l
			LEFT JOIN devices d ON d.locality_id = l.id
			GROUP BY l.id, l.name
			ORDER BY 3 DESC, l.name`)
		if err != nil {
			set("devices_by_locality", []any{})
			return nil
		}
		defer rows.Close()
		var byLoc []map[string]any
		for rows.Next() {
			var lid, name string
			var n int64
			if rows.Scan(&lid, &name, &n) == nil {
				byLoc = append(byLoc, map[string]any{"locality_id": lid, "locality_name": name, "count": n})
			}
		}
		set("devices_by_locality", byLoc)
		return nil
	})

	// Rankings de latência a partir do cache de probe (leve) em vez de AVG em ping_history.
	g.Go(func() error {
		rows, err := pool.Query(gctx, `
			SELECT d.id::text, d.description, c.latency_ms::float8, 1::bigint
			FROM device_probe_cache c
			JOIN devices d ON d.id = c.device_id AND `+sqlDeviceOperationalAtivoD+`
			WHERE c.latency_ms IS NOT NULL AND c.ok = true
			ORDER BY c.latency_ms DESC NULLS LAST
			LIMIT 12`)
		if err != nil {
			set("ping_ranking_worst_latency", []any{})
			return nil
		}
		defer rows.Close()
		var worst []map[string]any
		for rows.Next() {
			var id, desc string
			var avg float64
			var n int64
			if rows.Scan(&id, &desc, &avg, &n) == nil {
				worst = append(worst, map[string]any{"device_id": id, "description": desc, "avg_latency_ms": avg, "samples": n})
			}
		}
		set("ping_ranking_worst_latency", worst)
		return nil
	})

	g.Go(func() error {
		rows, err := pool.Query(gctx, `
			SELECT d.id::text, d.description, c.latency_ms::float8, 1::bigint
			FROM device_probe_cache c
			JOIN devices d ON d.id = c.device_id AND `+sqlDeviceOperationalAtivoD+`
			WHERE c.latency_ms IS NOT NULL AND c.ok = true
			ORDER BY c.latency_ms ASC NULLS LAST
			LIMIT 12`)
		if err != nil {
			set("ping_ranking_best_latency", []any{})
			return nil
		}
		defer rows.Close()
		var best []map[string]any
		for rows.Next() {
			var id, desc string
			var avg float64
			var n int64
			if rows.Scan(&id, &desc, &avg, &n) == nil {
				best = append(best, map[string]any{"device_id": id, "description": desc, "avg_latency_ms": avg, "samples": n})
			}
		}
		set("ping_ranking_best_latency", best)
		return nil
	})

	g.Go(func() error {
		var pingN, pingOk int64
		var pingAvg *float64
		// Janela curta (máx. 7 dias) para não varrer histórico enorme.
		winSince := since
		maxWin := time.Now().UTC().AddDate(0, 0, -7)
		if winSince.Before(maxWin) {
			winSince = maxWin
		}
		_ = pool.QueryRow(gctx, `
			SELECT COUNT(*)::bigint,
				COUNT(*) FILTER (WHERE ph.ok)::bigint,
				AVG(ph.latency_ms) FILTER (WHERE ph.ok AND ph.latency_ms IS NOT NULL)::float8
			FROM ping_history ph
			JOIN devices d ON d.id = ph.device_id AND `+sqlDeviceOperationalAtivoD+`
			WHERE ph.checked_at >= $1`, winSince).Scan(&pingN, &pingOk, &pingAvg)
		pingRatio := float64(0)
		if pingN > 0 {
			pingRatio = float64(pingOk) / float64(pingN) * 100
		}
		set("ping_window", map[string]any{
			"samples":        pingN,
			"ok_samples":     pingOk,
			"ok_percent":     pingRatio,
			"avg_latency_ms": pingAvg,
		})
		return nil
	})

	g.Go(func() error {
		// Contagem leve: últimas 24h em vez de varrer 30–90 dias de telemetry_samples.
		var telN int64
		_ = pool.QueryRow(gctx, `
			SELECT COUNT(*)::bigint
			FROM telemetry_samples ts
			JOIN devices d ON d.id = ts.device_id AND `+sqlDeviceOperationalAtivoD+`
			WHERE ts.collected_at >= now() - interval '24 hours'`).Scan(&telN)
		set("telemetry_window", map[string]any{"samples": telN, "window": "24h"})
		return nil
	})

	g.Go(func() error {
		rows, err := pool.Query(gctx, `
			SELECT ai.alert_type, COUNT(*)::bigint
			FROM alert_instances ai
			JOIN devices d ON d.id = ai.device_id AND `+sqlDeviceOperationalAtivoD+`
			WHERE ai.active_since >= $1
			GROUP BY ai.alert_type ORDER BY 2 DESC
			LIMIT 40`, since)
		if err != nil {
			set("alerts_by_type_30d", []any{})
			return nil
		}
		defer rows.Close()
		var at []map[string]any
		for rows.Next() {
			var typ string
			var n int64
			if rows.Scan(&typ, &n) == nil {
				at = append(at, map[string]any{"alert_type": typ, "count": n})
			}
		}
		set("alerts_by_type_30d", at)
		return nil
	})

	g.Go(func() error {
		var openAlerts int64
		_ = pool.QueryRow(gctx, `
			SELECT COUNT(*)::bigint
			FROM alert_instances ai
			JOIN devices d ON d.id = ai.device_id AND `+sqlDeviceOperationalAtivoD+`
			WHERE ai.closed_at IS NULL`).Scan(&openAlerts)
		set("alerts_open", openAlerts)
		return nil
	})

	g.Go(func() error {
		rows, err := pool.Query(gctx, `
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
			LIMIT 24`)
		if err != nil {
			set("olt_onu_by_device", []any{})
			set("olt_onu_fleet_totals", map[string]any{"onu_count": int64(0), "onu_online": int64(0), "onu_offline": int64(0)})
			return nil
		}
		defer rows.Close()
		var olts []map[string]any
		var fleetTotal, fleetOn, fleetOff int64
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
				fleetTotal += onuTotal
				fleetOn += onuOn
				fleetOff += onuOff
			}
		}
		set("olt_onu_by_device", olts)
		set("olt_onu_fleet_totals", map[string]any{
			"onu_count": fleetTotal, "onu_online": fleetOn, "onu_offline": fleetOff,
		})
		return nil
	})

	g.Go(func() error {
		// Evita puxar JSON enorme: só metadados + parse limitado (máx. 8 equipamentos, JSON < 400KB).
		rows, err := pool.Query(gctx, `
			SELECT DISTINCT ON (i.device_id)
				i.device_id, d.description, i.collected_at,
				CASE WHEN octet_length(i.interfaces::text) > 400000 THEN NULL ELSE i.interfaces::text END
			FROM interface_snapshots i
			JOIN devices d ON d.id = i.device_id AND `+sqlDeviceOperationalAtivoD+`
			WHERE i.collected_at >= now() - interval '7 days'
			  AND (
				lower(trim(d.category)) LIKE '%mikrotik%'
				OR lower(coalesce(d.brand, '')) LIKE '%mikrotik%'
			  )
			ORDER BY i.device_id, i.collected_at DESC
			LIMIT 8`)
		if err != nil {
			set("mikrotik_interface_traffic_latest", []any{})
			return nil
		}
		defer rows.Close()
		var mk []map[string]any
		for rows.Next() {
			var did uuid.UUID
			var desc string
			var ts time.Time
			var raw *string
			if rows.Scan(&did, &desc, &ts, &raw) != nil {
				continue
			}
			item := map[string]any{
				"device_id":    did.String(),
				"description":  desc,
				"collected_at": ts.Format(time.RFC3339),
				"note":         "Soma ifInOctets / ifOutOctets (IF-MIB) na última amostra (Ativo).",
			}
			if raw != nil && *raw != "" {
				inO, outO := parseIfOctetsFromSnapshotJSON(*raw)
				item["if_in_octets"] = inO
				item["if_out_octets"] = outO
			}
			mk = append(mk, item)
		}
		set("mikrotik_interface_traffic_latest", mk)
		return nil
	})

	// Dashboard → Fibra: por PON, quantas ONUs ONLINE estão com RX abaixo do limiar de "boa"
	// (Configurações → OLT → "Qualidade da potência RX (ONU)", coluna onu_rx_good_dbm). Só conta
	// linhas online e com rx_dbm numérico — offline e sem leitura óptica ficam de fora, como pedido.
	g.Go(func() error {
		var rxGood float64 = -23
		_ = pool.QueryRow(gctx, `SELECT COALESCE(onu_rx_good_dbm, -23) FROM monitoring_settings WHERE id=1`).Scan(&rxGood)
		rows, err := pool.Query(gctx, `
			SELECT d.id::text, d.description, (e->>'pon')::int AS pon, COUNT(*)::bigint AS low_rx
			FROM devices d
			JOIN olt_snapshots o ON o.device_id = d.id
			CROSS JOIN LATERAL jsonb_array_elements(
				CASE WHEN jsonb_typeof(o.summary->'vsol_onu_rows') = 'array' THEN o.summary->'vsol_onu_rows' ELSE '[]'::jsonb END
			) e
			WHERE lower(trim(d.category)) = 'olt' AND `+sqlDeviceOperationalAtivoD+`
				AND (e->>'online') = 'true'
				AND e->>'rx_dbm' ~ '^-?[0-9]+(\.[0-9]+)?$'
				AND (e->>'rx_dbm')::float8 < $1
				AND (e->>'pon') ~ '^[0-9]+$'
			GROUP BY d.id, d.description, (e->>'pon')::int
			ORDER BY low_rx DESC, d.description, pon
			LIMIT 300`, rxGood)
		if err != nil {
			set("low_rx_pons", map[string]any{"threshold_dbm": rxGood, "rows": []any{}})
			return nil
		}
		defer rows.Close()
		var list []map[string]any
		var totalOnus int64
		for rows.Next() {
			var id, desc string
			var pon int
			var cnt int64
			if rows.Scan(&id, &desc, &pon, &cnt) == nil {
				list = append(list, map[string]any{"olt_id": id, "olt": desc, "pon": pon, "count": cnt})
				totalOnus += cnt
			}
		}
		set("low_rx_pons", map[string]any{
			"threshold_dbm": rxGood,
			"total_onus":    totalOnus,
			"pon_count":     len(list),
			"rows":          list,
		})
		return nil
	})

	// Dashboard → Infraestrutura: estado das CTOs (vazia/disponível/próx. saturação/lotada),
	// CTOs por tipo de splitter, caixas de emenda vs distribuição, e totais por projeto.
	g.Go(func() error {
		infra := map[string]any{}

		// Baldes de ocupação das CTOs (mesma definição "ocupada"/"livre" do card "Portas de CTO").
		var totalCtos int64
		_ = pool.QueryRow(gctx, `SELECT COUNT(*) FROM network_ctos`).Scan(&totalCtos)
		buckets := map[string]int64{"vazia": 0, "disponivel": 0, "proxima_saturacao": 0, "lotada": 0, "sem_portas": 0}
		bRows, err := pool.Query(gctx, `
			SELECT bucket, COUNT(*)::bigint FROM (
				SELECT CASE
					WHEN total = 0 THEN 'sem_portas'
					WHEN free = 0 THEN 'lotada'
					WHEN used::float8 / total >= 0.8 THEN 'proxima_saturacao'
					WHEN used = 0 THEN 'vazia'
					ELSE 'disponivel'
				END AS bucket
				FROM (
					SELECT
						jsonb_array_length(sp) AS total,
						(SELECT COUNT(*) FROM jsonb_array_elements(sp) x WHERE x->>'status' = 'ocupada') AS used,
						(SELECT COUNT(*) FROM jsonb_array_elements(sp) x WHERE x->>'status' = 'livre') AS free
					FROM (
						SELECT CASE WHEN jsonb_typeof(splitter_ports) = 'array' THEN splitter_ports ELSE '[]'::jsonb END AS sp
						FROM network_ctos
					) s
				) c
			) b GROUP BY bucket`)
		if err == nil {
			for bRows.Next() {
				var b string
				var n int64
				if bRows.Scan(&b, &n) == nil {
					buckets[b] = n
				}
			}
			bRows.Close()
		}
		infra["total_ctos"] = totalCtos
		infra["cto_status"] = buckets

		// CTOs por tipo de splitter (campo splitter, ex. "1x8", "1x16").
		var bySplitter []map[string]any
		spRows, err := pool.Query(gctx, `
			SELECT COALESCE(NULLIF(trim(splitter), ''), '(não definido)'), COUNT(*)::bigint
			FROM network_ctos GROUP BY 1 ORDER BY 2 DESC, 1`)
		if err == nil {
			for spRows.Next() {
				var name string
				var n int64
				if spRows.Scan(&name, &n) == nil {
					bySplitter = append(bySplitter, map[string]any{"splitter": name, "count": n})
				}
			}
			spRows.Close()
		}
		infra["ctos_by_splitter"] = bySplitter

		// Caixas de emenda: por modelo (emenda = fusões; distribuicao = splitter interno).
		var emendas, distribuicoes int64
		_ = pool.QueryRow(gctx, `
			SELECT
				COUNT(*) FILTER (WHERE box_model = 'emenda')::bigint,
				COUNT(*) FILTER (WHERE box_model = 'distribuicao')::bigint
			FROM network_splice_boxes`).Scan(&emendas, &distribuicoes)
		infra["splice_boxes"] = map[string]any{"emenda": emendas, "distribuicao": distribuicoes, "total": emendas + distribuicoes}

		// Totais por projeto.
		var byProject []map[string]any
		pRows, err := pool.Query(gctx, `
			SELECT p.id::text, p.description, p.status,
				(SELECT COUNT(*) FROM network_ctos c WHERE c.project_id = p.id)::bigint,
				(SELECT COUNT(*) FROM network_splice_boxes s WHERE s.project_id = p.id AND s.box_model = 'emenda')::bigint,
				(SELECT COUNT(*) FROM network_splice_boxes s WHERE s.project_id = p.id AND s.box_model = 'distribuicao')::bigint,
				(SELECT COUNT(*) FROM network_cables cb WHERE cb.project_id = p.id)::bigint,
				(SELECT COUNT(*) FROM network_poles pl WHERE pl.project_id = p.id)::bigint
			FROM network_projects p
			ORDER BY p.description`)
		if err == nil {
			for pRows.Next() {
				var id, desc, status string
				var ctos, em, dist, cables, poles int64
				if pRows.Scan(&id, &desc, &status, &ctos, &em, &dist, &cables, &poles) == nil {
					byProject = append(byProject, map[string]any{
						"project_id": id, "description": desc, "status": status,
						"ctos": ctos, "emendas": em, "distribuicoes": dist, "cables": cables, "poles": poles,
					})
				}
			}
			pRows.Close()
		}
		infra["by_project"] = byProject

		var totalCables, totalPoles, totalProjects int64
		_ = pool.QueryRow(gctx, `SELECT
			(SELECT COUNT(*) FROM network_cables),
			(SELECT COUNT(*) FROM network_poles),
			(SELECT COUNT(*) FROM network_projects)`).Scan(&totalCables, &totalPoles, &totalProjects)
		infra["totals"] = map[string]any{
			"cables": totalCables, "poles": totalPoles, "projects": totalProjects,
			"ctos": totalCtos, "splice_boxes": emendas + distribuicoes,
		}

		set("infra_overview", infra)
		return nil
	})

	if err := g.Wait(); err != nil {
		return nil, err
	}
	return out, nil
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
