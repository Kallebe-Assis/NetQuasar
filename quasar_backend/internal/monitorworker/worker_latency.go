package monitorworker

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

// TryStartParallelPingCycle dispara ICMP/TCP numa goroutine separada (não bloqueia telemetria/OLT).
// Usa ping_seconds e last_latency_cycle_at; respeita o scope do primeiro passo «ping» activo no pipeline.
// Em modo full só corre quando ping_parallel=true; simple_ping corre sempre (independentemente do flag).
func TryStartParallelPingCycle(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, mode string, cfg intervalConfig, opts SweepOpts) bool {
	if pool == nil {
		return false
	}
	if mode == ModeOff || mode == "" {
		return false
	}
	if mode == ModeFull && !cfg.PingParallel {
		return false
	}
	var lastLatency *time.Time
	if err := pool.QueryRow(ctx, `SELECT last_latency_cycle_at FROM monitoring_runtime WHERE id=1`).Scan(&lastLatency); err != nil {
		return false
	}
	if !opts.Force && !cycleDue(lastLatency, cfg.PingSeconds) {
		return false
	}
	if !TryLockLatencyCycle() {
		if log != nil {
			log.Debug().Msg("ping paralelo adiado: ciclo anterior em curso")
		}
		return false
	}

	pingOpts := opts
	if pingOpts.Source == "" {
		pingOpts.Source = "worker_ping"
	}
	steps, _ := LoadPipelineSteps(ctx, pool)
	if pingStep := FirstEnabledPingStep(steps); pingStep != nil {
		pingOpts.PipelineStep = pingStep
		if devices, err := loadDevicesForPipelineStep(ctx, pool, *pingStep, opts.DeviceID); err == nil {
			pingOpts.ScopedDevices = devices
		}
	}

	go func(mode string, log *zerolog.Logger, pingOpts SweepOpts) {
		defer UnlockLatencyCycle()
		l := log.With().Str("cycle", "latency_parallel").Logger()
		setActivity(ctx, pool, "Ping (ICMP/TCP) — paralelo")
		if err := RunLatencySweep(ctx, pool, &l, mode, pingOpts); err != nil && log != nil {
			l.Warn().Err(err).Msg("ping paralelo")
		}
		setActivity(ctx, pool, "")
	}(mode, log, pingOpts)
	return true
}
