package api

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpdiscovery"
	"github.com/netquasar/netquasar/quasar_backend/internal/telegramclient"
	"github.com/netquasar/netquasar/quasar_backend/internal/telemetryengine"
)

func (s *Server) toolsSNMPBulkGet(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Host      string   `json:"host"`
		Community string   `json:"community"`
		OIDs      []string `json:"oids"`
		TimeoutMs int      `json:"timeout_ms"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.OIDs) == 0 {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "host e oids obrigatórios", nil)
		return
	}
	if strings.TrimSpace(body.Community) == "" {
		var def *string
		_ = s.DB().QueryRow(r.Context(), `SELECT snmp_community FROM settings_connection_defaults WHERE id=1`).Scan(&def)
		if def != nil {
			body.Community = strings.TrimSpace(*def)
		}
	}
	if strings.TrimSpace(body.Community) == "" {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "community não informada e sem padrão configurado", nil)
		return
	}
	to := time.Duration(body.TimeoutMs) * time.Millisecond
	if to <= 0 {
		to = 8 * time.Second
	}
	ctx, cancel := context.WithTimeout(r.Context(), to+time.Second)
	defer cancel()
	res := probing.SNMPGet(ctx, probing.SNMPGetParams{
		Host: body.Host, Community: body.Community, OIDs: body.OIDs, Version: "2c", Timeout: to, Retries: 0,
	})
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) toolsSNMPWalkRun(w http.ResponseWriter, r *http.Request) {
	type walkReq struct {
		Host      string `json:"host"`
		Port      int    `json:"port"`
		Community string `json:"community"`
		Version   string `json:"version"`
		TimeoutMs int    `json:"timeout_ms"`
		Retries   int    `json:"retries"`
		RootOID   string `json:"root_oid"`
		MaxRows   int    `json:"max_rows"`
	}
	var body walkReq
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	body.Host = strings.TrimSpace(body.Host)
	if body.Host == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "host obrigatório", nil)
		return
	}
	if strings.TrimSpace(body.Community) == "" {
		var def *string
		_ = s.DB().QueryRow(r.Context(), `SELECT snmp_community FROM settings_connection_defaults WHERE id=1`).Scan(&def)
		if def != nil {
			body.Community = strings.TrimSpace(*def)
		}
	}
	if strings.TrimSpace(body.Community) == "" {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "community não informada e sem padrão configurado", nil)
		return
	}
	if body.Port <= 0 || body.Port > 65535 {
		body.Port = 161
	}
	if strings.TrimSpace(body.Version) == "" {
		body.Version = "2c"
	}
	if body.TimeoutMs <= 0 {
		body.TimeoutMs = 8000
	}
	if body.MaxRows <= 0 {
		body.MaxRows = 8000
	}
	if body.MaxRows > 20000 {
		body.MaxRows = 20000
	}
	body.RootOID = strings.TrimSpace(body.RootOID)
	if body.RootOID == "" {
		body.RootOID = "1.3.6.1.2.1"
	}
	var jid uuid.UUID
	err := s.DB().QueryRow(r.Context(), `
		INSERT INTO snmp_walk_jobs (device_id, host, community, scope, status)
		VALUES (NULL,$1,$2,$3,'queued') RETURNING id
	`, body.Host, body.Community, body.RootOID).Scan(&jid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	jobID := jid
	go func(in walkReq) {
		pool := s.DB()
		if pool == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(in.TimeoutMs)*time.Millisecond+35*time.Second)
		defer cancel()
		_, _ = pool.Exec(ctx, `UPDATE snmp_walk_jobs SET status='running' WHERE id=$1`, jobID)
		rows, truncated, walkNote := probing.SNMPWalk(ctx, probing.SNMPWalkParams{
			Host:      in.Host,
			Port:      uint16(in.Port),
			Community: in.Community,
			RootOID:   in.RootOID,
			Version:   in.Version,
			Timeout:   time.Duration(in.TimeoutMs) * time.Millisecond,
			Retries:   in.Retries,
			MaxRows:   in.MaxRows,
		})
		out := map[string]any{
			"status":      "done",
			"host":        in.Host,
			"port":        in.Port,
			"version":     in.Version,
			"root_oid":    in.RootOID,
			"row_count":   len(rows),
			"truncated":   truncated,
			"walk_note":   walkNote,
			"rows":        rows,
			"discoveries": buildToolsWalkDiscoveries(rows),
		}
		if walkNote != "" && len(rows) == 0 {
			out["status"] = "failed"
			b, _ := json.Marshal(out)
			_, _ = pool.Exec(ctx, `UPDATE snmp_walk_jobs SET status='failed', error_message=$2, result=$3::jsonb, finished_at=now() WHERE id=$1`, jobID, walkNote, b)
			return
		}
		b, _ := json.Marshal(out)
		_, _ = pool.Exec(ctx, `UPDATE snmp_walk_jobs SET status='done', result=$2::jsonb, error_message=NULL, finished_at=now() WHERE id=$1`, jobID, b)
	}(body)
	writeJSON(w, http.StatusAccepted, map[string]any{
		"job_id":   jid,
		"status":   "queued",
		"host":     body.Host,
		"root_oid": body.RootOID,
		"note":     "SNMP walk iniciado. Consulte as linhas e descobertas pelo job_id.",
	})
}

func (s *Server) toolsSNMPWalkJobRows(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "jobId"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	search := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("search")))
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	var st, scope, host *string
	var res []byte
	err = s.DB().QueryRow(r.Context(), `
		SELECT status, scope, host, result::text FROM snmp_walk_jobs WHERE id=$1
	`, id).Scan(&st, &scope, &host, &res)
	if err == pgx.ErrNoRows {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "", nil)
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	type resultDoc struct {
		Rows []probing.SNMPVar `json:"rows"`
	}
	var doc resultDoc
	if len(res) > 0 {
		_ = json.Unmarshal(res, &doc)
	}
	filtered := make([]probing.SNMPVar, 0, len(doc.Rows))
	if search == "" {
		filtered = doc.Rows
	} else {
		for _, row := range doc.Rows {
			hay := strings.ToLower(strings.Join([]string{row.OID, row.Type, row.Value}, " "))
			if strings.Contains(hay, search) {
				filtered = append(filtered, row)
			}
		}
	}
	total := len(filtered)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"job_id": id,
		"status": st,
		"scope":  scope,
		"host":   host,
		"total":  total,
		"offset": offset,
		"limit":  limit,
		"rows":   filtered[offset:end],
	})
}

func (s *Server) toolsSNMPWalkJobDiscoveries(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "jobId"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	var st *string
	var res []byte
	err = s.DB().QueryRow(r.Context(), `SELECT status, result::text FROM snmp_walk_jobs WHERE id=$1`, id).Scan(&st, &res)
	if err == pgx.ErrNoRows {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "", nil)
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	var doc map[string]any
	if len(res) > 0 {
		_ = json.Unmarshal(res, &doc)
	}
	candidates := []any{}
	if v, ok := doc["discoveries"].([]any); ok {
		candidates = v
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"job_id":     id,
		"status":     st,
		"candidates": candidates,
	})
}

func buildToolsWalkDiscoveries(rows []probing.SNMPVar) []map[string]any {
	type bucket struct {
		Name  string
		Root  string
		Limit int
		Rows  []map[string]any
	}
	buckets := []bucket{
		{Name: "Sistema", Root: "1.3.6.1.2.1.1.", Limit: 12},
		{Name: "Interfaces", Root: "1.3.6.1.2.1.2.2.1.", Limit: 20},
		{Name: "CPU/Memória host", Root: "1.3.6.1.2.1.25.", Limit: 30},
		{Name: "UCD", Root: "1.3.6.1.4.1.2021.", Limit: 30},
		{Name: "MikroTik", Root: "1.3.6.1.4.1.14988.", Limit: 30},
	}
	for _, row := range rows {
		for bi := range buckets {
			if strings.HasPrefix(strings.TrimSpace(row.OID), buckets[bi].Root) {
				if len(buckets[bi].Rows) < buckets[bi].Limit {
					buckets[bi].Rows = append(buckets[bi].Rows, map[string]any{
						"oid":   row.OID,
						"type":  row.Type,
						"value": row.Value,
					})
				}
				break
			}
		}
	}
	out := make([]map[string]any, 0, len(buckets))
	for _, b := range buckets {
		if len(b.Rows) == 0 {
			continue
		}
		out = append(out, map[string]any{
			"kind":     b.Name,
			"root_oid": b.Root,
			"sample":   b.Rows,
		})
	}
	return out
}

func (s *Server) toolsMikrotikQuickMetrics(w http.ResponseWriter, r *http.Request) {
	writeErr(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "Coleta RouterOS dedicada ainda não implementada.", nil)
}

func (s *Server) toolsMikrotikInterfaces(w http.ResponseWriter, r *http.Request) {
	writeErr(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "Interfaces Mikrotik dedicadas não implementadas.", nil)
}

func (s *Server) toolsMikrotikWalk(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Host      string `json:"host"`
		Port      int    `json:"port"`
		Community string `json:"community"`
		Version   string `json:"version"`
		TimeoutMs int    `json:"timeout_ms"`
		Retries   int    `json:"retries"`
		MaxRows   int    `json:"max_rows"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	body.Host = strings.TrimSpace(body.Host)
	if body.Host == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "host (IP ou nome) obrigatório", nil)
		return
	}
	if strings.TrimSpace(body.Community) == "" {
		var def *string
		_ = s.DB().QueryRow(r.Context(), `SELECT snmp_community FROM settings_connection_defaults WHERE id=1`).Scan(&def)
		if def != nil {
			body.Community = strings.TrimSpace(*def)
		}
	}
	if strings.TrimSpace(body.Community) == "" {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "community não informada e sem padrão configurado", nil)
		return
	}
	if body.Port <= 0 || body.Port > 65535 {
		body.Port = 161
	}
	if strings.TrimSpace(body.Version) == "" {
		body.Version = "2c"
	}
	if body.TimeoutMs <= 0 {
		body.TimeoutMs = 8000
	}
	if body.TimeoutMs > 120000 {
		body.TimeoutMs = 120000
	}
	if body.MaxRows <= 0 {
		body.MaxRows = 8000
	}
	if body.MaxRows > 20000 {
		body.MaxRows = 20000
	}
	const ifMibIfTable = "1.3.6.1.2.1.2.2.1"
	var jid uuid.UUID
	err := s.DB().QueryRow(r.Context(), `
		INSERT INTO snmp_walk_jobs (device_id, host, community, scope, status)
		VALUES (NULL,$1,$2,$3,'queued') RETURNING id
	`, body.Host, body.Community, ifMibIfTable).Scan(&jid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	jobID := jid
	go func(host, community, version string, port int, timeoutMs, retries, maxRows int) {
		pool := s.DB()
		if pool == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutMs)*time.Millisecond+45*time.Second)
		defer cancel()
		_, _ = pool.Exec(ctx, `UPDATE snmp_walk_jobs SET status='running' WHERE id=$1`, jobID)
		rows, truncated, walkNote := probing.SNMPWalk(ctx, probing.SNMPWalkParams{
			Host:      host,
			Port:      uint16(port),
			Community: community,
			RootOID:   ifMibIfTable,
			Version:   version,
			Timeout:   time.Duration(timeoutMs) * time.Millisecond,
			Retries:   retries,
			MaxRows:   maxRows,
		})
		out := map[string]any{
			"status":       "done",
			"host":         host,
			"port":         port,
			"version":      version,
			"root_oid":     ifMibIfTable,
			"note":         "Walk IF-MIB ifTable (interfaces); adequado a equipamentos Mikrotik e outros agentes SNMP.",
			"row_count":    len(rows),
			"truncated":    truncated,
			"walk_note":    walkNote,
			"rows":         rows,
			"discoveries":  buildToolsWalkDiscoveries(rows),
		}
		if walkNote != "" && len(rows) == 0 {
			out["status"] = "failed"
			b, _ := json.Marshal(out)
			_, _ = pool.Exec(ctx, `UPDATE snmp_walk_jobs SET status='failed', error_message=$2, result=$3::jsonb, finished_at=now() WHERE id=$1`, jobID, walkNote, b)
			return
		}
		b, _ := json.Marshal(out)
		_, _ = pool.Exec(ctx, `UPDATE snmp_walk_jobs SET status='done', result=$2::jsonb, error_message=NULL, finished_at=now() WHERE id=$1`, jobID, b)
	}(body.Host, body.Community, body.Version, body.Port, body.TimeoutMs, body.Retries, body.MaxRows)
	writeJSON(w, http.StatusAccepted, map[string]any{
		"job_id":   jid,
		"status":   "queued",
		"host":     body.Host,
		"root_oid": ifMibIfTable,
		"note":     "Walk de interfaces (IF-MIB 1.3.6.1.2.1.2.2.1) em segundo plano. Use GET /api/v1/tools/snmp-walk/jobs/{jobId}/rows e /discoveries.",
	})
}

func (s *Server) deviceChecks(w http.ResponseWriter, r *http.Request) {
	writeErr(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "Matriz de checks unificada não implementada.", nil)
}

func (s *Server) devicesExport(w http.ResponseWriter, r *http.Request) {
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "csv"
	}
	rows, err := s.DB().Query(r.Context(), `
		SELECT id::text, description, category, host(ip)::text, network_status, operational_mode FROM devices ORDER BY description LIMIT 2000
	`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	if format != "csv" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "format=csv suportado nesta versão", nil)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="devices_export.csv"`)
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"id", "description", "category", "ip", "network_status", "operational_mode"})
	for rows.Next() {
		var id, desc, cat, ns, op string
		var ipn *string
		if err := rows.Scan(&id, &desc, &cat, &ipn, &ns, &op); err != nil {
			return
		}
		ip := ""
		if ipn != nil {
			ip = *ipn
		}
		_ = cw.Write([]string{id, desc, cat, ip, ns, op})
	}
	cw.Flush()
}

func (s *Server) commercialReportsExport(w http.ResponseWriter, r *http.Request) {
	month := r.URL.Query().Get("month")
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "csv"
	}
	if format != "csv" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "use format=csv; pdf async planejado na doc", nil)
		return
	}
	q := `SELECT l.name, m.year_month, m.client_count FROM commercial_monthly_records m JOIN commercial_localities l ON l.id = m.locality_id`
	args := []any{}
	if month != "" {
		q += ` WHERE m.year_month = $1`
		args = append(args, month)
	}
	q += ` ORDER BY m.year_month DESC, l.name`
	rows, err := s.DB().Query(r.Context(), q, args...)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="commercial_%s.csv"`, strings.ReplaceAll(month, "-", "_")))
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"locality", "year_month", "client_count"})
	for rows.Next() {
		var loc, ym string
		var c int
		if err := rows.Scan(&loc, &ym, &c); err != nil {
			return
		}
		_ = cw.Write([]string{loc, ym, strconv.Itoa(c)})
	}
	cw.Flush()
}

func formatYearMonthPTCommercial(ym string) string {
	parts := strings.Split(strings.TrimSpace(ym), "-")
	if len(parts) != 2 {
		return strings.TrimSpace(ym)
	}
	y := strings.TrimSpace(parts[0])
	mi, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil || mi < 1 || mi > 12 {
		return strings.TrimSpace(ym)
	}
	meses := [...]string{
		"Janeiro", "Fevereiro", "Março", "Abril", "Maio", "Junho",
		"Julho", "Agosto", "Setembro", "Outubro", "Novembro", "Dezembro",
	}
	return meses[mi-1] + " de " + y
}

func escapeCommercialTelegramHTML(s string) string {
	return strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
	).Replace(s)
}

func wrapCommercialTelegramPre(plain string) string {
	return "<pre>" + escapeCommercialTelegramHTML(plain) + "</pre>"
}

func clipCommercialPlain(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max < 80 {
		return s
	}
	return s[:max-60] + "\n… (lista truncada — há mais localidades)\n"
}

func truncateRunesCommercial(s string, max int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}

func commercialTelegramPrevYearMonth(month string) (string, error) {
	t, err := time.Parse("2006-01", month)
	if err != nil {
		return "", err
	}
	return t.AddDate(0, -1, 0).Format("2006-01"), nil
}

func formatCommercialSignedInt(n int64) string {
	if n > 0 {
		return "+" + strconv.FormatInt(n, 10)
	}
	return strconv.FormatInt(n, 10)
}

// formatCommercialGrowthParen devolve texto tipo "(+12, +5,3%)" ou "(+12, —)" quando não há base no mês anterior.
func formatCommercialGrowthParen(cur, prev int64) string {
	delta := cur - prev
	growth := formatCommercialSignedInt(delta)
	var pctPart string
	if prev > 0 {
		pct := float64(delta) / float64(prev) * 100
		pc := fmt.Sprintf("%.1f", pct)
		pc = strings.ReplaceAll(pc, ".", ",")
		pctPart = pc + "%"
	} else {
		pctPart = "—"
	}
	return "(" + growth + ", " + pctPart + ")"
}

// commercialTelegramCompose gera HTML (bloco <pre>) com todos os registos mensais do mês (tópicos vs mês anterior).
// automatic altera apenas a linha de rodapé (relatório agendado vs envio manual).
func (s *Server) commercialTelegramCompose(ctx context.Context, month string, automatic bool) (string, error) {
	month = strings.TrimSpace(month)
	if !yearMonthCommercialRe.MatchString(month) {
		return "", fmt.Errorf("month deve estar em AAAA-MM")
	}

	prevMonth, err := commercialTelegramPrevYearMonth(month)
	if err != nil {
		return "", fmt.Errorf("month inválido")
	}

	q := `
		SELECT l.name, m.client_count::bigint, COALESCE(p.client_count, 0)::bigint
		FROM commercial_monthly_records m
		JOIN commercial_localities l ON l.id = m.locality_id
		LEFT JOIN commercial_monthly_records p ON p.locality_id = m.locality_id AND p.year_month = $2
		WHERE m.year_month = $1
		ORDER BY l.name ASC`

	rows, err := s.DB().Query(ctx, q, month, prevMonth)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var list []struct {
		name     string
		count    int64
		prevCount int64
	}
	var totalSum int64
	for rows.Next() {
		var name string
		var c, p int64
		if err := rows.Scan(&name, &c, &p); err != nil {
			return "", err
		}
		list = append(list, struct {
			name      string
			count     int64
			prevCount int64
		}{name: name, count: c, prevCount: p})
		totalSum += c
	}
	if err := rows.Err(); err != nil {
		return "", err
	}

	const lineW = 50
	var sb strings.Builder
	sb.WriteString("NetQuasar — Base comercial\n")
	sb.WriteString("Mês: ")
	sb.WriteString(formatYearMonthPTCommercial(month))
	sb.WriteString("  (")
	sb.WriteString(month)
	sb.WriteString(")\n")
	sb.WriteString("Comparativo vs ")
	sb.WriteString(formatYearMonthPTCommercial(prevMonth))
	sb.WriteString("  (")
	sb.WriteString(prevMonth)
	sb.WriteString(")\n")
	sb.WriteString(strings.Repeat("─", lineW))
	sb.WriteString("\n")

	if len(list) == 0 {
		sb.WriteString("Nenhum registo mensal para este mês.\n")
		sb.WriteString("Total: 0 clientes.\n")
	} else {
		sb.WriteString(fmt.Sprintf("Localidades com registo: %d\n", len(list)))
		sb.WriteString(fmt.Sprintf("Total de clientes:     %d\n", totalSum))
		sb.WriteString(strings.Repeat("─", lineW))
		sb.WriteString("\n\n")

		const reserve = 500
		for i, r := range list {
			nm := truncateRunesCommercial(r.name, 44)
			paren := formatCommercialGrowthParen(r.count, r.prevCount)
			block := fmt.Sprintf("• %s\n  %d clientes %s", nm, r.count, paren)
			if i < len(list)-1 {
				block += "\n\n"
			} else {
				block += "\n"
			}
			if sb.Len()+len(block) > telegramCommercialPlainMax-reserve {
				sb.WriteString("\n… (lista truncada)\n")
				break
			}
			sb.WriteString(block)
		}
		sb.WriteString(strings.Repeat("─", lineW))
		sb.WriteString("\n")
	}

	if automatic {
		sb.WriteString("\nEnviado automaticamente pelo NetQuasar (relatório ONU mensal / base comercial).\n")
	} else {
		sb.WriteString("\nEnviado manualmente pela aplicação NetQuasar.\n")
	}

	plain := clipCommercialPlain(sb.String(), telegramCommercialPlainMax)
	return wrapCommercialTelegramPre(plain), nil
}

var (
	yearMonthCommercialRe        = regexp.MustCompile(`^\d{4}-\d{2}$`)
	telegramCommercialPlainMax = 3600
)

func (s *Server) commercialReportsSendTelegram(w http.ResponseWriter, r *http.Request) {
	cfg, err := telegramclient.LoadConfig(r.Context(), s.DB(), "reports")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if !cfg.Ready() {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "Telegram reports não configurado (bot_token/chat_id).", nil)
		return
	}
	var body struct {
		Month string `json:"month"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	month := strings.TrimSpace(body.Month)
	if month == "" {
		month = strings.TrimSpace(r.URL.Query().Get("month"))
	}
	if month == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "month é obrigatório (formato AAAA-MM) para montar o relatório.", nil)
		return
	}
	text, compErr := s.commercialTelegramCompose(r.Context(), month, false)
	if compErr != nil {
		writeErr(w, http.StatusBadRequest, "VALIDATION", compErr.Error(), nil)
		return
	}
	if err := telegramclient.SendMessageWithParseMode(r.Context(), cfg, text, "HTML"); err != nil {
		writeErr(w, http.StatusBadGateway, "TELEGRAM_SEND_FAILED", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "commercial_report", month, "telegram_send", actorFromRequest(r), nil, map[string]any{"month": month})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "sent": true, "month": month})
}

func (s *Server) mapEquipmentPointDetail(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "deviceId"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	var desc, cat, op, netSt, brand, model, mac, serial, sw, hw string
	var lat, lon float64
	var ip *string
	var popID, locID *uuid.UUID
	var pingEn, telEn bool
	var updatedAt *time.Time
	err = s.DB().QueryRow(r.Context(), `
		SELECT d.description, d.category, d.latitude, d.longitude, host(d.ip)::text, d.pop_id, d.operational_mode,
			COALESCE(d.network_status,''), COALESCE(d.brand,''), COALESCE(d.model,''), COALESCE(d.mac,''), COALESCE(d.serial_number,''),
			COALESCE(d.software_version,''), COALESCE(d.hardware_version,''),
			d.ping_enabled, d.telemetry_enabled, d.locality_id, d.updated_at
		FROM devices d
		WHERE d.id=$1 AND d.latitude IS NOT NULL AND d.longitude IS NOT NULL
	`, id).Scan(&desc, &cat, &lat, &lon, &ip, &popID, &op, &netSt, &brand, &model, &mac, &serial, &sw, &hw, &pingEn, &telEn, &locID, &updatedAt)
	if err == pgx.ErrNoRows {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "sem coordenadas", nil)
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	st := "unknown"
	var ok sql.NullBool
	var checkedAt *time.Time
	_ = s.DB().QueryRow(r.Context(), `SELECT ok, checked_at FROM device_probe_cache WHERE device_id=$1`, id).Scan(&ok, &checkedAt)
	if ok.Valid {
		if ok.Bool {
			st = "online"
		} else {
			st = "offline"
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id": id, "description": desc, "category": cat, "lat": lat, "lng": lon, "ip": ip, "pop_id": popID,
		"operational_mode": op, "status": st, "network_status": netSt, "brand": brand, "model": model,
		"mac": mac, "serial_number": serial, "software_version": sw, "hardware_version": hw,
		"ping_enabled": pingEn, "telemetry_enabled": telEn, "locality_id": locID, "updated_at": updatedAt,
		"last_check_at": checkedAt,
	})
}

func (s *Server) overviewTopLatency(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 20 {
		limit = 5
	}
	rows, err := s.DB().Query(r.Context(), `
		SELECT d.id, d.description, host(d.ip)::text, c.latency_ms
		FROM device_probe_cache c JOIN devices d ON d.id = c.device_id
		WHERE c.latency_ms IS NOT NULL ORDER BY c.latency_ms DESC LIMIT $1
	`, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	var list []map[string]any
	for rows.Next() {
		var id uuid.UUID
		var desc string
		var ip *string
		var ms int64
		if err := rows.Scan(&id, &desc, &ip, &ms); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		list = append(list, map[string]any{"device_id": id, "description": desc, "ip": ip, "latency_ms": ms})
	}
	writeJSON(w, http.StatusOK, map[string]any{"top": list})
}

func (s *Server) monitoringFullReportDevice(w http.ResponseWriter, r *http.Request) {
	s.setMonitoringActivity(r.Context(), "Executando relatório individual do equipamento")
	defer s.setMonitoringActivity(r.Context(), "")
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	var ip, category string
	var telemetryEnabled bool
	var devComm, defComm *string
	if err := s.DB().QueryRow(r.Context(), `SELECT host(ip)::text, category, telemetry_enabled, snmp_community FROM devices WHERE id=$1`, id).
		Scan(&ip, &category, &telemetryEnabled, &devComm); err != nil {
		if err == pgx.ErrNoRows {
			writeErr(w, http.StatusNotFound, "NOT_FOUND", "equipamento não encontrado", nil)
			return
		}
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	_ = s.DB().QueryRow(r.Context(), `SELECT snmp_community FROM settings_connection_defaults WHERE id=1`).Scan(&defComm)
	community := ""
	if devComm != nil && strings.TrimSpace(*devComm) != "" {
		community = strings.TrimSpace(*devComm)
	} else if defComm != nil {
		community = strings.TrimSpace(*defComm)
	}

	report := map[string]any{
		"device_id":         id,
		"category":          category,
		"ip":                ip,
		"telemetry_enabled": telemetryEnabled,
		"timestamp":         time.Now().UTC(),
	}
	status := "done"

	if telemetryEnabled {
		if community == "" {
			report["telemetry"] = map[string]any{"ok": false, "error": "snmp_community não configurada"}
			status = "partial"
		} else {
			ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
			invStatus, invErr := snmpdiscovery.EnsureFreshInventory(ctx, s.DB(), &s.Log, id, snmpdiscovery.DefaultInventoryMaxAge)
			if invErr != nil {
				report["snmp_inventory"] = map[string]any{"ok": false, "error": invErr.Error()}
			} else {
				report["snmp_inventory"] = map[string]any{"ok": true, "status": invStatus}
			}
			col, err := telemetryengine.CollectAndStore(ctx, s.DB(), id, ip, community)
			cancel()
			if err != nil {
				report["telemetry"] = map[string]any{"ok": false, "error": err.Error()}
				status = "partial"
			} else {
				report["telemetry"] = map[string]any{"ok": col.OK, "oid_count": len(col.OIDs), "snmp": col.SNMP}
				if !col.OK {
					status = "partial"
				}
			}
		}
	} else {
		report["telemetry"] = map[string]any{"ok": false, "skipped": true, "reason": "telemetry_enabled=false"}
	}

	if community != "" {
		s.setMonitoringActivity(r.Context(), "Coletando interfaces completas no relatório individual")
		if err := callInternalDevicePost(r.Context(), s.refreshDeviceInterfaces, "id", id); err != nil {
			report["interfaces"] = map[string]any{"ok": false, "error": "falha ao executar refresh completo de interfaces"}
			status = "partial"
		} else {
			report["interfaces"] = map[string]any{"ok": true, "note": "snapshot de interfaces atualizado via refresh completo"}
		}
	} else {
		report["interfaces"] = map[string]any{"ok": false, "skipped": true, "reason": "sem snmp_community"}
	}

	if strings.EqualFold(strings.TrimSpace(category), "OLT") {
		s.setMonitoringActivity(r.Context(), "Coletando PONs OLT no relatório individual")
		if err := callInternalDevicePost(r.Context(), s.refreshOLTDevice, "id", id); err != nil {
			report["olt"] = map[string]any{"ok": false, "error": "falha ao executar refresh OLT"}
			status = "partial"
		} else {
			report["olt"] = map[string]any{"ok": true, "note": "snapshot OLT atualizado via refresh OLT"}
		}
	}

	report["status"] = status
	writeJSON(w, http.StatusOK, report)
}

func (s *Server) settingsDatabaseLogs(w http.ResponseWriter, r *http.Request) {
	p := s.DB()
	if p == nil {
		writeErr(w, http.StatusServiceUnavailable, "NO_DB", "pool não configurado", nil)
		return
	}
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	rows, err := p.Query(r.Context(), `
		SELECT id, created_at, ok, phase, message, target_host, target_port, target_db
		FROM settings_connection_audit
		ORDER BY id DESC
		LIMIT $1
	`, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	var logs []map[string]any
	for rows.Next() {
		var id int64
		var created time.Time
		var ok bool
		var phase, msg string
		var th, tdb *string
		var tp sql.NullInt64
		if err := rows.Scan(&id, &created, &ok, &phase, &msg, &th, &tp, &tdb); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		var port any
		if tp.Valid {
			port = int(tp.Int64)
		}
		logs = append(logs, map[string]any{
			"id": id, "created_at": created, "ok": ok, "phase": phase, "message": msg,
			"target_host": th, "target_port": port, "target_db": tdb,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"logs": logs})
}
