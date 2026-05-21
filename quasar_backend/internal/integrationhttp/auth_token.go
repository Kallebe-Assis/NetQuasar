package integrationhttp

import (
	"encoding/base64"
	"strings"
)

// BearerAuthorizationValue monta o valor do header Authorization para auth_type=bearer.
// encodeBase64 só aplica quando o utilizador activa token_encode_base64 na configuração.
func BearerAuthorizationValue(token, prefix string, encodeBase64 bool) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = "Bearer"
	}
	if encodeBase64 {
		token = BasicCredentialForHeader(token, true)
	}
	return prefix + " " + token
}

// BasicCredentialForHeader devolve o segmento após «Basic ».
func BasicCredentialForHeader(token string, encode bool) string {
	token = strings.TrimSpace(token)
	if token == "" || !encode {
		return token
	}
	if LooksLikeBase64Credential(token) {
		return token
	}
	return base64.StdEncoding.EncodeToString([]byte(token))
}

// LooksLikeBase64Credential indica se o valor já parece Base64 (evita codificar duas vezes).
func LooksLikeBase64Credential(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) < 8 || strings.Contains(s, ":") {
		return false
	}
	for _, c := range s {
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '+', c == '/', c == '=':
			continue
		default:
			return false
		}
	}
	return true
}

// AuthUsesSessionLogin indica auth que exige obter token (OAuth/login) antes das chamadas à API.
func AuthUsesSessionLogin(authType string) bool {
	switch strings.ToLower(strings.TrimSpace(authType)) {
	case "oauth2_password", "login":
		return true
	default:
		return false
	}
}

// ValidateStaticAuthConfig verifica credenciais fixas (bearer/basic/api_key). Retorna mensagem de erro ou "".
func ValidateStaticAuthConfig(cfg IntegrationConfig) string {
	at := strings.ToLower(strings.TrimSpace(cfg.AuthType))
	ac := cfg.AuthConfig
	switch at {
	case "bearer":
		tok := strings.TrimSpace(ac.Token)
		if tok == "" {
			tok = strings.TrimSpace(cfg.SessionToken)
		}
		if tok == "" {
			return "Token não configurado. Preencha o token e clique em «Salvar auth»."
		}
	case "basic":
		if strings.TrimSpace(ac.Username) == "" {
			return "Usuário Basic não configurado."
		}
	case "api_key":
		key := strings.TrimSpace(ac.APIKey)
		if key == "" {
			key = strings.TrimSpace(ac.Token)
		}
		if key == "" {
			return "API Key não configurada."
		}
	}
	return ""
}
