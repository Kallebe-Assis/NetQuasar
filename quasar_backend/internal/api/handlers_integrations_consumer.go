package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/netquasar/netquasar/quasar_backend/internal/integrationconsumer"
	"github.com/netquasar/netquasar/quasar_backend/internal/integrationhttp"
)

func (s *Server) getIntegrationConsumerMeta(w http.ResponseWriter, r *http.Request) {
	id, err := s.resolveIntegrationID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "identificador inválido", nil)
		return
	}
	var name string
	var enabled bool
	var consumerCfg []byte
	err = s.DB().QueryRow(r.Context(), `
		SELECT name, enabled, consumer_config FROM integrations WHERE id=$1
	`, id).Scan(&name, &enabled, &consumerCfg)
	if err == pgx.ErrNoRows {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "integração não encontrada", nil)
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	cc := integrationconsumer.ConfigFromJSON(consumerCfg)
	reqID, reqName, _ := s.resolveClientSearchRequest(r.Context(), id, cc)
	profile := integrationconsumer.ProviderGeneric
	var baseURL string
	if reqID != uuid.Nil {
		var rr requestRow
		if err := s.DB().QueryRow(r.Context(), `
			SELECT method, path, headers, body_template, body_type
			FROM integration_requests WHERE id=$1 AND integration_id=$2
		`, reqID, id).Scan(&rr.Method, &rr.Path, &rr.Headers, &rr.BodyTemplate, &rr.BodyType); err == nil {
			rc := s.requestRowToConfig(rr)
			_ = s.DB().QueryRow(r.Context(), `SELECT base_url FROM integrations WHERE id=$1`, id).Scan(&baseURL)
			profile = integrationconsumer.DetectClientSearchProfile(cc.ClientSearch.Provider, rc, baseURL)
		}
	}
	attReqID, attReqName, _ := s.resolveClientAttendanceRequest(r.Context(), id, cc)
	woReqID, woReqName, _ := s.resolveClientWorkOrderRequest(r.Context(), id, cc)
	writeJSON(w, http.StatusOK, map[string]any{
		"integration_name":               name,
		"integration_enabled":            enabled,
		"client_search_enabled":          cc.ClientSearch.Enabled,
		"client_search_request_id":       reqID,
		"client_search_request_name":     reqName,
		"client_search_provider":         profile,
		"busca_options":                  integrationconsumer.BuscaOptionsResolved(cc.ClientSearch, profile),
		"client_attendance_enabled":      cc.ClientAttendance.Enabled,
		"client_attendance_request_id":   attReqID,
		"client_attendance_request_name": attReqName,
		"busca_atendimento_options":      integrationconsumer.BuscaAtendimentoOptions(),
		"client_work_order_enabled":      cc.ClientWorkOrder.Enabled,
		"client_work_order_request_id":   woReqID,
		"client_work_order_request_name": woReqName,
		"busca_ordem_servico_options":    integrationconsumer.BuscaOrdemServicoOptions(),
	})
}

func (s *Server) integrationConsumerClientSearch(w http.ResponseWriter, r *http.Request) {
	integID, err := s.resolveIntegrationID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "identificador inválido", nil)
		return
	}
	var body struct {
		Busca      string `json:"busca"`
		TermoBusca string `json:"termo_busca"`
		Detailed   bool   `json:"detailed"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	busca := strings.TrimSpace(body.Busca)
	if busca == "" {
		busca = "cpf_cnpj"
	}
	termo := strings.TrimSpace(body.TermoBusca)
	if termo == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "termo_busca é obrigatório", nil)
		return
	}

	var consumerCfg []byte
	var integEnabled bool
	err = s.DB().QueryRow(r.Context(), `
		SELECT enabled, consumer_config FROM integrations WHERE id=$1
	`, integID).Scan(&integEnabled, &consumerCfg)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if !integEnabled {
		writeErr(w, http.StatusBadRequest, "DISABLED", "integração inativa", nil)
		return
	}
	cc := integrationconsumer.ConfigFromJSON(consumerCfg)
	if !cc.ClientSearch.Enabled {
		writeErr(w, http.StatusBadRequest, "NOT_CONFIGURED", "consulta de cliente não ativa nesta integração", nil)
		return
	}
	reqID, _, err := s.resolveClientSearchRequest(r.Context(), integID, cc)
	if err != nil || reqID == uuid.Nil {
		writeErr(w, http.StatusBadRequest, "NOT_CONFIGURED", "requisição HTTP de consulta de cliente não configurado", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()

	cfg, err := s.loadIntegrationRunner(ctx, integID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if err := s.ensureIntegrationSession(ctx, integID, cfg); err != nil {
		writeErr(w, http.StatusBadGateway, "AUTH", err.Error(), nil)
		return
	}
	cfg, _ = s.loadIntegrationRunner(ctx, integID)

	var rr requestRow
	err = s.DB().QueryRow(ctx, `
		SELECT id, method, path, path_params, query_params, headers, body_template, body_type, extract_json_path, is_login
		FROM integration_requests WHERE id=$1 AND integration_id=$2 AND enabled=true
	`, reqID, integID).Scan(&rr.ID, &rr.Method, &rr.Path, &rr.PathParams, &rr.QueryParams, &rr.Headers,
		&rr.BodyTemplate, &rr.BodyType, &rr.ExtractJSONPath, &rr.IsLogin)
	if err != nil {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "requisição de consulta não encontrada", nil)
		return
	}

	rc := s.requestRowToConfig(rr)
	profile := integrationconsumer.DetectClientSearchProfile(cc.ClientSearch.Provider, rc, cfg.BaseURL)

	cfgExec := cfg
	if cfgExec.Variables == nil {
		cfgExec.Variables = map[string]string{}
	} else {
		vars := make(map[string]string, len(cfgExec.Variables)+4)
		for k, v := range cfgExec.Variables {
			vars[k] = v
		}
		cfgExec.Variables = vars
	}
	for k, v := range integrationconsumer.ClientSearchVariables(busca, termo) {
		cfgExec.Variables[k] = v
	}

	execute := func(rc integrationhttp.RequestConfig) integrationhttp.RunResult {
		return integrationhttp.RunWithLoginRequest(ctx, cfgExec, rc, false)
	}

	var res integrationhttp.RunResult
	var parsed integrationconsumer.SearchResult
	if profile == integrationconsumer.ProviderIXC && busca == "cpf_cnpj" && cc.ClientSearch.CpfMultiAttemptEnabled() {
		res, parsed = integrationconsumer.RunIXCClientSearchWithAttempts(rc, busca, termo, body.Detailed, cc.ClientSearch, execute)
	} else {
		rc = integrationconsumer.ApplyClientSearchContext(rc, profile, busca, termo, body.Detailed, cc.ClientSearch)
		res = execute(rc)
		rawBody := integrationconsumer.ResponseBodyForParse([]byte(res.ResponsePreview))
		parsed, profile = integrationconsumer.ParseClientSearchBest(rawBody, profile)
		if busca == "cpf_cnpj" {
			parsed.Clients = integrationconsumer.FilterClientsByDocument(parsed.Clients, termo)
			if parsed.OK && len(parsed.Clients) == 0 {
				parsed.Message = "Nenhum cliente encontrado para este CPF/CNPJ."
			}
		}
	}
	s.persistRequestRun(ctx, reqID, res)
	s.logIntegrationRun(ctx, integID, &reqID, "request", res)
	rawBody := integrationconsumer.ResponseBodyForParse([]byte(res.ResponsePreview))
	if !parsed.OK && strings.Contains(parsed.Message, "JSON") {
		if hint := integrationconsumer.NonJSONResponseHint(rawBody, res.StatusCode); hint != "" {
			parsed.Message = hint
		}
	}
	if !parsed.OK && !res.OK && res.ErrorMessage != "" {
		if parsed.Message == "" || strings.Contains(parsed.Message, "JSON") {
			parsed.Message = res.ErrorMessage
		}
	}
	if !res.OK && parsed.OK {
		parsed.OK = false
		if parsed.Message == "" || parsed.Message == "Nenhum cliente encontrado para este termo." {
			parsed.Message = res.ErrorMessage
		}
	}
	if !parsed.OK && parsed.Message == "" {
		parsed.Message = res.ErrorMessage
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":               parsed.OK && res.OK,
		"message":          parsed.Message,
		"clients":          parsed.Clients,
		"raw_status":       parsed.RawStatus,
		"provider":         profile,
		"status_code":      res.StatusCode,
		"latency_ms":       res.LatencyMS,
		"request_url":      res.RequestURL,
		"request_method":   res.RequestMethod,
		"response_preview": res.ResponsePreview,
	})
}

func (s *Server) integrationConsumerClientAttendance(w http.ResponseWriter, r *http.Request) {
	integID, err := s.resolveIntegrationID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "identificador inválido", nil)
		return
	}
	var body struct {
		Busca           string `json:"busca"`
		TermoBusca      string `json:"termo_busca"`
		ApenasPendente  string `json:"apenas_pendente"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	busca := strings.TrimSpace(body.Busca)
	if busca == "" {
		busca = "codigo_cliente"
	}
	termo := strings.TrimSpace(body.TermoBusca)
	if termo == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "termo_busca é obrigatório", nil)
		return
	}
	apenas := strings.TrimSpace(strings.ToLower(body.ApenasPendente))
	if apenas != "sim" && apenas != "nao" {
		apenas = "nao"
	}

	var consumerCfg []byte
	var integEnabled bool
	err = s.DB().QueryRow(r.Context(), `
		SELECT enabled, consumer_config FROM integrations WHERE id=$1
	`, integID).Scan(&integEnabled, &consumerCfg)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if !integEnabled {
		writeErr(w, http.StatusBadRequest, "DISABLED", "integração inativa", nil)
		return
	}
	cc := integrationconsumer.ConfigFromJSON(consumerCfg)
	if !cc.ClientAttendance.Enabled {
		writeErr(w, http.StatusBadRequest, "NOT_CONFIGURED", "consulta de atendimentos não ativa nesta integração", nil)
		return
	}
	reqID, _, err := s.resolveClientAttendanceRequest(r.Context(), integID, cc)
	if err != nil || reqID == uuid.Nil {
		writeErr(w, http.StatusBadRequest, "NOT_CONFIGURED", "requisição HTTP de atendimentos não configurada", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()

	cfg, err := s.loadIntegrationRunner(ctx, integID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if err := s.ensureIntegrationSession(ctx, integID, cfg); err != nil {
		writeErr(w, http.StatusBadGateway, "AUTH", err.Error(), nil)
		return
	}
	cfg, _ = s.loadIntegrationRunner(ctx, integID)

	var rr requestRow
	err = s.DB().QueryRow(ctx, `
		SELECT id, method, path, path_params, query_params, headers, body_template, body_type, extract_json_path, is_login
		FROM integration_requests WHERE id=$1 AND integration_id=$2 AND enabled=true
	`, reqID, integID).Scan(&rr.ID, &rr.Method, &rr.Path, &rr.PathParams, &rr.QueryParams, &rr.Headers,
		&rr.BodyTemplate, &rr.BodyType, &rr.ExtractJSONPath, &rr.IsLogin)
	if err != nil {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "requisição de atendimentos não encontrada", nil)
		return
	}

	rc := s.requestRowToConfig(rr)
	profile := integrationconsumer.DetectAttendanceProfile(cc.ClientAttendance.Provider, rc, cfg.BaseURL)

	execute := func(rc integrationhttp.RequestConfig) integrationhttp.RunResult {
		runCfg := cfg
		vars := map[string]string{
			"busca":       busca,
			"termo_busca": termo,
			"query":       termo,
		}
		if runCfg.Variables == nil {
			runCfg.Variables = vars
		} else {
			runCfg.Variables = make(map[string]string, len(runCfg.Variables)+len(vars))
			for k, v := range cfg.Variables {
				runCfg.Variables[k] = v
			}
			for k, v := range vars {
				runCfg.Variables[k] = v
			}
		}
		return integrationhttp.RunWithLoginRequest(ctx, runCfg, rc, false)
	}

	var res integrationhttp.RunResult
	var parsed integrationconsumer.AttendanceResult
	if profile == integrationconsumer.ProviderIXC || integrationconsumer.LooksLikeIXCAttendanceRequest(rc) {
		res, parsed = integrationconsumer.RunIXCAttendanceWithAttempts(rc, busca, termo, cc.ClientAttendance, cc.ClientSearch, execute)
	} else {
		rc = integrationconsumer.ApplyAttendanceContext(rc, profile, busca, termo, apenas, cc.ClientAttendance, cc.ClientSearch)
		res = execute(rc)
		rawBody := integrationconsumer.ResponseBodyForParse([]byte(res.ResponsePreview))
		parsed = integrationconsumer.ParseClientAttendance(rawBody, profile)
	}
	s.persistRequestRun(ctx, reqID, res)
	s.logIntegrationRun(ctx, integID, &reqID, "request", res)
	if !res.OK && parsed.OK {
		parsed.OK = false
		if parsed.Message == "" || parsed.Message == "Nenhum atendimento encontrado." {
			parsed.Message = res.ErrorMessage
		}
	}
	if !parsed.OK && parsed.Message == "" {
		parsed.Message = res.ErrorMessage
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":             parsed.OK && res.OK,
		"message":        parsed.Message,
		"items":          parsed.Items,
		"raw_status":     parsed.RawStatus,
		"status_code":    res.StatusCode,
		"latency_ms":     res.LatencyMS,
		"request_url":    res.RequestURL,
		"request_method": res.RequestMethod,
		"termo_busca":    termo,
		"busca":          busca,
	})
}

func (s *Server) integrationConsumerClientWorkOrder(w http.ResponseWriter, r *http.Request) {
	integID, err := s.resolveIntegrationID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "identificador inválido", nil)
		return
	}
	var body struct {
		Busca      string `json:"busca"`
		TermoBusca string `json:"termo_busca"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	busca := strings.TrimSpace(body.Busca)
	if busca == "" {
		busca = "codigo_cliente"
	}
	termo := strings.TrimSpace(body.TermoBusca)
	if termo == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "termo_busca é obrigatório", nil)
		return
	}

	var consumerCfg []byte
	var integEnabled bool
	err = s.DB().QueryRow(r.Context(), `
		SELECT enabled, consumer_config FROM integrations WHERE id=$1
	`, integID).Scan(&integEnabled, &consumerCfg)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if !integEnabled {
		writeErr(w, http.StatusBadRequest, "DISABLED", "integração inativa", nil)
		return
	}
	cc := integrationconsumer.ConfigFromJSON(consumerCfg)
	if !cc.ClientWorkOrder.Enabled {
		writeErr(w, http.StatusBadRequest, "NOT_CONFIGURED", "consulta de ordens de serviço não ativa nesta integração", nil)
		return
	}
	reqID, _, err := s.resolveClientWorkOrderRequest(r.Context(), integID, cc)
	if err != nil || reqID == uuid.Nil {
		writeErr(w, http.StatusBadRequest, "NOT_CONFIGURED", "requisição HTTP de ordens de serviço não configurada", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()

	cfg, err := s.loadIntegrationRunner(ctx, integID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if err := s.ensureIntegrationSession(ctx, integID, cfg); err != nil {
		writeErr(w, http.StatusBadGateway, "AUTH", err.Error(), nil)
		return
	}
	cfg, _ = s.loadIntegrationRunner(ctx, integID)

	var rr requestRow
	err = s.DB().QueryRow(ctx, `
		SELECT id, method, path, path_params, query_params, headers, body_template, body_type, extract_json_path, is_login
		FROM integration_requests WHERE id=$1 AND integration_id=$2 AND enabled=true
	`, reqID, integID).Scan(&rr.ID, &rr.Method, &rr.Path, &rr.PathParams, &rr.QueryParams, &rr.Headers,
		&rr.BodyTemplate, &rr.BodyType, &rr.ExtractJSONPath, &rr.IsLogin)
	if err != nil {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "requisição de ordens de serviço não encontrada", nil)
		return
	}

	rc := s.requestRowToConfig(rr)
	profile := integrationconsumer.DetectWorkOrderProfile(cc.ClientWorkOrder.Provider, rc, cfg.BaseURL)

	execute := func(rc integrationhttp.RequestConfig) integrationhttp.RunResult {
		runCfg := cfg
		vars := map[string]string{
			"busca":       busca,
			"termo_busca": termo,
			"query":       termo,
		}
		if runCfg.Variables == nil {
			runCfg.Variables = vars
		} else {
			runCfg.Variables = make(map[string]string, len(runCfg.Variables)+len(vars))
			for k, v := range cfg.Variables {
				runCfg.Variables[k] = v
			}
			for k, v := range vars {
				runCfg.Variables[k] = v
			}
		}
		return integrationhttp.RunWithLoginRequest(ctx, runCfg, rc, false)
	}

	var res integrationhttp.RunResult
	var parsed integrationconsumer.WorkOrderResult
	if profile == integrationconsumer.ProviderIXC || integrationconsumer.LooksLikeIXCWorkOrderRequest(rc) {
		res, parsed = integrationconsumer.RunIXCWorkOrderWithAttempts(rc, busca, termo, cc.ClientWorkOrder, cc.ClientSearch, execute)
	} else {
		overrides := integrationconsumer.HubsoftWorkOrderQueryOverrides(busca, termo)
		rc = integrationconsumer.ApplyQueryOverrides(rc, overrides)
		res = execute(rc)
		rawBody := integrationconsumer.ResponseBodyForParse([]byte(res.ResponsePreview))
		parsed = integrationconsumer.ParseClientWorkOrder(rawBody, profile)
	}
	s.persistRequestRun(ctx, reqID, res)
	s.logIntegrationRun(ctx, integID, &reqID, "request", res)
	if !res.OK && parsed.OK {
		parsed.OK = false
		if parsed.Message == "" || parsed.Message == "Nenhuma ordem de serviço encontrada." {
			parsed.Message = res.ErrorMessage
		}
	}
	if !parsed.OK && parsed.Message == "" {
		parsed.Message = res.ErrorMessage
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":             parsed.OK && res.OK,
		"message":        parsed.Message,
		"items":          parsed.Items,
		"raw_status":     parsed.RawStatus,
		"status_code":    res.StatusCode,
		"latency_ms":     res.LatencyMS,
		"request_url":    res.RequestURL,
		"request_method": res.RequestMethod,
		"termo_busca":    termo,
		"busca":          busca,
	})
}

func (s *Server) resolveClientSearchRequest(ctx context.Context, integID uuid.UUID, cc integrationconsumer.Config) (uuid.UUID, string, error) {
	if cc.ClientSearch.RequestID != "" {
		if rid, err := uuid.Parse(cc.ClientSearch.RequestID); err == nil {
			var name string
			err := s.DB().QueryRow(ctx, `
				SELECT name FROM integration_requests WHERE id=$1 AND integration_id=$2
			`, rid, integID).Scan(&name)
			if err == nil {
				return rid, name, nil
			}
		}
	}
	var rid uuid.UUID
	var name string
	err := s.DB().QueryRow(ctx, `
		SELECT id, name FROM integration_requests
		WHERE integration_id=$1 AND enabled=true AND COALESCE(is_login, false)=false
			AND (
				(path ILIKE '%integracao/cliente%' AND path NOT ILIKE '%atendimento%' AND path NOT ILIKE '%ordem_servico%')
				OR (path ILIKE '%/cliente%' AND path NOT ILIKE '%integracao%')
			)
		ORDER BY
			CASE WHEN path ILIKE '%integracao%' THEN 1 ELSE 0 END,
			CASE WHEN UPPER(method)='POST' THEN 0 ELSE 1 END,
			sort_order, name
		LIMIT 1
	`, integID).Scan(&rid, &name)
	return rid, name, err
}

func (s *Server) resolveClientAttendanceRequest(ctx context.Context, integID uuid.UUID, cc integrationconsumer.Config) (uuid.UUID, string, error) {
	if cc.ClientAttendance.RequestID != "" {
		if rid, err := uuid.Parse(cc.ClientAttendance.RequestID); err == nil {
			var name string
			err := s.DB().QueryRow(ctx, `
				SELECT name FROM integration_requests WHERE id=$1 AND integration_id=$2
			`, rid, integID).Scan(&name)
			if err == nil {
				return rid, name, nil
			}
		}
	}
	var rid uuid.UUID
	var name string
	err := s.DB().QueryRow(ctx, `
		SELECT id, name FROM integration_requests
		WHERE integration_id=$1 AND enabled=true AND COALESCE(is_login, false)=false
			AND (
				(UPPER(method)='GET' AND path ILIKE '%integracao/cliente/atendimento%')
				OR (UPPER(method)='POST' AND path ILIKE '%su_ticket%')
			)
		ORDER BY
			CASE WHEN path ILIKE '%integracao%' THEN 1 ELSE 0 END,
			CASE WHEN UPPER(method)='POST' THEN 0 ELSE 1 END,
			sort_order, name
		LIMIT 1
	`, integID).Scan(&rid, &name)
	return rid, name, err
}

func (s *Server) resolveClientWorkOrderRequest(ctx context.Context, integID uuid.UUID, cc integrationconsumer.Config) (uuid.UUID, string, error) {
	if cc.ClientWorkOrder.RequestID != "" {
		if rid, err := uuid.Parse(cc.ClientWorkOrder.RequestID); err == nil {
			var name string
			err := s.DB().QueryRow(ctx, `
				SELECT name FROM integration_requests WHERE id=$1 AND integration_id=$2
			`, rid, integID).Scan(&name)
			if err == nil {
				return rid, name, nil
			}
		}
	}
	var rid uuid.UUID
	var name string
	err := s.DB().QueryRow(ctx, `
		SELECT id, name FROM integration_requests
		WHERE integration_id=$1 AND enabled=true
			AND UPPER(method)='GET'
			AND path ILIKE '%integracao/cliente/ordem_servico%'
		ORDER BY sort_order, name LIMIT 1
	`, integID).Scan(&rid, &name)
	return rid, name, err
}

// ensureIntegrationSession obtém token OAuth/login se necessário; Bearer/Basic/API key usam credencial salva.
func (s *Server) ensureIntegrationSession(ctx context.Context, integID uuid.UUID, cfg integrationhttp.IntegrationConfig) error {
	auth := strings.ToLower(strings.TrimSpace(cfg.AuthType))
	if auth == "none" || auth == "" {
		return nil
	}
	if !integrationhttp.AuthUsesSessionLogin(cfg.AuthType) {
		if msg := integrationhttp.ValidateStaticAuthConfig(cfg); msg != "" {
			return errString(msg)
		}
		return nil
	}
	var token string
	var sessionValid bool
	err := s.DB().QueryRow(ctx, `
		SELECT COALESCE(session_token, ''),
			(session_expires_at IS NULL OR session_expires_at > now())
		FROM integrations WHERE id=$1
	`, integID).Scan(&token, &sessionValid)
	if err != nil {
		return err
	}
	if strings.TrimSpace(token) != "" && sessionValid {
		return nil
	}
	res := s.runIntegrationLogin(ctx, integID, cfg)
	if res.TokenFromLogin == "" {
		msg := res.ErrorMessage
		if msg == "" {
			msg = "falha ao obter token de autenticação"
		}
		return errString(msg)
	}
	return nil
}

type errString string

func (e errString) Error() string { return string(e) }

func (s *Server) runIntegrationLogin(ctx context.Context, integID uuid.UUID, cfg integrationhttp.IntegrationConfig) integrationhttp.RunResult {
	var loginReq *integrationhttp.RequestConfig
	var reqID *uuid.UUID

	var rr requestRow
	err := s.DB().QueryRow(ctx, `
		SELECT id, method, path, path_params, query_params, headers, body_template, body_type, extract_json_path, is_login
		FROM integration_requests WHERE integration_id=$1 AND is_login=true AND enabled=true
		ORDER BY sort_order LIMIT 1
	`, integID).Scan(&rr.ID, &rr.Method, &rr.Path, &rr.PathParams, &rr.QueryParams, &rr.Headers, &rr.BodyTemplate, &rr.BodyType, &rr.ExtractJSONPath, &rr.IsLogin)
	if err == nil {
		rc := s.requestRowToConfig(rr)
		loginReq = &rc
		rid := rr.ID
		reqID = &rid
	} else if strings.EqualFold(cfg.AuthType, "oauth2_password") && cfg.AuthConfig.LoginPath != "" {
		ac := cfg.AuthConfig
		if ac.TokenJSONPath == "" {
			ac.TokenJSONPath = "access_token"
			cfg.AuthConfig = ac
		}
		body, bt := integrationhttp.OAuth2PasswordBody(cfg.AuthConfig)
		rc := integrationhttp.RequestConfig{
			Method:             cfg.AuthConfig.LoginMethod,
			Path:               cfg.AuthConfig.LoginPath,
			BodyTemplate:       body,
			BodyType:           bt,
			ExtractJSONPath:    ac.TokenJSONPath,
			OmitDefaultHeaders: true,
		}
		if rc.Method == "" {
			rc.Method = "POST"
		}
		loginReq = &rc
	} else if strings.EqualFold(cfg.AuthType, "login") && cfg.AuthConfig.LoginPath != "" {
		bt := strings.ToLower(strings.TrimSpace(cfg.AuthConfig.LoginBodyType))
		if bt != "form" && bt != "json" {
			bt = "json"
		}
		rc := integrationhttp.RequestConfig{
			Method:             cfg.AuthConfig.LoginMethod,
			Path:               cfg.AuthConfig.LoginPath,
			BodyTemplate:       cfg.AuthConfig.LoginBody,
			BodyType:           bt,
			OmitDefaultHeaders: true,
		}
		if rc.Method == "" {
			rc.Method = "POST"
		}
		loginReq = &rc
	} else {
		return integrationhttp.RunResult{ErrorMessage: "autenticação não configurada"}
	}

	res := integrationhttp.RunWithLoginRequest(ctx, cfg, *loginReq, true)
	persistIntegrationSessionToken(ctx, s.DB(), integID, res)
	if reqID != nil {
		s.persistRequestRun(ctx, *reqID, res)
	}
	s.logIntegrationRun(ctx, integID, reqID, "login", res)
	return res
}
