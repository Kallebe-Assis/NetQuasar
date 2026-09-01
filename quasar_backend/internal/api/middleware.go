package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/netquasar/netquasar/quasar_backend/internal/config"
	"github.com/rs/zerolog"
)

type respCapture struct {
	http.ResponseWriter
	status int
}

func (rw *respCapture) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func baseMiddleware(cfg *config.Config, log zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rid := uuid.NewString()
			l := log.With().Str("request_id", rid).Str("path", r.URL.Path).Str("method", r.Method).Logger()
			r = r.WithContext(l.WithContext(r.Context()))
			rc := &respCapture{ResponseWriter: w, status: 200}
			t0 := time.Now()
			next.ServeHTTP(rc, r)
			l.Info().Int("status", rc.status).Dur("duration", time.Since(t0)).Msg("http")
		})
	}
}

func isAPIPublicPath(p string) bool {
	if p == "/health" || p == "/api/v1/health" {
		return true
	}
	if strings.HasPrefix(p, "/api/v1/setup/") {
		return true
	}
	if p == "/api/v1/auth/login" {
		return true
	}
	return false
}

// apiCombinedAuth exige X-API-Key válida ou JWT de usuário quando cfg.RequireAuth().
// Sem RequireAuth (dev típico), /api fica aberto como antes — MAS um JWT de usuário
// apresentado é sempre validado (assinatura + força-desconexão via sessions_invalidated_at),
// mesmo nesse modo: ver o comentário sobre isso logo abaixo.
func apiCombinedAuth(cfg *config.Config, s *Server) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}
			p := r.URL.Path
			if isAPIPublicPath(p) {
				next.ServeHTTP(w, r)
				return
			}
			if p != "/metrics" && !strings.HasPrefix(p, "/api/") {
				next.ServeHTTP(w, r)
				return
			}

			// Quando um JWT de usuário é apresentado, ele é sempre validado aqui — assinatura E
			// força-desconexão (users.sessions_invalidated_at) — antes de qualquer atalho de
			// RequireAuth()/API key abaixo. Sem isto, "Forçar desconexão" (Configurações →
			// Usuários) não tinha efeito algum em instalações com RequireAuth() desligado (o
			// atalho abaixo deixava passar tudo sem sequer olhar para o token), e mesmo com
			// RequireAuth() ligado o token continuava a ser aceite aqui (só a checagem de
			// permissões — requestAuthContext, authz.go — olhava para sessions_invalidated_at,
			// e nem toda rota passa por ela). Este é o único portão por onde TODA requisição
			// /api passa, então é o sítio certo para isto ter efeito imediato na requisição
			// seguinte do usuário-alvo, seja qual for a rota.
			bearer := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
			if bearer != "" {
				if uid, _, _, issuedAt, err := parseUserJWT(cfg, bearer); err == nil && uid != uuid.Nil {
					if invalidatedAt := s.userSessionsInvalidatedAt(r.Context(), uid); invalidatedAt != nil && !issuedAt.IsZero() && issuedAt.Before(*invalidatedAt) {
						writeErr(w, http.StatusUnauthorized, "UNAUTHORIZED", "sessão encerrada", nil)
						return
					}
					next.ServeHTTP(w, r)
					return
				}
				// Assinatura inválida/expirada — cai para as verificações abaixo (API key / RequireAuth).
			}

			if !cfg.RequireAuth() {
				next.ServeHTTP(w, r)
				return
			}

			if APIKeyMatches(cfg, r) {
				next.ServeHTTP(w, r)
				return
			}

			writeErr(w, http.StatusUnauthorized, "UNAUTHORIZED", "credenciais de API ou sessão inválidas ou ausentes", nil)
		})
	}
}

// userSessionsInvalidatedAt consulta directamente users.sessions_invalidated_at — usada pelo
// portão global apiCombinedAuth (chamada em toda requisição autenticada por JWT, então tem de
// ser barata: sem o JOIN de permission_profiles que resolveUserPermissions faz).
func (s *Server) userSessionsInvalidatedAt(ctx context.Context, userID uuid.UUID) *time.Time {
	if s == nil || s.DB() == nil {
		return nil
	}
	var t *time.Time
	if err := s.DB().QueryRow(ctx, `SELECT sessions_invalidated_at FROM users WHERE id=$1`, userID).Scan(&t); err != nil {
		return nil
	}
	return t
}

func chain(cfg *config.Config, log zerolog.Logger, s *Server, h http.Handler) http.Handler {
	wrapped := baseMiddleware(cfg, log)(h)
	wrapped = apiCombinedAuth(cfg, s)(wrapped)
	return wrapped
}
