package api

import (
	"context"
	"net/http"
	"runtime"
	"time"
)

// processStartedAt — capturado no carregamento do pacote, ou seja, praticamente no arranque
// do binário. Usado só para calcular uptime no painel "Servidor NetQuasar" — não precisa de
// mais precisão do que isso.
var processStartedAt = time.Now()

// systemHealthPanel agrega auto-monitorização do próprio NetQuasar — runtime Go, pool de BD,
// estado do worker de monitorização (reaproveita monitoring_runtime, já rico em timestamps de
// ciclo) e saúde das integrações externas (a partir de integration_run_logs, já gravado a cada
// chamada). Não introduz nova recolha de métricas: só lê o que o sistema já regista sobre si
// mesmo e apresenta como um painel. Alimenta a aba "Servidor NetQuasar" do dashboard.
func (s *Server) systemHealthPanel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	uptime := time.Since(processStartedAt)

	app := map[string]any{
		"started_at":     processStartedAt.UTC().Format(time.RFC3339),
		"uptime_seconds": int64(uptime.Seconds()),
		"go_version":     runtime.Version(),
		"goroutines":     runtime.NumGoroutine(),
		"mem_alloc_mb":   round2(float64(mem.Alloc) / 1024 / 1024),
		"mem_sys_mb":     round2(float64(mem.Sys) / 1024 / 1024),
		"gc_runs":        mem.NumGC,
		"cpu_num":        runtime.NumCPU(),
	}

	dbInfo := map[string]any{}
	if pool := s.DB(); pool != nil {
		st := pool.Stat()
		dbInfo = map[string]any{
			"acquired_conns": st.AcquiredConns(),
			"idle_conns":     st.IdleConns(),
			"total_conns":    st.TotalConns(),
			"max_conns":      st.MaxConns(),
		}
	}

	monitoring := s.systemHealthMonitoring(ctx)
	integrations := s.systemHealthIntegrations(ctx)
	alerts := s.systemHealthAlerts(ctx)

	writeJSON(w, http.StatusOK, map[string]any{
		"generated_at": time.Now().UTC().Format(time.RFC3339),
		"app":          app,
		"db":           dbInfo,
		"monitoring":   monitoring,
		"integrations": integrations,
		"alerts":       alerts,
	})
}

func round2(v float64) float64 {
	return float64(int64(v*100)) / 100
}

func (s *Server) systemHealthMonitoring(ctx context.Context) map[string]any {
	var running bool
	var mode string
	var lastCycle, lastLatency, lastTelemetry, lastIface, lastOlt, lastBng, lastPipeline *time.Time
	var okC, failC int
	var currentActivity *string
	var activityStarted *time.Time
	err := s.DB().QueryRow(ctx, `
		SELECT is_running, COALESCE(monitoring_mode, 'off'), last_cycle_at,
			last_latency_cycle_at, last_telemetry_cycle_at, last_interface_snapshot_cycle_at,
			last_olt_if_derived_cycle_at, last_bng_cycle_at, last_pipeline_cycle_at,
			COALESCE(last_cycle_ok_count, 0), COALESCE(last_cycle_fail_count, 0),
			current_activity, activity_started_at
		FROM monitoring_runtime WHERE id = 1
	`).Scan(&running, &mode, &lastCycle, &lastLatency, &lastTelemetry, &lastIface, &lastOlt, &lastBng,
		&lastPipeline, &okC, &failC, &currentActivity, &activityStarted)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}
	}
	return map[string]any{
		"ok":                    true,
		"is_running":            running,
		"monitoring_mode":       mode,
		"last_cycle_at":         lastCycle,
		"last_cycle_ok_count":   okC,
		"last_cycle_fail_count": failC,
		"current_activity":      currentActivity,
		"activity_started_at":   activityStarted,
		"cycles": map[string]any{
			"latency":            lastLatency,
			"telemetry":          lastTelemetry,
			"interface_snapshot": lastIface,
			"olt_if_derived":     lastOlt,
			"bng":                lastBng,
			"pipeline":           lastPipeline,
		},
	}
}

type systemHealthIntegrationRow struct {
	IntegrationID string   `json:"integration_id"`
	Name          string   `json:"name"`
	Slug          string   `json:"slug"`
	Enabled       bool     `json:"enabled"`
	Calls24h      int      `json:"calls_24h"`
	Ok24h         int      `json:"ok_24h"`
	AvgLatencyMs  *float64 `json:"avg_latency_ms"`
	MaxLatencyMs  *int     `json:"max_latency_ms"`
	LastRunAt     *string  `json:"last_run_at"`
	LastOK        *bool    `json:"last_ok"`
}

// systemHealthIntegrations agrega internal/integrationhttp's integration_run_logs (já gravado
// pelo motor genérico a cada chamada, ver logIntegrationRun) — últimas 24h por integração.
func (s *Server) systemHealthIntegrations(ctx context.Context) []systemHealthIntegrationRow {
	rows, err := s.DB().Query(ctx, `
		WITH win AS (
			SELECT integration_id, ok, latency_ms, created_at
			FROM integration_run_logs
			WHERE created_at >= now() - interval '24 hours'
		),
		agg AS (
			SELECT integration_id,
				count(*) AS calls,
				count(*) FILTER (WHERE ok) AS ok_calls,
				avg(latency_ms) FILTER (WHERE latency_ms IS NOT NULL) AS avg_latency,
				max(latency_ms) AS max_latency
			FROM win GROUP BY integration_id
		),
		last_run AS (
			SELECT DISTINCT ON (integration_id) integration_id, ok, created_at
			FROM integration_run_logs
			ORDER BY integration_id, created_at DESC
		)
		SELECT i.id, i.name, i.slug, i.enabled,
			COALESCE(agg.calls, 0), COALESCE(agg.ok_calls, 0), agg.avg_latency, agg.max_latency,
			last_run.created_at, last_run.ok
		FROM integrations i
		LEFT JOIN agg ON agg.integration_id = i.id
		LEFT JOIN last_run ON last_run.integration_id = i.id
		ORDER BY i.name
	`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := make([]systemHealthIntegrationRow, 0, 4)
	for rows.Next() {
		var row systemHealthIntegrationRow
		var avgLatency *float64
		var maxLatency *int
		var lastRunAt *time.Time
		var lastOK *bool
		if err := rows.Scan(&row.IntegrationID, &row.Name, &row.Slug, &row.Enabled,
			&row.Calls24h, &row.Ok24h, &avgLatency, &maxLatency, &lastRunAt, &lastOK); err != nil {
			continue
		}
		if avgLatency != nil {
			v := round2(*avgLatency)
			row.AvgLatencyMs = &v
		}
		row.MaxLatencyMs = maxLatency
		if lastRunAt != nil {
			t := lastRunAt.UTC().Format(time.RFC3339)
			row.LastRunAt = &t
		}
		row.LastOK = lastOK
		out = append(out, row)
	}
	return out
}

func (s *Server) systemHealthAlerts(ctx context.Context) map[string]any {
	rows, err := s.DB().Query(ctx, `
		SELECT severity, count(*) FROM alert_instances WHERE closed_at IS NULL GROUP BY severity
	`)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}
	}
	defer rows.Close()
	byS := map[string]int{"critical": 0, "warning": 0, "info": 0}
	for rows.Next() {
		var sev string
		var n int
		if err := rows.Scan(&sev, &n); err != nil {
			continue
		}
		byS[sev] = n
	}
	return map[string]any{
		"ok":       true,
		"critical": byS["critical"],
		"warning":  byS["warning"],
		"info":     byS["info"],
	}
}
