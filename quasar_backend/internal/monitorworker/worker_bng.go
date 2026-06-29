package monitorworker

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

// TryStartParallelBngCycle dispara coleta de totais BNG (PPPoE online, IPv4/IPv6) numa goroutine separada.
// Usa telemetry_seconds e last_bng_cycle_at; respeita scope/modo do primeiro passo «bng» activo no pipeline.
func TryStartParallelBngCycle(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, mode string, cfg intervalConfig, opts SweepOpts) bool {
	if pool == nil || mode != ModeFull {
		return false
	}
	steps, _ := LoadPipelineSteps(ctx, pool)
	bngStep := FirstEnabledBngStep(steps)
	if bngStep == nil {
		return false
	}
	var lastBng *time.Time
	if err := pool.QueryRow(ctx, `SELECT last_bng_cycle_at FROM monitoring_runtime WHERE id=1`).Scan(&lastBng); err != nil {
		return false
	}
	if !opts.Force && !cycleDue(lastBng, cfg.TelemetrySeconds) {
		return false
	}
	if !TryLockBngCycle() {
		if log != nil {
			log.Debug().Msg("BNG paralelo adiado: ciclo anterior em curso")
		}
		return false
	}

	bngOpts := opts
	if bngOpts.Source == "" {
		bngOpts.Source = "worker_bng"
	}
	bngOpts.PipelineStep = bngStep
	if devices, err := loadDevicesForPipelineStep(ctx, pool, *bngStep, opts.DeviceID); err == nil {
		bngOpts.ScopedDevices = devices
	}

	go func(mode string, log *zerolog.Logger, bngOpts SweepOpts) {
		defer UnlockBngCycle()
		l := log.With().Str("cycle", "bng_parallel").Logger()
		setActivity(ctx, pool, "BNG — totais PPPoE/logins (paralelo)")
		if err := RunBngSweep(ctx, pool, &l, mode, bngOpts); err != nil && log != nil {
			l.Warn().Err(err).Msg("coleta BNG paralela")
		}
		setActivity(ctx, pool, "")
	}(mode, log, bngOpts)
	return true
}
