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

// comMock simula a HubSoft para o fluxo de comodato.
type comMock struct {
	itemStatus   string // prefixo do status do patrimônio 5000
	itemLocal    int
	svcStatus    string
	linked       bool // o patrimônio já está vinculado ao serviço
	posts        []map[string]any
	statusAfter  string // status que o patrimônio passa a ter depois da saída (simula o tipo de movimento)
	failPost     string
	productPatr  bool
	movementList []map[string]any
}

func newComMock() *comMock {
	return &comMock{itemStatus: "estoque", itemLocal: 6, svcStatus: "Serviço Habilitado", statusAfter: "comodato", productPatr: true}
}

func (m *comMock) item() map[string]any {
	st := map[string]any{"estoque": "ESTOQUE", "comodato": "COMODATO", "vendido": "Vendido"}
	it := map[string]any{
		"id_produto_item": 5000, "produto": map[string]any{"id_produto": 10, "nome": "ONU TESTE"}, "local_estoque": map[string]any{"id_local_estoque": m.itemLocal, "descricao": "ALMOXARIFADO MATRIZ"},
		"produto_item_status": map[string]any{"descricao": st[m.itemStatus], "prefixo": m.itemStatus}, "identificador_proprio": "9455", "numero_serie": "SER-1", "mac_address": "AA:BB:CC:00:00:01", "codigo_item": 123,
	}
	if m.itemStatus != "estoque" {
		it["cliente_servico"] = map[string]any{"id_cliente_servico": 777, "cliente": map[string]any{"display": "(1) FULANO"}}
	}
	return it
}

func (m *comMock) handler(t *testing.T) http.Handler {
	const P = "/api/v1/integracao"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		raw, _ := io.ReadAll(r.Body)
		enc := json.NewEncoder(w)
		switch {
		case r.Method == "GET" && r.URL.Path == P+"/cliente":
			q := r.URL.Query()
			if q.Get("busca") == "id_cliente_servico" && q.Get("termo_busca") == "777" || q.Get("busca") == "login_radius" && q.Get("termo_busca") == "fulano" {
				_ = enc.Encode(map[string]any{"status": "success", "clientes": []any{map[string]any{"id_cliente": 1, "codigo_cliente": 100, "nome_razaosocial": "FULANO", "servicos": []any{
					map[string]any{"id_cliente_servico": 777, "login": "fulano", "nome": "200 MB PÓS", "status": m.svcStatus}, map[string]any{"id_cliente_servico": 778, "login": "outro", "nome": "100 MB", "status": "Habilitado"}}}}})
				return
			}
			_ = enc.Encode(map[string]any{"status": "success", "clientes": []any{}})
		case r.Method == "GET" && r.URL.Path == P+"/estoque/produto_item/consultar":
			if r.URL.Query().Get("busca") == "identificador_proprio" && r.URL.Query().Get("termo_busca") == "9455" {
				_ = enc.Encode(map[string]any{"status": "success", "produto": []any{m.item()}})
				return
			}
			_ = enc.Encode(map[string]any{"status": "error", "msg": "Nenhum patrimônio encontrado"})
		case r.Method == "GET" && r.URL.Path == P+"/estoque/produto_item/5000":
			_ = enc.Encode(map[string]any{"status": "success", "produto": m.item()})
		case r.Method == "GET" && r.URL.Path == P+"/estoque/produto/10":
			_ = enc.Encode(map[string]any{"status": "success", "produto": map[string]any{"id_produto": 10, "nome": "ONU TESTE", "controle_patrimonial": m.productPatr}})
		case r.Method == "GET" && r.URL.Path == P+"/estoque/produto_vinculo/cliente_servico/777":
			var prods []any
			if m.linked {
				prods = append(prods, map[string]any{"id_produto": 10, "patrimonios": []any{map[string]any{"id_produto_item": 5000, "produto_item_status": map[string]any{"descricao": "COMODATO"}, "codigo_item": 123}}})
			}
			prods = append(prods, map[string]any{"id_produto": 11, "patrimonios": []any{map[string]any{"id_produto_item": 6000, "produto_item_status": map[string]any{"descricao": "COMODATO"}}}})
			_ = enc.Encode(map[string]any{"status": "success", "produtos_cliente_servico": prods, "paginacao": map[string]any{"pagina_atual": 0, "ultima_pagina": 0}})
		case r.Method == "GET" && r.URL.Path == P+"/estoque/movimento_estoque":
			_ = enc.Encode(map[string]any{"status": "success", "movimentos_estoque": m.movementList, "paginacao": map[string]any{"pagina_atual": 0, "ultima_pagina": 0}})
		case r.Method == "POST" && r.URL.Path == P+"/estoque/movimento_estoque/saida/cliente_servico":
			if m.failPost != "" {
				w.WriteHeader(422)
				_ = enc.Encode(map[string]any{"status": "error", "msg": "Dados inválidos", "errors": []string{m.failPost}})
				return
			}
			var b map[string]any
			_ = json.Unmarshal(raw, &b)
			m.posts = append(m.posts, b)
			m.itemStatus, m.linked = m.statusAfter, true
			_ = enc.Encode(map[string]any{"status": "success", "msg": "Movimento criado com sucesso", "movimento_estoque": map[string]any{"id_movimento_estoque": 4012}})
		default:
			t.Errorf("chamada inesperada: %s %s", r.Method, r.URL.String())
			w.WriteHeader(404)
		}
	})
}

func mov(typeID int, nome, statusPrefix, destino string) map[string]any {
	return map[string]any{"vinculo_destino": map[string]any{"tipo_vinculo": destino}, "tipo_movimento_estoque": map[string]any{
		"id_tipo_movimento_estoque": typeID, "nome": nome, "prefixo": strings.ToLower(nome), "produto_item_status": map[string]any{"prefixo": statusPrefix, "descricao": strings.ToUpper(statusPrefix)}}}
}

func comReq() ComodatoRequest {
	return ComodatoRequest{IDClienteServico: "777", Campo: "identificador_proprio", Valor: "9455", IDTipoMovimento: "4", Observacao: "Comodato — migração IXC"}
}

var comTypes = []MovementType{{ID: "4", Nome: "Comodato", Prefixo: "comodato", StatusPrefixo: "comodato", StatusNome: "COMODATO", Usos: 9}, {ID: "6", Nome: "Venda", StatusPrefixo: "vendido", StatusNome: "Vendido", Usos: 3}}

func TestDiscoverMovementTypes(t *testing.T) {
	m := newComMock()
	m.movementList = []map[string]any{mov(6, "Venda", "vendido", "servico_cliente"), mov(4, "Comodato", "comodato", "servico_cliente"), mov(4, "Comodato", "comodato", "servico_cliente"),
		mov(3, "Compra", "estoque", "local_estoque")} // não é saída para cliente: ignorado
	srv := httptest.NewServer(m.handler(t))
	defer srv.Close()
	got, err := DiscoverMovementTypes(context.Background(), Config{BaseURL: srv.URL}, "tok")
	if err != nil || len(got) != 2 {
		t.Fatalf("tipos: %+v err=%v", got, err)
	}
	if got[0].ID != "4" || !got[0].IsComodato() || got[0].Usos != 2 || got[1].ID != "6" || got[1].IsComodato() {
		t.Errorf("comodato primeiro e contagem: %+v", got)
	}
}

func TestFindServicesEFindItem(t *testing.T) {
	m := newComMock()
	srv := httptest.NewServer(m.handler(t))
	defer srv.Close()
	cfg := Config{BaseURL: srv.URL}
	s1, err := FindServices(context.Background(), cfg, "tok", "777")
	if err != nil || len(s1) != 1 || s1[0].Login != "fulano" || s1[0].Cliente != "FULANO" {
		t.Errorf("por id: %+v err=%v", s1, err)
	}
	s2, _ := FindServices(context.Background(), cfg, "tok", "fulano")
	if len(s2) != 1 || s2[0].IDClienteServico != "777" {
		t.Errorf("por login (só o serviço com o login): %+v", s2)
	}
	if none, _ := FindServices(context.Background(), cfg, "tok", "999"); len(none) != 0 {
		t.Errorf("inexistente: %+v", none)
	}
	it, err := FindItem(context.Background(), cfg, "tok", "identificador_proprio", "9455")
	if err != nil || it.ID != "5000" || it.StatusPrefix != "estoque" {
		t.Errorf("item: %+v err=%v", it, err)
	}
	if _, err := FindItem(context.Background(), cfg, "tok", "numero_serie", "NAO-EXISTE"); err == nil {
		t.Error("deveria dizer que não achou")
	}
	if _, err := FindItem(context.Background(), cfg, "tok", "campo_invalido", "x"); err == nil {
		t.Error("campo inválido")
	}
}

func TestPreviewComodatoBloqueios(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*comMock, *ComodatoRequest)
		want  string
	}{
		{"ok", func(m *comMock, r *ComodatoRequest) {}, ""},
		{"patrimônio fora do estoque", func(m *comMock, r *ComodatoRequest) { m.itemStatus = "comodato" }, "só um patrimônio em ESTOQUE"},
		{"serviço cancelado", func(m *comMock, r *ComodatoRequest) { m.svcStatus = "Cancelado" }, "serviço cancelado"},
		{"tipo que gera venda", func(m *comMock, r *ComodatoRequest) { r.IDTipoMovimento = "6" }, "não «Comodato»"},
		{"tipo desconhecido sem confirmação", func(m *comMock, r *ComodatoRequest) { r.IDTipoMovimento = "99" }, "não foi visto em nenhum movimento"},
		{"local diferente", func(m *comMock, r *ComodatoRequest) { r.IDLocalEstoque = "9" }, "não no id 9"},
		{"produto sem patrimônio", func(m *comMock, r *ComodatoRequest) { m.productPatr = false }, "não tem controle patrimonial"},
		{"serviço inexistente", func(m *comMock, r *ComodatoRequest) { r.IDClienteServico = "999" }, "não existe na HubSoft"},
	}
	for _, c := range cases {
		m := newComMock()
		req := comReq()
		c.setup(m, &req)
		srv := httptest.NewServer(m.handler(t))
		pv := PreviewComodato(context.Background(), Config{BaseURL: srv.URL}, "tok", req, comTypes)
		srv.Close()
		joined := strings.Join(pv.Problems, " | ")
		if c.want == "" {
			if !pv.OK || len(pv.Links) != 1 {
				t.Errorf("%s: deveria liberar e listar o que o serviço já tem: %+v", c.name, pv)
			}
			continue
		}
		if pv.OK || !strings.Contains(joined, c.want) {
			t.Errorf("%s: esperava bloqueio com %q, veio ok=%v %s", c.name, c.want, pv.OK, joined)
		}
	}
	// tipo desconhecido COM confirmação: libera, com aviso
	m := newComMock()
	srv := httptest.NewServer(m.handler(t))
	defer srv.Close()
	req := comReq()
	req.IDTipoMovimento, req.TipoConfirmadoComodato = "99", true
	pv := PreviewComodato(context.Background(), Config{BaseURL: srv.URL}, "tok", req, comTypes)
	if !pv.OK || len(pv.Warnings) == 0 {
		t.Errorf("tipo confirmado manualmente: %+v", pv)
	}
}

func TestApplyComodatoCriaEConfere(t *testing.T) {
	m := newComMock()
	srv := httptest.NewServer(m.handler(t))
	defer srv.Close()
	res := ApplyComodato(context.Background(), Config{BaseURL: srv.URL}, "tok", comReq(), comTypes)
	if !res.OK || res.Action != "created" || res.IDMovimento != "4012" || res.StatusDepois != "COMODATO" {
		t.Fatalf("resultado: %+v", res)
	}
	if len(m.posts) != 1 {
		t.Fatalf("posts: %d", len(m.posts))
	}
	b := m.posts[0]
	prod := b["produtos"].([]any)[0].(map[string]any)
	pat := prod["patrimonios"].([]any)[0].(map[string]any)
	if b["id_cliente_servico"] != float64(777) || b["id_tipo_movimento_estoque"] != float64(4) || b["id_local_estoque"] != float64(6) || prod["id_produto"] != float64(10) ||
		prod["quantidade"] != float64(1) || prod["campo_identificacao_patrimonio"] != "id_produto_item" || pat["id_produto_item"] != float64(5000) || b["observacao"] != "Comodato — migração IXC" {
		t.Errorf("corpo da saída: %+v", b)
	}
}

func TestApplyComodatoBloqueadoNaoEnvia(t *testing.T) {
	m := newComMock()
	m.itemStatus = "comodato"
	srv := httptest.NewServer(m.handler(t))
	defer srv.Close()
	res := ApplyComodato(context.Background(), Config{BaseURL: srv.URL}, "tok", comReq(), comTypes)
	if res.OK || res.Action != "blocked" || len(m.posts) != 0 {
		t.Errorf("bloqueado: %+v posts=%d", res, len(m.posts))
	}
}

func TestApplyComodatoFalhaDaHubsoftEStatusErrado(t *testing.T) {
	m := newComMock()
	m.failPost = "O produto não permite comodato."
	srv := httptest.NewServer(m.handler(t))
	defer srv.Close()
	res := ApplyComodato(context.Background(), Config{BaseURL: srv.URL}, "tok", comReq(), comTypes)
	if res.OK || res.Action != "failed" || !strings.Contains(res.Message, "O produto não permite comodato.") || res.Detail == "" {
		t.Errorf("recusa da HubSoft: %+v", res)
	}
	// tipo informado à mão gerou status Vendido: a conferência acusa
	m2 := newComMock()
	m2.statusAfter = "vendido"
	srv2 := httptest.NewServer(m2.handler(t))
	defer srv2.Close()
	req := comReq()
	req.IDTipoMovimento, req.TipoConfirmadoComodato = "99", true
	res2 := ApplyComodato(context.Background(), Config{BaseURL: srv2.URL}, "tok", req, comTypes)
	if res2.OK || res2.Action != "verify_failed" || !strings.Contains(res2.VerifyMessage, "NÃO gera comodato") || res2.IDMovimento != "4012" {
		t.Errorf("status errado: %+v", res2)
	}
}

func TestComodatoJaVinculadoNaoEErroNemEnvia(t *testing.T) {
	m := newComMock()
	m.itemStatus, m.linked = "comodato", true
	srv := httptest.NewServer(m.handler(t))
	defer srv.Close()
	cfg := Config{BaseURL: srv.URL}
	pv := PreviewComodato(context.Background(), cfg, "tok", comReq(), comTypes)
	if !pv.OK || !pv.AlreadyDone || len(pv.Problems) != 0 {
		t.Fatalf("já vinculado deve ser resultado normal: %+v", pv)
	}
	res := ApplyComodato(context.Background(), cfg, "tok", comReq(), comTypes)
	if !res.OK || res.Action != "already_done" || len(m.posts) != 0 {
		t.Errorf("não pode enviar nada: %+v posts=%d", res, len(m.posts))
	}
}

func TestComodatoLoteLocalizaConfereEEnvia(t *testing.T) {
	m := newComMock()
	srv := httptest.NewServer(m.handler(t))
	defer srv.Close()
	cfg := Config{BaseURL: srv.URL}
	opt := ComodatoBatchOptions{IDTipoMovimento: "4"}
	rows := []ComodatoRow{{Linha: 2, IDClienteServico: "777", IdentificadorProprio: "9455", IDProduto: "10", Cliente: "fulano", Login: "FULANO"}}
	pvs := PreviewComodatoRows(context.Background(), cfg, "tok", rows, opt, comTypes)
	if len(pvs) != 1 || !pvs[0].Preview.OK || pvs[0].Locator != "identificador_proprio" {
		t.Fatalf("preview do lote: %+v", pvs)
	}
	// MAC que não existe na HubSoft (omitido no cadastro) não é divergência quando outro campo achou o patrimônio
	rows[0].MacAddress = "ZTE3Q"
	if pv := PreviewComodatoRows(context.Background(), cfg, "tok", rows, opt, comTypes); !pv[0].Preview.OK {
		t.Errorf("MAC ausente não deveria bloquear: %+v", pv[0].Preview.Problems)
	}
	// divergências bloqueiam
	bad := []ComodatoRow{
		{Linha: 3, IDClienteServico: "777", IdentificadorProprio: "9455", IDProduto: "99"},
		{Linha: 4, IDClienteServico: "777", IdentificadorProprio: "9455", Cliente: "OUTRA PESSOA"},
		{Linha: 5, IDClienteServico: "777", IdentificadorProprio: "9455", NumeroSerie: "NAO-EXISTE"},
		{Linha: 6, IDClienteServico: "777"},
	}
	for i, pv := range PreviewComodatoRows(context.Background(), cfg, "tok", bad, opt, comTypes) {
		if pv.Preview.OK {
			t.Errorf("linha %d deveria bloquear: %+v", bad[i].Linha, pv.Preview)
		}
	}
	if len(m.posts) != 0 {
		t.Fatalf("preview não pode enviar: %d", len(m.posts))
	}
	res := ApplyComodatoRows(context.Background(), cfg, "tok", rows[:1], opt, comTypes)
	if len(res) != 1 || res[0].Result.Action != "created" || len(m.posts) != 1 {
		t.Errorf("apply do lote: %+v posts=%d", res, len(m.posts))
	}
	// segunda rodada: já está no serviço → already_done, sem novo POST
	res = ApplyComodatoRows(context.Background(), cfg, "tok", rows[:1], opt, comTypes)
	if res[0].Result.Action != "already_done" || len(m.posts) != 1 {
		t.Errorf("reenvio deveria ser neutro: %+v posts=%d", res[0].Result, len(m.posts))
	}
}
