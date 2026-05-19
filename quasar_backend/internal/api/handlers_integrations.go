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
	"github.com/netquasar/netquasar/quasar_backend/internal/integrationhttp"
)

type integrationSummary struct {
	ID              uuid.UUID  `json:"id"`
	Name            string     `json:"name"`
	Slug            string     `json:"slug"`
	Description     *string    `json:"description"`
	BaseURL         string     `json:"base_url"`
	Enabled         bool       `json:"enabled"`
	AuthType        string     `json:"auth_type"`
	RequestCount    int64      `json:"request_count"`
	LastTestAt      *time.Time `json:"last_test_at"`
	LastTestOK      *bool      `json:"last_test_ok"`
	LastTestMessage *string    `json:"last_test_message"`
}

type integrationDetail struct {
	integrationSummary
	DefaultHeaders   json.RawMessage `json:"default_headers"`
	Variables        json.RawMessage `json:"variables"`
	AuthConfig       json.RawMessage `json:"auth_config"`
	TimeoutMs        int             `json:"timeout_ms"`
	TLSInsecure      bool            `json:"tls_insecure"`
	PasswordSet      bool            `json:"password_configured"`
	TokenSet         bool            `json:"token_configured"`
	SessionActive    bool            `json:"session_active"`
	Requests         []requestRow    `json:"requests"`
}

type requestRow struct {
	ID              uuid.UUID       `json:"id"`
	IntegrationID   uuid.UUID       `json:"integration_id"`
	Name            string          `json:"name"`
	Description     *string         `json:"description"`
	Method          string          `json:"method"`
	Path            string          `json:"path"`
	PathParams      json.RawMessage `json:"path_params"`
	QueryParams     json.RawMessage `json:"query_params"`
	Headers         json.RawMessage `json:"headers"`
	BodyTemplate    *string         `json:"body_template"`
	BodyType        string          `json:"body_type"`
	ExtractJSONPath *string         `json:"extract_json_path"`
	IsLogin         bool            `json:"is_login"`
	SortOrder       int             `json:"sort_order"`
	Enabled         bool            `json:"enabled"`
	LastRunAt       *time.Time      `json:"last_run_at"`
	LastRunOK       *bool           `json:"last_run_ok"`
	LastRunStatus   *int            `json:"last_run_status"`
	LastRunMessage  *string         `json:"last_run_message"`
}

func (s *Server) listIntegrations(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB().Query(r.Context(), `
		SELECT i.id, i.name, i.slug, i.description, i.base_url, i.enabled, i.auth_type,
			(SELECT COUNT(*) FROM integration_requests ir WHERE ir.integration_id = i.id),
			i.last_test_at, i.last_test_ok, i.last_test_message
		FROM integrations i
		ORDER BY i.name
	`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	var list []integrationSummary
	for rows.Next() {
		var row integrationSummary
		if err := rows.Scan(&row.ID, &row.Name, &row.Slug, &row.Description, &row.BaseURL, &row.Enabled, &row.AuthType,
			&row.RequestCount, &row.LastTestAt, &row.LastTestOK, &row.LastTestMessage); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		list = append(list, row)
	}
	if list == nil {
		list = []integrationSummary{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"integrations": list})
}

func (s *Server) createIntegration(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name        string          `json:"name"`
		Description *string         `json:"description"`
		BaseURL     string          `json:"base_url"`
		AuthType    string          `json:"auth_type"`
		AuthConfig  json.RawMessage `json:"auth_config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	body.BaseURL = strings.TrimSpace(body.BaseURL)
	if body.Name == "" || body.BaseURL == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "name e base_url são obrigatórios", nil)
		return
	}
	authType := strings.TrimSpace(body.AuthType)
	if authType == "" {
		authType = "none"
	}
	slug := integrationhttp.UniqueSlug(integrationhttp.Slugify(body.Name), func(sl string) bool {
		var exists bool
		_ = s.DB().QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM integrations WHERE slug=$1)`, sl).Scan(&exists)
		return exists
	})
	var id uuid.UUID
	err := s.DB().QueryRow(r.Context(), `
		INSERT INTO integrations (name, slug, description, base_url, auth_type, auth_config)
		VALUES ($1, $2, $3, $4, $5, COALESCE($6::jsonb, '{}'::jsonb))
		RETURNING id
	`, body.Name, slug, body.Description, body.BaseURL, authType, body.AuthConfig).Scan(&id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "integration", id.String(), "create", actorFromRequest(r), nil, body)
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "slug": slug})
}

func (s *Server) resolveIntegrationID(ctx context.Context, idOrSlug string) (uuid.UUID, error) {
	if id, err := uuid.Parse(idOrSlug); err == nil {
		return id, nil
	}
	var id uuid.UUID
	err := s.DB().QueryRow(ctx, `SELECT id FROM integrations WHERE slug=$1`, idOrSlug).Scan(&id)
	return id, err
}

func (s *Server) getIntegration(w http.ResponseWriter, r *http.Request) {
	id, err := s.resolveIntegrationID(r.Context(), chi.URLParam(r, "id"))
	if err == pgx.ErrNoRows {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "integração não encontrada", nil)
		return
	}
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "identificador inválido", nil)
		return
	}
	detail, err := s.loadIntegrationDetail(r.Context(), id)
	if err == pgx.ErrNoRows {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "integração não encontrada", nil)
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (s *Server) loadIntegrationDetail(ctx context.Context, id uuid.UUID) (*integrationDetail, error) {
	var d integrationDetail
	var authCfg, defHdr, vars []byte
	var sessionToken *string
	var sessionExp *time.Time
	err := s.DB().QueryRow(ctx, `
		SELECT i.id, i.name, i.slug, i.description, i.base_url, i.enabled, i.auth_type,
			(SELECT COUNT(*) FROM integration_requests ir WHERE ir.integration_id = i.id),
			i.last_test_at, i.last_test_ok, i.last_test_message,
			i.default_headers, i.variables, i.auth_config, i.timeout_ms, i.tls_insecure,
			i.session_token, i.session_expires_at
		FROM integrations i WHERE i.id=$1
	`, id).Scan(
		&d.ID, &d.Name, &d.Slug, &d.Description, &d.BaseURL, &d.Enabled, &d.AuthType,
		&d.RequestCount, &d.LastTestAt, &d.LastTestOK, &d.LastTestMessage,
		&defHdr, &vars, &authCfg, &d.TimeoutMs, &d.TLSInsecure, &sessionToken, &sessionExp,
	)
	if err != nil {
		return nil, err
	}
	d.DefaultHeaders = defHdr
	d.Variables = vars
	ac := integrationhttp.AuthConfigFromJSON(authCfg)
	d.AuthConfig = maskAuthConfigJSON(authCfg, ac)
	d.PasswordSet = ac.Password != "" || ac.APIKey != ""
	d.TokenSet = ac.Token != "" || (sessionToken != nil && *sessionToken != "")
	d.SessionActive = sessionToken != nil && *sessionToken != "" &&
		(sessionExp == nil || sessionExp.After(time.Now()))

	rows, err := s.DB().Query(ctx, `
		SELECT id, integration_id, name, description, method, path, path_params, query_params, headers,
			body_template, body_type, extract_json_path, is_login, sort_order, enabled,
			last_run_at, last_run_ok, last_run_status, last_run_message
		FROM integration_requests WHERE integration_id=$1 ORDER BY sort_order, name
	`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var rr requestRow
		if err := rows.Scan(&rr.ID, &rr.IntegrationID, &rr.Name, &rr.Description, &rr.Method, &rr.Path,
			&rr.PathParams, &rr.QueryParams, &rr.Headers, &rr.BodyTemplate, &rr.BodyType, &rr.ExtractJSONPath,
			&rr.IsLogin, &rr.SortOrder, &rr.Enabled, &rr.LastRunAt, &rr.LastRunOK, &rr.LastRunStatus, &rr.LastRunMessage); err != nil {
			return nil, err
		}
		d.Requests = append(d.Requests, rr)
	}
	if d.Requests == nil {
		d.Requests = []requestRow{}
	}
	return &d, nil
}

func maskAuthConfigJSON(raw []byte, ac integrationhttp.AuthConfig) json.RawMessage {
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		m = map[string]any{}
	}
	if ac.Password != "" {
		m["password"] = ""
		m["password_configured"] = true
	}
	if ac.Token != "" {
		m["token"] = ""
		m["token_configured"] = true
	}
	if ac.APIKey != "" {
		m["api_key"] = ""
		m["api_key_configured"] = true
	}
	out, _ := json.Marshal(m)
	return out
}

func (s *Server) patchIntegration(w http.ResponseWriter, r *http.Request) {
	id, err := s.resolveIntegrationID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "identificador inválido", nil)
		return
	}
	var body map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}

	var name, baseURL, authType, slug string
	var desc *string
	var en bool
	var tms int
	var tls bool
	var defHdr, vars, authCfg []byte

	err = s.DB().QueryRow(r.Context(), `
		SELECT name, description, base_url, enabled, auth_type, default_headers, variables, auth_config, timeout_ms, tls_insecure, slug
		FROM integrations WHERE id=$1
	`, id).Scan(&name, &desc, &baseURL, &en, &authType, &defHdr, &vars, &authCfg, &tms, &tls, &slug)
	if err == pgx.ErrNoRows {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "integração não encontrada", nil)
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}

	if v, ok := body["name"]; ok {
		_ = json.Unmarshal(v, &name)
	}
	if v, ok := body["description"]; ok {
		_ = json.Unmarshal(v, &desc)
	}
	if v, ok := body["base_url"]; ok {
		_ = json.Unmarshal(v, &baseURL)
	}
	if v, ok := body["enabled"]; ok {
		_ = json.Unmarshal(v, &en)
	}
	if v, ok := body["auth_type"]; ok {
		_ = json.Unmarshal(v, &authType)
	}
	if v, ok := body["default_headers"]; ok {
		defHdr = []byte(v)
	}
	if v, ok := body["variables"]; ok {
		vars = []byte(v)
	}
	if v, ok := body["auth_config"]; ok {
		authCfg = mergeAuthConfig(authCfg, []byte(v))
	}
	if v, ok := body["timeout_ms"]; ok {
		_ = json.Unmarshal(v, &tms)
	}
	if v, ok := body["tls_insecure"]; ok {
		_ = json.Unmarshal(v, &tls)
	}

	_, err = s.DB().Exec(r.Context(), `
		UPDATE integrations SET
			name=$2, description=$3, base_url=$4, enabled=$5, auth_type=$6,
			default_headers=COALESCE($7::jsonb, default_headers),
			variables=COALESCE($8::jsonb, variables),
			auth_config=COALESCE($9::jsonb, auth_config),
			timeout_ms=$10, tls_insecure=$11, updated_at=now()
		WHERE id=$1
	`, id, strings.TrimSpace(name), desc, strings.TrimSpace(baseURL), en, authType,
		nullJSON(defHdr), nullJSON(vars), nullJSON(authCfg), tms, tls)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "slug": slug})
}

func mergeAuthConfig(prev, patch []byte) []byte {
	var a, b map[string]any
	_ = json.Unmarshal(prev, &a)
	_ = json.Unmarshal(patch, &b)
	if a == nil {
		a = map[string]any{}
	}
	for k, v := range b {
		if s, ok := v.(string); ok && s == "" {
			if k == "password" || k == "token" || k == "api_key" {
				continue
			}
		}
		a[k] = v
	}
	out, _ := json.Marshal(a)
	return out
}

func nullJSON(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return string(b)
}

func (s *Server) deleteIntegration(w http.ResponseWriter, r *http.Request) {
	id, err := s.resolveIntegrationID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "identificador inválido", nil)
		return
	}
	tag, err := s.DB().Exec(r.Context(), `DELETE FROM integrations WHERE id=$1`, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "integração não encontrada", nil)
		return
	}
	s.appendAuditLog(r.Context(), "integration", id.String(), "delete", actorFromRequest(r), nil, nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) loadIntegrationRunner(ctx context.Context, id uuid.UUID) (integrationhttp.IntegrationConfig, error) {
	var cfg integrationhttp.IntegrationConfig
	var defHdr, vars, authCfg []byte
	var authType string
	var sessionToken *string
	err := s.DB().QueryRow(ctx, `
		SELECT base_url, default_headers, variables, auth_type, auth_config, timeout_ms, tls_insecure, session_token
		FROM integrations WHERE id=$1
	`, id).Scan(&cfg.BaseURL, &defHdr, &vars, &authType, &authCfg, &cfg.TimeoutMs, &cfg.TLSInsecure, &sessionToken)
	if err != nil {
		return cfg, err
	}
	cfg.DefaultHeaders = integrationhttp.ParseHeadersJSON(defHdr)
	cfg.Variables = integrationhttp.ParseVariablesJSON(vars)
	cfg.AuthType = authType
	cfg.AuthConfig = integrationhttp.AuthConfigFromJSON(authCfg)
	if sessionToken != nil {
		cfg.SessionToken = *sessionToken
	}
	return cfg, nil
}

func (s *Server) integrationTest(w http.ResponseWriter, r *http.Request) {
	id, err := s.resolveIntegrationID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "identificador inválido", nil)
		return
	}
	cfg, err := s.loadIntegrationRunner(r.Context(), id)
	if err == pgx.ErrNoRows {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "integração não encontrada", nil)
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	res := integrationhttp.TestConnection(ctx, cfg)
	s.persistIntegrationTest(r.Context(), id, res)
	s.logIntegrationRun(r.Context(), id, nil, "test", res)
	writeJSON(w, http.StatusOK, runResultPayload(res))
}

func (s *Server) persistIntegrationTest(ctx context.Context, id uuid.UUID, res integrationhttp.RunResult) {
	msg := res.ErrorMessage
	if res.OK {
		msg = "Conexão OK"
	}
	_, _ = s.DB().Exec(ctx, `
		UPDATE integrations SET last_test_at=now(), last_test_ok=$2, last_test_message=$3, updated_at=now() WHERE id=$1
	`, id, res.OK, msg)
}

func (s *Server) integrationLogin(w http.ResponseWriter, r *http.Request) {
	id, err := s.resolveIntegrationID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "identificador inválido", nil)
		return
	}
	cfg, err := s.loadIntegrationRunner(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}

	var loginReq *integrationhttp.RequestConfig
	var reqID *uuid.UUID

	// Pedido marcado is_login
	var rr requestRow
	err = s.DB().QueryRow(r.Context(), `
		SELECT id, method, path, path_params, query_params, headers, body_template, body_type, extract_json_path, is_login
		FROM integration_requests WHERE integration_id=$1 AND is_login=true AND enabled=true
		ORDER BY sort_order LIMIT 1
	`, id).Scan(&rr.ID, &rr.Method, &rr.Path, &rr.PathParams, &rr.QueryParams, &rr.Headers, &rr.BodyTemplate, &rr.BodyType, &rr.ExtractJSONPath, &rr.IsLogin)
	if err == nil {
		rc := s.requestRowToConfig(rr)
		loginReq = &rc
		rid := rr.ID
		reqID = &rid
	} else if strings.EqualFold(cfg.AuthType, "login") && cfg.AuthConfig.LoginPath != "" {
		bt := "json"
		body := cfg.AuthConfig.LoginBody
		rc := integrationhttp.RequestConfig{
			Method:       cfg.AuthConfig.LoginMethod,
			Path:         cfg.AuthConfig.LoginPath,
			BodyTemplate: body,
			BodyType:     bt,
		}
		if rc.Method == "" {
			rc.Method = "POST"
		}
		loginReq = &rc
	} else {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "configure auth_type=login ou um pedido marcado como login", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	res := integrationhttp.RunWithLoginRequest(ctx, cfg, *loginReq, true)
	if res.TokenFromLogin != "" {
		_, _ = s.DB().Exec(r.Context(), `
			UPDATE integrations SET session_token=$2, session_expires_at=now() + interval '24 hours', updated_at=now() WHERE id=$1
		`, id, res.TokenFromLogin)
	}
	if reqID != nil {
		s.persistRequestRun(r.Context(), *reqID, res)
	}
	s.logIntegrationRun(r.Context(), id, reqID, "login", res)
	writeJSON(w, http.StatusOK, runResultPayload(res))
}

func (s *Server) requestRowToConfig(rr requestRow) integrationhttp.RequestConfig {
	rc := integrationhttp.RequestConfig{
		Method:          rr.Method,
		Path:            rr.Path,
		PathParams:      integrationhttp.ParsePathParams(rr.PathParams),
		QueryParams:     integrationhttp.ParseQueryParams(rr.QueryParams),
		Headers:         integrationhttp.ParseHeadersJSON(rr.Headers),
		BodyType:        rr.BodyType,
	}
	if rr.BodyTemplate != nil {
		rc.BodyTemplate = *rr.BodyTemplate
	}
	if rr.ExtractJSONPath != nil {
		rc.ExtractJSONPath = *rr.ExtractJSONPath
	}
	return rc
}

func (s *Server) integrationRunRequest(w http.ResponseWriter, r *http.Request) {
	integID, err := s.resolveIntegrationID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "integração inválida", nil)
		return
	}
	reqID, err := uuid.Parse(chi.URLParam(r, "requestId"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "requestId inválido", nil)
		return
	}
	var rr requestRow
	err = s.DB().QueryRow(r.Context(), `
		SELECT id, integration_id, name, description, method, path, path_params, query_params, headers,
			body_template, body_type, extract_json_path, is_login, sort_order, enabled,
			last_run_at, last_run_ok, last_run_status, last_run_message
		FROM integration_requests WHERE id=$1 AND integration_id=$2
	`, reqID, integID).Scan(
		&rr.ID, &rr.IntegrationID, &rr.Name, &rr.Description, &rr.Method, &rr.Path,
		&rr.PathParams, &rr.QueryParams, &rr.Headers, &rr.BodyTemplate, &rr.BodyType, &rr.ExtractJSONPath,
		&rr.IsLogin, &rr.SortOrder, &rr.Enabled, &rr.LastRunAt, &rr.LastRunOK, &rr.LastRunStatus, &rr.LastRunMessage,
	)
	if err == pgx.ErrNoRows {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "pedido não encontrado", nil)
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}

	cfg, err := s.loadIntegrationRunner(r.Context(), integID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	rc := s.requestRowToConfig(rr)
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	res := integrationhttp.RunWithLoginRequest(ctx, cfg, rc, rr.IsLogin)
	if rr.IsLogin && res.TokenFromLogin != "" {
		_, _ = s.DB().Exec(r.Context(), `
			UPDATE integrations SET session_token=$2, session_expires_at=now() + interval '24 hours', updated_at=now() WHERE id=$1
		`, integID, res.TokenFromLogin)
	}
	s.persistRequestRun(r.Context(), reqID, res)
	s.logIntegrationRun(r.Context(), integID, &reqID, "request", res)
	writeJSON(w, http.StatusOK, runResultPayload(res))
}

func (s *Server) integrationRunAll(w http.ResponseWriter, r *http.Request) {
	integID, err := s.resolveIntegrationID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "integração inválida", nil)
		return
	}
	rows, err := s.DB().Query(r.Context(), `
		SELECT id, integration_id, name, description, method, path, path_params, query_params, headers,
			body_template, body_type, extract_json_path, is_login, sort_order, enabled,
			last_run_at, last_run_ok, last_run_status, last_run_message
		FROM integration_requests WHERE integration_id=$1 AND enabled=true ORDER BY sort_order, name
	`, integID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()

	cfg, err := s.loadIntegrationRunner(r.Context(), integID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}

	var results []map[string]any
	for rows.Next() {
		var rr requestRow
		if err := rows.Scan(&rr.ID, &rr.IntegrationID, &rr.Name, &rr.Description, &rr.Method, &rr.Path,
			&rr.PathParams, &rr.QueryParams, &rr.Headers, &rr.BodyTemplate, &rr.BodyType, &rr.ExtractJSONPath,
			&rr.IsLogin, &rr.SortOrder, &rr.Enabled, &rr.LastRunAt, &rr.LastRunOK, &rr.LastRunStatus, &rr.LastRunMessage); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		rc := s.requestRowToConfig(rr)
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		res := integrationhttp.RunWithLoginRequest(ctx, cfg, rc, rr.IsLogin)
		cancel()
		if rr.IsLogin && res.TokenFromLogin != "" {
			cfg.SessionToken = res.TokenFromLogin
			_, _ = s.DB().Exec(r.Context(), `
				UPDATE integrations SET session_token=$2, session_expires_at=now() + interval '24 hours', updated_at=now() WHERE id=$1
			`, integID, res.TokenFromLogin)
		}
		s.persistRequestRun(r.Context(), rr.ID, res)
		rid := rr.ID
		s.logIntegrationRun(r.Context(), integID, &rid, "request", res)
		payload := runResultPayload(res)
		payload["request_id"] = rr.ID
		payload["request_name"] = rr.Name
		results = append(results, payload)
	}
	if results == nil {
		results = []map[string]any{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

func (s *Server) persistRequestRun(ctx context.Context, reqID uuid.UUID, res integrationhttp.RunResult) {
	msg := res.ErrorMessage
	_, _ = s.DB().Exec(ctx, `
		UPDATE integration_requests SET
			last_run_at=now(), last_run_ok=$2, last_run_status=$3, last_run_message=$4, updated_at=now()
		WHERE id=$1
	`, reqID, res.OK, res.StatusCode, msg)
}

func (s *Server) logIntegrationRun(ctx context.Context, integID uuid.UUID, reqID *uuid.UUID, kind string, res integrationhttp.RunResult) {
	_, _ = s.DB().Exec(ctx, `
		INSERT INTO integration_run_logs (integration_id, request_id, run_kind, ok, status_code, latency_ms, request_url, response_preview, error_message)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, integID, reqID, kind, res.OK, res.StatusCode, res.LatencyMS, res.RequestURL, res.ResponsePreview, res.ErrorMessage)
}

func runResultPayload(res integrationhttp.RunResult) map[string]any {
	return map[string]any{
		"ok":               res.OK,
		"status_code":      res.StatusCode,
		"latency_ms":       res.LatencyMS,
		"request_url":      res.RequestURL,
		"request_method":   res.RequestMethod,
		"response_preview": res.ResponsePreview,
		"extracted":        res.Extracted,
		"error":            res.ErrorMessage,
		"token_received":   res.TokenFromLogin != "",
	}
}

func (s *Server) createIntegrationRequest(w http.ResponseWriter, r *http.Request) {
	integID, err := s.resolveIntegrationID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "integração inválida", nil)
		return
	}
	var body struct {
		Name            string          `json:"name"`
		Description     *string         `json:"description"`
		Method          string          `json:"method"`
		Path            string          `json:"path"`
		PathParams      json.RawMessage `json:"path_params"`
		QueryParams     json.RawMessage `json:"query_params"`
		Headers         json.RawMessage `json:"headers"`
		BodyTemplate    *string         `json:"body_template"`
		BodyType        string          `json:"body_type"`
		ExtractJSONPath *string         `json:"extract_json_path"`
		IsLogin         bool            `json:"is_login"`
		SortOrder       int             `json:"sort_order"`
		Enabled         *bool           `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Name) == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "name obrigatório", nil)
		return
	}
	en := true
	if body.Enabled != nil {
		en = *body.Enabled
	}
	method := strings.ToUpper(strings.TrimSpace(body.Method))
	if method == "" {
		method = "GET"
	}
	path := strings.TrimSpace(body.Path)
	if path == "" {
		path = "/"
	}
	bt := strings.TrimSpace(body.BodyType)
	if bt == "" {
		bt = "json"
	}
	var id uuid.UUID
	err = s.DB().QueryRow(r.Context(), `
		INSERT INTO integration_requests (
			integration_id, name, description, method, path, path_params, query_params, headers,
			body_template, body_type, extract_json_path, is_login, sort_order, enabled
		) VALUES ($1,$2,$3,$4,$5,COALESCE($6::jsonb,'[]'),COALESCE($7::jsonb,'[]'),COALESCE($8::jsonb,'{}'),
			$9,$10,$11,$12,$13,$14)
		RETURNING id
	`, integID, body.Name, body.Description, method, path, body.PathParams, body.QueryParams, body.Headers,
		body.BodyTemplate, bt, body.ExtractJSONPath, body.IsLogin, body.SortOrder, en).Scan(&id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (s *Server) patchIntegrationRequest(w http.ResponseWriter, r *http.Request) {
	integID, err := s.resolveIntegrationID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "integração inválida", nil)
		return
	}
	reqID, err := uuid.Parse(chi.URLParam(r, "requestId"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "requestId inválido", nil)
		return
	}
	var body map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}

	var name, method, path, bt string
	var desc *string
	var bodyTpl *string
	var extract *string
	var isLogin bool
	var sortOrder int
	var enabled bool
	var pathParams, queryParams, headers []byte

	err = s.DB().QueryRow(r.Context(), `
		SELECT name, description, method, path, path_params, query_params, headers, body_template, body_type,
			extract_json_path, is_login, sort_order, enabled
		FROM integration_requests WHERE id=$1 AND integration_id=$2
	`, reqID, integID).Scan(&name, &desc, &method, &path, &pathParams, &queryParams, &headers, &bodyTpl, &bt,
		&extract, &isLogin, &sortOrder, &enabled)
	if err == pgx.ErrNoRows {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "pedido não encontrado", nil)
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}

	if v, ok := body["name"]; ok {
		_ = json.Unmarshal(v, &name)
	}
	if v, ok := body["description"]; ok {
		_ = json.Unmarshal(v, &desc)
	}
	if v, ok := body["method"]; ok {
		_ = json.Unmarshal(v, &method)
	}
	if v, ok := body["path"]; ok {
		_ = json.Unmarshal(v, &path)
	}
	if v, ok := body["path_params"]; ok {
		pathParams = []byte(v)
	}
	if v, ok := body["query_params"]; ok {
		queryParams = []byte(v)
	}
	if v, ok := body["headers"]; ok {
		headers = []byte(v)
	}
	if v, ok := body["body_template"]; ok {
		_ = json.Unmarshal(v, &bodyTpl)
	}
	if v, ok := body["body_type"]; ok {
		_ = json.Unmarshal(v, &bt)
	}
	if v, ok := body["extract_json_path"]; ok {
		_ = json.Unmarshal(v, &extract)
	}
	if v, ok := body["is_login"]; ok {
		_ = json.Unmarshal(v, &isLogin)
	}
	if v, ok := body["sort_order"]; ok {
		_ = json.Unmarshal(v, &sortOrder)
	}
	if v, ok := body["enabled"]; ok {
		_ = json.Unmarshal(v, &enabled)
	}

	_, err = s.DB().Exec(r.Context(), `
		UPDATE integration_requests SET
			name=$3, description=$4, method=$5, path=$6,
			path_params=COALESCE($7::jsonb, path_params),
			query_params=COALESCE($8::jsonb, query_params),
			headers=COALESCE($9::jsonb, headers),
			body_template=$10, body_type=$11, extract_json_path=$12,
			is_login=$13, sort_order=$14, enabled=$15, updated_at=now()
		WHERE id=$1 AND integration_id=$2
	`, reqID, integID, name, desc, strings.ToUpper(method), path,
		nullJSON(pathParams), nullJSON(queryParams), nullJSON(headers),
		bodyTpl, bt, extract, isLogin, sortOrder, enabled)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) deleteIntegrationRequest(w http.ResponseWriter, r *http.Request) {
	integID, err := s.resolveIntegrationID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "integração inválida", nil)
		return
	}
	reqID, err := uuid.Parse(chi.URLParam(r, "requestId"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "requestId inválido", nil)
		return
	}
	tag, err := s.DB().Exec(r.Context(), `DELETE FROM integration_requests WHERE id=$1 AND integration_id=$2`, reqID, integID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "pedido não encontrado", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) listIntegrationLogs(w http.ResponseWriter, r *http.Request) {
	integID, err := s.resolveIntegrationID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "integração inválida", nil)
		return
	}
	rows, err := s.DB().Query(r.Context(), `
		SELECT id, request_id, run_kind, ok, status_code, latency_ms, request_url, error_message, created_at
		FROM integration_run_logs WHERE integration_id=$1 ORDER BY created_at DESC LIMIT 50
	`, integID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	type logRow struct {
		ID           uuid.UUID  `json:"id"`
		RequestID    *uuid.UUID `json:"request_id"`
		RunKind      string     `json:"run_kind"`
		OK           bool       `json:"ok"`
		StatusCode   *int       `json:"status_code"`
		LatencyMS    *int       `json:"latency_ms"`
		RequestURL   *string    `json:"request_url"`
		ErrorMessage *string    `json:"error_message"`
		CreatedAt    time.Time  `json:"created_at"`
	}
	var list []logRow
	for rows.Next() {
		var row logRow
		if err := rows.Scan(&row.ID, &row.RequestID, &row.RunKind, &row.OK, &row.StatusCode, &row.LatencyMS, &row.RequestURL, &row.ErrorMessage, &row.CreatedAt); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		list = append(list, row)
	}
	if list == nil {
		list = []logRow{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"logs": list})
}
