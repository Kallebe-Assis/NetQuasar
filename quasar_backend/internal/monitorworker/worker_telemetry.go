package monitorworker

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

// TryStartParallelTelemetryCycle dispara telemetria SNMP (fase health) numa goroutine separada.
// Não espera o pipeline de interfaces/OLT. Usa telemetry_seconds e last_telemetry_cycle_at.
//
// Uma vez iniciado, o ciclo corre com deadline próprio (WithoutCancel do runCtx) para
// garantir conclusão dos KPIs mesmo se o tick pai for cancelado a meio — excepto shutdown
// do processo (parent de ActiveRun).
func TryStartParallelTelemetryCycle(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, mode string, cfg intervalConfig, opts SweepOpts) bool {
	if pool == nil || mode != ModeFull {
		return false
	}
	steps, _ := LoadPipelineSteps(ctx, pool)
	telStep := FirstEnabledTelemetryStep(steps)
	if telStep == nil {
		if log != nil {
			log.Warn().Msg("telemetria paralela: nenhum passo telemetry activo no pipeline")
		}
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
	devices, err := loadDevicesForTelemetryStep(ctx, pool, *telStep, opts.DeviceID)
	if err == nil {
		telOpts.ScopedDevices = devices
	}

	n := len(devices)
	if n == 0 && err == nil {
		// lista vazia conhecida
	}
	// Orçamento: ~25s por device / concorrência, mínimo 90s, máximo 8 min.
	conc := sweepConcurrency()
	if conc > 8 {
		conc = 8
	}
	if conc < 1 {
		conc = 1
	}
	batches := (n + conc - 1) / conc
	if batches < 1 {
		batches = 1
	}
	cycleBudget := time.Duration(batches) * 28 * time.Second
	if cycleBudget < 90*time.Second {
		cycleBudget = 90 * time.Second
	}
	if cycleBudget > 8*time.Minute {
		cycleBudget = 8 * time.Minute
	}

	go func(mode string, log *zerolog.Logger, telOpts SweepOpts, budget time.Duration) {
		defer UnlockTelemetryCycle()
		l := log.With().Str("cycle", "telemetry_health").Logger()
		setActivity(ctx, pool, "Telemetria SNMP (health) — paralelo")
		cycleCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), budget)
		defer cancel()
		if err := RunTelemetrySweep(cycleCtx, pool, &l, mode, telOpts); err != nil && log != nil {
			l.Warn().Err(err).Dur("budget", budget).Msg("telemetria health")
		}
		setActivity(ctx, pool, "")
	}(mode, log, telOpts, cycleBudget)
	return true
}

func loadDevicesForTelemetryStep(ctx context.Context, pool *pgxpool.Pool, step PipelineStep, only *uuid.UUID) ([]pingableDeviceRow, error) {
	base, err := loadTelemetryDevices(ctx, pool, only)
	if err != nil {
		return nil, err
	}
	if only != nil {
		return base, nil
	}
	return filterDevicesByScope(base, step.Scope), nil
}
