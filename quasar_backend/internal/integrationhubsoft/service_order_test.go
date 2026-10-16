package integrationhubsoft

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type soMock struct {
	mu    sync.Mutex
	posts []map[string]any
	fail  bool
	// atend — atendimentos já existentes do cliente 77 (assunto → status)
	atend map[string]string
}

func (m *soMock) handler(t *testing.T) http.Handler {
	const P = "/api/v1/integracao"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		raw, _ := io.ReadAll(r.Body)
		enc := json.NewEncoder(w)
		m.mu.Lock()
		defer m.mu.Unlock()
		switch {
		case r.Method == "GET" && r.URL.Path == P+"/cliente":
			q := r.URL.Query()
			svc := func(id, login, st string) map[string]any {
				return map[string]any{"id_cliente_servico": id, "login": login, "nome": "200MB", "status": st}
			}
			switch q.Get("busca") {
			case "id_cliente_servico":
				if q.Get("termo_busca") == "11" {
					_ = enc.Encode(map[string]any{"clientes": []any{map[string]any{"id_cliente": 5, "codigo_cliente": 77, "nome_razaosocial": "MARIA CELIA BELGA DA SILVA", "servicos": []any{svc("11", "c2-6mariacelia1", "Serviço Habilitado")}}}})
					return
				}
			case "nome_razaosocial":
				if strings.Contains(strings.ToUpper(q.Get("termo_busca")), "MARIA CELIA") {
					_ = enc.Encode(map[string]any{"clientes": []any{map[string]any{"id_cliente": 5, "codigo_cliente": 77, "nome_razaosocial": "MARIA CELIA BELGA DA SILVA", "servicos": []any{svc("11", "c2-6mariacelia1", "Serviço Habilitado"), svc("12", "c2-7outro", "Cancelado")}}}})
					return
				}
			}
			_ = enc.Encode(map[string]any{"status": "error", "msg": "Nenhum cliente encontrado"})
		case r.Method == "GET" && r.URL.Path == P+"/cliente/atendimento":
			var items []any
			for sub, st := range m.atend {
				items = append(items, map[string]any{"id_atendimento": 1, "protocolo": "2026OLD", "tipo_atendimento": sub, "status": st, "data_cadastro": "01/10/2026"})
			}
			_ = enc.Encode(map[string]any{"status": "success", "atendimentos": items})
		case r.Method == "POST" && r.URL.Path == P+"/atendimento":
			if m.fail {
				w.WriteHeader(422)
				_ = enc.Encode(map[string]any{"status": "error", "msg": "Dados inválidos", "errors": []string{"tipo de O.S. não aceito"}})
				return
			}
			var b map[string]any
			_ = json.Unmarshal(raw, &b)
			m.posts = append(m.posts, b)
			_ = enc.Encode(map[string]any{"status": "success", "atendimento": map[string]any{"id_atendimento": 900 + len(m.posts), "protocolo": "2026101500001", "cliente": map[string]any{"codigo_cliente": 77},
				"ordens_servico": []any{map[string]any{"id_ordem_servico": 500, "numero_ordem_servico": "321", "tipo": "RETIRADA"}}}})
		default:
			t.Errorf("chamada inesperada: %s %s", r.Method, r.URL.String())
			w.WriteHeader(404)
		}
	})
}

var soCat = ServiceOrderCatalogs{
	TiposAtendimento: []SOCatalogItem{{ID: "63", Nome: "SUPORTE"}, {ID: "64", Nome: "SUPORTE TÉCNICO NIVEL 2"}},
	TiposOS:          []SOCatalogItem{{ID: "134", Nome: "RETIRADA DE EQUIPAMENTO", Extra: "cliente_servico"}, {ID: "205", Nome: "MANUTENÇÃO CTO", Extra: "pop"}},
	Tecnicos:         []SOCatalogItem{{ID: "7", Nome: "MARIO LUIZ DE ARAUJO PINHO"}},
	Disponibilidades: []SOCatalogItem{{ID: "1", Nome: "Manhã"}, {ID: "2", Nome: "Tarde"}},
}

func TestResolveCatalogENormPhone(t *testing.T) {
	if it, ok, _ := resolveCatalog(soCat.TiposAtendimento, "suporte"); !ok || it.ID != "63" {
		t.Errorf("igual vence o «contém»: %+v", it)
	}
	if _, ok, why := resolveCatalog(soCat.TiposOS, "retirada"); !ok {
		t.Errorf("único «contém» deve resolver: %s", why)
	}
	if _, ok, _ := resolveCatalog(soCat.TiposAtendimento, "inexistente"); ok {
		t.Error("nome inexistente não pode resolver")
	}
	if got := normPhone("(22) 99225-1761"); got != "22992251761" {
		t.Errorf("telefone: %q", got)
	}
	for _, bad := range []string{"(00) 00000-0000", "", "123"} {
		if normPhone(bad) != "" {
			t.Errorf("%q deveria ser inválido", bad)
		}
	}
}

func TestPreviewEApplyServiceOrder(t *testing.T) {
	m := &soMock{}
	srv := httptest.NewServer(m.handler(t))
	defer srv.Close()
	cfg := Config{BaseURL: srv.URL}
	opt := ServiceOrderOptions{TipoAtendimento: "SUPORTE", AbrirOS: true, TipoOS: "RETIRADA DE EQUIPAMENTO", Tecnicos: "MARIO LUIZ DE ARAUJO PINHO", StatusOS: "aguardando agendamento", TelefonePadrao: "(32) 99999-0000", Done: map[string]string{"214000": "atendimento 1"}}
	row := ServiceOrderRow{Linha: 2, Referencia: "214289", Cliente: "MARIA CELIA BELGA DA SILVA", LoginIXC: "mariacelia1", Descricao: "RETIRADA", Telefone: "(00) 00000-0000"}
	pv := PreviewServiceOrder(context.Background(), cfg, "tok", row, opt, soCat)
	if !pv.OK || pv.Service == nil || pv.Service.IDClienteServico != "11" {
		t.Fatalf("o login do IXC deve desempatar o serviço: %+v", pv)
	}
	po := pv.Body["parametros_ordem_servico"].(map[string]any)
	if po["id_tipo_ordem_servico"] != 134 || po["status"] != "aguardando_agendamento" || pv.Body["telefone"] != "32999990000" || len(pv.Warnings) == 0 {
		t.Errorf("corpo: %+v warnings %v", pv.Body, pv.Warnings)
	}
	// duplicada (já aberta antes) não envia
	dup := PreviewServiceOrder(context.Background(), cfg, "tok", ServiceOrderRow{Linha: 3, Referencia: "214000"}, opt, soCat)
	if !dup.AlreadyDone {
		t.Errorf("referência já feita deve ser reconhecida")
	}
	// tipo de O.S. de outro destino é recusado
	bad := PreviewServiceOrder(context.Background(), cfg, "tok", ServiceOrderRow{Linha: 4, IDClienteServico: "11", Descricao: "x", Nome: "A", Telefone: "22992251761", TipoOS: "MANUTENÇÃO CTO"}, opt, soCat)
	if bad.OK || !strings.Contains(strings.Join(bad.Problems, " "), "destino") {
		t.Errorf("tipo de destino pop deve ser recusado: %+v", bad.Problems)
	}
	res, halted := ApplyServiceOrderRows(context.Background(), cfg, "tok", []ServiceOrderRow{row, row}, opt, soCat)
	if halted || res[0].Action != "created" || res[0].IDAtendimento != "901" || len(res[0].OrdensServico) != 1 {
		t.Fatalf("envio: %+v", res[0])
	}
	if res[1].Action != "blocked" || len(m.posts) != 1 {
		t.Errorf("a mesma referência no lote só envia uma vez: %+v posts=%d", res[1], len(m.posts))
	}
}

func TestApplyServiceOrderParaAposTresFalhas(t *testing.T) {
	m := &soMock{fail: true}
	srv := httptest.NewServer(m.handler(t))
	defer srv.Close()
	opt := ServiceOrderOptions{AbrirOS: false, TelefonePadrao: "22992251761"}
	var rows []ServiceOrderRow
	for i := 0; i < 5; i++ {
		rows = append(rows, ServiceOrderRow{Linha: i + 2, IDClienteServico: "11", Descricao: "x", Nome: "A", Referencia: string(rune('a' + i))})
	}
	res, halted := ApplyServiceOrderRows(context.Background(), Config{BaseURL: srv.URL}, "tok", rows, opt, soCat)
	if !halted || res[0].Action != "failed" || !strings.Contains(res[0].Message, "tipo de O.S. não aceito") || res[3].Action != "skipped" {
		t.Errorf("deve parar após 3 recusas mostrando a mensagem da HubSoft: %+v", res)
	}
}

func TestOpenDuplicateRetirada(t *testing.T) {
	m := &soMock{atend: map[string]string{"RETIRADA DE EQUIPAMENTO": "Aguardando Análise", "SEM CONEXÃO": "Resolvido"}}
	srv := httptest.NewServer(m.handler(t))
	defer srv.Close()
	cfg := Config{BaseURL: srv.URL}
	opt := ServiceOrderOptions{TipoAtendimento: "SUPORTE", AbrirOS: true, TipoOS: "RETIRADA DE EQUIPAMENTO", TelefonePadrao: "22992251761"}
	row := ServiceOrderRow{Linha: 2, IDClienteServico: "11", Descricao: "retirar", Nome: "MARIA"}
	pv := PreviewServiceOrder(context.Background(), cfg, "tok", row, opt, soCat)
	if pv.OK || !strings.Contains(strings.Join(pv.Problems, " "), "já tem o atendimento aberto") {
		t.Fatalf("atendimento de RETIRADA aberto deve bloquear: %+v", pv)
	}
	opt.PermitirDuplicadas = true
	if pv2 := PreviewServiceOrder(context.Background(), cfg, "tok", row, opt, soCat); !pv2.OK || len(pv2.Warnings) == 0 {
		t.Errorf("com permitir_duplicadas vira aviso: %+v", pv2)
	}
	// resolvido não conta
	m.atend = map[string]string{"RETIRADA DE EQUIPAMENTO": "Resolvido"}
	opt.PermitirDuplicadas = false
	if pv3 := PreviewServiceOrder(context.Background(), cfg, "tok", row, opt, soCat); !pv3.OK {
		t.Errorf("atendimento resolvido não é duplicidade: %+v", pv3.Problems)
	}
}
