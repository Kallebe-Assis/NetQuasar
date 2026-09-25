package integrationhubsoft

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/netquasar/netquasar/quasar_backend/internal/integrationhttp"
)

// --- Correção em lote da "data da venda" (PROVISÓRIO, só administradores) ---------------------------
//
// PUT /api/v1/integracao/cliente/cliente_servico/editar/:id  {"data_venda":"YYYY-MM-DD"}
// (a doc exige que data_venda seja o único parâmetro do pedido). Fluxo em duas fases:
//   1. PreviewDataVenda — não altera nada: cruza cada linha do CSV com a base viva da HubSoft.
//   2. ApplyDataVendaRow — uma linha por chamada; revalida tudo imediatamente antes do PUT e
//      confere a resposta e uma releitura depois. Qualquer inconsistência devolve Halt=true.

type DataVendaInputRow struct {
	Line       int    `json:"line"`
	Login      string `json:"login"`
	ServiceID  string `json:"service_id"`
	ClientCode string `json:"client_code"`
	ClientName string `json:"client_name"`
	NewDate    string `json:"new_date"` // YYYY-MM-DD ou DD/MM/YYYY
}

type DataVendaPreviewRow struct {
	Line        int      `json:"line"`
	Login       string   `json:"login"`
	ServiceID   string   `json:"service_id,omitempty"`
	ClientID    string   `json:"client_id,omitempty"`
	ClientCode  string   `json:"client_code,omitempty"`
	ClientName  string   `json:"client_name,omitempty"`
	Status      string   `json:"status,omitempty"`
	CurrentDate string   `json:"current_date,omitempty"` // só se o payload da HubSoft trouxer
	NewDate     string   `json:"new_date,omitempty"`     // YYYY-MM-DD normalizada
	Approved    bool     `json:"approved"`
	Reason      string   `json:"reason,omitempty"`
	Warnings    []string `json:"warnings,omitempty"`
}

type DataVendaPreview struct {
	OK       bool                  `json:"ok"`
	Message  string                `json:"message,omitempty"`
	Rows     []DataVendaPreviewRow `json:"rows"`
	Approved int                   `json:"approved"`
	Blocked  int                   `json:"blocked"`
	BaseSize int                   `json:"base_size"`
}

type DataVendaApplyInput struct {
	Line       int    `json:"line"`
	Login      string `json:"login"`
	ServiceID  string `json:"service_id"`
	ClientID   string `json:"client_id"`
	ClientName string `json:"client_name"`
	NewDate    string `json:"new_date"` // YYYY-MM-DD
}

type DataVendaApplyResult struct {
	Line       int    `json:"line"`
	ServiceID  string `json:"service_id"`
	Login      string `json:"login"`
	OK         bool   `json:"ok"`
	Halt       bool   `json:"halt,omitempty"` // inconsistência grave: o chamador deve parar tudo
	Message    string `json:"message"`
	DateBefore string `json:"date_before,omitempty"`
	DateAfter  string `json:"date_after,omitempty"`
}

var dvAccents = map[rune]rune{'á': 'a', 'à': 'a', 'â': 'a', 'ã': 'a', 'ä': 'a', 'é': 'e', 'è': 'e', 'ê': 'e', 'ë': 'e', 'í': 'i', 'ì': 'i', 'î': 'i', 'ï': 'i', 'ó': 'o', 'ò': 'o', 'ô': 'o', 'õ': 'o', 'ö': 'o', 'ú': 'u', 'ù': 'u', 'û': 'u', 'ü': 'u', 'ç': 'c', 'ñ': 'n'}

func dvNormName(s string) string {
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

func dvNormLogin(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

func dvNormDate(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, " T"); i > 0 {
		s = s[:i]
	}
	for _, l := range []string{"2006-01-02", "02/01/2006", "2/1/2006", "02-01-2006"} {
		if t, err := time.Parse(l, s); err == nil {
			return t.Format("2006-01-02")
		}
	}
	return ""
}

func dvSuspiciousLogin(s string) string {
	var p []string
	add := func(x string) {
		for _, e := range p {
			if e == x {
				return
			}
		}
		p = append(p, x)
	}
	if s != strings.TrimSpace(s) {
		add("espaço nas pontas")
	}
	for _, r := range s {
		switch {
		case r == 0xA0 || r == 0x200B || r == 0xFEFF || r == 0x200E || r == 0x200F:
			add("caractere invisível")
		case r > 127:
			add("caractere não-ASCII")
		}
	}
	if strings.Contains(strings.TrimSpace(s), " ") {
		add("espaço no meio")
	}
	return strings.Join(p, ", ")
}

type dvService struct {
	ServiceID, ClientID, ClientCode, ClientName, Login, Status, DataVenda string
}

func dvServicesFrom(clients []map[string]any) []dvService {
	var out []dvService
	for _, c := range clients {
		arr, _ := c["servicos"].([]any)
		for _, it := range arr {
			s, ok := it.(map[string]any)
			if !ok {
				continue
			}
			id := pickStr(s, "id_cliente_servico")
			if id == "" {
				continue
			}
			out = append(out, dvService{
				ServiceID: id, ClientID: pickStr(c, "id_cliente"), ClientCode: pickStr(c, "codigo_cliente"),
				ClientName: pickStr(c, "nome_razaosocial"), Login: pickStr(s, "login"), Status: pickStr(s, "status"),
				DataVenda: dvNormDate(pickStr(s, "data_venda")),
			})
		}
	}
	return out
}

// dvLookup consulta /cliente (busca=<tipo>) e devolve todos os serviços retornados.
func dvLookup(ctx context.Context, cfg Config, token, busca, termo string) ([]dvService, error) {
	res := integrationhttp.Execute(ctx, cfg.integ(token), integrationhttp.RequestConfig{
		Method: "GET", Path: "/api/v1/integracao/cliente",
		QueryParams: paramKVs(map[string]string{"busca": busca, "termo_busca": termo, "cancelado": "todos", "inativo": "todos", "limit": "100"}),
	})
	if !res.OK {
		return nil, fmt.Errorf("%s", firstNonEmpty(res.ErrorMessage, fmt.Sprintf("HTTP %d", res.StatusCode)))
	}
	var doc struct {
		Clientes []map[string]any `json:"clientes"`
	}
	if err := json.Unmarshal(ResponseBodyBytes(res), &doc); err != nil {
		return nil, err
	}
	return dvServicesFrom(doc.Clientes), nil
}

// dvFetchBase lê a base COMPLETA de serviços (cliente/todos). Devolve mensagem de erro se a leitura
// não for completa — sem a base inteira não dá para garantir que um login é único.
func dvFetchBase(ctx context.Context, cfg Config, token string, includeCancelled bool) ([]dvService, string) {
	var base []dvService
	seenSvc := map[string]bool{}
	flags := []string{"nao"}
	if includeCancelled {
		flags = append(flags, "sim")
	}
	for _, canc := range flags {
		items, total, err := fetchAllPages(ctx, cfg, token, "/api/v1/integracao/cliente/todos", map[string]string{"cancelado": canc}, 400, "clientes")
		if err != nil {
			return nil, "Falha ao consultar a HubSoft: " + err.Error()
		}
		if total > len(items) {
			return nil, fmt.Sprintf("Leitura incompleta da base (%d de %d clientes) — recusado por segurança. Tente de novo.", len(items), total)
		}
		for _, s := range dvServicesFrom(items) {
			if !seenSvc[s.ServiceID] {
				seenSvc[s.ServiceID] = true
				base = append(base, s)
			}
		}
	}
	return base, ""
}

type DataVendaExportRow struct {
	Login       string `json:"login"`
	ServiceID   string `json:"service_id"`
	ClientCode  string `json:"client_code"`
	ClientName  string `json:"client_name"`
	Status      string `json:"status"`
	CurrentDate string `json:"current_date,omitempty"`
}

// ExportDataVendaBase devolve todos os serviços para o operador montar o CSV de correção.
func ExportDataVendaBase(ctx context.Context, cfg Config, token string, includeCancelled bool) ([]DataVendaExportRow, string) {
	base, msg := dvFetchBase(ctx, cfg, token, includeCancelled)
	if msg != "" {
		return nil, msg
	}
	rows := make([]DataVendaExportRow, 0, len(base))
	for _, s := range base {
		rows = append(rows, DataVendaExportRow{Login: s.Login, ServiceID: s.ServiceID, ClientCode: s.ClientCode, ClientName: s.ClientName, Status: s.Status, CurrentDate: s.DataVenda})
	}
	return rows, ""
}

// PreviewDataVenda cruza as linhas do CSV com a base inteira (cliente/todos) — não altera nada.
func PreviewDataVenda(ctx context.Context, cfg Config, token string, rows []DataVendaInputRow, includeCancelled bool) DataVendaPreview {
	out := DataVendaPreview{Rows: []DataVendaPreviewRow{}}
	if len(rows) == 0 {
		out.Message = "CSV sem linhas preenchidas."
		return out
	}
	base, msg := dvFetchBase(ctx, cfg, token, includeCancelled)
	if msg != "" {
		out.Message = msg
		return out
	}
	out.BaseSize = len(base)
	byLogin := map[string][]dvService{}
	byID := map[string]dvService{}
	for _, s := range base {
		byID[s.ServiceID] = s
		if k := dvNormLogin(s.Login); k != "" {
			byLogin[k] = append(byLogin[k], s)
		}
	}

	now := time.Now()
	minDate := time.Date(2005, 1, 1, 0, 0, 0, 0, time.UTC)
	seenLogin := map[string]int{}
	seenID := map[string]int{}
	for _, in := range rows {
		p := DataVendaPreviewRow{Line: in.Line, Login: strings.TrimSpace(in.Login), NewDate: dvNormDate(in.NewDate)}
		block := func(msg string) {
			if p.Reason == "" {
				p.Reason = msg
			}
		}
		key := dvNormLogin(in.Login)
		if w := dvSuspiciousLogin(in.Login); w != "" {
			p.Warnings = append(p.Warnings, "login com "+w)
		}
		switch {
		case key == "":
			block("login vazio")
		case p.NewDate == "":
			block("data inválida: " + in.NewDate)
		default:
			d, _ := time.Parse("2006-01-02", p.NewDate)
			if d.After(now) {
				block("data nova no futuro")
			} else if d.Before(minDate) {
				block("data nova anterior a 2005")
			}
		}
		if key != "" {
			if prev, dup := seenLogin[key]; dup {
				block(fmt.Sprintf("login repetido no CSV (linha %d)", prev))
			}
			seenLogin[key] = in.Line
		}
		if id := strings.TrimSpace(in.ServiceID); id != "" {
			if prev, dup := seenID[id]; dup {
				block(fmt.Sprintf("id do serviço repetido no CSV (linha %d)", prev))
			}
			seenID[id] = in.Line
		}
		if p.Reason == "" {
			m := byLogin[key]
			switch {
			case len(m) == 0:
				if s, ok := byID[strings.TrimSpace(in.ServiceID)]; ok {
					block(fmt.Sprintf("login não encontrado; o id %s hoje tem o login %q", s.ServiceID, s.Login))
				} else {
					block("login não encontrado na HubSoft")
				}
			case len(m) > 1:
				block(fmt.Sprintf("login duplicado na HubSoft (%d serviços) — corrigir manualmente", len(m)))
			default:
				s := m[0]
				p.ServiceID, p.ClientID, p.ClientCode, p.ClientName, p.Status, p.CurrentDate = s.ServiceID, s.ClientID, s.ClientCode, s.ClientName, s.Status, s.DataVenda
				if s.Login != strings.TrimSpace(in.Login) {
					p.Warnings = append(p.Warnings, fmt.Sprintf("login difere em maiúsculas (HubSoft: %q)", s.Login))
				}
				switch {
				case strings.TrimSpace(in.ServiceID) == "":
					block("id_cliente_servico vazio no CSV")
				case strings.TrimSpace(in.ServiceID) != s.ServiceID:
					block(fmt.Sprintf("id do CSV (%s) ≠ id atual do login (%s)", in.ServiceID, s.ServiceID))
				case strings.TrimSpace(in.ClientCode) != "" && strings.TrimSpace(in.ClientCode) != s.ClientCode:
					block(fmt.Sprintf("código do cliente do CSV (%s) ≠ HubSoft (%s)", in.ClientCode, s.ClientCode))
				case dvNormName(in.ClientName) != dvNormName(s.ClientName):
					block(fmt.Sprintf("nome do CSV (%q) ≠ HubSoft (%q)", in.ClientName, s.ClientName))
				case s.DataVenda != "" && s.DataVenda == p.NewDate:
					block("data de venda já está correta")
				}
				if strings.Contains(strings.ToLower(s.Status), "cancel") {
					p.Warnings = append(p.Warnings, "serviço CANCELADO")
				}
			}
		}
		p.Approved = p.Reason == ""
		if p.Approved {
			out.Approved++
		} else {
			out.Blocked++
		}
		out.Rows = append(out.Rows, p)
	}
	out.OK = true
	return out
}

// ApplyDataVendaRow altera UM serviço, com revalidação antes do PUT e conferência depois.
func ApplyDataVendaRow(ctx context.Context, cfg Config, token string, in DataVendaApplyInput) DataVendaApplyResult {
	res := DataVendaApplyResult{Line: in.Line, ServiceID: strings.TrimSpace(in.ServiceID), Login: strings.TrimSpace(in.Login)}
	fail := func(halt bool, f string, a ...any) DataVendaApplyResult {
		res.Halt, res.Message = halt, fmt.Sprintf(f, a...)
		return res
	}
	id := res.ServiceID
	newDate := dvNormDate(in.NewDate)
	if id == "" || dvNormLogin(in.Login) == "" || newDate == "" {
		return fail(false, "linha incompleta (id, login e data são obrigatórios)")
	}
	if d, _ := time.Parse("2006-01-02", newDate); d.After(time.Now()) || d.Before(time.Date(2005, 1, 1, 0, 0, 0, 0, time.UTC)) {
		return fail(false, "data fora do intervalo aceito (2005 até hoje)")
	}

	// (1) o id ainda pertence ao mesmo login / cliente / nome?
	byID, err := dvLookup(ctx, cfg, token, "id_cliente_servico", id)
	if err != nil {
		return fail(false, "recheck por id falhou: %v", err)
	}
	var cur *dvService
	for i := range byID {
		if byID[i].ServiceID == id {
			cur = &byID[i]
			break
		}
	}
	switch {
	case cur == nil:
		return fail(false, "serviço %s não encontrado no recheck", id)
	case dvNormLogin(cur.Login) != dvNormLogin(in.Login):
		return fail(false, "recheck: o serviço %s agora tem o login %q (esperado %q)", id, cur.Login, in.Login)
	case strings.TrimSpace(in.ClientID) != "" && cur.ClientID != strings.TrimSpace(in.ClientID):
		return fail(false, "recheck: o dono do serviço mudou")
	case dvNormName(cur.ClientName) != dvNormName(in.ClientName):
		return fail(false, "recheck: nome do cliente mudou para %q", cur.ClientName)
	}
	res.DateBefore = cur.DataVenda

	// (2) o login continua único e aponta para este id? (consulta independente por login)
	byLogin, err := dvLookup(ctx, cfg, token, "login_radius", strings.TrimSpace(in.Login))
	if err != nil {
		return fail(false, "recheck por login falhou: %v", err)
	}
	exact := 0
	sameID := false
	for _, s := range byLogin {
		if dvNormLogin(s.Login) == dvNormLogin(in.Login) {
			exact++
			if s.ServiceID == id {
				sameID = true
			}
		}
	}
	if exact != 1 || !sameID {
		return fail(false, "recheck por login: %d serviço(s) com o login %q (esperado exatamente 1, o id %s)", exact, in.Login, id)
	}

	// (3) PUT — só data_venda, como a doc exige.
	body, _ := json.Marshal(map[string]string{"data_venda": newDate})
	put := integrationhttp.Execute(ctx, cfg.integ(token), integrationhttp.RequestConfig{
		Method: "PUT", Path: "/api/v1/integracao/cliente/cliente_servico/editar/" + id,
		BodyTemplate: string(body), BodyType: "json",
	})
	raw := ResponseBodyBytes(put)
	if !put.OK {
		if put.StatusCode == 0 {
			return fail(true, "PUT sem resposta — estado desconhecido, confira o serviço %s antes de continuar", id)
		}
		if msg := hubsoftActionMessage(raw); msg != "" {
			return fail(false, "HubSoft recusou: %s", msg)
		}
		return fail(false, "HubSoft recusou: %s", firstNonEmpty(put.ErrorMessage, fmt.Sprintf("HTTP %d", put.StatusCode)))
	}

	// (4) a resposta descreve exatamente o que era esperado?
	var doc map[string]any
	_ = json.Unmarshal(raw, &doc)
	cs, _ := doc["cliente_servico"].(map[string]any)
	if pickStr(doc, "status") != "success" || pickStr(cs, "id_cliente_servico") != id ||
		(pickStr(cs, "id_cliente") != "" && cur.ClientID != "" && pickStr(cs, "id_cliente") != cur.ClientID) ||
		dvNormDate(pickStr(cs, "data_venda")) != newDate {
		return fail(true, "RESPOSTA INCONSISTENTE do PUT no serviço %s (id=%s, cliente=%s, data=%s) — pare e confira", id,
			pickStr(cs, "id_cliente_servico"), pickStr(cs, "id_cliente"), pickStr(cs, "data_venda"))
	}
	res.DateAfter = newDate

	// (5) releitura independente
	after, err := dvLookup(ctx, cfg, token, "id_cliente_servico", id)
	if err == nil {
		for _, s := range after {
			if s.ServiceID != id {
				continue
			}
			if dvNormLogin(s.Login) != dvNormLogin(in.Login) {
				return fail(true, "PÓS-PUT: o login do serviço %s virou %q", id, s.Login)
			}
			if s.DataVenda != "" && s.DataVenda != newDate {
				return fail(true, "PÓS-PUT: data_venda lida = %s (esperado %s) no serviço %s", s.DataVenda, newDate, id)
			}
		}
	}
	res.OK = true
	res.Message = "data de venda atualizada"
	return res
}
