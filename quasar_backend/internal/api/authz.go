package api

import (
	"net/http"
	"strings"

	"github.com/netquasar/netquasar/quasar_backend/internal/config"
)

// APIKeyMatches devolve true se X-API-Key ou Bearer coincidir com uma chave configurada.
func APIKeyMatches(cfg *config.Config, r *http.Request) bool {
	xKey := strings.TrimSpace(r.Header.Get("X-API-Key"))
	bearer := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	qKey := strings.TrimSpace(r.URL.Query().Get("api_key"))
	for _, k := range cfg.APIKeys {
		kk := strings.TrimSpace(k)
		if kk == "" {
			continue
		}
		if xKey == kk || bearer == kk || qKey == kk {
			return true
		}
	}
	return false
}

// requestAuthRole devolve o papel efetivo: "admin", "viewer", ou ("", false) se não autenticado.
// Sem RequireAuth, trata-se como admin (comportamento de desenvolvimento).
// Chave API válida equivale a admin (acesso total).
func (s *Server) requestAuthRole(r *http.Request) (role string, ok bool) {
	if !s.Cfg.RequireAuth() {
		return "admin", true
	}
	bearer := bearerFromRequest(r)
	if bearer != "" {
		if _, _, role, err := parseUserJWT(s.Cfg, bearer); err == nil {
			return role, true
		}
	}
	if APIKeyMatches(s.Cfg, r) {
		return "admin", true
	}
	return "", false
}

func (s *Server) requireAdminMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		role, ok := s.requestAuthRole(r)
		if !ok || role != "admin" {
			writeErr(w, http.StatusForbidden, "FORBIDDEN", "apenas administradores podem executar esta ação", nil)
			return
		}
		next.ServeHTTP(w, r)
	})
}
