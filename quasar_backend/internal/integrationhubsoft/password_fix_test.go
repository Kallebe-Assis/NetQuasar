package integrationhubsoft

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLiteralExcelPassword(t *testing.T) {
	for in, want := range map[string]string{`="12345"`: "12345", ` ="0012" `: "0012", `="abc`: "abc", `=""`: ""} {
		if got, ok := literalExcelPassword(in); !ok || got != want {
			t.Errorf("literalExcelPassword(%q) = %q,%v want %q", in, got, ok, want)
		}
	}
	for _, in := range []string{"12345", "g212345", `a="1"`, "", `"12345"`} {
		if _, ok := literalExcelPassword(in); ok {
			t.Errorf("%q não é senha em formato de planilha", in)
		}
	}
}

func TestScanLiteralPasswords(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if p := r.URL.Query().Get("pagina"); p != "0" && p != "" {
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "clientes": []any{}, "paginacao": map[string]any{"pagina_atual": 1, "ultima_pagina": 0, "total_registros": 2}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "clientes": []any{
			map[string]any{"id_cliente": 1, "codigo_cliente": 10, "nome_razaosocial": "A", "servicos": []any{
				map[string]any{"id_cliente_servico": 11, "login": "ok1", "senha": "g212345"},
				map[string]any{"id_cliente_servico": 12, "login": "erika1", "senha": `="12345"`},
			}},
			map[string]any{"id_cliente": 2, "nome_razaosocial": "B", "servicos": []any{
				map[string]any{"id_cliente_servico": 21, "login": "curta", "senha": `="1"`},
			}},
		}, "paginacao": map[string]any{"pagina_atual": 0, "ultima_pagina": 0, "total_registros": 2}})
	}))
	defer srv.Close()
	scan, err := ScanLiteralPasswords(context.Background(), Config{BaseURL: srv.URL}, "tok")
	if err != nil {
		t.Fatal(err)
	}
	if scan.ServicosLidos != 3 || len(scan.Rows) != 2 {
		t.Fatalf("scan: %+v", scan)
	}
	if scan.Rows[0].IDClienteServico != "12" || scan.Rows[0].SenhaCorreta != "12345" || scan.Rows[0].Problema != "" {
		t.Errorf("linha 0: %+v", scan.Rows[0])
	}
	if scan.Rows[1].Problema == "" {
		t.Errorf("senha de 1 caractere deve vir com problema: %+v", scan.Rows[1])
	}
}

func TestFixLiteralPassword(t *testing.T) {
	cur := `="12345"`
	var posts []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" {
			_ = json.NewEncoder(w).Encode(map[string]any{"clientes": []any{map[string]any{"servicos": []any{map[string]any{"id_cliente_servico": 12, "login": "erika1", "senha": cur}}}}})
			return
		}
		var b map[string]any
		_ = json.NewDecoder(r.Body).Decode(&b)
		posts = append(posts, b)
		cur, _ = b["password"].(string)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "msg": "ok"})
	}))
	defer srv.Close()
	cfg := Config{BaseURL: srv.URL}
	res := FixLiteralPassword(context.Background(), cfg, "tok", "12")
	if !res.OK || res.Skipped || cur != "12345" {
		t.Fatalf("devia corrigir: %+v cur=%q", res, cur)
	}
	if _, has := posts[0]["login"]; has || len(posts[0]) != 2 {
		t.Errorf("o POST só pode levar id_cliente_servico e password: %v", posts[0])
	}
	// já corrigida → não mexe
	posts = nil
	if res = FixLiteralPassword(context.Background(), cfg, "tok", "12"); !res.OK || !res.Skipped || len(posts) != 0 {
		t.Errorf("senha já limpa não pode gerar POST: %+v posts=%v", res, posts)
	}
	// outra senha qualquer (do cliente) nunca é tocada
	cur = "SenhaDoCliente9"
	if res = FixLiteralPassword(context.Background(), cfg, "tok", "12"); !res.Skipped || len(posts) != 0 || !strings.Contains(res.Message, "nada a fazer") {
		t.Errorf("senha diferente não pode ser alterada: %+v", res)
	}
}
