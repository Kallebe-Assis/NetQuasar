package monitorworker

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

// TryStartParallelTelemetryCycle dispara telemetria SNMP numa goroutine separada
// (não espera o pipeline de interfaces/OLT). Usa telemetry_seconds e last_telemetry_cycle_at.
// Respeita scope do primeiro passo «telemetry» activo no pipeline.
func TryStartParallelTelemetryCycle(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, mode string, cfg intervalConfig, opts SweepOpts) bool {
	if pool == nil || mode != ModeFull {
		return false
	}
	steps, _ := LoadPipelineSteps(ctx, pool)
	telStep := FirstEnabledTelemetryStep(steps)
	if telStep == nil {
		return false
	}
	var lastTel *time.Time
	if err := pool.QueryRow(ctx, `SELECT last_telemetry_cycle_at FROM monitoring_runtime WHERE id=1`).Scan(&lastTel); err != nil {
		return false
	}
	if !opts.Force && !cycleDue(lastTel, cfg.TelemetrySeconds) {
		return false
	}
	if !TryLockTelemetryCycle() {
		if log != nil {
			log.Debug().Msg("telemetria paralela adiada: ciclo anterior em curso")
		}
		return false
	}

	telOpts := opts
	if telOpts.Source == "" {
		telOpts.Source = "worker_telemetry"
	}
	telOpts.PipelineStep = telStep
	if devices, err := loadDevicesForPipelineStep(ctx, pool, *telStep, opts.DeviceID); err == nil {
		telOpts.ScopedDevices = devices
	}

	go func(mode string, log *zerolog.Logger, telOpts SweepOpts) {
		defer UnlockTelemetryCycle()
		l := log.With().Str("cycle", "telemetry_parallel").Logger()
		setActivity(ctx, pool, "Telemetria SNMP — paralelo")
		if err := RunTelemetrySweep(ctx, pool, &l, mode, telOpts); err != nil && log != nil {
			l.Warn().Err(err).Msg("telemetria paralela")
		}
		setActivity(ctx, pool, "")
	}(mode, log, telOpts)
	return true
}
