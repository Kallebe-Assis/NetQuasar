package integrationhubsoft

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClassifyAddresses(t *testing.T) {
	cases := []struct {
		f, c, b, i string
		want       string
		dif        int
	}{
		{"A", "A", "A", "A", "", 0},
		{"A", "B", "B", "B", "fiscal_diferente", 3},     // o caso relatado: só o fiscal é outro
		{"A", "A", "A", "B", "instalacao_diferente", 1}, // 2º ponto
		{"A", "B", "C", "B", "outra", 3},
		{"A", "B", "A", "A", "outra", 1},
	}
	for _, c := range cases {
		got, dif := ClassifyAddresses(c.f, c.c, c.b, c.i)
		if got != c.want || len(dif) != c.dif {
			t.Errorf("%v → %q %v, esperado %q (%d)", c, got, dif, c.want, c.dif)
		}
	}
}

func TestScanAddresses(t *testing.T) {
	addr := func(rua, num string) map[string]any {
		return map[string]any{"endereco": rua, "numero": num, "bairro": "Centro", "cep": "28460000", "cidade": "Miracema", "completo": rua + ", " + num}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if !strings.HasSuffix(r.URL.Path, "/cliente/todos") || !strings.Contains(r.URL.Query().Get("relacoes"), "endereco_fiscal") {
			t.Errorf("chamada inesperada: %s", r.URL.String())
		}
		page := r.URL.Query().Get("pagina")
		list := []any{}
		if page == "0" || page == "" {
			list = []any{
				map[string]any{"id_cliente": 1, "nome_razaosocial": "TUDO IGUAL", "servicos": []any{
					map[string]any{"id_cliente_servico": 10, "login": "a", "endereco_fiscal": addr("Rua A", "1"), "endereco_cadastral": addr("RUA A", "1"), "endereco_cobranca": addr("rua a", "1"), "endereco_instalacao": addr("Rua Á", "1")},
				}},
				map[string]any{"id_cliente": 2, "nome_razaosocial": "FISCAL SOZINHO", "servicos": []any{
					map[string]any{"id_cliente_servico": 20, "login": "b1", "endereco_fiscal": addr("Rua Nova", "9"), "endereco_cadastral": addr("Rua Velha", "5"), "endereco_cobranca": addr("Rua Velha", "5"), "endereco_instalacao": addr("Rua Velha", "5")},
					map[string]any{"id_cliente_servico": 21, "login": "b2", "endereco_fiscal": addr("Rua Nova", "9"), "endereco_cadastral": addr("Rua Nova", "9"), "endereco_cobranca": addr("Rua Nova", "9"), "endereco_instalacao": addr("Rua Outra", "2")},
				}},
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "clientes": list, "paginacao": map[string]any{"pagina_atual": 0, "ultima_pagina": 0, "total_registros": 2}})
	}))
	defer srv.Close()
	res, err := ScanAddresses(context.Background(), Config{BaseURL: srv.URL}, "tok", false)
	if err != nil {
		t.Fatal(err)
	}
	if res.ClientesLidos != 2 || res.ServicosLidos != 3 || res.Iguais != 1 || len(res.Rows) != 2 {
		t.Fatalf("resultado: %+v", res)
	}
	if r := res.Rows[0]; r.Login != "b1" || r.Padrao != "fiscal_diferente" || r.NServicos != 2 || r.Fiscal != "Rua Nova, 9" {
		t.Errorf("linha 0: %+v", r)
	}
	if r := res.Rows[1]; r.Login != "b2" || r.Padrao != "instalacao_diferente" {
		t.Errorf("linha 1: %+v", r)
	}
}
