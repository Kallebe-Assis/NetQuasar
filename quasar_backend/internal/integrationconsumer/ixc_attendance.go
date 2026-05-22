package integrationconsumer

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/netquasar/netquasar/quasar_backend/internal/integrationhttp"
)

// IXCAttendanceBuscaToQtype mapeia busca da UI para qtype IXC em su_ticket.
func IXCAttendanceBuscaToQtype(busca string) string {
	switch strings.ToLower(strings.TrimSpace(busca)) {
	case "codigo_cliente":
		return "su_ticket.id_cliente"
	case "cpf_cnpj":
		return "cnpj_cpf"
	case "id_cliente_servico":
		return "id_contrato"
	case "protocolo":
		return "protocolo"
	default:
		return "su_ticket.id_cliente"
	}
}

// IXCAttendanceOperForBusca operador padrão por tipo de busca em tickets.
func IXCAttendanceOperForBusca(busca string) string {
	switch strings.ToLower(strings.TrimSpace(busca)) {
	case "nome_razaosocial", "protocolo":
		return "L"
	default:
		return "="
	}
}

func mergeAttendanceConfig(attCfg ClientAttendanceConfig, searchCfg ClientSearchConfig) ClientAttendanceConfig {
	if len(attCfg.FieldMappings) > 0 {
		return attCfg
	}
	if len(searchCfg.FieldMappings) == 0 {
		return attCfg
	}
	out := attCfg
	out.FieldMappings = searchCfg.FieldMappings
	return out
}

func (cfg ClientAttendanceConfig) attendanceMapping(busca string) SearchFieldConfig {
	if cfg.FieldMappings == nil {
		return SearchFieldConfig{}
	}
	if m, ok := cfg.FieldMappings[strings.ToLower(strings.TrimSpace(busca))]; ok {
		return m
	}
	return SearchFieldConfig{}
}

func (cfg ClientAttendanceConfig) ResolveAttendanceQtype(busca string) string {
	qtype := ""
	if m := cfg.attendanceMapping(busca); m.Qtype != "" {
		qtype = m.Qtype
	} else {
		qtype = IXCAttendanceBuscaToQtype(busca)
	}
	return normalizeIXCAttendanceQtype(qtype, busca)
}

func (cfg ClientAttendanceConfig) ResolveAttendanceOper(busca string) string {
	if m := cfg.attendanceMapping(busca); m.Oper != "" {
		return m.Oper
	}
	return IXCAttendanceOperForBusca(busca)
}

func (cfg ClientAttendanceConfig) ResolveAttendanceTermo(busca, termo string) string {
	format := cfg.attendanceMapping(busca).TermoFormat
	if format == "" {
		if strings.ToLower(strings.TrimSpace(busca)) == "cpf_cnpj" {
			return digitsOnly(termo)
		}
		return strings.TrimSpace(termo)
	}
	termo = strings.TrimSpace(termo)
	switch strings.ToLower(format) {
	case "digits":
		return digitsOnly(termo)
	case "br_document":
		d := digitsOnly(termo)
		if fd := formatBRDocument(d); fd != "" {
			return fd
		}
		return d
	default:
		return termo
	}
}

func (cfg ClientAttendanceConfig) ixcListAction(searchCfg ClientSearchConfig) string {
	if v := strings.TrimSpace(cfg.IxcListAction); v != "" {
		return v
	}
	return searchCfg.IxcListHeaderValue()
}

// PrepareIXCAttendanceListRequest POST su_ticket + ixcsoft:listar + filtro no body.
func PrepareIXCAttendanceListRequest(
	rc integrationhttp.RequestConfig,
	busca, termo string,
	attCfg ClientAttendanceConfig,
	searchCfg ClientSearchConfig,
) integrationhttp.RequestConfig {
	rc = ensureIXCListarHeader(rc, attCfg.ixcListAction(searchCfg))
	method := strings.ToUpper(strings.TrimSpace(rc.Method))
	if method == "" || method == "GET" {
		rc.Method = "POST"
	}
	baseBody := strings.TrimSpace(rc.BodyTemplate)
	if baseBody == "" {
		baseBody = ixcBodyFromQueryParams(rc.QueryParams)
	}
	rc.QueryParams = nil
	rc.BodyTemplate = applyIXCAttendanceBody(baseBody, busca, termo, attCfg)
	if strings.TrimSpace(rc.BodyType) == "" {
		rc.BodyType = "json"
	}
	if p := strings.TrimSpace(rc.Path); p == "" {
		rc.Path = "/su_ticket"
	}
	return rc
}

func ixcBodyTemplateQtype(bodyTemplate string) string {
	obj := map[string]any{}
	if strings.TrimSpace(bodyTemplate) == "" {
		return ""
	}
	if err := json.Unmarshal([]byte(bodyTemplate), &obj); err != nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(obj["qtype"]))
}

func applyIXCAttendanceBody(bodyTemplate, busca, termo string, cfg ClientAttendanceConfig) string {
	obj := map[string]any{}
	if strings.TrimSpace(bodyTemplate) != "" {
		_ = json.Unmarshal([]byte(bodyTemplate), &obj)
	}
	if obj == nil {
		obj = map[string]any{}
	}
	qtype := cfg.ResolveAttendanceQtype(busca)
	if tq := ixcBodyTemplateQtype(bodyTemplate); tq != "" {
		qtype = tq
	}
	obj["qtype"] = normalizeIXCAttendanceQtype(qtype, busca)
	obj["query"] = cfg.ResolveAttendanceTermo(busca, termo)
	oper := cfg.ResolveAttendanceOper(busca)
	if strings.ToLower(strings.TrimSpace(busca)) == "codigo_cliente" {
		oper = "="
	}
	obj["oper"] = oper
	if _, ok := obj["sortname"]; !ok {
		obj["sortname"] = "su_ticket.id"
	}
	if _, ok := obj["sortorder"]; !ok {
		obj["sortorder"] = "desc"
	}
	if _, ok := obj["page"]; !ok {
		obj["page"] = "1"
	}
	if v, ok := obj["rp"]; !ok || strings.TrimSpace(fmtAny(v)) == "" {
		obj["rp"] = "50"
	}
	raw, err := json.Marshal(obj)
	if err != nil {
		return bodyTemplate
	}
	return string(raw)
}

// BuildIXCAttendanceAttempts variantes de filtro IXC; se falharem, usa-se listagem ampla + filtro local.
func BuildIXCAttendanceAttempts(cfg ClientAttendanceConfig, busca, termo, baseBody string) []IXCSearchAttempt {
	busca = strings.ToLower(strings.TrimSpace(busca))
	d := digitsOnly(termo)
	t := strings.TrimSpace(termo)
	var attempts []IXCSearchAttempt
	add := func(qtype, query, oper string) {
		query = strings.TrimSpace(query)
		if query == "" {
			return
		}
		qtype = normalizeIXCAttendanceQtype(qtype, busca)
		attempts = append(attempts, IXCSearchAttempt{Qtype: qtype, Query: query, Oper: oper})
	}
	if tq := ixcBodyTemplateQtype(baseBody); tq != "" && busca == "codigo_cliente" {
		add(tq, t, "=")
	}
	add(cfg.ResolveAttendanceQtype(busca), cfg.ResolveAttendanceTermo(busca, termo), "=")
	switch busca {
	case "codigo_cliente":
		add("su_ticket.id_cliente", t, "=")
		add("id_cliente", t, "=")
		add("su_ticket.id_cliente", d, "=")
		add("id_cliente", d, "=")
	case "cpf_cnpj":
		add("su_ticket.cnpj_cpf", d, "=")
		add("cnpj_cpf", d, "=")
		if fd := formatBRDocument(d); fd != d {
			add("su_ticket.cnpj_cpf", fd, "=")
			add("cnpj_cpf", fd, "=")
		}
		add("su_ticket.cnpj_cpf", d, "L")
	case "id_cliente_servico":
		add("su_ticket.id_contrato", t, "=")
		add("id_contrato", t, "=")
	}
	return dedupeIXCAttempts(attempts)
}

func applyIXCAttendanceListAllBody(bodyTemplate, rp string, page string) string {
	obj := map[string]any{}
	if strings.TrimSpace(bodyTemplate) != "" {
		_ = json.Unmarshal([]byte(bodyTemplate), &obj)
	}
	if obj == nil {
		obj = map[string]any{}
	}
	obj["qtype"] = "su_ticket.id"
	obj["query"] = "0"
	obj["oper"] = ">"
	if _, ok := obj["sortname"]; !ok {
		obj["sortname"] = "su_ticket.id"
	}
	if _, ok := obj["sortorder"]; !ok {
		obj["sortorder"] = "desc"
	}
	if page != "" {
		obj["page"] = page
	} else if _, ok := obj["page"]; !ok {
		obj["page"] = "1"
	}
	obj["rp"] = rp
	raw, _ := json.Marshal(obj)
	return string(raw)
}

// normalizeIXCAttendanceQtype ajusta qtype do mapeamento de /cliente para su_ticket (ex.: id_cliente → su_ticket.id_cliente).
func normalizeIXCAttendanceQtype(qtype, busca string) string {
	qtype = strings.TrimSpace(qtype)
	busca = strings.ToLower(strings.TrimSpace(busca))
	if qtype == "" {
		return qtype
	}
	low := strings.ToLower(qtype)
	if busca == "codigo_cliente" && (low == "su_ticket.id" || low == "id") {
		return "su_ticket.id_cliente"
	}
	if low == "id_cliente" || low == "cliente.id_cliente" {
		return "su_ticket.id_cliente"
	}
	if low == "id_contrato" && busca == "id_cliente_servico" {
		return "su_ticket.id_contrato"
	}
	if low == "cnpj_cpf" || low == "cliente.cnpj_cpf" {
		return "su_ticket.cnpj_cpf"
	}
	return qtype
}

func isIXCClientScopedQtype(qtype string) bool {
	low := strings.ToLower(strings.TrimSpace(qtype))
	switch low {
	case "su_ticket.id_cliente", "id_cliente", "su_ticket.id_contrato", "id_contrato",
		"su_ticket.cnpj_cpf", "cnpj_cpf", "su_ticket.id_login", "id_login":
		return true
	default:
		return strings.Contains(low, "id_cliente") || strings.Contains(low, "cnpj_cpf") || strings.Contains(low, "id_contrato")
	}
}

func attendanceItemsHaveClientKey(items []AttendanceItem) bool {
	for _, it := range items {
		m := attendanceItemRawMap(it)
		if m == nil {
			continue
		}
		if pickStr(m, "id_cliente", "id_login", "id_contrato") != "" {
			return true
		}
	}
	return false
}

// ApplyIXCAttendanceBodySearchAttempt aplica tentativa ao body su_ticket (preserva sortname do template).
func ApplyIXCAttendanceBodySearchAttempt(bodyTemplate string, att IXCSearchAttempt) string {
	obj := map[string]any{}
	if strings.TrimSpace(bodyTemplate) != "" {
		_ = json.Unmarshal([]byte(bodyTemplate), &obj)
	}
	if obj == nil {
		obj = map[string]any{}
	}
	obj["qtype"] = normalizeIXCAttendanceQtype(att.Qtype, "codigo_cliente")
	obj["query"] = att.Query
	obj["oper"] = att.Oper
	if _, ok := obj["sortname"]; !ok {
		obj["sortname"] = "su_ticket.id"
	}
	if _, ok := obj["sortorder"]; !ok {
		obj["sortorder"] = "desc"
	}
	if _, ok := obj["page"]; !ok {
		obj["page"] = "1"
	}
	if v, ok := obj["rp"]; !ok || strings.TrimSpace(fmtAny(v)) == "" {
		obj["rp"] = "50"
	}
	raw, err := json.Marshal(obj)
	if err != nil {
		return bodyTemplate
	}
	return string(raw)
}

// FilterAttendanceByClient mantém tickets cujo registro IXC pertence ao cliente consultado.
func FilterAttendanceByClient(items []AttendanceItem, busca, termo string) []AttendanceItem {
	termo = strings.TrimSpace(termo)
	if termo == "" {
		return items
	}
	digits := digitsOnly(termo)
	var out []AttendanceItem
	for _, it := range items {
		if attendanceMatchesClient(it, busca, termo, digits) {
			out = append(out, it)
		}
	}
	return out
}

func attendanceMatchesClient(it AttendanceItem, busca, termo, digits string) bool {
	m := attendanceItemRawMap(it)
	if m == nil {
		return false
	}
	// Campo oficial IXC su_ticket (documentação).
	clientKeys := []string{"id_cliente", "id_login", "id_contrato", "id_circuito"}
	docKeys := []string{"cnpj_cpf", "cpf_cnpj", "cpf", "cnpj", "documento", "cnpj_cpf_cliente"}
	if strings.ToLower(strings.TrimSpace(busca)) == "cpf_cnpj" {
		for _, k := range docKeys {
			if fieldMatchesTerm(pickStr(m, k), termo, digits) {
				return true
			}
		}
	}
	for _, k := range clientKeys {
		if fieldMatchesTerm(pickStr(m, k), termo, digits) {
			return true
		}
	}
	// Contrato / serviço
	if strings.ToLower(strings.TrimSpace(busca)) == "id_cliente_servico" {
		for _, k := range []string{"id_contrato", "id_cliente_servico", "id_servico", "id_contrato_cliente"} {
			if fieldMatchesTerm(pickStr(m, k), termo, digits) {
				return true
			}
		}
	}
	return false
}

func attendanceItemRawMap(it AttendanceItem) map[string]any {
	if it.Raw == nil {
		return nil
	}
	out := make(map[string]any, len(it.Raw))
	for k, v := range it.Raw {
		out[k] = v
	}
	return out
}

func fieldMatchesTerm(value, termo, digits string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	if value == termo {
		return true
	}
	vd := digitsOnly(value)
	if digits != "" && vd == digits {
		return true
	}
	if digits != "" && vd != "" && (strings.HasSuffix(vd, digits) || strings.HasSuffix(digits, vd)) {
		return len(digits) >= 4
	}
	return false
}

// RunIXCAttendanceWithAttempts executa POST su_ticket com filtro API ou listagem + filtro local.
func RunIXCAttendanceWithAttempts(
	rc integrationhttp.RequestConfig,
	busca, termo string,
	attCfg ClientAttendanceConfig,
	searchCfg ClientSearchConfig,
	execute func(rc integrationhttp.RequestConfig) integrationhttp.RunResult,
) (integrationhttp.RunResult, AttendanceResult) {
	attCfg = mergeAttendanceConfig(attCfg, searchCfg)
	rc, baseBody := prepareIXCBaseAttendance(rc, attCfg, searchCfg)
	attempts := BuildIXCAttendanceAttempts(attCfg, busca, termo, baseBody)
	var lastRes integrationhttp.RunResult
	lastParsed := AttendanceResult{Items: []AttendanceItem{}, Message: "Nenhum atendimento encontrado."}
	for _, att := range attempts {
		rc.BodyTemplate = ApplyIXCAttendanceBodySearchAttempt(baseBody, att)
		lastRes = execute(rc)
		raw := ResponseBodyForParse([]byte(lastRes.ResponsePreview))
		parsed := ParseIXCClientAttendance(raw)
		trustAPI := att.Oper == "=" && isIXCClientScopedQtype(att.Qtype) && len(parsed.Items) > 0
		if !trustAPI {
			parsed.Items = FilterAttendanceByClient(parsed.Items, busca, termo)
		}
		if parsed.OK && len(parsed.Items) > 0 {
			if parsed.Message == "Nenhum atendimento encontrado." {
				parsed.Message = ""
			}
			return lastRes, parsed
		}
		lastParsed = parsed
	}
	// Fallback: listagem ampla (como no teste query=0) e filtra por id_cliente no registro.
	const pageSize = "100"
	for page := 1; page <= 20; page++ {
		rc.BodyTemplate = applyIXCAttendanceListAllBody(baseBody, pageSize, fmt.Sprint(page))
		lastRes = execute(rc)
		raw := ResponseBodyForParse([]byte(lastRes.ResponsePreview))
		lastParsed = ParseIXCClientAttendance(raw)
		lastParsed.Items = FilterAttendanceByClient(lastParsed.Items, busca, termo)
		if len(lastParsed.Items) > 0 {
			lastParsed.OK = true
			lastParsed.Message = ""
			return lastRes, lastParsed
		}
		if n := countIXCRegistrosInResponse(raw); n == 0 || n < atoiDefault(pageSize, 100) {
			break
		}
	}
	if lastParsed.Message == "" || lastParsed.Message == "Nenhum atendimento encontrado." {
		lastParsed.Message = "Nenhum atendimento encontrado para este cliente."
	}
	return lastRes, lastParsed
}

func countIXCRegistrosInResponse(raw []byte) int {
	doc, err := decodeJSONDocument(raw)
	if err != nil {
		return 0
	}
	return len(extractAttendanceArray(doc))
}

func atoiDefault(s string, def int) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	var n int
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		return def
	}
	return n
}

func prepareIXCBaseAttendance(rc integrationhttp.RequestConfig, attCfg ClientAttendanceConfig, searchCfg ClientSearchConfig) (integrationhttp.RequestConfig, string) {
	rc = ensureIXCListarHeader(rc, attCfg.ixcListAction(searchCfg))
	method := strings.ToUpper(strings.TrimSpace(rc.Method))
	if method == "" || method == "GET" {
		rc.Method = "POST"
	}
	baseBody := strings.TrimSpace(rc.BodyTemplate)
	if baseBody == "" {
		baseBody = ixcBodyFromQueryParams(rc.QueryParams)
	}
	rc.QueryParams = nil
	if strings.TrimSpace(rc.BodyType) == "" {
		rc.BodyType = "json"
	}
	if p := strings.TrimSpace(rc.Path); p == "" {
		rc.Path = "/su_ticket"
	}
	return rc, baseBody
}
