package integrationhubsoft

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// itemMock simula os endpoints de estoque da HubSoft usados pela importação de patrimônios.
type itemMock struct {
	products []map[string]any
	items    []map[string]any // patrimônios existentes
	nextID   int
	entries  []map[string]any // corpos recebidos em movimento_estoque/entrada
	puts     map[string]map[string]any
	// comportamentos
	failPut         bool
	rejectMAC       bool   // o PUT recusa qualquer mac_address (formato inválido para a HubSoft)
	consultFail     bool   // consultar devolve erro 500
	consultNotFound string // mensagem de «não encontrado» (status error) em vez de lista vazia
	entryReturns    int    // quantos patrimônios a entrada devolve (padrão 1)
}

func newItemMock() *itemMock {
	return &itemMock{
		nextID: 9000,
		puts:   map[string]map[string]any{},
		products: []map[string]any{
			{"id_produto": 10, "nome": "ONU TESTE", "codigo": "700", "controle_patrimonial": true},
			{"id_produto": 11, "nome": "ROTEADOR TESTE", "codigo": "701", "controle_patrimonial": true},
			{"id_produto": 12, "nome": "PREGO", "codigo": "702", "controle_patrimonial": false},
		},
	}
}

func (m *itemMock) item(id int, prod int, ident, serie, mac string) map[string]any {
	return map[string]any{
		"id_produto_item": id, "produto": map[string]any{"id_produto": prod, "nome": "PROD " + strconv.Itoa(prod)},
		"local_estoque":         map[string]any{"id_local_estoque": 6, "descricao": "ALMOXARIFADO MATRIZ"},
		"produto_item_status":   map[string]any{"descricao": "ESTOQUE", "prefixo": "estoque"},
		"identificador_proprio": nz(ident), "numero_serie": nz(serie), "mac_address": nz(mac), "codigo_item": id + 5,
	}
}

func nz(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (m *itemMock) find(id string) map[string]any {
	for _, it := range m.items {
		if jsonStr(it["id_produto_item"]) == id {
			return it
		}
	}
	return nil
}

func (m *itemMock) handler(t *testing.T) http.Handler {
	const P = "/api/v1/integracao/estoque"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		raw, _ := io.ReadAll(r.Body)
		enc := json.NewEncoder(w)
		switch {
		case r.Method == "GET" && r.URL.Path == P+"/produto":
			_ = enc.Encode(map[string]any{"status": "success", "produtos": m.products, "paginacao": map[string]any{"pagina_atual": 0, "ultima_pagina": 0}})
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, P+"/produto/"):
			id := strings.TrimPrefix(r.URL.Path, P+"/produto/")
			for _, p := range m.products {
				if jsonStr(p["id_produto"]) == id {
					_ = enc.Encode(map[string]any{"status": "success", "produto": p})
					return
				}
			}
			_ = enc.Encode(map[string]any{"status": "error", "msg": "Produto não encontrado"})
		case r.Method == "GET" && r.URL.Path == P+"/produto_item":
			var list []map[string]any
			for _, it := range m.items {
				if jsonStr(it["produto"].(map[string]any)["id_produto"]) == r.URL.Query().Get("id_produto") {
					list = append(list, it)
				}
			}
			_ = enc.Encode(map[string]any{"status": "success", "produto_itens": list, "paginacao": map[string]any{"pagina_atual": 0, "ultima_pagina": 0}})
		case r.Method == "GET" && r.URL.Path == P+"/produto_item/consultar":
			if m.consultFail {
				w.WriteHeader(500)
				_, _ = w.Write([]byte(`{"status":"error","msg":"Erro interno"}`))
				return
			}
			field, term := r.URL.Query().Get("busca"), r.URL.Query().Get("termo_busca")
			var list []map[string]any
			for _, it := range m.items {
				if v, _ := it[field].(string); v != "" && strings.Contains(strings.ToLower(v), strings.ToLower(term)) { // busca por trecho, como a HubSoft pode fazer
					list = append(list, it)
				}
			}
			if len(list) == 0 && m.consultNotFound != "" {
				_ = enc.Encode(map[string]any{"status": "error", "msg": m.consultNotFound})
				return
			}
			_ = enc.Encode(map[string]any{"status": "success", "msg": "Patrimônio consultado com sucesso!", "produto": list})
		case r.Method == "POST" && r.URL.Path == P+"/movimento_estoque/entrada":
			var b map[string]any
			_ = json.Unmarshal(raw, &b)
			m.entries = append(m.entries, b)
			prodID := int(b["produtos"].([]any)[0].(map[string]any)["produto"].(map[string]any)["id_produto"].(float64))
			n := m.entryReturns
			if n == 0 {
				n = 1
			}
			var pats []any
			for i := 0; i < n; i++ {
				m.nextID++
				it := m.item(m.nextID, prodID, "", "", "")
				m.items = append(m.items, it)
				pats = append(pats, it)
			}
			_ = enc.Encode(map[string]any{"status": "success", "msg": "Movimento criado com sucesso", "movimento_estoque": map[string]any{"produtos": []any{map[string]any{"patrimonios": pats}}}})
		case r.Method == "PUT" && strings.HasPrefix(r.URL.Path, P+"/produto_item/"):
			id := strings.TrimPrefix(r.URL.Path, P+"/produto_item/")
			if m.failPut {
				w.WriteHeader(422)
				_, _ = w.Write([]byte(`{"status":"error","msg":"Dados inválidos","errors":["O número de série já está em uso."]}`))
				return
			}
			var b map[string]any
			_ = json.Unmarshal(raw, &b)
			if _, hasMAC := b["mac_address"]; hasMAC && m.rejectMAC {
				w.WriteHeader(422)
				_, _ = w.Write([]byte(`{"status":"error","msg":"Dados inválidos","errors":["O campo mac_address é inválido."]}`))
				return
			}
			m.puts[id] = b
			if it := m.find(id); it != nil {
				for k, v := range b {
					it[k] = v
				}
			}
			_ = enc.Encode(map[string]any{"status": "success", "msg": "Edição realizado com sucesso"})
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, P+"/produto_item/"):
			if it := m.find(strings.TrimPrefix(r.URL.Path, P+"/produto_item/")); it != nil {
				_ = enc.Encode(map[string]any{"status": "success", "produto": it})
				return
			}
			_ = enc.Encode(map[string]any{"status": "error", "msg": "Item de produto não encontrado"})
		default:
			t.Errorf("chamada inesperada: %s %s", r.Method, r.URL.String())
			w.WriteHeader(404)
		}
	})
}

func itemRow() StockItemRow {
	return StockItemRow{Line: 2, IDProduto: "10", IdentificadorProprio: "WSJU9", NumeroSerie: "PH941011528001438", MacAddress: "d8380df4d480", Observacoes: "IXC cód 9691", IDLocalEstoque: "6"}
}

var itemProds = map[string]StockItemProduct{"10": {Nome: "ONU TESTE", Patrimonio: true}, "11": {Nome: "ROTEADOR TESTE", Patrimonio: true}, "12": {Nome: "PREGO", Patrimonio: false}}

func TestNormalizeMAC(t *testing.T) {
	for in, want := range map[string]string{
		"d8380df4d480": "D8:38:0D:F4:D4:80", "D8-38-0D-F4-D4-80": "D8:38:0D:F4:D4:80", "d838.0df4.d480": "D8:38:0D:F4:D4:80", "": "",
		// texto que não é MAC (padrão do equipamento no IXC): gravado EXATAMENTE como está
		"ZTE3QJNMCM36439": "ZTE3QJNMCM36439", "ITBSE8D17A23": "ITBSE8D17A23", "D8380DF4D48": "D8380DF4D48",
	} {
		if got, ok := NormalizeMAC(in); !ok || got != want {
			t.Errorf("NormalizeMAC(%q) = %q ok=%v (esperado %q)", in, got, ok, want)
		}
	}
	for _, bad := range []string{"linha1" + string(rune(10)) + "linha2", strings.Repeat("A", 81)} {
		if _, ok := NormalizeMAC(bad); ok {
			t.Errorf("%q deveria ser recusado", bad)
		}
	}
	if macKey("ZTE3QJNMCM36439") != macKey("zte3qjnmcm36439") || macKey("D8:38:0D:F4:D4:80") != macKey("d8380df4d480") {
		t.Error("macKey")
	}
}

func TestValidateStockItemRows(t *testing.T) {
	a := itemRow()
	b := itemRow()
	b.Line, b.IdentificadorProprio = 3, "OUTRO" // mesma série e MAC da linha 2
	c := itemRow()
	c.Line, c.IdentificadorProprio, c.NumeroSerie, c.MacAddress, c.IDProduto = 4, "", "", "XYZ", "12" // sem identificador, produto sem patrimônio ("XYZ" no MAC é aceito como está)
	d := itemRow()
	d.Line, d.IdentificadorProprio, d.NumeroSerie, d.MacAddress, d.IDProduto, d.IDLocalEstoque = 5, "D1", "S-OK", "", "999", ""
	res := ValidateStockItemRows([]StockItemRow{a, b, c, d}, itemProds)
	if res.Valid != 0 || res.Invalid != 4 {
		t.Fatalf("esperava 0 válidas: %+v", res)
	}
	j := func(i int) string { return strings.Join(res.Rows[i].Problems, " | ") }
	if !strings.Contains(j(0), "numero_serie repetido no arquivo (também nas linhas 3)") || !strings.Contains(j(0), "mac_address repetido") {
		t.Errorf("linha 2: %s", j(0))
	}
	for _, w := range []string{"identificador_proprio é obrigatório", "não tem controle patrimonial"} {
		if !strings.Contains(j(2), w) {
			t.Errorf("linha 4 deveria citar %q: %s", w, j(2))
		}
	}
	if !strings.Contains(j(3), "id_produto 999 não existe") || !strings.Contains(j(3), "id_local_estoque") {
		t.Errorf("linha 5: %s", j(3))
	}
	// linha boa
	ok := ValidateStockItemRows([]StockItemRow{a}, itemProds)
	if ok.Valid != 1 {
		t.Errorf("linha boa: %+v", ok)
	}
}

func TestPreflightStockItemsClassifica(t *testing.T) {
	m := newItemMock()
	m.items = []map[string]any{
		m.item(1, 10, "JA-IMPORTADO", "SER-A", "AA:BB:CC:00:00:01"),
		m.item(2, 10, "OUTRO-ID", "SER-DUP", "AA:BB:CC:00:00:02"),
		m.item(3, 11, "ID-DE-ROTEADOR", "", ""),
		m.item(4, 10, "", "", ""), // sem identificação
	}
	srv := httptest.NewServer(m.handler(t))
	defer srv.Close()
	cfg := Config{BaseURL: srv.URL}
	mk := func(line int, prod, ident, serie, mac string) StockItemRow {
		return StockItemRow{Line: line, IDProduto: prod, IdentificadorProprio: ident, NumeroSerie: serie, MacAddress: mac, IDLocalEstoque: "6"}
	}
	rows := []StockItemRow{
		mk(2, "10", "NOVO-1", "SER-NOVA", "AA:BB:CC:00:00:99"),
		mk(3, "10", "JA-IMPORTADO", "SER-A", "aabbcc000001"),                                                  // mesmo patrimônio
		mk(4, "10", "ja-importado", "SER-OUTRA", ""),                                                          // mesmo identificador, série diferente
		mk(5, "10", "NOVO-2", "ser-dup", ""),                                                                  // série já existe
		mk(6, "10", "NOVO-3", "", "AA-BB-CC-00-00-02"),                                                        // MAC já existe
		mk(7, "10", "ID-DE-ROTEADOR", "", ""),                                                                 // identificador de outro produto
		mk(8, "12", "PREGO-1", "", ""),                                                                        // produto sem controle patrimonial
		{Line: 9, IDProduto: "10", IdentificadorProprio: "RETOMADO", IDProdutoItem: "4", IDLocalEstoque: "6"}, // retomar o item 4
		{Line: 10, IDProduto: "11", IdentificadorProprio: "RET-ERRADO", IDProdutoItem: "4", IDLocalEstoque: "6"},
	}
	pre, sum, err := PreflightStockItems(context.Background(), cfg, "tok", rows)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{StockItemNovo, StockItemJaExiste, StockItemJaExisteDivergente, StockItemConflitoSerie, StockItemConflitoMAC, StockItemConflitoIdent, "erro_produto", StockItemRetomar, StockItemRetomarInvalido}
	for i, w := range want {
		if pre[i].Status != w {
			t.Errorf("linha %d: %s (esperado %s) — %s", pre[i].Line, pre[i].Status, w, pre[i].Message)
		}
	}
	if sum.ProdutosPatrimoniais != 2 || sum.PatrimoniosNaHubsoft != 4 || sum.SemIdentificacao != 1 {
		t.Errorf("resumo: %+v", sum)
	}
	if len(m.entries) != 0 {
		t.Error("a conferência não pode criar nada")
	}
}

func TestApplyStockItemCriaIdentificaEConfere(t *testing.T) {
	m := newItemMock()
	srv := httptest.NewServer(m.handler(t))
	defer srv.Close()
	cfg := Config{BaseURL: srv.URL}
	res := ApplyStockItem(context.Background(), cfg, "tok", itemRow(), nil)
	if !res.OK || !res.Created || res.Action != StockItemActionCreated || res.IDProdutoItem != "9001" {
		t.Fatalf("resultado: %+v", res)
	}
	if len(m.entries) != 1 {
		t.Fatalf("entradas: %d", len(m.entries))
	}
	e := m.entries[0]
	prod := e["produtos"].([]any)[0].(map[string]any)
	if e["id_local_estoque"] != float64(6) || prod["quantidade"] != float64(1) || prod["produto"].(map[string]any)["id_produto"] != float64(10) {
		t.Errorf("corpo da entrada: %+v", e)
	}
	put := m.puts["9001"]
	if put["identificador_proprio"] != "WSJU9" || put["numero_serie"] != "PH941011528001438" || put["mac_address"] != "D8:38:0D:F4:D4:80" || put["observacoes"] != "IXC cód 9691" {
		t.Errorf("PUT dos identificadores: %+v", put)
	}
	if res.IdentifyOK == nil || !*res.IdentifyOK || res.Verified == nil || !*res.Verified {
		t.Errorf("etapas: %+v", res)
	}
	// sem série/MAC: só o identificador vai no PUT
	m2 := newItemMock()
	srv2 := httptest.NewServer(m2.handler(t))
	defer srv2.Close()
	r2 := itemRow()
	r2.NumeroSerie, r2.MacAddress, r2.Observacoes = "", "", ""
	res2 := ApplyStockItem(context.Background(), Config{BaseURL: srv2.URL}, "tok", r2, itemProds)
	if !res2.OK || len(m2.puts["9001"]) != 1 {
		t.Errorf("só identificador: %+v put=%+v", res2, m2.puts["9001"])
	}
}

func TestApplyStockItemDuplicadoNaoCria(t *testing.T) {
	m := newItemMock()
	m.items = []map[string]any{m.item(1, 10, "JA-IMPORTADO", "SER-A", "AA:BB:CC:00:00:01"), m.item(2, 11, "X", "SER-DUP", "")}
	srv := httptest.NewServer(m.handler(t))
	defer srv.Close()
	cfg := Config{BaseURL: srv.URL}
	// série já existe (em outro produto) → pendência
	r := itemRow()
	r.NumeroSerie = "ser-dup"
	res := ApplyStockItem(context.Background(), cfg, "tok", r, itemProds)
	if res.OK || !res.Pending || res.Action != StockItemActionConflict || !strings.Contains(res.Message, "número de série") {
		t.Errorf("série duplicada: %+v", res)
	}
	// já importado antes (mesmo identificador, mesmos dados) → OK sem criar
	r2 := StockItemRow{Line: 3, IDProduto: "10", IdentificadorProprio: "ja-importado", NumeroSerie: "SER-A", MacAddress: "AABBCC000001", IDLocalEstoque: "6"}
	res2 := ApplyStockItem(context.Background(), cfg, "tok", r2, itemProds)
	if !res2.OK || res2.Action != StockItemActionAlreadyExists || res2.Created {
		t.Errorf("já existe: %+v", res2)
	}
	if len(m.entries) != 0 {
		t.Errorf("nada podia ser criado: %+v", m.entries)
	}
}

func TestApplyStockItemFalhaNaConsultaNaoCria(t *testing.T) {
	m := newItemMock()
	m.consultFail = true
	srv := httptest.NewServer(m.handler(t))
	defer srv.Close()
	res := ApplyStockItem(context.Background(), Config{BaseURL: srv.URL}, "tok", itemRow(), itemProds)
	if res.OK || res.Action != StockItemActionFailed || len(m.entries) != 0 || !strings.Contains(res.Message, "nada foi criado") {
		t.Errorf("erro de consulta nunca pode virar «não existe»: %+v (entradas=%d)", res, len(m.entries))
	}
	// mensagem de «não encontrado» da HubSoft é lida como inexistente
	m2 := newItemMock()
	m2.consultNotFound = "Nenhum patrimônio encontrado"
	srv2 := httptest.NewServer(m2.handler(t))
	defer srv2.Close()
	res2 := ApplyStockItem(context.Background(), Config{BaseURL: srv2.URL}, "tok", itemRow(), itemProds)
	if !res2.OK || !res2.Created {
		t.Errorf("«não encontrado» deveria permitir criar: %+v", res2)
	}
}

func TestApplyStockItemFalhaAoIdentificarEDepoisRetoma(t *testing.T) {
	m := newItemMock()
	m.failPut = true
	srv := httptest.NewServer(m.handler(t))
	defer srv.Close()
	cfg := Config{BaseURL: srv.URL}
	res := ApplyStockItem(context.Background(), cfg, "tok", itemRow(), itemProds)
	if res.OK || res.Action != StockItemActionIdentifyFailed || !res.Created || res.IDProdutoItem != "9001" ||
		!strings.Contains(res.Message, "JÁ CRIADO") || !strings.Contains(res.Message, "id_produto_item=9001") || !strings.Contains(res.IdentifyMsg, "O número de série já está em uso") {
		t.Fatalf("falha ao identificar: %+v", res)
	}
	// agora a HubSoft aceita; a linha é reenviada com o id do patrimônio já criado: NÃO faz nova entrada
	m.failPut = false
	r := itemRow()
	r.IDProdutoItem = "9001"
	res2 := ApplyStockItem(context.Background(), cfg, "tok", r, itemProds)
	if !res2.OK || res2.Created || res2.Action != StockItemActionResumed || len(m.entries) != 1 {
		t.Errorf("retomada: %+v (entradas=%d)", res2, len(m.entries))
	}
}

func TestApplyStockItemEntradaComQuantidadeInesperada(t *testing.T) {
	m := newItemMock()
	m.entryReturns = 2
	srv := httptest.NewServer(m.handler(t))
	defer srv.Close()
	res := ApplyStockItem(context.Background(), Config{BaseURL: srv.URL}, "tok", itemRow(), itemProds)
	if res.OK || res.Action != StockItemActionFailed || !strings.Contains(res.Message, "devolveu 2 patrimônio(s) em vez de 1") || len(m.puts) != 0 {
		t.Errorf("quantidade inesperada: %+v", res)
	}
}

func TestVerifyStockItemDetectaDiferencas(t *testing.T) {
	body := []byte(`{"status":"success","produto":{"id_produto_item":1,"produto":{"id_produto":10},"local_estoque":{"id_local_estoque":9,"descricao":"OUTRO"},"produto_item_status":{"descricao":"ESTOQUE","prefixo":"estoque"},"identificador_proprio":"WSJU9","numero_serie":"ERRADA","mac_address":"D8:38:0D:F4:D4:80"}}`)
	ok, msg := verifyStockItem(body, "", itemRow())
	if ok || !strings.Contains(msg, "local «OUTRO» (id 9) em vez do id 6") || !strings.Contains(msg, "série gravada «ERRADA»") {
		t.Errorf("diferenças: ok=%v %s", ok, msg)
	}
}

func TestApplyStockItemProdutoSemControlePatrimonialNaoFazEntrada(t *testing.T) {
	m := newItemMock()
	srv := httptest.NewServer(m.handler(t))
	defer srv.Close()
	r := itemRow()
	r.IDProduto = "12" // PREGO — sem controle patrimonial
	res := ApplyStockItem(context.Background(), Config{BaseURL: srv.URL}, "tok", r, nil)
	if res.OK || !res.Rejected || len(m.entries) != 0 || !strings.Contains(res.Message, "controle patrimonial") {
		t.Errorf("produto agrupado não pode receber entrada: %+v (entradas=%d)", res, len(m.entries))
	}
}

func TestIdentificadorAlternativoEvitaDuplicar(t *testing.T) {
	// a equipe já cadastrou o patrimônio à mão com o Nº patrimônio do IXC como identificador
	m := newItemMock()
	m.items = []map[string]any{m.item(1, 10, "NP-777", "", ""), m.item(2, 11, "NP-888", "", "")}
	srv := httptest.NewServer(m.handler(t))
	defer srv.Close()
	cfg := Config{BaseURL: srv.URL}
	row := StockItemRow{Line: 2, IDProduto: "10", IdentificadorProprio: "9455", IdentificadorAlternativo: "np-777", IDLocalEstoque: "6"}
	res := ApplyStockItem(context.Background(), cfg, "tok", row, nil)
	if !res.OK || res.Action != StockItemActionAlreadyExists || res.Created || len(m.entries) != 0 || !strings.Contains(res.Message, "identificador antigo") {
		t.Errorf("deveria reconhecer o cadastro antigo: %+v (entradas=%d)", res, len(m.entries))
	}
	// mesmo identificador antigo, mas em OUTRO produto → conflito, não cria
	row2 := StockItemRow{Line: 3, IDProduto: "10", IdentificadorProprio: "9456", IdentificadorAlternativo: "NP-888", IDLocalEstoque: "6"}
	res2 := ApplyStockItem(context.Background(), cfg, "tok", row2, nil)
	if res2.OK || !res2.Pending || res2.Action != StockItemActionConflict {
		t.Errorf("identificador antigo de outro produto: %+v", res2)
	}
	// sem cadastro antigo: cria normalmente com o novo identificador
	row3 := StockItemRow{Line: 4, IDProduto: "10", IdentificadorProprio: "9457", IdentificadorAlternativo: "NP-999", IDLocalEstoque: "6"}
	res3 := ApplyStockItem(context.Background(), cfg, "tok", row3, nil)
	if !res3.OK || !res3.Created || m.puts[res3.IDProdutoItem]["identificador_proprio"] != "9457" {
		t.Errorf("novo: %+v", res3)
	}
	// o preflight também enxerga
	pre, _, err := PreflightStockItems(context.Background(), cfg, "tok", []StockItemRow{row})
	if err != nil || pre[0].Status != StockItemJaExiste {
		t.Errorf("preflight: %+v err=%v", pre, err)
	}
}

func TestMACForaDoPadraoGravadoComoEsta(t *testing.T) {
	m := newItemMock()
	srv := httptest.NewServer(m.handler(t))
	defer srv.Close()
	r := itemRow()
	r.MacAddress = "ZTE3QJNMCM36439"
	res := ApplyStockItem(context.Background(), Config{BaseURL: srv.URL}, "tok", r, nil)
	if !res.OK || m.puts["9001"]["mac_address"] != "ZTE3QJNMCM36439" {
		t.Errorf("MAC fora do padrão deve ir como está: %+v put=%+v", res, m.puts["9001"])
	}
	// e a duplicidade também vale para esse padrão
	r2 := itemRow()
	r2.IdentificadorProprio, r2.NumeroSerie, r2.MacAddress = "OUTRO", "", "zte3qjnmcm36439"
	res2 := ApplyStockItem(context.Background(), Config{BaseURL: srv.URL}, "tok", r2, nil)
	if res2.OK || res2.Action != StockItemActionConflict || !strings.Contains(res2.Message, "MAC") {
		t.Errorf("duplicidade do MAC fora do padrão: %+v", res2)
	}
}

func TestHubsoftRecusaOMACSalvaSemMAC(t *testing.T) {
	m := newItemMock()
	m.rejectMAC = true
	srv := httptest.NewServer(m.handler(t))
	defer srv.Close()
	r := itemRow()
	r.MacAddress = "ZTE3QJNMCM36439"
	res := ApplyStockItem(context.Background(), Config{BaseURL: srv.URL}, "tok", r, nil)
	if !res.OK || !res.Created || !res.MacOmitted || res.Action != StockItemActionCreated {
		t.Fatalf("deveria criar o patrimônio sem o MAC: %+v", res)
	}
	put := m.puts["9001"]
	if _, has := put["mac_address"]; has {
		t.Errorf("o PUT final não pode levar o MAC: %+v", put)
	}
	obs, _ := put["observacoes"].(string)
	if !strings.Contains(obs, "MAC não aceito pela HubSoft") || !strings.Contains(obs, "ZTE3QJNMCM36439") || !strings.Contains(obs, "IXC cód 9691") {
		t.Errorf("o MAC original e a observação anterior precisam ficar nas observações: %q", obs)
	}
	if put["numero_serie"] != "PH941011528001438" || put["identificador_proprio"] != "WSJU9" {
		t.Errorf("o resto dos dados continua: %+v", put)
	}
	if res.Verified == nil || !*res.Verified || !strings.Contains(res.Message, "SEM o MAC") || len(m.entries) != 1 {
		t.Errorf("conferência/mensagem/entradas: %+v", res)
	}
	// só erro de MAC é tratado assim: outro erro continua sendo erro (nunca mascarado)
	m2 := newItemMock()
	m2.failPut = true
	srv2 := httptest.NewServer(m2.handler(t))
	defer srv2.Close()
	res2 := ApplyStockItem(context.Background(), Config{BaseURL: srv2.URL}, "tok", r, nil)
	if res2.OK || res2.MacOmitted || res2.Action != StockItemActionIdentifyFailed {
		t.Errorf("erro que não é de MAC: %+v", res2)
	}
}
