package api

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/reporttelegram"
	"github.com/netquasar/netquasar/quasar_backend/internal/telegramclient"
)

var systemReportCatalog = []map[string]string{
	{"id": "active-alerts", "title": "Alertas ativos", "description": "Lista detalhada de todos os alertas em aberto."},
	{"id": "connections", "title": "Conexões de clientes", "description": "Quantidade e detalhes das conexões cadastradas."},
	{"id": "equipment-by-pop", "title": "Equipamentos por POP", "description": "Distribuição de equipamentos por ponto de presença e categoria."},
	{"id": "olt-overview", "title": "OLTs — informações e gráfico", "description": "Frota OLT, ONUs e evolução recente (últimos 7 dias)."},
	{"id": "system-general", "title": "Visão geral do sistema", "description": "Métricas consolidadas de equipamentos, localidades, clientes, PONs, Mikrotik e mais."},
	{"id": "integrations", "title": "Integrações", "description": "Integrações configuradas e estado de cada uma."},
	{"id": "attention-devices", "title": "Equipamentos precisando de atenção", "description": "Lacunas de cadastro e equipamentos com alertas abertos."},
	{"id": "alerts-by-category", "title": "Alertas por categoria", "description": "Alertas ativos agrupados por categoria operacional."},
	{"id": "onu-per-pon", "title": "ONUs por PON", "description": "Última coleta por porta PON (sem nova coleta SNMP)."},
}

func systemReportIDValid(id string) bool {
	for _, r := range systemReportCatalog {
		if r["id"] == id {
			return true
		}
	}
	return false
}

func (s *Server) systemReportsCatalog(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"reports": systemReportCatalog})
}

func (s *Server) systemReportData(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if !systemReportIDValid(id) {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "relatório desconhecido", nil)
		return
	}
	payload, err := s.buildSystemReport(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) systemReportCSV(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if !systemReportIDValid(id) {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "relatório desconhecido", nil)
		return
	}
	payload, err := s.buildSystemReport(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	cols, _ := payload["columns"].([]string)
	rows, _ := payload["rows"].([][]string)
	if cols == nil {
		cols = []string{}
	}
	if rows == nil {
		rows = [][]string{}
	}
	fname := fmt.Sprintf("relatorio_%s_%s.csv", id, time.Now().UTC().Format("20060102_150405"))
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, fname))
	cw := csv.NewWriter(w)
	_ = cw.Write(cols)
	for _, row := range rows {
		_ = cw.Write(row)
	}
	cw.Flush()
}

func (s *Server) systemReportTelegram(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if !systemReportIDValid(id) {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "relatório desconhecido", nil)
		return
	}
	cfg, err := telegramclient.LoadConfig(r.Context(), s.DB(), "reports")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if !cfg.Ready() {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "Telegram de relatórios não configurado (bot_token/chat_id).", nil)
		return
	}
	payload, err := s.buildSystemReport(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	title, _ := payload["title"].(string)
	text := reporttelegram.ComposeSystemReport(title, payload)
	if err := telegramclient.SendMessage(r.Context(), cfg, text); err != nil {
		writeErr(w, http.StatusBadGateway, "TELEGRAM_SEND_FAILED", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "system_report", id, "telegram_send", s.actorFromRequest(r), nil, map[string]any{"report_id": id})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "report_id": id})
}

func (s *Server) buildSystemReport(ctx context.Context, id string) (map[string]any, error) {
	pool := s.DB()
	if pool == nil {
		return nil, fmt.Errorf("base indisponível")
	}
	now := time.Now().UTC()
	base := map[string]any{
		"report_id":    id,
		"generated_at": now.Format(time.RFC3339),
	}
	switch id {
	case "active-alerts":
		return s.reportActiveAlerts(ctx, pool, base)
	case "connections":
		return s.reportConnections(ctx, pool, base)
	case "equipment-by-pop":
		return s.reportEquipmentByPop(ctx, pool, base)
	case "olt-overview":
		return s.reportOltOverview(ctx, pool, base)
	case "system-general":
		return s.reportSystemGeneral(ctx, pool, base)
	case "integrations":
		return s.reportIntegrations(ctx, pool, base)
	case "attention-devices":
		return s.reportAttentionDevices(ctx, pool, base)
	case "alerts-by-category":
		return s.reportAlertsByCategory(ctx, pool, base)
	case "onu-per-pon":
		return s.reportOnuPerPon(ctx, pool, base)
	default:
		return nil, fmt.Errorf("relatório desconhecido")
	}
}

func alertCategoryLabelGo(alertType string) string {
	switch strings.TrimSpace(alertType) {
	case "ping_unreachable", "uptime_restart_low", "snmp_failure", "telemetry_threshold":
		return "Equipamento"
	case "interface_down", "interface_down_transition":
		return "Interface"
	case "olt_onu_drop", "olt_onu_rise", "pon_down":
		return "OLT / PON"
	case "mikrotik_sfp_tx", "mikrotik_sfp_rx":
		return "Óptica / SFP"
	case "latency_high", "latency_degraded", "cpu_high", "memory_high", "temperature_high", "temperature_low":
		return "Performance"
	default:
		return "Sistema"
	}
}

func severityLabelGo(sev string) string {
	switch strings.ToLower(strings.TrimSpace(sev)) {
	case "critical":
		return "Crítico"
	case "warning":
		return "Aviso"
	case "info":
		return "Info"
	default:
		return sev
	}
}

func gapLabelGo(flag string) string {
	switch flag {
	case "without_locality":
		return "Sem localidade"
	case "without_ip":
		return "Sem IP"
	case "without_snmp_community":
		return "Sem comunidade SNMP"
	case "without_coordinates":
		return "Sem coordenadas"
	case "without_telemetry":
		return "Telemetria desativada"
	default:
		return flag
	}
}

func (s *Server) reportActiveAlerts(ctx context.Context, pool *pgxpool.Pool, base map[string]any) (map[string]any, error) {
	rows, err := pool.Query(ctx, `
		SELECT a.id::text, COALESCE(NULLIF(trim(a.device_name), ''), NULLIF(trim(d.description), ''), '—'),
			COALESCE(NULLIF(trim(a.ip), ''), '—'), a.severity, a.alert_type, a.message,
			a.active_since, a.meta::text,
			COALESCE(NULLIF(trim(d.category), ''), '—'),
			COALESCE(NULLIF(trim(p.description), ''), '—')
		FROM alert_instances a
		LEFT JOIN devices d ON d.id = a.device_id
		LEFT JOIN pops p ON p.id = d.pop_id
		WHERE a.closed_at IS NULL
		ORDER BY
			CASE a.severity WHEN 'critical' THEN 0 WHEN 'warning' THEN 1 ELSE 2 END,
			a.active_since DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols := []string{"ID", "Equipamento", "IP", "Severidade", "Tipo", "Categoria", "POP", "Desde", "Mensagem"}
	var dataRows [][]string
	var crit, warn, info int
	for rows.Next() {
		var id, dev, ip, sev, typ, msg, cat, pop string
		var since time.Time
		var meta []byte
		if err := rows.Scan(&id, &dev, &ip, &sev, &typ, &msg, &since, &meta, &cat, &pop); err != nil {
			return nil, err
		}
		switch sev {
		case "critical":
			crit++
		case "warning":
			warn++
		default:
			info++
		}
		catLbl := alertCategoryLabelGo(typ)
		dataRows = append(dataRows, []string{
			id, dev, ip, severityLabelGo(sev), typ, catLbl, pop,
			since.UTC().Format(time.RFC3339), strings.TrimSpace(msg),
		})
	}
	base["title"] = "Alertas ativos"
	base["description"] = "Todos os alertas em aberto com contexto operacional."
	base["columns"] = cols
	base["rows"] = dataRows
	base["summary"] = map[string]any{
		"Total":    len(dataRows),
		"Críticos": crit,
		"Avisos":   warn,
		"Info":     info,
	}
	return base, nil
}

func (s *Server) reportConnections(ctx context.Context, pool *pgxpool.Pool, base map[string]any) (map[string]any, error) {
	var total int64
	_ = pool.QueryRow(ctx, `SELECT COUNT(*) FROM client_connections`).Scan(&total)
	rows, err := pool.Query(ctx, `
		SELECT COALESCE(display_number::text, ''), client_name, login, COALESCE(ip_address, ''),
			connection_kind, COALESCE(medium_type, ''), COALESCE(sales_plan, ''), created_at
		FROM client_connections
		ORDER BY client_name, login
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols := []string{"Nº", "Cliente", "Login", "IP", "Tipo", "Meio", "Plano", "Cadastro"}
	var dataRows [][]string
	byKind := map[string]int{}
	byMedium := map[string]int{}
	for rows.Next() {
		var num, name, login, ip, kind, medium, plan string
		var created time.Time
		if err := rows.Scan(&num, &name, &login, &ip, &kind, &medium, &plan, &created); err != nil {
			return nil, err
		}
		byKind[kind]++
		if medium != "" {
			byMedium[medium]++
		}
		dataRows = append(dataRows, []string{
			num, name, login, ip, kind, medium, plan, created.UTC().Format(time.RFC3339),
		})
	}
	base["title"] = "Conexões de clientes"
	base["description"] = "Inventário de conexões cadastradas."
	base["columns"] = cols
	base["rows"] = dataRows
	base["summary"] = map[string]any{
		"Total conexões": total,
		"Por tipo":       byKind,
		"Por meio":       byMedium,
	}
	return base, nil
}

func (s *Server) reportEquipmentByPop(ctx context.Context, pool *pgxpool.Pool, base map[string]any) (map[string]any, error) {
	rows, err := pool.Query(ctx, `
		SELECT COALESCE(p.description, '(sem POP)'), COALESCE(NULLIF(trim(d.category), ''), '(sem categoria)'), COUNT(*)::bigint
		FROM devices d
		LEFT JOIN pops p ON p.id = d.pop_id
		GROUP BY 1, 2
		ORDER BY 1, 3 DESC, 2
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols := []string{"POP", "Categoria", "Quantidade"}
	var dataRows [][]string
	popTotals := map[string]int64{}
	for rows.Next() {
		var pop, cat string
		var n int64
		if err := rows.Scan(&pop, &cat, &n); err != nil {
			return nil, err
		}
		popTotals[pop] += n
		dataRows = append(dataRows, []string{pop, cat, strconv.FormatInt(n, 10)})
	}
	var popCount, devCount int64
	_ = pool.QueryRow(ctx, `SELECT COUNT(*) FROM pops`).Scan(&popCount)
	_ = pool.QueryRow(ctx, `SELECT COUNT(*) FROM devices`).Scan(&devCount)
	base["title"] = "Equipamentos por POP"
	base["description"] = "Distribuição por ponto de presença e categoria."
	base["columns"] = cols
	base["rows"] = dataRows
	base["summary"] = map[string]any{
		"POPs":                 popCount,
		"Equipamentos (total)": devCount,
		"Por POP":              popTotals,
	}
	return base, nil
}

func (s *Server) reportOltOverview(ctx context.Context, pool *pgxpool.Pool, base map[string]any) (map[string]any, error) {
	rows, err := pool.Query(ctx, `
		SELECT d.id::text, d.description, COALESCE(d.brand, ''), o.updated_at,
			COALESCE((
				SELECT SUM(COALESCE((NULLIF(trim(e->>'onu_total'), ''))::bigint, 0))
				FROM jsonb_array_elements(CASE WHEN jsonb_typeof(o.pons) = 'array' THEN o.pons ELSE '[]'::jsonb END) e
			), 0)::bigint,
			COALESCE((
				SELECT SUM(COALESCE((NULLIF(trim(e->>'onu_online'), ''))::bigint, 0))
				FROM jsonb_array_elements(CASE WHEN jsonb_typeof(o.pons) = 'array' THEN o.pons ELSE '[]'::jsonb END) e
			), 0)::bigint,
			COALESCE((
				SELECT SUM(COALESCE((NULLIF(trim(e->>'onu_offline'), ''))::bigint, 0))
				FROM jsonb_array_elements(CASE WHEN jsonb_typeof(o.pons) = 'array' THEN o.pons ELSE '[]'::jsonb END) e
			), 0)::bigint,
			jsonb_array_length(CASE WHEN jsonb_typeof(o.pons) = 'array' THEN o.pons ELSE '[]'::jsonb END)
		FROM devices d
		JOIN olt_snapshots o ON o.device_id = d.id
		WHERE lower(trim(d.category)) = 'olt'
		ORDER BY d.description
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols := []string{"OLT", "Marca", "PONs", "ONUs total", "Online", "Offline", "Snapshot"}
	var dataRows [][]string
	var fleetTotal, fleetOn, fleetOff, ponPorts int64
	for rows.Next() {
		var id, desc, brand string
		var upd time.Time
		var total, on, off, pons int64
		if err := rows.Scan(&id, &desc, &brand, &upd, &total, &on, &off, &pons); err != nil {
			return nil, err
		}
		_ = id
		fleetTotal += total
		fleetOn += on
		fleetOff += off
		ponPorts += pons
		dataRows = append(dataRows, []string{
			desc, brand, strconv.FormatInt(pons, 10),
			strconv.FormatInt(total, 10), strconv.FormatInt(on, 10), strconv.FormatInt(off, 10),
			upd.UTC().Format(time.RFC3339),
		})
	}

	since := time.Now().UTC().Add(-7 * 24 * time.Hour)
	chartPts := []map[string]any{}
	dayRows, err := pool.Query(ctx, `
		SELECT (updated_at AT TIME ZONE 'UTC')::date::text AS day,
			COALESCE(SUM((
				SELECT SUM(COALESCE((NULLIF(trim(e->>'onu_total'),''))::bigint,0))
				FROM jsonb_array_elements(CASE WHEN jsonb_typeof(pons)='array' THEN pons ELSE '[]'::jsonb END) e
			)),0)::bigint AS onu_total,
			COALESCE(SUM((
				SELECT SUM(COALESCE((NULLIF(trim(e->>'onu_online'),''))::bigint,0))
				FROM jsonb_array_elements(CASE WHEN jsonb_typeof(pons)='array' THEN pons ELSE '[]'::jsonb END) e
			)),0)::bigint AS onu_online,
			COALESCE(SUM((
				SELECT SUM(COALESCE((NULLIF(trim(e->>'onu_offline'),''))::bigint,0))
				FROM jsonb_array_elements(CASE WHEN jsonb_typeof(pons)='array' THEN pons ELSE '[]'::jsonb END) e
			)),0)::bigint AS onu_offline
		FROM olt_snapshots
		WHERE updated_at >= $1
		GROUP BY 1 ORDER BY 1
	`, since)
	if err == nil {
		defer dayRows.Close()
		for dayRows.Next() {
			var day string
			var t, on, off int64
			if dayRows.Scan(&day, &t, &on, &off) == nil {
				chartPts = append(chartPts, map[string]any{
					"t": day, "total": t, "online": on, "offline": off,
				})
			}
		}
	}

	base["title"] = "OLTs — informações e gráfico"
	base["description"] = "Estado actual da frota OLT e tendência de snapshots (7 dias)."
	base["columns"] = cols
	base["rows"] = dataRows
	base["summary"] = map[string]any{
		"OLTs com snapshot": len(dataRows),
		"Portas PON":        ponPorts,
		"ONUs (frota)":      fleetTotal,
		"Online":            fleetOn,
		"Offline":           fleetOff,
	}
	base["chart"] = map[string]any{"points": chartPts, "label": "ONUs por dia (snapshots)"}
	return base, nil
}

func (s *Server) reportSystemGeneral(ctx context.Context, pool *pgxpool.Pool, base map[string]any) (map[string]any, error) {
	summary := map[string]any{}
	metrics := []struct {
		key string
		q   string
	}{
		{"Equipamentos", `SELECT COUNT(*) FROM devices`},
		{"POPs", `SELECT COUNT(*) FROM pops`},
		{"Localidades comerciais", `SELECT COUNT(*) FROM commercial_localities`},
		{"Conexões de clientes", `SELECT COUNT(*) FROM client_connections`},
		{"Integrações", `SELECT COUNT(*) FROM integrations`},
		{"Alertas abertos", `SELECT COUNT(*) FROM alert_instances WHERE closed_at IS NULL`},
		{"Incidentes abertos", `SELECT COUNT(*) FROM alert_incidents WHERE status = 'open'`},
		{"Janelas de manutenção activas", `SELECT COUNT(*) FROM maintenance_windows WHERE now() BETWEEN starts_at AND ends_at AND status IN ('planned','running')`},
		{"Utilizadores", `SELECT COUNT(*) FROM users`},
		{"Regras de alerta", `SELECT COUNT(*) FROM alert_rules`},
		{"Telemetria activa (equip.)", `SELECT COUNT(*) FROM devices WHERE telemetry_enabled = true`},
		{"Ping activo (equip.)", `SELECT COUNT(*) FROM devices WHERE ping_enabled = true`},
		{"OLTs", `SELECT COUNT(*) FROM devices WHERE lower(trim(category)) = 'olt'`},
		{"Mikrotik", `SELECT COUNT(*) FROM devices WHERE lower(trim(category)) LIKE '%mikrotik%' OR lower(coalesce(brand,'')) LIKE '%mikrotik%'`},
		{"Snapshots OLT", `SELECT COUNT(*) FROM olt_snapshots`},
		{"Amostras telemetria (30d)", `SELECT COUNT(*) FROM telemetry_samples WHERE collected_at >= now() - interval '30 days'`},
		{"Amostras ping (30d)", `SELECT COUNT(*) FROM ping_history WHERE checked_at >= now() - interval '30 days'`},
	}
	for _, m := range metrics {
		var n int64
		if pool.QueryRow(ctx, m.q).Scan(&n) == nil {
			summary[m.key] = n
		}
	}
	var clientsMonth int64
	_ = pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(client_count), 0)::bigint FROM commercial_monthly_records
		WHERE year_month = to_char((CURRENT_TIMESTAMP AT TIME ZONE 'UTC'), 'YYYY-MM')
	`).Scan(&clientsMonth)
	summary["Clientes (mês actual)"] = clientsMonth

	var ponPorts, onuTotal, onuOn, onuOff int64
	_ = pool.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(jsonb_array_length(CASE WHEN jsonb_typeof(pons)='array' THEN pons ELSE '[]'::jsonb END)), 0),
			COALESCE(SUM((
				SELECT SUM(COALESCE((NULLIF(trim(e->>'onu_total'),''))::bigint,0))
				FROM jsonb_array_elements(CASE WHEN jsonb_typeof(pons)='array' THEN pons ELSE '[]'::jsonb END) e
			)), 0),
			COALESCE(SUM((
				SELECT SUM(COALESCE((NULLIF(trim(e->>'onu_online'),''))::bigint,0))
				FROM jsonb_array_elements(CASE WHEN jsonb_typeof(pons)='array' THEN pons ELSE '[]'::jsonb END) e
			)), 0),
			COALESCE(SUM((
				SELECT SUM(COALESCE((NULLIF(trim(e->>'onu_offline'),''))::bigint,0))
				FROM jsonb_array_elements(CASE WHEN jsonb_typeof(pons)='array' THEN pons ELSE '[]'::jsonb END) e
			)), 0)
		FROM olt_snapshots
	`).Scan(&ponPorts, &onuTotal, &onuOn, &onuOff)
	summary["Portas PON (snapshots)"] = ponPorts
	summary["ONUs total (snapshots)"] = onuTotal
	summary["ONUs online (snapshots)"] = onuOn
	summary["ONUs offline (snapshots)"] = onuOff

	var monRunning bool
	_ = pool.QueryRow(ctx, `SELECT is_running FROM monitoring_runtime WHERE id=1`).Scan(&monRunning)
	summary["Monitoramento activo"] = monRunning

	byCat := map[string]int64{}
	if rows, err := pool.Query(ctx, `
		SELECT COALESCE(NULLIF(trim(category), ''), '(sem categoria)'), COUNT(*)::bigint
		FROM devices GROUP BY 1 ORDER BY 2 DESC`); err == nil {
		defer rows.Close()
		for rows.Next() {
			var c string
			var n int64
			if rows.Scan(&c, &n) == nil {
				byCat[c] = n
			}
		}
	}
	summary["Equipamentos por categoria"] = byCat

	cols := []string{"Métrica", "Valor"}
	var dataRows [][]string
	keys := make([]string, 0, len(summary))
	for k := range summary {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		dataRows = append(dataRows, []string{k, fmt.Sprintf("%v", summary[k])})
	}

	base["title"] = "Visão geral do sistema"
	base["description"] = "Métricas consolidadas da base de dados."
	base["columns"] = cols
	base["rows"] = dataRows
	base["summary"] = summary
	return base, nil
}

func (s *Server) reportIntegrations(ctx context.Context, pool *pgxpool.Pool, base map[string]any) (map[string]any, error) {
	rows, err := pool.Query(ctx, `
		SELECT name, slug, COALESCE(description, ''), COALESCE(base_url, ''),
			auth_type, enabled, last_test_ok, COALESCE(last_test_message, ''), last_test_at, updated_at
		FROM integrations
		ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols := []string{"Nome", "Slug", "URL", "Auth", "Activa", "Último teste", "Resultado", "Mensagem", "Actualizado"}
	var dataRows [][]string
	var enabled, disabled int
	for rows.Next() {
		var name, slug, desc, url, auth string
		var en bool
		var testOk *bool
		var testMsg string
		var testAt, upd *time.Time
		if err := rows.Scan(&name, &slug, &desc, &url, &auth, &en, &testOk, &testMsg, &testAt, &upd); err != nil {
			return nil, err
		}
		if en {
			enabled++
		} else {
			disabled++
		}
		testLbl := "—"
		if testOk != nil {
			if *testOk {
				testLbl = "OK"
			} else {
				testLbl = "Falhou"
			}
		}
		testAtStr := "—"
		if testAt != nil {
			testAtStr = testAt.UTC().Format(time.RFC3339)
		}
		updStr := "—"
		if upd != nil {
			updStr = upd.UTC().Format(time.RFC3339)
		}
		dataRows = append(dataRows, []string{
			name, slug, url, auth, strconv.FormatBool(en), testLbl, testAtStr, testMsg, updStr,
		})
		_ = desc
	}
	base["title"] = "Integrações"
	base["description"] = "Integrações configuradas no sistema."
	base["columns"] = cols
	base["rows"] = dataRows
	base["summary"] = map[string]any{
		"Total":      len(dataRows),
		"Activas":    enabled,
		"Inactivas":  disabled,
	}
	return base, nil
}

func (s *Server) reportAttentionDevices(ctx context.Context, pool *pgxpool.Pool, base map[string]any) (map[string]any, error) {
	cols := []string{"Equipamento", "Categoria", "IP", "Motivo", "Detalhe"}
	var dataRows [][]string
	seen := map[string]bool{}

	gapRows, err := pool.Query(ctx, `
		SELECT id::text, description, category, host(ip)::text, locality_id::text, snmp_community, latitude, longitude, telemetry_enabled
		FROM devices ORDER BY description
	`)
	if err != nil {
		return nil, err
	}
	defer gapRows.Close()
	gapCount := 0
	for gapRows.Next() {
		var id, desc, cat string
		var ip, localityID, comm *string
		var lat, lon *float64
		var tel bool
		if err := gapRows.Scan(&id, &desc, &cat, &ip, &localityID, &comm, &lat, &lon, &tel); err != nil {
			return nil, err
		}
		flags := []string{}
		if localityID == nil {
			flags = append(flags, "without_locality")
		}
		if ip == nil || strings.TrimSpace(*ip) == "" {
			flags = append(flags, "without_ip")
		}
		if comm == nil || strings.TrimSpace(*comm) == "" {
			flags = append(flags, "without_snmp_community")
		}
		if lat == nil || lon == nil {
			flags = append(flags, "without_coordinates")
		}
		if !tel {
			flags = append(flags, "without_telemetry")
		}
		if len(flags) == 0 {
			continue
		}
		gapCount++
		ipStr := "—"
		if ip != nil {
			ipStr = *ip
		}
		lbls := make([]string, len(flags))
		for i, f := range flags {
			lbls[i] = gapLabelGo(f)
		}
		seen[id] = true
		dataRows = append(dataRows, []string{desc, cat, ipStr, "Lacuna de cadastro", strings.Join(lbls, "; ")})
	}

	alertRows, err := pool.Query(ctx, `
		SELECT DISTINCT d.id::text, COALESCE(NULLIF(trim(d.description), ''), '—'),
			COALESCE(NULLIF(trim(d.category), ''), '—'),
			COALESCE(host(d.ip)::text, '—'),
			a.severity, COUNT(*)::bigint
		FROM alert_instances a
		JOIN devices d ON d.id = a.device_id
		WHERE a.closed_at IS NULL
		GROUP BY d.id, d.description, d.category, d.ip, a.severity
		ORDER BY
			CASE a.severity WHEN 'critical' THEN 0 WHEN 'warning' THEN 1 ELSE 2 END,
			d.description
	`)
	if err != nil {
		return nil, err
	}
	defer alertRows.Close()
	alertDevCount := 0
	for alertRows.Next() {
		var id, desc, cat, ip, sev string
		var n int64
		if err := alertRows.Scan(&id, &desc, &cat, &ip, &sev, &n); err != nil {
			return nil, err
		}
		alertDevCount++
		detail := fmt.Sprintf("%d alerta(s) %s", n, severityLabelGo(sev))
		if seen[id] {
			for i := range dataRows {
				if dataRows[i][0] == desc {
					dataRows[i][4] = dataRows[i][4] + "; Alertas abertos"
					dataRows[i][5] = dataRows[i][5] + "; " + detail
					break
				}
			}
			continue
		}
		dataRows = append(dataRows, []string{desc, cat, ip, "Alertas abertos", detail})
	}

	base["title"] = "Equipamentos precisando de atenção"
	base["description"] = "Lacunas de cadastro e equipamentos com alertas em aberto."
	base["columns"] = cols
	base["rows"] = dataRows
	base["summary"] = map[string]any{
		"Com lacunas":        gapCount,
		"Com alertas":        alertDevCount,
		"Total listados":     len(dataRows),
	}
	return base, nil
}

func (s *Server) reportAlertsByCategory(ctx context.Context, pool *pgxpool.Pool, base map[string]any) (map[string]any, error) {
	rows, err := pool.Query(ctx, `
		SELECT alert_type, severity, COUNT(*)::bigint
		FROM alert_instances
		WHERE closed_at IS NULL
		GROUP BY alert_type, severity
		ORDER BY 3 DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols := []string{"Categoria", "Tipo", "Severidade", "Quantidade"}
	var dataRows [][]string
	byCat := map[string]int64{}
	var total int64
	for rows.Next() {
		var typ, sev string
		var n int64
		if err := rows.Scan(&typ, &sev, &n); err != nil {
			return nil, err
		}
		cat := alertCategoryLabelGo(typ)
		byCat[cat] += n
		total += n
		dataRows = append(dataRows, []string{cat, typ, severityLabelGo(sev), strconv.FormatInt(n, 10)})
	}
	base["title"] = "Alertas por categoria"
	base["description"] = "Alertas activos agrupados por categoria e tipo."
	base["columns"] = cols
	base["rows"] = dataRows
	base["summary"] = map[string]any{
		"Total alertas": total,
		"Por categoria": byCat,
	}
	return base, nil
}

func (s *Server) reportOnuPerPon(ctx context.Context, pool *pgxpool.Pool, base map[string]any) (map[string]any, error) {
	rows, err := pool.Query(ctx, `
		SELECT d.description, o.updated_at, o.pons::text
		FROM devices d
		JOIN olt_snapshots o ON o.device_id = d.id
		WHERE lower(trim(d.category)) = 'olt'
		ORDER BY d.description
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols := []string{"OLT", "PON", "Nome PON", "Total", "Online", "Offline", "Snapshot"}
	var dataRows [][]string
	var totalOnu, totalOn, totalOff int64
	ponCount := 0
	for rows.Next() {
		var olt string
		var upd time.Time
		var ponsRaw string
		if err := rows.Scan(&olt, &upd, &ponsRaw); err != nil {
			return nil, err
		}
		var pons []map[string]any
		_ = json.Unmarshal([]byte(ponsRaw), &pons)
		snap := upd.UTC().Format(time.RFC3339)
		for _, p := range pons {
			ponID := strings.TrimSpace(fmt.Sprint(p["id"]))
			ponName := strings.TrimSpace(fmt.Sprint(p["name"]))
			total := toInt(p["onu_total"])
			on := toInt(p["onu_online"])
			off := toInt(p["onu_offline"])
			ponCount++
			totalOnu += int64(total)
			totalOn += int64(on)
			totalOff += int64(off)
			dataRows = append(dataRows, []string{
				olt, ponID, ponName,
				strconv.Itoa(total), strconv.Itoa(on), strconv.Itoa(off), snap,
			})
		}
	}
	base["title"] = "ONUs por PON"
	base["description"] = "Última coleta armazenada em olt_snapshots (sem nova coleta SNMP)."
	base["columns"] = cols
	base["rows"] = dataRows
	base["summary"] = map[string]any{
		"Portas PON":   ponCount,
		"ONUs total":   totalOnu,
		"Online":       totalOn,
		"Offline":      totalOff,
	}
	return base, nil
}
