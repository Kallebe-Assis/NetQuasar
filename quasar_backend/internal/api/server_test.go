package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/netquasar/netquasar/quasar_backend/internal/api"
	"github.com/netquasar/netquasar/quasar_backend/internal/config"
	"github.com/rs/zerolog"
)

func TestHealth(t *testing.T) {
	cfg := &config.Config{HTTPAddr: ":0", LogLevel: "error"}
	h := api.NewServer(zerolog.Nop(), cfg, nil, context.Background())
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d", rr.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" {
		t.Fatalf("body %+v", body)
	}
}

func TestToolsDNSRun(t *testing.T) {
	cfg := &config.Config{HTTPAddr: ":0", LogLevel: "error"}
	h := api.NewServer(zerolog.Nop(), cfg, nil, context.Background())
	payload := `{"host":"example.com","record_types":["A"],"timeout_ms":5000}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tools/dns/run", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	recs, _ := body["records"].(map[string]any)
	if len(recs) == 0 && body["error"] == nil {
		t.Fatalf("sem registros nem erro: %+v", body)
	}
}

func TestToolsHTTPProbe(t *testing.T) {
	cfg := &config.Config{HTTPAddr: ":0", LogLevel: "error"}
	h := api.NewServer(zerolog.Nop(), cfg, nil, context.Background())
	payload := `{"url":"https://example.com","timeout_ms":8000,"follow_redirects":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tools/http-https-probe", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d", rr.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if _, ok := body["ok"]; !ok {
		t.Fatalf("body %+v", body)
	}
}
