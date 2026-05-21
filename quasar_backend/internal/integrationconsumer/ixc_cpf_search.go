package integrationconsumer

import (
	"encoding/json"
	"strings"

	"github.com/netquasar/netquasar/quasar_backend/internal/integrationhttp"
)

// IXCSearchAttempt uma combinação qtype/query/oper para o POST listar IXC.
type IXCSearchAttempt struct {
	Qtype string
	Query string
	Oper  string
}

// FilterClientsByDocument mantém só cartões cujo CPF/CNPJ coincide com o termo digitado.
func FilterClientsByDocument(clients []ClientCard, termo string) []ClientCard {
	want := digitsOnly(termo)
	if want == "" {
		return clients
	}
	var out []ClientCard
	for _, c := range clients {
		got := digitsOnly(c.Document)
		if got == want {
			out = append(out, c)
			continue
		}
		if len(want) >= 8 && (strings.Contains(got, want) || strings.Contains(want, got)) {
			out = append(out, c)
		}
	}
	return out
}

// ApplyIXCBodySearchAttempt aplica uma tentativa de busca ao template JSON.
func ApplyIXCBodySearchAttempt(bodyTemplate string, att IXCSearchAttempt, detailed bool) string {
	obj := map[string]any{}
	if strings.TrimSpace(bodyTemplate) != "" {
		_ = json.Unmarshal([]byte(bodyTemplate), &obj)
	}
	if obj == nil {
		obj = map[string]any{}
	}
	obj["qtype"] = att.Qtype
	obj["query"] = att.Query
	obj["oper"] = att.Oper
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

func prepareIXCBaseRequest(rc integrationhttp.RequestConfig, cfg ClientSearchConfig) (integrationhttp.RequestConfig, string) {
	rc = ensureIXCListarHeader(rc, cfg.IxcListHeaderValue())
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
		rc.Path = "/cliente"
	}
	return rc, baseBody
}

// RunIXCClientSearchWithAttempts executa variantes de qtype/oper até achar cliente(s).
func RunIXCClientSearchWithAttempts(
	rc integrationhttp.RequestConfig,
	busca, termo string,
	detailed bool,
	cfg ClientSearchConfig,
	execute func(rc integrationhttp.RequestConfig) integrationhttp.RunResult,
) (integrationhttp.RunResult, SearchResult) {
	rc, baseBody := prepareIXCBaseRequest(rc, cfg)
	attempts := BuildIXCSearchAttempts(cfg, busca, termo)
	var lastRes integrationhttp.RunResult
	lastParsed := SearchResult{Clients: []ClientCard{}, Message: "Nenhum cliente encontrado para este CPF/CNPJ."}
	for _, att := range attempts {
		rc.BodyTemplate = ApplyIXCBodySearchAttempt(baseBody, att, detailed)
		lastRes = execute(rc)
		raw := ResponseBodyForParse([]byte(lastRes.ResponsePreview))
		parsed := ParseIXCClientSearch(raw)
		if busca == "cpf_cnpj" {
			parsed.Clients = FilterClientsByDocument(parsed.Clients, termo)
		}
		if parsed.OK && len(parsed.Clients) > 0 {
			return lastRes, parsed
		}
		lastParsed = parsed
		if lastParsed.Message == "" {
			lastParsed.Message = "Nenhum cliente encontrado para este CPF/CNPJ."
		}
	}
	if !lastParsed.OK && lastParsed.Message == "" {
		lastParsed.Message = "Nenhum cliente encontrado para este CPF/CNPJ."
	}
	return lastRes, lastParsed
}
