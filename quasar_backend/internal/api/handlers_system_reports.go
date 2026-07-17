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
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/bngcollect"
	"github.com/netquasar/netquasar/quasar_backend/internal/reporttelegram"
	"github.com/netquasar/netquasar/quasar_backend/internal/telegramclient"
)

var systemReportCatalog = []map[string]string{
	{"id": "active-alerts", "title": "Alertas ativos", "description": "Lista detalhada de todos os alertas em aberto."},
	{"id": "connections", "title": "Conexões de clientes", "description": "Quantidade e detalhes das conexões cadastradas."},
	{"id": "equipment-by-pop", "title": "Equipamentos por POP", "description": "Lista de equipamentos agrupados por ponto de presença."},
	{"id": "olt-overview", "title": "OLTs — informações e gráfico", "description": "Frota OLT, ONUs e evolução recente (últimos 7 dias)."},
	{"id": "system-general", "title": "Visão geral do sistema", "description": "Métricas consolidadas de equipamentos, localidades, clientes, PONs, Mikrotik e mais."},
	{"id": "integrations", "title": "Integrações", "description": "Integrações configuradas e estado de cada uma."},
	{"id": "attention-devices", "title": "Equipamentos precisando de atenção", "description": "Lacunas de cadastro e equipamentos com alertas abertos."},
	{"id": "alerts-by-category", "title": "Alertas por categoria", "description": "Alertas ativos agrupados por categoria operacional."},
	{"id": "onu-per-pon", "title": "ONUs por PON", "description": "Última coleta por porta PON (sem nova coleta SNMP)."},
	{"id": "bng-subscribers", "title": "BNG — totais de logins", "description": "Totais PPPoE, IPv4, IPv6 e dual-stack por BNG e evolução recente (7 dias)."},
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
	opts := parseSystemReportOptions(r, id)
	payload, err := s.buildSystemReport(r.Context(), id, opts)
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
	opts := parseSystemReportOptions(r, id)
	payload, err := s.buildSystemReport(r.Context(), id, opts)
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
	opts := parseSystemReportOptions(r, id)
	payload, err := s.buildSystemReport(r.Context(), id, opts)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	title, _ := payload["title"].(string)
	text := reporttelegram.ComposeSystemReport(title, payload)
	if err := telegramclient.SendMessageChunks(r.Context(), cfg, text); err != nil {
		writeErr(w, http.StatusBadGateway, "TELEGRAM_SEND_FAILED", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "system_report", id, "telegram_send", s.actorFromRequest(r), nil, map[string]any{"report_id": id})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "report_id": id})
}

func (s *Server) buildSystemReport(ctx context.Context, id string, opts systemReportOptions) (map[string]any, error) {
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
		return s.reportConnections(ctx, pool, base, opts.Connections)
	case "equipment-by-pop":
		return s.reportEquipmentByPop(ctx, pool, base, opts.EquipmentByPop)
	case "olt-overview":
		return s.reportOltOverview(ctx, pool, base, opts.OltOverview)
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
	case "bng-subscribers":
		return s.reportBngSubscribers(ctx, pool, base)
	default:
		return nil, fmt.Errorf("relatório desconhecido")
	}
}

func alertTypeLabelGo(alertType string) string {
	switch strings.TrimSpace(alertType) {
	case "ping_unreachable":
		return "Equipamento offline"
	case "latency_high":
		return "Latência elevada"
	case "latency_degraded":
		return "Latência degradada"
	case "cpu_high":
		return "CPU elevada"
	case "memory_high":
		return "Memória elevada"
	case "temperature_high":
		return "Temperatura elevada"
	case "temperature_low":
		return "Temperatura baixa"
	case "snmp_failure":
		return "Falha SNMP"
	case "uptime_restart_low":
		return "Possível reinício (uptime baixo)"
	case "interface_down":
		return "Interface inativa"
	case "interface_down_transition":
		return "Interface mudou para DOWN"
	case "pon_down":
		return "PON inativa"
	case "mikrotik_sfp_tx":
		return "SFP — potência TX"
	case "mikrotik_sfp_rx":
		return "SFP — potência RX"
	case "telemetry_threshold":
		return "Telemetria — limiar global"
	case "olt_onu_drop":
		return "Queda de ONUs online (OLT)"
	case "olt_onu_rise":
		return "Subida de ONUs online (OLT)"
	case "bng_subscriber_drop":
		return "Queda de logins (BNG)"
	default:
		t := strings.TrimSpace(alertType)
		if t == "" {
			return "—"
		}
		return strings.ReplaceAll(t, "_", " ")
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
	case "bng_subscriber_drop":
		return "BNG"
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
	cols := []string{"Equipamento", "IP", "Severidade", "Tipo", "Categoria", "POP", "Desde", "Mensagem"}
	var dataRows [][]string
	var crit, warn, info int
	for rows.Next() {
		var id, dev, ip, sev, typ, msg, cat, pop string
		var since time.Time
		var meta []byte
		if err := rows.Scan(&id, &dev, &ip, &sev, &typ, &msg, &since, &meta, &cat, &pop); err != nil {
			return nil, err
		}
		_ = id
		_ = meta
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
			dev, ip, severityLabelGo(sev), alertTypeLabelGo(typ), catLbl, pop,
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

func (s *Server) reportConnections(ctx context.Context, pool *pgxpool.Pool, base map[string]any, opts connectionsReportOptions) (map[string]any, error) {
	opts = normalizeConnectionsReportOptions(opts)
	if opts.Source == "bng_cache" {
		return s.reportConnectionsFromBngCache(ctx, pool, base, opts)
	}
	if opts.Mode == "summary" {
		return s.reportConnectionsSummary(ctx, pool, base)
	}
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
	var total int64
	for rows.Next() {
		var num, name, login, ip, kind, medium, plan string
		var created time.Time
		if err := rows.Scan(&num, &name, &login, &ip, &kind, &medium, &plan, &created); err != nil {
			return nil, err
		}
		total++
		byKind[kind]++
		if medium != "" {
			byMedium[medium]++
		}
		dataRows = append(dataRows, []string{
			num, name, login, ip, kind, medium, plan, created.UTC().Format(time.RFC3339),
		})
	}
	base["title"] = "Conexões de clientes"
	base["description"] = "Inventário detalhado de conexões cadastradas."
	base["options"] = opts
	base["columns"] = cols
	base["rows"] = dataRows
	base["summary"] = map[string]any{
		"Total conexões": total,
		"Por tipo":       byKind,
		"Por meio":       byMedium,
	}
	return base, nil
}

func (s *Server) reportConnectionsSummary(ctx context.Context, pool *pgxpool.Pool, base map[string]any) (map[string]any, error) {
	var total int64
	_ = pool.QueryRow(ctx, `SELECT COUNT(*) FROM client_connections`).Scan(&total)

	byKind := map[string]int{}
	kindRows, err := pool.Query(ctx, `
		SELECT connection_kind, COUNT(*)::bigint FROM client_connections GROUP BY 1 ORDER BY 2 DESC, 1
	`)
	if err != nil {
		return nil, err
	}
	defer kindRows.Close()
	for kindRows.Next() {
		var kind string
		var n int64
		if kindRows.Scan(&kind, &n) == nil {
			byKind[kind] = int(n)
		}
	}

	byMedium := map[string]int{}
	mediumRows, err := pool.Query(ctx, `
		SELECT COALESCE(NULLIF(trim(medium_type), ''), '(sem meio)'), COUNT(*)::bigint
		FROM client_connections GROUP BY 1 ORDER BY 2 DESC, 1
	`)
	if err != nil {
		return nil, err
	}
	defer mediumRows.Close()
	for mediumRows.Next() {
		var medium string
		var n int64
		if mediumRows.Scan(&medium, &n) == nil {
			byMedium[medium] = int(n)
		}
	}

	byPlan := map[string]int{}
	planRows, err := pool.Query(ctx, `
		SELECT COALESCE(NULLIF(trim(sales_plan), ''), '(sem plano)'), COUNT(*)::bigint
		FROM client_connections GROUP BY 1 ORDER BY 2 DESC, 1
	`)
	if err != nil {
		return nil, err
	}
	defer planRows.Close()
	for planRows.Next() {
		var plan string
		var n int64
		if planRows.Scan(&plan, &n) == nil {
			byPlan[plan] = int(n)
		}
	}

	cols := []string{"Métrica", "Valor"}
	dataRows := [][]string{{"Total conexões", strconv.FormatInt(total, 10)}}
	for kind, n := range byKind {
		dataRows = append(dataRows, []string{"Tipo — " + kind, strconv.Itoa(n)})
	}
	for medium, n := range byMedium {
		dataRows = append(dataRows, []string{"Meio — " + medium, strconv.Itoa(n)})
	}
	for plan, n := range byPlan {
		dataRows = append(dataRows, []string{"Plano — " + plan, strconv.Itoa(n)})
	}
	sort.Slice(dataRows[1:], func(i, j int) bool {
		return dataRows[i+1][0] < dataRows[j+1][0]
	})

	base["title"] = "Conexões de clientes — resumo"
	base["description"] = "Totais por tipo, meio de acesso e plano comercial (cadastro de conexões)."
	base["options"] = connectionsReportOptions{Mode: "summary", Source: "connections"}
	base["columns"] = cols
	base["rows"] = dataRows
	base["summary"] = map[string]any{
		"Total conexões": total,
		"Por tipo":       byKind,
		"Por meio":       byMedium,
		"Por plano":      byPlan,
	}
	return base, nil
}

func (s *Server) reportConnectionsFromBngCache(ctx context.Context, pool *pgxpool.Pool, base map[string]any, opts connectionsReportOptions) (map[string]any, error) {
	deviceID, err := uuid.Parse(strings.TrimSpace(opts.BngDeviceID))
	if err != nil {
		return nil, fmt.Errorf("BNG inválido")
	}
	var bngName, bngIP string
	err = pool.QueryRow(ctx, `
		SELECT description, host(ip)::text FROM devices
		WHERE id=$1 AND coalesce(bng_enabled, false)=true
	`, deviceID).Scan(&bngName, &bngIP)
	if err != nil {
		return nil, fmt.Errorf("BNG não encontrado ou BNG não activo")
	}

	sessions, capturedAt, _, note := s.loadCachedBngSessions(ctx, deviceID)
	if len(sessions) == 0 {
		if note == "" {
			note = "Nenhuma consulta completa guardada para este BNG."
		}
		return nil, fmt.Errorf("%s", note)
	}

	profile := bngcollect.LoadGlobalProfile(ctx, pool)
	sessions = normalizeCachedSessionLogins(sessions, profile.Options.PPPoELoginStripSuffix)

	if opts.Mode == "summary" {
		byIPType := map[string]int{}
		byDomain := map[string]int{}
		byVLAN := map[string]int{}
		ipv4, ipv6, dual := 0, 0, 0
		for _, sm := range sessions {
			ipType := strings.TrimSpace(fmt.Sprint(sm["ip_type"]))
			if ipType == "" {
				ipType = "(desconhecido)"
			}
			byIPType[ipType]++
			domain := strings.TrimSpace(fmt.Sprint(sm["domain"]))
			if domain == "" {
				domain = "(sem domínio)"
			}
			byDomain[domain]++
			vlan := strings.TrimSpace(fmt.Sprint(sm["vlan"]))
			if vlan == "" {
				vlan = "(sem VLAN)"
			}
			byVLAN[vlan]++
			has4 := strings.TrimSpace(fmt.Sprint(sm["ipv4"])) != "" && strings.TrimSpace(fmt.Sprint(sm["ipv4"])) != "0.0.0.0"
			has6 := strings.TrimSpace(fmt.Sprint(sm["ipv6"])) != "" || strings.TrimSpace(fmt.Sprint(sm["ipv6_pd"])) != ""
			if has4 && has6 {
				dual++
			} else if has6 {
				ipv6++
			} else if has4 {
				ipv4++
			}
		}
		cols := []string{"Métrica", "Valor"}
		dataRows := [][]string{
			{"BNG", bngName},
			{"IP gestão", bngIP},
			{"Total sessões PPPoE", strconv.Itoa(len(sessions))},
			{"Com IPv4", strconv.Itoa(ipv4)},
			{"Com IPv6", strconv.Itoa(ipv6)},
			{"Dual-stack", strconv.Itoa(dual)},
		}
		for k, n := range byIPType {
			dataRows = append(dataRows, []string{"Tipo IP — " + k, strconv.Itoa(n)})
		}
		for k, n := range byDomain {
			dataRows = append(dataRows, []string{"Domínio — " + k, strconv.Itoa(n)})
		}
		for k, n := range byVLAN {
			dataRows = append(dataRows, []string{"VLAN — " + k, strconv.Itoa(n)})
		}
		snapAt := "—"
		if capturedAt != nil {
			snapAt = capturedAt.UTC().Format(time.RFC3339)
		}
		base["title"] = "Conexões — resumo cache PPPoE (BNG)"
		base["description"] = fmt.Sprintf("Totais da última consulta SNMP guardada no BNG %s.", bngName)
		base["options"] = opts
		base["columns"] = cols
		base["rows"] = dataRows
		base["summary"] = map[string]any{
			"BNG":              bngName,
			"Snapshot":         snapAt,
			"Total sessões":    len(sessions),
			"Por tipo IP":      byIPType,
			"Por domínio AAA":  byDomain,
			"Por VLAN":         byVLAN,
			"IPv4":             ipv4,
			"IPv6":             ipv6,
			"Dual-stack":       dual,
		}
		return base, nil
	}

	cols := []string{"Login", "IPv4", "MAC", "IPv6", "Tipo IP", "Domínio", "VLAN", "Tempo online"}
	var dataRows [][]string
	for _, sm := range sessions {
		dataRows = append(dataRows, []string{
			strings.TrimSpace(fmt.Sprint(sm["login"])),
			strings.TrimSpace(fmt.Sprint(sm["ipv4"])),
			strings.TrimSpace(fmt.Sprint(sm["mac"])),
			strings.TrimSpace(fmt.Sprint(sm["ipv6"])),
			strings.TrimSpace(fmt.Sprint(sm["ip_type"])),
			strings.TrimSpace(fmt.Sprint(sm["domain"])),
			strings.TrimSpace(fmt.Sprint(sm["vlan"])),
			firstNonEmptyString(fmt.Sprint(sm["online_time"]), fmt.Sprint(sm["online_time_sec"])),
		})
	}
	sort.Slice(dataRows, func(i, j int) bool {
		return strings.ToLower(dataRows[i][0]) < strings.ToLower(dataRows[j][0])
	})
	snapAt := "—"
	if capturedAt != nil {
		snapAt = capturedAt.UTC().Format(time.RFC3339)
	}
	base["title"] = "Conexões — detalhe cache PPPoE (BNG)"
	base["description"] = fmt.Sprintf("Logins online na última consulta SNMP do BNG %s.", bngName)
	base["options"] = opts
	base["columns"] = cols
	base["rows"] = dataRows
	base["summary"] = map[string]any{
		"BNG":           bngName,
		"Snapshot":      snapAt,
		"Total sessões": len(sessions),
	}
	return base, nil
}

const equipmentByPopNoPopLabel = "(sem POP)"

type equipmentByPopReportOptions struct {
	IncludeWithoutPop     bool `json:"include_without_pop"`
	IncludePopCoordinates bool `json:"include_pop_coordinates"`
}

type connectionsReportOptions struct {
	Mode         string `json:"mode"`          // summary | detailed
	Source       string `json:"source"`        // connections | bng_cache
	BngDeviceID  string `json:"bng_device_id"` // obrigatório se source=bng_cache
}

type oltOverviewReportOptions struct {
	Period string `json:"period"` // today | 3d | 7d | 30d
}

type systemReportOptions struct {
	EquipmentByPop equipmentByPopReportOptions
	Connections    connectionsReportOptions
	OltOverview    oltOverviewReportOptions
}

func parseSystemReportOptions(r *http.Request, reportID string) systemReportOptions {
	opts := systemReportOptions{
		Connections: connectionsReportOptions{Mode: "detailed", Source: "connections"},
		OltOverview: oltOverviewReportOptions{Period: "7d"},
	}
	q := r.URL.Query()
	switch reportID {
	case "equipment-by-pop":
		opts.EquipmentByPop.IncludeWithoutPop = queryBool(q.Get("include_without_pop"))
		opts.EquipmentByPop.IncludePopCoordinates = queryBool(q.Get("include_pop_coordinates"))
	case "connections":
		if v := strings.TrimSpace(q.Get("mode")); v != "" {
			opts.Connections.Mode = v
		}
		if v := strings.TrimSpace(q.Get("source")); v != "" {
			opts.Connections.Source = v
		}
		if v := strings.TrimSpace(q.Get("bng_device_id")); v != "" {
			opts.Connections.BngDeviceID = v
		}
	case "olt-overview":
		if v := strings.TrimSpace(q.Get("period")); v != "" {
			opts.OltOverview.Period = v
		}
	}
	if r.Method == http.MethodPost && r.Body != nil {
		switch reportID {
		case "equipment-by-pop":
			var body equipmentByPopReportOptions
			if json.NewDecoder(r.Body).Decode(&body) == nil {
				opts.EquipmentByPop = body
			}
		case "connections":
			var body connectionsReportOptions
			if json.NewDecoder(r.Body).Decode(&body) == nil {
				opts.Connections = normalizeConnectionsReportOptions(body)
			}
		case "olt-overview":
			var body oltOverviewReportOptions
			if json.NewDecoder(r.Body).Decode(&body) == nil {
				opts.OltOverview = normalizeOltOverviewReportOptions(body)
			}
		}
	}
	return opts
}

func normalizeConnectionsReportOptions(o connectionsReportOptions) connectionsReportOptions {
	mode := strings.ToLower(strings.TrimSpace(o.Mode))
	if mode != "summary" {
		mode = "detailed"
	}
	source := strings.ToLower(strings.TrimSpace(o.Source))
	if source != "bng_cache" {
		source = "connections"
	}
	return connectionsReportOptions{
		Mode:        mode,
		Source:      source,
		BngDeviceID: strings.TrimSpace(o.BngDeviceID),
	}
}

func normalizeOltOverviewReportOptions(o oltOverviewReportOptions) oltOverviewReportOptions {
	p := strings.ToLower(strings.TrimSpace(o.Period))
	switch p {
	case "today", "1d", "1":
		p = "today"
	case "3d", "3":
		p = "3d"
	case "30d", "30":
		p = "30d"
	default:
		p = "7d"
	}
	return oltOverviewReportOptions{Period: p}
}

func oltOverviewPeriodDays(period string) int {
	switch normalizeOltOverviewReportOptions(oltOverviewReportOptions{Period: period}).Period {
	case "today":
		return 1
	case "3d":
		return 3
	case "30d":
		return 30
	default:
		return 7
	}
}

func queryBool(v string) bool {
	v = strings.TrimSpace(strings.ToLower(v))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func formatPopCoordinates(lat, lng *float64) string {
	if lat == nil || lng == nil {
		return ""
	}
	return fmt.Sprintf("%.6f, %.6f", *lat, *lng)
}

func (s *Server) reportEquipmentByPop(ctx context.Context, pool *pgxpool.Pool, base map[string]any, opts equipmentByPopReportOptions) (map[string]any, error) {
	rows, err := pool.Query(ctx, `
		SELECT COALESCE(p.description, $1), d.description,
			COALESCE(NULLIF(trim(d.category), ''), '(sem categoria)'),
			p.latitude, p.longitude, (d.pop_id IS NULL) AS no_pop
		FROM devices d
		LEFT JOIN pops p ON p.id = d.pop_id
		WHERE ($2::boolean OR d.pop_id IS NOT NULL)
		ORDER BY lower(trim(COALESCE(p.description, $1))), lower(trim(d.description))
	`, equipmentByPopNoPopLabel, opts.IncludeWithoutPop)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type deviceRow struct {
		Name     string
		Category string
		Label    string
	}

	type popBucket struct {
		devices []deviceRow
		lat     *float64
		lng     *float64
	}

	groupByPop := map[string]*popBucket{}
	popOrder := []string{}
	var devCount int64

	for rows.Next() {
		var pop, name, cat string
		var lat, lng *float64
		var noPop bool
		if err := rows.Scan(&pop, &name, &cat, &lat, &lng, &noPop); err != nil {
			return nil, err
		}
		if noPop || pop == equipmentByPopNoPopLabel {
			pop = equipmentByPopNoPopLabel
		}
		devCount++
		b := groupByPop[pop]
		if b == nil {
			b = &popBucket{devices: []deviceRow{}}
			groupByPop[pop] = b
			popOrder = append(popOrder, pop)
		}
		if lat != nil && lng != nil && b.lat == nil {
			b.lat = lat
			b.lng = lng
		}
		b.devices = append(b.devices, deviceRow{
			Name:     name,
			Category: cat,
			Label:    fmt.Sprintf("%s [%s]", name, cat),
		})
	}

	groups := make([]map[string]any, 0, len(popOrder))
	csvCols := []string{"POP", "Equipamento"}
	if opts.IncludePopCoordinates {
		csvCols = []string{"POP", "Latitude", "Longitude", "Equipamento"}
	}
	csvRows := [][]string{}
	for _, pop := range popOrder {
		b := groupByPop[pop]
		deviceMaps := make([]map[string]any, 0, len(b.devices))
		group := map[string]any{"pop": pop, "devices": deviceMaps}
		coords := ""
		if opts.IncludePopCoordinates {
			coords = formatPopCoordinates(b.lat, b.lng)
			if coords != "" {
				group["latitude"] = *b.lat
				group["longitude"] = *b.lng
				group["coordinates"] = coords
			}
		}
		for _, d := range b.devices {
			deviceMaps = append(deviceMaps, map[string]any{
				"name": d.Name, "category": d.Category, "label": d.Label,
			})
			if opts.IncludePopCoordinates {
				csvRows = append(csvRows, []string{pop, formatCoordCell(b.lat), formatCoordCell(b.lng), d.Label})
			} else {
				csvRows = append(csvRows, []string{pop, d.Label})
			}
		}
		group["devices"] = deviceMaps
		groups = append(groups, group)
	}

	var popCount int64
	_ = pool.QueryRow(ctx, `SELECT COUNT(*) FROM pops`).Scan(&popCount)

	base["title"] = "Equipamentos por POP"
	base["description"] = "Equipamentos agrupados por ponto de presença."
	base["options"] = map[string]any{
		"include_without_pop":     opts.IncludeWithoutPop,
		"include_pop_coordinates": opts.IncludePopCoordinates,
	}
	base["columns"] = csvCols
	base["rows"] = csvRows
	base["groups"] = groups
	base["summary"] = map[string]any{
		"POPs":                 popCount,
		"Equipamentos (total)": devCount,
	}
	return base, nil
}

func formatCoordCell(v *float64) string {
	if v == nil {
		return ""
	}
	return strconv.FormatFloat(*v, 'f', 6, 64)
}

func firstNonEmptyString(vals ...string) string {
	for _, v := range vals {
		v = strings.TrimSpace(v)
		if v != "" && v != "<nil>" {
			return v
		}
	}
	return "—"
}

func (s *Server) reportOltOverview(ctx context.Context, pool *pgxpool.Pool, base map[string]any, opts oltOverviewReportOptions) (map[string]any, error) {
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

	period := normalizeOltOverviewReportOptions(opts).Period
	periodDays := oltOverviewPeriodDays(period)
	since := time.Now().UTC().Add(-time.Duration(periodDays) * 24 * time.Hour)
	if period == "today" {
		now := time.Now().UTC()
		since = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	}

	chartPts := []map[string]any{}
	if period == "today" {
		hourRows, err := pool.Query(ctx, `
			SELECT to_char(date_trunc('hour', updated_at AT TIME ZONE 'UTC'), 'YYYY-MM-DD"T"HH24:00:00"Z"') AS bucket,
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
			defer hourRows.Close()
			for hourRows.Next() {
				var bucket string
				var t, on, off int64
				if hourRows.Scan(&bucket, &t, &on, &off) == nil {
					chartPts = append(chartPts, map[string]any{
						"t": bucket, "total": t, "online": on, "offline": off,
					})
				}
			}
		}
	} else {
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
	}

	periodLabel := map[string]string{"today": "Hoje", "3d": "3 dias", "7d": "7 dias", "30d": "30 dias"}[period]
	if periodLabel == "" {
		periodLabel = "7 dias"
	}
	base["title"] = "OLTs — informações e gráfico"
	base["description"] = fmt.Sprintf("Estado actual da frota OLT e tendência de snapshots (%s).", periodLabel)
	base["options"] = normalizeOltOverviewReportOptions(opts)
	base["columns"] = cols
	base["rows"] = dataRows
	base["summary"] = map[string]any{
		"Período":           periodLabel,
		"OLTs com snapshot": len(dataRows),
		"Portas PON":        ponPorts,
		"ONUs (frota)":      fleetTotal,
		"Online":            fleetOn,
		"Offline":           fleetOff,
	}
	chartLabel := "ONUs por dia (snapshots)"
	if period == "today" {
		chartLabel = "ONUs por hora (hoje)"
	}
	base["chart"] = map[string]any{"points": chartPts, "label": chartLabel}
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
		SELECT d.id::text, COALESCE(NULLIF(trim(d.description), ''), '—'),
			COALESCE(NULLIF(trim(d.category), ''), '—'),
			COALESCE(host(d.ip)::text, '—'),
			a.severity, COUNT(*)::bigint
		FROM alert_instances a
		JOIN devices d ON d.id = a.device_id
		WHERE a.closed_at IS NULL
		GROUP BY d.id, d.description, d.category, d.ip, a.severity
		ORDER BY
			MIN(CASE a.severity WHEN 'critical' THEN 0 WHEN 'warning' THEN 1 ELSE 2 END),
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

func intPtrStr(p *int) string {
	if p == nil {
		return "—"
	}
	return strconv.Itoa(*p)
}

func (s *Server) reportBngSubscribers(ctx context.Context, pool *pgxpool.Pool, base map[string]any) (map[string]any, error) {
	rows, err := pool.Query(ctx, `
		SELECT d.description, host(d.ip)::text, s.collected_at,
			s.total_online, s.pppoe_online, s.ipv4_online, s.ipv6_online, s.dual_stack_online
		FROM devices d
		LEFT JOIN LATERAL (
			SELECT collected_at, total_online, pppoe_online, ipv4_online, ipv6_online, dual_stack_online
			FROM bng_stats_samples
			WHERE device_id = d.id
			ORDER BY collected_at DESC
			LIMIT 1
		) s ON true
		WHERE coalesce(d.bng_enabled, false) = true
		ORDER BY d.description
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols := []string{"BNG", "IP", "Última coleta", "Total online", "PPPoE", "IPv4", "IPv6", "Dual-stack"}
	var dataRows [][]string
	var fleetTotal, fleetPPPoE, fleetIPv4, fleetIPv6, fleetDual int64
	var devicesWithStats int
	for rows.Next() {
		var desc, ip string
		var collectedAt *time.Time
		var total, pppoe, ipv4, ipv6, dual *int
		if err := rows.Scan(&desc, &ip, &collectedAt, &total, &pppoe, &ipv4, &ipv6, &dual); err != nil {
			return nil, err
		}
		if collectedAt != nil {
			devicesWithStats++
		}
		if total != nil {
			fleetTotal += int64(*total)
		}
		if pppoe != nil {
			fleetPPPoE += int64(*pppoe)
		}
		if ipv4 != nil {
			fleetIPv4 += int64(*ipv4)
		}
		if ipv6 != nil {
			fleetIPv6 += int64(*ipv6)
		}
		if dual != nil {
			fleetDual += int64(*dual)
		}
		lastAt := "—"
		if collectedAt != nil {
			lastAt = reporttelegram.FormatGeneratedAt(collectedAt.UTC().Format(time.RFC3339))
		}
		dataRows = append(dataRows, []string{
			desc, ip, lastAt,
			intPtrStr(total), intPtrStr(pppoe), intPtrStr(ipv4), intPtrStr(ipv6), intPtrStr(dual),
		})
	}

	since := time.Now().UTC().Add(-7 * 24 * time.Hour)
	chartPts := []map[string]any{}
	sampleRows, err := pool.Query(ctx, `
		SELECT b.collected_at, d.description,
			b.total_online, b.pppoe_online, b.ipv4_online, b.ipv6_online, b.dual_stack_online
		FROM bng_stats_samples b
		JOIN devices d ON d.id = b.device_id AND coalesce(d.bng_enabled, false) = true
		WHERE b.collected_at >= $1
		ORDER BY b.collected_at ASC
		LIMIT 500
	`, since)
	if err == nil {
		defer sampleRows.Close()
		for sampleRows.Next() {
			var ts time.Time
			var device string
			var total, pppoe, ipv4, ipv6, dual *int
			if sampleRows.Scan(&ts, &device, &total, &pppoe, &ipv4, &ipv6, &dual) == nil {
				pt := map[string]any{
					"t":            ts.UTC().Format(time.RFC3339),
					"collected_at": ts.UTC().Format(time.RFC3339),
					"device":       device,
				}
				if total != nil {
					pt["total"] = *total
				}
				if pppoe != nil {
					pt["pppoe"] = *pppoe
				}
				if ipv4 != nil {
					pt["ipv4"] = *ipv4
				}
				if ipv6 != nil {
					pt["ipv6"] = *ipv6
				}
				if dual != nil {
					pt["dual_stack"] = *dual
				}
				chartPts = append(chartPts, pt)
			}
		}
	}

	averages := bngSubscriberAverages(ctx, pool)

	base["title"] = "BNG — totais de logins"
	base["columns"] = cols
	base["rows"] = dataRows
	base["summary"] = map[string]any{
		"BNGs activos":     len(dataRows),
		"BNGs com amostra": devicesWithStats,
		"Total online":     fleetTotal,
		"PPPoE online":     fleetPPPoE,
		"IPv4 online":      fleetIPv4,
		"IPv6 online":      fleetIPv6,
		"Dual-stack":       fleetDual,
	}
	base["averages"] = averages
	base["chart"] = map[string]any{"points": chartPts, "label": "Totais por coleta (últimos 7 dias — UI)", "kind": "bng-subscribers"}
	return base, nil
}

func bngSubscriberAverages(ctx context.Context, pool *pgxpool.Pool) map[string]any {
	windows := []int{7, 30, 60}
	var out []map[string]any
	for _, days := range windows {
		since := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
		var sampleCount int64
		var avgTotal, avgPPPoE, avgIPv4, avgIPv6, avgDual *float64
		_ = pool.QueryRow(ctx, `
			SELECT COUNT(*)::bigint,
				AVG(b.total_online::float), AVG(b.pppoe_online::float), AVG(b.ipv4_online::float),
				AVG(b.ipv6_online::float), AVG(b.dual_stack_online::float)
			FROM bng_stats_samples b
			JOIN devices d ON d.id = b.device_id AND coalesce(d.bng_enabled, false) = true
			WHERE b.collected_at >= $1
		`, since).Scan(&sampleCount, &avgTotal, &avgPPPoE, &avgIPv4, &avgIPv6, &avgDual)
		if sampleCount == 0 {
			continue
		}
		win := map[string]any{
			"days": days, "label": fmt.Sprintf("%d dias", days), "samples": sampleCount,
		}
		if avgTotal != nil {
			win["total"] = int64(*avgTotal + 0.5)
		}
		if avgPPPoE != nil {
			win["pppoe"] = int64(*avgPPPoE + 0.5)
		}
		if avgIPv4 != nil {
			win["ipv4"] = int64(*avgIPv4 + 0.5)
		}
		if avgIPv6 != nil {
			win["ipv6"] = int64(*avgIPv6 + 0.5)
		}
		if avgDual != nil {
			win["dual_stack"] = int64(*avgDual + 0.5)
		}
		out = append(out, win)
	}
	return map[string]any{"windows": out}
}
