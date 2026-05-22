package integrationconsumer

import (
	"strings"

	"github.com/netquasar/netquasar/quasar_backend/internal/integrationhttp"
)

// DefaultClientSearchBusca tipo de consulta padrão na UI (razão social).
const DefaultClientSearchBusca = "nome_razaosocial"

func ixcClientNameFromRow(sm map[string]any) string {
	if sub, ok := sm["cliente"].(map[string]any); ok {
		return firstNonEmpty(
			pickStr(sub, "razao", "nome_razaosocial", "nome", "razao_social"),
			pickStr(sub, "fantasia", "nome_fantasia"),
		)
	}
	return firstNonEmpty(
		pickStr(sm, "razao", "razao_social", "razao_social_cliente"),
		pickStr(sm, "cliente_razao", "nome_cliente", "nome_razaosocial", "nome"),
		pickStr(sm, "fantasia", "nome_fantasia"),
	)
}

func cardNeedsClientNameEnrich(c ClientCard) bool {
	name := strings.TrimSpace(c.Name)
	id := strings.TrimSpace(c.ID)
	if name == "" {
		return id != ""
	}
	if id != "" && name == id {
		return true
	}
	if strings.HasPrefix(name, "Cliente ") {
		return true
	}
	return false
}

// EnrichIXCClientCardNames busca razão social em /cliente para cartões vindos de radusuarios.
func EnrichIXCClientCardNames(
	cards []ClientCard,
	cfg ClientSearchConfig,
	clienteRC integrationhttp.RequestConfig,
	execute func(rc integrationhttp.RequestConfig) integrationhttp.RunResult,
	max int,
) []ClientCard {
	if len(cards) == 0 || max <= 0 {
		return cards
	}
	out := make([]ClientCard, len(cards))
	copy(out, cards)
	baseRC := PrepareIXCClientListRequest(clienteRC, "codigo_cliente", "", false, cfg)
	baseBody := baseRC.BodyTemplate
	for i := range out {
		if i >= max {
			break
		}
		if !cardNeedsClientNameEnrich(out[i]) {
			continue
		}
		id := strings.TrimSpace(out[i].ID)
		if id == "" {
			continue
		}
		rc := baseRC
		rc.BodyTemplate = ApplyIXCBodySearchAttempt(baseBody, IXCSearchAttempt{
			Qtype: cfg.ResolveQtype("codigo_cliente"),
			Query: id,
			Oper:  cfg.ResolveOper("codigo_cliente"),
		}, false)
		res := execute(rc)
		raw := ResponseBodyForParse([]byte(res.ResponsePreview))
		parsed := ParseIXCClientSearch(raw)
		if len(parsed.Clients) == 0 {
			continue
		}
		full := parsed.Clients[0]
		if n := strings.TrimSpace(full.Name); n != "" {
			out[i].Name = n
		}
		if t := strings.TrimSpace(full.TradeName); t != "" {
			out[i].TradeName = t
		}
		if d := strings.TrimSpace(full.Document); d != "" {
			out[i].Document = d
		}
		if e := strings.TrimSpace(full.Email); e != "" {
			out[i].Email = e
		}
		if p := strings.TrimSpace(full.Phone); p != "" {
			out[i].Phone = p
		}
		if a := strings.TrimSpace(full.Address); a != "" {
			out[i].Address = a
		}
		if len(full.Services) > 0 {
			out[i].Services = mergeServiceLists(out[i].Services, full.Services)
		}
	}
	return out
}
