package integrationhubsoft

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCheckServiceFormas(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		id := r.URL.Query().Get("termo_busca")
		svc := func(sid int, login, status string, forma any) map[string]any {
			m := map[string]any{"id_cliente_servico": sid, "login": login, "status": status, "nome": "500 MB - PÓS PAGO (G2)"}
			if forma != nil {
				m["forma_cobranca"] = forma
			}
			return m
		}
		var cli map[string]any
		switch id {
		case "1": // todos OK
			cli = map[string]any{"id_cliente": 1, "nome_razaosocial": "A", "servicos": []any{svc(10, "a1", "Serviço Habilitado", map[string]any{"id_forma_cobranca": 14, "descricao": "BB API G2"})}}
		case "2": // um serviço em outra forma
			cli = map[string]any{"id_cliente": 2, "nome_razaosocial": "B", "servicos": []any{
				svc(20, "b1", "Serviço Habilitado", map[string]any{"id_forma_cobranca": 19, "descricao": "Sicoob - API (G2)"}),
				svc(21, "b2", "Serviço Habilitado", map[string]any{"id_forma_cobranca": 14, "descricao": "BB API G2"})}}
		case "3": // serviço cancelado em outra forma não conta; ativo sem dado
			cli = map[string]any{"id_cliente": 3, "nome_razaosocial": "C", "servicos": []any{
				svc(30, "c1", "Cancelado", map[string]any{"id_forma_cobranca": 19, "descricao": "Sicoob"}),
				svc(31, "c2", "Serviço Habilitado", nil)}}
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "clientes": []any{}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "clientes": []any{cli}})
	}))
	defer srv.Close()

	rep := CheckServiceFormas(context.Background(), Config{BaseURL: srv.URL}, "tok", []string{"1", "2", "3", "9", "1"}, "14")
	if !rep.OK || rep.Clientes != 4 || rep.Servicos != 4 {
		t.Fatalf("relatório: %+v", rep)
	}
	if rep.ServicosOK != 2 || rep.ServicosDiff != 1 || rep.ServicosSemDado != 1 || rep.ClientesTodosOK != 1 {
		t.Errorf("contagens: ok=%d dif=%d semdado=%d clientesOK=%d", rep.ServicosOK, rep.ServicosDiff, rep.ServicosSemDado, rep.ClientesTodosOK)
	}
	if len(rep.ClientesComDiferenca) != 2 || len(rep.NaoEncontrados) != 1 || rep.NaoEncontrados[0] != "9" {
		t.Errorf("clientes: dif=%v nao=%v", rep.ClientesComDiferenca, rep.NaoEncontrados)
	}
	if rep.Rows[0].Resultado == ServiceFormaOK {
		t.Errorf("divergentes devem vir primeiro: %+v", rep.Rows[0])
	}
}

func TestCheckServiceFormasSemCampo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "clientes": []any{map[string]any{"id_cliente": 1, "servicos": []any{map[string]any{"id_cliente_servico": 1, "login": "x", "status": "Habilitado"}}}}})
	}))
	defer srv.Close()
	rep := CheckServiceFormas(context.Background(), Config{BaseURL: srv.URL}, "tok", []string{"1"}, "14")
	if rep.ServicosSemDado != 1 || rep.Message == "" || rep.CamposServico["login"] != "x" {
		t.Errorf("sem o campo deve avisar e mostrar os campos do serviço: %+v", rep)
	}
}
