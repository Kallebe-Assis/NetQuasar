package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/netquasar/netquasar/quasar_backend/internal/integrationhttp"
	"github.com/netquasar/netquasar/quasar_backend/internal/integrationhubsoft"
	"github.com/netquasar/netquasar/quasar_backend/internal/reporttelegram"
	"github.com/netquasar/netquasar/quasar_backend/internal/telegramclient"
)

// persistHubsoftToken grava o token/expiração obtidos por integrationhubsoft.Login
// diretamente (evita o round-trip via RunResult.ResponsePreview que
// persistIntegrationSessionToken usa para o motor genérico).
func persistHubsoftToken(ctx context.Context, s *Server, integID uuid.UUID, token string, expiresInSec int) {
	if token == "" {
		return
	}
	if expiresInSec <= 0 {
		expiresInSec = 86400
	}
	_, _ = s.DB().Exec(ctx, `
		UPDATE integrations SET session_token=$2, session_expires_at=now() + ($3::int * interval '1 second'), updated_at=now() WHERE id=$1
	`, integID, token, expiresInSec)
}

// handlers_hubsoft.go — caminho dedicado da integração HubSoft (não usa o motor genérico
// de internal/integrationconsumer, que continua a servir o IXC sem alterações). Reaproveita
// só o que já é genuinamente genérico/partilhado: a leitura/escrita da linha `integrations`
// (GET/PATCH /api/v1/integrations/{id}, já usados por ambas as integrações) e a persistência
// de sessão/teste (persistIntegrationSessionToken, persistIntegrationTest).

// loadHubsoftConfig confirma que {id} resolve para a linha slug="hubsoft" e devolve a config
// pronta para o pacote integrationhubsoft.
func (s *Server) loadHubsoftConfig(ctx context.Context, integID uuid.UUID) (integrationhubsoft.Config, error) {
	var slug, baseURL string
	var authCfg []byte
	err := s.DB().QueryRow(ctx, `SELECT slug, base_url, auth_config FROM integrations WHERE id=$1`, integID).
		Scan(&slug, &baseURL, &authCfg)
	if err != nil {
		return integrationhubsoft.Config{}, err
	}
	if slug != "hubsoft" {
		return integrationhubsoft.Config{}, errString("esta rota é exclusiva da integração HubSoft")
	}
	return integrationhubsoft.Config{
		BaseURL: strings.TrimSpace(baseURL),
		Auth:    integrationhttp.AuthConfigFromJSON(authCfg),
	}, nil
}

// hubsoftToken devolve um token válido, reaproveitando a sessão gravada em `integrations`
// (session_token/session_expires_at — mesmas colunas que o motor genérico já usa) e só
// fazendo login de novo quando expira.
func (s *Server) hubsoftToken(ctx context.Context, integID uuid.UUID, cfg integrationhubsoft.Config) (string, error) {
	var token string
	var validUntilOK bool
	err := s.DB().QueryRow(ctx, `
		SELECT COALESCE(session_token, ''), (session_expires_at IS NULL OR session_expires_at > now())
		FROM integrations WHERE id=$1
	`, integID).Scan(&token, &validUntilOK)
	if err != nil {
		return "", err
	}
	if token != "" && validUntilOK {
		return token, nil
	}
	newToken, expiresIn, res := integrationhubsoft.Login(ctx, cfg)
	if newToken == "" {
		msg := res.ErrorMessage
		if msg == "" {
			msg = "falha ao autenticar na HubSoft"
		}
		return "", errString(msg)
	}
	persistHubsoftToken(ctx, s, integID, newToken, expiresIn)
	s.logIntegrationRun(ctx, integID, nil, "login", res)
	return newToken, nil
}

func (s *Server) hubsoftTest(w http.ResponseWriter, r *http.Request) {
	integID, err := s.resolveIntegrationID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "identificador inválido", nil)
		return
	}
	cfg, err := s.loadHubsoftConfig(r.Context(), integID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "NOT_HUBSOFT", err.Error(), nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	res := integrationhubsoft.TestConnection(ctx, cfg)
	persistHubsoftToken(ctx, s, integID, res.Token, res.ExpiresIn)
	s.persistIntegrationTest(ctx, integID, integrationhttp.RunResult{OK: res.OK, ErrorMessage: msgIfNotOK(res)})
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": res.OK, "message": res.Message, "latency_ms": res.LatencyMS,
	})
}

func msgIfNotOK(res integrationhubsoft.TestResult) string {
	if res.OK {
		return ""
	}
	return res.Message
}

type hubsoftSearchRequest struct {
	Busca    string `json:"busca"`
	Termo    string `json:"termo"`
	Detailed bool   `json:"detailed"`
}

func (s *Server) hubsoftSearch(w http.ResponseWriter, r *http.Request) {
	integID, err := s.resolveIntegrationID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "identificador inválido", nil)
		return
	}
	var body hubsoftSearchRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	busca := strings.TrimSpace(body.Busca)
	termo := strings.TrimSpace(body.Termo)
	if busca == "" || termo == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "busca e termo são obrigatórios", nil)
		return
	}

	cfg, err := s.loadHubsoftConfig(r.Context(), integID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "NOT_HUBSOFT", err.Error(), nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	token, err := s.hubsoftToken(ctx, integID, cfg)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "AUTH", err.Error(), nil)
		return
	}

	if busca == "ipv4" || busca == "mac" {
		result, serr := integrationhubsoft.SearchClientByConnectionTerm(ctx, cfg, token, termo, busca)
		if serr != nil {
			writeErr(w, http.StatusBadGateway, "HUBSOFT", serr.Error(), nil)
			return
		}
		writeJSON(w, http.StatusOK, result)
		return
	}

	result, res := integrationhubsoft.SearchClients(ctx, cfg, token, busca, termo, body.Detailed)
	s.logIntegrationRun(ctx, integID, nil, "request", res)
	if !res.OK && result.Message == "" {
		result.Message = res.ErrorMessage
	}
	writeJSON(w, http.StatusOK, result)
}

type hubsoftClientRequest struct {
	CodigoCliente string `json:"codigo_cliente"`
}

func (s *Server) hubsoftClientAttendance(w http.ResponseWriter, r *http.Request) {
	integID, err := s.resolveIntegrationID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "identificador inválido", nil)
		return
	}
	var body hubsoftClientRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	codigo := strings.TrimSpace(body.CodigoCliente)
	if codigo == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "codigo_cliente é obrigatório", nil)
		return
	}
	cfg, err := s.loadHubsoftConfig(r.Context(), integID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "NOT_HUBSOFT", err.Error(), nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	token, err := s.hubsoftToken(ctx, integID, cfg)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "AUTH", err.Error(), nil)
		return
	}
	result, res := integrationhubsoft.SearchAttendance(ctx, cfg, token, "codigo_cliente", codigo)
	s.logIntegrationRun(ctx, integID, nil, "request", res)
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) hubsoftClientWorkOrders(w http.ResponseWriter, r *http.Request) {
	integID, err := s.resolveIntegrationID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "identificador inválido", nil)
		return
	}
	var body hubsoftClientRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	codigo := strings.TrimSpace(body.CodigoCliente)
	if codigo == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "codigo_cliente é obrigatório", nil)
		return
	}
	cfg, err := s.loadHubsoftConfig(r.Context(), integID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "NOT_HUBSOFT", err.Error(), nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	token, err := s.hubsoftToken(ctx, integID, cfg)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "AUTH", err.Error(), nil)
		return
	}
	result, res := integrationhubsoft.SearchWorkOrders(ctx, cfg, token, "codigo_cliente", codigo)
	s.logIntegrationRun(ctx, integID, nil, "request", res)
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) hubsoftClientFinancial(w http.ResponseWriter, r *http.Request) {
	integID, err := s.resolveIntegrationID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "identificador inválido", nil)
		return
	}
	var body hubsoftClientRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	codigo := strings.TrimSpace(body.CodigoCliente)
	if codigo == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "codigo_cliente é obrigatório", nil)
		return
	}
	cfg, err := s.loadHubsoftConfig(r.Context(), integID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "NOT_HUBSOFT", err.Error(), nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	token, err := s.hubsoftToken(ctx, integID, cfg)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "AUTH", err.Error(), nil)
		return
	}
	result, res := integrationhubsoft.SearchFinancial(ctx, cfg, token, "codigo_cliente", codigo)
	s.logIntegrationRun(ctx, integID, nil, "request", res)
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) hubsoftDashboard(w http.ResponseWriter, r *http.Request) {
	integID, err := s.resolveIntegrationID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "identificador inválido", nil)
		return
	}
	cfg, err := s.loadHubsoftConfig(r.Context(), integID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "NOT_HUBSOFT", err.Error(), nil)
		return
	}
	// A varredura faz várias centenas de chamadas à API (em paralelo, mas ainda assim
	// demorado) — margem bem maior que os outros handlers (que fazem 1 chamada só).
	extendWriteDeadline(w, 6*time.Minute)
	ctx, cancel := context.WithTimeout(r.Context(), 280*time.Second)
	defer cancel()
	token, err := s.hubsoftToken(ctx, integID, cfg)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "AUTH", err.Error(), nil)
		return
	}
	result := integrationhubsoft.BuildDashboard(ctx, cfg, token)
	writeJSON(w, http.StatusOK, result)
}

// hubsoftCacheTTL janela do cache (Redis — ver s.rt.redis, internal/api/realtime_broker.go) de
// recent-activity/financial-summary: evita repetir a coleta pesada a cada navegação para a tela,
// e é o mesmo cache que ensureIntegrationPreload (server.go) aquece no arranque quando "Carregar
// ao iniciar" está ligado para a integração. Sem Redis configurado, cai para sempre buscar ao
// vivo (comportamento actual, sem cache) — degradação segura, não um erro.
const hubsoftCacheTTL = 10 * time.Minute

func hubsoftRecentActivityCacheKey(integID uuid.UUID) string {
	return "netquasar:hubsoft:recent-activity:" + integID.String()
}

func hubsoftFinancialSummaryCacheKey(integID uuid.UUID) string {
	return "netquasar:hubsoft:financial-summary:" + integID.String()
}

func (s *Server) hubsoftCacheGet(ctx context.Context, key string, out any) bool {
	if s.rt == nil || s.rt.redis == nil {
		return false
	}
	txt, err := s.rt.redis.Get(ctx, key).Result()
	if err != nil || strings.TrimSpace(txt) == "" {
		return false
	}
	return json.Unmarshal([]byte(txt), out) == nil
}

func (s *Server) hubsoftCacheSet(ctx context.Context, key string, v any) {
	if s.rt == nil || s.rt.redis == nil {
		return
	}
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	_ = s.rt.redis.Set(ctx, key, string(b), hubsoftCacheTTL).Err()
}

// fetchHubsoftRecentActivityCached devolve do cache quando fresco, senão coleta e grava — usada
// pelo handler HTTP e pelo pré-aquecimento no arranque (mesma função, mesma chave).
func (s *Server) fetchHubsoftRecentActivityCached(ctx context.Context, integID uuid.UUID, cfg integrationhubsoft.Config, token string) integrationhubsoft.RecentActivityResult {
	key := hubsoftRecentActivityCacheKey(integID)
	var cached integrationhubsoft.RecentActivityResult
	if s.hubsoftCacheGet(ctx, key, &cached) {
		return cached
	}
	result := integrationhubsoft.BuildRecentActivityFast(ctx, cfg, token, 20)
	if result.OK {
		s.hubsoftCacheSet(ctx, key, result)
	}
	return result
}

func (s *Server) fetchHubsoftFinancialSummaryCached(ctx context.Context, integID uuid.UUID, cfg integrationhubsoft.Config, token string) integrationhubsoft.FinancialSummaryResult {
	key := hubsoftFinancialSummaryCacheKey(integID)
	var cached integrationhubsoft.FinancialSummaryResult
	if s.hubsoftCacheGet(ctx, key, &cached) {
		return cached
	}
	result := integrationhubsoft.BuildFinancialSummaryFast(ctx, cfg, token)
	if result.OK {
		s.hubsoftCacheSet(ctx, key, result)
	}
	return result
}

func (s *Server) hubsoftRecentActivity(w http.ResponseWriter, r *http.Request) {
	integID, err := s.resolveIntegrationID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "identificador inválido", nil)
		return
	}
	cfg, err := s.loadHubsoftConfig(r.Context(), integID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "NOT_HUBSOFT", err.Error(), nil)
		return
	}
	// /atendimento/todos e /ordem_servico/todos (paginação real, últimos 30 dias) — bem mais
	// rápido e completo que a antiga varredura por amostra de clientes (BuildRecentActivity).
	extendWriteDeadline(w, 90*time.Second)
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	token, err := s.hubsoftToken(ctx, integID, cfg)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "AUTH", err.Error(), nil)
		return
	}
	result := s.fetchHubsoftRecentActivityCached(ctx, integID, cfg, token)
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) hubsoftFinancialSummary(w http.ResponseWriter, r *http.Request) {
	integID, err := s.resolveIntegrationID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "identificador inválido", nil)
		return
	}
	cfg, err := s.loadHubsoftConfig(r.Context(), integID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "NOT_HUBSOFT", err.Error(), nil)
		return
	}
	// /financeiro/fatura (paginação real, últimos 6 meses) — bem mais rápido e completo que a
	// antiga varredura por amostra de clientes (BuildFinancialSummary).
	extendWriteDeadline(w, 90*time.Second)
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	token, err := s.hubsoftToken(ctx, integID, cfg)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "AUTH", err.Error(), nil)
		return
	}
	result := s.fetchHubsoftFinancialSummaryCached(ctx, integID, cfg, token)
	writeJSON(w, http.StatusOK, result)
}

type hubsoftResendInvoiceBody struct {
	ExtraEmails []string `json:"extra_emails"`
}

// hubsoftResendInvoiceEmail — "Reenviar fatura por e-mail" (aba Financeiro): dispara o próprio
// envio da HubSoft (servidor de e-mail dela) para a fatura indicada. Acção manual, disparada só
// quando o operador clica — não é chamada por nenhuma automação/agendamento.
func (s *Server) hubsoftResendInvoiceEmail(w http.ResponseWriter, r *http.Request) {
	integID, err := s.resolveIntegrationID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "identificador inválido", nil)
		return
	}
	invoiceID := strings.TrimSpace(chi.URLParam(r, "invoiceId"))
	if invoiceID == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "id da fatura é obrigatório", nil)
		return
	}
	var body hubsoftResendInvoiceBody
	_ = json.NewDecoder(r.Body).Decode(&body)
	cfg, err := s.loadHubsoftConfig(r.Context(), integID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "NOT_HUBSOFT", err.Error(), nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	token, err := s.hubsoftToken(ctx, integID, cfg)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "AUTH", err.Error(), nil)
		return
	}
	if err := integrationhubsoft.ResendInvoiceEmail(ctx, cfg, token, invoiceID, body.ExtraEmails); err != nil {
		writeErr(w, http.StatusBadGateway, "HUBSOFT", err.Error(), nil)
		return
	}
	s.appendAuditLog(ctx, "hubsoft_invoice", invoiceID, "resend_email", s.actorFromRequest(r), nil, map[string]any{"integration_id": integID.String()})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// hubsoftPreloadBody corpo do PATCH .../hubsoft/preload — liga/desliga o pré-aquecimento desta
// integração no arranque do servidor (ver ensureIntegrationPreload, server.go).
type hubsoftPreloadBody struct {
	PreloadOnStartup bool `json:"preload_on_startup"`
}

func (s *Server) hubsoftGetPreload(w http.ResponseWriter, r *http.Request) {
	integID, err := s.resolveIntegrationID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "identificador inválido", nil)
		return
	}
	var v bool
	if err := s.DB().QueryRow(r.Context(), `SELECT preload_on_startup FROM integrations WHERE id=$1`, integID).Scan(&v); err != nil {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "integração não encontrada", nil)
		return
	}
	writeJSON(w, http.StatusOK, hubsoftPreloadBody{PreloadOnStartup: v})
}

func (s *Server) hubsoftSetPreload(w http.ResponseWriter, r *http.Request) {
	integID, err := s.resolveIntegrationID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "identificador inválido", nil)
		return
	}
	var body hubsoftPreloadBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	if _, err := s.DB().Exec(r.Context(), `UPDATE integrations SET preload_on_startup=$1 WHERE id=$2`, body.PreloadOnStartup, integID); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) hubsoftBuscaOptions(w http.ResponseWriter, r *http.Request) {
	opts := integrationhubsoft.BuscaOptions()
	opts = append(opts, integrationhubsoft.BuscaOption{Value: "ipv4", Label: "IPv4"}, integrationhubsoft.BuscaOption{Value: "mac", Label: "MAC"})
	writeJSON(w, http.StatusOK, map[string]any{"busca_options": opts})
}

// --- Aba Relatório (Configurações → Integrações → HubSoft → Relatório) -----------------------
// Usa os endpoints "todos"/"listar" (paginação real, confirmados na documentação oficial —
// ver comentário no início de report* em internal/integrationhubsoft/hubsoft.go) em vez da
// varredura por amostra usada pelo Dashboard/Atendimentos-recentes/Financeiro-resumo antigos.

func (s *Server) hubsoftReportClients(w http.ResponseWriter, r *http.Request) {
	integID, err := s.resolveIntegrationID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "identificador inválido", nil)
		return
	}
	cfg, err := s.loadHubsoftConfig(r.Context(), integID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "NOT_HUBSOFT", err.Error(), nil)
		return
	}
	q := r.URL.Query()
	filter := integrationhubsoft.ReportListFilter{
		ServiceStatus: strings.TrimSpace(q.Get("servico_status")),
		Cancelado:     strings.TrimSpace(q.Get("cancelado")),
		State:         strings.TrimSpace(q.Get("estado")),
		City:          strings.TrimSpace(q.Get("cidade")),
		Neighborhood:  strings.TrimSpace(q.Get("bairro")),
		IPv4:          strings.TrimSpace(q.Get("ipv4")),
		MAC:           strings.TrimSpace(q.Get("mac")),
		Login:         strings.TrimSpace(q.Get("login")),
	}
	// Filtro por conexão (IPv4/MAC/Login) é 1 chamada directa e rápida; a varredura paginada de
	// /cliente/todos é mais pesada — margem maior só quando ela vai mesmo ser usada.
	timeout := 45 * time.Second
	if filter.IPv4 == "" && filter.MAC == "" && filter.Login == "" {
		timeout = 3 * time.Minute
		extendWriteDeadline(w, 4*time.Minute)
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	token, err := s.hubsoftToken(ctx, integID, cfg)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "AUTH", err.Error(), nil)
		return
	}
	result, rerr := integrationhubsoft.ListClientServiceReport(ctx, cfg, token, filter)
	if rerr != nil {
		writeErr(w, http.StatusBadGateway, "HUBSOFT", rerr.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func periodFromQuery(r *http.Request) (from, to string) {
	q := r.URL.Query()
	return strings.TrimSpace(q.Get("data_inicio")), strings.TrimSpace(q.Get("data_fim"))
}

func (s *Server) hubsoftReportAttendance(w http.ResponseWriter, r *http.Request) {
	integID, err := s.resolveIntegrationID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "identificador inválido", nil)
		return
	}
	cfg, err := s.loadHubsoftConfig(r.Context(), integID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "NOT_HUBSOFT", err.Error(), nil)
		return
	}
	extendWriteDeadline(w, 3*time.Minute)
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	token, err := s.hubsoftToken(ctx, integID, cfg)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "AUTH", err.Error(), nil)
		return
	}
	from, to := periodFromQuery(r)
	writeJSON(w, http.StatusOK, integrationhubsoft.BuildAttendancePeriodReport(ctx, cfg, token, from, to))
}

func (s *Server) hubsoftReportWorkOrders(w http.ResponseWriter, r *http.Request) {
	integID, err := s.resolveIntegrationID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "identificador inválido", nil)
		return
	}
	cfg, err := s.loadHubsoftConfig(r.Context(), integID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "NOT_HUBSOFT", err.Error(), nil)
		return
	}
	extendWriteDeadline(w, 3*time.Minute)
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	token, err := s.hubsoftToken(ctx, integID, cfg)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "AUTH", err.Error(), nil)
		return
	}
	from, to := periodFromQuery(r)
	writeJSON(w, http.StatusOK, integrationhubsoft.BuildWorkOrderPeriodReport(ctx, cfg, token, from, to))
}

func (s *Server) hubsoftReportFinancial(w http.ResponseWriter, r *http.Request) {
	integID, err := s.resolveIntegrationID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "identificador inválido", nil)
		return
	}
	cfg, err := s.loadHubsoftConfig(r.Context(), integID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "NOT_HUBSOFT", err.Error(), nil)
		return
	}
	extendWriteDeadline(w, 3*time.Minute)
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	token, err := s.hubsoftToken(ctx, integID, cfg)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "AUTH", err.Error(), nil)
		return
	}
	from, to := periodFromQuery(r)
	writeJSON(w, http.StatusOK, integrationhubsoft.BuildFinancialPeriodReport(ctx, cfg, token, from, to))
}

// hubsoftAttendanceDetail — "Ver mais" na aba Atendimentos: busca UM atendimento pelo protocolo,
// com a conversa completa (mensagens). Pedido leve (1 registo), não precisa de deadline extra.
func (s *Server) hubsoftAttendanceDetail(w http.ResponseWriter, r *http.Request) {
	integID, err := s.resolveIntegrationID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "identificador inválido", nil)
		return
	}
	protocolo := strings.TrimSpace(r.URL.Query().Get("protocolo"))
	if protocolo == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "protocolo é obrigatório", nil)
		return
	}
	cfg, err := s.loadHubsoftConfig(r.Context(), integID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "NOT_HUBSOFT", err.Error(), nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	token, err := s.hubsoftToken(ctx, integID, cfg)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "AUTH", err.Error(), nil)
		return
	}
	out, derr := integrationhubsoft.FetchAttendanceDetail(ctx, cfg, token, protocolo)
	if derr != nil {
		writeErr(w, http.StatusBadGateway, "HUBSOFT", derr.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// hubsoftWorkOrderDetail — "Ver mais" na aba Ordens de serviço: busca UMA O.S. pelo número, com
// a conversa completa (mensagens).
func (s *Server) hubsoftWorkOrderDetail(w http.ResponseWriter, r *http.Request) {
	integID, err := s.resolveIntegrationID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "identificador inválido", nil)
		return
	}
	numero := strings.TrimSpace(r.URL.Query().Get("numero"))
	if numero == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "numero é obrigatório", nil)
		return
	}
	cfg, err := s.loadHubsoftConfig(r.Context(), integID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "NOT_HUBSOFT", err.Error(), nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	token, err := s.hubsoftToken(ctx, integID, cfg)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "AUTH", err.Error(), nil)
		return
	}
	out, derr := integrationhubsoft.FetchWorkOrderDetail(ctx, cfg, token, numero)
	if derr != nil {
		writeErr(w, http.StatusBadGateway, "HUBSOFT", derr.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// hubsoftFinancialList — aba Financeiro: lista paginada de faturas (1 página HTTP por página
// pedida — ver ListInvoices), com filtro por período e busca livre.
func (s *Server) hubsoftFinancialList(w http.ResponseWriter, r *http.Request) {
	integID, err := s.resolveIntegrationID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "identificador inválido", nil)
		return
	}
	cfg, err := s.loadHubsoftConfig(r.Context(), integID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "NOT_HUBSOFT", err.Error(), nil)
		return
	}
	q := r.URL.Query()
	page, _ := strconv.Atoi(strings.TrimSpace(q.Get("page")))
	perPage, _ := strconv.Atoi(strings.TrimSpace(q.Get("per_page")))
	filter := integrationhubsoft.InvoiceListFilter{
		From: strings.TrimSpace(q.Get("data_inicio")), To: strings.TrimSpace(q.Get("data_fim")),
		ApenasEmAberto: strings.TrimSpace(q.Get("apenas_em_aberto")),
		ApenasQuitado:  strings.TrimSpace(q.Get("apenas_quitado")),
		Busca:          strings.TrimSpace(q.Get("busca")),
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	token, err := s.hubsoftToken(ctx, integID, cfg)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "AUTH", err.Error(), nil)
		return
	}
	out, lerr := integrationhubsoft.ListInvoices(ctx, cfg, token, filter, page, perPage)
	if lerr != nil {
		writeErr(w, http.StatusBadGateway, "HUBSOFT", lerr.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// --- Envio por Telegram (aba Relatório da integração HubSoft) ---------------------------------
// Reaproveita o mesmo bot "reports" (Configurações → Telegram) e o mesmo formatador de texto
// (reporttelegram.ComposeSystemReport) já usados pelos Relatórios do sistema (handlers_system_
// reports.go) — só muda a fonte dos dados (Build*PeriodReport, já existentes) e o período (o que
// o usuário tiver seleccionado na tela naquele momento, não um período fixo).

func (s *Server) hubsoftSendTelegram(ctx context.Context, title string, summary map[string]any) error {
	cfg, err := telegramclient.LoadConfig(ctx, s.DB(), "reports")
	if err != nil {
		return err
	}
	if !cfg.Ready() {
		return fmt.Errorf("Telegram de relatórios não configurado (bot_token/chat_id) — configure em Configurações → Telegram")
	}
	payload := map[string]any{"generated_at": time.Now().UTC().Format(time.RFC3339), "summary": summary}
	text := reporttelegram.ComposeSystemReport(title, payload)
	return telegramclient.SendMessageChunks(ctx, cfg, text)
}

func (s *Server) hubsoftReportAttendanceTelegram(w http.ResponseWriter, r *http.Request) {
	integID, err := s.resolveIntegrationID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "identificador inválido", nil)
		return
	}
	cfg, err := s.loadHubsoftConfig(r.Context(), integID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "NOT_HUBSOFT", err.Error(), nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	token, err := s.hubsoftToken(ctx, integID, cfg)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "AUTH", err.Error(), nil)
		return
	}
	from, to := periodFromQuery(r)
	rep := integrationhubsoft.BuildAttendancePeriodReport(ctx, cfg, token, from, to)
	if !rep.OK {
		writeErr(w, http.StatusBadGateway, "HUBSOFT", firstNonEmptyStr(rep.Message, "falha ao coletar atendimentos"), nil)
		return
	}
	summary := map[string]any{
		"Período":    rep.From + " a " + rep.To,
		"Total":      rep.Total,
		"Fechados":   rep.Closed,
		"Abertos":    rep.Open,
		"% fechados": fmt.Sprintf("%.1f%%", rep.ClosedPct),
	}
	for _, st := range rep.ByStatus {
		summary["Status: "+st.Name] = st.Count
	}
	if err := s.hubsoftSendTelegram(ctx, "HubSoft — Atendimentos por período", summary); err != nil {
		writeErr(w, http.StatusBadGateway, "TELEGRAM_SEND_FAILED", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) hubsoftReportWorkOrdersTelegram(w http.ResponseWriter, r *http.Request) {
	integID, err := s.resolveIntegrationID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "identificador inválido", nil)
		return
	}
	cfg, err := s.loadHubsoftConfig(r.Context(), integID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "NOT_HUBSOFT", err.Error(), nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	token, err := s.hubsoftToken(ctx, integID, cfg)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "AUTH", err.Error(), nil)
		return
	}
	from, to := periodFromQuery(r)
	rep := integrationhubsoft.BuildWorkOrderPeriodReport(ctx, cfg, token, from, to)
	if !rep.OK {
		writeErr(w, http.StatusBadGateway, "HUBSOFT", firstNonEmptyStr(rep.Message, "falha ao coletar ordens de serviço"), nil)
		return
	}
	summary := map[string]any{
		"Período":       rep.From + " a " + rep.To,
		"Total":         rep.Total,
		"Finalizadas":   rep.Finished,
		"% finalizadas": fmt.Sprintf("%.1f%%", rep.FinishedPct),
	}
	for i, t := range rep.ByTechnician {
		if i >= 10 {
			break
		}
		summary[fmt.Sprintf("Técnico: %s", t.Technician)] = fmt.Sprintf("%d finalizadas de %d (%.1f%% do total)", t.Finished, t.Total, t.PctOfFinished)
	}
	if err := s.hubsoftSendTelegram(ctx, "HubSoft — Ordens de serviço por período", summary); err != nil {
		writeErr(w, http.StatusBadGateway, "TELEGRAM_SEND_FAILED", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) hubsoftReportFinancialTelegram(w http.ResponseWriter, r *http.Request) {
	integID, err := s.resolveIntegrationID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "identificador inválido", nil)
		return
	}
	cfg, err := s.loadHubsoftConfig(r.Context(), integID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "NOT_HUBSOFT", err.Error(), nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	token, err := s.hubsoftToken(ctx, integID, cfg)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "AUTH", err.Error(), nil)
		return
	}
	from, to := periodFromQuery(r)
	rep := integrationhubsoft.BuildFinancialPeriodReport(ctx, cfg, token, from, to)
	if !rep.OK {
		writeErr(w, http.StatusBadGateway, "HUBSOFT", firstNonEmptyStr(rep.Message, "falha ao coletar faturas"), nil)
		return
	}
	summary := map[string]any{
		"Período":     rep.From + " a " + rep.To,
		"Faturas":     rep.Total,
		"Valor total": fmt.Sprintf("R$ %.2f", rep.TotalValue),
		"Recebido":    fmt.Sprintf("R$ %.2f (%.1f%%) — %d fatura(s)", rep.PaidValue, rep.PaidPct, rep.PaidCount),
		"Em aberto":   fmt.Sprintf("R$ %.2f (%.1f%%) — %d fatura(s)", rep.OpenValue, rep.OpenPct, rep.OpenCount),
		"Vencido":     fmt.Sprintf("R$ %.2f (%.1f%%) — %d fatura(s)", rep.OverdueValue, rep.OverduePct, rep.OverdueCount),
	}
	if err := s.hubsoftSendTelegram(ctx, "HubSoft — Financeiro por período", summary); err != nil {
		writeErr(w, http.StatusBadGateway, "TELEGRAM_SEND_FAILED", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
