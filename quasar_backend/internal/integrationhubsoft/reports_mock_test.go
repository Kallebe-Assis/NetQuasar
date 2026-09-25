package integrationhubsoft

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Servidor de mentira que imita os endpoints da HubSoft usados pelos relatórios e pela edição em massa.
type mockHubsoft struct {
	todos      []map[string]any               // resposta de /cliente/todos (todas as páginas numa só)
	byServiceQ func(id string) map[string]any // resposta de /cliente?busca=id_cliente_servico
	byLoginQ   func(login string) []map[string]any
	put        func(id string, body map[string]string) (int, map[string]any)
	puts       []string
}

func (m *mockHubsoft) server(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/cliente/todos"):
			page := r.URL.Query().Get("pagina")
			list := m.todos
			if page != "0" && page != "" {
				list = nil
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success", "clientes": list,
				"paginacao": map[string]any{"pagina_atual": 0, "ultima_pagina": 0, "total_registros": len(m.todos)},
			})
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/integracao/cliente"):
			q := r.URL.Query()
			var clientes []map[string]any
			switch q.Get("busca") {
			case "id_cliente_servico":
				if c := m.byServiceQ(q.Get("termo_busca")); c != nil {
					clientes = []map[string]any{c}
				}
			case "login_radius":
				clientes = m.byLoginQ(q.Get("termo_busca"))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "clientes": clientes})
		case r.Method == "PUT" && strings.Contains(r.URL.Path, "/cliente_servico/editar/"):
			id := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			m.puts = append(m.puts, id+"="+body["data_venda"])
			code, resp := m.put(id, body)
			w.WriteHeader(code)
			_ = json.NewEncoder(w).Encode(resp)
		default:
			t.Errorf("chamada inesperada: %s %s", r.Method, r.URL.String())
			w.WriteHeader(404)
		}
	}))
}

func client(id, code, name string, services ...map[string]any) map[string]any {
	arr := make([]any, len(services))
	for i, s := range services {
		arr[i] = s
	}
	return map[string]any{"id_cliente": id, "codigo_cliente": code, "nome_razaosocial": name, "servicos": arr}
}

func svc(id, login, status, prefix string, extra map[string]any) map[string]any {
	m := map[string]any{"id_cliente_servico": id, "login": login, "status": status, "status_prefixo": prefix, "nome": "PLANO 200"}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

func day(offset int) string { return time.Now().AddDate(0, 0, offset).Format("2006-01-02") }

func TestBlockedReport_usesLastSuspensionDate(t *testing.T) {
	// Suspenso em 01/09 (data_atualizacao antiga), liberado e bloqueado de novo há 3 dias.
	m := &mockHubsoft{todos: []map[string]any{
		client("1", "10", "Maria", svc("100", "maria", "Suspenso por Débito", "suspenso_debito", map[string]any{
			"data_ultima_suspensao": day(-3) + " 09:00:27", "data_atualizacao": day(-40) + " 10:00:00",
		})),
		client("2", "11", "João", svc("101", "joao", "Suspenso por Débito", "suspenso_debito", map[string]any{
			"data_ultima_suspensao": day(-50) + " 09:00:27",
		})),
	}}
	srv := m.server(t)
	defer srv.Close()

	rep := BuildBlockedServicesReport(context.Background(), Config{BaseURL: srv.URL}, "tok", day(-10), day(0), "")
	if !rep.OK || rep.Total != 1 {
		t.Fatalf("esperava 1 serviço na janela, veio %+v", rep)
	}
	r := rep.Rows[0]
	if r.Login != "maria" || r.BlockedAt != day(-3) || r.DaysBlocked != 3 || r.DateApprox {
		t.Fatalf("data do último bloqueio errada: %+v", r)
	}
}

func TestPreviewDataVenda_checks(t *testing.T) {
	m := &mockHubsoft{todos: []map[string]any{
		client("1", "10", "João da Silva", svc("100", "joao.silva", "Serviço Habilitado", "servico_habilitado", nil)),
		client("2", "11", "Maria Souza", svc("101", "dup", "x", "servico_habilitado", nil), svc("102", "dup", "x", "servico_habilitado", nil)),
	}}
	srv := m.server(t)
	defer srv.Close()

	rows := []DataVendaInputRow{
		{Line: 2, Login: "joao.silva", ServiceID: "100", ClientCode: "10", ClientName: "JOAO DA SILVA", NewDate: "15/03/2021"}, // ok (nome sem acento/caixa)
		{Line: 3, Login: "dup", ServiceID: "101", ClientName: "Maria Souza", NewDate: "2021-01-01"},                            // login duplicado
		{Line: 4, Login: "naoexiste", ServiceID: "9", ClientName: "X", NewDate: "2021-01-01"},                                  // não encontrado
		{Line: 5, Login: "joao.silva", ServiceID: "100", ClientName: "João da Silva", NewDate: "2021-01-01"},                   // login repetido no CSV
		{Line: 6, Login: "x", ServiceID: "1", ClientName: "X", NewDate: "2099-01-01"},                                          // data no futuro
	}
	out := PreviewDataVenda(context.Background(), Config{BaseURL: srv.URL}, "tok", rows, false)
	if !out.OK {
		t.Fatalf("preview falhou: %s", out.Message)
	}
	want := map[int]bool{2: true, 3: false, 4: false, 5: false, 6: false}
	for _, r := range out.Rows {
		if r.Approved != want[r.Line] {
			t.Errorf("linha %d: aprovada=%v (esperado %v) motivo=%q", r.Line, r.Approved, want[r.Line], r.Reason)
		}
	}
	if out.Approved != 1 || out.Blocked != 4 {
		t.Fatalf("contagem errada: %d aprovadas / %d bloqueadas", out.Approved, out.Blocked)
	}
}

func applyMock(putResp func(id string) (int, map[string]any)) *mockHubsoft {
	cur := client("1", "10", "João da Silva", svc("100", "joao.silva", "Serviço Habilitado", "servico_habilitado", nil))
	return &mockHubsoft{
		byServiceQ: func(id string) map[string]any {
			if id == "100" {
				return cur
			}
			return nil
		},
		byLoginQ: func(login string) []map[string]any { return []map[string]any{cur} },
		put: func(id string, body map[string]string) (int, map[string]any) {
			return putResp(id)
		},
	}
}

func TestApplyDataVendaRow_ok(t *testing.T) {
	m := applyMock(func(id string) (int, map[string]any) {
		return 200, map[string]any{"status": "success", "msg": "ok", "cliente_servico": map[string]any{"id_cliente_servico": 100, "id_cliente": 1, "data_venda": "2021-03-15 00:00:00"}}
	})
	srv := m.server(t)
	defer srv.Close()
	res := ApplyDataVendaRow(context.Background(), Config{BaseURL: srv.URL}, "tok",
		DataVendaApplyInput{Line: 2, Login: "joao.silva", ServiceID: "100", ClientID: "1", ClientName: "João da Silva", NewDate: "2021-03-15"})
	if !res.OK || res.Halt {
		t.Fatalf("esperava sucesso: %+v", res)
	}
	if len(m.puts) != 1 || m.puts[0] != "100=2021-03-15" {
		t.Fatalf("PUT inesperado: %v", m.puts)
	}
}

func TestApplyDataVendaRow_haltsOnInconsistentResponse(t *testing.T) {
	// A HubSoft "confirma" outro serviço/data: tem que parar tudo.
	m := applyMock(func(id string) (int, map[string]any) {
		return 200, map[string]any{"status": "success", "cliente_servico": map[string]any{"id_cliente_servico": 999, "id_cliente": 1, "data_venda": "2020-01-01"}}
	})
	srv := m.server(t)
	defer srv.Close()
	res := ApplyDataVendaRow(context.Background(), Config{BaseURL: srv.URL}, "tok",
		DataVendaApplyInput{Line: 2, Login: "joao.silva", ServiceID: "100", ClientID: "1", ClientName: "João da Silva", NewDate: "2021-03-15"})
	if res.OK || !res.Halt {
		t.Fatalf("esperava halt: %+v", res)
	}
}

func TestApplyDataVendaRow_blocksWhenLoginMoved(t *testing.T) {
	m := applyMock(func(id string) (int, map[string]any) { return 200, map[string]any{"status": "success"} })
	srv := m.server(t)
	defer srv.Close()
	// O CSV diz que o login é outro: recheck por id deve bloquear ANTES do PUT.
	res := ApplyDataVendaRow(context.Background(), Config{BaseURL: srv.URL}, "tok",
		DataVendaApplyInput{Line: 2, Login: "outro.login", ServiceID: "100", ClientID: "1", ClientName: "João da Silva", NewDate: "2021-03-15"})
	if res.OK || len(m.puts) != 0 {
		t.Fatalf("não podia alterar nada: %+v puts=%v", res, m.puts)
	}
}

func TestCountActiveServices(t *testing.T) {
	from := time.Date(2026, 3, 25, 0, 0, 0, 0, time.Local)
	to := time.Date(2026, 9, 25, 0, 0, 0, 0, time.Local)
	c := client("1", "1", "A",
		svc("1", "a", "ok", "servico_habilitado", map[string]any{"data_venda": "10/05/2026"}), // dentro
		svc("2", "b", "ok", "servico_habilitado", map[string]any{"data_venda": "10/05/2020"}), // fora da faixa
		svc("3", "c", "susp", "suspenso_debito", map[string]any{"data_venda": "10/05/2026"}),  // não ativo
	)
	if got := countActiveServices(c, from, to); got != 1 {
		t.Fatalf("esperava 1 serviço ativo na faixa, veio %d", got)
	}
}
