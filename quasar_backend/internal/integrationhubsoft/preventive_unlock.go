package integrationhubsoft

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/netquasar/netquasar/quasar_backend/internal/integrationhttp"
)

// --- Relatório: "Desbloqueio preventivo" -----------------------------------------------------------
//
// O log de status do painel da HubSoft não existe na API, mas o "desbloqueio em confiança" de cada
// serviço existe: /cliente?incluir_desbloqueios=sim devolve servicos[].desbloqueios[] com a
// observação (ex.: "DESBLOQUEIO PREVENTIVO - Acordo de pagamento."), a data de cadastro e o término.
// Só há esse dado por cliente (não em /cliente/todos), então o relatório é em duas fases:
//   1. BuildPreventiveBase — lista leve de clientes/serviços (1 varredura de /cliente/todos);
//   2. BuildPreventiveChunk — para um lote de clientes, conta os desbloqueios de cada serviço.
// O front encadeia os lotes (com progresso e botão de parar).

type PreventiveServiceRef struct {
	ServiceID string `json:"id"`
	Login     string `json:"login,omitempty"`
	Plan      string `json:"plan,omitempty"`
	Status    string `json:"status,omitempty"`
	City      string `json:"city,omitempty"`
}

type PreventiveClientRef struct {
	ClientID   string                 `json:"id"`
	ClientCode string                 `json:"code,omitempty"`
	Name       string                 `json:"name,omitempty"`
	Services   []PreventiveServiceRef `json:"services"`
}

type PreventiveBase struct {
	OK        bool                  `json:"ok"`
	Message   string                `json:"message,omitempty"`
	Clients   []PreventiveClientRef `json:"clients"`
	Services  int                   `json:"services"`
	Truncated bool                  `json:"truncated,omitempty"`
}

func BuildPreventiveBase(ctx context.Context, cfg Config, token string) PreventiveBase {
	items, total, err := fetchAllPages(ctx, cfg, token, "/api/v1/integracao/cliente/todos", map[string]string{"cancelado": "nao", "relacoes": "endereco_instalacao"}, 400, "clientes")
	if err != nil {
		return PreventiveBase{OK: false, Message: "Falha ao consultar a HubSoft: " + err.Error()}
	}
	out := PreventiveBase{OK: true, Clients: []PreventiveClientRef{}, Truncated: total > len(items)}
	for _, m := range items {
		c := PreventiveClientRef{ClientID: pickStr(m, "id_cliente"), ClientCode: pickStr(m, "codigo_cliente"), Name: pickStr(m, "nome_razaosocial")}
		if c.ClientID == "" {
			continue
		}
		arr, _ := m["servicos"].([]any)
		for _, it := range arr {
			sm, ok := it.(map[string]any)
			if !ok {
				continue
			}
			ref := PreventiveServiceRef{ServiceID: pickStr(sm, "id_cliente_servico"), Login: pickStr(sm, "login"), Plan: pickStr(sm, "nome"), Status: pickStr(sm, "status")}
			if addr, ok := sm["endereco_instalacao"].(map[string]any); ok {
				ref.City = pickStr(addr, "cidade")
			}
			c.Services = append(c.Services, ref)
			out.Services++
		}
		if len(c.Services) > 0 {
			out.Clients = append(out.Clients, c)
		}
	}
	return out
}

type PreventiveCount struct {
	ServiceID  string `json:"service_id"`
	Preventive int    `json:"preventive"` // desbloqueios cuja observação contém "preventivo"
	LastAt     string `json:"last_at,omitempty"`
	// Dates — datas (YYYY-MM-DD) de cada desbloqueio PREVENTIVO, para o filtro de período do front.
	Dates []string `json:"dates,omitempty"`
}

type PreventiveChunk struct {
	OK      bool              `json:"ok"`
	Message string            `json:"message,omitempty"`
	Items   []PreventiveCount `json:"items"` // só serviços com pelo menos 1 desbloqueio
	Failed  []string          `json:"failed,omitempty"`
}

// BuildPreventiveChunk consulta os clientes do lote (até 4 em paralelo) e conta os desbloqueios.
func BuildPreventiveChunk(ctx context.Context, cfg Config, token string, clientIDs []string) PreventiveChunk {
	out := PreventiveChunk{OK: true, Items: []PreventiveCount{}}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	asked := 0
	for _, cid := range clientIDs {
		cid = strings.TrimSpace(cid)
		if cid == "" {
			continue
		}
		asked++
		wg.Add(1)
		go func(cid string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()
			res := integrationhttp.Execute(ctx, cfg.integ(token), integrationhttp.RequestConfig{
				Method: "GET", Path: "/api/v1/integracao/cliente",
				QueryParams: paramKVs(map[string]string{"busca": "id_cliente", "termo_busca": cid, "cancelado": "nao", "inativo": "todos", "limit": "5", "incluir_desbloqueios": "sim"}),
			})
			mu.Lock()
			defer mu.Unlock()
			if !res.OK {
				out.Failed = append(out.Failed, cid)
				return
			}
			var doc struct {
				Clientes []map[string]any `json:"clientes"`
			}
			if json.Unmarshal(ResponseBodyBytes(res), &doc) != nil {
				out.Failed = append(out.Failed, cid)
				return
			}
			for _, c := range doc.Clientes {
				if pickStr(c, "id_cliente") != cid {
					continue
				}
				svcs, _ := c["servicos"].([]any)
				for _, it := range svcs {
					sm, ok := it.(map[string]any)
					if !ok {
						continue
					}
					ds, _ := sm["desbloqueios"].([]any)
					if len(ds) == 0 {
						continue
					}
					pc := PreventiveCount{ServiceID: pickStr(sm, "id_cliente_servico")}
					var last string
					for _, dit := range ds {
						dm, ok := dit.(map[string]any)
						if !ok {
							continue
						}
						if strings.Contains(strings.ToLower(pickStr(dm, "observacao")), "preventivo") {
							pc.Preventive++
							if t := parseBRDate(pickStr(dm, "data_cadastro")); !t.IsZero() {
								pc.Dates = append(pc.Dates, t.Format("2006-01-02"))
							}
						}
						if t := parseBRDate(pickStr(dm, "data_cadastro")); !t.IsZero() {
							if s := t.Format("2006-01-02"); s > last {
								last = s
							}
						}
					}
					pc.LastAt = last
					out.Items = append(out.Items, pc)
				}
			}
		}(cid)
	}
	wg.Wait()
	if asked > 0 && len(out.Failed) == asked {
		out.OK = false
		out.Message = fmt.Sprintf("Falha ao consultar %d cliente(s) na HubSoft.", len(out.Failed))
	}
	return out
}
