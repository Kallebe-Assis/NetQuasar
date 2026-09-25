package integrationhubsoft

import (
	"context"
	"strings"
	"sync"
	"unicode"
)

// --- Consulta em massa de clientes por nome --------------------------------------------------------

type BulkServiceRow struct {
	Plan   string `json:"plan"`
	Status string `json:"status,omitempty"`
	Login  string `json:"login,omitempty"`
	City   string `json:"city,omitempty"`
	SoldAt string `json:"sold_at,omitempty"` // DD/MM/YYYY
}

type BulkClientMatch struct {
	ID       string           `json:"id"`
	Code     string           `json:"code,omitempty"`
	Name     string           `json:"name"`
	Phone    string           `json:"phone,omitempty"`
	City     string           `json:"city,omitempty"`
	Services []BulkServiceRow `json:"services"`
}

type BulkNameResult struct {
	Query   string            `json:"query"`
	Status  string            `json:"status"` // found | approx | not_found | error
	Message string            `json:"message,omitempty"`
	Matches []BulkClientMatch `json:"matches"`
}

func bulkNorm(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if m, ok := dvAccents[r]; ok {
			r = m
		}
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		case unicode.IsSpace(r) || r == '-' || r == '.':
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// BuildBulkClientLookup busca cada nome (até 4 em paralelo). Se algum resultado tiver o nome
// idêntico (sem acento/caixa), só esses são devolvidos (status "found"); senão devolve as
// correspondências parciais (até 10) com status "approx" para o operador conferir.
func BuildBulkClientLookup(ctx context.Context, cfg Config, token string, names []string) []BulkNameResult {
	out := make([]BulkNameResult, len(names))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for i, name := range names {
		out[i] = BulkNameResult{Query: name, Matches: []BulkClientMatch{}}
		if strings.TrimSpace(name) == "" {
			out[i].Status, out[i].Message = "error", "nome vazio"
			continue
		}
		wg.Add(1)
		go func(i int, name string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				out[i].Status, out[i].Message = "error", "consulta cancelada"
				return
			}
			defer func() { <-sem }()
			res, run := SearchClients(ctx, cfg, token, "nome_razaosocial", strings.TrimSpace(name), false)
			if !run.OK && len(res.Clients) == 0 {
				out[i].Status = "error"
				out[i].Message = firstNonEmpty(res.Message, run.ErrorMessage, "falha na consulta")
				return
			}
			want := bulkNorm(name)
			var exact, all []BulkClientMatch
			for _, c := range res.Clients {
				m := BulkClientMatch{ID: c.ID, Code: c.Code, Name: c.Name, Phone: c.Phone, Services: []BulkServiceRow{}}
				for _, s := range c.Services {
					m.Services = append(m.Services, BulkServiceRow{Plan: s.Name, Status: s.Status, Login: s.Login, City: s.City, SoldAt: s.SoldAt})
					if m.City == "" {
						m.City = s.City
					}
				}
				all = append(all, m)
				if bulkNorm(c.Name) == want || bulkNorm(c.TradeName) == want {
					exact = append(exact, m)
				}
			}
			switch {
			case len(exact) > 0:
				out[i].Status, out[i].Matches = "found", exact
			case len(all) > 0:
				if len(all) > 10 {
					all = all[:10]
				}
				out[i].Status, out[i].Matches = "approx", all
			default:
				out[i].Status = "not_found"
			}
		}(i, name)
	}
	wg.Wait()
	return out
}
