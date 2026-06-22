// Package api — exportação/importação do pacote de configuração do sistema NetQuasar.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/config"
	"github.com/netquasar/netquasar/quasar_backend/internal/monitorworker"
	"golang.org/x/crypto/bcrypt"
)

const systemConfigExportVersion = 1

type sysConfigImportJob struct {
	ID          string     `json:"job_id"`
	Status      string     `json:"status"`
	ProgressPct int        `json:"progress_pct"`
	CurrentStep string     `json:"current_step"`
	StepsTotal  int        `json:"steps_total"`
	StepsDone   int        `json:"steps_done"`
	Logs        []string   `json:"logs"`
	Errors      []string   `json:"errors"`
	StartedAt   time.Time  `json:"started_at"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
}

type systemConfigImportOptions struct {
	ApplyDatabaseConnection bool `json:"apply_database_connection"`
	ImportUsers             bool `json:"import_users"`
	OverwriteAlertRules     bool `json:"overwrite_alert_rules"`
}

type systemConfigImportRequest struct {
	Bundle  map[string]any            `json:"bundle"`
	Options systemConfigImportOptions `json:"options"`
}

type systemConfigStep struct {
	Key   string
	Title string
	Apply func(ctx context.Context, s *Server, sections map[string]any, opts systemConfigImportOptions) error
}

func queryJSONRow(ctx context.Context, pool *pgxpool.Pool, innerSQL string, args ...any) (map[string]any, error) {
	q := `SELECT COALESCE(row_to_json(t), '{}'::json)::text FROM (` + innerSQL + `) t`
	var raw string
	if err := pool.QueryRow(ctx, q, args...).Scan(&raw); err != nil {
		return nil, err
	}
	out := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	return out, nil
}

func queryJSONArray(ctx context.Context, pool *pgxpool.Pool, innerSQL string, args ...any) ([]any, error) {
	q := `SELECT COALESCE(json_agg(row_to_json(t)), '[]'::json)::text FROM (` + innerSQL + `) t`
	var raw string
	if err := pool.QueryRow(ctx, q, args...).Scan(&raw); err != nil {
		return nil, err
	}
	var out []any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	return out, nil
}

func buildEnvironmentSnapshot(cfg *config.Config) map[string]any {
	vars := map[string]string{}
	for _, kv := range os.Environ() {
		i := strings.IndexByte(kv, '=')
		if i <= 0 {
			continue
		}
		key := kv[:i]
		if strings.HasPrefix(key, "NETQUASAR_") {
			vars[key] = kv[i+1:]
		}
	}
	return map[string]any{
		"_comment": "Snapshot NETQUASAR_* do processo. Contém segredos — guarde em local seguro.",
		"process_env": vars,
		"resolved_config": map[string]any{
			"http_addr": cfg.HTTPAddr, "log_level": cfg.LogLevel, "cors_origins": cfg.CORSOrigins,
			"database_url": cfg.DatabaseURL, "db_host": cfg.DBHost, "db_port": cfg.DBPort,
			"db_user": cfg.DBUser, "db_name": cfg.DBName, "db_ssl_mode": cfg.DBSSLMode,
			"db_ssl_root_cert": cfg.DBSSLRootCert, "db_password": cfg.DBPassword,
			"redis_url": cfg.RedisURL, "api_keys": cfg.APIKeys, "session_secret": cfg.SessionSecret,
			"embedded_ui": cfg.EmbeddedUI,
		},
	}
}

func (s *Server) collectSystemConfigurationBundle(ctx context.Context) (map[string]any, error) {
	pool := s.DB()
	if pool == nil {
		return nil, fmt.Errorf("base de dados indisponível")
	}

	databaseMeta, err := queryJSONRow(ctx, pool, `
		SELECT id, host, port, db_user, db_name, ssl_mode,
			(db_password IS NOT NULL AND db_password <> '') AS password_configured,
			db_password, updated_at FROM settings_database_meta WHERE id=1`)
	if err != nil && err != pgx.ErrNoRows {
		return nil, fmt.Errorf("database_meta: %w", err)
	}

	connectionDefaults, _ := queryJSONRow(ctx, pool, `SELECT * FROM settings_connection_defaults WHERE id=1`)
	uiAppearance, _ := queryJSONRow(ctx, pool, `SELECT * FROM settings_ui WHERE id=1`)
	monitoringIntervals, _ := queryJSONRow(ctx, pool, `SELECT * FROM monitoring_intervals WHERE id=1`)
	monitoringSettings, _ := queryJSONRow(ctx, pool, `SELECT * FROM monitoring_settings WHERE id=1`)
	nightlyCollection, _ := queryJSONRow(ctx, pool, `SELECT * FROM nightly_collection_settings WHERE id=1`)
	mikrotikCollection, _ := queryJSONRow(ctx, pool, `SELECT * FROM settings_mikrotik_collection WHERE id=1`)
	smtpSettings, _ := queryJSONRow(ctx, pool, `SELECT * FROM settings_smtp WHERE id=1`)
	alertRules, _ := queryJSONArray(ctx, pool, `SELECT id, name, enabled, condition_json, channels_json, created_at, updated_at FROM alert_rules ORDER BY name`)
	telegramRows, _ := queryJSONArray(ctx, pool, `SELECT id, bot_token, chat_id, topic_id, updated_at FROM settings_telegram ORDER BY id`)
	oltProfiles, _ := queryJSONArray(ctx, pool, `SELECT * FROM olt_vendor_profiles ORDER BY brand`)
	oltModels, _ := queryJSONArray(ctx, pool, `SELECT * FROM olt_vendor_models ORDER BY brand, model`)
	integrations, _ := queryJSONArray(ctx, pool, `
		SELECT id, slug, name, description, enabled, auth_type, base_url,
			default_headers, variables, auth_config, timeout_ms, tls_insecure, consumer_config,
			created_at, updated_at FROM integrations ORDER BY slug`)
	users, _ := queryJSONArray(ctx, pool, `SELECT id, display_name, email, phone, role, created_at, updated_at FROM users ORDER BY email`)
	automationOnu, _ := queryJSONRow(ctx, pool, `SELECT * FROM automation_onu_report WHERE id=1`)
	automationDigest, _ := queryJSONRow(ctx, pool, `SELECT * FROM automation_alerts_digest WHERE id=1`)
	automationCommercial, _ := queryJSONRow(ctx, pool, `SELECT * FROM automation_commercial_report WHERE id=1`)

	return map[string]any{
		"_schema": map[string]any{
			"kind": "netquasar_system_configuration", "version": systemConfigExportVersion,
			"exported_at": time.Now().UTC().Format(time.RFC3339),
			"description": "Backup de definições NetQuasar (PostgreSQL + ambiente). Sem inventário operacional.",
		},
		"environment_variables": buildEnvironmentSnapshot(s.Cfg),
		"sections": map[string]any{
			"database_connection":           sectionWrap("Metadados PostgreSQL (settings_database_meta).", databaseMeta, false),
			"connection_defaults":           sectionWrap("SNMP/SSH/Telnet por defeito.", connectionDefaults, false),
			"ui_appearance":                 sectionWrap("Tema global (settings_ui).", uiAppearance, false),
			"monitoring_intervals":          sectionWrap("Intervalos e pipeline.", monitoringIntervals, false),
			"monitoring_settings":           sectionWrap("Opções gerais de monitoramento.", monitoringSettings, false),
			"nightly_collection":            sectionWrap("Coleta nocturna.", nightlyCollection, false),
			"mikrotik_collection":           sectionWrap("Perfil SNMP MikroTik.", mikrotikCollection, false),
			"smtp":                          sectionWrap("SMTP para e-mail.", smtpSettings, false),
			"telegram":                      sectionWrap("Telegram monitoring/reports.", telegramRows, true),
			"alert_rules":                   sectionWrap("Regras de alerta.", alertRules, true),
			"olt_vendor_profiles":           sectionWrap("Marcas OLT.", oltProfiles, true),
			"olt_vendor_models":             sectionWrap("Modelos OLT.", oltModels, true),
			"integrations":                  sectionWrap("Integrações ERP/CRM.", integrations, true),
			"users":                         sectionWrap("Utilizadores (sem password_hash).", users, true),
			"automation_onu_monthly_report": sectionWrap("Agendamento ONU.", automationOnu, false),
			"automation_alerts_digest":      sectionWrap("Agendamento resumo alertas.", automationDigest, false),
			"automation_commercial_report":  sectionWrap("Agendamento comercial.", automationCommercial, false),
		},
	}, nil
}

func sectionWrap(comment string, data any, isRows bool) map[string]any {
	out := map[string]any{"_comment": comment}
	if isRows {
		out["rows"] = data
	} else {
		out["data"] = data
	}
	return out
}

func sectionData(sections map[string]any, key string) (any, bool) {
	sec, ok := sections[key].(map[string]any)
	if !ok {
		return nil, false
	}
	if d, ok := sec["data"]; ok {
		return d, true
	}
	if r, ok := sec["rows"]; ok {
		return r, true
	}
	return nil, false
}

func importSteps() []systemConfigStep {
	return []systemConfigStep{
		{"connection_defaults", "Credenciais SNMP/SSH por defeito", applyConnectionDefaultsImport},
		{"ui_appearance", "Aparência da interface", applyUIAppearanceImport},
		{"monitoring_intervals", "Intervalos e pipeline", applyMonitoringIntervalsImport},
		{"monitoring_settings", "Definições de monitoramento", applyMonitoringSettingsImport},
		{"nightly_collection", "Coleta nocturna", applyNightlyCollectionImport},
		{"mikrotik_collection", "Perfil MikroTik", applyMikrotikCollectionImport},
		{"smtp", "SMTP", applySMTPImport},
		{"telegram", "Telegram", applyTelegramImport},
		{"alert_rules", "Regras de alerta", applyAlertRulesImport},
		{"olt_vendor_profiles", "Perfis OLT (marcas)", applyOltProfilesImport},
		{"olt_vendor_models", "Modelos OLT", applyOltModelsImport},
		{"integrations", "Integrações", applyIntegrationsImport},
		{"automation_onu_monthly_report", "Automação ONU", applyAutomationOnuImport},
		{"automation_alerts_digest", "Automação resumo alertas", applyAutomationDigestImport},
		{"automation_commercial_report", "Automação comercial", applyAutomationCommercialImport},
		{"users", "Utilizadores", applyUsersImport},
		{"database_connection", "Ligação PostgreSQL", applyDatabaseMetaImport},
	}
}

// updateSingletonFromJSON actualiza a linha id=1 de tabelas singleton via json_populate_record.
func updateSingletonFromJSON(ctx context.Context, pool *pgxpool.Pool, table string, data any, skipCols ...string) error {
	skip := map[string]bool{"id": true}
	for _, c := range skipCols {
		skip[c] = true
	}
	m := map[string]any{}
	if err := json.Unmarshal(mustJSON(data), &m); err != nil {
		return err
	}
	m["id"] = 1
	payload := string(mustJSON(m))

	rows, err := pool.Query(ctx, `
		SELECT column_name FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = $1
		ORDER BY ordinal_position`, table)
	if err != nil {
		return err
	}
	defer rows.Close()
	var setParts []string
	for rows.Next() {
		var col string
		if err := rows.Scan(&col); err != nil {
			return err
		}
		if skip[col] {
			continue
		}
		setParts = append(setParts, fmt.Sprintf("%s = s.%s", col, col))
	}
	if len(setParts) == 0 {
		return nil
	}
	q := fmt.Sprintf(`
		UPDATE %s AS t SET %s
		FROM (SELECT * FROM json_populate_record(NULL::%s, $1::json)) AS s
		WHERE t.id = s.id`, table, strings.Join(setParts, ", "), table)
	_, err = pool.Exec(ctx, q, payload)
	return err
}

func applyConnectionDefaultsImport(ctx context.Context, s *Server, sections map[string]any, _ systemConfigImportOptions) error {
	data, ok := sectionData(sections, "connection_defaults")
	if !ok {
		return nil
	}
	return updateSingletonFromJSON(ctx, s.DB(), "settings_connection_defaults", data)
}

func applyUIAppearanceImport(ctx context.Context, s *Server, sections map[string]any, _ systemConfigImportOptions) error {
	data, ok := sectionData(sections, "ui_appearance")
	if !ok {
		return nil
	}
	return updateSingletonFromJSON(ctx, s.DB(), "settings_ui", data)
}

func applyMonitoringIntervalsImport(ctx context.Context, s *Server, sections map[string]any, _ systemConfigImportOptions) error {
	data, ok := sectionData(sections, "monitoring_intervals")
	if !ok {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(mustJSON(data), &m); err != nil {
		return err
	}
	psJSON, _ := json.Marshal(m["pipeline_steps"])
	_, err := s.DB().Exec(ctx, `
		UPDATE monitoring_intervals SET
			ping_seconds = COALESCE($2::int, ping_seconds), telemetry_minutes = COALESCE($3::int, telemetry_minutes),
			ping_timeout_ms = COALESCE($4::int, ping_timeout_ms), telemetry_seconds = COALESCE($5::int, telemetry_seconds),
			interface_snapshot_seconds = COALESCE($6::int, interface_snapshot_seconds),
			olt_if_derived_pon_seconds = COALESCE($7::int, olt_if_derived_pon_seconds),
			telemetry_timeout_ms = COALESCE($8::int, telemetry_timeout_ms),
			interface_snapshot_timeout_ms = COALESCE($9::int, interface_snapshot_timeout_ms),
			olt_if_derived_pon_timeout_ms = COALESCE($10::int, olt_if_derived_pon_timeout_ms),
			icmp_payload_bytes = COALESCE($11::int, icmp_payload_bytes),
			offline_ping_fail_threshold = COALESCE($12::int, offline_ping_fail_threshold),
			uptime_restart_alert_minutes = COALESCE($13::int, uptime_restart_alert_minutes),
			pipeline_cycle_seconds = COALESCE($14::int, pipeline_cycle_seconds),
			mikrotik_timeout_ms = COALESCE($15::int, mikrotik_timeout_ms),
			ping_parallel = COALESCE($16::boolean, ping_parallel),
			pipeline_steps = COALESCE($17::jsonb, pipeline_steps), updated_at = now()
		WHERE id=1`,
		1, intField(m, "ping_seconds"), intField(m, "telemetry_minutes"), intField(m, "ping_timeout_ms"),
		intField(m, "telemetry_seconds"), intField(m, "interface_snapshot_seconds"), intField(m, "olt_if_derived_pon_seconds"),
		intField(m, "telemetry_timeout_ms"), intField(m, "interface_snapshot_timeout_ms"), intField(m, "olt_if_derived_pon_timeout_ms"),
		intField(m, "icmp_payload_bytes"), intField(m, "offline_ping_fail_threshold"), intField(m, "uptime_restart_alert_minutes"),
		intField(m, "pipeline_cycle_seconds"), intField(m, "mikrotik_timeout_ms"), boolField(m, "ping_parallel"), psJSON)
	return err
}

func applyMonitoringSettingsImport(ctx context.Context, s *Server, sections map[string]any, _ systemConfigImportOptions) error {
	data, ok := sectionData(sections, "monitoring_settings")
	if !ok {
		return nil
	}
	return updateSingletonFromJSON(ctx, s.DB(), "monitoring_settings", data)
}

func applyNightlyCollectionImport(ctx context.Context, s *Server, sections map[string]any, _ systemConfigImportOptions) error {
	data, ok := sectionData(sections, "nightly_collection")
	if !ok {
		return nil
	}
	return updateSingletonFromJSON(ctx, s.DB(), "nightly_collection_settings", data)
}

func applyMikrotikCollectionImport(ctx context.Context, s *Server, sections map[string]any, _ systemConfigImportOptions) error {
	data, ok := sectionData(sections, "mikrotik_collection")
	if !ok {
		return nil
	}
	return updateSingletonFromJSON(ctx, s.DB(), "settings_mikrotik_collection", data)
}

func applySMTPImport(ctx context.Context, s *Server, sections map[string]any, _ systemConfigImportOptions) error {
	data, ok := sectionData(sections, "smtp")
	if !ok {
		return nil
	}
	return updateSingletonFromJSON(ctx, s.DB(), "settings_smtp", data)
}

func applyTelegramImport(ctx context.Context, s *Server, sections map[string]any, _ systemConfigImportOptions) error {
	rows, ok := sectionData(sections, "telegram")
	if !ok {
		return nil
	}
	arr, _ := rows.([]any)
	for _, row := range arr {
		m, ok := row.(map[string]any)
		if !ok || strField(m, "id") == "" {
			continue
		}
		if _, err := s.DB().Exec(ctx, `
			INSERT INTO settings_telegram (id, bot_token, chat_id, topic_id, updated_at) VALUES ($1, $2, $3, $4, now())
			ON CONFLICT (id) DO UPDATE SET bot_token = COALESCE(EXCLUDED.bot_token, settings_telegram.bot_token),
				chat_id = COALESCE(EXCLUDED.chat_id, settings_telegram.chat_id),
				topic_id = COALESCE(EXCLUDED.topic_id, settings_telegram.topic_id), updated_at = now()`,
			strField(m, "id"), strField(m, "bot_token"), strField(m, "chat_id"), strField(m, "topic_id")); err != nil {
			return err
		}
	}
	return nil
}

func applyAlertRulesImport(ctx context.Context, s *Server, sections map[string]any, opts systemConfigImportOptions) error {
	rows, ok := sectionData(sections, "alert_rules")
	if !ok {
		return nil
	}
	if opts.OverwriteAlertRules {
		_, _ = s.DB().Exec(ctx, `DELETE FROM alert_rules`)
	}
	arr, _ := rows.([]any)
	for _, row := range arr {
		m, ok := row.(map[string]any)
		if !ok || strField(m, "name") == "" {
			continue
		}
		cj, _ := json.Marshal(m["condition_json"])
		ch, _ := json.Marshal(m["channels_json"])
		if _, err := s.DB().Exec(ctx, `
			INSERT INTO alert_rules (id, name, enabled, condition_json, channels_json)
			VALUES (COALESCE(NULLIF($1,'')::uuid, gen_random_uuid()), $2, COALESCE($3, true), $4::jsonb, $5::jsonb)
			ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, enabled = EXCLUDED.enabled,
				condition_json = EXCLUDED.condition_json, channels_json = EXCLUDED.channels_json, updated_at = now()`,
			strField(m, "id"), strField(m, "name"), boolField(m, "enabled"), cj, ch); err != nil {
			return err
		}
	}
	return nil
}

func applyOltProfilesImport(ctx context.Context, s *Server, sections map[string]any, _ systemConfigImportOptions) error {
	rows, ok := sectionData(sections, "olt_vendor_profiles")
	if !ok {
		return nil
	}
	for _, row := range rows.([]any) {
		m, _ := row.(map[string]any)
		if strField(m, "brand") == "" {
			continue
		}
		if _, err := s.DB().Exec(ctx, `
			INSERT INTO olt_vendor_profiles (brand, onu_online_oid, pon_status_oid, transceiver_oid, snmp_base_oid)
			VALUES ($1,$2,$3,$4,$5) ON CONFLICT (brand) DO UPDATE SET
				onu_online_oid = EXCLUDED.onu_online_oid, pon_status_oid = EXCLUDED.pon_status_oid,
				transceiver_oid = EXCLUDED.transceiver_oid, snmp_base_oid = EXCLUDED.snmp_base_oid`,
			strField(m, "brand"), strField(m, "onu_online_oid"), strField(m, "pon_status_oid"),
			strField(m, "transceiver_oid"), strField(m, "snmp_base_oid")); err != nil {
			return err
		}
	}
	return nil
}

func applyOltModelsImport(ctx context.Context, s *Server, sections map[string]any, _ systemConfigImportOptions) error {
	rows, ok := sectionData(sections, "olt_vendor_models")
	if !ok {
		return nil
	}
	for _, row := range rows.([]any) {
		m, _ := row.(map[string]any)
		if strField(m, "brand") == "" || strField(m, "model") == "" {
			continue
		}
		cj, _ := json.Marshal(m["collection_steps"])
		om, _ := json.Marshal(m["onu_metrics"])
		or, _ := json.Marshal(m["onu_report_commands"])
		pt, _ := json.Marshal(m["pon_telnet_commands"])
		if _, err := s.DB().Exec(ctx, `
			INSERT INTO olt_vendor_models (brand, model, onu_online_oid, pon_status_oid, transceiver_oid, snmp_base_oid, collection_steps, onu_metrics, onu_report_commands, pon_telnet_commands)
			VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb,$8::jsonb,$9::jsonb,$10::jsonb)
			ON CONFLICT (brand, model) DO UPDATE SET
				onu_online_oid = EXCLUDED.onu_online_oid, pon_status_oid = EXCLUDED.pon_status_oid,
				transceiver_oid = EXCLUDED.transceiver_oid, snmp_base_oid = EXCLUDED.snmp_base_oid,
				collection_steps = EXCLUDED.collection_steps, onu_metrics = EXCLUDED.onu_metrics,
				onu_report_commands = EXCLUDED.onu_report_commands,
				pon_telnet_commands = EXCLUDED.pon_telnet_commands`,
			strField(m, "brand"), strField(m, "model"), strField(m, "onu_online_oid"), strField(m, "pon_status_oid"),
			strField(m, "transceiver_oid"), strField(m, "snmp_base_oid"), cj, om, or, pt); err != nil {
			return err
		}
	}
	return nil
}

func applyIntegrationsImport(ctx context.Context, s *Server, sections map[string]any, _ systemConfigImportOptions) error {
	rows, ok := sectionData(sections, "integrations")
	if !ok {
		return nil
	}
	for _, row := range rows.([]any) {
		m, _ := row.(map[string]any)
		if strField(m, "slug") == "" {
			continue
		}
		defHdr, _ := json.Marshal(m["default_headers"])
		vars, _ := json.Marshal(m["variables"])
		authCfg, _ := json.Marshal(m["auth_config"])
		conJ, _ := json.Marshal(m["consumer_config"])
		if _, err := s.DB().Exec(ctx, `
			INSERT INTO integrations (id, slug, name, description, enabled, auth_type, base_url,
				default_headers, variables, auth_config, timeout_ms, tls_insecure, consumer_config)
			VALUES (COALESCE(NULLIF($1,'')::uuid, gen_random_uuid()), $2, $3, $4, COALESCE($5,true), $6, $7,
				COALESCE($8::jsonb, '{}'::jsonb), COALESCE($9::jsonb, '{}'::jsonb),
				COALESCE($10::jsonb, '{}'::jsonb), COALESCE($11, 15000), COALESCE($12, false), COALESCE($13::jsonb, '{}'::jsonb))
			ON CONFLICT (slug) DO UPDATE SET
				name = EXCLUDED.name, description = EXCLUDED.description, enabled = EXCLUDED.enabled,
				auth_type = EXCLUDED.auth_type, base_url = EXCLUDED.base_url,
				default_headers = EXCLUDED.default_headers, variables = EXCLUDED.variables,
				auth_config = EXCLUDED.auth_config, timeout_ms = EXCLUDED.timeout_ms,
				tls_insecure = EXCLUDED.tls_insecure, consumer_config = EXCLUDED.consumer_config,
				updated_at = now()`,
			strField(m, "id"), strField(m, "slug"), strField(m, "name"), nullStrField(m, "description"),
			boolField(m, "enabled"), strField(m, "auth_type"), strField(m, "base_url"),
			string(defHdr), string(vars), string(authCfg), intField(m, "timeout_ms"), boolField(m, "tls_insecure"), string(conJ)); err != nil {
			return err
		}
	}
	return nil
}

func applyAutomationOnuImport(ctx context.Context, s *Server, sections map[string]any, _ systemConfigImportOptions) error {
	return applyAutomationTable(ctx, s, sections, "automation_onu_monthly_report", "automation_onu_report")
}
func applyAutomationDigestImport(ctx context.Context, s *Server, sections map[string]any, _ systemConfigImportOptions) error {
	return applyAutomationTable(ctx, s, sections, "automation_alerts_digest", "automation_alerts_digest")
}
func applyAutomationCommercialImport(ctx context.Context, s *Server, sections map[string]any, _ systemConfigImportOptions) error {
	return applyAutomationTable(ctx, s, sections, "automation_commercial_report", "automation_commercial_report")
}

func applyAutomationTable(ctx context.Context, s *Server, sections map[string]any, sectionKey, table string) error {
	data, ok := sectionData(sections, sectionKey)
	if !ok {
		return nil
	}
	return updateSingletonFromJSON(ctx, s.DB(), table, data)
}

func applyUsersImport(ctx context.Context, s *Server, sections map[string]any, opts systemConfigImportOptions) error {
	if !opts.ImportUsers {
		return nil
	}
	rows, ok := sectionData(sections, "users")
	if !ok {
		return nil
	}
	tempHash, _ := bcrypt.GenerateFromPassword([]byte(uuid.NewString()), bcrypt.DefaultCost)
	for _, row := range rows.([]any) {
		m, _ := row.(map[string]any)
		email := strings.TrimSpace(strField(m, "email"))
		if email == "" {
			continue
		}
		role := strField(m, "role")
		if role == "" {
			role = "viewer"
		}
		if _, err := s.DB().Exec(ctx, `
			INSERT INTO users (id, display_name, email, phone, role, password_hash)
			VALUES (COALESCE(NULLIF($1,'')::uuid, gen_random_uuid()), $2, $3, $4, $5, $6)
			ON CONFLICT (email) DO UPDATE SET display_name = EXCLUDED.display_name,
				phone = EXCLUDED.phone, role = EXCLUDED.role, updated_at = now()`,
			strField(m, "id"), strField(m, "display_name"), email, strField(m, "phone"), role, string(tempHash)); err != nil {
			return err
		}
	}
	return nil
}

func applyDatabaseMetaImport(ctx context.Context, s *Server, sections map[string]any, opts systemConfigImportOptions) error {
	data, ok := sectionData(sections, "database_connection")
	if !ok {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(mustJSON(data), &m); err != nil {
		return err
	}
	_, err := s.DB().Exec(ctx, `
		INSERT INTO settings_database_meta (id, host, port, db_user, db_name, ssl_mode, db_password, updated_at)
		VALUES (1, $1, $2, $3, $4, $5, $6, now())
		ON CONFLICT (id) DO UPDATE SET
			host = COALESCE(EXCLUDED.host, settings_database_meta.host),
			port = COALESCE(EXCLUDED.port, settings_database_meta.port),
			db_user = COALESCE(EXCLUDED.db_user, settings_database_meta.db_user),
			db_name = COALESCE(EXCLUDED.db_name, settings_database_meta.db_name),
			ssl_mode = COALESCE(EXCLUDED.ssl_mode, settings_database_meta.ssl_mode),
			db_password = COALESCE(NULLIF(EXCLUDED.db_password, ''), settings_database_meta.db_password),
			updated_at = now()`,
		strField(m, "host"), intField(m, "port"), strField(m, "db_user"), strField(m, "db_name"),
		strField(m, "ssl_mode"), strField(m, "db_password"))
	if err == nil && opts.ApplyDatabaseConnection {
		monitorworker.NudgeMonitoringRuntimeRefresh(ctx, s.DB())
	}
	return err
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func nullStrField(m map[string]any, key string) *string {
	s := strField(m, key)
	if s == "" {
		return nil
	}
	return &s
}

func strField(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	default:
		return strings.TrimSpace(fmt.Sprint(x))
	}
}

func intField(m map[string]any, key string) *int {
	v, ok := m[key]
	if !ok || v == nil {
		return nil
	}
	switch x := v.(type) {
	case float64:
		n := int(x)
		return &n
	case int:
		return &x
	default:
		return nil
	}
}

func boolField(m map[string]any, key string) *bool {
	v, ok := m[key]
	if !ok || v == nil {
		return nil
	}
	if b, ok := v.(bool); ok {
		return &b
	}
	return nil
}

func (s *Server) runSystemConfigurationImport(jobID string, bundle map[string]any, opts systemConfigImportOptions) {
	s.sysCfgImportMu.Lock()
	job := s.sysCfgImportJobs[jobID]
	s.sysCfgImportMu.Unlock()
	if job == nil {
		return
	}
	sections, _ := bundle["sections"].(map[string]any)
	steps := importSteps()
	job.StepsTotal = len(steps)
	ctx := context.Background()

	logLine := func(msg string) {
		s.sysCfgImportMu.Lock()
		job.Logs = append(job.Logs, msg)
		s.sysCfgImportMu.Unlock()
	}

	logLine("Importação iniciada.")
	for i, step := range steps {
		s.sysCfgImportMu.Lock()
		job.CurrentStep = step.Title
		job.StepsDone = i
		if job.StepsTotal > 0 {
			job.ProgressPct = (i * 100) / job.StepsTotal
		}
		s.sysCfgImportMu.Unlock()

		logLine(fmt.Sprintf("[%d/%d] %s…", i+1, job.StepsTotal, step.Title))
		if err := step.Apply(ctx, s, sections, opts); err != nil {
			s.sysCfgImportMu.Lock()
			job.Errors = append(job.Errors, err.Error())
			job.Logs = append(job.Logs, "ERRO: "+err.Error())
			job.Status = "failed"
			now := time.Now()
			job.FinishedAt = &now
			job.ProgressPct = 100
			s.sysCfgImportMu.Unlock()
			s.appendAuditLog(ctx, "system_configuration", jobID, "import_failed", auditActorSistema, nil, map[string]any{"step": step.Key})
			return
		}
		logLine(fmt.Sprintf("[%d/%d] %s — OK.", i+1, job.StepsTotal, step.Title))
	}

	now := time.Now()
	s.sysCfgImportMu.Lock()
	job.Status = "done"
	job.StepsDone = job.StepsTotal
	job.ProgressPct = 100
	job.CurrentStep = "Concluído"
	job.FinishedAt = &now
	s.sysCfgImportMu.Unlock()
	logLine("Importação concluída com sucesso.")
	s.appendAuditLog(ctx, "system_configuration", jobID, "import_success", auditActorSistema, nil, nil)
}
