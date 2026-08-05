package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) getAutomationExecutionHistory(w http.ResponseWriter, r *http.Request) {
	pool := s.DB()
	if pool == nil {
		writeErr(w, http.StatusServiceUnavailable, "DB", "base indisponível", nil)
		return
	}
	q := r.URL.Query()
	jobType := q.Get("job_type")
	search := q.Get("q")
	triggerType := q.Get("trigger_type")
	limit := 200
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			if n > 500 {
				n = 500
			}
			limit = n
		}
	}
	var from, to *time.Time
	if v := q.Get("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			from = &t
		}
	}
	if v := q.Get("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			to = &t
		}
	}
	items, err := s.listAutomationExecutionHistory(r.Context(), pool, jobType, search, triggerType, from, to, limit)
	if err != nil {
		msg := err.Error()
		if strings.Contains(strings.ToLower(msg), "automation_execution_log") && strings.Contains(strings.ToLower(msg), "does not exist") {
			msg = "tabela automation_execution_log inexistente — reinicie o backend após migração 035 ou execute db/migrate"
		}
		writeErr(w, http.StatusInternalServerError, "DB", msg, nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) getAutomationOverview(w http.ResponseWriter, r *http.Request) {
	pool := s.DB()
	if pool == nil {
		writeErr(w, http.StatusServiceUnavailable, "DB", "base indisponível", nil)
		return
	}
	ctx := r.Context()

	type jobRow struct {
		enabled bool
		running bool
		lastStatus, lastError *string
		lastAt                *time.Time
		extra                 map[string]any
	}
	readSimple := func(table string) jobRow {
		var jr jobRow
		_ = pool.QueryRow(ctx, `
			SELECT enabled, running, last_status, last_error
			FROM `+table+` WHERE id = 1`).Scan(&jr.enabled, &jr.running, &jr.lastStatus, &jr.lastError)
		return jr
	}

	jobs := []map[string]any{}
	add := func(jobType string, jr jobRow) {
		item := map[string]any{
			"job_type":    jobType,
			"label":       automationJobLabel(jobType),
			"category":    automationJobCategory(jobType),
			"description": automationJobDescription(jobType),
			"enabled":     jr.enabled,
			"running":     jr.running,
			"last_status": jr.lastStatus,
			"last_error":  jr.lastError,
		}
		if jr.lastAt != nil {
			item["last_run_at"] = jr.lastAt
		}
		for k, v := range jr.extra {
			item[k] = v
		}
		// Contagens recentes
		var runs24, ok24, fail24, manual30, sched30 int64
		_ = pool.QueryRow(ctx, `
			SELECT
				COUNT(*) FILTER (WHERE started_at >= now() - interval '24 hours'),
				COUNT(*) FILTER (WHERE started_at >= now() - interval '24 hours' AND ok),
				COUNT(*) FILTER (WHERE started_at >= now() - interval '24 hours' AND NOT ok),
				COUNT(*) FILTER (WHERE started_at >= now() - interval '30 days' AND trigger_type = 'manual'),
				COUNT(*) FILTER (WHERE started_at >= now() - interval '30 days' AND trigger_type = 'scheduled')
			FROM automation_execution_log WHERE job_type = $1
		`, jobType).Scan(&runs24, &ok24, &fail24, &manual30, &sched30)
		item["runs_24h"] = runs24
		item["ok_24h"] = ok24
		item["fail_24h"] = fail24
		item["manual_30d"] = manual30
		item["scheduled_30d"] = sched30
		jobs = append(jobs, item)
	}

	{
		jr := readSimple("automation_alerts_digest")
		add(jobAlertsDigest, jr)
	}
	{
		jr := readSimple("automation_commercial_report")
		add(jobCommercialReport, jr)
	}
	{
		var jr jobRow
		_ = pool.QueryRow(ctx, `
			SELECT enabled, running, last_status, last_error, last_run_at
			FROM automation_onu_report WHERE id = 1`).Scan(&jr.enabled, &jr.running, &jr.lastStatus, &jr.lastError, &jr.lastAt)
		add(jobOnuMonthlyReport, jr)
	}
	{
		jr := readSimple("automation_bng_stats_report")
		add(jobBngStatsReport, jr)
	}
	{
		var jr jobRow
		_ = pool.QueryRow(ctx, `
			SELECT enabled, running, last_status, last_error, last_run_at
			FROM automation_database_backup WHERE id = 1`).Scan(&jr.enabled, &jr.running, &jr.lastStatus, &jr.lastError, &jr.lastAt)
		add(jobDatabaseBackup, jr)
	}

	var total, enabledN, runningN int
	var runsToday, okToday, fail30d int64
	var lastFailAt *time.Time
	for _, j := range jobs {
		total++
		if j["enabled"] == true {
			enabledN++
		}
		if j["running"] == true {
			runningN++
		}
	}
	_ = pool.QueryRow(ctx, `
		SELECT COUNT(*), COUNT(*) FILTER (WHERE ok)
		FROM automation_execution_log
		WHERE started_at >= date_trunc('day', now())
	`).Scan(&runsToday, &okToday)
	_ = pool.QueryRow(ctx, `
		SELECT COUNT(*), MAX(started_at)
		FROM automation_execution_log
		WHERE NOT ok AND started_at >= now() - interval '30 days'
	`).Scan(&fail30d, &lastFailAt)

	var successRate *float64
	var runs30, ok30 int64
	_ = pool.QueryRow(ctx, `
		SELECT COUNT(*), COUNT(*) FILTER (WHERE ok)
		FROM automation_execution_log
		WHERE started_at >= now() - interval '30 days'
	`).Scan(&runs30, &ok30)
	if runs30 > 0 {
		v := float64(ok30) * 100.0 / float64(runs30)
		successRate = &v
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"jobs": jobs,
		"kpis": map[string]any{
			"total":            total,
			"enabled":          enabledN,
			"disabled":         total - enabledN,
			"running":          runningN,
			"executed_today":   runsToday,
			"success_rate_30d": successRate,
			"runs_30d":         runs30,
			"failures_30d":     fail30d,
			"last_failure_at":  lastFailAt,
		},
	})
}
