package integrationconsumer

import (
	"encoding/json"
	"strings"

	"github.com/netquasar/netquasar/quasar_backend/internal/integrationhttp"
)

// pickIXCStatusInternet lê apenas status_internet (nunca o campo «status»).
func pickIXCStatusInternet(sm map[string]any) string {
	if sm == nil {
		return ""
	}
	if v := pickStr(sm, "status_internet"); v != "" {
		return v
	}
	for _, nestKey := range []string{"cliente_contrato", "contrato", "dados_contrato"} {
		if sub := pickNestedMap(sm, nestKey); sub != nil {
			if v := pickStr(sub, "status_internet"); v != "" {
				return v
			}
		}
	}
	return ""
}

// LooksLikeIXCClienteContratoRequest indica webservice IXC cliente_contrato (status_internet).
func LooksLikeIXCClienteContratoRequest(rc integrationhttp.RequestConfig) bool {
	path := strings.ToLower(strings.TrimSpace(rc.Path))
	if strings.Contains(path, "cliente_contrato") {
		return true
	}
	body := strings.ToLower(rc.BodyTemplate)
	return strings.Contains(body, "cliente_contrato")
}

func prepareIXCClienteContratoList(rc integrationhttp.RequestConfig, searchCfg ClientSearchConfig) integrationhttp.RequestConfig {
	action := strings.TrimSpace(searchCfg.IxcListAction)
	if action == "" {
		action = "listar"
	}
	rc = ensureIXCListarHeader(rc, action)
	path := strings.ToLower(strings.TrimSpace(rc.Path))
	if !strings.Contains(path, "cliente_contrato") {
		rc.Path = "/cliente_contrato"
	}
	if strings.ToUpper(strings.TrimSpace(rc.Method)) == "" {
		rc.Method = "POST"
	}
	if strings.TrimSpace(rc.BodyTemplate) == "" {
		rc.BodyTemplate = ixcBodyFromQueryParams(rc.QueryParams)
	}
	rc.QueryParams = nil
	if strings.TrimSpace(rc.BodyType) == "" {
		rc.BodyType = "json"
	}
	return rc
}

// ApplyIXCClienteContratoBodySearchAttempt corpo listar IXC para /cliente_contrato.
func ApplyIXCClienteContratoBodySearchAttempt(bodyTemplate string, att IXCSearchAttempt) string {
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
		obj["sortname"] = "cliente_contrato.id"
	}
	if _, ok := obj["sortorder"]; !ok {
		obj["sortorder"] = "desc"
	}
	if v, ok := obj["rp"]; !ok || strings.TrimSpace(fmtAny(v)) == "" {
		obj["rp"] = "20"
	}
	if _, ok := obj["page"]; !ok {
		obj["page"] = "1"
	}
	raw, err := json.Marshal(obj)
	if err != nil {
		return bodyTemplate
	}
	return string(raw)
}

func extractClienteContratoArray(doc map[string]any) []any {
	for _, key := range []string{"registros", "cliente_contrato", "cliente_contratos", "contratos", "contrato", "results", "data"} {
		if arr, ok := doc[key].([]any); ok && len(arr) > 0 {
			return arr
		}
	}
	if data, ok := doc["data"].(map[string]any); ok {
		for _, key := range []string{"registros", "cliente_contrato", "cliente_contratos", "contratos"} {
			if arr, ok := data[key].([]any); ok && len(arr) > 0 {
				return arr
			}
		}
	}
	return nil
}

func indexClienteContratoRecord(index map[string]string, sm map[string]any) {
	st := pickIXCStatusInternet(sm)
	if st == "" {
		return
	}
	if id := pickStr(sm, "id", "id_cliente_contrato", "id_contrato"); id != "" {
		index[id] = st
	}
}

func runIXCClienteContratoQuery(
	rc integrationhttp.RequestConfig,
	baseBody string,
	att IXCSearchAttempt,
	searchCfg ClientSearchConfig,
	execute func(rc integrationhttp.RequestConfig) integrationhttp.RunResult,
) (map[string]any, bool) {
	rc = prepareIXCClienteContratoList(rc, searchCfg)
	rc.BodyTemplate = ApplyIXCClienteContratoBodySearchAttempt(baseBody, att)
	res := execute(rc)
	raw := ResponseBodyForParse([]byte(res.ResponsePreview))
	doc, err := decodeJSONDocument(raw)
	if err != nil {
		return nil, false
	}
	if msg := ixcErrorMessage(doc); msg != "" {
		return nil, false
	}
	return doc, true
}

// fetchIXCContractByID busca um contrato pelo ID (qtype cliente_contrato.id — padrão IXC).
func fetchIXCContractByID(
	contratoRC integrationhttp.RequestConfig,
	contractID string,
	searchCfg ClientSearchConfig,
	execute func(rc integrationhttp.RequestConfig) integrationhttp.RunResult,
) string {
	contractID = strings.TrimSpace(contractID)
	if contractID == "" {
		return ""
	}
	baseBody := strings.TrimSpace(contratoRC.BodyTemplate)
	doc, ok := runIXCClienteContratoQuery(contratoRC, baseBody, IXCSearchAttempt{
		Qtype: "cliente_contrato.id",
		Query: contractID,
		Oper:  "=",
	}, searchCfg, execute)
	if !ok || doc == nil {
		return ""
	}
	for _, it := range extractClienteContratoArray(doc) {
		sm, ok := it.(map[string]any)
		if !ok {
			continue
		}
		if st := pickIXCStatusInternet(sm); st != "" {
			return st
		}
	}
	return ""
}

func contractIDsFromServices(services []ServiceSummary) []string {
	seen := map[string]struct{}{}
	var ids []string
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		if _, dup := seen[id]; dup {
			return
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	for _, s := range services {
		add(s.ContratoID)
	}
	return ids
}

// FetchIXCContractStatusIndexFromServices busca status por cliente_contrato.id de cada login.
func FetchIXCContractStatusIndexFromServices(
	services []ServiceSummary,
	contratoRC integrationhttp.RequestConfig,
	searchCfg ClientSearchConfig,
	execute func(rc integrationhttp.RequestConfig) integrationhttp.RunResult,
	maxFetches int,
) map[string]string {
	if maxFetches <= 0 {
		maxFetches = 12
	}
	index := map[string]string{}
	for _, contractID := range contractIDsFromServices(services) {
		if len(index) >= maxFetches {
			break
		}
		st := fetchIXCContractByID(contratoRC, contractID, searchCfg, execute)
		if st != "" {
			index[contractID] = st
		}
	}
	return index
}

// FetchIXCContractStatusIndex lista contratos do cliente (fallback por id_cliente).
func FetchIXCContractStatusIndex(
	contratoRC integrationhttp.RequestConfig,
	clientID string,
	searchCfg ClientSearchConfig,
	execute func(rc integrationhttp.RequestConfig) integrationhttp.RunResult,
) map[string]string {
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		return nil
	}
	baseBody := strings.TrimSpace(contratoRC.BodyTemplate)
	index := map[string]string{}
	attempts := []IXCSearchAttempt{
		{Qtype: "cliente_contrato.id_cliente", Query: clientID, Oper: "="},
	}
	for _, att := range attempts {
		doc, ok := runIXCClienteContratoQuery(contratoRC, baseBody, att, searchCfg, execute)
		if !ok {
			continue
		}
		for _, it := range extractClienteContratoArray(doc) {
			sm, ok := it.(map[string]any)
			if !ok {
				continue
			}
			indexClienteContratoRecord(index, sm)
		}
		if len(index) > 0 {
			return index
		}
	}
	return index
}

func contractStatusForService(s ServiceSummary, index map[string]string) string {
	if s.StatusInternet != "" {
		return s.StatusInternet
	}
	if len(index) == 0 {
		return ""
	}
	if k := strings.TrimSpace(s.ContratoID); k != "" {
		if st, ok := index[k]; ok {
			return st
		}
	}
	return ""
}

// ApplyContractStatusIndex preenche status_internet nos logins a partir do índice de contratos.
func ApplyContractStatusIndex(services []ServiceSummary, index map[string]string) []ServiceSummary {
	if len(services) == 0 || len(index) == 0 {
		return services
	}
	out := make([]ServiceSummary, len(services))
	for i, s := range services {
		s2 := s
		if st := contractStatusForService(s2, index); st != "" {
			s2.StatusInternet = st
			s2.StatusLabel = FormatIXCStatusInternet(st)
		}
		out[i] = s2
	}
	return out
}

// EnrichIXCClientsContractStatus busca status_internet em /cliente_contrato para cada cartão.
func EnrichIXCClientsContractStatus(
	clients []ClientCard,
	contratoRC integrationhttp.RequestConfig,
	searchCfg ClientSearchConfig,
	execute func(rc integrationhttp.RequestConfig) integrationhttp.RunResult,
	maxClients int,
) []ClientCard {
	if maxClients <= 0 || len(clients) == 0 {
		return clients
	}
	if maxClients > len(clients) {
		maxClients = len(clients)
	}
	out := make([]ClientCard, len(clients))
	copy(out, clients)
	for i := 0; i < maxClients; i++ {
		services := out[i].Services
		idx := FetchIXCContractStatusIndexFromServices(services, contratoRC, searchCfg, execute, 12)
		if len(idx) == 0 {
			clientID := strings.TrimSpace(out[i].ID)
			if clientID != "" {
				idx = FetchIXCContractStatusIndex(contratoRC, clientID, searchCfg, execute)
			}
		}
		if len(idx) > 0 {
			out[i].Services = ApplyContractStatusIndex(services, idx)
		}
	}
	return out
}

// mergeServiceSummary funde dois resumos (prioriza status_internet e dados do contrato).
func mergeServiceSummary(dst, src ServiceSummary) ServiceSummary {
	out := dst
	if out.StatusInternet == "" && src.StatusInternet != "" {
		out.StatusInternet = src.StatusInternet
		out.StatusLabel = src.StatusLabel
	}
	if out.ContratoID == "" && src.ContratoID != "" {
		out.ContratoID = src.ContratoID
	}
	if out.Contrato == "" && src.Contrato != "" {
		out.Contrato = src.Contrato
	}
	if out.PlanoVenda == "" && src.PlanoVenda != "" {
		out.PlanoVenda = src.PlanoVenda
	}
	if out.MAC == "" && src.MAC != "" {
		out.MAC = src.MAC
	}
	if out.Online == "" && src.Online != "" {
		out.Online = src.Online
		out.OnlineLabel = src.OnlineLabel
	}
	if out.Login == "" && src.Login != "" {
		out.Login = src.Login
	}
	if out.IPv4 == "" && src.IPv4 != "" {
		out.IPv4 = src.IPv4
	}
	if out.Name == "" && src.Name != "" {
		out.Name = src.Name
	}
	if out.ID == "" && src.ID != "" && out.Login == "" {
		out.ID = src.ID
	}
	return out
}
