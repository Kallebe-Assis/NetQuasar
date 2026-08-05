package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	jobAlertsDigest     = "alerts_digest"
	jobCommercialReport = "commercial_report"
	jobOnuMonthlyReport = "onu_monthly_report"
	jobBngStatsReport   = "bng_stats_report"
	jobDatabaseBackup   = "database_backup"
)

func automationJobLabel(jobType string) string {
	switch jobType {
	case jobAlertsDigest:
		return "Resumo de alertas"
	case jobCommercialReport:
		return "Base comercial"
	case jobOnuMonthlyReport:
		return "Relatório ONU mensal"
	case jobBngStatsReport:
		return "Totais BNG"
	case jobDatabaseBackup:
		return "Backup PostgreSQL (B2)"
	default:
		return jobType
	}
}

func automationJobCategory(jobType string) string {
	switch jobType {
	case jobDatabaseBackup:
		return "Sistema"
	case jobAlertsDigest, jobCommercialReport, jobOnuMonthlyReport, jobBngStatsReport:
		return "Relatórios"
	default:
		return "Outros"
	}
}

func automationJobDescription(jobType string) string {
	switch jobType {
	case jobAlertsDigest:
		return "Envia resumo de alertas abertos e incidentes por Telegram e/ou e-mail."
	case jobCommercialReport:
		return "Gera e envia o relatório mensal da base comercial."
	case jobOnuMonthlyReport:
		return "Recolhe dados OLT e envia o relatório mensal de ONUs."
	case jobBngStatsReport:
		return "Envia totais de sessões BNG (PPPoE/IPv4/IPv6) por canal configurado."
	case jobDatabaseBackup:
		return "Dump completo PostgreSQL enviado para o bucket Backblaze B2."
	default:
		return ""
	}
}

// automationRunMeta normaliza actor + origem (sistema vs manual) para o execution log.
type automationRunMeta struct {
	Actor     string
	Trigger   string // "scheduled" | "manual"
	UserID    *uuid.UUID
	ActorKind string // "system" | "user" | "api_key"
}

func automationMetaFromActor(actor string, userID *uuid.UUID) automationRunMeta {
	raw := strings.TrimSpace(actor)
	norm := normalizeAuditActor(raw)
	if raw == "" || norm == auditActorSistema || strings.EqualFold(raw, "scheduler") {
		return automationRunMeta{
			Actor:     auditActorSistema,
			Trigger:   "scheduled",
			UserID:    nil,
			ActorKind: "system",
		}
	}
	kind := "user"
	if strings.EqualFold(norm, "Chave API") {
		kind = "api_key"
	}
	return automationRunMeta{
		Actor:     norm,
		Trigger:   "manual",
		UserID:    userID,
		ActorKind: kind,
	}
}

func (s *Server) automationMetaFromRequest(r *http.Request) automationRunMeta {
	return automationMetaFromActor(s.actorFromRequest(r), s.userIDFromRequest(r))
}

func (s *Server) userIDFromRequest(r *http.Request) *uuid.UUID {
	if r == nil || s.Cfg == nil {
		return nil
	}
	bearer := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if bearer == "" {
		return nil
	}
	uid, _, _, err := parseUserJWT(s.Cfg, bearer)
	if err != nil || uid == uuid.Nil {
		return nil
	}
	id := uid
	return &id
}

func (s *Server) recordAutomationExecution(
	ctx context.Context,
	jobType string,
	meta automationRunMeta,
	started time.Time,
	ok bool,
	statusMessage string,
	err error,
	summary map[string]any,
	runKey string,
) {
	pool := s.DB()
	if pool == nil {
		return
	}
	if summary == nil {
		summary = map[string]any{}
	}
	summary["actor_kind"] = meta.ActorKind
	summary["origin"] = map[string]any{
		"trigger":    meta.Trigger,
		"actor":      meta.Actor,
		"actor_kind": meta.ActorKind,
	}
	var errMsg *string
	if err != nil && strings.TrimSpace(err.Error()) != "" {
		e := err.Error()
		errMsg = &e
	}
	if strings.TrimSpace(statusMessage) == "" {
		if ok {
			statusMessage = "Concluído com sucesso"
		} else {
			statusMessage = "Falhou"
		}
	}
	sb, _ := json.Marshal(summary)
	var rk *string
	if strings.TrimSpace(runKey) != "" {
		r := runKey
		rk = &r
	}
	actor := strings.TrimSpace(meta.Actor)
	if actor == "" {
		actor = auditActorSistema
	}
	trigger := meta.Trigger
	if trigger != "manual" {
		trigger = "scheduled"
	}
	if _, execErr := pool.Exec(ctx, `
		INSERT INTO automation_execution_log (
			job_type, actor, trigger_type, triggered_by_user_id,
			started_at, finished_at, ok,
			status_message, error_message, summary, run_key
		) VALUES ($1, $2, $3, $4, $5, now(), $6, $7, $8, $9::jsonb, $10)
	`, jobType, actor, trigger, meta.UserID, started, ok, statusMessage, errMsg, string(sb), rk); execErr != nil {
		if _, execErr2 := pool.Exec(ctx, `
			INSERT INTO automation_execution_log (
				job_type, actor, trigger_type, started_at, finished_at, ok,
				status_message, error_message, summary, run_key
			) VALUES ($1, $2, $3, $4, now(), $5, $6, $7, $8::jsonb, $9)
		`, jobType, actor, trigger, started, ok, statusMessage, errMsg, string(sb), rk); execErr2 != nil {
			s.Log.Warn().Err(execErr2).Str("job_type", jobType).Msg("falha ao gravar automation_execution_log")
		}
	}
}

func (s *Server) alertsDigestSummary(ctx context.Context) map[string]any {
	pool := s.DB()
	if pool == nil {
		return nil
	}
	var openTotal, closed24h, openIncidents int64
	_ = pool.QueryRow(ctx, `SELECT COUNT(*) FROM alert_instances WHERE closed_at IS NULL`).Scan(&openTotal)
	_ = pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM alert_instances
		WHERE closed_at IS NOT NULL AND closed_at >= now() - interval '24 hours'
	`).Scan(&closed24h)
	_ = pool.QueryRow(ctx, `SELECT COUNT(*) FROM alert_incidents WHERE status = 'open'`).Scan(&openIncidents)
	return map[string]any{
		"alerts_open":       openTotal,
		"alerts_closed_24h": closed24h,
		"incidents_open":    openIncidents,
		"alerts_summarized": openTotal,
	}
}

func (s *Server) commercialReportSummary(ctx context.Context, period string) map[string]any {
	pool := s.DB()
	if pool == nil {
		return nil
	}
	var total int64
	var localities int64
	_ = pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(client_count), 0)::bigint, COUNT(DISTINCT locality_id)::bigint
		FROM commercial_monthly_records WHERE year_month = $1
	`, period).Scan(&total, &localities)
	return map[string]any{
		"period":           period,
		"clients_total":    total,
		"localities_count": localities,
	}
}

func (s *Server) listAutomationExecutionHistory(ctx context.Context, pool *pgxpool.Pool, jobType, q, triggerType string, from, to *time.Time, limit int) ([]map[string]any, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	args := []any{}
	var where []string
	n := 1
	if strings.TrimSpace(jobType) != "" {
		where = append(where, "l.job_type = $"+strconv.Itoa(n))
		args = append(args, jobType)
		n++
	}
	if t := strings.TrimSpace(strings.ToLower(triggerType)); t == "manual" || t == "scheduled" {
		where = append(where, "l.trigger_type = $"+strconv.Itoa(n))
		args = append(args, t)
		n++
	}
	if from != nil {
		where = append(where, "l.started_at >= $"+strconv.Itoa(n))
		args = append(args, *from)
		n++
	}
	if to != nil {
		where = append(where, "l.started_at <= $"+strconv.Itoa(n))
		args = append(args, *to)
		n++
	}
	if tq := strings.TrimSpace(q); tq != "" {
		where = append(where, `(
			l.status_message ILIKE $`+strconv.Itoa(n)+` OR COALESCE(l.error_message,'') ILIKE $`+strconv.Itoa(n)+`
			OR l.job_type ILIKE $`+strconv.Itoa(n)+` OR l.summary::text ILIKE $`+strconv.Itoa(n)+`
			OR l.actor ILIKE $`+strconv.Itoa(n)+`
		)`)
		args = append(args, "%"+tq+"%")
		n++
	}
	sql := `
		SELECT l.id, l.job_type, l.actor, l.trigger_type, l.triggered_by_user_id,
			l.started_at, l.finished_at, l.ok,
			l.status_message, l.error_message, l.summary, l.run_key,
			u.email, COALESCE(NULLIF(trim(u.display_name), ''), '')
		FROM automation_execution_log l
		LEFT JOIN users u ON u.id = l.triggered_by_user_id`
	if len(where) > 0 {
		sql += " WHERE " + strings.Join(where, " AND ")
	}
	sql += " ORDER BY l.started_at DESC LIMIT $" + strconv.Itoa(n)
	args = append(args, limit)

	rows, err := pool.Query(ctx, sql, args...)
	if err != nil {
		return s.listAutomationExecutionHistoryLegacy(ctx, pool, jobType, q, from, to, limit)
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id, jt, actor, trig, statusMsg string
		var runKey, errMsg *string
		var userID *uuid.UUID
		var userEmail, userDisplay *string
		var started, finished time.Time
		var ok bool
		var sum []byte
		if err := rows.Scan(&id, &jt, &actor, &trig, &userID, &started, &finished, &ok, &statusMsg, &errMsg, &sum, &runKey, &userEmail, &userDisplay); err != nil {
			return nil, err
		}
		var sm any
		if len(sum) > 0 {
			_ = json.Unmarshal(sum, &sm)
		}
		originLabel := "Sistema (agendado)"
		if trig == "manual" {
			originLabel = "Manual"
		}
		row := map[string]any{
			"id":             id,
			"job_type":       jt,
			"job_label":      automationJobLabel(jt),
			"actor":          actor,
			"trigger_type":   trig,
			"origin_label":   originLabel,
			"started_at":     started,
			"finished_at":    finished,
			"ok":             ok,
			"status_message": statusMsg,
			"error_message":  errMsg,
			"summary":        sm,
			"run_key":        runKey,
			"triggered_by":   actor,
		}
		if userID != nil {
			row["triggered_by_user_id"] = userID.String()
		}
		if userEmail != nil && strings.TrimSpace(*userEmail) != "" {
			row["triggered_by_email"] = strings.TrimSpace(*userEmail)
		}
		if userDisplay != nil && strings.TrimSpace(*userDisplay) != "" {
			row["triggered_by_display_name"] = strings.TrimSpace(*userDisplay)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Server) listAutomationExecutionHistoryLegacy(ctx context.Context, pool *pgxpool.Pool, jobType, q string, from, to *time.Time, limit int) ([]map[string]any, error) {
	args := []any{}
	var where []string
	n := 1
	if strings.TrimSpace(jobType) != "" {
		where = append(where, "job_type = $"+strconv.Itoa(n))
		args = append(args, jobType)
		n++
	}
	if from != nil {
		where = append(where, "started_at >= $"+strconv.Itoa(n))
		args = append(args, *from)
		n++
	}
	if to != nil {
		where = append(where, "started_at <= $"+strconv.Itoa(n))
		args = append(args, *to)
		n++
	}
	if tq := strings.TrimSpace(q); tq != "" {
		where = append(where, `(
			status_message ILIKE $`+strconv.Itoa(n)+` OR COALESCE(error_message,'') ILIKE $`+strconv.Itoa(n)+`
			OR job_type ILIKE $`+strconv.Itoa(n)+` OR summary::text ILIKE $`+strconv.Itoa(n)+`
		)`)
		args = append(args, "%"+tq+"%")
		n++
	}
	sql := `
		SELECT id, job_type, actor, trigger_type, started_at, finished_at, ok,
			status_message, error_message, summary, run_key
		FROM automation_execution_log`
	if len(where) > 0 {
		sql += " WHERE " + strings.Join(where, " AND ")
	}
	sql += " ORDER BY started_at DESC LIMIT $" + strconv.Itoa(n)
	args = append(args, limit)
	rows, err := pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id, jt, actor, trig, statusMsg string
		var runKey, errMsg *string
		var started, finished time.Time
		var ok bool
		var sum []byte
		if err := rows.Scan(&id, &jt, &actor, &trig, &started, &finished, &ok, &statusMsg, &errMsg, &sum, &runKey); err != nil {
			return nil, err
		}
		var sm any
		if len(sum) > 0 {
			_ = json.Unmarshal(sum, &sm)
		}
		originLabel := "Sistema (agendado)"
		if trig == "manual" {
			originLabel = "Manual"
		}
		out = append(out, map[string]any{
			"id": id, "job_type": jt, "job_label": automationJobLabel(jt),
			"actor": actor, "trigger_type": trig, "origin_label": originLabel,
			"started_at": started, "finished_at": finished, "ok": ok,
			"status_message": statusMsg, "error_message": errMsg, "summary": sm, "run_key": runKey,
			"triggered_by": actor,
		})
	}
	return out, rows.Err()
}
