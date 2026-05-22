package integrationconsumer

import (
	"fmt"
	"strings"

	"github.com/netquasar/netquasar/quasar_backend/internal/integrationhttp"
)

// BuildClientCardsFromLoginServices agrupa logins IXC em cartões por id_cliente.
func BuildClientCardsFromLoginServices(items []ServiceSummary) []ClientCard {
	type bucket struct {
		card     ClientCard
		seenSvc  map[string]struct{}
	}
	byClient := map[string]*bucket{}
	order := []string{}

	addCard := func(clientID string, svc ServiceSummary) {
		clientID = strings.TrimSpace(clientID)
		if clientID == "" {
			clientID = "login:" + strings.TrimSpace(svc.Login)
		}
		b, ok := byClient[clientID]
		if !ok {
			name := strings.TrimSpace(svc.ClientName)
			if name == "" {
				name = "Cliente " + clientID
			}
			b = &bucket{
				card: ClientCard{
					ID:       clientID,
					Code:     clientID,
					Name:     name,
					Services: []ServiceSummary{},
				},
				seenSvc: map[string]struct{}{},
			}
			byClient[clientID] = b
			order = append(order, clientID)
		}
		k := servicesDedupeKey(svc)
		if _, dup := b.seenSvc[k]; dup {
			return
		}
		b.seenSvc[k] = struct{}{}
		b.card.Services = append(b.card.Services, svc)
		if b.card.Name == "" || strings.HasPrefix(b.card.Name, "Cliente ") {
			if n := strings.TrimSpace(svc.ClientName); n != "" {
				b.card.Name = n
			}
		}
	}

	for _, svc := range items {
		addCard(svc.ClientID, svc)
	}

	out := make([]ClientCard, 0, len(order))
	for _, id := range order {
		out = append(out, byClient[id].card)
	}
	return out
}

// RunIXCClientSearchByLogin pesquisa clientes via radusuarios (login) e enriquece com /cliente.
func RunIXCClientSearchByLogin(
	loginRC, clienteRC, contractRC integrationhttp.RequestConfig,
	busca, termo string,
	loginCfg ClientLoginConfig,
	searchCfg ClientSearchConfig,
	execute func(rc integrationhttp.RequestConfig) integrationhttp.RunResult,
) (integrationhttp.RunResult, SearchResult) {
	res, loginRes := RunIXCLoginWithAttempts(loginRC, busca, termo, loginCfg, searchCfg, execute)
	out := SearchResult{Clients: []ClientCard{}}
	if !loginRes.OK {
		out.Message = loginRes.Message
		return res, out
	}
	if len(loginRes.Items) == 0 {
		out.Message = fmt.Sprintf("Nenhum cliente encontrado para o login «%s».", strings.TrimSpace(termo))
		return res, out
	}
	cards := BuildClientCardsFromLoginServices(loginRes.Items)
	max := len(cards)
	if max > 24 {
		max = 24
	}
	cards = EnrichIXCClientCardNames(cards, searchCfg, clienteRC, execute, max)
	out.Clients = EnrichIXCClientsContractStatus(cards, contractRC, searchCfg, execute, max)
	out.OK = true
	out.Message = ""
	return res, out
}
