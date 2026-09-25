package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	pdfapi "github.com/pdfcpu/pdfcpu/pkg/api"
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
		integrationhubsoft.EnrichSuspendedSince(ctx, cfg, token, &result)
		writeJSON(w, http.StatusOK, result)
		return
	}

	result, res := integrationhubsoft.SearchClients(ctx, cfg, token, busca, termo, body.Detailed)
	s.logIntegrationRun(ctx, integID, nil, "request", res)
	if !res.OK && result.Message == "" {
		result.Message = res.ErrorMessage
	}
	// A API da HubSoft não pagina de verdade este endpoint (testado ao vivo — ver comentário em
	// SearchClientsQueryOverrides) e rejeita limit acima de 100: bater exactamente nesse teto é o
	// único sinal de que pode haver mais resultados não mostrados — avisa em vez de dar a entender
	// silenciosamente que a lista está completa.
	integrationhubsoft.EnrichSuspendedSince(ctx, cfg, token, &result)
	if result.OK && len(result.Clients) == 100 {
		result.Message = "Mostrando os 100 primeiros resultados (limite da API HubSoft para esta consulta) — pode haver mais; refine o termo de busca para ver os demais."
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

type hubsoftEnableServiceBody struct {
	MotivoHabilitacao string `json:"motivo_habilitacao"`
}

// hubsoftEnableClientService — "Habilitar serviço" (aba Consulta/serviços do cliente): acção
// manual, disparada só quando o operador clica. A HubSoft só aceita esta transição quando o
// serviço está Suspenso por Débito, Suspenso Parcialmente ou Suspenso Pedido Cliente — qualquer
// outro estado actual devolve o erro da própria HubSoft, relançado tal-qual em HUBSOFT.
func (s *Server) hubsoftEnableClientService(w http.ResponseWriter, r *http.Request) {
	integID, err := s.resolveIntegrationID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "identificador inválido", nil)
		return
	}
	serviceID := strings.TrimSpace(chi.URLParam(r, "serviceId"))
	if serviceID == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "id do serviço é obrigatório", nil)
		return
	}
	var body hubsoftEnableServiceBody
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
	if err := integrationhubsoft.EnableClientService(ctx, cfg, token, serviceID, body.MotivoHabilitacao); err != nil {
		writeErr(w, http.StatusBadGateway, "HUBSOFT", err.Error(), nil)
		return
	}
	s.appendAuditLog(ctx, "hubsoft_client_service", serviceID, "enable", s.actorFromRequest(r), nil, map[string]any{"integration_id": integID.String(), "motivo_habilitacao": body.MotivoHabilitacao})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

type hubsoftSuspendServiceBody struct {
	TipoSuspensao string `json:"tipo_suspensao"`
}

// hubsoftSuspendClientService — "Suspender serviço" (aba Consulta/serviços do cliente). A HubSoft
// só aceita tipo_suspensao "suspenso_debito" ou "suspenso_pedido_cliente" (únicos documentados).
func (s *Server) hubsoftSuspendClientService(w http.ResponseWriter, r *http.Request) {
	integID, err := s.resolveIntegrationID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "identificador inválido", nil)
		return
	}
	serviceID := strings.TrimSpace(chi.URLParam(r, "serviceId"))
	if serviceID == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "id do serviço é obrigatório", nil)
		return
	}
	var body hubsoftSuspendServiceBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	reason := integrationhubsoft.HubsoftSuspendReason(strings.TrimSpace(body.TipoSuspensao))
	if reason != integrationhubsoft.SuspendReasonDebito && reason != integrationhubsoft.SuspendReasonPedidoCliente {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "tipo_suspensao deve ser suspenso_debito ou suspenso_pedido_cliente", nil)
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
	if err := integrationhubsoft.SuspendClientService(ctx, cfg, token, serviceID, reason); err != nil {
		writeErr(w, http.StatusBadGateway, "HUBSOFT", err.Error(), nil)
		return
	}
	s.appendAuditLog(ctx, "hubsoft_client_service", serviceID, "suspend", s.actorFromRequest(r), nil, map[string]any{"integration_id": integID.String(), "tipo_suspensao": string(reason)})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

const (
	hubsoftMaxBoletosPerMerge = 50
	hubsoftMaxBoletoBytes     = 25 << 20 // 25MB por boleto — generoso para um PDF de fatura, evita abuso
)

type hubsoftMergeBoletosBody struct {
	CodigoCliente string   `json:"codigo_cliente"`
	InvoiceIDs    []string `json:"invoice_ids"`
}

// fetchPDFBytes baixa um PDF por HTTP simples (não via integrationhttp.Execute — esse capa a
// pré-visualização em 2MB e ANEXA texto ao corpo quando corta, o que corromperia o PDF; ver
// internal/integrationhttp/client.go). Valida o cabeçalho mágico "%PDF" para não juntar lixo
// (ex.: uma página de erro HTML da HubSoft) no ficheiro final.
func fetchPDFBytes(ctx context.Context, client *http.Client, url string, maxBytes int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("boleto excede o tamanho máximo permitido")
	}
	if len(data) < 4 || string(data[:4]) != "%PDF" {
		return nil, fmt.Errorf("resposta não é um PDF válido")
	}
	return data, nil
}

func sanitizeFilenamePart(s string) string {
	safe := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, s)
	if safe == "" {
		return "cliente"
	}
	return safe
}

// hubsoftMergeBoletos — "Baixar selecionados" (aba Financeiro do cliente): junta N boletos
// escolhidos pelo operador num único PDF para download. Recebe só os invoice_ids escolhidos e
// busca de novo o financeiro do cliente na HubSoft para obter os boleto_link ACTUAIS — evita
// confiar em URLs vindas do pedido do browser (o link vem sempre de uma resposta fresca e
// autenticada da própria HubSoft, nunca de entrada do utilizador).
func (s *Server) hubsoftMergeBoletos(w http.ResponseWriter, r *http.Request) {
	integID, err := s.resolveIntegrationID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "identificador inválido", nil)
		return
	}
	var body hubsoftMergeBoletosBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	codigo := strings.TrimSpace(body.CodigoCliente)
	if codigo == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "codigo_cliente é obrigatório", nil)
		return
	}
	seen := map[string]bool{}
	var ids []string
	for _, id := range body.InvoiceIDs {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "selecione pelo menos um boleto", nil)
		return
	}
	if len(ids) > hubsoftMaxBoletosPerMerge {
		writeErr(w, http.StatusBadRequest, "VALIDATION", fmt.Sprintf("máximo de %d boletos por download", hubsoftMaxBoletosPerMerge), nil)
		return
	}

	cfg, err := s.loadHubsoftConfig(r.Context(), integID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "NOT_HUBSOFT", err.Error(), nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	token, err := s.hubsoftToken(ctx, integID, cfg)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "AUTH", err.Error(), nil)
		return
	}

	result, runRes := integrationhubsoft.SearchFinancial(ctx, cfg, token, "codigo_cliente", codigo)
	s.logIntegrationRun(ctx, integID, nil, "request", runRes)

	linkByID := make(map[string]string, len(result.Invoices))
	for _, inv := range result.Invoices {
		if inv.ID != "" {
			linkByID[inv.ID] = strings.TrimSpace(inv.BoletoLink)
		}
	}

	type fetched struct {
		id   string
		data []byte
	}
	var pdfs []fetched
	var missing []string
	httpClient := &http.Client{Timeout: 20 * time.Second}
	for _, id := range ids {
		link := linkByID[id]
		if link == "" {
			missing = append(missing, id)
			continue
		}
		data, ferr := fetchPDFBytes(ctx, httpClient, link, hubsoftMaxBoletoBytes)
		if ferr != nil {
			missing = append(missing, id)
			continue
		}
		pdfs = append(pdfs, fetched{id: id, data: data})
	}
	if len(pdfs) == 0 {
		writeErr(w, http.StatusBadGateway, "HUBSOFT", "não foi possível obter nenhum dos boletos selecionados", nil)
		return
	}

	readers := make([]io.ReadSeeker, len(pdfs))
	for i, p := range pdfs {
		readers[i] = bytes.NewReader(p.data)
	}
	var out bytes.Buffer
	if err := pdfapi.MergeRaw(readers, &out, false, nil); err != nil {
		writeErr(w, http.StatusInternalServerError, "PDF_MERGE", "falha ao juntar os boletos num único PDF: "+err.Error(), nil)
		return
	}

	s.appendAuditLog(ctx, "hubsoft_invoice", codigo, "merge_boletos_download", s.actorFromRequest(r), nil, map[string]any{
		"integration_id": integID.String(), "invoice_ids": ids, "merged_count": len(pdfs), "missing": missing,
	})

	filename := fmt.Sprintf("boletos_%s_%s.pdf", sanitizeFilenamePart(codigo), time.Now().Format("20060102"))
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	if len(missing) > 0 {
		w.Header().Set("X-Hubsoft-Missing-Invoices", strings.Join(missing, ","))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(out.Bytes())
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

// hubsoftReportServices — aba Relatório → Serviços: quantos serviços existem, quantos em cada
// status, quantos em cada plano, e a mesma repartição por localidade (ver BuildServicesReport).
// Sem período (fotografia do estado actual) — varre a base inteira via /cliente/todos, por isso
// usa a mesma folga generosa de timeout do relatório de Clientes sem filtro de conexão.
func (s *Server) hubsoftReportServices(w http.ResponseWriter, r *http.Request) {
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
	extendWriteDeadline(w, 4*time.Minute)
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	token, err := s.hubsoftToken(ctx, integID, cfg)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "AUTH", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, integrationhubsoft.BuildServicesReport(ctx, cfg, token))
}

// hubsoftDataVendaExport — PROVISÓRIO (só administradores). Todos os serviços, para montar o CSV.
func (s *Server) hubsoftDataVendaExport(w http.ResponseWriter, r *http.Request) {
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
	extendWriteDeadline(w, 6*time.Minute)
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	token, err := s.hubsoftToken(ctx, integID, cfg)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "AUTH", err.Error(), nil)
		return
	}
	rows, msg := integrationhubsoft.ExportDataVendaBase(ctx, cfg, token, r.URL.Query().Get("include_cancelled") == "1")
	if msg != "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "message": msg, "rows": []any{}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "rows": rows})
}

type hubsoftDataVendaPreviewBody struct {
	Rows             []integrationhubsoft.DataVendaInputRow `json:"rows"`
	IncludeCancelled bool                                   `json:"include_cancelled"`
}

// hubsoftDataVendaPreview — PROVISÓRIO (só administradores). Cruza o CSV com a base viva da HubSoft
// por login PPPoE + id + código + nome e devolve o que seria alterado. Não altera nada.
func (s *Server) hubsoftDataVendaPreview(w http.ResponseWriter, r *http.Request) {
	integID, err := s.resolveIntegrationID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "identificador inválido", nil)
		return
	}
	var body hubsoftDataVendaPreviewBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<20)).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_BODY", "corpo inválido", nil)
		return
	}
	if len(body.Rows) > 2000 {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "máximo de 2000 linhas por pré-visualização", nil)
		return
	}
	cfg, err := s.loadHubsoftConfig(r.Context(), integID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "NOT_HUBSOFT", err.Error(), nil)
		return
	}
	extendWriteDeadline(w, 6*time.Minute)
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	token, err := s.hubsoftToken(ctx, integID, cfg)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "AUTH", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, integrationhubsoft.PreviewDataVenda(ctx, cfg, token, body.Rows, body.IncludeCancelled))
}

type hubsoftDataVendaApplyBody struct {
	Rows []integrationhubsoft.DataVendaApplyInput `json:"rows"`
}

// hubsoftDataVendaApply — PROVISÓRIO (só administradores). Aplica um LOTE PEQUENO (≤ 20 linhas por
// chamada; o front encadeia os lotes). Cada linha é revalidada e conferida; ao primeiro sinal de
// inconsistência grave (halt) o servidor interrompe o lote e o front deve parar tudo.
func (s *Server) hubsoftDataVendaApply(w http.ResponseWriter, r *http.Request) {
	integID, err := s.resolveIntegrationID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "identificador inválido", nil)
		return
	}
	var body hubsoftDataVendaApplyBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_BODY", "corpo inválido", nil)
		return
	}
	if len(body.Rows) == 0 || len(body.Rows) > 20 {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "envie de 1 a 20 linhas por chamada", nil)
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
	actor := s.actorFromRequest(r)
	results := make([]integrationhubsoft.DataVendaApplyResult, 0, len(body.Rows))
	halted := false
	for i, row := range body.Rows {
		if i > 0 {
			time.Sleep(350 * time.Millisecond) // folga sob o limite de 20 req/s da HubSoft (cada linha faz ~4 chamadas)
		}
		res := integrationhubsoft.ApplyDataVendaRow(ctx, cfg, token, row)
		results = append(results, res)
		s.appendAuditLog(ctx, "hubsoft_client_service", res.ServiceID, "set_data_venda", actor,
			map[string]any{"data_venda": res.DateBefore},
			map[string]any{"integration_id": integID.String(), "login": res.Login, "data_venda": row.NewDate, "ok": res.OK, "mensagem": res.Message})
		if res.Halt {
			halted = true
			break
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results, "halted": halted})
}

// hubsoftReportTenure — aba Relatório → Tempo de cliente: totais de clientes ativos por faixa de
// data da venda (ver BuildTenureReport). Só números, sem lista.
func (s *Server) hubsoftReportTenure(w http.ResponseWriter, r *http.Request) {
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
	extendWriteDeadline(w, 6*time.Minute)
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	token, err := s.hubsoftToken(ctx, integID, cfg)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "AUTH", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, integrationhubsoft.BuildTenureReport(ctx, cfg, token))
}

// hubsoftReportTenureDetail — modal de uma faixa de "Tempo de cliente": serviços ativos da faixa por
// localidade e por plano (from/to = limites da faixa, devolvidos por report/tenure).
func (s *Server) hubsoftReportTenureDetail(w http.ResponseWriter, r *http.Request) {
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
	extendWriteDeadline(w, 4*time.Minute)
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	token, err := s.hubsoftToken(ctx, integID, cfg)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "AUTH", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, integrationhubsoft.BuildTenureBandDetail(ctx, cfg, token, q.Get("from"), q.Get("to")))
}

// hubsoftPreventiveBase — relatório "Desbloqueio preventivo", fase 1: clientes/serviços não cancelados.
func (s *Server) hubsoftPreventiveBase(w http.ResponseWriter, r *http.Request) {
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
	extendWriteDeadline(w, 6*time.Minute)
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	token, err := s.hubsoftToken(ctx, integID, cfg)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "AUTH", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, integrationhubsoft.BuildPreventiveBase(ctx, cfg, token))
}

// hubsoftPreventiveChunk — fase 2: conta os desbloqueios (preventivos e totais) de um lote de clientes.
func (s *Server) hubsoftPreventiveChunk(w http.ResponseWriter, r *http.Request) {
	integID, err := s.resolveIntegrationID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "identificador inválido", nil)
		return
	}
	var body struct {
		ClientIDs []string `json:"client_ids"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil || len(body.ClientIDs) == 0 || len(body.ClientIDs) > 100 {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "envie de 1 a 100 client_ids", nil)
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
	writeJSON(w, http.StatusOK, integrationhubsoft.BuildPreventiveChunk(ctx, cfg, token, body.ClientIDs))
}

// hubsoftBulkClients — "Consulta em massa" (aba Relatório → Clientes): lista de nomes → ID, nome,
// telefone, serviços com plano, cidade e data da venda. Lotes de até 25 nomes por chamada.
func (s *Server) hubsoftBulkClients(w http.ResponseWriter, r *http.Request) {
	integID, err := s.resolveIntegrationID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "identificador inválido", nil)
		return
	}
	var body struct {
		Names []string `json:"names"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil || len(body.Names) == 0 || len(body.Names) > 25 {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "envie de 1 a 25 nomes por chamada", nil)
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
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "results": integrationhubsoft.BuildBulkClientLookup(ctx, cfg, token, body.Names)})
}

// hubsoftReportBlocked — aba Relatório → Bloqueios: serviços suspensos por débito numa janela
// de datas (ver BuildBlockedServicesReport — a API não expõe a data exacta da suspensão).
func (s *Server) hubsoftReportBlocked(w http.ResponseWriter, r *http.Request) {
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
	extendWriteDeadline(w, 4*time.Minute)
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	token, err := s.hubsoftToken(ctx, integID, cfg)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "AUTH", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, integrationhubsoft.BuildBlockedServicesReport(ctx, cfg, token,
		strings.TrimSpace(q.Get("from")), strings.TrimSpace(q.Get("to")), strings.TrimSpace(q.Get("status"))))
}

// hubsoftServicesTelegramRequest — o frontend já tem o relatório carregado (cacheado até 5min,
// ver ServicesReportSection no React) — em vez de varrer /cliente/todos outra vez só para
// mandar uma mensagem, o pedido vem com os DADOS já calculados (mesmo formato de
// integrationhubsoft.ServicesReport) e a SELECÇÃO do que o utilizador quer mandar. Isto evita
// tanto um round-trip lento à HubSoft quanto o risco de a mensagem divergir do que está no ecrã.
type hubsoftServicesTelegramRequest struct {
	integrationhubsoft.ServicesReport
	// Sections: qualquer combinação de "total", "status", "plan", "locality_totals",
	// "specific_locality" — ver SECTION_OPTIONS em HubsoftReportPage.tsx.
	Sections []string `json:"sections"`
	// SpecificLocalityKey identifica a localidade escolhida quando "specific_locality" está em
	// Sections — mesma chave usada como key de React no frontend: "{city}|{state}".
	SpecificLocalityKey string `json:"specific_locality_key,omitempty"`
}

// hubsoftReportServicesTelegram — botão "Enviar por Telegram" da aba Serviços: o utilizador
// escolhe quais blocos mandar (ver hubsoftServicesTelegramRequest); "tudo" no frontend equivale a
// seleccionar total+status+plan+locality_totals — deliberadamente NUNCA inclui o detalhe de
// plano/status de CADA localidade (só "specific_locality", uma de cada vez, faz isso), senão a
// mensagem explode de tamanho com dezenas de localidades × dezenas de planos.
func (s *Server) hubsoftReportServicesTelegram(w http.ResponseWriter, r *http.Request) {
	integID, err := s.resolveIntegrationID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "identificador inválido", nil)
		return
	}
	if _, err := s.loadHubsoftConfig(r.Context(), integID); err != nil {
		writeErr(w, http.StatusBadRequest, "NOT_HUBSOFT", err.Error(), nil)
		return
	}

	var body hubsoftServicesTelegramRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	sections := map[string]bool{}
	for _, sc := range body.Sections {
		sections[strings.ToLower(strings.TrimSpace(sc))] = true
	}
	if len(sections) == 0 {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "selecione ao menos um item para enviar", nil)
		return
	}

	var sb strings.Builder
	sb.WriteString("📊 HUBSOFT — SERVIÇOS\n")
	wrote := false

	if sections["total"] {
		sb.WriteString(fmt.Sprintf("\nTotal de serviços: %d\n", body.Total))
		wrote = true
	}
	if sections["status"] && len(body.ByStatus) > 0 {
		sb.WriteString("\n📌 Por status\n")
		for _, st := range body.ByStatus {
			sb.WriteString(fmt.Sprintf("  • %s — %d\n", st.Name, st.Count))
		}
		wrote = true
	}
	if sections["plan"] && len(body.ByPlan) > 0 {
		sb.WriteString("\n📦 Por plano\n")
		for i, p := range body.ByPlan {
			if i >= 25 {
				sb.WriteString(fmt.Sprintf("  … e mais %d plano(s)\n", len(body.ByPlan)-25))
				break
			}
			sb.WriteString(fmt.Sprintf("  • %s — %d\n", p.Name, p.Count))
		}
		wrote = true
	}
	if sections["locality_totals"] && len(body.ByLocality) > 0 {
		sb.WriteString("\n📍 Por localidade\n")
		for i, loc := range body.ByLocality {
			if i >= 25 {
				sb.WriteString(fmt.Sprintf("  … e mais %d localidade(s)\n", len(body.ByLocality)-25))
				break
			}
			label := loc.City
			if loc.State != "" {
				label += "/" + loc.State
			}
			sb.WriteString(fmt.Sprintf("  • %s — %d\n", label, loc.Total))
		}
		wrote = true
	}
	if sections["specific_locality"] && strings.TrimSpace(body.SpecificLocalityKey) != "" {
		for _, loc := range body.ByLocality {
			if loc.City+"|"+loc.State != body.SpecificLocalityKey {
				continue
			}
			label := loc.City
			if loc.State != "" {
				label += "/" + loc.State
			}
			sb.WriteString(fmt.Sprintf("\n🏙️ %s — %d serviço(s)\n", label, loc.Total))
			if len(loc.ByStatus) > 0 {
				sb.WriteString("  Status:\n")
				for _, st := range loc.ByStatus {
					sb.WriteString(fmt.Sprintf("    • %s — %d\n", st.Name, st.Count))
				}
			}
			if len(loc.ByPlan) > 0 {
				sb.WriteString("  Plano:\n")
				for _, p := range loc.ByPlan {
					sb.WriteString(fmt.Sprintf("    • %s — %d\n", p.Name, p.Count))
				}
			}
			wrote = true
			break
		}
	}
	if !wrote {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "nada para enviar com os itens seleccionados", nil)
		return
	}
	sb.WriteString("\n—\nNetQuasar · relatório")

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	tgCfg, err := telegramclient.LoadConfig(ctx, s.DB(), "reports")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if !tgCfg.Ready() {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "Telegram de relatórios não configurado (bot_token/chat_id) — configure em Configurações → Telegram.", nil)
		return
	}
	if err := telegramclient.SendMessageChunks(ctx, tgCfg, sb.String()); err != nil {
		writeErr(w, http.StatusBadGateway, "TELEGRAM_SEND_FAILED", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
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
		"Período":    reporttelegram.FormatPeriodBR(rep.From, rep.To),
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
		"Período":       reporttelegram.FormatPeriodBR(rep.From, rep.To),
		"Total":         rep.Total,
		"Finalizadas":   rep.Finished,
		"% finalizadas": fmt.Sprintf("%.1f%%", rep.FinishedPct),
	}
	// Ranking por técnico como UM bloco só (não uma entrada "Técnico: X" por técnico) — o resumo
	// ordena as chaves alfabeticamente (ComposeSystemReport), o que misturava os técnicos com os
	// totais gerais e os reordenava por nome, perdendo o ranking por nº de O.S. fechadas que
	// rep.ByTechnician já traz pronto (reportado como "mensagem não está bem organizada").
	if len(rep.ByTechnician) > 0 {
		var sb strings.Builder
		for i, t := range rep.ByTechnician {
			if i >= 10 {
				sb.WriteString(fmt.Sprintf("\n  … e mais %d técnico(s)", len(rep.ByTechnician)-10))
				break
			}
			sb.WriteString(fmt.Sprintf("\n  %d. %s — %d fechadas de %d (%.1f%% do total)", i+1, t.Technician, t.Finished, t.Total, t.PctOfFinished))
		}
		summary["Técnicos (ranking)"] = sb.String()
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
		"Período":     reporttelegram.FormatPeriodBR(rep.From, rep.To),
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
