package integrationhubsoft

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestOSUserNameAndNames(t *testing.T) {
	if got := osUserName(map[string]any{"id": 97, "name": "DIOGO FONSECA"}); got != "DIOGO FONSECA" {
		t.Errorf("osUserName objeto = %q", got)
	}
	if got := osUserName([]any{}); got != "" { // O.S. ainda aberta: a HubSoft manda lista vazia
		t.Errorf("osUserName lista vazia = %q", got)
	}
	got := osNames([]any{
		map[string]any{"descricao": "FIBRA ROMPIDA"}, map[string]any{"descricao": "FIBRA ROMPIDA"}, map[string]any{"descricao": "ONU"},
	}, "descricao")
	if len(got) != 2 || got[0] != "FIBRA ROMPIDA" || got[1] != "ONU" {
		t.Errorf("osNames = %v", got)
	}
}

// A conferência pede só as finalizadas à HubSoft (filtro no servidor), traz os dados do fechamento, consulta os
// clientes um a um (poucos clientes) e informa o progresso.
func TestBuildWorkOrderConferenceOnlyFinished(t *testing.T) {
	var mu sync.Mutex
	var osQuery map[string]string
	clientCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/ordem_servico/todos"):
			mu.Lock()
			osQuery = map[string]string{}
			for k := range r.URL.Query() {
				osQuery[k] = r.URL.Query().Get(k)
			}
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success", "paginacao": map[string]any{"pagina_atual": 0, "ultima_pagina": 0, "total_registros": 1},
				"ordens_servico": []any{map[string]any{
					"id_ordem_servico": 679, "numero": "1358", "tipo": "SEM CONEXÃO", "status": "Finalizado",
					"tipo_ordem_servico":     map[string]any{"descricao": "SEM CONEXÃO"},
					"data_inicio_executado":  "2026-10-06 09:24:28",
					"data_termino_executado": "2026-10-06 09:46:39",
					"descricao_fechamento":   "POTÊNCIA ALTA ",
					"motivo_fechamento":      []any{map[string]any{"descricao": "FIBRA DROP ROMPIDA/DANIFICADA"}},
					"usuario_fechamento":     map[string]any{"id": 97, "name": "DIOGO DOS SANTOS FONSECA"},
					"tecnicos":               []any{map[string]any{"id": 97, "name": "DIOGO DOS SANTOS FONSECA", "display": "DIOGO FONSECA"}},
					"atendimento":            map[string]any{"protocolo": "2026"},
					"cliente":                "(2455) TANIA",
					"dados_cliente":          map[string]any{"codigo_cliente": 2455, "nome_razaosocial": "TANIA"},
					"dados_servico":          map[string]any{"id_cliente_servico": 3008, "descricao": "500 MB"},
				}},
			})
		case strings.HasSuffix(r.URL.Path, "/integracao/cliente"):
			mu.Lock()
			clientCalls++
			mu.Unlock()
			if r.URL.Query().Get("busca") != "codigo_cliente" || r.URL.Query().Get("termo_busca") != "2455" {
				t.Errorf("consulta de cliente inesperada: %s", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "clientes": []any{map[string]any{
				"id_cliente": 2621, "codigo_cliente": 2455, "nome_razaosocial": "TANIA",
				"servicos": []any{map[string]any{
					"id_cliente_servico": 3008, "login": "tania01", "ipv4": "",
					"ultima_conexao": map[string]any{"conectado": true, "ultimo_ipv4": "100.64.2.227", "status_txt": "x - 100.64.2.227 - 2804:4d68:48b:1c00::/56(45.235.87.124)"},
				}},
			}}})
		default:
			t.Errorf("chamada inesperada: %s", r.URL.String())
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	var pcts []int
	data := BuildWorkOrderConference(context.Background(), Config{BaseURL: srv.URL}, "tok", ConferenceOptions{
		From: "2026-10-06", To: "2026-10-06", OnlyFinished: true,
		Progress: func(pct int, _ string) { pcts = append(pcts, pct) },
	})
	if !data.OK || len(data.Items) != 1 {
		t.Fatalf("resultado: %+v", data)
	}
	if osQuery["status"] != "finalizado" || osQuery["tipo_data"] != "data_termino_executado" || osQuery["data_inicio"] != "2026-10-06" || osQuery["relacoes"] != "tecnicos" {
		t.Errorf("parâmetros enviados à HubSoft: %v", osQuery)
	}
	it := data.Items[0]
	if it.ClosedBy != "DIOGO DOS SANTOS FONSECA" || it.ClosedAt != "2026-10-06 09:46:39" || it.ClosingDescription != "POTÊNCIA ALTA" ||
		it.Type != "SEM CONEXÃO" || len(it.ClosingReasons) != 1 || len(it.Technicians) != 1 || it.Protocol != "2026" || it.ServiceName != "500 MB" {
		t.Errorf("dados do fechamento: %+v", it)
	}
	if !it.Resolved || it.Login != "tania01" || it.IPv4 != "100.64.2.227" || it.Connected != "true" || it.IPv6 == "" {
		t.Errorf("dados de conexão: %+v", it)
	}
	if clientCalls != 1 {
		t.Errorf("devia consultar 1 cliente, consultou %d", clientCalls)
	}
	if len(pcts) == 0 || pcts[len(pcts)-1] < 90 {
		t.Errorf("progresso informado: %v", pcts)
	}

	// Sem OnlyFinished, o filtro de status não vai para a HubSoft.
	osQuery = nil
	_ = BuildWorkOrderConference(context.Background(), Config{BaseURL: srv.URL}, "tok", ConferenceOptions{From: "2026-10-06", To: "2026-10-06"})
	if _, has := osQuery["status"]; has {
		t.Errorf("sem OnlyFinished não pode filtrar por status: %v", osQuery)
	}
}
