package integrationhttp

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const maxResponsePreview = 64 * 1024

var pathParamRe = regexp.MustCompile(`\{([a-zA-Z_][a-zA-Z0-9_]*)\}`)

type ParamKV struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Enabled *bool  `json:"enabled"`
}

func (p ParamKV) IsEnabled() bool {
	if p.Enabled == nil {
		return true
	}
	return *p.Enabled
}

type PathParam struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Source string `json:"source"` // static | variable
}

type AuthConfig struct {
	Token         string `json:"token"`
	Username      string `json:"username"`
	Password      string `json:"password"`
	ClientID      string `json:"client_id"`
	ClientSecret  string `json:"client_secret"`
	GrantType     string `json:"grant_type"`
	HeaderName    string `json:"header_name"`
	APIKey        string `json:"api_key"`
	TokenPrefix   string `json:"token_prefix"`
	LoginPath     string `json:"login_path"`
	LoginMethod   string `json:"login_method"`
	LoginBody     string `json:"login_body"`
	LoginBodyType string `json:"login_body_type"` // json | form (form = x-www-form-urlencoded, padrão OAuth2/Postman)
	TokenJSONPath string `json:"token_json_path"`
	TokenHeader   string `json:"token_header"`
}

// OAuth2PasswordBody monta corpo e tipo para requisição de token (form é o mais comum em APIs OAuth2).
func OAuth2PasswordBody(ac AuthConfig) (body string, bodyType string) {
	bt := strings.ToLower(strings.TrimSpace(ac.LoginBodyType))
	if bt != "json" {
		bt = "form"
	}
	if bt == "json" {
		return BuildOAuth2PasswordLoginBody(ac), "json"
	}
	return BuildOAuth2PasswordFormEncoded(ac), "form"
}

// BuildOAuth2PasswordLoginBody monta o JSON típico de token OAuth2 (grant password).
func BuildOAuth2PasswordLoginBody(ac AuthConfig) string {
	grant := strings.TrimSpace(ac.GrantType)
	if grant == "" {
		grant = "password"
	}
	body := map[string]string{
		"client_id":     strings.TrimSpace(ac.ClientID),
		"client_secret": strings.TrimSpace(ac.ClientSecret),
		"username":      strings.TrimSpace(ac.Username),
		"password":      strings.TrimSpace(ac.Password),
		"grant_type":    grant,
	}
	b, _ := json.Marshal(body)
	return string(b)
}

// BuildOAuth2PasswordFormEncoded corpo application/x-www-form-urlencoded (como Postman OAuth2).
func BuildOAuth2PasswordFormEncoded(ac AuthConfig) string {
	grant := strings.TrimSpace(ac.GrantType)
	if grant == "" {
		grant = "password"
	}
	v := url.Values{}
	v.Set("client_id", strings.TrimSpace(ac.ClientID))
	v.Set("client_secret", strings.TrimSpace(ac.ClientSecret))
	v.Set("username", strings.TrimSpace(ac.Username))
	v.Set("password", strings.TrimSpace(ac.Password))
	v.Set("grant_type", grant)
	return v.Encode()
}

// TokenExpiresInSeconds lê expires_in da resposta de login (segundos).
func TokenExpiresInSeconds(raw []byte) int {
	v := extractJSONPath(raw, "expires_in")
	if v == nil {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	default:
		var sec int
		if _, err := fmt.Sscanf(fmt.Sprint(v), "%d", &sec); err == nil {
			return sec
		}
	}
	return 0
}

type IntegrationConfig struct {
	BaseURL        string
	DefaultHeaders map[string]string
	Variables      map[string]string
	AuthType       string
	AuthConfig     AuthConfig
	SessionToken   string
	TimeoutMs      int
	TLSInsecure    bool
}

type RequestConfig struct {
	Method             string
	Path               string
	PathParams         []PathParam
	QueryParams        []ParamKV
	Headers            map[string]string
	BodyTemplate       string
	BodyType           string
	ExtractJSONPath    string
	OmitDefaultHeaders bool // requisição de token: não enviar headers por defeito (ex. credenciais em header)
}

type RunResult struct {
	OK              bool
	StatusCode      int
	LatencyMS       int64
	RequestURL      string
	RequestMethod   string
	ResponsePreview string
	Extracted       any
	ErrorMessage    string
	TokenFromLogin  string
}

func SubstituteVars(s string, vars map[string]string) string {
	if s == "" {
		return s
	}
	out := s
	for k, v := range vars {
		out = strings.ReplaceAll(out, "{{"+k+"}}", v)
		out = strings.ReplaceAll(out, "${"+k+"}", v)
	}
	return out
}

func BuildPath(path string, pathParams []PathParam, vars map[string]string) (string, error) {
	p := strings.TrimSpace(path)
	if p == "" {
		p = "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	used := map[string]string{}
	for _, pp := range pathParams {
		name := strings.TrimSpace(pp.Name)
		if name == "" {
			continue
		}
		val := strings.TrimSpace(pp.Value)
		if strings.EqualFold(pp.Source, "variable") && val != "" {
			if v, ok := vars[val]; ok {
				val = v
			}
		}
		used[name] = val
	}
	var missing []string
	out := pathParamRe.ReplaceAllStringFunc(p, func(m string) string {
		name := m[1 : len(m)-1]
		if v, ok := used[name]; ok {
			return url.PathEscape(v)
		}
		if v, ok := vars[name]; ok {
			return url.PathEscape(v)
		}
		missing = append(missing, name)
		return m
	})
	if len(missing) > 0 {
		return "", fmt.Errorf("path params em falta: %s", strings.Join(missing, ", "))
	}
	return out, nil
}

func BuildQuery(queryParams []ParamKV, vars map[string]string) string {
	q := url.Values{}
	for _, p := range queryParams {
		key := strings.TrimSpace(p.Key)
		if key == "" || !p.IsEnabled() {
			continue
		}
		q.Set(key, SubstituteVars(p.Value, vars))
	}
	return q.Encode()
}

func MergeHeaders(def, req map[string]string, auth IntegrationConfig) map[string]string {
	out := map[string]string{}
	for k, v := range def {
		out[k] = v
	}
	for k, v := range req {
		out[k] = v
	}
	applyAuthHeaders(out, auth)
	return out
}

func applyAuthHeaders(h map[string]string, cfg IntegrationConfig) {
	ac := cfg.AuthConfig
	switch strings.ToLower(cfg.AuthType) {
	case "bearer":
		tok := ac.Token
		if tok == "" {
			tok = cfg.SessionToken
		}
		if tok != "" {
			prefix := strings.TrimSpace(ac.TokenPrefix)
			if prefix == "" {
				prefix = "Bearer"
			}
			h["Authorization"] = strings.TrimSpace(prefix) + " " + strings.TrimSpace(tok)
		}
	case "basic":
		if ac.Username != "" {
			// net/http SetBasicAuth in Execute
		}
	case "api_key":
		name := strings.TrimSpace(ac.HeaderName)
		if name == "" {
			name = "X-API-Key"
		}
		key := ac.APIKey
		if key == "" {
			key = ac.Token
		}
		if key != "" {
			h[name] = key
		}
	case "login", "oauth2_password":
		if cfg.SessionToken != "" {
			hdr := strings.TrimSpace(ac.TokenHeader)
			if hdr == "" {
				hdr = "Authorization"
			}
			prefix := strings.TrimSpace(ac.TokenPrefix)
			if prefix == "" {
				prefix = "Bearer"
			}
			h[hdr] = strings.TrimSpace(prefix) + " " + strings.TrimSpace(cfg.SessionToken)
		}
	}
}

func Execute(ctx context.Context, integ IntegrationConfig, req RequestConfig) RunResult {
	start := time.Now()
	res := RunResult{RequestMethod: strings.ToUpper(strings.TrimSpace(req.Method))}
	if res.RequestMethod == "" {
		res.RequestMethod = "GET"
	}

	vars := map[string]string{}
	for k, v := range integ.Variables {
		vars[k] = v
	}

	path, err := BuildPath(req.Path, req.PathParams, vars)
	if err != nil {
		res.ErrorMessage = err.Error()
		return res
	}
	path = SubstituteVars(path, vars)

	base := strings.TrimRight(strings.TrimSpace(integ.BaseURL), "/")
	fullURL := base + path
	q := BuildQuery(req.QueryParams, vars)
	if q != "" {
		fullURL += "?" + q
	}
	res.RequestURL = fullURL

	bodyReader, contentType, err := buildBody(req, vars)
	if err != nil {
		res.ErrorMessage = err.Error()
		return res
	}

	httpReq, err := http.NewRequestWithContext(ctx, res.RequestMethod, fullURL, bodyReader)
	if err != nil {
		res.ErrorMessage = err.Error()
		return res
	}
	defHdr := integ.DefaultHeaders
	if req.OmitDefaultHeaders {
		defHdr = nil
	}
	headers := MergeHeaders(defHdr, req.Headers, integ)
	for k, v := range headers {
		if strings.EqualFold(k, "host") {
			continue
		}
		httpReq.Header.Set(k, v)
	}
	if contentType != "" {
		httpReq.Header.Set("Content-Type", contentType)
	}
	if strings.EqualFold(integ.AuthType, "basic") {
		httpReq.SetBasicAuth(integ.AuthConfig.Username, integ.AuthConfig.Password)
	}

	timeout := time.Duration(integ.TimeoutMs) * time.Millisecond
	if timeout < time.Second {
		timeout = 15 * time.Second
	}
	client := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: integ.TLSInsecure},
		},
	}

	resp, err := client.Do(httpReq)
	res.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		res.ErrorMessage = err.Error()
		return res
	}
	defer resp.Body.Close()
	res.StatusCode = resp.StatusCode
	res.OK = resp.StatusCode >= 200 && resp.StatusCode < 300

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponsePreview+1))
	if len(raw) > maxResponsePreview {
		raw = raw[:maxResponsePreview]
		res.ResponsePreview = string(raw) + "\n… (resposta truncada)"
	} else {
		res.ResponsePreview = string(raw)
	}

	if !res.OK {
		res.ErrorMessage = fmt.Sprintf("HTTP %d", resp.StatusCode)
	}

	if req.ExtractJSONPath != "" {
		res.Extracted = extractJSONPath(raw, req.ExtractJSONPath)
	}
	return res
}

func buildBody(req RequestConfig, vars map[string]string) (io.Reader, string, error) {
	bt := strings.ToLower(strings.TrimSpace(req.BodyType))
	if bt == "" || bt == "none" {
		return nil, "", nil
	}
	body := SubstituteVars(req.BodyTemplate, vars)
	switch bt {
	case "json":
		if body != "" {
			var tmp any
			if err := json.Unmarshal([]byte(body), &tmp); err != nil {
				return nil, "", fmt.Errorf("body JSON inválido: %w", err)
			}
		}
		return strings.NewReader(body), "application/json", nil
	case "text":
		return strings.NewReader(body), "text/plain; charset=utf-8", nil
	case "form":
		// Corpo já codificado (client_id=…&client_secret=…)
		if body != "" && strings.Contains(body, "=") && !strings.Contains(body, "\n") {
			return strings.NewReader(body), "application/x-www-form-urlencoded", nil
		}
		vals := url.Values{}
		for _, line := range strings.Split(body, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				vals.Set(parts[0], parts[1])
			}
		}
		return strings.NewReader(vals.Encode()), "application/x-www-form-urlencoded", nil
	default:
		return strings.NewReader(body), "application/json", nil
	}
}

func extractJSONPath(raw []byte, path string) any {
	path = strings.TrimSpace(path)
	if path == "" || len(raw) == 0 {
		return nil
	}
	var doc any
	if json.Unmarshal(raw, &doc) != nil {
		return nil
	}
	parts := strings.Split(path, ".")
	cur := doc
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur, ok = m[p]
		if !ok {
			return nil
		}
	}
	return cur
}

func extractTokenFromBody(raw []byte, jsonPath string) string {
	if jsonPath == "" {
		jsonPath = "token"
	}
	v := extractJSONPath(raw, jsonPath)
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

// TestConnection faz GET na base_url (ou HEAD).
func TestConnection(ctx context.Context, integ IntegrationConfig) RunResult {
	return Execute(ctx, integ, RequestConfig{
		Method: "GET",
		Path:   "/",
	})
}

// ParseHeadersJSON converte mapa jsonb em headers.
func ParseHeadersJSON(b []byte) map[string]string {
	out := map[string]string{}
	if len(b) == 0 {
		return out
	}
	var m map[string]any
	if json.Unmarshal(b, &m) != nil {
		return out
	}
	for k, v := range m {
		out[k] = fmt.Sprint(v)
	}
	return out
}

// ParseVariablesJSON object -> map.
func ParseVariablesJSON(b []byte) map[string]string {
	out := map[string]string{}
	if len(b) == 0 {
		return out
	}
	var m map[string]any
	if json.Unmarshal(b, &m) != nil {
		return out
	}
	for k, v := range m {
		out[k] = fmt.Sprint(v)
	}
	return out
}

func ParsePathParams(b []byte) []PathParam {
	var list []PathParam
	_ = json.Unmarshal(b, &list)
	return list
}

func ParseQueryParams(b []byte) []ParamKV {
	var list []ParamKV
	_ = json.Unmarshal(b, &list)
	return list
}

func AuthConfigFromJSON(b []byte) AuthConfig {
	var ac AuthConfig
	_ = json.Unmarshal(b, &ac)
	return ac
}

func Slugify(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "integracao"
	}
	return s
}

func UniqueSlug(base string, exists func(string) bool) string {
	if !exists(base) {
		return base
	}
	for i := 2; i < 1000; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if !exists(candidate) {
			return candidate
		}
	}
	return base + "-" + fmt.Sprint(time.Now().Unix())
}

// RunWithLoginRequest sets login flag on config for token extraction.
func RunWithLoginRequest(ctx context.Context, integ IntegrationConfig, req RequestConfig, isLogin bool) RunResult {
	if isLogin {
		ac := integ.AuthConfig
		if ac.TokenJSONPath == "" {
			ac.TokenJSONPath = "access_token"
			integ.AuthConfig = ac
		}
		req.OmitDefaultHeaders = true
	}
	res := Execute(ctx, integ, req)
	if isLogin && res.OK {
		tok := extractTokenFromBody([]byte(res.ResponsePreview), integ.AuthConfig.TokenJSONPath)
		if tok == "" {
			tok = extractTokenFromBody([]byte(res.ResponsePreview), "token")
		}
		res.TokenFromLogin = tok
	}
	return res
}
