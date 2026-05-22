package integrationconsumer

import (
	"encoding/json"
	"strings"

	"github.com/netquasar/netquasar/quasar_backend/internal/integrationhttp"
)

func mergeLoginConfig(loginCfg ClientLoginConfig, searchCfg ClientSearchConfig) ClientLoginConfig {
	if len(loginCfg.FieldMappings) > 0 {
		return loginCfg
	}
	if len(searchCfg.FieldMappings) == 0 {
		return loginCfg
	}
	out := loginCfg
	out.FieldMappings = searchCfg.FieldMappings
	return out
}

func (cfg ClientLoginConfig) ResolveLoginQtype(busca string) string {
	if m, ok := cfg.FieldMappings[strings.ToLower(strings.TrimSpace(busca))]; ok && m.Qtype != "" {
		return normalizeIXCLoginQtype(m.Qtype, busca)
	}
	switch strings.ToLower(strings.TrimSpace(busca)) {
	case "login", "login_radius":
		return "radusuarios.login"
	case "codigo_cliente":
		return "radusuarios.id_cliente"
	default:
		return "radusuarios.id_cliente"
	}
}

func normalizeIXCLoginQtype(qtype, busca string) string {
	qtype = strings.TrimSpace(qtype)
	low := strings.ToLower(qtype)
	if busca == "codigo_cliente" && (low == "radusuarios.id" || low == "id") {
		return "radusuarios.id_cliente"
	}
	if low == "id_cliente" || low == "cliente.id_cliente" {
		return "radusuarios.id_cliente"
	}
	if strings.Contains(low, "id_cliente") && !strings.Contains(low, "radusuarios") {
		return "radusuarios.id_cliente"
	}
	return qtype
}

func (cfg ClientLoginConfig) ixcListAction(searchCfg ClientSearchConfig) string {
	if v := strings.TrimSpace(cfg.IxcListAction); v != "" {
		return v
	}
	return searchCfg.IxcListHeaderValue()
}

func LooksLikeIXCLoginRequest(rc integrationhttp.RequestConfig) bool {
	path := strings.ToLower(strings.TrimSpace(rc.Path))
	if strings.Contains(path, "radusuarios") || strings.Contains(path, "radusuario") {
		return true
	}
	body := strings.ToLower(rc.BodyTemplate)
	return strings.Contains(body, "radusuarios")
}

func ApplyIXCLoginBodySearchAttempt(bodyTemplate string, att IXCSearchAttempt) string {
	obj := map[string]any{}
	if strings.TrimSpace(bodyTemplate) != "" {
		_ = json.Unmarshal([]byte(bodyTemplate), &obj)
	}
	if obj == nil {
		obj = map[string]any{}
	}
	obj["qtype"] = normalizeIXCLoginQtype(att.Qtype, "codigo_cliente")
	obj["query"] = att.Query
	obj["oper"] = att.Oper
	if _, ok := obj["sortname"]; !ok {
		obj["sortname"] = "radusuarios.id"
	}
	if _, ok := obj["sortorder"]; !ok {
		obj["sortorder"] = "asc"
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

func BuildIXCLoginAttempts(cfg ClientLoginConfig, busca, termo, baseBody string) []IXCSearchAttempt {
	busca = strings.ToLower(strings.TrimSpace(busca))
	t := strings.TrimSpace(termo)
	var attempts []IXCSearchAttempt
	add := func(qtype, query, oper string) {
		if strings.TrimSpace(query) == "" {
			return
		}
		attempts = append(attempts, IXCSearchAttempt{
			Qtype: normalizeIXCLoginQtype(qtype, busca),
			Query: query,
			Oper:  oper,
		})
	}
	if tq := ixcBodyTemplateQtype(baseBody); tq != "" && busca == "codigo_cliente" {
		add(tq, t, "=")
	}
	oper := "="
	if IsLoginBusca(busca) {
		oper = "L"
	}
	add(cfg.ResolveLoginQtype(busca), t, oper)
	if busca == "codigo_cliente" {
		add("radusuarios.id_cliente", t, "=")
	}
	if IsLoginBusca(busca) {
		add("login", t, "L")
	}
	return dedupeIXCAttempts(attempts)
}

func prepareIXCBaseLogin(rc integrationhttp.RequestConfig, loginCfg ClientLoginConfig, searchCfg ClientSearchConfig) (integrationhttp.RequestConfig, string) {
	rc = ensureIXCListarHeader(rc, loginCfg.ixcListAction(searchCfg))
	if strings.ToUpper(strings.TrimSpace(rc.Method)) == "" || strings.ToUpper(strings.TrimSpace(rc.Method)) == "GET" {
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
		rc.Path = "/radusuarios"
	}
	return rc, baseBody
}

// LoginListResult lista de logins/planos do cliente.
type LoginListResult struct {
	OK      bool             `json:"ok"`
	Message string           `json:"message,omitempty"`
	Items   []ServiceSummary `json:"items"`
}

// ParseIXCClientLogins interpreta listagem radusuarios IXC.
func ParseIXCClientLogins(raw []byte) LoginListResult {
	out := LoginListResult{Items: []ServiceSummary{}}
	if len(raw) == 0 {
		out.Message = "Resposta vazia"
		return out
	}
	doc, err := decodeJSONDocument(raw)
	if err != nil {
		out.Message = NonJSONResponseHint(raw, 0)
		return out
	}
	if msg := ixcErrorMessage(doc); msg != "" {
		out.OK = false
		out.Message = msg
		return out
	}
	items := extractLoginArray(doc)
	seen := map[string]struct{}{}
	for _, it := range items {
		if sm, ok := it.(map[string]any); ok {
			svc := mapIXCServiceItem(sm)
			if svc.Login == "" && svc.Name == "" && svc.ID == "" {
				continue
			}
			k := servicesDedupeKey(svc)
			if _, dup := seen[k]; dup {
				continue
			}
			seen[k] = struct{}{}
			out.Items = append(out.Items, svc)
		}
	}
	out.OK = true
	if len(out.Items) == 0 {
		out.Message = "Nenhum login encontrado."
	}
	return out
}

func extractLoginArray(doc map[string]any) []any {
	for _, key := range []string{"registros", "radusuarios", "logins", "results", "data", "items"} {
		if arr, ok := doc[key].([]any); ok && len(arr) > 0 {
			return arr
		}
	}
	if data, ok := doc["data"].(map[string]any); ok {
		for _, key := range []string{"registros", "radusuarios", "logins"} {
			if arr, ok := data[key].([]any); ok && len(arr) > 0 {
				return arr
			}
		}
	}
	return nil
}

// RunIXCLoginWithAttempts executa listagem de logins por cliente.
func RunIXCLoginWithAttempts(
	rc integrationhttp.RequestConfig,
	busca, termo string,
	loginCfg ClientLoginConfig,
	searchCfg ClientSearchConfig,
	execute func(rc integrationhttp.RequestConfig) integrationhttp.RunResult,
) (integrationhttp.RunResult, LoginListResult) {
	loginCfg = mergeLoginConfig(loginCfg, searchCfg)
	rc, baseBody := prepareIXCBaseLogin(rc, loginCfg, searchCfg)
	attempts := BuildIXCLoginAttempts(loginCfg, busca, termo, baseBody)
	var lastRes integrationhttp.RunResult
	lastParsed := LoginListResult{Items: []ServiceSummary{}, Message: "Nenhum login encontrado."}
	for _, att := range attempts {
		rc.BodyTemplate = ApplyIXCLoginBodySearchAttempt(baseBody, att)
		lastRes = execute(rc)
		raw := ResponseBodyForParse([]byte(lastRes.ResponsePreview))
		lastParsed = ParseIXCClientLogins(raw)
		if lastParsed.OK && len(lastParsed.Items) > 0 {
			if lastParsed.Message == "Nenhum login encontrado." {
				lastParsed.Message = ""
			}
			return lastRes, lastParsed
		}
	}
	if lastParsed.Message == "" {
		lastParsed.Message = "Nenhum login encontrado para este cliente."
	}
	return lastRes, lastParsed
}

// EnrichIXCClientServices busca logins na API e funde em Services do cartão (máx. maxClients).
func EnrichIXCClientServices(
	clients []ClientCard,
	loginCfg ClientLoginConfig,
	searchCfg ClientSearchConfig,
	rc integrationhttp.RequestConfig,
	execute func(rc integrationhttp.RequestConfig) integrationhttp.RunResult,
	maxClients int,
) []ClientCard {
	if maxClients <= 0 || len(clients) == 0 {
		return clients
	}
	if maxClients > len(clients) {
		maxClients = len(clients)
	}
	rc, _ = prepareIXCBaseLogin(rc, loginCfg, searchCfg)
	out := make([]ClientCard, len(clients))
	copy(out, clients)
	for i := 0; i < maxClients; i++ {
		id := strings.TrimSpace(out[i].ID)
		if id == "" {
			continue
		}
		_, parsed := RunIXCLoginWithAttempts(rc, "codigo_cliente", id, loginCfg, searchCfg, execute)
		if len(parsed.Items) > 0 {
			out[i].Services = mergeServiceLists(out[i].Services, parsed.Items)
		}
	}
	return out
}
