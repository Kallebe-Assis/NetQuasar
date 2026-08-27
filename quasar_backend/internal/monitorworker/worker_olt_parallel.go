package monitorworker

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

// TryStartParallelOltCycle dispara a coleta OLT "leve" (baseline/pon_status/onu_counts —
// contagem de ONUs online por PON, sem telnet nem potência óptica detalhada) numa goroutine
// separada, com cadência própria (olt_baseline_parallel_seconds, default 30s) e
// independente do pipeline sequencial de interfaces/OLT completo.
//
// Antes desta função, a única coleta OLT automática corria dentro do pipeline sequencial
// (RunConfiguredPipeline), gatilhada só a cada pipeline_cycle_seconds (default 120s) e
// depois de outros passos (interfaces MikroTik/switch) já terem corrido nesse ciclo — ou
// seja, uma queda de ONUs podia demorar bem mais que 120s a ser detectada. Ver
// DIAGNOSTICO-PERFORMANCE-ARQUITETURA.md, secção "detecção ágil de queda de ONUs".
//
// O tier "full" (telnet, potência óptica ONU/PON completa) continua no pipeline
// sequencial, na sua cadência própria (mais lenta, aceitável para esse detalhe) — ver
// SkipOltBaselineInPipeline em pipeline_runner.go.
func TryStartParallelOltCycle(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, mode string, cfg intervalConfig, opts SweepOpts) bool {
	if pool == nil || mode != ModeFull {
		return false
	}
	steps, _ := LoadPipelineSteps(ctx, pool)
	oltStep := FirstEnabledOltLightStep(steps)
	if oltStep == nil {
		return false
	}
	var lastRun *time.Time
	if err := pool.QueryRow(ctx, `SELECT last_olt_baseline_parallel_cycle_at FROM monitoring_runtime WHERE id=1`).Scan(&lastRun); err != nil {
		return false
	}
	if !opts.Force && !cycleDue(lastRun, cfg.OltBaselineParallelSeconds) {
		return false
	}
	if !TryLockOltPonCycle() {
		if log != nil {
			log.Debug().Msg("OLT paralelo adiado: ciclo anterior ainda em curso")
		}
		return false
	}

	oltOpts := opts
	if oltOpts.Source == "" {
		oltOpts.Source = "worker_olt_parallel"
	}
	oltOpts.PipelineStep = oltStep
	devices, err := loadDevicesForOltLightStep(ctx, pool, *oltStep, opts.DeviceID)
	if err == nil {
		oltOpts.ScopedDevices = devices
	}
	n := len(devices)

	// Orçamento: ~1 SNMP walk por device / concorrência, mínimo 30s, máximo 4 min —
	// deve caber confortavelmente dentro do próprio intervalo do ciclo para não empilhar.
	conc := sweepConcurrency()
	if conc < 1 {
		conc = 1
	}
	batches := (n + conc - 1) / conc
	if batches < 1 {
		batches = 1
	}
	cycleBudget := time.Duration(batches) * 15 * time.Second
	if cycleBudget < 30*time.Second {
		cycleBudget = 30 * time.Second
	}
	if cycleBudget > 4*time.Minute {
		cycleBudget = 4 * time.Minute
	}

	go func(oltOpts SweepOpts, budget time.Duration) {
		defer UnlockOltPonCycle()
		l := log.With().Str("cycle", "olt_baseline_parallel").Logger()
		setActivity(ctx, pool, "Coleta ONUs/PON (ciclo rápido, paralelo)")
		cycleCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), budget)
		defer cancel()
		if _, err := RunOltCollectAll(cycleCtx, pool, &l, mode, oltOpts); err != nil && log != nil {
			l.Warn().Err(err).Dur("budget", budget).Msg("coleta OLT paralela")
		}
		setActivity(ctx, pool, "")
		_, _ = pool.Exec(context.WithoutCancel(ctx), `
			UPDATE monitoring_runtime SET last_olt_baseline_parallel_cycle_at = now(), updated_at = now()
			WHERE id = 1
		`)
	}(oltOpts, cycleBudget)
	return true
}

func loadDevicesForOltLightStep(ctx context.Context, pool *pgxpool.Pool, step PipelineStep, only *uuid.UUID) ([]pingableDeviceRow, error) {
	base, err := loadOltDevicesForCollect(ctx, pool, only)
	if err != nil {
		return nil, err
	}
	if only != nil {
		return base, nil
	}
	return filterDevicesByScope(base, step.Scope), nil
}
