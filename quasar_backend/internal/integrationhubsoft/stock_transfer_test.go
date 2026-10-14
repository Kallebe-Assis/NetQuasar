package integrationhubsoft

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// trMock simula a HubSoft para a transferência entre locais.
type trMock struct {
	mu       sync.Mutex
	items    map[int]map[string]any // id_produto_item -> {local, status, produto}
	posts    []map[string]any
	failPost bool
	noMove   bool // aceita a transferência mas não muda o local (simula problema)
}

func newTrMock() *trMock {
	m := &trMock{items: map[int]map[string]any{}}
	add := func(id, local int, status string, prod int) {
		m.items[id] = map[string]any{"local": local, "status": status, "prod": prod}
	}
	add(1, 6, "estoque", 10)
	add(2, 6, "estoque", 10)
	add(3, 6, "estoque", 11)
	add(4, 6, "comodato", 10) // fora do estoque
	add(5, 7, "estoque", 10)  // já no destino
	return m
}

func (m *trMock) item(id int) map[string]any {
	it := m.items[id]
	nomes := map[int]string{6: "ALMOXARIFADO MATRIZ", 7: "ALMOXARIFADO SUPORTE"}
	local := it["local"].(int)
	st := it["status"].(string)
	return map[string]any{
		"id_produto_item": id, "codigo_item": 1000 + id, "identificador_proprio": "ID" + strconv.Itoa(id),
		"produto":             map[string]any{"id_produto": it["prod"], "nome": "PRODUTO"},
		"local_estoque":       map[string]any{"id_local_estoque": local, "descricao": nomes[local]},
		"produto_item_status": map[string]any{"descricao": strings.ToUpper(st), "prefixo": st},
	}
}

func (m *trMock) handler(t *testing.T) http.Handler {
	const P = "/api/v1/integracao/estoque"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		raw, _ := io.ReadAll(r.Body)
		enc := json.NewEncoder(w)
		m.mu.Lock()
		defer m.mu.Unlock()
		switch {
		case r.Method == "GET" && r.URL.Path == P+"/local_estoque/7":
			_ = enc.Encode(map[string]any{"status": "success", "local_estoque": map[string]any{"id_local_estoque": 7, "descricao": "ALMOXARIFADO SUPORTE"}})
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, P+"/local_estoque/"):
			_ = enc.Encode(map[string]any{"status": "error", "msg": "Local de estoque não encontrado"})
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, P+"/produto_item/"):
			id, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, P+"/produto_item/"))
			if _, ok := m.items[id]; !ok {
				_ = enc.Encode(map[string]any{"status": "error", "msg": "Patrimônio não encontrado"})
				return
			}
			_ = enc.Encode(map[string]any{"status": "success", "produto": m.item(id)})
		case r.Method == "POST" && r.URL.Path == P+"/movimento_estoque/transferencia":
			if m.failPost {
				w.WriteHeader(422)
				_ = enc.Encode(map[string]any{"status": "error", "msg": "Dados inválidos", "errors": []string{"sem permissão no local de origem"}})
				return
			}
			var b map[string]any
			_ = json.Unmarshal(raw, &b)
			m.posts = append(m.posts, b)
			if !m.noMove {
				for _, p := range b["produtos"].([]any) {
					for _, pat := range p.(map[string]any)["patrimonios"].([]any) {
						id := int(pat.(map[string]any)["id_produto_item"].(float64))
						m.items[id]["local"] = int(b["id_local_estoque_destino"].(float64))
					}
				}
			}
			_ = enc.Encode(map[string]any{"status": "success", "msg": "Transferência realizada com sucesso", "movimentos_estoque": map[string]any{"saida": map[string]any{"id_movimento_estoque": 900}, "entrada": map[string]any{"id_movimento_estoque": 901}}})
		default:
			t.Errorf("chamada inesperada: %s %s", r.Method, r.URL.String())
			w.WriteHeader(404)
		}
	})
}

func trRows(ids ...int) []TransferRow {
	var out []TransferRow
	for i, id := range ids {
		out = append(out, TransferRow{Linha: i + 2, IDProdutoItem: strconv.Itoa(id)})
	}
	return out
}

func TestPreviewTransferBloqueiosEJaNoDestino(t *testing.T) {
	m := newTrMock()
	srv := httptest.NewServer(m.handler(t))
	defer srv.Close()
	cfg := Config{BaseURL: srv.URL}
	dest, pv := PreviewTransferRows(context.Background(), cfg, "tok", append(trRows(1, 4, 5, 99), TransferRow{Linha: 9, IDProdutoItem: "abc"}), TransferOptions{IDLocalDestino: "7"})
	if dest != "ALMOXARIFADO SUPORTE" {
		t.Fatalf("destino: %q", dest)
	}
	if !pv[0].OK || pv[0].AlreadyDone {
		t.Errorf("item em estoque deve liberar: %+v", pv[0])
	}
	if pv[1].OK || !strings.Contains(strings.Join(pv[1].Problems, " "), "só patrimônio em ESTOQUE") {
		t.Errorf("comodato deve bloquear: %+v", pv[1])
	}
	if !pv[2].OK || !pv[2].AlreadyDone {
		t.Errorf("já no destino não é erro: %+v", pv[2])
	}
	if pv[3].OK || pv[4].OK {
		t.Errorf("inexistente / id inválido devem bloquear: %+v %+v", pv[3], pv[4])
	}
	// destino inexistente bloqueia tudo
	_, bad := PreviewTransferRows(context.Background(), cfg, "tok", trRows(1), TransferOptions{IDLocalDestino: "55"})
	if bad[0].OK || !strings.Contains(strings.Join(bad[0].Problems, " "), "não existe") {
		t.Errorf("destino inexistente: %+v", bad[0])
	}
}

func TestApplyTransferAgrupaEConfere(t *testing.T) {
	m := newTrMock()
	srv := httptest.NewServer(m.handler(t))
	defer srv.Close()
	res, halted := ApplyTransferRows(context.Background(), Config{BaseURL: srv.URL}, "tok", trRows(1, 2, 3, 4, 5), TransferOptions{IDLocalDestino: "7", Observacao: "organização"})
	if halted {
		t.Fatal("não deveria parar")
	}
	want := []string{"transferred", "transferred", "transferred", "blocked", "already_done"}
	for i, w := range want {
		if res[i].Action != w {
			t.Errorf("linha %d: %s (%s), esperava %s", i, res[i].Action, res[i].Message, w)
		}
	}
	// 1 e 2 são do mesmo produto e local → uma chamada; 3 é de outro produto → outra
	if len(m.posts) != 2 {
		t.Fatalf("posts: %d", len(m.posts))
	}
	p0 := m.posts[0]
	prod := p0["produtos"].([]any)[0].(map[string]any)
	if p0["id_local_estoque_origem"] != float64(6) || p0["id_local_estoque_destino"] != float64(7) || p0["observacao"] != "organização" ||
		prod["quantidade"] != float64(2) || prod["campo_identificacao_patrimonio"] != "id_produto_item" || len(prod["patrimonios"].([]any)) != 2 {
		t.Errorf("corpo: %+v", p0)
	}
	if res[0].IDMovimento != "900/901" || res[0].LocalDepois != "ALMOXARIFADO SUPORTE" {
		t.Errorf("resultado: %+v", res[0])
	}
	// reenviar: tudo «já estava» e nada novo é enviado
	res2, _ := ApplyTransferRows(context.Background(), Config{BaseURL: srv.URL}, "tok", trRows(1, 2, 3), TransferOptions{IDLocalDestino: "7"})
	for _, r := range res2 {
		if r.Action != "already_done" {
			t.Errorf("reenvio: %+v", r)
		}
	}
	if len(m.posts) != 2 {
		t.Errorf("reenvio não pode transferir de novo: %d", len(m.posts))
	}
}

func TestApplyTransferFalhaEConferenciaDivergente(t *testing.T) {
	m := newTrMock()
	m.failPost = true
	srv := httptest.NewServer(m.handler(t))
	res, _ := ApplyTransferRows(context.Background(), Config{BaseURL: srv.URL}, "tok", trRows(1, 2), TransferOptions{IDLocalDestino: "7"})
	srv.Close()
	if res[0].Action != "failed" || !strings.Contains(res[0].Message, "sem permissão") || res[0].Detail == "" {
		t.Errorf("falha da HubSoft deve mostrar a mensagem e o corpo bruto: %+v", res[0])
	}
	m2 := newTrMock()
	m2.noMove = true
	srv2 := httptest.NewServer(m2.handler(t))
	defer srv2.Close()
	res2, _ := ApplyTransferRows(context.Background(), Config{BaseURL: srv2.URL}, "tok", trRows(1), TransferOptions{IDLocalDestino: "7"})
	if res2[0].Action != "verify_failed" || !strings.Contains(res2[0].Message, "continua no local") {
		t.Errorf("releitura deve pegar o local que não mudou: %+v", res2[0])
	}
}

func TestPreviewTransferRepetidoNoLote(t *testing.T) {
	m := newTrMock()
	srv := httptest.NewServer(m.handler(t))
	defer srv.Close()
	_, pv := PreviewTransferRows(context.Background(), Config{BaseURL: srv.URL}, "tok", trRows(1, 2, 1), TransferOptions{IDLocalDestino: "7"})
	if !pv[0].OK || !pv[1].OK || pv[2].OK || !strings.Contains(strings.Join(pv[2].Problems, " "), "repetido") {
		t.Fatalf("segunda ocorrência deve ser bloqueada: %+v", pv)
	}
	res, _ := ApplyTransferRows(context.Background(), Config{BaseURL: srv.URL}, "tok", trRows(1, 2, 1), TransferOptions{IDLocalDestino: "7"})
	if res[0].Action != "transferred" || res[2].Action != "blocked" || len(m.posts) != 1 {
		t.Errorf("envio: %+v posts=%d", res, len(m.posts))
	}
}
