package integrationhubsoft

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// --- Relatório: tempo de cliente (clientes ativos por faixa de data da venda) --------------------
//
// Só totais. A listagem /cliente/todos não devolve a data da venda dos serviços, mas aceita filtrar
// por ela (tipo_data_cliente_servico=data_venda + data_inicio/fim_cliente_servico) junto com
// servico_status=servico_habilitado. Por isso faz UMA consulta por faixa e usa o próprio filtro da
// API para classificar. Um cliente com mais de um serviço ativo cai na faixa MAIS ANTIGA em que tem
// um serviço (o tempo de casa dele é o do serviço mais velho).

type TenureReport struct {
	OK      bool         `json:"ok"`
	Message string       `json:"message,omitempty"`
	Total   int          `json:"total"` // clientes ativos segundo a HubSoft (sem filtro de data)
	Buckets []NamedCount `json:"buckets"`
	// Services — serviços ativos por faixa (mesma ordem de Buckets). Um cliente pode ter mais de um.
	Services []int `json:"services"`
	// Bands — limites (YYYY-MM-DD) de cada faixa, na mesma ordem de Buckets; o front os usa para abrir
	// o detalhe da faixa (report/tenure/detail).
	Bands []TenureBandRange `json:"details"`
	// Truncated — alguma faixa passou do teto de páginas: os números dela estão incompletos.
	Truncated bool `json:"truncated,omitempty"`
}

type TenureBandRange struct {
	Name string `json:"name"`
	From string `json:"from"`
	To   string `json:"to"`
}

func BuildTenureReport(ctx context.Context, cfg Config, token string) TenureReport {
	const path = "/api/v1/integracao/cliente/todos"
	today := dayStart(time.Now())
	day := func(t time.Time) string { return t.Format("2006-01-02") }
	back := func(months int) time.Time { return today.AddDate(0, -months, 0) }

	type band struct {
		label    string
		from, to time.Time
	}
	// do mais antigo para o mais novo (a ordem define a prioridade de um cliente com vários serviços)
	// Limites inclusivos nas duas pontas, como o filtro da tela da HubSoft (de 25/03 até hoje = "até 6
	// meses"): cada faixa começa NO dia do limite mais antigo e termina no dia anterior ao limite mais novo.
	dayBefore := func(t time.Time) time.Time { return t.AddDate(0, 0, -1) }
	bands := []band{
		{"Mais de 10 anos", time.Date(1990, 1, 1, 0, 0, 0, 0, time.Local), dayBefore(back(120))},
		{"De 9 a 10 anos", back(120), dayBefore(back(108))},
		{"De 8 a 9 anos", back(108), dayBefore(back(96))},
		{"De 7 a 8 anos", back(96), dayBefore(back(84))},
		{"De 6 a 7 anos", back(84), dayBefore(back(72))},
		{"De 5 a 6 anos", back(72), dayBefore(back(60))},
		{"De 4 a 5 anos", back(60), dayBefore(back(48))},
		{"De 3 a 4 anos", back(48), dayBefore(back(36))},
		{"De 2 a 3 anos", back(36), dayBefore(back(24))},
		{"De 1 a 2 anos", back(24), dayBefore(back(12))},
		{"De 6 meses a 1 ano", back(12), dayBefore(back(6))},
		{"Até 6 meses", back(6), today},
	}

	// total de clientes ativos: 1 pedido (paginacao.total_registros)
	_, total, err := fetchAllPages(ctx, cfg, token, path, map[string]string{"servico_status": "servico_habilitado", "cancelado": "nao"}, 1, "clientes")
	if err != nil {
		return TenureReport{OK: false, Message: "Falha ao consultar a HubSoft: " + err.Error()}
	}

	type result struct {
		ids       []string // clientes com serviço ativo na faixa
		services  int
		truncated bool
		err       error
	}
	results := make([]result, len(bands))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 3)
	for i, b := range bands {
		wg.Add(1)
		go func(i int, b band) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				results[i].err = ctx.Err()
				return
			}
			defer func() { <-sem }()
			items, tr, err := fetchAllPages(ctx, cfg, token, path, map[string]string{
				"servico_status": "servico_habilitado", "cancelado": "nao",
				"tipo_data_cliente_servico": "data_venda", "data_inicio_cliente_servico": day(b.from), "data_fim_cliente_servico": day(b.to),
			}, 300, "clientes")
			if err != nil {
				results[i].err = err
				return
			}
			results[i].truncated = tr > len(items)
			for _, m := range items {
				svc := countActiveServices(m, b.from, b.to)
				if svc == 0 {
					continue
				}
				results[i].services += svc
				if id := pickStr(m, "id_cliente", "codigo_cliente"); id != "" {
					results[i].ids = append(results[i].ids, id)
				}
			}
		}(i, b)
	}
	wg.Wait()

	rep := TenureReport{OK: true, Total: total}
	counted := 0
	seen := map[string]bool{}
	counts := make([]int, len(bands))
	for i := range bands {
		if results[i].err != nil {
			return TenureReport{OK: false, Message: fmt.Sprintf("Falha na faixa %q: %v", bands[i].label, results[i].err)}
		}
		if results[i].truncated {
			rep.Truncated = true
		}
		for _, id := range results[i].ids {
			if !seen[id] {
				seen[id] = true
				counts[i]++
			}
		}
	}
	// exibição do mais novo para o mais antigo
	for i := len(bands) - 1; i >= 0; i-- {
		rep.Buckets = append(rep.Buckets, NamedCount{Name: bands[i].label, Count: counts[i]})
		rep.Services = append(rep.Services, results[i].services)
		rep.Bands = append(rep.Bands, TenureBandRange{Name: bands[i].label, From: day(bands[i].from), To: day(bands[i].to)})
		counted += counts[i]
	}
	if rep.Total < counted {
		rep.Total = counted
	}
	return rep
}

// countActiveServices conta os serviços ATIVOS do cliente que pertencem à faixa [from,to]. Se o payload
// trouxer data_venda no serviço, ela confirma a faixa (exato); se não trouxer, confia-se no filtro da
// API. Sem status_prefixo em nenhum serviço, também se confia no filtro da API.
func countActiveServices(m map[string]any, from, to time.Time) int {
	arr, _ := m["servicos"].([]any)
	if len(arr) == 0 {
		return 1
	}
	services, outside := 0, 0
	anyPrefix := false
	for _, it := range arr {
		sm, ok := it.(map[string]any)
		if !ok {
			continue
		}
		if p := pickStr(sm, "status_prefixo"); p != "" {
			anyPrefix = true
			if p != "servico_habilitado" {
				continue
			}
		}
		if t := parseBRDate(pickStr(sm, "data_venda")); !t.IsZero() {
			if d := dayStart(t); d.Before(from) || d.After(to) {
				outside++
				continue
			}
		}
		services++
	}
	if !anyPrefix && services == 0 && outside == 0 {
		return 1
	}
	return services
}

// TenureBandDetailReport — repartição de UMA faixa de tempo de cliente por localidade e por plano
// (serviços ativos com data de venda em [From, To]). Alimenta os gráficos do modal da faixa.
type TenureBandDetailReport struct {
	OK        bool         `json:"ok"`
	Message   string       `json:"message,omitempty"`
	From      string       `json:"from"`
	To        string       `json:"to"`
	Services  int          `json:"services"`
	Clients   int          `json:"clients"`
	ByCity    []NamedCount `json:"by_city"`
	ByPlan    []NamedCount `json:"by_plan"`
	Truncated bool         `json:"truncated,omitempty"`
}

func BuildTenureBandDetail(ctx context.Context, cfg Config, token, from, to string) TenureBandDetailReport {
	fromT, err1 := time.ParseInLocation("2006-01-02", from, time.Local)
	toT, err2 := time.ParseInLocation("2006-01-02", to, time.Local)
	if err1 != nil || err2 != nil || toT.Before(fromT) {
		return TenureBandDetailReport{OK: false, Message: "Período inválido (use YYYY-MM-DD, com 'até' ≥ 'de')."}
	}
	items, total, err := fetchAllPages(ctx, cfg, token, "/api/v1/integracao/cliente/todos", map[string]string{
		"servico_status": "servico_habilitado", "cancelado": "nao", "relacoes": "endereco_instalacao",
		"tipo_data_cliente_servico": "data_venda", "data_inicio_cliente_servico": from, "data_fim_cliente_servico": to,
	}, 300, "clientes")
	if err != nil {
		return TenureBandDetailReport{OK: false, Message: "Falha ao consultar a HubSoft: " + err.Error()}
	}
	rep := TenureBandDetailReport{OK: true, From: from, To: to, Truncated: total > len(items)}
	city := map[string]int{}
	plan := map[string]int{}
	clients := map[string]bool{}
	for _, m := range items {
		arr, _ := m["servicos"].([]any)
		for _, it := range arr {
			sm, ok := it.(map[string]any)
			if !ok {
				continue
			}
			if p := pickStr(sm, "status_prefixo"); p != "" && p != "servico_habilitado" {
				continue
			}
			if t := parseBRDate(pickStr(sm, "data_venda")); !t.IsZero() {
				if d := dayStart(t); d.Before(fromT) || d.After(toT) {
					continue
				}
			}
			label := "Sem localidade"
			if addr, ok := sm["endereco_instalacao"].(map[string]any); ok {
				if c := pickStr(addr, "cidade"); c != "" {
					label = c
					if uf := pickStr(addr, "estado"); uf != "" {
						label = c + " / " + uf
					}
				}
			}
			name := pickStr(sm, "nome")
			if name == "" {
				name = "Sem plano"
			}
			city[label]++
			plan[name]++
			rep.Services++
			if id := pickStr(m, "id_cliente", "codigo_cliente"); id != "" {
				clients[id] = true
			}
		}
	}
	rep.Clients = len(clients)
	rep.ByCity = sortedCounts(city)
	rep.ByPlan = sortedCounts(plan)
	if rep.Services == 0 {
		rep.Message = "Nenhum serviço ativo nesta faixa."
	}
	return rep
}

func sortedCounts(m map[string]int) []NamedCount {
	out := make([]NamedCount, 0, len(m))
	for k, v := range m {
		out = append(out, NamedCount{Name: k, Count: v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Name < out[j].Name
	})
	return out
}
