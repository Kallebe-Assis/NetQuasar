package integrationconsumer

import (
	"encoding/json"
	"strings"

	"github.com/netquasar/netquasar/quasar_backend/internal/integrationhttp"
)

// IXCBuscaToQtype mapeia o tipo de busca da UI para qtype IXC.
func IXCBuscaToQtype(busca string) string {
	switch strings.ToLower(strings.TrimSpace(busca)) {
	case "cpf_cnpj":
		return "cliente.cnpj_cpf"
	case "nome_razaosocial":
		return "razao"
	case "nome_fantasia":
		return "fantasia"
	case "codigo_cliente":
		return "cliente.id"
	case "telefone":
		return "fone"
	case "email":
		return "email"
	default:
		return "cliente.cnpj_cpf"
	}
}

// IXCOperForBusca operador IXC por tipo de busca.
func IXCOperForBusca(busca string) string {
	switch strings.ToLower(strings.TrimSpace(busca)) {
	case "cpf_cnpj", "codigo_cliente", "email":
		return "="
	case "nome_razaosocial", "nome_fantasia", "telefone":
		return "L"
	default:
		return "="
	}
}

// IXCTermoForQuery valor do campo query conforme o tipo de busca.
func IXCTermoForQuery(busca, termo string) string {
	termo = strings.TrimSpace(termo)
	switch strings.ToLower(strings.TrimSpace(busca)) {
	case "cpf_cnpj":
		return digitsOnly(termo)
	default:
		return termo
	}
}

func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ApplyIXCBodySearch monta/atualiza corpo JSON POST IXC para listar com filtro.
func ApplyIXCBodySearch(bodyTemplate, busca, termo string, detailed bool, cfg ClientSearchConfig) string {
	obj := map[string]any{}
	if strings.TrimSpace(bodyTemplate) != "" {
		_ = json.Unmarshal([]byte(bodyTemplate), &obj)
	}
	if obj == nil {
		obj = map[string]any{}
	}
	obj["qtype"] = cfg.ResolveQtype(busca)
	obj["query"] = cfg.ResolveTermo(busca, termo)
	obj["oper"] = cfg.ResolveOper(busca)
	if _, ok := obj["sortname"]; !ok {
		obj["sortname"] = "cliente.id"
	}
	if _, ok := obj["sortorder"]; !ok {
		obj["sortorder"] = "desc"
	}
	rp := "20"
	if detailed {
		rp = "100"
	}
	if v, ok := obj["rp"]; !ok || strings.TrimSpace(fmtAny(v)) == "" {
		obj["rp"] = rp
	}
	raw, err := json.Marshal(obj)
	if err != nil {
		return bodyTemplate
	}
	return string(raw)
}

func fmtAny(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	default:
		b, _ := json.Marshal(x)
		return strings.TrimSpace(string(b))
	}
}

// ixcBodyFromQueryParams move parâmetros IXC da query string para o corpo (configuração comum na UI).
func ixcBodyFromQueryParams(params []integrationhttp.ParamKV) string {
	if len(params) == 0 {
		return ""
	}
	obj := map[string]any{}
	for _, p := range params {
		key := strings.TrimSpace(p.Key)
		if key == "" {
			continue
		}
		switch key {
		case "busca", "termo_busca", "limit", "cancelado", "inativo", "ultima_conexao",
			"incluir_alarmes", "incluir_anexos", "incluir_contrato", "incluir_desbloqueios",
			"incluir_mvno", "incluir_stfc", "order_by", "order_type":
			continue
		}
		val := strings.TrimSpace(p.Value)
		if val == "" {
			continue
		}
		obj[key] = val
	}
	if len(obj) == 0 {
		return ""
	}
	b, err := json.Marshal(obj)
	if err != nil {
		return ""
	}
	return string(b)
}

// PrepareIXCClientListRequest força o fluxo oficial IXC: POST + ixcsoft:listar + JSON.
// O webservice não expõe GET /cliente?busca=… (retorna «recurso não está disponível»).
func PrepareIXCClientListRequest(rc integrationhttp.RequestConfig, busca, termo string, detailed bool, cfg ClientSearchConfig) integrationhttp.RequestConfig {
	rc = ensureIXCListarHeader(rc, cfg.IxcListHeaderValue())
	method := strings.ToUpper(strings.TrimSpace(rc.Method))
	if method == "" || method == "GET" {
		rc.Method = "POST"
	}
	baseBody := strings.TrimSpace(rc.BodyTemplate)
	if baseBody == "" {
		baseBody = ixcBodyFromQueryParams(rc.QueryParams)
	}
	// IXC documentado: parâmetros no corpo JSON (query string quebra ou é ignorada no POST).
	rc.QueryParams = nil
	rc.BodyTemplate = ApplyIXCBodySearch(baseBody, busca, termo, detailed, cfg)
	if strings.TrimSpace(rc.BodyType) == "" {
		rc.BodyType = "json"
	}
	if p := strings.TrimSpace(rc.Path); p == "" {
		rc.Path = "/cliente"
	}
	return rc
}

func ensureIXCListarHeader(rc integrationhttp.RequestConfig, listAction string) integrationhttp.RequestConfig {
	if rc.Headers == nil {
		rc.Headers = map[string]string{}
	}
	for k := range rc.Headers {
		if strings.EqualFold(k, "ixcsoft") {
			rc.Headers[k] = listAction
			return rc
		}
	}
	rc.Headers["ixcsoft"] = listAction
	return rc
}
