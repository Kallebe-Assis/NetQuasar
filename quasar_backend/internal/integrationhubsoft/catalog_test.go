package integrationhubsoft

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

// Catálogo paginado (locais de estoque): o servidor lê todas as páginas e devolve um corpo único.
func TestFetchCatalogPaginado(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/integracao/estoque/local_estoque" || r.URL.Query().Get("relacoes") != "usuarios" {
			t.Errorf("chamada inesperada: %s", r.URL.String())
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("pagina"))
		locais := [][]any{{map[string]any{"id_local_estoque": 1, "descricao": "ALMOX A"}, map[string]any{"id_local_estoque": 2, "descricao": "ALMOX B"}}, {map[string]any{"id_local_estoque": 3, "descricao": "ALMOX C"}}}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "locais_estoque": locais[page],
			"paginacao": map[string]any{"pagina_atual": page, "ultima_pagina": 1, "total_registros": 3}})
	}))
	defer srv.Close()
	code, body, err := FetchCatalog(context.Background(), Config{BaseURL: srv.URL}, "tok", "estoque_local")
	if err != nil || code != 200 {
		t.Fatalf("code=%d err=%v", code, err)
	}
	var doc struct {
		Locais    []map[string]any `json:"locais_estoque"`
		Paginacao map[string]any   `json:"paginacao"`
	}
	if json.Unmarshal(body, &doc) != nil || len(doc.Locais) != 3 || doc.Locais[2]["descricao"] != "ALMOX C" {
		t.Errorf("corpo: %s", body)
	}
}

// Todo catálogo da lista de exibição precisa ter endpoint, e vice-versa.
func TestCatalogListaCoerente(t *testing.T) {
	for _, c := range CatalogList {
		if _, ok := CatalogEndpoints[c.ID]; !ok {
			t.Errorf("catálogo %q na lista sem endpoint", c.ID)
		}
	}
	for id := range CatalogEndpoints {
		found := false
		for _, c := range CatalogList {
			found = found || c.ID == id
		}
		if !found {
			t.Errorf("endpoint %q fora da lista de exibição", id)
		}
	}
}
