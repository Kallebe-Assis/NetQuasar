package integrationhubsoft

import (
	"context"
	"sync"
	"time"
)

// --- "Bloqueado desde" no cartão do cliente --------------------------------------------------------
//
// /cliente (consulta) não devolve data_ultima_suspensao, mas /cliente/todos devolve. Para mostrar
// "bloqueado desde" no cartão sem uma consulta por cliente, mantém-se um mapa
// id_cliente_servico → data da última suspensão dos serviços suspensos por débito, lido de uma
// varredura de /cliente/todos?servico_status=suspenso_debito e guardado por alguns minutos.

const suspendedSinceTTL = 10 * time.Minute

type suspendedSinceCache struct {
	mu     sync.Mutex
	loaded time.Time
	base   string
	m      map[string]string // id_cliente_servico → DD/MM/YYYY
}

var suspendedSince suspendedSinceCache

func (c *suspendedSinceCache) get(ctx context.Context, cfg Config, token string) map[string]string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m != nil && c.base == cfg.BaseURL && time.Since(c.loaded) < suspendedSinceTTL {
		return c.m
	}
	items, _, err := fetchAllPages(ctx, cfg, token, "/api/v1/integracao/cliente/todos", map[string]string{
		"servico_status": "suspenso_debito", "cancelado": "nao",
	}, 200, "clientes")
	if err != nil {
		return c.m // mantém o que já havia (pode ser nil): o cartão só fica sem a data
	}
	m := map[string]string{}
	for _, it := range items {
		arr, _ := it["servicos"].([]any)
		for _, sit := range arr {
			sm, ok := sit.(map[string]any)
			if !ok {
				continue
			}
			id := pickStr(sm, "id_cliente_servico")
			if id == "" {
				continue
			}
			if t := parseBRDate(pickStr(sm, "data_ultima_suspensao")); !t.IsZero() {
				m[id] = t.Format("02/01/2006")
			}
		}
	}
	c.m, c.loaded, c.base = m, time.Now(), cfg.BaseURL
	return m
}

// EnrichSuspendedSince preenche ServiceSummary.SuspendedAt nos serviços "Suspenso por Débito".
func EnrichSuspendedSince(ctx context.Context, cfg Config, token string, res *ClientSearchResult) {
	need := false
	for i := range res.Clients {
		for _, s := range res.Clients[i].Services {
			if s.StatusPrefix == "suspenso_debito" {
				need = true
			}
		}
	}
	if !need {
		return
	}
	m := suspendedSince.get(ctx, cfg, token)
	if len(m) == 0 {
		return
	}
	for i := range res.Clients {
		for j := range res.Clients[i].Services {
			s := &res.Clients[i].Services[j]
			if s.StatusPrefix == "suspenso_debito" {
				s.SuspendedAt = m[s.ID]
			}
		}
	}
}
