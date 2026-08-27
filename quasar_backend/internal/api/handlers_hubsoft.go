package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/netquasar/netquasar/quasar_backend/internal/integrationhttp"
	"github.com/netquasar/netquasar/quasar_backend/internal/integrationhubsoft"
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
		result, serr := integrationhubsoft.SearchByIPOrMAC(ctx, cfg, token, termo, busca)
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
	// Amostra rápida (scanClientSampleFast) — bem mais curta que o Dashboard.
	extendWriteDeadline(w, 2*time.Minute)
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	token, err := s.hubsoftToken(ctx, integID, cfg)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "AUTH", err.Error(), nil)
		return
	}
	result := integrationhubsoft.BuildRecentActivity(ctx, cfg, token, 20)
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
	// Amostra rápida (scanClientSampleFast) — bem mais curta que o Dashboard.
	extendWriteDeadline(w, 2*time.Minute)
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	token, err := s.hubsoftToken(ctx, integID, cfg)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "AUTH", err.Error(), nil)
		return
	}
	result := integrationhubsoft.BuildFinancialSummary(ctx, cfg, token)
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) hubsoftBuscaOptions(w http.ResponseWriter, r *http.Request) {
	opts := integrationhubsoft.BuscaOptions()
	opts = append(opts, integrationhubsoft.BuscaOption{Value: "ipv4", Label: "IPv4"}, integrationhubsoft.BuscaOption{Value: "mac", Label: "MAC"})
	writeJSON(w, http.StatusOK, map[string]any{"busca_options": opts})
}
