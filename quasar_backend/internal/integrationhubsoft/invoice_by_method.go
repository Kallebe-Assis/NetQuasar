package integrationhubsoft

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// --- Boletos em aberto por forma de cobrança (aba Relatório → Boletos por forma) -------------------------
//
// A forma de cobrança fica gravada na FATURA no momento em que o boleto é gerado. Quando o operador troca a
// forma de cobrança do cliente/serviço em massa, os boletos já gerados continuam com a forma antiga — e é
// justamente esses que este relatório localiza: varre as faturas EM ABERTO (/financeiro/fatura, paginado) e
// filtra pela forma de cobrança da própria fatura. É SOMENTE LEITURA.
//
// O modo "simplificado" do /financeiro/fatura não traz a forma de cobrança, por isso a varredura NÃO envia
// tipo_resultado. Como o nome exato do campo na resposta completa não é garantido pela documentação pública,
// invoiceFormaCobranca procura por variações conhecidas (inclusive aninhadas) e o relatório devolve, junto,
// a lista dos campos que a fatura traz (CamposFatura) para diagnosticar quando nada for encontrado.

// invoiceMethodMaxPages — 100 faturas por página: até 50 mil faturas por varredura.
const invoiceMethodMaxPages = 500

// InvoiceMethodRow é uma fatura em aberto, com a forma de cobrança que ela carrega.
type InvoiceMethodRow struct {
	IDFatura       string `json:"id_fatura"`
	IDCliente      string `json:"id_cliente,omitempty"`
	CodigoCliente  string `json:"codigo_cliente,omitempty"`
	Cliente        string `json:"cliente"`
	Servico        string `json:"servico,omitempty"` // login PPPoE do serviço (quando a fatura o traz)
	Plano          string `json:"plano,omitempty"`   // nome do serviço/plano (fallback visual do login)
	IDServico      string `json:"id_cliente_servico,omitempty"`
	Valor          string `json:"valor"`
	Vencimento     string `json:"vencimento"`
	Status         string `json:"status"` // "pending" (a vencer) | "overdue" (vencida)
	FormaID        string `json:"forma_id,omitempty"`
	FormaNome      string `json:"forma_nome,omitempty"`
	NossoNumero    string `json:"nosso_numero,omitempty"`
	LinhaDigitavel string `json:"linha_digitavel,omitempty"`
	Link           string `json:"link,omitempty"`
}

// InvoiceMethodFormaCount resume quantas faturas em aberto existem em cada forma de cobrança encontrada.
type InvoiceMethodFormaCount struct {
	ID    string  `json:"id,omitempty"`
	Nome  string  `json:"nome"`
	Count int     `json:"count"`
	Valor float64 `json:"valor"`
}

type InvoiceMethodReport struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
	From    string `json:"from"`
	To      string `json:"to"`
	// Match é o filtro pedido (id numérico da forma ou parte do nome).
	Match string `json:"match"`
	// Scanned: faturas em aberto lidas; TotalRegistros: total que a HubSoft informou; Truncated se não coube tudo.
	Scanned        int  `json:"scanned"`
	TotalRegistros int  `json:"total_registros"`
	Truncated      bool `json:"truncated,omitempty"`
	// FormaFound: quantas das faturas lidas trouxeram a forma de cobrança. Zero = o campo não veio na resposta.
	FormaFound int `json:"forma_found"`
	// Formas: contagem por forma de cobrança entre TODAS as faturas lidas (permite escolher o nome certo do filtro).
	Formas       []InvoiceMethodFormaCount `json:"formas"`
	MatchCount   int                       `json:"match_count"`
	MatchValue   float64                   `json:"match_value"`
	Rows         []InvoiceMethodRow        `json:"rows"`
	CamposFatura []string                  `json:"campos_fatura,omitempty"`
	// AmostraFatura: valores (achatados, truncados) da 1ª fatura lida — diagnóstico de campos.
	AmostraFatura map[string]string `json:"amostra_fatura,omitempty"`
}

// BuildInvoicesByMethod varre as faturas em aberto vencidas entre from e to (YYYY-MM-DD) e devolve as que têm a
// forma de cobrança `match`. Com match vazio só devolve a contagem por forma (para descobrir os nomes).
func BuildInvoicesByMethod(ctx context.Context, cfg Config, token, from, to, match string) InvoiceMethodReport {
	rep := InvoiceMethodReport{From: from, To: to, Match: strings.TrimSpace(match), Formas: []InvoiceMethodFormaCount{}, Rows: []InvoiceMethodRow{}}
	items, total, err := fetchAllPages(ctx, cfg, token, "/api/v1/integracao/financeiro/fatura",
		map[string]string{"tipo_data": "data_vencimento", "data_inicio": from, "data_fim": to, "apenas_em_aberto": "sim"},
		invoiceMethodMaxPages, "faturas")
	if err != nil {
		rep.Message = "Falha ao consultar as faturas: " + err.Error()
		return rep
	}
	rep.TotalRegistros = total
	rep.Truncated = total > len(items)
	rep.OK = true
	rep.Rows, rep.Formas, rep.CamposFatura, rep.Scanned, rep.FormaFound, rep.MatchCount, rep.MatchValue = classifyInvoicesByMethod(items, rep.Match)
	if len(items) > 0 {
		rep.AmostraFatura = flattenInvoiceSample(items[0])
	}
	switch {
	case rep.Scanned == 0:
		rep.Message = "Nenhuma fatura em aberto com vencimento nesse período."
	case rep.FormaFound == 0:
		rep.Message = "A HubSoft não devolveu a forma de cobrança nas faturas lidas — veja em «campos da fatura» o que ela traz."
	case rep.Truncated:
		rep.Message = fmt.Sprintf("A varredura leu %d de %d faturas (limite de segurança). Reduza o período para conferir tudo.", len(items), total)
	}
	return rep
}

// classifyInvoicesByMethod é a parte pura (testável) do relatório: separa as faturas realmente em aberto,
// conta por forma de cobrança e devolve as que casam com `match`, das mais antigas para as mais novas.
func classifyInvoicesByMethod(items []map[string]any, match string) (rows []InvoiceMethodRow, formas []InvoiceMethodFormaCount, campos []string, scanned, found, matchCount int, matchValue float64) {
	rows = []InvoiceMethodRow{}
	formas = []InvoiceMethodFormaCount{}
	byForma := map[string]*InvoiceMethodFormaCount{}
	for _, m := range items {
		// Defesa extra: o filtro apenas_em_aberto já vem da HubSoft, mas paga/cancelada nunca entra.
		if strings.TrimSpace(pickStr(m, "data_pagamento")) != "" || pickBool(m, "quitado") || pickBool(m, "cancelado") {
			continue
		}
		scanned++
		if campos == nil {
			campos = invoiceFieldNames(m)
		}
		id, nome := invoiceFormaCobranca(m)
		val := parseBRFloat(pickStr(m, "valor"))
		if id != "" || nome != "" {
			found++
			key := id + "|" + alnumKey(nome)
			fc := byForma[key]
			if fc == nil {
				fc = &InvoiceMethodFormaCount{ID: id, Nome: nome}
				byForma[key] = fc
			}
			fc.Count++
			fc.Valor += val
		}
		if match == "" || !formaMatches(id, nome, match) {
			continue
		}
		row := InvoiceMethodRow{
			IDFatura: pickStr(m, "id_fatura", "id"), Valor: pickStr(m, "valor"), Vencimento: formatBRDate(pickStr(m, "data_vencimento")),
			Status: deriveInvoiceStatus(m), FormaID: id, FormaNome: nome,
			NossoNumero: pickStr(m, "nosso_numero"), LinhaDigitavel: pickStr(m, "linha_digitavel"), Link: pickStr(m, "link"),
		}
		row.IDCliente = findInvoiceStr(m, "id_cliente")
		row.CodigoCliente = findInvoiceStr(m, "codigo_cliente")
		row.Cliente = findInvoiceStr(m, "nome_razaosocial", "nome_cliente", "razao_social")
		row.Servico = findInvoiceStr(m, "login", "login_radius", "login_pppoe")
		row.IDServico = findInvoiceStr(m, "id_cliente_servico")
		row.Plano = invoiceServiceName(m)
		rows = append(rows, row)
		matchCount++
		matchValue += val
	}
	for _, fc := range byForma {
		fc.Valor = round2(fc.Valor)
		formas = append(formas, *fc)
	}
	sort.Slice(formas, func(i, j int) bool {
		if formas[i].Count != formas[j].Count {
			return formas[i].Count > formas[j].Count
		}
		return formas[i].Nome < formas[j].Nome
	})
	sort.SliceStable(rows, func(i, j int) bool { return parseBRDate(rows[i].Vencimento).Before(parseBRDate(rows[j].Vencimento)) })
	return rows, formas, campos, scanned, found, matchCount, round2(matchValue)
}

// formaMatches: filtro numérico compara o id da forma; texto casa por "contém" ignorando acento, caixa e pontuação.
func formaMatches(id, nome, want string) bool {
	want = strings.TrimSpace(want)
	if want == "" {
		return false
	}
	if _, err := strconv.Atoi(want); err == nil {
		return id == want
	}
	return strings.Contains(alnumKey(nome), alnumKey(want))
}

// alnumKey deixa só letras/dígitos minúsculos sem acento — "Sicoob - API (G2)" e "sicoob api g2" ficam iguais.
func alnumKey(s string) string {
	var b strings.Builder
	for _, r := range normText(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// invoiceFormaCobranca extrai a forma de cobrança de uma fatura. Procura primeiro nas chaves da própria fatura e
// só desce para objetos/listas aninhados (até 3 níveis) quando nada foi achado no nível atual — assim a forma da
// fatura vence a de uma linha de detalhamento.
func invoiceFormaCobranca(m map[string]any) (id, nome string) {
	var walk func(v any, depth int) (string, string)
	walk = func(v any, depth int) (string, string) {
		switch t := v.(type) {
		case map[string]any:
			var fid, fnome string
			for k, val := range t {
				switch normText(strings.ReplaceAll(k, "_", " ")) {
				case "forma cobranca", "formacobranca":
					switch fv := val.(type) {
					case map[string]any:
						fid = firstNonEmpty(fid, pickStr(fv, "id_forma_cobranca", "id"))
						fnome = firstNonEmpty(fnome, pickStr(fv, "descricao", "nome", "name"))
					case nil:
					default:
						// escalar: número = id; texto = descrição
						s := strings.TrimSpace(scalarToString(fv))
						if _, err := strconv.Atoi(s); err == nil {
							fid = firstNonEmpty(fid, s)
						} else {
							fnome = firstNonEmpty(fnome, s)
						}
					}
				case "id forma cobranca":
					fid = firstNonEmpty(fid, strings.TrimSpace(scalarToString(val)))
				case "forma cobranca descricao", "descricao forma cobranca", "forma cobranca nome", "nome forma cobranca":
					fnome = firstNonEmpty(fnome, strings.TrimSpace(scalarToString(val)))
				}
			}
			if fid != "" || fnome != "" {
				return fid, fnome
			}
			if depth >= 3 {
				return "", ""
			}
			keys := make([]string, 0, len(t))
			for k := range t {
				keys = append(keys, k)
			}
			sort.Strings(keys) // determinístico
			for _, k := range keys {
				if a, b := walk(t[k], depth+1); a != "" || b != "" {
					return a, b
				}
			}
		case []any:
			if depth >= 3 {
				return "", ""
			}
			for _, el := range t {
				if a, b := walk(el, depth+1); a != "" || b != "" {
					return a, b
				}
			}
		}
		return "", ""
	}
	return walk(m, 0)
}

// invoiceFieldNames lista as chaves de uma fatura (com um nível de aninhamento: "cliente.nome_razaosocial") —
// diagnóstico para quando a forma de cobrança não vem na resposta.
func invoiceFieldNames(m map[string]any) []string {
	var out []string
	for k, v := range m {
		out = append(out, k)
		if sub, ok := v.(map[string]any); ok {
			for sk := range sub {
				out = append(out, k+"."+sk)
			}
		}
	}
	sort.Strings(out)
	return out
}

// flattenInvoiceSample achata uma fatura em "chave" → valor (objetos como "cliente.nome_razaosocial", listas só pelo
// tamanho), com valores longos truncados. Só para diagnóstico na tela — nunca é gravado.
func flattenInvoiceSample(m map[string]any) map[string]string {
	out := map[string]string{}
	var walk func(prefix string, v any, depth int)
	walk = func(prefix string, v any, depth int) {
		switch t := v.(type) {
		case map[string]any:
			if depth >= 3 {
				return
			}
			for k, val := range t {
				key := k
				if prefix != "" {
					key = prefix + "." + k
				}
				walk(key, val, depth+1)
			}
		case []any:
			out[prefix] = fmt.Sprintf("[lista com %d item(ns)]", len(t))
		case nil:
			out[prefix] = ""
		default:
			s := strings.TrimSpace(scalarToString(t))
			if len(s) > 80 {
				s = s[:80] + "…"
			}
			out[prefix] = s
		}
	}
	walk("", m, 0)
	return out
}

// formatBRDate normaliza uma data da HubSoft (ISO "2026-12-15", "2026-12-15 00:00:00" ou já "15/12/2026") para DD/MM/AAAA.
// Se não for uma data reconhecível devolve o texto original.
func formatBRDate(s string) string {
	t := parseBRDate(s)
	if t.IsZero() {
		return strings.TrimSpace(s)
	}
	return t.Format("02/01/2006")
}

// findInvoiceStr devolve o primeiro valor textual não vazio de qualquer uma das chaves, procurando primeiro na
// própria fatura e depois em objetos/listas aninhados (até 4 níveis). A resposta completa de /financeiro/fatura
// não tem esquema documentado publicamente, então a leitura é deliberadamente tolerante ao lugar onde o dado vem
// (ex.: "cliente.servico.login" ou "servicos[0].login").
func findInvoiceStr(m map[string]any, keys ...string) string {
	var walk func(v any, depth int) string
	walk = func(v any, depth int) string {
		switch t := v.(type) {
		case map[string]any:
			if got := pickStr(t, keys...); got != "" {
				return got
			}
			if depth >= 4 {
				return ""
			}
			names := make([]string, 0, len(t))
			for k := range t {
				names = append(names, k)
			}
			sort.Strings(names)
			for _, k := range names {
				if got := walk(t[k], depth+1); got != "" {
					return got
				}
			}
		case []any:
			if depth >= 4 {
				return ""
			}
			for _, el := range t {
				if got := walk(el, depth+1); got != "" {
					return got
				}
			}
		}
		return ""
	}
	return walk(m, 0)
}

// invoiceServiceName acha o nome do serviço/plano: dentro de um objeto (ou lista) chamado "servico",
// "cliente_servico" ou "servicos", pega "nome"/"descricao".
func invoiceServiceName(m map[string]any) string {
	var walk func(v any, depth int) string
	walk = func(v any, depth int) string {
		switch t := v.(type) {
		case map[string]any:
			for _, k := range []string{"servico", "cliente_servico", "servicos"} {
				switch sv := t[k].(type) {
				case map[string]any:
					if n := pickStr(sv, "nome", "descricao"); n != "" {
						return n
					}
				case []any:
					for _, el := range sv {
						if em, ok := el.(map[string]any); ok {
							if n := pickStr(em, "nome", "descricao"); n != "" {
								return n
							}
						}
					}
				}
			}
			if depth >= 3 {
				return ""
			}
			names := make([]string, 0, len(t))
			for k := range t {
				names = append(names, k)
			}
			sort.Strings(names)
			for _, k := range names {
				if got := walk(t[k], depth+1); got != "" {
					return got
				}
			}
		}
		return ""
	}
	return walk(m, 0)
}

// FormaCobrancaOption é uma forma de cobrança cadastrada na HubSoft (catálogo de Configuração).
type FormaCobrancaOption struct {
	ID   string `json:"id"`
	Nome string `json:"nome"`
	Tipo string `json:"tipo,omitempty"`
}

// ListFormasCobranca devolve as formas de cobrança da conta (somente leitura), para a tela escolher em vez de digitar.
func ListFormasCobranca(ctx context.Context, cfg Config, token string) ([]FormaCobrancaOption, error) {
	code, body, err := FetchCatalog(ctx, cfg, token, "forma_cobranca")
	if err != nil {
		return nil, err
	}
	if code < 200 || code >= 300 {
		return nil, fmt.Errorf("HubSoft respondeu HTTP %d", code)
	}
	var doc map[string]any
	if json.Unmarshal(body, &doc) != nil {
		return nil, fmt.Errorf("resposta inválida da HubSoft")
	}
	out := []FormaCobrancaOption{}
	for _, it := range extractArray(doc, "formas_cobranca", "forma_cobranca", "registros", "data") {
		fm, ok := it.(map[string]any)
		if !ok {
			continue
		}
		o := FormaCobrancaOption{ID: pickStr(fm, "id_forma_cobranca", "id"), Nome: pickStr(fm, "descricao", "nome"), Tipo: pickStr(fm, "tipo_cobranca")}
		if o.ID != "" || o.Nome != "" {
			out = append(out, o)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return alnumKey(out[i].Nome) < alnumKey(out[j].Nome) })
	return out, nil
}
