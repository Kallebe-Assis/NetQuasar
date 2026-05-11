package api

import (
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

// apiCombinedAuth exige X-API-Key válida ou JWT de utilizador quando cfg.RequireAuth().
// Sem RequireAuth (dev típico), /api fica aberto como antes.
func apiCombinedAuth(cfg *config.Config) func(http.Handler) http.Handler {
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

			if !cfg.RequireAuth() {
				next.ServeHTTP(w, r)
				return
			}

			if APIKeyMatches(cfg, r) {
				next.ServeHTTP(w, r)
				return
			}

			bearer := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
			if bearer != "" {
				if _, _, _, err := parseUserJWT(cfg, bearer); err == nil {
					next.ServeHTTP(w, r)
					return
				}
			}
			writeErr(w, http.StatusUnauthorized, "UNAUTHORIZED", "credenciais de API ou sessão inválidas ou ausentes", nil)
		})
	}
}

func chain(cfg *config.Config, log zerolog.Logger, h http.Handler) http.Handler {
	wrapped := baseMiddleware(cfg, log)(h)
	wrapped = apiCombinedAuth(cfg)(wrapped)
	return wrapped
}
