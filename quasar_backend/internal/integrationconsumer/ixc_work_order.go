package integrationconsumer

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/netquasar/netquasar/quasar_backend/internal/integrationhttp"
)

// IXCWorkOrderBuscaToQtype mapeia busca da UI para qtype IXC em su_oss_chamado.
func IXCWorkOrderBuscaToQtype(busca string) string {
	switch strings.ToLower(strings.TrimSpace(busca)) {
	case "codigo_cliente":
		return "su_oss_chamado.id_cliente"
	case "cpf_cnpj":
		return "cnpj_cpf"
	case "id_cliente_servico":
		return "id_contrato"
	case "numero_ordem_servico":
		return "su_oss_chamado.id"
	default:
		return "su_oss_chamado.id_cliente"
	}
}

func IXCWorkOrderOperForBusca(busca string) string {
	switch strings.ToLower(strings.TrimSpace(busca)) {
	case "nome_razaosocial", "numero_ordem_servico":
		return "L"
	default:
		return "="
	}
}

func mergeWorkOrderConfig(woCfg ClientWorkOrderConfig, searchCfg ClientSearchConfig) ClientWorkOrderConfig {
	if len(woCfg.FieldMappings) > 0 {
		return woCfg
	}
	if len(searchCfg.FieldMappings) == 0 {
		return woCfg
	}
	out := woCfg
	out.FieldMappings = searchCfg.FieldMappings
	return out
}

func (cfg ClientWorkOrderConfig) workOrderMapping(busca string) SearchFieldConfig {
	if cfg.FieldMappings == nil {
		return SearchFieldConfig{}
	}
	if m, ok := cfg.FieldMappings[strings.ToLower(strings.TrimSpace(busca))]; ok {
		return m
	}
	return SearchFieldConfig{}
}

func (cfg ClientWorkOrderConfig) ResolveWorkOrderQtype(busca string) string {
	qtype := ""
	if m := cfg.workOrderMapping(busca); m.Qtype != "" {
		qtype = m.Qtype
	} else {
		qtype = IXCWorkOrderBuscaToQtype(busca)
	}
	return normalizeIXCWorkOrderQtype(qtype, busca)
}

func (cfg ClientWorkOrderConfig) ResolveWorkOrderOper(busca string) string {
	if m := cfg.workOrderMapping(busca); m.Oper != "" {
		return m.Oper
	}
	return IXCWorkOrderOperForBusca(busca)
}

func (cfg ClientWorkOrderConfig) ResolveWorkOrderTermo(busca, termo string) string {
	format := cfg.workOrderMapping(busca).TermoFormat
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

func (cfg ClientWorkOrderConfig) ixcListAction(searchCfg ClientSearchConfig) string {
	if v := strings.TrimSpace(cfg.IxcListAction); v != "" {
		return v
	}
	return searchCfg.IxcListHeaderValue()
}

func normalizeIXCWorkOrderQtype(qtype, busca string) string {
	qtype = strings.TrimSpace(qtype)
	busca = strings.ToLower(strings.TrimSpace(busca))
	if qtype == "" {
		return qtype
	}
	low := strings.ToLower(qtype)
	if busca == "codigo_cliente" && (low == "su_oss_chamado.id" || low == "id") {
		return "su_oss_chamado.id_cliente"
	}
	if low == "id_cliente" || low == "cliente.id_cliente" {
		return "su_oss_chamado.id_cliente"
	}
	if !strings.Contains(low, "su_oss_chamado") && strings.Contains(low, "id_cliente") {
		return "su_oss_chamado.id_cliente"
	}
	if low == "id_contrato" && busca == "id_cliente_servico" {
		return "su_oss_chamado.id_contrato"
	}
	return qtype
}

func isIXCWorkOrderClientScopedQtype(qtype string) bool {
	low := strings.ToLower(strings.TrimSpace(qtype))
	switch low {
	case "su_oss_chamado.id_cliente", "id_cliente", "su_oss_chamado.id_contrato", "id_contrato",
		"su_oss_chamado.cnpj_cpf", "cnpj_cpf":
		return true
	default:
		return strings.Contains(low, "id_cliente") || strings.Contains(low, "cnpj_cpf") || strings.Contains(low, "id_contrato")
	}
}

// LooksLikeIXCWorkOrderRequest indica listagem IXC de O.S. (su_oss_chamado).
func LooksLikeIXCWorkOrderRequest(rc integrationhttp.RequestConfig) bool {
	path := strings.ToLower(strings.TrimSpace(rc.Path))
	if strings.Contains(path, "su_oss_chamado") || strings.Contains(path, "oss_chamado") {
		return true
	}
	for k, v := range rc.Headers {
		if strings.EqualFold(k, "ixcsoft") && strings.TrimSpace(v) != "" {
			body := strings.ToLower(rc.BodyTemplate)
			if strings.Contains(body, "su_oss_chamado") || strings.Contains(body, "oss_chamado") {
				return true
			}
		}
	}
	return false
}

// DetectWorkOrderProfile infere ERP para ordens de serviço.
func DetectWorkOrderProfile(configured string, rc integrationhttp.RequestConfig, baseURL string) string {
	if LooksLikeIXCWorkOrderRequest(rc) || LooksLikeIXCRequest(rc, baseURL) {
		return ProviderIXC
	}
	configured = strings.ToLower(strings.TrimSpace(configured))
	switch configured {
	case ProviderHubsoft, ProviderIXC, ProviderGeneric:
		return configured
	}
	path := strings.ToLower(strings.TrimSpace(rc.Path))
	if strings.Contains(path, "integracao/cliente/ordem_servico") {
		return ProviderHubsoft
	}
	return ProviderGeneric
}

func ApplyIXCWorkOrderBodySearchAttempt(bodyTemplate string, att IXCSearchAttempt) string {
	obj := map[string]any{}
	if strings.TrimSpace(bodyTemplate) != "" {
		_ = json.Unmarshal([]byte(bodyTemplate), &obj)
	}
	if obj == nil {
		obj = map[string]any{}
	}
	obj["qtype"] = normalizeIXCWorkOrderQtype(att.Qtype, "codigo_cliente")
	obj["query"] = att.Query
	obj["oper"] = att.Oper
	if _, ok := obj["sortname"]; !ok {
		obj["sortname"] = "su_oss_chamado.id"
	}
	if _, ok := obj["sortorder"]; !ok {
		obj["sortorder"] = "asc"
	}
	if _, ok := obj["page"]; !ok {
		obj["page"] = "1"
	}
	if v, ok := obj["rp"]; !ok || strings.TrimSpace(fmtAny(v)) == "" {
		obj["rp"] = "20"
	}
	raw, err := json.Marshal(obj)
	if err != nil {
		return bodyTemplate
	}
	return string(raw)
}

func BuildIXCWorkOrderAttempts(cfg ClientWorkOrderConfig, busca, termo, baseBody string) []IXCSearchAttempt {
	busca = strings.ToLower(strings.TrimSpace(busca))
	d := digitsOnly(termo)
	t := strings.TrimSpace(termo)
	var attempts []IXCSearchAttempt
	add := func(qtype, query, oper string) {
		query = strings.TrimSpace(query)
		if query == "" {
			return
		}
		qtype = normalizeIXCWorkOrderQtype(qtype, busca)
		attempts = append(attempts, IXCSearchAttempt{Qtype: qtype, Query: query, Oper: oper})
	}
	if tq := ixcBodyTemplateQtype(baseBody); tq != "" && busca == "codigo_cliente" {
		add(tq, t, "=")
	}
	add(cfg.ResolveWorkOrderQtype(busca), cfg.ResolveWorkOrderTermo(busca, termo), "=")
	switch busca {
	case "codigo_cliente":
		add("su_oss_chamado.id_cliente", t, "=")
		add("id_cliente", t, "=")
		add("su_oss_chamado.id_cliente", d, "=")
	case "cpf_cnpj":
		add("su_oss_chamado.cnpj_cpf", d, "=")
		add("cnpj_cpf", d, "=")
	case "id_cliente_servico":
		add("su_oss_chamado.id_contrato", t, "=")
		add("id_contrato", t, "=")
	}
	return dedupeIXCAttempts(attempts)
}

func applyIXCWorkOrderListAllBody(bodyTemplate, rp, page string) string {
	obj := map[string]any{}
	if strings.TrimSpace(bodyTemplate) != "" {
		_ = json.Unmarshal([]byte(bodyTemplate), &obj)
	}
	if obj == nil {
		obj = map[string]any{}
	}
	obj["qtype"] = "su_oss_chamado.id"
	obj["query"] = "0"
	obj["oper"] = ">"
	if _, ok := obj["sortname"]; !ok {
		obj["sortname"] = "su_oss_chamado.id"
	}
	if _, ok := obj["sortorder"]; !ok {
		obj["sortorder"] = "asc"
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

func FilterWorkOrderByClient(items []WorkOrderItem, busca, termo string) []WorkOrderItem {
	termo = strings.TrimSpace(termo)
	if termo == "" {
		return items
	}
	digits := digitsOnly(termo)
	var out []WorkOrderItem
	for _, it := range items {
		if workOrderMatchesClient(it, busca, termo, digits) {
			out = append(out, it)
		}
	}
	return out
}

func workOrderMatchesClient(it WorkOrderItem, busca, termo, digits string) bool {
	if it.Raw == nil {
		return false
	}
	m := make(map[string]any, len(it.Raw))
	for k, v := range it.Raw {
		m[k] = v
	}
	clientKeys := []string{"id_cliente", "id_login", "id_contrato"}
	if strings.ToLower(strings.TrimSpace(busca)) == "cpf_cnpj" {
		for _, k := range []string{"cnpj_cpf", "cpf_cnpj", "cpf", "cnpj", "documento"} {
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
	return false
}

func prepareIXCBaseWorkOrder(rc integrationhttp.RequestConfig, woCfg ClientWorkOrderConfig, searchCfg ClientSearchConfig) (integrationhttp.RequestConfig, string) {
	rc = ensureIXCListarHeader(rc, woCfg.ixcListAction(searchCfg))
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
		rc.Path = "/su_oss_chamado"
	}
	return rc, baseBody
}

func countIXCWorkOrderRegistros(raw []byte) int {
	doc, err := decodeJSONDocument(raw)
	if err != nil {
		return 0
	}
	return len(extractWorkOrderArray(doc))
}

// RunIXCWorkOrderWithAttempts executa POST su_oss_chamado com filtro por cliente.
func RunIXCWorkOrderWithAttempts(
	rc integrationhttp.RequestConfig,
	busca, termo string,
	woCfg ClientWorkOrderConfig,
	searchCfg ClientSearchConfig,
	execute func(rc integrationhttp.RequestConfig) integrationhttp.RunResult,
) (integrationhttp.RunResult, WorkOrderResult) {
	woCfg = mergeWorkOrderConfig(woCfg, searchCfg)
	rc, baseBody := prepareIXCBaseWorkOrder(rc, woCfg, searchCfg)
	attempts := BuildIXCWorkOrderAttempts(woCfg, busca, termo, baseBody)
	var lastRes integrationhttp.RunResult
	lastParsed := WorkOrderResult{Items: []WorkOrderItem{}, Message: "Nenhuma ordem de serviço encontrada."}
	for _, att := range attempts {
		rc.BodyTemplate = ApplyIXCWorkOrderBodySearchAttempt(baseBody, att)
		lastRes = execute(rc)
		raw := ResponseBodyForParse([]byte(lastRes.ResponsePreview))
		parsed := ParseIXCClientWorkOrder(raw)
		trustAPI := att.Oper == "=" && isIXCWorkOrderClientScopedQtype(att.Qtype) && len(parsed.Items) > 0
		if !trustAPI {
			parsed.Items = FilterWorkOrderByClient(parsed.Items, busca, termo)
		}
		if parsed.OK && len(parsed.Items) > 0 {
			if parsed.Message == "Nenhuma ordem de serviço encontrada." {
				parsed.Message = ""
			}
			return lastRes, parsed
		}
		lastParsed = parsed
	}
	const pageSize = "100"
	for page := 1; page <= 20; page++ {
		rc.BodyTemplate = applyIXCWorkOrderListAllBody(baseBody, pageSize, fmt.Sprint(page))
		lastRes = execute(rc)
		raw := ResponseBodyForParse([]byte(lastRes.ResponsePreview))
		lastParsed = ParseIXCClientWorkOrder(raw)
		lastParsed.Items = FilterWorkOrderByClient(lastParsed.Items, busca, termo)
		if len(lastParsed.Items) > 0 {
			lastParsed.OK = true
			lastParsed.Message = ""
			return lastRes, lastParsed
		}
		if n := countIXCWorkOrderRegistros(raw); n == 0 || n < atoiDefault(pageSize, 100) {
			break
		}
	}
	if lastParsed.Message == "" || lastParsed.Message == "Nenhuma ordem de serviço encontrada." {
		lastParsed.Message = "Nenhuma ordem de serviço encontrada para este cliente."
	}
	return lastRes, lastParsed
}
