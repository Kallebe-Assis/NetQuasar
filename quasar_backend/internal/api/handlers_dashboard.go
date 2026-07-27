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
				AVG(ph.latency_ms)::float8 FILTER (WHERE ph.ok AND ph.latency_ms IS NOT NULL)
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
