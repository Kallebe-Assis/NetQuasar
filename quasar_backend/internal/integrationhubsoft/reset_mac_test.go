package integrationhubsoft

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func resetMacServer(t *testing.T, status int, resp map[string]any, gotBody *map[string]string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || !strings.HasSuffix(r.URL.Path, "/api/v1/integracao/cliente/reset_mac_addr") {
			t.Errorf("chamada inesperada: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

func TestResetClientServiceMAC_success(t *testing.T) {
	var body map[string]string
	srv := resetMacServer(t, 200, map[string]any{"status": "success", "msg": "Endereço MAC resetado com sucesso!"}, &body)
	defer srv.Close()
	msg, err := ResetClientServiceMAC(context.Background(), Config{BaseURL: srv.URL}, "tok", " 11000 ")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if body["id_cliente_servico"] != "11000" {
		t.Errorf("corpo enviado = %v", body)
	}
	if !strings.Contains(msg, "MAC") {
		t.Errorf("msg = %q", msg)
	}
}

// A HubSoft devolve "status":"error" com HTTP 200 — não pode passar por sucesso.
func TestResetClientServiceMAC_errorOnHTTP200(t *testing.T) {
	var body map[string]string
	srv := resetMacServer(t, 200, map[string]any{"status": "error", "msg": "Serviço sem dados de autenticação", "errors": []string{"login ausente"}}, &body)
	defer srv.Close()
	_, err := ResetClientServiceMAC(context.Background(), Config{BaseURL: srv.URL}, "tok", "11000")
	if err == nil || !strings.Contains(err.Error(), "sem dados de autenticação") || !strings.Contains(err.Error(), "login ausente") {
		t.Fatalf("erro = %v", err)
	}
}

func TestResetClientServiceMAC_requiresID(t *testing.T) {
	if _, err := ResetClientServiceMAC(context.Background(), Config{}, "tok", "  "); err == nil {
		t.Fatal("devia exigir id_cliente_servico")
	}
}
