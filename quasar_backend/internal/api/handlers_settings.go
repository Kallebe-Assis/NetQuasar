package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/netquasar/netquasar/quasar_backend/internal/config"
	"github.com/netquasar/netquasar/quasar_backend/internal/db"
	"github.com/netquasar/netquasar/quasar_backend/internal/localdbstore"
	"github.com/netquasar/netquasar/quasar_backend/internal/telegramclient"
	"golang.org/x/crypto/bcrypt"
)

func maskDBUser(u *string) any {
	if u == nil || *u == "" {
		return nil
	}
	s := *u
	if len(s) <= 2 {
		return "***"
	}
	return s[:2] + "***"
}

func (s *Server) auditDBConnection(ctx context.Context, ok bool, phase, msg string, host *string, port *int, dbname *string) {
	p := s.DB()
	if p == nil {
		return
	}
	_, _ = p.Exec(ctx, `
		INSERT INTO settings_connection_audit (ok, phase, message, target_host, target_port, target_db)
		VALUES ($1,$2,$3,$4,$5,$6)
	`, ok, phase, msg, derefStr(host), derefInt(port), derefStr(dbname))
}

func derefStr(p *string) *string {
	if p == nil || *p == "" {
		return nil
	}
	return p
}

func derefInt(p *int) *int {
	if p == nil || *p == 0 {
		return nil
	}
	return p
}

type databaseConnectionBody struct {
	Host            *string `json:"host"`
	Port            *int    `json:"port"`
	DBUser          *string `json:"db_user"`
	DBPassword      *string `json:"db_password"`
	DBName          *string `json:"db_name"`
	SSLMode         *string `json:"ssl_mode"`
	ApplyConnection *bool   `json:"apply_connection"`
	DatabaseURL     *string `json:"database_url"`
}

func (s *Server) getDatabaseMeta(w http.ResponseWriter, r *http.Request) {
	p := s.DB()
	if p == nil {
		writeErr(w, http.StatusServiceUnavailable, "NO_DB", "pool não configurado", nil)
		return
	}
	var host *string
	var port *int
	var user, dbname, ssl *string
	var hasPass bool
	err := p.QueryRow(r.Context(), `
		SELECT host, port, db_user, db_name, ssl_mode,
			(db_password IS NOT NULL AND length(trim(db_password)) > 0)
		FROM settings_database_meta WHERE id=1
	`).Scan(&host, &port, &user, &dbname, &ssl, &hasPass)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	active := "settings_row"
	if s.Cfg != nil && s.Cfg.DatabaseURL != "" {
		active = "env_NETQUASAR_DATABASE_URL"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"host":                host,
		"port":                port,
		"db_user_masked":      maskDBUser(user),
		"db_name":             dbname,
		"ssl_mode":            ssl,
		"password_configured": hasPass,
		"active_dsn_source":   active,
		"note":                "PATCH com apply_connection=true valida DSN, aplica migrações no alvo e recarrega o pool; senha nunca é devolvida no GET.",
	})
}

func (s *Server) patchDatabaseMeta(w http.ResponseWriter, r *http.Request) {
	p := s.DB()
	if p == nil {
		writeErr(w, http.StatusServiceUnavailable, "NO_DB", "pool não configurado", nil)
		return
	}
	var body databaseConnectionBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	apply := body.ApplyConnection != nil && *body.ApplyConnection
	if body.DatabaseURL != nil && strings.TrimSpace(*body.DatabaseURL) != "" {
		if !apply {
			writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "database_url exige apply_connection=true (troca de instância)", nil)
			return
		}
		dsn := strings.TrimSpace(*body.DatabaseURL)
		if err := s.switchDatabasePool(w, r, dsn, nil, nil, nil, nil, nil, nil); err != nil {
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "applied": true, "message": "pool recarregado a partir de database_url"})
		return
	}

	if apply {
		var curHost, curUser, curName, curSSL *string
		var curPort *int
		var curPass *string
		err := p.QueryRow(r.Context(), `
			SELECT host, port, db_user, db_name, ssl_mode, db_password FROM settings_database_meta WHERE id=1
		`).Scan(&curHost, &curPort, &curUser, &curName, &curSSL, &curPass)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		h := coalesceStrPtr(body.Host, curHost)
		u := coalesceStrPtr(body.DBUser, curUser)
		n := coalesceStrPtr(body.DBName, curName)
		sm := coalesceStrPtr(body.SSLMode, curSSL)
		pt := coalesceIntPtr(body.Port, curPort)
		var pw *string
		if body.DBPassword != nil && *body.DBPassword != "" {
			pw = body.DBPassword
		} else {
			pw = curPass
		}
		if h == nil || *h == "" || u == nil || *u == "" || n == nil || *n == "" || pt == nil || *pt <= 0 || pw == nil || *pw == "" {
			writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "apply_connection requer host, port, db_user, db_name e senha (envie db_password ou grave antes sem aplicar)", nil)
			return
		}
		dsn := config.PostgresURLFromParts(*h, *pt, *u, *pw, *n, derefStrOr(sm, "disable"))
		if err := s.switchDatabasePool(w, r, dsn, h, pt, u, n, sm, pw); err != nil {
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "applied": true, "message": "pool recarregado; metadados gravados no novo banco"})
		return
	}

	passUpdate := false
	var passVal any
	if body.DBPassword != nil {
		passUpdate = true
		if strings.TrimSpace(*body.DBPassword) == "" {
			passVal = nil
		} else {
			passVal = *body.DBPassword
		}
	}
	_, err := p.Exec(r.Context(), `
		UPDATE settings_database_meta SET
			host = COALESCE($1, host),
			port = COALESCE($2, port),
			db_user = COALESCE($3, db_user),
			db_name = COALESCE($4, db_name),
			ssl_mode = COALESCE($5, ssl_mode),
			db_password = CASE WHEN $6::boolean THEN $7::text ELSE db_password END,
			updated_at = now()
		WHERE id=1
	`, body.Host, body.Port, body.DBUser, body.DBName, body.SSLMode,
		passUpdate, passVal)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "applied": false})
}

func coalesceStrPtr(a, b *string) *string {
	if a != nil && *a != "" {
		return a
	}
	return b
}

func coalesceIntPtr(a, b *int) *int {
	if a != nil && *a > 0 {
		return a
	}
	return b
}

func derefStrOr(p *string, def string) string {
	if p == nil || *p == "" {
		return def
	}
	return *p
}

// supabaseConnectDetails JSON opcional para erros de ligação (hint IPv6 / pooler).
func supabaseConnectDetails(ctx context.Context, dsn, errMsg string) any {
	return config.ErrDetailsWithSupabaseHint(ctx, dsn, errMsg)
}

// switchDatabasePool valida DSN, migra o alvo, grava meta no novo banco, troca o pool atômico e agenda fechamento do antigo.
func (s *Server) switchDatabasePool(w http.ResponseWriter, r *http.Request, dsn string, host *string, port *int, user, dbname, sslmode, password *string) error {
	ctx := r.Context()
	cfg := config.ConfigFromPostgresDSN(dsn)
	if err := db.Migrate(ctx, cfg); err != nil {
		s.auditDBConnection(ctx, false, "migrate", err.Error(), host, port, dbname)
		writeErr(w, http.StatusBadGateway, "MIGRATE_FAILED", err.Error(), supabaseConnectDetails(ctx, dsn, err.Error()))
		return err
	}
	newPool, err := db.NewPool(ctx, cfg)
	if err != nil {
		s.auditDBConnection(ctx, false, "connect", err.Error(), host, port, dbname)
		writeErr(w, http.StatusBadGateway, "CONNECT_FAILED", err.Error(), supabaseConnectDetails(ctx, dsn, err.Error()))
		return err
	}
	if host != nil && port != nil && user != nil && dbname != nil {
		sm := derefStrOr(sslmode, "disable")
		_, err = newPool.Exec(ctx, `
			INSERT INTO settings_database_meta (id, host, port, db_user, db_name, ssl_mode, db_password, updated_at)
			VALUES (1, $1, $2, $3, $4, $5, $6, now())
			ON CONFLICT (id) DO UPDATE SET
				host = EXCLUDED.host,
				port = EXCLUDED.port,
				db_user = EXCLUDED.db_user,
				db_name = EXCLUDED.db_name,
				ssl_mode = EXCLUDED.ssl_mode,
				db_password = EXCLUDED.db_password,
				updated_at = now()
		`, *host, *port, *user, *dbname, sm, password)
		if err != nil {
			newPool.Close()
			s.auditDBConnection(ctx, false, "meta_persist", err.Error(), host, port, dbname)
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return err
		}
	}
	if s.DBHolder == nil {
		newPool.Close()
		writeErr(w, http.StatusInternalServerError, "NO_HOLDER", "holder de pool ausente", nil)
		return errors.New("no db holder")
	}
	old := s.DBHolder.Swap(newPool)
	s.auditDBConnection(ctx, true, "apply", "pool substituído", host, port, dbname)
	if old != nil {
		go func(cl *time.Timer) {
			<-cl.C
			old.Close()
		}(time.NewTimer(2 * time.Second))
	}
	if err := localdbstore.WriteFromDSN(dsn); err != nil {
		s.Log.Warn().Err(err).Msg("credenciais locais não gravadas (reinicie sem env se precisar do ficheiro)")
	}
	return nil
}

func (s *Server) testDatabaseConnection(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p := s.DB()
	if p == nil {
		writeErr(w, http.StatusServiceUnavailable, "NO_DB", "pool não configurado", nil)
		return
	}
	var body databaseConnectionBody
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.DatabaseURL != nil && strings.TrimSpace(*body.DatabaseURL) != "" {
		dsn := strings.TrimSpace(*body.DatabaseURL)
		cfg := config.ConfigFromPostgresDSN(dsn)
		ep, err := db.NewEphemeralPool(ctx, cfg)
		if err != nil {
			s.auditDBConnection(ctx, false, "test", err.Error(), nil, nil, nil)
			writeErr(w, http.StatusBadGateway, "TEST_FAILED", err.Error(), supabaseConnectDetails(ctx, dsn, err.Error()))
			return
		}
		defer ep.Close()
		s.auditDBConnection(ctx, true, "test", "conexão OK (database_url)", nil, nil, nil)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "teste com URL informada bem-sucedido (sem persistir pool)"})
		return
	}
	if body.Host != nil || body.Port != nil || body.DBUser != nil || body.DBName != nil || (body.DBPassword != nil && *body.DBPassword != "") || body.SSLMode != nil {
		var curHost, curUser, curName, curSSL *string
		var curPort *int
		var curPass *string
		_ = p.QueryRow(ctx, `SELECT host, port, db_user, db_name, ssl_mode, db_password FROM settings_database_meta WHERE id=1`).
			Scan(&curHost, &curPort, &curUser, &curName, &curSSL, &curPass)
		h := coalesceStrPtr(body.Host, curHost)
		u := coalesceStrPtr(body.DBUser, curUser)
		n := coalesceStrPtr(body.DBName, curName)
		sm := coalesceStrPtr(body.SSLMode, curSSL)
		pt := coalesceIntPtr(body.Port, curPort)
		var pw *string
		if body.DBPassword != nil && *body.DBPassword != "" {
			pw = body.DBPassword
		} else {
			pw = curPass
		}
		if h == nil || *h == "" || u == nil || *u == "" || n == nil || *n == "" || pt == nil || *pt <= 0 || pw == nil || *pw == "" {
			writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "informe host, port, db_user, db_name e db_password (ou grave senha em PATCH sem apply)", nil)
			return
		}
		dsn := config.PostgresURLFromParts(*h, *pt, *u, *pw, *n, derefStrOr(sm, "disable"))
		cfg := config.ConfigFromPostgresDSN(dsn)
		ep, err := db.NewEphemeralPool(ctx, cfg)
		if err != nil {
			s.auditDBConnection(ctx, false, "test", err.Error(), h, pt, n)
			writeErr(w, http.StatusBadGateway, "TEST_FAILED", err.Error(), supabaseConnectDetails(ctx, dsn, err.Error()))
			return
		}
		defer ep.Close()
		s.auditDBConnection(ctx, true, "test", "conexão OK (parâmetros)", h, pt, n)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "teste com parâmetros bem-sucedido (sem trocar pool)"})
		return
	}
	if err := p.Ping(ctx); err != nil {
		s.auditDBConnection(ctx, false, "ping", err.Error(), nil, nil, nil)
		writeErr(w, http.StatusBadGateway, "PING_FAILED", err.Error(), nil)
		return
	}
	s.auditDBConnection(ctx, true, "ping", "pool atual OK", nil, nil, nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "pool atual respondeu ao ping"})
}

func (s *Server) getConnectionDefaults(w http.ResponseWriter, r *http.Request) {
	var snmp, tu, tp, te, su, sp *string
	var oltCPU, oltCPUAvail, oltMemUsed, oltMemSize, oltTemp, oltUptime *string
	var mkCPU, mkCPUAvail, mkMemUsed, mkMemSize, mkTemp, mkUptime *string
	var srvCPU, srvCPUAvail, srvMemUsed, srvMemSize, srvTemp, srvUptime *string
	var snmpOIDOverrides []byte
	var updated time.Time
	err := s.DB().QueryRow(r.Context(), `
		SELECT snmp_community, telnet_user, telnet_password, telnet_enable, ssh_user, ssh_password,
			olt_cpu_oid, olt_memory_used_oid, olt_memory_size_oid, olt_temp_oid, olt_uptime_oid, olt_cpu_available_oid,
			mikrotik_cpu_oid, mikrotik_memory_used_oid, mikrotik_memory_size_oid, mikrotik_temp_oid, mikrotik_uptime_oid, mikrotik_cpu_available_oid,
			server_cpu_oid, server_memory_used_oid, server_memory_size_oid, server_temp_oid, server_uptime_oid, server_cpu_available_oid,
			snmp_oid_overrides::text,
			updated_at
		FROM settings_connection_defaults WHERE id=1
	`).Scan(
		&snmp, &tu, &tp, &te, &su, &sp,
		&oltCPU, &oltMemUsed, &oltMemSize, &oltTemp, &oltUptime, &oltCPUAvail,
		&mkCPU, &mkMemUsed, &mkMemSize, &mkTemp, &mkUptime, &mkCPUAvail,
		&srvCPU, &srvMemUsed, &srvMemSize, &srvTemp, &srvUptime, &srvCPUAvail,
		&snmpOIDOverrides,
		&updated,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"snmp_community":             maskSecret(snmp),
		"snmp_community_value":       derefStrOr(snmp, ""),
		"snmp_community_configured":  hasSecret(snmp),
		"telnet_user":                tu,
		"telnet_password":            "***",
		"telnet_password_configured": hasSecret(tp),
		"telnet_enable":              "***",
		"telnet_enable_configured":   hasSecret(te),
		"ssh_user":                   su,
		"ssh_password":               "***",
		"ssh_password_configured":    hasSecret(sp),
		"oid_defaults": map[string]any{
			"olt": map[string]any{
				"cpu_oid":           derefStrOr(oltCPU, ""),
				"cpu_available_oid": derefStrOr(oltCPUAvail, ""),
				"memory_used_oid":   derefStrOr(oltMemUsed, ""),
				"memory_size_oid":   derefStrOr(oltMemSize, ""),
				"temp_oid":          derefStrOr(oltTemp, ""),
				"uptime_oid":        derefStrOr(oltUptime, ""),
			},
			"mikrotik": map[string]any{
				"cpu_oid":           derefStrOr(mkCPU, ""),
				"cpu_available_oid": derefStrOr(mkCPUAvail, ""),
				"memory_used_oid":   derefStrOr(mkMemUsed, ""),
				"memory_size_oid":   derefStrOr(mkMemSize, ""),
				"temp_oid":          derefStrOr(mkTemp, ""),
				"uptime_oid":        derefStrOr(mkUptime, ""),
			},
			"server": map[string]any{
				"cpu_oid":           derefStrOr(srvCPU, ""),
				"cpu_available_oid": derefStrOr(srvCPUAvail, ""),
				"memory_used_oid":   derefStrOr(srvMemUsed, ""),
				"memory_size_oid":   derefStrOr(srvMemSize, ""),
				"temp_oid":          derefStrOr(srvTemp, ""),
				"uptime_oid":        derefStrOr(srvUptime, ""),
			},
		},
		"snmp_oid_overrides": json.RawMessage(snmpOIDOverrides),
		"updated_at":         updated,
	})
}

func maskSecret(p *string) any {
	if p == nil || *p == "" {
		return nil
	}
	return "***"
}

func hasSecret(p *string) bool {
	return p != nil && strings.TrimSpace(*p) != ""
}

func (s *Server) patchConnectionDefaults(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SNMPCommunity         *string         `json:"snmp_community"`
		TelnetUser            *string         `json:"telnet_user"`
		TelnetPassword        *string         `json:"telnet_password"`
		TelnetEnable          *string         `json:"telnet_enable"`
		SSHUser               *string         `json:"ssh_user"`
		SSHPassword           *string         `json:"ssh_password"`
		OltCPUOID             *string         `json:"olt_cpu_oid"`
		OltCPUAvailOID        *string         `json:"olt_cpu_available_oid"`
		OltMemoryUsedOID      *string         `json:"olt_memory_used_oid"`
		OltMemorySizeOID      *string         `json:"olt_memory_size_oid"`
		OltTempOID            *string         `json:"olt_temp_oid"`
		OltUptimeOID          *string         `json:"olt_uptime_oid"`
		MikrotikCPUOID        *string         `json:"mikrotik_cpu_oid"`
		MikrotikCPUAvailOID   *string         `json:"mikrotik_cpu_available_oid"`
		MikrotikMemoryUsedOID *string         `json:"mikrotik_memory_used_oid"`
		MikrotikMemorySizeOID *string         `json:"mikrotik_memory_size_oid"`
		MikrotikTempOID       *string         `json:"mikrotik_temp_oid"`
		MikrotikUptimeOID     *string         `json:"mikrotik_uptime_oid"`
		ServerCPUOID          *string         `json:"server_cpu_oid"`
		ServerCPUAvailOID     *string         `json:"server_cpu_available_oid"`
		ServerMemoryUsedOID   *string         `json:"server_memory_used_oid"`
		ServerMemorySizeOID   *string         `json:"server_memory_size_oid"`
		ServerTempOID         *string         `json:"server_temp_oid"`
		ServerUptimeOID       *string         `json:"server_uptime_oid"`
		SNMPOIDOverrides      json.RawMessage `json:"snmp_oid_overrides"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	_, err := s.DB().Exec(r.Context(), `
		UPDATE settings_connection_defaults SET
			snmp_community = COALESCE($1, snmp_community),
			telnet_user = COALESCE($2, telnet_user),
			telnet_password = COALESCE($3, telnet_password),
			telnet_enable = COALESCE($4, telnet_enable),
			ssh_user = COALESCE($5, ssh_user),
			ssh_password = COALESCE($6, ssh_password),
			olt_cpu_oid = COALESCE($7, olt_cpu_oid),
			olt_memory_used_oid = COALESCE($8, olt_memory_used_oid),
			olt_memory_size_oid = COALESCE($9, olt_memory_size_oid),
			olt_temp_oid = COALESCE($10, olt_temp_oid),
			olt_uptime_oid = COALESCE($11, olt_uptime_oid),
			olt_cpu_available_oid = COALESCE($12, olt_cpu_available_oid),
			mikrotik_cpu_oid = COALESCE($13, mikrotik_cpu_oid),
			mikrotik_memory_used_oid = COALESCE($14, mikrotik_memory_used_oid),
			mikrotik_memory_size_oid = COALESCE($15, mikrotik_memory_size_oid),
			mikrotik_temp_oid = COALESCE($16, mikrotik_temp_oid),
			mikrotik_uptime_oid = COALESCE($17, mikrotik_uptime_oid),
			mikrotik_cpu_available_oid = COALESCE($18, mikrotik_cpu_available_oid),
			server_cpu_oid = COALESCE($19, server_cpu_oid),
			server_memory_used_oid = COALESCE($20, server_memory_used_oid),
			server_memory_size_oid = COALESCE($21, server_memory_size_oid),
			server_temp_oid = COALESCE($22, server_temp_oid),
			server_uptime_oid = COALESCE($23, server_uptime_oid),
			server_cpu_available_oid = COALESCE($24, server_cpu_available_oid),
			snmp_oid_overrides = CASE WHEN COALESCE($25::text, '') = '' THEN snmp_oid_overrides ELSE $25::jsonb END,
			updated_at = now()
		WHERE id=1
	`, body.SNMPCommunity, body.TelnetUser, body.TelnetPassword, body.TelnetEnable, body.SSHUser, body.SSHPassword,
		body.OltCPUOID, body.OltMemoryUsedOID, body.OltMemorySizeOID, body.OltTempOID, body.OltUptimeOID, body.OltCPUAvailOID,
		body.MikrotikCPUOID, body.MikrotikMemoryUsedOID, body.MikrotikMemorySizeOID, body.MikrotikTempOID, body.MikrotikUptimeOID, body.MikrotikCPUAvailOID,
		body.ServerCPUOID, body.ServerMemoryUsedOID, body.ServerMemorySizeOID, body.ServerTempOID, body.ServerUptimeOID, body.ServerCPUAvailOID, body.SNMPOIDOverrides)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	auditAfter := map[string]any{}
	auditSetMasked(auditAfter, "snmp_community", body.SNMPCommunity)
	auditSetStr(auditAfter, "telnet_user", body.TelnetUser)
	auditSetSecret(auditAfter, "telnet_password", body.TelnetPassword)
	auditSetSecret(auditAfter, "telnet_enable", body.TelnetEnable)
	auditSetStr(auditAfter, "ssh_user", body.SSHUser)
	auditSetSecret(auditAfter, "ssh_password", body.SSHPassword)
	auditSetOIDFields(auditAfter, map[string]*string{
		"olt_cpu_oid": body.OltCPUOID, "olt_cpu_available_oid": body.OltCPUAvailOID,
		"olt_memory_used_oid": body.OltMemoryUsedOID, "olt_memory_size_oid": body.OltMemorySizeOID,
		"olt_temp_oid": body.OltTempOID, "olt_uptime_oid": body.OltUptimeOID,
		"mikrotik_cpu_oid": body.MikrotikCPUOID, "mikrotik_cpu_available_oid": body.MikrotikCPUAvailOID,
		"mikrotik_memory_used_oid": body.MikrotikMemoryUsedOID, "mikrotik_memory_size_oid": body.MikrotikMemorySizeOID,
		"mikrotik_temp_oid": body.MikrotikTempOID, "mikrotik_uptime_oid": body.MikrotikUptimeOID,
		"server_cpu_oid": body.ServerCPUOID, "server_cpu_available_oid": body.ServerCPUAvailOID,
		"server_memory_used_oid": body.ServerMemoryUsedOID, "server_memory_size_oid": body.ServerMemorySizeOID,
		"server_temp_oid": body.ServerTempOID, "server_uptime_oid": body.ServerUptimeOID,
	})
	if len(body.SNMPOIDOverrides) > 0 {
		auditAfter["snmp_oid_overrides"] = true
	}
	s.appendAuditLog(r.Context(), "settings_connection_defaults", "1", "patch", actorFromRequest(r), nil, auditAfter)
	s.getConnectionDefaults(w, r)
}

func (s *Server) getTelegramMonitoring(w http.ResponseWriter, r *http.Request) {
	s.getTelegram(w, r, "monitoring")
}
func (s *Server) patchTelegramMonitoring(w http.ResponseWriter, r *http.Request) {
	s.patchTelegram(w, r, "monitoring")
}
func (s *Server) testTelegramMonitoring(w http.ResponseWriter, r *http.Request) {
	s.testTelegramByID(w, r, "monitoring", "Teste de alerta de monitoramento")
}

func (s *Server) getTelegramReports(w http.ResponseWriter, r *http.Request) {
	s.getTelegram(w, r, "reports")
}
func (s *Server) patchTelegramReports(w http.ResponseWriter, r *http.Request) {
	s.patchTelegram(w, r, "reports")
}
func (s *Server) testTelegramReports(w http.ResponseWriter, r *http.Request) {
	s.testTelegramByID(w, r, "reports", "Teste de envio de relatório")
}

func (s *Server) getTelegram(w http.ResponseWriter, r *http.Request, id string) {
	var tok, chat, topic *string
	err := s.DB().QueryRow(r.Context(), `SELECT bot_token, chat_id, topic_id FROM settings_telegram WHERE id=$1`, id).Scan(&tok, &chat, &topic)
	if err == pgx.ErrNoRows {
		writeJSON(w, http.StatusOK, map[string]any{"id": id, "bot_token": nil, "chat_id": nil, "topic_id": nil})
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "bot_token": maskSecret(tok), "chat_id": chat, "topic_id": topic})
}

func (s *Server) patchTelegram(w http.ResponseWriter, r *http.Request, id string) {
	var body struct {
		BotToken *string `json:"bot_token"`
		ChatID   *string `json:"chat_id"`
		TopicID  *string `json:"topic_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	_, err := s.DB().Exec(r.Context(), `
		INSERT INTO settings_telegram (id, bot_token, chat_id, topic_id) VALUES ($1,$2,$3,$4)
		ON CONFLICT (id) DO UPDATE SET
			bot_token = COALESCE(EXCLUDED.bot_token, settings_telegram.bot_token),
			chat_id = COALESCE(EXCLUDED.chat_id, settings_telegram.chat_id),
			topic_id = COALESCE(EXCLUDED.topic_id, settings_telegram.topic_id),
			updated_at = now()
	`, id, body.BotToken, body.ChatID, body.TopicID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "settings_telegram", id, "patch", actorFromRequest(r), nil, body)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) testTelegramByID(w http.ResponseWriter, r *http.Request, id, title string) {
	cfg, err := telegramclient.LoadConfig(r.Context(), s.DB(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if !cfg.Ready() {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":      false,
			"sent":    false,
			"message": "Configuração Telegram incompleta: defina bot_token e chat_id.",
		})
		return
	}
	text := fmt.Sprintf("NetQuasar\n%s\n%s", title, time.Now().Format(time.RFC3339))
	if err := telegramclient.SendMessage(r.Context(), cfg, text); err != nil {
		writeErr(w, http.StatusBadGateway, "TELEGRAM_SEND_FAILED", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "settings_telegram", id, "test_send", actorFromRequest(r), nil, map[string]any{
		"title": title,
		"sent":  true,
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "sent": true})
}

func (s *Server) listOltVendors(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB().Query(r.Context(), `SELECT brand FROM olt_vendor_profiles ORDER BY brand`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	var brands []string
	for rows.Next() {
		var b string
		_ = rows.Scan(&b)
		brands = append(brands, b)
	}
	writeJSON(w, http.StatusOK, map[string]any{"brands": brands})
}

func (s *Server) getOltVendor(w http.ResponseWriter, r *http.Request) {
	b := chi.URLParam(r, "brand")
	var onu, pon, trx, base *string
	err := s.DB().QueryRow(r.Context(), `
		SELECT onu_online_oid, pon_status_oid, transceiver_oid, snmp_base_oid FROM olt_vendor_profiles WHERE brand=$1
	`, b).Scan(&onu, &pon, &trx, &base)
	if err == pgx.ErrNoRows {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "marca não cadastrada", nil)
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"brand": b, "onu_online_oid": onu, "pon_status_oid": pon, "transceiver_oid": trx, "snmp_base_oid": base})
}

func (s *Server) patchOltVendor(w http.ResponseWriter, r *http.Request) {
	b := chi.URLParam(r, "brand")
	var body map[string]*string
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	_, err := s.DB().Exec(r.Context(), `
		INSERT INTO olt_vendor_profiles (brand, onu_online_oid, pon_status_oid, transceiver_oid, snmp_base_oid)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (brand) DO UPDATE SET
			onu_online_oid = COALESCE(EXCLUDED.onu_online_oid, olt_vendor_profiles.onu_online_oid),
			pon_status_oid = COALESCE(EXCLUDED.pon_status_oid, olt_vendor_profiles.pon_status_oid),
			transceiver_oid = COALESCE(EXCLUDED.transceiver_oid, olt_vendor_profiles.transceiver_oid),
			snmp_base_oid = COALESCE(EXCLUDED.snmp_base_oid, olt_vendor_profiles.snmp_base_oid),
			updated_at = now()
	`, b, body["onu_online_oid"], body["pon_status_oid"], body["transceiver_oid"], body["snmp_base_oid"])
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "olt_vendor_profile", b, "patch", actorFromRequest(r), nil, body)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func bearerFromRequest(r *http.Request) string {
	return strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
}

// requireSettingsUsersAdmin exige JWT de administrador quando a API está em modo autenticado.
func (s *Server) requireSettingsUsersAdmin(w http.ResponseWriter, r *http.Request) bool {
	if !s.Cfg.RequireAuth() {
		return true
	}
	raw := bearerFromRequest(r)
	if raw == "" {
		writeErr(w, http.StatusUnauthorized, "UNAUTHORIZED", "sessão ausente", nil)
		return false
	}
	_, _, role, err := parseUserJWT(s.Cfg, raw)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "UNAUTHORIZED", "sessão inválida", nil)
		return false
	}
	if role != "admin" {
		writeErr(w, http.StatusForbidden, "FORBIDDEN", "apenas administradores podem gerir usuários", nil)
		return false
	}
	return true
}

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	if !s.requireSettingsUsersAdmin(w, r) {
		return
	}
	rows, err := s.DB().Query(r.Context(), `
		SELECT id, display_name, email, phone, role FROM users ORDER BY display_name
	`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	type u struct {
		ID          uuid.UUID `json:"id"`
		DisplayName string    `json:"display_name"`
		Email       string    `json:"email"`
		Phone       *string   `json:"phone"`
		Role        string    `json:"role"`
	}
	var list []u
	for rows.Next() {
		var x u
		var ph sql.NullString
		if err := rows.Scan(&x.ID, &x.DisplayName, &x.Email, &ph, &x.Role); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		if ph.Valid {
			s := ph.String
			x.Phone = &s
		}
		list = append(list, x)
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": list})
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	if !s.requireSettingsUsersAdmin(w, r) {
		return
	}
	var body struct {
		DisplayName string  `json:"display_name"`
		Email       string  `json:"email"`
		Phone       *string `json:"phone"`
		Password    string  `json:"password"`
		Role        string  `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	if strings.TrimSpace(body.DisplayName) == "" || strings.TrimSpace(body.Email) == "" || body.Password == "" {
		writeErr(w, 422, "VALIDATION", "display_name, email, telefone e password são obrigatórios", nil)
		return
	}
	if body.Role != "admin" && body.Role != "viewer" {
		writeErr(w, 422, "VALIDATION", "role deve ser admin ou viewer", nil)
		return
	}
	if body.Phone == nil || strings.TrimSpace(*body.Phone) == "" {
		writeErr(w, 422, "VALIDATION", "telefone é obrigatório (DDD, 10 ou 11 dígitos)", nil)
		return
	}
	phoneNorm, err := normalizeBRPhone(*body.Phone)
	if err != nil {
		writeErr(w, 422, "VALIDATION", err.Error(), nil)
		return
	}
	email := strings.ToLower(strings.TrimSpace(body.Email))
	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "CRYPTO", err.Error(), nil)
		return
	}
	var id uuid.UUID
	err = s.DB().QueryRow(r.Context(), `
		INSERT INTO users (display_name, email, phone, password_hash, role)
		VALUES ($1,$2,$3,$4,$5) RETURNING id
	`, strings.TrimSpace(body.DisplayName), email, phoneNorm, string(hash), body.Role).Scan(&id)
	if err != nil {
		if strings.Contains(err.Error(), "unique") || strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			writeErr(w, http.StatusConflict, "DUPLICATE", "e-mail já registado", nil)
			return
		}
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "user", id.String(), "create", actorFromRequest(r), nil, map[string]any{
		"email": email, "role": body.Role, "display_name": strings.TrimSpace(body.DisplayName),
	})
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (s *Server) getUser(w http.ResponseWriter, r *http.Request) {
	if !s.requireSettingsUsersAdmin(w, r) {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "UUID inválido", nil)
		return
	}
	var displayName, email, role string
	var ph sql.NullString
	var created any
	err = s.DB().QueryRow(r.Context(), `
		SELECT display_name, email, phone, role, created_at FROM users WHERE id=$1
	`, id).Scan(&displayName, &email, &ph, &role, &created)
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "usuário não encontrado", nil)
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	var phonePtr *string
	if ph.Valid {
		s := ph.String
		phonePtr = &s
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":           id,
		"display_name": displayName,
		"email":        email,
		"phone":        phonePtr,
		"role":         role,
		"created_at":   created,
	})
}

func (s *Server) patchUser(w http.ResponseWriter, r *http.Request) {
	if !s.requireSettingsUsersAdmin(w, r) {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "UUID inválido", nil)
		return
	}
	var body struct {
		DisplayName *string `json:"display_name"`
		Email       *string `json:"email"`
		Phone       *string `json:"phone"`
		Password    *string `json:"password"`
		Role        *string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	if body.Role != nil && *body.Role != "" && *body.Role != "admin" && *body.Role != "viewer" {
		writeErr(w, 422, "VALIDATION", "role deve ser admin ou viewer", nil)
		return
	}
	if body.Password != nil && *body.Password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(*body.Password), bcrypt.DefaultCost)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "CRYPTO", err.Error(), nil)
			return
		}
		_, err = s.DB().Exec(r.Context(), `UPDATE users SET password_hash=$2, updated_at=now() WHERE id=$1`, id, string(hash))
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
	}
	if body.DisplayName != nil || body.Email != nil || body.Phone != nil || body.Role != nil {
		phoneProvided := body.Phone != nil
		var phoneArg any
		if body.Phone != nil {
			t := strings.TrimSpace(*body.Phone)
			if t == "" {
				writeErr(w, 422, "VALIDATION", "telefone é obrigatório (DDD, 10 ou 11 dígitos)", nil)
				return
			}
			norm, err := normalizeBRPhone(t)
			if err != nil {
				writeErr(w, 422, "VALIDATION", err.Error(), nil)
				return
			}
			phoneArg = norm
		}
		_, err = s.DB().Exec(r.Context(), `
			UPDATE users SET
				display_name = COALESCE($2::text, display_name),
				email = COALESCE($3::text, email),
				phone = CASE WHEN $4 THEN $5::text ELSE phone END,
				role = COALESCE($6::text, role),
				updated_at = now()
			WHERE id=$1
		`, id, strPtrOrNil(body.DisplayName, true), emailPtrOrNil(body.Email), phoneProvided, phoneArg, strPtrOrNil(body.Role, true))
		if err != nil {
			if strings.Contains(err.Error(), "unique") || strings.Contains(strings.ToLower(err.Error()), "duplicate") {
				writeErr(w, http.StatusConflict, "DUPLICATE", "e-mail já registado", nil)
				return
			}
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
	}
	s.appendAuditLog(r.Context(), "user", id.String(), "patch", actorFromRequest(r), nil, body)
	s.getUser(w, r)
}

func strPtrOrNil(p *string, trim bool) any {
	if p == nil {
		return nil
	}
	t := *p
	if trim {
		t = strings.TrimSpace(t)
	}
	if t == "" {
		return nil
	}
	return t
}

func emailPtrOrNil(p *string) any {
	if p == nil {
		return nil
	}
	t := strings.ToLower(strings.TrimSpace(*p))
	if t == "" {
		return nil
	}
	return t
}

func (s *Server) deleteUser(w http.ResponseWriter, r *http.Request) {
	if !s.requireSettingsUsersAdmin(w, r) {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "UUID inválido", nil)
		return
	}
	ct, err := s.DB().Exec(r.Context(), `DELETE FROM users WHERE id=$1`, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if ct.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "usuário não encontrado", nil)
		return
	}
	s.appendAuditLog(r.Context(), "user", id.String(), "delete", actorFromRequest(r), nil, nil)
	w.WriteHeader(http.StatusNoContent)
}
