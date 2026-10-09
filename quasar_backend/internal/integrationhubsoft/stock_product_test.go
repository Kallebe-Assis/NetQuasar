package integrationhubsoft

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

var brands = map[string]string{"5": "TP-LINK", "21": "TERA PRO"}

func okRow() StockProductRow {
	return StockProductRow{
		Line: 2, Codigo: "748", Nome: "ROTEADOR TESTE AX1500", IDCategoria: "7", IDMarca: "5", UnidadeMedida: "UN", ControlePatrimonial: "sim",
		ValorCompra: "163,98", ValorVenda: "199,90",
		IncluirNotaFiscal: "sim", PermiteVendaCliente: "sim", PermiteComodatoCliente: "sim", PermiteVinculoPop: "sim",
		PermiteVinculoProjetoMapeamento: "sim", PermiteVinculoUsuario: "sim", PermiteVinculoComposicao: "não",
	}
}

func TestValidateStockProductRows(t *testing.T) {
	good := okRow()
	bad := okRow()
	bad.Line, bad.Codigo, bad.Nome = 3, "749", "OUTRO"
	bad.IDMarca, bad.ValorVenda, bad.PermiteVinculoPop, bad.UnidadeMedida = "abc", "x", "", "1"
	dupNome := okRow()
	dupNome.Line, dupNome.Codigo = 4, "750" // mesmo nome da linha 2
	semConfig := okRow()
	semConfig.Line, semConfig.Codigo, semConfig.Nome, semConfig.IncluirNotaFiscal = 5, "751", "SEM NF", ""
	cat := StockCatalogSets{Categoria: map[string]bool{"7": true}, Marca: map[string]bool{"5": true}}
	foraCat := okRow()
	foraCat.Line, foraCat.Codigo, foraCat.Nome, foraCat.IDCategoria = 6, "752", "CAT 99", "99"

	res := ValidateStockProductRows([]StockProductRow{good, bad, dupNome, semConfig, foraCat}, cat)
	if res.Valid != 1 || res.Invalid != 4 {
		t.Fatalf("válidas=%d inválidas=%d: %+v", res.Valid, res.Invalid, res.Rows)
	}
	joined := func(i int) string { return strings.Join(res.Rows[i].Problems, " | ") }
	for _, want := range []string{"id_marca", "valor_venda", "permite_vinculo_pop", "unidade_medida"} {
		if !strings.Contains(joined(1), want) {
			t.Errorf("linha 3 deveria citar %q: %s", want, joined(1))
		}
	}
	if !strings.Contains(joined(2), "nome repetido") {
		t.Errorf("nome repetido: %s", joined(2))
	}
	if !strings.Contains(joined(3), "incluir_nota_fiscal") {
		t.Errorf("config obrigatória: %s", joined(3))
	}
	if !strings.Contains(joined(4), "id_categoria 99 não existe") {
		t.Errorf("catálogo: %s", joined(4))
	}
}

// stockMock simula os endpoints de produto da HubSoft e registra o que recebeu.
type stockMock struct {
	products  []map[string]any
	created   []map[string]any
	configs   map[string]map[string]any
	failPost  string // mensagem de erro a devolver no POST
	failPut   bool
	returnCfg bool
	// ignoraMarca: simula a HubSoft que descarta a marca enviada e grava a marca «API».
	ignoraMarca bool
	// marcaViaRaiz: o PUT só aceita trocar a marca quando o corpo traz id_produto_marca na raiz.
	marcaViaRaiz bool
	puts         []map[string]any
}

func jsonStr(v any) string { return strings.TrimSpace(scalarToString(v)) }

func (m *stockMock) handler(t *testing.T) http.Handler {
	const base = "/api/v1/integracao/estoque/produto"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		raw, _ := io.ReadAll(r.Body)
		switch {
		case r.Method == "GET" && r.URL.Path == base:
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "produtos": m.products, "paginacao": map[string]any{"pagina_atual": 0, "ultima_pagina": 0, "total_registros": len(m.products)}})
		case r.Method == "POST" && r.URL.Path == base:
			if m.failPost != "" {
				w.WriteHeader(422)
				_ = json.NewEncoder(w).Encode(map[string]any{"status": "error", "msg": "Dados inválidos", "errors": []string{m.failPost}})
				return
			}
			var b map[string]any
			_ = json.Unmarshal(raw, &b)
			b["id_produto"] = 5000 + len(m.created)
			m.created = append(m.created, b)
			marca, _ := b["produto_marca"].(map[string]any)
			marcaGravada := map[string]any{"id_produto_marca": marca["id_produto_marca"], "nome": marca["nome"]}
			if m.ignoraMarca {
				marcaGravada = map[string]any{"id_produto_marca": 49, "nome": "API"}
			}
			cat, _ := b["produto_categoria"].(map[string]any)
			m.products = append(m.products, map[string]any{"id_produto": b["id_produto"], "nome": b["nome"], "codigo": b["codigo"], "produto_marca": marcaGravada,
				"produto_categoria": []any{map[string]any{"id_categoria": cat["id_categoria"], "descricao": "X"}}})
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "msg": "Produto cadastrado com sucesso", "produto": map[string]any{"id_produto": b["id_produto"], "nome": b["nome"]}})
		case r.Method == "PUT" && strings.HasPrefix(r.URL.Path, base+"/"):
			if m.failPut {
				w.WriteHeader(500)
				_, _ = w.Write([]byte(`{"status":"error","msg":"Erro interno ao salvar configuração"}`))
				return
			}
			var put map[string]any
			_ = json.Unmarshal(raw, &put)
			m.puts = append(m.puts, put)
			if _, isCfg := put["produto_configuracao"]; !isCfg {
				if idm, ok := put["id_produto_marca"]; ok && m.marcaViaRaiz {
					id := strings.TrimPrefix(r.URL.Path, base+"/")
					for _, p := range m.products {
						if jsonStr(p["id_produto"]) == id {
							p["produto_marca"] = map[string]any{"id_produto_marca": idm, "nome": "TROCADA"}
						}
					}
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "msg": "Produto atualizado com sucesso"})
				return
			}
			var b struct {
				Cfg map[string]any `json:"produto_configuracao"`
			}
			_ = json.Unmarshal(raw, &b)
			if m.configs == nil {
				m.configs = map[string]map[string]any{}
			}
			m.configs[strings.TrimPrefix(r.URL.Path, base+"/")] = b.Cfg
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "msg": "Produto atualizado com sucesso"})
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, base+"/"):
			id := strings.TrimPrefix(r.URL.Path, base+"/")
			for _, p := range m.products {
				if jsonStr(p["id_produto"]) == id {
					prod := map[string]any{"id_produto": p["id_produto"], "nome": p["nome"], "codigo": p["codigo"], "produto_marca": p["produto_marca"], "produto_categoria": p["produto_categoria"]}
					if m.returnCfg {
						prod["produto_configuracao"] = m.configs[id]
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "produto": prod})
					return
				}
			}
			w.WriteHeader(404)
		default:
			t.Errorf("chamada inesperada: %s %s", r.Method, r.URL.String())
			w.WriteHeader(404)
		}
	})
}

func TestApplyStockProductCriaConfiguraEConfere(t *testing.T) {
	m := &stockMock{products: []map[string]any{{"id_produto": 1, "nome": "PRODUTO ANTIGO", "codigo": "1"}}, returnCfg: true}
	srv := httptest.NewServer(m.handler(t))
	defer srv.Close()
	cfg := Config{BaseURL: srv.URL}
	ix, err := NewStockProductIndex(context.Background(), cfg, "tok")
	if err != nil {
		t.Fatal(err)
	}
	res := ApplyStockProduct(context.Background(), cfg, "tok", okRow(), ix, brands)
	if !res.OK || !res.Created || res.Action != StockActionCreated || res.IDProduto != "5000" {
		t.Fatalf("resultado: %+v", res)
	}
	if res.ConfigOK == nil || !*res.ConfigOK || res.Verified == nil || !*res.Verified {
		t.Errorf("config/conferência: %+v", res)
	}
	c := m.created[0] // corpo do POST
	if c["nome"] != "ROTEADOR TESTE AX1500" || c["unidade_medida"] != "UN" || c["controle_patrimonial"] != true || c["valor_compra"] != 163.98 || c["codigo"] != "748" {
		t.Errorf("corpo do POST: %+v", c)
	}
	if v, present := c["estoque_minimo"]; !present || v != float64(0) { // a HubSoft exige o campo; vazio no CSV = 0
		t.Errorf("estoque_minimo: %v (presente=%v)", v, present)
	}
	marca, _ := c["produto_marca"].(map[string]any)
	if marca["id_produto_marca"] != float64(5) || marca["nome"] != "TP-LINK" {
		t.Errorf("a marca precisa ir com id E nome exato: %+v", marca)
	}
	cat, _ := c["produto_categoria"].(map[string]any)
	if cat["id_categoria"] != float64(7) || cat["id_protudo_categoria"] != float64(7) {
		t.Errorf("categoria: %+v", cat)
	}
	cfgSent := m.configs["5000"] // configuração enviada
	if cfgSent["incluir_nota_fiscal"] != true || cfgSent["permite_vinculo_composicao"] != false || len(cfgSent) != 7 {
		t.Errorf("configuração enviada: %+v", cfgSent)
	}
}

func TestApplyStockProductJaExisteNaoCria(t *testing.T) {
	m := &stockMock{products: []map[string]any{{"id_produto": 77, "nome": "Roteador Teste AX-1500", "codigo": ""}, {"id_produto": 78, "nome": "OUTRO", "codigo": "748"}}}
	srv := httptest.NewServer(m.handler(t))
	defer srv.Close()
	cfg := Config{BaseURL: srv.URL}
	ix, _ := NewStockProductIndex(context.Background(), cfg, "tok")

	porNome := okRow()
	porNome.Codigo = "999"
	r1 := ApplyStockProduct(context.Background(), cfg, "tok", porNome, ix, brands)
	if !r1.OK || r1.Action != StockActionAlreadyExists || r1.IDProduto != "77" || r1.Created {
		t.Errorf("por nome: %+v", r1)
	}
	r2 := ApplyStockProduct(context.Background(), cfg, "tok", okRow(), ix, brands) // código 748 já existe (id 78)
	if !r2.OK || r2.Action != StockActionAlreadyExists || r2.IDProduto != "78" {
		t.Errorf("por código: %+v", r2)
	}
	if len(m.created) != 0 {
		t.Errorf("não podia criar nada: %+v", m.created)
	}
	pre, err := PreflightStockProducts(context.Background(), cfg, "tok", []StockProductRow{porNome, okRow(), {Line: 9, Codigo: "1", Nome: "PRODUTO NOVO"}})
	if err != nil || pre[0].Status != "ja_existe_nome" || pre[1].Status != "ja_existe_codigo" || pre[2].Status != "novo" {
		t.Errorf("preflight: %+v err=%v", pre, err)
	}
}

func TestApplyStockProductErrosVisiveis(t *testing.T) {
	// POST recusado: mensagem da HubSoft + corpo bruto no detalhe
	m := &stockMock{failPost: "O campo produto_marca é inválido."}
	srv := httptest.NewServer(m.handler(t))
	defer srv.Close()
	cfg := Config{BaseURL: srv.URL}
	ix, _ := NewStockProductIndex(context.Background(), cfg, "tok")
	res := ApplyStockProduct(context.Background(), cfg, "tok", okRow(), ix, brands)
	if res.OK || res.Action != StockActionFailed || !strings.Contains(res.Message, "O campo produto_marca é inválido.") || !strings.Contains(res.Detail, "Dados inválidos") {
		t.Errorf("POST recusado: %+v", res)
	}

	// PUT de configuração falha: o produto FOI criado e isso precisa estar claro
	m2 := &stockMock{failPut: true}
	srv2 := httptest.NewServer(m2.handler(t))
	defer srv2.Close()
	cfg2 := Config{BaseURL: srv2.URL}
	ix2, _ := NewStockProductIndex(context.Background(), cfg2, "tok")
	r2 := ApplyStockProduct(context.Background(), cfg2, "tok", okRow(), ix2, brands)
	if r2.OK || r2.Action != StockActionConfigFailed || !r2.Created || r2.IDProduto != "5000" || !strings.Contains(r2.Message, "CRIADO (id 5000)") || r2.ConfigOK == nil || *r2.ConfigOK {
		t.Errorf("config falhou: %+v", r2)
	}

	// linha malformada nunca é enviada
	bad := okRow()
	bad.ValorVenda = ""
	r3 := ApplyStockProduct(context.Background(), cfg2, "tok", bad, ix2, brands)
	if !r3.Rejected || r3.OK || len(m2.created) != 1 {
		t.Errorf("rejeitada: %+v (criados=%d)", r3, len(m2.created))
	}
}

func TestVerifyStockProductDetectaDiferencas(t *testing.T) {
	ok, msg := verifyStockProduct([]byte(`{"status":"success","produto":{"nome":"ROTEADOR TESTE AX1500","codigo":"748","produto_configuracao":{"incluir_nota_fiscal":false,"permite_vinculo_pop":true}}}`), "", okRow())
	if ok || !strings.Contains(msg, "incluir_nota_fiscal=false (esperado true)") {
		t.Errorf("deveria apontar a diferença: ok=%v msg=%s", ok, msg)
	}
	ok2, msg2 := verifyStockProduct([]byte(`{"status":"success","produto":{"nome":"OUTRO NOME","codigo":"748"}}`), "", okRow())
	if ok2 || !strings.Contains(msg2, "nome gravado") {
		t.Errorf("nome diferente: ok=%v msg=%s", ok2, msg2)
	}
	ok3, msg3 := verifyStockProduct([]byte(`{"status":"success","produto":{"nome":"ROTEADOR TESTE AX1500","codigo":"748"}}`), "", okRow())
	if !ok3 || !strings.Contains(msg3, "não devolve a configuração") {
		t.Errorf("sem configuração na resposta: ok=%v msg=%s", ok3, msg3)
	}
}

func TestApplyStockProductMarcaIgnoradaEMarcaDesconhecida(t *testing.T) {
	// a HubSoft descarta a marca e grava «API»: a conferência TEM que acusar
	m := &stockMock{ignoraMarca: true}
	srv := httptest.NewServer(m.handler(t))
	defer srv.Close()
	cfg := Config{BaseURL: srv.URL}
	ix, _ := NewStockProductIndex(context.Background(), cfg, "tok")
	res := ApplyStockProduct(context.Background(), cfg, "tok", okRow(), ix, brands)
	if res.OK || res.Verified == nil || *res.Verified || !strings.Contains(res.VerifyMessage, "MARCA gravada «API» (id 49) em vez do id 5") {
		t.Errorf("marca trocada precisa reprovar a conferência: %+v", res)
	}
	// marca que não está no catálogo: nada é enviado (evita a HubSoft criar uma marca nova)
	m2 := &stockMock{}
	srv2 := httptest.NewServer(m2.handler(t))
	defer srv2.Close()
	cfg2 := Config{BaseURL: srv2.URL}
	ix2, _ := NewStockProductIndex(context.Background(), cfg2, "tok")
	r := okRow()
	r.IDMarca = "999"
	res2 := ApplyStockProduct(context.Background(), cfg2, "tok", r, ix2, brands)
	if !res2.Rejected || res2.OK || len(m2.created) != 0 || !strings.Contains(res2.Message, "marca id 999") {
		t.Errorf("marca desconhecida: %+v (criados=%d)", res2, len(m2.created))
	}
}

func TestLoadStockBrandNames(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","produto_marcas":[{"id_produto_marca":21,"nome":"TERA PRO"},{"id_produto_marca":5,"nome":"TP-LINK"}]}`))
	}))
	defer srv.Close()
	got, err := LoadStockBrandNames(context.Background(), Config{BaseURL: srv.URL}, "tok")
	if err != nil || got["21"] != "TERA PRO" || got["5"] != "TP-LINK" {
		t.Errorf("marcas: %+v err=%v", got, err)
	}
}

func TestRepararMarcaDepoisDeCriar(t *testing.T) {
	// a HubSoft grava «API» no POST, mas aceita trocar a marca pelo PUT com id_produto_marca na raiz
	m := &stockMock{ignoraMarca: true, marcaViaRaiz: true}
	srv := httptest.NewServer(m.handler(t))
	defer srv.Close()
	cfg := Config{BaseURL: srv.URL}
	ix, _ := NewStockProductIndex(context.Background(), cfg, "tok")
	res := ApplyStockProduct(context.Background(), cfg, "tok", okRow(), ix, brands)
	if !res.OK || res.Action != StockActionCreated || !strings.Contains(res.BrandRepair, "id_produto_marca na raiz") {
		t.Errorf("deveria criar e corrigir a marca: %+v", res)
	}
	// e se NENHUMA forma funcionar: não esconde — reprova e lista as tentativas
	m2 := &stockMock{ignoraMarca: true}
	srv2 := httptest.NewServer(m2.handler(t))
	defer srv2.Close()
	cfg2 := Config{BaseURL: srv2.URL}
	ix2, _ := NewStockProductIndex(context.Background(), cfg2, "tok")
	r2 := ApplyStockProduct(context.Background(), cfg2, "tok", okRow(), ix2, brands)
	if r2.OK || !strings.Contains(r2.BrandRepair, "não foi possível corrigir a marca") || !strings.Contains(r2.VerifyMessage, "MARCA gravada") {
		t.Errorf("sem correção possível: %+v", r2)
	}
}

func TestRepararMarcaDeProdutoExistente(t *testing.T) {
	existente := func() []map[string]any {
		return []map[string]any{{"id_produto": 131, "nome": "TESTE", "codigo": "748", "produto_marca": map[string]any{"id_produto_marca": 49, "nome": "API"}}}
	}
	row := okRow()
	row.RepairBrand = true

	m := &stockMock{products: existente(), marcaViaRaiz: true}
	srv := httptest.NewServer(m.handler(t))
	defer srv.Close()
	cfg := Config{BaseURL: srv.URL}
	ix, _ := NewStockProductIndex(context.Background(), cfg, "tok")
	res := ApplyStockProduct(context.Background(), cfg, "tok", row, ix, brands)
	if !res.OK || res.Action != StockActionBrandRepaired || len(m.created) != 0 {
		t.Errorf("reparo: %+v", res)
	}
	// marca já correta: não faz nenhuma alteração
	ok := []map[string]any{{"id_produto": 131, "nome": "TESTE", "codigo": "748", "produto_marca": map[string]any{"id_produto_marca": 5, "nome": "TP-LINK"}}}
	m3 := &stockMock{products: ok}
	srv3 := httptest.NewServer(m3.handler(t))
	defer srv3.Close()
	cfg3 := Config{BaseURL: srv3.URL}
	ix3, _ := NewStockProductIndex(context.Background(), cfg3, "tok")
	r3 := ApplyStockProduct(context.Background(), cfg3, "tok", row, ix3, brands)
	if !r3.OK || r3.Action != StockActionAlreadyExists || len(m3.puts) != 0 {
		t.Errorf("marca correta não pode gerar PUT: %+v (puts=%d)", r3, len(m3.puts))
	}
	// reparo só vale para o mesmo CÓDIGO — produto encontrado só pelo nome nunca é alterado
	porNome := []map[string]any{{"id_produto": 7, "nome": "ROTEADOR TESTE AX1500", "codigo": "", "produto_marca": map[string]any{"id_produto_marca": 49, "nome": "API"}}}
	m4 := &stockMock{products: porNome, marcaViaRaiz: true}
	srv4 := httptest.NewServer(m4.handler(t))
	defer srv4.Close()
	cfg4 := Config{BaseURL: srv4.URL}
	ix4, _ := NewStockProductIndex(context.Background(), cfg4, "tok")
	r4 := ApplyStockProduct(context.Background(), cfg4, "tok", row, ix4, brands)
	if r4.Action != StockActionAlreadyExists || len(m4.puts) != 0 {
		t.Errorf("por nome não altera: %+v", r4)
	}
}
