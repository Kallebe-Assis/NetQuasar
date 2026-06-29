// Package monitorworker executa ciclos de sondagem enquanto monitoring_runtime.is_running = true,
// independentemente de conexões HTTP do front (troca de tela não interrompe). Parar só via POST /monitoring/stop.
//
// O worker executa ping em paralelo (ping_seconds) quando ping_parallel=true (padrão).
// Os restantes passos do pipeline correm em sequência (Configurações → Monitoramento).
// são serializadas com WithDeviceProbeRowLock; SNMP por snmpdevicelock.
package monitorworker

import (
	"context"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

const (
	ModeOff        = "off"
	ModeSimplePing = "simple_ping"
	ModeFull       = "full"
)

// Run loop até ctx cancelado (shutdown do processo). Estado on/off vem exclusivamente do Postgres.
// dbHolder aponta para o mesmo atomic do servidor HTTP para que troca de pool (settings/database) seja vista aqui.
func Run(ctx context.Context, dbHolder *atomic.Pointer[pgxpool.Pool], log zerolog.Logger) {
	l := log.With().Str("component", "monitor_worker").Logger()
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			l.Info().Msg("monitor worker encerrado (shutdown)")
			return
		case <-ticker.C:
			pool := dbHolder.Load()
			if pool == nil {
				continue
			}
			if err := tick(ctx, pool, &l); err != nil {
				l.Debug().Err(err).Msg("monitor tick")
			}
		}
	}
}

func cycleDue(last *time.Time, sec int) bool {
	sec = clampInt(sec, 30, 86400)
	if last == nil || last.IsZero() {
		return true
	}
	return time.Since(*last) >= time.Duration(sec)*time.Second
}

func tick(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger) error {
	var running bool
	var mode string
	err := pool.QueryRow(ctx, `
		SELECT is_running,
			COALESCE(NULLIF(TRIM(monitoring_mode), ''), 'off')
		FROM monitoring_runtime WHERE id = 1
	`).Scan(&running, &mode)
	if err != nil {
		return err
	}
	if !running || mode == ModeOff || mode == "" {
		EndActiveRun()
		return nil
	}
	if mode != ModeSimplePing && mode != ModeFull {
		EndActiveRun()
		return nil
	}
	EnsureActiveRun(ctx)

	cfg, err := loadClampMonitoringIntervals(ctx, pool)
	if err != nil {
		return err
	}

	runCtx := ActiveRunContext(ctx)

	// Ping paralelo (intervalo ping_seconds) — não espera telemetria/OLT.
	if cfg.PingParallel || mode == ModeSimplePing {
		TryStartParallelPingCycle(runCtx, pool, log, mode, cfg, SweepOpts{Source: "worker"})
	}
	if mode == ModeSimplePing {
		return nil
	}

	// Totais BNG (PPPoE online, etc.) em paralelo — intervalo telemetry_seconds.
	TryStartParallelBngCycle(runCtx, pool, log, mode, cfg, SweepOpts{Source: "worker"})

	var lastPipeline *time.Time
	if err := pool.QueryRow(ctx, `SELECT last_pipeline_cycle_at FROM monitoring_runtime WHERE id=1`).Scan(&lastPipeline); err != nil {
		return err
	}

	pipelineDue := cycleDue(lastPipeline, cfg.PipelineCycleSeconds)
	if !pipelineDue {
		return nil
	}
	if !TryLockMonitoringPipeline() {
		if log != nil {
			log.Debug().Msg("pipeline adiado: execução anterior ainda em curso")
		}
		return nil
	}
	go func(mode string, log *zerolog.Logger, skipPing bool, pipelineCtx context.Context) {
		defer UnlockMonitoringPipeline()
		l := log.With().Str("component", "monitor_worker").Str("cycle", "pipeline").Logger()
		if err := RunConfiguredPipeline(pipelineCtx, pool, &l, mode, SweepOpts{Source: "worker", SkipPingInPipeline: skipPing}); err != nil {
			l.Warn().Err(err).Msg("pipeline de monitoramento")
		}
	}(mode, log, cfg.PingParallel, runCtx)

	return nil
}

// monitoringDeviceLabel devolve a descrição do equipamento ou, em falta, o IP/host.
func monitoringDeviceLabel(description, host string) string {
	d := strings.TrimSpace(description)
	if d != "" {
		return d
	}
	return strings.TrimSpace(host)
}

// NudgeMonitoringRuntimeRefresh sinaliza ao frontend que novas leituras foram gravadas (telemetria, OLT, etc.).
func NudgeMonitoringRuntimeRefresh(ctx context.Context, pool *pgxpool.Pool) {
	if pool == nil {
		return
	}
	_, _ = pool.Exec(ctx, `
		UPDATE monitoring_runtime
		SET updated_at = now(), activity_updated_at = now()
		WHERE id = 1
	`)
}

func setActivity(ctx context.Context, pool *pgxpool.Pool, activity string) {
	if pool == nil {
		return
	}
	activity = strings.TrimSpace(activity)
	_, _ = pool.Exec(ctx, `
		UPDATE monitoring_runtime
		SET last_activity = CASE
				WHEN NULLIF($1, '') IS NULL AND current_activity IS NOT NULL THEN current_activity
				ELSE last_activity
			END,
			last_activity_finished_at = CASE
				WHEN NULLIF($1, '') IS NULL AND current_activity IS NOT NULL THEN now()
				ELSE last_activity_finished_at
			END,
			activity_started_at = CASE
				WHEN NULLIF($1, '') IS NULL THEN NULL
				WHEN current_activity IS DISTINCT FROM NULLIF($1, '') THEN now()
				ELSE activity_started_at
			END,
			current_activity = NULLIF($1, ''),
			activity_updated_at = CASE WHEN NULLIF($1, '') IS NULL THEN NULL ELSE now() END,
			updated_at = now()
		WHERE id = 1
	`, activity)
}
