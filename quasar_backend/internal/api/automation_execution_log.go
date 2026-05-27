package api

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	jobAlertsDigest      = "alerts_digest"
	jobCommercialReport  = "commercial_report"
	jobOnuMonthlyReport  = "onu_monthly_report"
)

func automationJobLabel(jobType string) string {
	switch jobType {
	case jobAlertsDigest:
		return "Resumo de alertas"
	case jobCommercialReport:
		return "Base comercial"
	case jobOnuMonthlyReport:
		return "Relatório ONU mensal"
	default:
		return jobType
	}
}

func (s *Server) recordAutomationExecution(
	ctx context.Context,
	jobType, actor, triggerType string,
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
	var errMsg *string
	if err != nil && strings.TrimSpace(err.Error()) != "" {
		e := err.Error()
		errMsg = &e
	}
	if strings.TrimSpace(statusMessage) == "" {
		if ok {
			statusMessage = "Concluído com sucesso"
		} else if errMsg != nil {
			statusMessage = "Falhou"
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
	if _, execErr := pool.Exec(ctx, `
		INSERT INTO automation_execution_log (
			job_type, actor, trigger_type, started_at, finished_at, ok,
			status_message, error_message, summary, run_key
		) VALUES ($1, $2, $3, $4, now(), $5, $6, $7, $8::jsonb, $9)
	`, jobType, actor, triggerType, started, ok, statusMessage, errMsg, string(sb), rk); execErr != nil {
		s.Log.Warn().Err(execErr).Str("job_type", jobType).Msg("falha ao gravar automation_execution_log")
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
		"alerts_open":            openTotal,
		"alerts_closed_24h":      closed24h,
		"incidents_open":         openIncidents,
		"alerts_summarized":      openTotal,
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
		"period":            period,
		"clients_total":     total,
		"localities_count":  localities,
	}
}

func (s *Server) listAutomationExecutionHistory(ctx context.Context, pool *pgxpool.Pool, jobType, q string, from, to *time.Time, limit int) ([]map[string]any, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
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
		pat := "%" + tq + "%"
		args = append(args, pat)
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
		out = append(out, map[string]any{
			"id":              id,
			"job_type":        jt,
			"job_label":       automationJobLabel(jt),
			"actor":           actor,
			"trigger_type":    trig,
			"started_at":      started,
			"finished_at":     finished,
			"ok":              ok,
			"status_message":  statusMsg,
			"error_message":   errMsg,
			"summary":         sm,
			"run_key":         runKey,
		})
	}
	return out, rows.Err()
}
