package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"
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

type authContext struct {
	UserID      uuid.UUID
	Email       string
	Role        string
	ProfileID   *uuid.UUID
	ProfileSlug string
	Permissions []string
	OK          bool
}

func defaultViewerPermissions() []string {
	return []string{
		"dashboard.view", "monitoring.view", "realtime.view", "integrations.view",
		"pops.view", "devices.view", "commercial.view", "connections.view",
		"alerts.view", "map.view", "tools.view", "olt.view", "mikrotik.view",
		"switch.view", "bng.view", "reports.view", "fleet.view",
	}
}

func (s *Server) resolveUserPermissions(ctx context.Context, userID uuid.UUID, role string) (profileID *uuid.UUID, profileSlug string, perms []string) {
	role = strings.ToLower(strings.TrimSpace(role))
	if s.DB() == nil {
		if role == "admin" {
			return nil, "admin", []string{"*"}
		}
		return nil, "user", defaultViewerPermissions()
	}
	var pid uuid.UUID
	var slug string
	var raw []byte
	err := s.DB().QueryRow(ctx, `
		SELECT COALESCE(u.permission_profile_id, p_fallback.id),
			COALESCE(p.slug, p_fallback.slug),
			COALESCE(p.permissions, p_fallback.permissions)
		FROM users u
		LEFT JOIN permission_profiles p ON p.id = u.permission_profile_id
		LEFT JOIN permission_profiles p_fallback ON p_fallback.slug = CASE WHEN u.role = 'admin' THEN 'admin' ELSE 'user' END
		WHERE u.id = $1
	`, userID).Scan(&pid, &slug, &raw)
	if err != nil {
		if role == "admin" {
			return nil, "admin", []string{"*"}
		}
		return nil, "user", defaultViewerPermissions()
	}
	idCopy := pid
	perms = decodePermissionsJSON(raw)
	if role == "admin" || strings.EqualFold(slug, "admin") || permissionGranted(perms, "*") {
		return &idCopy, "admin", []string{"*"}
	}
	if len(perms) == 0 {
		perms = defaultViewerPermissions()
	}
	return &idCopy, slug, perms
}

// requestAuthRole devolve o papel efetivo: "admin", "viewer", ou ("", false) se não autenticado.
// Sem RequireAuth, trata-se como admin (comportamento de desenvolvimento).
// Chave API válida equivale a admin (acesso total).
func (s *Server) requestAuthRole(r *http.Request) (role string, ok bool) {
	ac := s.requestAuthContext(r)
	return ac.Role, ac.OK
}

func (s *Server) requestAuthContext(r *http.Request) authContext {
	if !s.Cfg.RequireAuth() {
		return authContext{Role: "admin", ProfileSlug: "admin", Permissions: []string{"*"}, OK: true}
	}
	bearer := bearerFromRequest(r)
	if bearer != "" {
		uid, email, role, err := parseUserJWT(s.Cfg, bearer)
		if err == nil {
			pid, slug, perms := s.resolveUserPermissions(r.Context(), uid, role)
			effectiveRole := role
			if permissionGranted(perms, "*") {
				effectiveRole = "admin"
			} else if strings.EqualFold(role, "admin") {
				effectiveRole = "admin"
			} else {
				effectiveRole = "viewer"
			}
			return authContext{
				UserID: uid, Email: email, Role: effectiveRole,
				ProfileID: pid, ProfileSlug: slug, Permissions: perms, OK: true,
			}
		}
	}
	if APIKeyMatches(s.Cfg, r) {
		return authContext{Role: "admin", ProfileSlug: "admin", Permissions: []string{"*"}, OK: true}
	}
	return authContext{}
}

func (s *Server) requireAdminMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ac := s.requestAuthContext(r)
		if !ac.OK || (ac.Role != "admin" && !permissionGranted(ac.Permissions, "*")) {
			writeErr(w, http.StatusForbidden, "FORBIDDEN", "apenas administradores podem executar esta ação", nil)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requestHasAnyPermission(r *http.Request, keys ...string) bool {
	ac := s.requestAuthContext(r)
	if !ac.OK {
		return false
	}
	if permissionGranted(ac.Permissions, "*") {
		return true
	}
	for _, key := range keys {
		if permissionGranted(ac.Permissions, key) {
			return true
		}
	}
	return false
}

// requirePermission exige autenticação e pelo menos uma das permissões indicadas.
func (s *Server) requirePermission(w http.ResponseWriter, r *http.Request, keys ...string) bool {
	ac := s.requestAuthContext(r)
	if !ac.OK {
		if s.Cfg.RequireAuth() {
			writeErr(w, http.StatusUnauthorized, "UNAUTHORIZED", "sessão inválida ou ausente", nil)
		} else {
			writeErr(w, http.StatusForbidden, "FORBIDDEN", "acesso negado", nil)
		}
		return false
	}
	if s.requestHasAnyPermission(r, keys...) {
		return true
	}
	writeErr(w, http.StatusForbidden, "FORBIDDEN", "sem permissão para esta ação", map[string]any{"required": keys})
	return false
}

func (s *Server) requirePermissionMiddleware(keys ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !s.requirePermission(w, r, keys...) {
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (s *Server) legacyRoleFromProfileSlug(slug string, perms []string) string {
	if permissionGranted(perms, "*") || strings.EqualFold(slug, "admin") {
		return "admin"
	}
	return "viewer"
}

func (s *Server) profileIDForRoleOrID(ctx context.Context, role string, profileID *uuid.UUID) (*uuid.UUID, string, error) {
	if profileID != nil {
		row, err := s.loadPermissionProfileByID(ctx, *profileID)
		if err != nil {
			return nil, "", err
		}
		return &row.ID, row.Slug, nil
	}
	slug := "user"
	if strings.EqualFold(strings.TrimSpace(role), "admin") {
		slug = "admin"
	}
	row, err := s.loadPermissionProfileBySlug(ctx, slug)
	if err != nil {
		return nil, slug, err
	}
	id := row.ID
	return &id, row.Slug, nil
}
