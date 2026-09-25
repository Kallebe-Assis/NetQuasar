package integrationhubsoft

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// --- Relatório: serviços bloqueados/suspensos por débito ----------------------------------------
//
// A data do bloqueio vem de `data_ultima_suspensao` — campo que a HubSoft devolve em cada serviço
// de /cliente/todos (confirmado ao vivo): é a data da ÚLTIMA suspensão, então um serviço bloqueado
// em 01/09, liberado no mesmo dia e bloqueado de novo em 14/09 aparece como 14/09 (e não 01/09).
// A API não filtra por esse campo, então varre todos os serviços que estão suspensos AGORA
// (servico_status) e filtra a janela aqui. Sem `data_ultima_suspensao` no serviço (raro), cai para
// `data_atualizacao` e a linha é marcada como aproximada.

type BlockedServiceRow struct {
	ClientCode  string `json:"client_code,omitempty"`
	ClientName  string `json:"client_name,omitempty"`
	ClientID    string `json:"client_id,omitempty"`
	Document    string `json:"document,omitempty"`
	Phone       string `json:"phone,omitempty"`
	ServiceID   string `json:"service_id,omitempty"`
	ServiceName string `json:"service_name,omitempty"`
	Login       string `json:"login,omitempty"`
	Status      string `json:"status,omitempty"`
	City        string `json:"city,omitempty"`
	BlockedAt   string `json:"blocked_at,omitempty"` // YYYY-MM-DD
	DaysBlocked int    `json:"days_blocked"`
	// DateApprox — sem data_ultima_suspensao no serviço: a data é a da última alteração do cadastro.
	DateApprox bool `json:"date_approx,omitempty"`
}

type BlockedServicesReport struct {
	OK      bool                `json:"ok"`
	Message string              `json:"message,omitempty"`
	From    string              `json:"from"`
	To      string              `json:"to"`
	Rows    []BlockedServiceRow `json:"rows"`
	Total   int                 `json:"total"`
	AvgDays float64             `json:"avg_days"`
	Buckets []NamedCount        `json:"buckets"`
	// DateSource: "payload" (todas as linhas com data_ultima_suspensao) | "mixed" (algumas caíram para
	// data_atualizacao — ver DateApprox por linha).
	DateSource string `json:"date_source,omitempty"`
	Truncated  bool   `json:"truncated,omitempty"`
}

func dayStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
}

// BuildBlockedServicesReport lista serviços suspensos (por débito por omissão) cujo ÚLTIMO bloqueio
// caiu em [from, to] (YYYY-MM-DD).
func BuildBlockedServicesReport(ctx context.Context, cfg Config, token, from, to, statuses string) BlockedServicesReport {
	fromT, err1 := time.ParseInLocation("2006-01-02", strings.TrimSpace(from), time.Local)
	toT, err2 := time.ParseInLocation("2006-01-02", strings.TrimSpace(to), time.Local)
	if err1 != nil || err2 != nil || toT.Before(fromT) {
		return BlockedServicesReport{OK: false, Message: "Período inválido (use YYYY-MM-DD, com 'até' ≥ 'de')."}
	}
	if strings.TrimSpace(statuses) == "" {
		statuses = "suspenso_debito"
	}
	today := dayStart(time.Now())
	rep := BlockedServicesReport{OK: true, From: from, To: to, Rows: []BlockedServiceRow{}, DateSource: "payload"}

	items, total, err := fetchAllPages(ctx, cfg, token, "/api/v1/integracao/cliente/todos", map[string]string{
		"servico_status": statuses, "cancelado": "nao", "relacoes": "endereco_instalacao",
	}, 300, "clientes")
	if err != nil {
		return BlockedServicesReport{OK: false, Message: "Falha ao consultar a HubSoft: " + err.Error()}
	}
	rep.Truncated = total > len(items)

	for _, m := range items {
		svcArr, _ := m["servicos"].([]any)
		for _, sit := range svcArr {
			sm, ok := sit.(map[string]any)
			if !ok {
				continue
			}
			// Só suspensos — o cliente pode ter outros serviços em outro status.
			if prefix := pickStr(sm, "status_prefixo"); prefix != "" && prefix != "suspenso_debito" && prefix != "suspenso_parcialmente" && prefix != "suspenso_pedido_cliente" {
				continue
			}
			approx := false
			t := parseBRDate(pickStr(sm, "data_ultima_suspensao"))
			if t.IsZero() {
				t = parseBRDate(pickStr(sm, "data_atualizacao"))
				approx = true
			}
			if t.IsZero() {
				continue
			}
			d := dayStart(t)
			if d.Before(fromT) || d.After(toT) {
				continue
			}
			days := int(today.Sub(d).Hours() / 24)
			if days < 0 {
				days = 0
			}
			if approx {
				rep.DateSource = "mixed"
			}
			row := BlockedServiceRow{
				ClientID: pickStr(m, "id_cliente"), ClientCode: pickStr(m, "codigo_cliente"),
				ClientName: pickStr(m, "nome_razaosocial"), Document: formatCPFCNPJ(pickStr(m, "cpf_cnpj")),
				Phone:     formatPhoneBR(pickStr(m, "telefone_primario", "telefone")),
				ServiceID: pickStr(sm, "id_cliente_servico"), ServiceName: pickStr(sm, "nome"), Login: pickStr(sm, "login"),
				Status: pickStr(sm, "status"), BlockedAt: d.Format("2006-01-02"), DaysBlocked: days, DateApprox: approx,
			}
			if addr, ok := sm["endereco_instalacao"].(map[string]any); ok {
				row.City = pickStr(addr, "cidade")
			}
			rep.Rows = append(rep.Rows, row)
		}
	}

	sort.Slice(rep.Rows, func(i, j int) bool { return rep.Rows[i].DaysBlocked > rep.Rows[j].DaysBlocked })
	rep.Total = len(rep.Rows)
	edges := []struct {
		label string
		max   int
	}{{"0–7 dias", 7}, {"8–15 dias", 15}, {"16–30 dias", 30}, {"31–60 dias", 60}, {"61–90 dias", 90}, {"Mais de 90 dias", 1 << 30}}
	counts := make([]int, len(edges))
	sum := 0
	for _, r := range rep.Rows {
		sum += r.DaysBlocked
		for i, e := range edges {
			if r.DaysBlocked <= e.max {
				counts[i]++
				break
			}
		}
	}
	for i, e := range edges {
		rep.Buckets = append(rep.Buckets, NamedCount{Name: e.label, Count: counts[i]})
	}
	if rep.Total > 0 {
		rep.AvgDays = float64(sum) / float64(rep.Total)
	}
	if rep.Total == 0 {
		rep.Message = fmt.Sprintf("Nenhum serviço suspenso com último bloqueio entre %s e %s.", from, to)
	}
	return rep
}
