package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/netquasar/netquasar/quasar_backend/internal/bootstrap"
	"github.com/netquasar/netquasar/quasar_backend/internal/config"
	"github.com/netquasar/netquasar/quasar_backend/internal/db"
)

func (s *Server) setupStatus(w http.ResponseWriter, r *http.Request) {
	configured := s.DB() != nil
	writeJSON(w, http.StatusOK, map[string]any{"database_configured": configured})
}

func (s *Server) setupDatabaseTest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var body databaseConnectionBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	if body.DatabaseURL != nil && strings.TrimSpace(*body.DatabaseURL) != "" {
		dsn := strings.TrimSpace(*body.DatabaseURL)
		cfg := config.ConfigFromPostgresDSN(dsn)
		ep, err := db.NewEphemeralPool(ctx, cfg)
		if err != nil {
			writeErr(w, http.StatusBadGateway, "TEST_FAILED", err.Error(), supabaseConnectDetails(ctx, dsn, err.Error()))
			return
		}
		defer ep.Close()
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "teste com URL informada bem-sucedido (sem persistir pool)"})
		return
	}
	h, pt, u, n, pw, sm := body.Host, body.Port, body.DBUser, body.DBName, body.DBPassword, body.SSLMode
	if h == nil || strings.TrimSpace(*h) == "" || pt == nil || *pt <= 0 ||
		u == nil || strings.TrimSpace(*u) == "" || n == nil || strings.TrimSpace(*n) == "" ||
		pw == nil || strings.TrimSpace(*pw) == "" {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "informe host, port, db_user, db_name e db_password (ou use database_url)", nil)
		return
	}
	dsn := config.PostgresURLFromParts(strings.TrimSpace(*h), *pt, strings.TrimSpace(*u), strings.TrimSpace(*pw), strings.TrimSpace(*n), derefStrOr(sm, "disable"))
	cfg := config.ConfigFromPostgresDSN(dsn)
	ep, err := db.NewEphemeralPool(ctx, cfg)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "TEST_FAILED", err.Error(), supabaseConnectDetails(ctx, dsn, err.Error()))
		return
	}
	defer ep.Close()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "teste com parâmetros bem-sucedido (sem persistir pool)"})
}

func (s *Server) setupDatabaseApply(w http.ResponseWriter, r *http.Request) {
	if s.DB() != nil {
		writeErr(w, http.StatusConflict, "ALREADY_CONFIGURED", "a base de dados já está configurada; use Definições → Base de dados.", nil)
		return
	}
	var body databaseConnectionBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	ctx := r.Context()
	var dsn string
	if body.DatabaseURL != nil && strings.TrimSpace(*body.DatabaseURL) != "" {
		dsn = strings.TrimSpace(*body.DatabaseURL)
		if err := s.switchDatabasePool(w, r, dsn, nil, nil, nil, nil, nil, nil); err != nil {
			return
		}
	} else {
		h, pt, u, n, pw, sm := body.Host, body.Port, body.DBUser, body.DBName, body.DBPassword, body.SSLMode
		if h == nil || strings.TrimSpace(*h) == "" || pt == nil || *pt <= 0 ||
			u == nil || strings.TrimSpace(*u) == "" || n == nil || strings.TrimSpace(*n) == "" ||
			pw == nil || strings.TrimSpace(*pw) == "" {
			writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "informe host, port, db_user, db_name e db_password (ou use database_url)", nil)
			return
		}
		h2 := strings.TrimSpace(*h)
		pt2 := *pt
		u2 := strings.TrimSpace(*u)
		n2 := strings.TrimSpace(*n)
		smStr := derefStrOr(sm, "disable")
		dsn = config.PostgresURLFromParts(h2, pt2, u2, strings.TrimSpace(*pw), n2, smStr)
		if err := s.switchDatabasePool(w, r, dsn, &h2, &pt2, &u2, &n2, sm, pw); err != nil {
			return
		}
	}

	pool := s.DB()
	if pool == nil {
		writeErr(w, http.StatusInternalServerError, "INTERNAL", "pool após aplicar é nil", nil)
		return
	}
	if err := bootstrap.EnsureDefaultUsers(ctx, pool); err != nil {
		writeErr(w, http.StatusInternalServerError, "SEED_USERS", err.Error(), nil)
		return
	}
	cfgApplied := config.ConfigFromPostgresDSN(dsn)
	if err := bootstrap.EnsureDatabaseMetaRow(ctx, pool, cfgApplied); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB_META", err.Error(), nil)
		return
	}
	s.ensureMonitoringWorker()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "base de dados configurada; credenciais gravadas localmente para o próximo arranque."})
}
