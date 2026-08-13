package api

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/networkevents"
)

func reportPeriodWhere(from, to, col string, startArg int) (string, []any) {
	var parts []string
	var args []any
	n := startArg
	if strings.TrimSpace(from) != "" {
		parts = append(parts, fmt.Sprintf("%s >= $%d::timestamptz", col, n))
		args = append(args, strings.TrimSpace(from))
		n++
	}
	if strings.TrimSpace(to) != "" {
		parts = append(parts, fmt.Sprintf("%s < $%d::timestamptz + interval '1 day'", col, n))
		args = append(args, strings.TrimSpace(to))
	}
	if len(parts) == 0 {
		return "", nil
	}
	return " AND " + strings.Join(parts, " AND "), args
}

func (s *Server) reportNetworkEvents(ctx context.Context, pool *pgxpool.Pool, base map[string]any, opts periodModeReportOptions) (map[string]any, error) {
	mode := normalizePeriodMode(opts.Mode)
	where, args := reportPeriodWhere(opts.From, opts.To, "e.occurred_at", 1)
	base["title"] = "Eventos de rede"
	base["description"] = "Manutenções, alterações e rompimentos registados na tela Eventos."
	base["options"] = map[string]any{"mode": mode, "from": opts.From, "to": opts.To}

	summary := map[string]any{}
	var total, monthN, incidentN int
	_ = pool.QueryRow(ctx, `SELECT COUNT(*) FROM network_events e WHERE 1=1`+where, args...).Scan(&total)
	_ = pool.QueryRow(ctx, `SELECT COUNT(*) FROM network_events WHERE occurred_at >= date_trunc('month', now())`).Scan(&monthN)
	_ = pool.QueryRow(ctx, `SELECT COUNT(*) FROM network_events e WHERE e.category_code = 'incident'`+where, args...).Scan(&incidentN)
	summary["Total"] = total
	summary["Este mês"] = monthN
	summary["Rompimentos / incidentes"] = incidentN

	type pair struct{ code, label string; n int }
	byCat := []pair{}
	if rows, err := pool.Query(ctx, `
		SELECT e.category_code, COUNT(*) FROM network_events e WHERE 1=1`+where+`
		GROUP BY 1 ORDER BY 2 DESC`, args...); err == nil {
		defer rows.Close()
		for rows.Next() {
			var p pair
			if rows.Scan(&p.code, &p.n) == nil {
				if c := networkevents.CategoryByCode(p.code); c != nil {
					p.label = c.Label
				} else {
					p.label = p.code
				}
				byCat = append(byCat, p)
			}
		}
	}
	for _, p := range byCat {
		summary["Cat. "+p.label] = p.n
	}

	if mode == "summary" {
		cols := []string{"Dimensão", "Código", "Quantidade"}
		var data [][]string
		for _, p := range byCat {
			data = append(data, []string{"Categoria", p.label, strconv.Itoa(p.n)})
		}
		if rows, err := pool.Query(ctx, `
			SELECT e.impact, COUNT(*) FROM network_events e WHERE 1=1`+where+`
			GROUP BY 1 ORDER BY 2 DESC`, args...); err == nil {
			defer rows.Close()
			for rows.Next() {
				var code string
				var n int
				if rows.Scan(&code, &n) == nil {
					data = append(data, []string{"Impacto", impactLabel(code), strconv.Itoa(n)})
				}
			}
		}
		if rows, err := pool.Query(ctx, `
			SELECT e.type_code, COUNT(*) FROM network_events e WHERE 1=1`+where+`
			GROUP BY 1 ORDER BY 2 DESC LIMIT 40`, args...); err == nil {
			defer rows.Close()
			for rows.Next() {
				var code string
				var n int
				if rows.Scan(&code, &n) == nil {
					label := code
					if t := networkevents.TypeByCode(code); t != nil {
						label = t.Label
					}
					data = append(data, []string{"Tipo", label, strconv.Itoa(n)})
				}
			}
		}
		base["columns"] = cols
		base["rows"] = data
		base["summary"] = summary
		return base, nil
	}

	q := `
		SELECT e.occurred_at, e.category_code, e.type_code, e.impact, e.notes,
			p.description, d.description,
			COALESCE(NULLIF(trim(tu.display_name),''), tu.email),
			np.description, cto.description,
			COALESCE(NULLIF(trim(cab.description),''), 'Cabo #'||cab.display_number::text),
			sp.description,
			COALESCE(NULLIF(trim(pol.description),''), 'Poste #'||pol.display_number::text),
			e.interface_name, e.vlan
		FROM network_events e
		LEFT JOIN pops p ON p.id = e.pop_id
		LEFT JOIN devices d ON d.id = e.device_id
		LEFT JOIN users tu ON tu.id = e.technician_id
		LEFT JOIN network_projects np ON np.id = e.project_id
		LEFT JOIN network_ctos cto ON cto.id = e.cto_id
		LEFT JOIN network_cables cab ON cab.id = e.cable_id
		LEFT JOIN network_splice_boxes sp ON sp.id = e.splice_box_id
		LEFT JOIN network_poles pol ON pol.id = e.pole_id
		WHERE 1=1` + where + `
		ORDER BY e.occurred_at DESC
		LIMIT 2000`
	rows, err := pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols := []string{"Data", "Tipo", "Categoria", "Impacto", "POP", "Equipamento", "Projeto", "CTO", "Cabo", "Emenda", "Poste", "Interface", "VLAN", "Técnico", "Notas"}
	var data [][]string
	for rows.Next() {
		var at time.Time
		var cat, typ, impact string
		var notes, pop, dev, tech, proj, cto, cable, splice, pole, iface, vlan *string
		if err := rows.Scan(&at, &cat, &typ, &impact, &notes, &pop, &dev, &tech, &proj, &cto, &cable, &splice, &pole, &iface, &vlan); err != nil {
			return nil, err
		}
		catLabel, typeLabel := cat, typ
		if c := networkevents.CategoryByCode(cat); c != nil {
			catLabel = c.Label
		}
		if t := networkevents.TypeByCode(typ); t != nil {
			typeLabel = t.Label
		}
		data = append(data, []string{
			at.In(time.Local).Format("2006-01-02 15:04"),
			typeLabel, catLabel, impactLabel(impact),
			netevText(pop), netevText(dev), netevText(proj), netevText(cto), netevText(cable),
			netevText(splice), netevText(pole), netevText(iface), netevText(vlan), netevText(tech), netevText(notes),
		})
	}
	base["columns"] = cols
	base["rows"] = data
	base["summary"] = summary
	return base, nil
}

func (s *Server) reportFtthInfra(ctx context.Context, pool *pgxpool.Pool, base map[string]any, opts periodModeReportOptions) (map[string]any, error) {
	mode := normalizePeriodMode(opts.Mode)
	base["title"] = "Infraestrutura FTTH"
	base["description"] = "POPs, projetos, CTOs, cabos, postes e caixas de emenda."
	base["options"] = map[string]any{"mode": mode}

	count := func(q string) int {
		var n int
		_ = pool.QueryRow(ctx, q).Scan(&n)
		return n
	}
	summary := map[string]any{
		"POPs":              count(`SELECT COUNT(*) FROM pops`),
		"Projetos":          count(`SELECT COUNT(*) FROM network_projects WHERE COALESCE(status,'') <> 'inativo'`),
		"CTOs":              count(`SELECT COUNT(*) FROM network_ctos`),
		"CTOs em manutenção": count(`SELECT COUNT(*) FROM network_ctos WHERE needs_maintenance`),
		"Cabos":             count(`SELECT COUNT(*) FROM network_cables`),
		"Postes":            count(`SELECT COUNT(*) FROM network_poles`),
		"Caixas de emenda":  count(`SELECT COUNT(*) FROM network_splice_boxes`),
		"Emendas em manutenção": count(`SELECT COUNT(*) FROM network_splice_boxes WHERE needs_maintenance`),
	}

	if mode == "summary" {
		if rows, err := pool.Query(ctx, `
			SELECT COALESCE(NULLIF(trim(np.description),''), '(sem projeto)'), COUNT(*)
			FROM network_ctos c
			LEFT JOIN network_projects np ON np.id = c.project_id
			GROUP BY 1 ORDER BY 2 DESC`); err == nil {
			defer rows.Close()
			i := 0
			for rows.Next() {
				var name string
				var n int
				if rows.Scan(&name, &n) == nil {
					i++
					summary[fmt.Sprintf("CTOs · %s", name)] = n
				}
				if i >= 20 {
					break
				}
			}
		}
		cols := []string{"Métrica", "Valor"}
		var data [][]string
		keys := make([]string, 0, len(summary))
		for k := range summary {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			data = append(data, []string{k, fmt.Sprintf("%v", summary[k])})
		}
		base["columns"] = cols
		base["rows"] = data
		base["summary"] = summary
		return base, nil
	}

	cols := []string{"Tipo", "Nº", "Descrição", "Projeto", "Extra"}
	var data [][]string
	appendRows := func(q string) error {
		rows, err := pool.Query(ctx, q)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var kind, num, desc, proj, extra string
			if err := rows.Scan(&kind, &num, &desc, &proj, &extra); err != nil {
				return err
			}
			data = append(data, []string{kind, num, desc, proj, extra})
		}
		return nil
	}
	_ = appendRows(`
		SELECT 'POP', '', p.description, COALESCE(cl.name,''),
			TRIM(BOTH ' · ' FROM CONCAT_WS(' · ', NULLIF(p.address,''),
				CASE WHEN p.latitude IS NOT NULL AND p.longitude IS NOT NULL
					THEN round(p.latitude::numeric, 5)::text || ', ' || round(p.longitude::numeric, 5)::text
					ELSE NULL END))
		FROM pops p
		LEFT JOIN commercial_localities cl ON cl.id = p.locality_id
		ORDER BY p.description`)
	_ = appendRows(`
		SELECT 'Projeto', display_number::text, description, COALESCE(status,''), COALESCE(cl.name,'')
		FROM network_projects p
		LEFT JOIN commercial_localities cl ON cl.id = p.locality_id
		WHERE COALESCE(p.status,'') <> 'inativo'
		ORDER BY display_number`)
	_ = appendRows(`
		SELECT 'CTO', c.display_number::text, c.description, COALESCE(np.description,''),
			TRIM(BOTH ' · ' FROM CONCAT_WS(' · ',
				NULLIF(c.splitter,''),
				CASE WHEN c.needs_maintenance THEN 'manutenção' ELSE NULL END))
		FROM network_ctos c
		LEFT JOIN network_projects np ON np.id = c.project_id
		ORDER BY c.display_number`)
	_ = appendRows(`
		SELECT 'Cabo', c.display_number::text, COALESCE(NULLIF(trim(c.description),''), 'Cabo #'||c.display_number::text),
			COALESCE(np.description,''),
			TRIM(BOTH ' · ' FROM CONCAT_WS(' · ', NULLIF(c.cable_type,''), CASE WHEN c.fiber_count IS NOT NULL THEN c.fiber_count::text||' fibras' END, c.status))
		FROM network_cables c
		LEFT JOIN network_projects np ON np.id = c.project_id
		ORDER BY c.display_number`)
	_ = appendRows(`
		SELECT 'Poste', p.display_number::text, COALESCE(NULLIF(trim(p.description),''), 'Poste #'||p.display_number::text),
			COALESCE(np.description,''), COALESCE(p.pole_type,'')
		FROM network_poles p
		LEFT JOIN network_projects np ON np.id = p.project_id
		ORDER BY p.display_number`)
	_ = appendRows(`
		SELECT 'Emenda', s.display_number::text, s.description, COALESCE(np.description,''),
			TRIM(BOTH ' · ' FROM CONCAT_WS(' · ',
				CASE WHEN s.fiber_count IS NOT NULL THEN s.fiber_count::text||' fibras' END,
				CASE WHEN s.needs_maintenance THEN 'manutenção' ELSE NULL END))
		FROM network_splice_boxes s
		LEFT JOIN network_projects np ON np.id = s.project_id
		ORDER BY s.display_number`)

	base["columns"] = cols
	base["rows"] = data
	base["summary"] = summary
	return base, nil
}

func (s *Server) reportPonDown(ctx context.Context, pool *pgxpool.Pool, base map[string]any, opts periodModeReportOptions) (map[string]any, error) {
	mode := normalizePeriodMode(opts.Mode)
	base["title"] = "PONs inactivas"
	base["description"] = "Alertas pon_down em aberto e histórico no período."
	base["options"] = map[string]any{"mode": mode, "from": opts.From, "to": opts.To}

	var openN, oltN int
	_ = pool.QueryRow(ctx, `SELECT COUNT(*) FROM alert_instances WHERE closed_at IS NULL AND alert_type = 'pon_down'`).Scan(&openN)
	_ = pool.QueryRow(ctx, `SELECT COUNT(DISTINCT device_id) FROM alert_instances WHERE closed_at IS NULL AND alert_type = 'pon_down'`).Scan(&oltN)
	summary := map[string]any{"PONs DOWN abertas": openN, "OLTs afectadas": oltN}

	where, args := reportPeriodWhere(opts.From, opts.To, "a.active_since", 1)
	if mode == "summary" {
		cols := []string{"OLT", "PONs DOWN abertas"}
		var data [][]string
		if rows, err := pool.Query(ctx, `
			SELECT COALESCE(NULLIF(trim(a.device_name),''), d.description, a.ip, 'OLT'), COUNT(*)
			FROM alert_instances a
			LEFT JOIN devices d ON d.id = a.device_id
			WHERE a.closed_at IS NULL AND a.alert_type = 'pon_down'
			GROUP BY 1 ORDER BY 2 DESC`); err == nil {
			defer rows.Close()
			for rows.Next() {
				var name string
				var n int
				if rows.Scan(&name, &n) == nil {
					data = append(data, []string{name, strconv.Itoa(n)})
				}
			}
		}
		base["columns"] = cols
		base["rows"] = data
		base["summary"] = summary
		return base, nil
	}

	q := `
		SELECT COALESCE(NULLIF(trim(a.device_name),''), d.description, ''), COALESCE(a.ip,''),
			COALESCE(a.meta->>'pon', a.meta->>'key', ''), a.severity, a.message, a.active_since,
			CASE WHEN a.closed_at IS NULL THEN 'aberto' ELSE 'fechado' END
		FROM alert_instances a
		LEFT JOIN devices d ON d.id = a.device_id
		WHERE a.alert_type = 'pon_down'` + where + `
		ORDER BY a.closed_at NULLS FIRST, a.active_since DESC
		LIMIT 2000`
	rows, err := pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols := []string{"OLT", "IP", "PON", "Severidade", "Estado", "Desde", "Mensagem"}
	var data [][]string
	for rows.Next() {
		var name, ip, pon, sev, msg, st string
		var since time.Time
		if err := rows.Scan(&name, &ip, &pon, &sev, &msg, &since, &st); err != nil {
			return nil, err
		}
		data = append(data, []string{name, ip, pon, sev, st, since.In(time.Local).Format("2006-01-02 15:04"), msg})
	}
	base["columns"] = cols
	base["rows"] = data
	base["summary"] = summary
	return base, nil
}

func (s *Server) reportAutomations(ctx context.Context, pool *pgxpool.Pool, base map[string]any, opts periodModeReportOptions) (map[string]any, error) {
	mode := normalizePeriodMode(opts.Mode)
	base["title"] = "Automações"
	base["description"] = "Execuções de automações e relatórios agendados."
	base["options"] = map[string]any{"mode": mode, "from": opts.From, "to": opts.To}
	where, args := reportPeriodWhere(opts.From, opts.To, "started_at", 1)
	if where == "" {
		where = " AND started_at >= now() - interval '30 days'"
	}

	var total, okN, failN int
	_ = pool.QueryRow(ctx, `SELECT COUNT(*), COUNT(*) FILTER (WHERE ok), COUNT(*) FILTER (WHERE NOT ok) FROM automation_execution_log WHERE 1=1`+where, args...).Scan(&total, &okN, &failN)
	summary := map[string]any{"Execuções": total, "OK": okN, "Falhas": failN}

	if mode == "summary" {
		cols := []string{"Automação", "Execuções", "OK", "Falhas"}
		var data [][]string
		if rows, err := pool.Query(ctx, `
			SELECT job_type, COUNT(*), COUNT(*) FILTER (WHERE ok), COUNT(*) FILTER (WHERE NOT ok)
			FROM automation_execution_log WHERE 1=1`+where+`
			GROUP BY 1 ORDER BY 2 DESC`, args...); err == nil {
			defer rows.Close()
			for rows.Next() {
				var jt string
				var n, okc, fail int
				if rows.Scan(&jt, &n, &okc, &fail) == nil {
					data = append(data, []string{automationJobLabel(jt), strconv.Itoa(n), strconv.Itoa(okc), strconv.Itoa(fail)})
				}
			}
		}
		base["columns"] = cols
		base["rows"] = data
		base["summary"] = summary
		return base, nil
	}

	rows, err := pool.Query(ctx, `
		SELECT job_type, actor, trigger_type, started_at, finished_at, ok, COALESCE(status_message,''), COALESCE(error_message,'')
		FROM automation_execution_log WHERE 1=1`+where+`
		ORDER BY started_at DESC LIMIT 500`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols := []string{"Início", "Automação", "Actor", "Gatilho", "OK", "Mensagem", "Erro"}
	var data [][]string
	for rows.Next() {
		var jt, actor, trig, msg, errMsg string
		var start, fin time.Time
		var ok bool
		if err := rows.Scan(&jt, &actor, &trig, &start, &fin, &ok, &msg, &errMsg); err != nil {
			return nil, err
		}
		okS := "não"
		if ok {
			okS = "sim"
		}
		data = append(data, []string{
			start.In(time.Local).Format("2006-01-02 15:04"),
			automationJobLabel(jt), actor, trig, okS, msg, errMsg,
		})
	}
	base["columns"] = cols
	base["rows"] = data
	base["summary"] = summary
	return base, nil
}

func (s *Server) reportMonitoringHealth(ctx context.Context, pool *pgxpool.Pool, base map[string]any, opts periodModeReportOptions) (map[string]any, error) {
	mode := normalizePeriodMode(opts.Mode)
	base["title"] = "Saúde do monitoramento"
	base["description"] = "Estado do monitoramento, equipamentos offline e falhas SNMP."
	base["options"] = map[string]any{"mode": mode}

	summary := map[string]any{}
	var mon bool
	_ = pool.QueryRow(ctx, `SELECT COALESCE(is_running,false) FROM monitoring_runtime WHERE id=1`).Scan(&mon)
	summary["Monitoramento activo"] = mon
	var pingN, telN, offN, snmpFail, alertsN int
	_ = pool.QueryRow(ctx, `SELECT COUNT(*) FROM devices WHERE ping_enabled`).Scan(&pingN)
	_ = pool.QueryRow(ctx, `SELECT COUNT(*) FROM devices WHERE telemetry_enabled`).Scan(&telN)
	_ = pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM devices d
		JOIN device_probe_cache c ON c.device_id = d.id
		WHERE d.ping_enabled AND COALESCE(c.reach_ok, false) = false`).Scan(&offN)
	_ = pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM devices d
		JOIN device_probe_cache c ON c.device_id = d.id
		WHERE COALESCE(c.snmp_health_status,'') IN ('failed','partial')`).Scan(&snmpFail)
	_ = pool.QueryRow(ctx, `SELECT COUNT(*) FROM alert_instances WHERE closed_at IS NULL`).Scan(&alertsN)
	summary["Ping activo"] = pingN
	summary["Telemetria activa"] = telN
	summary["Sem resposta (ping)"] = offN
	summary["SNMP falho/parcial"] = snmpFail
	summary["Alertas abertos"] = alertsN

	if mode == "summary" {
		cols := []string{"Métrica", "Valor"}
		var data [][]string
		keys := make([]string, 0, len(summary))
		for k := range summary {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			data = append(data, []string{k, fmt.Sprintf("%v", summary[k])})
		}
		base["columns"] = cols
		base["rows"] = data
		base["summary"] = summary
		return base, nil
	}

	rows, err := pool.Query(ctx, `
		SELECT COALESCE(d.description,''), COALESCE(host(d.ip)::text,''), COALESCE(d.category,''),
			COALESCE(p.description,''),
			CASE WHEN COALESCE(c.reach_ok, true) THEN 'ok' ELSE 'offline' END,
			COALESCE(c.snmp_health_status, '—'),
			COALESCE(c.snmp_health_reason, ''),
			c.checked_at
		FROM devices d
		LEFT JOIN pops p ON p.id = d.pop_id
		LEFT JOIN device_probe_cache c ON c.device_id = d.id
		WHERE d.ping_enabled
		  AND (COALESCE(c.reach_ok, true) = false OR COALESCE(c.snmp_health_status,'') IN ('failed','partial'))
		ORDER BY COALESCE(c.reach_ok, true), d.description
		LIMIT 1000`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols := []string{"Equipamento", "IP", "Categoria", "POP", "Alcance", "SNMP", "Detalhe", "Verificado"}
	var data [][]string
	for rows.Next() {
		var name, ip, cat, pop, reach, snmp, reason string
		var at *time.Time
		if err := rows.Scan(&name, &ip, &cat, &pop, &reach, &snmp, &reason, &at); err != nil {
			return nil, err
		}
		when := ""
		if at != nil {
			when = at.In(time.Local).Format("2006-01-02 15:04")
		}
		data = append(data, []string{name, ip, cat, pop, reach, snmp, reason, when})
	}
	base["columns"] = cols
	base["rows"] = data
	base["summary"] = summary
	return base, nil
}

func (s *Server) reportCommercialBase(ctx context.Context, pool *pgxpool.Pool, base map[string]any, opts periodModeReportOptions) (map[string]any, error) {
	mode := normalizePeriodMode(opts.Mode)
	base["title"] = "Base comercial"
	base["description"] = "Clientes por localidade (registos mensais)."
	base["options"] = map[string]any{"mode": mode}

	var monthTotal, locN int
	_ = pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(client_count),0), COUNT(*)
		FROM commercial_monthly_records
		WHERE year_month = to_char((CURRENT_TIMESTAMP AT TIME ZONE 'UTC'), 'YYYY-MM')`).Scan(&monthTotal, &locN)
	summary := map[string]any{"Clientes (mês actual)": monthTotal, "Localidades com registo": locN}

	if mode == "summary" {
		cols := []string{"Mês", "Clientes", "Localidades"}
		var data [][]string
		if rows, err := pool.Query(ctx, `
			SELECT year_month, SUM(client_count), COUNT(*)
			FROM commercial_monthly_records
			GROUP BY 1 ORDER BY 1 DESC LIMIT 18`); err == nil {
			defer rows.Close()
			for rows.Next() {
				var ym string
				var n, locs int
				if rows.Scan(&ym, &n, &locs) == nil {
					data = append(data, []string{ym, strconv.Itoa(n), strconv.Itoa(locs)})
				}
			}
		}
		base["columns"] = cols
		base["rows"] = data
		base["summary"] = summary
		return base, nil
	}

	rows, err := pool.Query(ctx, `
		SELECT r.year_month, COALESCE(l.name,'(sem localidade)'), COALESCE(l.uf,''), r.client_count
		FROM commercial_monthly_records r
		LEFT JOIN commercial_localities l ON l.id = r.locality_id
		ORDER BY r.year_month DESC, l.name
		LIMIT 3000`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols := []string{"Mês", "Localidade", "UF", "Clientes"}
	var data [][]string
	for rows.Next() {
		var ym, name, uf string
		var n int
		if err := rows.Scan(&ym, &name, &uf, &n); err != nil {
			return nil, err
		}
		data = append(data, []string{ym, name, uf, strconv.Itoa(n)})
	}
	base["columns"] = cols
	base["rows"] = data
	base["summary"] = summary
	return base, nil
}

func normalizePeriodMode(mode string) string {
	if strings.ToLower(strings.TrimSpace(mode)) == "detailed" {
		return "detailed"
	}
	return "summary"
}
