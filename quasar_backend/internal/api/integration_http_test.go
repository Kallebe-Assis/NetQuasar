// Testes de integração HTTP contra Postgres real.
//
// Configure NETQUASAR_TEST_DATABASE_URL (ex.: postgres://user:pass@localhost:5432/netquasar_test?sslmode=disable).
// Opcional: go test -short ./... pula internet-check, monitoring/start e tools que usam rede.
package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/api"
	"github.com/netquasar/netquasar/quasar_backend/internal/bootstrap"
	"github.com/netquasar/netquasar/quasar_backend/internal/config"
	"github.com/netquasar/netquasar/quasar_backend/internal/db"
	"github.com/rs/zerolog"
)

type integrationHarness struct {
	T      *testing.T
	Base   string
	Client *http.Client
}

func newIntegration(t *testing.T) (integrationHarness, func()) {
	t.Helper()
	dsn := os.Getenv("NETQUASAR_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("NETQUASAR_TEST_DATABASE_URL não definido — Postgres necessário para integração")
	}
	t.Setenv("NETQUASAR_DATABASE_URL", dsn)
	t.Setenv("NETQUASAR_API_KEYS", "")
	t.Setenv("NETQUASAR_CORS_ORIGINS", "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := db.Migrate(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	pool, err := db.NewPool(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := bootstrap.EnsureDefaultUsers(ctx, pool); err != nil {
		pool.Close()
		t.Fatal(err)
	}
	if err := bootstrap.EnsureDatabaseMetaRow(ctx, pool, cfg); err != nil {
		pool.Close()
		t.Fatal(err)
	}

	var dbHolder atomic.Pointer[pgxpool.Pool]
	dbHolder.Store(pool)
	srv := httptest.NewServer(api.NewServer(zerolog.Nop(), cfg, &dbHolder, context.Background()))
	h := integrationHarness{
		T:      t,
		Base:   srv.URL + "/api/v1",
		Client: &http.Client{Timeout: 45 * time.Second},
	}

	return h, func() {
		srv.Close()
		if p := dbHolder.Swap(nil); p != nil {
			p.Close()
		}
	}
}

func (h integrationHarness) url(path string) string {
	if strings.HasPrefix(path, "http") {
		return path
	}
	return h.Base + path
}

func (h integrationHarness) do(method, path string, body io.Reader) *http.Response {
	h.T.Helper()
	req, err := http.NewRequest(method, h.url(path), body)
	if err != nil {
		h.T.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := h.Client.Do(req)
	if err != nil {
		h.T.Fatal(err)
	}
	return res
}

// slurp lê e fecha o corpo e exige um dos status permitidos.
func (h integrationHarness) slurp(res *http.Response, allowed ...int) []byte {
	h.T.Helper()
	b, err := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if err != nil {
		h.T.Fatal(err)
	}
	ok := false
	for _, c := range allowed {
		if res.StatusCode == c {
			ok = true
			break
		}
	}
	if !ok {
		h.T.Fatalf("HTTP %d (esperado um de %v) corpo: %s", res.StatusCode, allowed, b)
	}
	return b
}

func (h integrationHarness) decodeJSON(b []byte, dst any) {
	h.T.Helper()
	if err := json.Unmarshal(b, dst); err != nil {
		h.T.Fatalf("json: %v; corpo: %s", err, b)
	}
}

func TestIntegrationAPI_FullStack(t *testing.T) {
	hc, done := newIntegration(t)
	defer done()

	t.Run("api_v1_health", func(t *testing.T) {
		b := hc.slurp(hc.do(http.MethodGet, "/health", nil), http.StatusOK)
		var m map[string]any
		hc.decodeJSON(b, &m)
		if m["status"] != "ok" {
			t.Fatalf("%+v", m)
		}
	})

	t.Run("overview_summary", func(t *testing.T) {
		b := hc.slurp(hc.do(http.MethodGet, "/overview/summary", nil), http.StatusOK)
		var m map[string]any
		hc.decodeJSON(b, &m)
		for _, k := range []string{"devices", "pops", "commercial_clients_sum", "monitoring_running"} {
			if _, ok := m[k]; !ok {
				t.Fatalf("falta chave %q em %+v", k, m)
			}
		}
	})

	t.Run("dashboard_analytics", func(t *testing.T) {
		b := hc.slurp(hc.do(http.MethodGet, "/dashboard/analytics?days=7", nil), http.StatusOK)
		var m map[string]any
		hc.decodeJSON(b, &m)
		for _, k := range []string{"generated_at", "days", "since", "totals", "ping_window", "devices_by_category"} {
			if _, ok := m[k]; !ok {
				t.Fatalf("GET /dashboard/analytics falta %q: %+v", k, m)
			}
		}
	})

	t.Run("settings_database", func(t *testing.T) {
		b := hc.slurp(hc.do(http.MethodGet, "/settings/database", nil), http.StatusOK)
		var dbm map[string]any
		hc.decodeJSON(b, &dbm)
		for _, k := range []string{"db_user_masked", "ssl_mode", "password_configured", "active_dsn_source"} {
			if _, ok := dbm[k]; !ok {
				t.Fatalf("GET /settings/database falta %q: %+v", k, dbm)
			}
		}
		hc.slurp(hc.do(http.MethodPatch, "/settings/database", strings.NewReader(`{"host":"db.example","port":5432,"db_user":"u","db_name":"n","ssl_mode":"require"}`)), http.StatusOK)
		hc.slurp(hc.do(http.MethodPost, "/settings/database/test", strings.NewReader(`{}`)), http.StatusOK)
		hc.slurp(hc.do(http.MethodGet, "/settings/database/logs?limit=10", nil), http.StatusOK)
	})

	t.Run("settings_connection_defaults", func(t *testing.T) {
		hc.slurp(hc.do(http.MethodGet, "/settings/connection/defaults", nil), http.StatusOK)
		hc.slurp(hc.do(http.MethodPatch, "/settings/connection/defaults",
			strings.NewReader(`{"snmp_community":"public-test","telnet_user":"u"}`)), http.StatusOK)
	})

	t.Run("settings_monitoring_intervals", func(t *testing.T) {
		hc.slurp(hc.do(http.MethodGet, "/settings/monitoring-intervals", nil), http.StatusOK)
		hc.slurp(hc.do(http.MethodPatch, "/settings/monitoring-intervals",
			strings.NewReader(`{"ping_seconds":10,"telemetry_minutes":1}`)), http.StatusUnprocessableEntity)
		hc.slurp(hc.do(http.MethodPatch, "/settings/monitoring-intervals",
			strings.NewReader(`{"ping_seconds":45,"telemetry_minutes":3}`)), http.StatusOK)
		hc.slurp(hc.do(http.MethodPatch, "/settings/monitoring-intervals",
			strings.NewReader(`{"ping_seconds":30,"telemetry_minutes":2}`)), http.StatusOK)
	})

	t.Run("settings_monitoring_patch_invalid_timeout", func(t *testing.T) {
		hc.slurp(hc.do(http.MethodPatch, "/settings/monitoring",
			strings.NewReader(`{"internet_check_timeout_ms":999999}`)), http.StatusUnprocessableEntity)
	})

	t.Run("settings_monitoring_get_patch", func(t *testing.T) {
		hc.slurp(hc.do(http.MethodGet, "/settings/monitoring", nil), http.StatusOK)
		payload := `{"vps_latency_offset_ms":5,"internet_check_targets":["https://1.1.1.1"],"internet_check_timeout_ms":4000}`
		hc.slurp(hc.do(http.MethodPatch, "/settings/monitoring", strings.NewReader(payload)), http.StatusOK)
	})

	t.Run("settings_olt_vendors", func(t *testing.T) {
		b := hc.slurp(hc.do(http.MethodGet, "/settings/olt-vendors", nil), http.StatusOK)
		var m map[string]any
		hc.decodeJSON(b, &m)
		brands, _ := m["brands"].([]any)
		if len(brands) < 3 {
			t.Fatalf("esperava várias marcas, veio %+v", m)
		}
		hc.slurp(hc.do(http.MethodGet, "/settings/olt-vendors/catalog", nil), http.StatusOK)
		hc.slurp(hc.do(http.MethodGet, "/settings/olt-vendors/ZTE/models", nil), http.StatusOK)
		hc.slurp(hc.do(http.MethodPatch, "/settings/olt-vendors/ZTE/models/Padr%C3%A3o",
			strings.NewReader(`{"onu_online_oid":"1.3.6.1.4.1.3902.1012.3.1.1.1"}`)), http.StatusOK)
	})

	t.Run("settings_telegram_and_automation", func(t *testing.T) {
		hc.slurp(hc.do(http.MethodGet, "/settings/notifications/telegram/monitoring", nil), http.StatusOK)
		hc.slurp(hc.do(http.MethodPatch, "/settings/notifications/telegram/monitoring",
			strings.NewReader(`{"chat_id":"-100123","topic_id":"1"}`)), http.StatusOK)
		hc.slurp(hc.do(http.MethodPost, "/settings/notifications/telegram/monitoring/test", strings.NewReader(`{}`)), http.StatusOK)
		hc.slurp(hc.do(http.MethodGet, "/settings/notifications/telegram/reports", nil), http.StatusOK)
		hc.slurp(hc.do(http.MethodPatch, "/settings/notifications/telegram/reports",
			strings.NewReader(`{"chat_id":"-200456"}`)), http.StatusOK)
		hc.slurp(hc.do(http.MethodPost, "/settings/notifications/telegram/reports/test", strings.NewReader(`{}`)), http.StatusOK)

		hc.slurp(hc.do(http.MethodGet, "/settings/automation/onu-monthly-report", nil), http.StatusOK)
		hc.slurp(hc.do(http.MethodPatch, "/settings/automation/onu-monthly-report",
			strings.NewReader(`{"enabled":false,"mode":"disabled","time_hhmm":"09:30"}`)), http.StatusOK)
		hc.slurp(hc.do(http.MethodPost, "/settings/automation/onu-monthly-report/run", strings.NewReader(`{}`)), http.StatusAccepted)
		hc.slurp(hc.do(http.MethodGet, "/settings/automation/onu-monthly-report/runs", nil), http.StatusOK)
	})

	t.Run("settings_user_not_found", func(t *testing.T) {
		hc.slurp(hc.do(http.MethodGet, "/settings/users/00000000-0000-0000-0000-000000000099", nil), http.StatusNotFound)
	})

	t.Run("settings_users_crud", func(t *testing.T) {
		hc.slurp(hc.do(http.MethodGet, "/settings/users", nil), http.StatusOK)
		em := fmt.Sprintf("it%d@test.netquasar", time.Now().UnixNano())
		payload := fmt.Sprintf(`{"display_name":"IT User","email":%q,"phone":"11987654321","password":"x12345678","role":"viewer"}`, em)
		b := hc.slurp(hc.do(http.MethodPost, "/settings/users", strings.NewReader(payload)), http.StatusCreated)
		var created map[string]any
		hc.decodeJSON(b, &created)
		uid := created["id"].(string)
		hc.slurp(hc.do(http.MethodGet, "/settings/users/"+uid, nil), http.StatusOK)
		hc.slurp(hc.do(http.MethodPatch, "/settings/users/"+uid,
			strings.NewReader(`{"password":"y87654321"}`)), http.StatusOK)
		hc.slurp(hc.do(http.MethodDelete, "/settings/users/"+uid, nil), http.StatusNoContent)
	})

	t.Run("monitoring_reload_state_internet", func(t *testing.T) {
		hc.slurp(hc.do(http.MethodPost, "/monitoring/reload-devices", strings.NewReader(`{}`)), http.StatusOK)
		hc.slurp(hc.do(http.MethodGet, "/monitoring/state", nil), http.StatusOK)
		if testing.Short() {
			t.Skip("short: sem internet-check/start")
		}
		hc.slurp(hc.do(http.MethodGet, "/monitoring/internet-check", nil), http.StatusOK)
		res := hc.do(http.MethodPost, "/monitoring/start", strings.NewReader(`{"mode":"simple_ping"}`))
		hc.slurp(res, http.StatusOK, http.StatusFailedDependency)
		hc.slurp(hc.do(http.MethodPost, "/monitoring/stop", strings.NewReader(`{}`)), http.StatusOK)
	})

	t.Run("pops_devices_map_ping_flow", func(t *testing.T) {
		suf := time.Now().UnixNano()
		popBody := fmt.Sprintf(`{"description":"POP-IT-%d","address":"Rua A","latitude":-19.9,"longitude":-43.9}`, suf)
		b := hc.slurp(hc.do(http.MethodPost, "/pops", strings.NewReader(popBody)), http.StatusCreated)
		var popOut map[string]any
		hc.decodeJSON(b, &popOut)
		popID := popOut["id"].(string)

		hc.slurp(hc.do(http.MethodGet, "/pops", nil), http.StatusOK)
		hc.slurp(hc.do(http.MethodGet, "/pops/"+popID, nil), http.StatusOK)
		hc.slurp(hc.do(http.MethodPatch, "/pops/"+popID, strings.NewReader(`{"description":"POP-IT-Patched"}`)), http.StatusOK)

		devJSON := fmt.Sprintf(`{"pop_id":%q,"category":"OLT","description":"Dev-%d","network_status":"Normal","ip":"1.1.1.1","ping_enabled":true,"telemetry_enabled":false,"latitude":-23.55,"longitude":-46.63}`,
			popID, suf)
		b = hc.slurp(hc.do(http.MethodPost, "/devices", strings.NewReader(devJSON)), http.StatusCreated)
		var devOut map[string]any
		hc.decodeJSON(b, &devOut)
		devID := devOut["id"].(string)

		hc.slurp(hc.do(http.MethodGet, "/devices", nil), http.StatusOK)
		hc.slurp(hc.do(http.MethodGet, "/devices/"+devID, nil), http.StatusOK)
		hc.slurp(hc.do(http.MethodGet, "/devices/"+devID+"/status", nil), http.StatusOK)
		hc.slurp(hc.do(http.MethodPatch, "/devices/"+devID, strings.NewReader(`{"description":"Dev-Patched","brand":"ZTE"}`)), http.StatusOK)

		bulk := fmt.Sprintf(`{"device_ids":["%s"]}`, devID)
		hc.slurp(hc.do(http.MethodPost, "/pops/"+popID+"/devices/bulk", strings.NewReader(bulk)), http.StatusOK)

		mb := hc.slurp(hc.do(http.MethodGet, "/map/equipment-points", nil), http.StatusOK)
		var mp map[string]any
		hc.decodeJSON(mb, &mp)
		pts, _ := mp["points"].([]any)
		if len(pts) == 0 {
			t.Fatalf("esperava pontos no mapa após device com lat/lng")
		}

		hc.slurp(hc.do(http.MethodGet, "/ping/devices/"+devID+"/run?port=443&timeout_ms=5000", nil), http.StatusOK)

		hc.slurp(hc.do(http.MethodDelete, "/devices/"+devID, nil), http.StatusNoContent)
		hc.slurp(hc.do(http.MethodDelete, "/pops/"+popID, nil), http.StatusNoContent)
	})

	t.Run("commercial_full", func(t *testing.T) {
		suf := time.Now().UnixNano()
		locBody := fmt.Sprintf(`{"name":"Loc-%d","region_code":"BR-SP"}`, suf)
		b := hc.slurp(hc.do(http.MethodPost, "/commercial/localities", strings.NewReader(locBody)), http.StatusCreated)
		var loc map[string]any
		hc.decodeJSON(b, &loc)
		lid := loc["id"].(string)

		hc.slurp(hc.do(http.MethodGet, "/commercial/localities", nil), http.StatusOK)
		hc.slurp(hc.do(http.MethodPatch, "/commercial/localities/"+lid, strings.NewReader(`{"region_code":"BR-RJ"}`)), http.StatusOK)

		rec := fmt.Sprintf(`{"locality_id":%q,"year_month":"2026-04","client_count":10}`, lid)
		br := hc.slurp(hc.do(http.MethodPost, "/commercial/monthly-records", strings.NewReader(rec)), http.StatusCreated)
		var mrec map[string]any
		hc.decodeJSON(br, &mrec)
		mid := mrec["id"].(string)
		hc.slurp(hc.do(http.MethodGet, "/commercial/monthly-records/"+mid, nil), http.StatusOK)
		hc.slurp(hc.do(http.MethodPatch, "/commercial/monthly-records/"+mid, strings.NewReader(`{"client_count":11}`)), http.StatusOK)
		hc.slurp(hc.do(http.MethodDelete, "/commercial/monthly-records/"+mid, nil), http.StatusNoContent)

		bulk := fmt.Sprintf(`{"records":[{"locality_id":%q,"year_month":"2026-05","client_count":20}]}`, lid)
		hc.slurp(hc.do(http.MethodPost, "/commercial/monthly-records/bulk", strings.NewReader(bulk)), http.StatusOK)

		hc.slurp(hc.do(http.MethodGet, "/commercial/monthly-records", nil), http.StatusOK)
		hc.slurp(hc.do(http.MethodGet, "/commercial/aggregates?month=2026-05", nil), http.StatusOK)

		hc.slurp(hc.do(http.MethodDelete, "/commercial/localities/"+lid, nil), http.StatusNoContent)
	})

	t.Run("alerts_endpoints", func(t *testing.T) {
		hc.slurp(hc.do(http.MethodGet, "/alerts/active", nil), http.StatusOK)
		hc.slurp(hc.do(http.MethodPost, "/alerts/revalidate", strings.NewReader(`{}`)), http.StatusOK)
		hc.slurp(hc.do(http.MethodGet, "/alerts/suppressions", nil), http.StatusOK)
		b := hc.slurp(hc.do(http.MethodPost, "/alerts/suppressions", strings.NewReader(
			`{"scope_type":"pop","scope_ref":"all","reason":"integration"}`)), http.StatusCreated)
		var sup map[string]any
		hc.decodeJSON(b, &sup)
		sid := sup["id"].(string)
		hc.slurp(hc.do(http.MethodGet, "/alerts/suppressions/"+sid, nil), http.StatusOK)
		hc.slurp(hc.do(http.MethodPatch, "/alerts/suppressions/"+sid, strings.NewReader(`{"reason":"integration-patched"}`)), http.StatusOK)
		hc.slurp(hc.do(http.MethodDelete, "/alerts/suppressions/"+sid, nil), http.StatusNoContent)
	})

	t.Run("tools_endpoints", func(t *testing.T) {
		if testing.Short() {
			t.Skip("short: tools dns/http usam rede")
		}
		hc.slurp(hc.do(http.MethodPost, "/tools/dns/run",
			strings.NewReader(`{"host":"example.com","record_types":["A"],"timeout_ms":5000}`)), http.StatusOK)
		hc.slurp(hc.do(http.MethodPost, "/tools/http-https-probe",
			strings.NewReader(`{"url":"https://example.com","timeout_ms":8000,"follow_redirects":true}`)), http.StatusOK)
		hc.slurp(hc.do(http.MethodPost, "/tools/icmp/ping",
			strings.NewReader(`{"host":"127.0.0.1","timeout_ms":3000}`)), http.StatusOK)
		hc.slurp(hc.do(http.MethodPost, "/tools/snmp/get",
			strings.NewReader(`{"host":"127.0.0.1","port":161,"community":"public","oids":["1.3.6.1.2.1.1.1.0"],"timeout_ms":1500,"retries":0}`)), http.StatusOK)
		hc.slurp(hc.do(http.MethodPost, "/tools/telnet/test",
			strings.NewReader(`{"host":"127.0.0.1","port":"65530","timeout_ms":1500}`)), http.StatusOK)
		hc.slurp(hc.do(http.MethodPost, "/tools/ssh/test",
			strings.NewReader(`{"host":"127.0.0.1","port":"65529","user":"nouser","password":"x","timeout_ms":2000}`)), http.StatusOK)
	})
}
