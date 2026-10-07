package integrationhubsoft

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInvoiceFormaCobranca(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		id, nome string
	}{
		{"objeto", `{"forma_cobranca":{"id_forma_cobranca":19,"descricao":"Sicoob - API (G2)"}}`, "19", "Sicoob - API (G2)"},
		{"campos planos", `{"id_forma_cobranca":14,"forma_cobranca_descricao":"BB API G2"}`, "14", "BB API G2"},
		{"texto", `{"forma_cobranca":"Caixa"}`, "", "Caixa"},
		{"numero", `{"forma_cobranca":7}`, "7", ""},
		{"aninhado", `{"cobranca":{"forma_cobranca":{"id":19,"nome":"Sicoob - API (G2)"}}}`, "19", "Sicoob - API (G2)"},
		{"a da fatura vence a do detalhamento", `{"forma_cobranca":{"id_forma_cobranca":19,"descricao":"G2"},"detalhamento":[{"forma_cobranca":{"id_forma_cobranca":7,"descricao":"Caixa"}}]}`, "19", "G2"},
		{"em lista", `{"detalhamento":[{"forma_cobranca":{"id_forma_cobranca":7,"descricao":"Caixa"}}]}`, "7", "Caixa"},
		{"ausente", `{"valor":"10,00","cliente":{"nome_razaosocial":"X"}}`, "", ""},
	}
	for _, c := range cases {
		var m map[string]any
		if err := json.Unmarshal([]byte(c.in), &m); err != nil {
			t.Fatal(err)
		}
		id, nome := invoiceFormaCobranca(m)
		if id != c.id || nome != c.nome {
			t.Errorf("%s: id=%q nome=%q, esperado %q/%q", c.name, id, nome, c.id, c.nome)
		}
	}
}

func TestFormaMatches(t *testing.T) {
	if !formaMatches("19", "Sicoob - API (G2)", "19") || formaMatches("14", "BB API G2", "19") {
		t.Error("filtro por id")
	}
	if !formaMatches("", "Sicoob - API (G2)", " sicoob  api (g2) ") || formaMatches("", "Sicoob - API (Origem)", "sicoob - api (g2)") {
		t.Error("filtro por nome")
	}
	if formaMatches("19", "x", "") {
		t.Error("filtro vazio não casa com nada")
	}
}

func invoicePage(items ...map[string]any) map[string]any {
	return map[string]any{"status": "success", "faturas": items, "paginacao": map[string]any{"pagina_atual": 0, "ultima_pagina": 0, "total_registros": len(items)}}
}

func invoice(id, forma, venc, valor string, extra map[string]any) map[string]any {
	m := map[string]any{
		"id_fatura": id, "valor": valor, "data_vencimento": venc, "link": "http://boleto/" + id, "nosso_numero": "n" + id,
		"forma_cobranca": map[string]any{"id_forma_cobranca": forma, "descricao": map[string]string{"19": "Sicoob - API (G2)", "6": "BB API Origem"}[forma]},
		"cliente":        map[string]any{"id_cliente": 5, "codigo_cliente": 100, "nome_razaosocial": "FULANO " + id, "servico": map[string]any{"login": "lg" + id}},
	}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

func TestBuildInvoicesByMethod(t *testing.T) {
	var gotQuery map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = map[string]string{}
		for k := range r.URL.Query() {
			gotQuery[k] = r.URL.Query().Get(k)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(invoicePage(
			invoice("1", "19", "20/10/2026", "99,90", nil),
			invoice("2", "6", "10/10/2026", "89,90", nil),
			invoice("3", "19", "05/09/2026", "79,90", nil),                                            // mais antiga
			invoice("4", "19", "01/10/2026", "59,90", map[string]any{"data_pagamento": "02/10/2026"}), // paga: fora
			invoice("5", "19", "01/10/2026", "59,90", map[string]any{"cancelado": true}),              // cancelada: fora
		))
	}))
	defer srv.Close()

	rep := BuildInvoicesByMethod(context.Background(), Config{BaseURL: srv.URL}, "tok", "2023-01-01", "2027-01-01", "Sicoob - API (G2)")
	if !rep.OK || rep.Message != "" {
		t.Fatalf("relatório: %+v", rep)
	}
	if gotQuery["apenas_em_aberto"] != "sim" || gotQuery["tipo_data"] != "data_vencimento" {
		t.Errorf("parâmetros: %v", gotQuery)
	}
	if _, ok := gotQuery["tipo_resultado"]; ok {
		t.Error("não pode enviar tipo_resultado (o modo simplificado não traz a forma de cobrança)")
	}
	if rep.Scanned != 3 || rep.FormaFound != 3 || rep.MatchCount != 2 || rep.MatchValue != 179.8 {
		t.Errorf("contagens: scanned=%d found=%d match=%d valor=%v", rep.Scanned, rep.FormaFound, rep.MatchCount, rep.MatchValue)
	}
	if len(rep.Rows) != 2 || rep.Rows[0].IDFatura != "3" || rep.Rows[1].IDFatura != "1" {
		t.Fatalf("linhas (mais antigas primeiro): %+v", rep.Rows)
	}
	if r := rep.Rows[0]; r.Cliente != "FULANO 3" || r.Servico != "lg3" || r.CodigoCliente != "100" || r.Status != "overdue" || r.Link != "http://boleto/3" {
		t.Errorf("linha 0: %+v", r)
	}
	if len(rep.Formas) != 2 || rep.Formas[0].Nome != "Sicoob - API (G2)" || rep.Formas[0].Count != 2 {
		t.Errorf("formas: %+v", rep.Formas)
	}
	if len(rep.CamposFatura) == 0 {
		t.Error("campos da fatura deveriam vir para diagnóstico")
	}
}

func TestBuildInvoicesByMethodSemCampo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(invoicePage(map[string]any{"id_fatura": 1, "valor": "10,00", "data_vencimento": "20/10/2026"}))
	}))
	defer srv.Close()
	rep := BuildInvoicesByMethod(context.Background(), Config{BaseURL: srv.URL}, "tok", "2023-01-01", "2027-01-01", "Sicoob - API (G2)")
	if !rep.OK || rep.FormaFound != 0 || rep.MatchCount != 0 || rep.Message == "" || len(rep.CamposFatura) == 0 {
		t.Errorf("sem o campo deve avisar e listar os campos da fatura: %+v", rep)
	}
}
