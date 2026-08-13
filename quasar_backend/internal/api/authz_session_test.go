package api

import (
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/netquasar/netquasar/quasar_backend/internal/config"
)

func TestRequestAuthContext_JWTWinsWithoutRequireAuth(t *testing.T) {
	cfg := &config.Config{}
	if cfg.RequireAuth() {
		t.Fatal("expected RequireAuth false without keys/secret")
	}
	s := &Server{Cfg: cfg}
	uid := uuid.New()
	tok, err := mintUserJWT(cfg, uid, "ops@netquasar.local", "admin")
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "/api/v1/me/preferences", nil)
	r.Header.Set("Authorization", "Bearer "+tok)
	ac := s.requestAuthContext(r)
	if !ac.OK || ac.UserID != uid {
		t.Fatalf("want user %s, got ok=%v uid=%s", uid, ac.OK, ac.UserID)
	}
}

func TestRequestAuthContext_JWTWinsWithAPIKey(t *testing.T) {
	cfg := &config.Config{APIKeys: []string{"dev-key"}}
	s := &Server{Cfg: cfg}
	uid := uuid.New()
	tok, err := mintUserJWT(cfg, uid, "ops@netquasar.local", "viewer")
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("PATCH", "/api/v1/me/preferences", nil)
	r.Header.Set("Authorization", "Bearer "+tok)
	r.Header.Set("X-API-Key", "dev-key")
	ac := s.requestAuthContext(r)
	if ac.UserID != uid {
		t.Fatalf("JWT should identify the user even with API key, got %s", ac.UserID)
	}
}
