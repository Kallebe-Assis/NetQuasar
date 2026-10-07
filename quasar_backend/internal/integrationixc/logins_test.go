package integrationixc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/netquasar/netquasar/quasar_backend/internal/integrationhttp"
)

// IXC de mentira: radusuarios com PUT por id e listagem por login/id.
type fakeIXC struct {
	mu   sync.Mutex
	recs map[string]map[string]any
	puts []map[string]any
	deny bool
}

func (f *fakeIXC) server(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/radusuarios") && r.Header.Get("ixcsoft") == "listar":
			var q map[string]string
			_ = json.NewDecoder(r.Body).Decode(&q)
			var out []map[string]any
			for _, rec := range f.recs {
				if (q["qtype"] == "radusuarios.login" && strings.EqualFold(rec["login"].(string), q["query"])) || (q["qtype"] == "radusuarios.id" && rec["id"] == q["query"]) {
					out = append(out, rec)
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"page": "1", "total": len(out), "registros": out})
		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/cliente"):
			_ = json.NewEncoder(w).Encode(map[string]any{"total": 1, "registros": []any{map[string]any{"id": "13409", "razao": "JOSE EDUARDO"}}})
		case r.Method == "PUT" && strings.Contains(r.URL.Path, "/radusuarios/"):
			id := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.puts = append(f.puts, body)
			if f.deny {
				_ = json.NewEncoder(w).Encode(map[string]any{"type": "error", "message": "sem permissão"})
				return
			}
			f.recs[id] = body
			_ = json.NewEncoder(w).Encode(map[string]any{"type": "success", "message": "Registro atualizado com sucesso!", "id": id})
		default:
			t.Errorf("chamada inesperada: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	}))
}

func newFake() *fakeIXC {
	return &fakeIXC{recs: map[string]map[string]any{
		"15371": {"id": "15371", "login": "eduardooliveira", "ativo": "S", "online": "S", "id_cliente": "13409", "senha": "g212345", "porta_http": ""},
	}}
}

func TestSetActiveInactivatesAndVerifies(t *testing.T) {
	f := newFake()
	srv := f.server(t)
	defer srv.Close()
	c := Client{Cfg: integrationhttp.IntegrationConfig{BaseURL: srv.URL}}

	recs, err := c.FindByLogin(context.Background(), "EduardoOliveira")
	if err != nil || len(recs) != 1 || recs[0].Str("id") != "15371" || !recs[0].Active() {
		t.Fatalf("FindByLogin: %v %v", recs, err)
	}
	before, after, changed, err := c.SetActive(context.Background(), "15371", "eduardooliveira", false)
	if err != nil || !changed || !before.Active() || after.Active() {
		t.Fatalf("SetActive: changed=%v before=%v after=%v err=%v", changed, before.Str("ativo"), after.Str("ativo"), err)
	}
	// o PUT leva o registro COMPLETO, só com ativo trocado
	put := f.puts[0]
	if put["ativo"] != "N" || put["senha"] != "g212345" || put["id_cliente"] != "13409" || put["login"] != "eduardooliveira" {
		t.Errorf("corpo do PUT: %v", put)
	}
	// já inativo → não envia nada
	n := len(f.puts)
	if _, _, changed, err := c.SetActive(context.Background(), "15371", "eduardooliveira", false); err != nil || changed || len(f.puts) != n {
		t.Errorf("já inativo não pode gerar PUT (changed=%v err=%v)", changed, err)
	}
	// desfazer
	if _, after, changed, err := c.SetActive(context.Background(), "15371", "eduardooliveira", true); err != nil || !changed || !after.Active() {
		t.Errorf("reativar: %v %v", changed, err)
	}
}

func TestSetActiveSafetyChecks(t *testing.T) {
	f := newFake()
	srv := f.server(t)
	defer srv.Close()
	c := Client{Cfg: integrationhttp.IntegrationConfig{BaseURL: srv.URL}}

	// id de outro login → recusa antes de qualquer PUT
	if _, _, _, err := c.SetActive(context.Background(), "15371", "outrologin", false); err == nil || len(f.puts) != 0 {
		t.Errorf("id/login divergentes devem ser recusados sem PUT: err=%v puts=%d", err, len(f.puts))
	}
	// id inexistente
	if _, _, _, err := c.SetActive(context.Background(), "999", "", false); err == nil {
		t.Error("id inexistente devia dar erro")
	}
	// IXC recusa
	f.deny = true
	if _, _, changed, err := c.SetActive(context.Background(), "15371", "eduardooliveira", false); err == nil || changed || !strings.Contains(err.Error(), "recusou") {
		t.Errorf("recusa do IXC: changed=%v err=%v", changed, err)
	}
	if got := c.ClientName(context.Background(), "13409"); got != "JOSE EDUARDO" {
		t.Errorf("ClientName = %q", got)
	}
}
