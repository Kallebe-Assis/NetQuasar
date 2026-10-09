package integrationhubsoft

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCheckStockProducts(t *testing.T) {
	m := &stockMock{products: []map[string]any{
		{"id_produto": 1, "nome": "ROTEADOR OK", "codigo": "10", "controle_patrimonial": true, "epi": false, "valor_compra": 100.0, "valor_venda": 140.0,
			"produto_categoria": []any{map[string]any{"id_categoria": 7}}, "produto_marca": map[string]any{"id_produto_marca": 5, "nome": "TP-LINK"},
			"unidade_medida": map[string]any{"abreviacao": "UN"}},
		{"id_produto": 2, "nome": "ONU DIVERGENTE", "codigo": "11", "controle_patrimonial": true, "valor_compra": 90.0, "valor_venda": 90.0,
			"produto_categoria": []any{map[string]any{"id_categoria": 8}}, "produto_marca": map[string]any{"id_produto_marca": 49, "nome": "API"},
			"unidade_medida": map[string]any{"abreviacao": "UN"}},
		{"id_produto": 3, "nome": "REPETIDO", "codigo": "12"}, {"id_produto": 4, "nome": "REPETIDO", "codigo": ""},
	}}
	srv := httptest.NewServer(m.handler(t))
	defer srv.Close()
	base := okRow()
	ok := base
	ok.Line, ok.Codigo, ok.Nome, ok.ValorCompra, ok.ValorVenda = 2, "10", "ROTEADOR OK", "100,00", "140,00"
	div := base
	div.Line, div.Codigo, div.Nome, div.IDCategoria, div.IDMarca, div.ValorVenda = 3, "11", "ONU DIVERGENTE", "8", "21", "126,00"
	nao := base
	nao.Line, nao.Codigo, nao.Nome = 4, "99", "NÃO EXISTE"
	dup := base
	dup.Line, dup.Codigo, dup.Nome = 5, "", "REPETIDO"
	res, err := CheckStockProducts(context.Background(), Config{BaseURL: srv.URL}, "tok", []StockProductRow{ok, div, nao, dup})
	if err != nil {
		t.Fatal(err)
	}
	if res[0].Status != StockCheckOK || len(res[0].Diffs) != 0 {
		t.Errorf("ok: %+v", res[0])
	}
	if res[1].Status != StockCheckDiverge {
		t.Fatalf("divergente: %+v", res[1])
	}
	campos := []string{}
	for _, d := range res[1].Diffs {
		campos = append(campos, d.Campo)
	}
	for _, want := range []string{"marca (id)", "valor de venda"} {
		if !strings.Contains(strings.Join(campos, ","), want) {
			t.Errorf("deveria apontar %q: %v", want, res[1].Diffs)
		}
	}
	if res[2].Status != StockCheckNaoAchado || res[3].Status != StockCheckDuplicado {
		t.Errorf("não achado/duplicado: %+v %+v", res[2], res[3])
	}
	if res[0].Aviso == "" {
		t.Error("deve avisar que a configuração não é verificável")
	}
}

func TestCheckStockItems(t *testing.T) {
	m := newItemMock()
	m.items = []map[string]any{
		m.item(1, 10, "9455", "SER-1", "AA:BB:CC:00:00:01"),
		m.item(2, 10, "9456", "SER-2", "AA:BB:CC:00:00:02"),
		m.item(3, 11, "OUTRO", "SER-3", ""),
	}
	m.items[0]["observacoes"] = "IXC cód 9455"
	srv := httptest.NewServer(m.handler(t))
	defer srv.Close()
	cfg := Config{BaseURL: srv.URL}
	mk := func(line int, prod, ident, serie, mac, obs string) StockItemRow {
		return StockItemRow{Line: line, IDProduto: prod, IdentificadorProprio: ident, NumeroSerie: serie, MacAddress: mac, Observacoes: obs, IDLocalEstoque: "6"}
	}
	rows := []StockItemRow{
		mk(2, "10", "9455", "SER-1", "aabbcc000001", "IXC cód 9455"), // tudo igual
		mk(3, "11", "9456", "SER-X", "", ""),                         // produto e série diferentes
		mk(4, "10", "9999", "", "", ""),                              // não existe
		mk(5, "10", "9455", "SER-2", "", ""),                         // identificador de um patrimônio e série de outro
	}
	res, sum, err := CheckStockItems(context.Background(), cfg, "tok", rows)
	if err != nil {
		t.Fatal(err)
	}
	if res[0].Status != StockCheckOK || res[0].ID != "1" {
		t.Errorf("ok: %+v", res[0])
	}
	if res[1].Status != StockCheckDiverge {
		t.Fatalf("divergente: %+v", res[1])
	}
	cs := ""
	for _, d := range res[1].Diffs {
		cs += d.Campo + ","
	}
	if !strings.Contains(cs, "produto (id)") || !strings.Contains(cs, "número de série") {
		t.Errorf("diferenças: %s", cs)
	}
	if res[2].Status != StockCheckNaoAchado {
		t.Errorf("não achado: %+v", res[2])
	}
	if sum.PatrimoniosNaHubsoft != 3 {
		t.Errorf("resumo: %+v", sum)
	}
	if len(m.entries) != 0 || len(m.puts) != 0 {
		t.Error("a conferência não pode alterar nada")
	}
}
