package monitorworker

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertthresholds"
	"github.com/rs/zerolog"
)

// RunWorkerOrderedSteps executa passos na ordem exigida: ping → telemetria → IF MikroTik → IF OLT → PON IF-MIB.
// bootstrap=true força todos os passos (ignora intervalos), usado ao iniciar monitoramento full.
func RunWorkerOrderedSteps(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, mode string, bootstrap bool) {
	if pool == nil || log == nil {
		return
	}
	src := "worker"
	if bootstrap {
		src = "bootstrap"
	}

	cfg, err := loadClampMonitoringIntervals(ctx, pool)
	if err != nil {
		log.Error().Err(err).Msg("ordered pipeline: intervalos")
		return
	}

	var lastLat, lastTel, lastIf, lastOlt *time.Time
	_ = pool.QueryRow(ctx, `
		SELECT last_latency_cycle_at, last_telemetry_cycle_at,
			last_interface_snapshot_cycle_at, last_olt_if_derived_cycle_at
		FROM monitoring_runtime WHERE id=1`).Scan(&lastLat, &lastTel, &lastIf, &lastOlt)

	runLat := bootstrap || cycleDue(lastLat, cfg.PingSeconds)
	runTel := bootstrap || (mode == ModeFull && cycleDue(lastTel, cfg.TelemetrySeconds))
	runIf := bootstrap || (mode == ModeFull && cycleDue(lastIf, cfg.IfaceSeconds))
	runOlt := bootstrap || (mode == ModeFull && cycleDue(lastOlt, cfg.OltDerivedSeconds))
	if runOlt && !alertthresholds.OltOnuQuantityAlertsEnabled(ctx, pool) {
		runOlt = false
	}

	if !runLat && !runTel && !runIf && !runOlt {
		return
	}

	if runLat {
		setActivity(ctx, pool, "1/5 — Ping (ICMP/TCP) em todos os equipamentos")
		if err := RunLatencySweep(ctx, pool, log, mode, SweepOpts{Source: src, Force: bootstrap}); err != nil {
			log.Warn().Err(err).Msg("pipeline: latência")
		}
		setActivity(ctx, pool, "")
	}

	if mode != ModeFull {
		return
	}

	if runTel {
		setActivity(ctx, pool, "2/5 — Telemetria SNMP (equipamentos com telemetria activa)")
		if err := RunTelemetrySweep(ctx, pool, log, mode, SweepOpts{Source: src, Force: bootstrap}); err != nil {
			log.Warn().Err(err).Msg("pipeline: telemetria")
		}
		setActivity(ctx, pool, "")
	}

	if runIf {
		setActivity(ctx, pool, "3/5 — Interfaces SNMP (MikroTik / RouterOS)")
		if err := RunInterfaceSnapshotSweep(ctx, pool, log, mode, SweepOpts{Source: src, Force: bootstrap, InterfacePhase: InterfacePhaseMikrotik}); err != nil {
			log.Warn().Err(err).Msg("pipeline: interfaces MikroTik")
		}
		setActivity(ctx, pool, "4/5 — Interfaces SNMP (OLT)")
		if err := RunInterfaceSnapshotSweep(ctx, pool, log, mode, SweepOpts{Source: src, Force: bootstrap, InterfacePhase: InterfacePhaseOLT}); err != nil {
			log.Warn().Err(err).Msg("pipeline: interfaces OLT")
		}
		setActivity(ctx, pool, "")
	}

	if runOlt {
		setActivity(ctx, pool, "5/5 — PON/ONUs derivados IF-MIB (OLT compatíveis)")
		if err := RunOltIfDerivedSweep(ctx, pool, log, mode, SweepOpts{Source: src, Force: bootstrap}); err != nil {
			log.Warn().Err(err).Msg("pipeline: OLT IF-derived")
		}
		setActivity(ctx, pool, "")
	}

	log.Info().Bool("bootstrap", bootstrap).Str("mode", mode).
		Bool("ran_latency", runLat).Bool("ran_telemetry", runTel).
		Bool("ran_interfaces", runIf).Bool("ran_olt_if", runOlt).
		Msg("pipeline de monitorização concluído")
}

// PipelineHasWorkDue indica se algum passo está em janela de execução (para o tick decidir se dispara goroutine).
func PipelineHasWorkDue(ctx context.Context, pool *pgxpool.Pool, mode string) (bool, error) {
	cfg, err := loadClampMonitoringIntervals(ctx, pool)
	if err != nil {
		return false, err
	}
	var lastLat, lastTel, lastIf, lastOlt *time.Time
	if err := pool.QueryRow(ctx, `
		SELECT last_latency_cycle_at, last_telemetry_cycle_at,
			last_interface_snapshot_cycle_at, last_olt_if_derived_cycle_at
		FROM monitoring_runtime WHERE id=1`).Scan(&lastLat, &lastTel, &lastIf, &lastOlt); err != nil {
		return false, err
	}
	if cycleDue(lastLat, cfg.PingSeconds) {
		return true, nil
	}
	if mode != ModeFull {
		return false, nil
	}
	oltDue := cycleDue(lastOlt, cfg.OltDerivedSeconds) && alertthresholds.OltOnuQuantityAlertsEnabled(ctx, pool)
	return cycleDue(lastTel, cfg.TelemetrySeconds) ||
		cycleDue(lastIf, cfg.IfaceSeconds) ||
		oltDue, nil
}
