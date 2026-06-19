package monitorworker

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
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

	if !runLat && !runTel && !runIf && !runOlt {
		return
	}

	if bootstrap {
		LockLatencyCycle()
		defer UnlockLatencyCycle()
	}

	if runLat {
		setActivity(ctx, pool, "1/5 — Ping (ICMP/TCP) em todos os equipamentos")
		err := RunLatencySweep(ctx, pool, log, mode, SweepOpts{Source: src, Force: bootstrap})
		if err != nil {
			log.Warn().Err(err).Msg("pipeline: latência")
		}
		setActivity(ctx, pool, "")
	}

	if mode != ModeFull {
		return
	}

	runWorkerSNMPStepsFromFlags(ctx, pool, log, mode, src, bootstrap, runTel, runIf, runOlt)
}

// RunWorkerInterfaceSteps executa apenas snapshots IF-MIB (sem telemetria).
func RunWorkerInterfaceSteps(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, mode string, bootstrap bool) {
	if pool == nil || log == nil || mode != ModeFull {
		return
	}
	cfg, err := loadClampMonitoringIntervals(ctx, pool)
	if err != nil {
		log.Error().Err(err).Msg("interface pipeline: intervalos")
		return
	}
	var lastIf *time.Time
	_ = pool.QueryRow(ctx, `SELECT last_interface_snapshot_cycle_at FROM monitoring_runtime WHERE id=1`).Scan(&lastIf)
	src := "worker"
	if bootstrap {
		src = "bootstrap"
	}
	runIf := bootstrap || cycleDue(lastIf, cfg.IfaceSeconds)
	if !runIf {
		return
	}
	setActivity(ctx, pool, "3/5 — Interfaces SNMP (MikroTik / RouterOS)")
	if err := RunInterfaceSnapshotSweep(ctx, pool, log, mode, SweepOpts{Source: src, Force: bootstrap, InterfacePhase: InterfacePhaseMikrotik}); err != nil {
		log.Warn().Err(err).Msg("pipeline: interfaces MikroTik")
	}
	setActivity(ctx, pool, "4/5 — Interfaces SNMP (OLT)")
	if err := RunInterfaceSnapshotSweep(ctx, pool, log, mode, SweepOpts{Source: src, Force: bootstrap, InterfacePhase: InterfacePhaseOLT}); err != nil {
		log.Warn().Err(err).Msg("pipeline: interfaces OLT")
	}
	setActivity(ctx, pool, "")
	log.Info().Bool("bootstrap", bootstrap).Bool("ran_interfaces", true).Msg("pipeline interfaces concluído")
}

// RunWorkerSNMPSteps executa telemetria e interfaces (modo full), sem ICMP/TCP.
func RunWorkerSNMPSteps(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, mode string, bootstrap bool) {
	if pool == nil || log == nil || mode != ModeFull {
		return
	}
	cfg, err := loadClampMonitoringIntervals(ctx, pool)
	if err != nil {
		log.Error().Err(err).Msg("snmp pipeline: intervalos")
		return
	}
	var lastTel, lastIf, lastOlt *time.Time
	_ = pool.QueryRow(ctx, `
		SELECT last_telemetry_cycle_at, last_interface_snapshot_cycle_at, last_olt_if_derived_cycle_at
		FROM monitoring_runtime WHERE id=1`).Scan(&lastTel, &lastIf, &lastOlt)
	src := "worker"
	if bootstrap {
		src = "bootstrap"
	}
	runTel := bootstrap || cycleDue(lastTel, cfg.TelemetrySeconds)
	runIf := bootstrap || cycleDue(lastIf, cfg.IfaceSeconds)
	runOlt := bootstrap || cycleDue(lastOlt, cfg.OltDerivedSeconds)
	runWorkerSNMPStepsFromFlags(ctx, pool, log, mode, src, bootstrap, runTel, runIf, runOlt)
}

func runWorkerSNMPStepsFromFlags(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, mode, src string, bootstrap, runTel, runIf, runOlt bool) {
	if !runTel && !runIf && !runOlt {
		return
	}

	if runTel {
		setActivity(ctx, pool, "2/5 — Telemetria SNMP (equipamentos com telemetria ativa)")
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

	if bootstrap && runOlt {
		setActivity(ctx, pool, "5/5 — PON/ONUs (OLT)")
		if err := RunOltIfDerivedSweep(ctx, pool, log, mode, SweepOpts{Source: src, Force: bootstrap}); err != nil {
			log.Warn().Err(err).Msg("pipeline: OLT PON bootstrap")
		}
		setActivity(ctx, pool, "")
	}

	log.Info().Bool("bootstrap", bootstrap).
		Bool("ran_telemetry", runTel).Bool("ran_interfaces", runIf).Bool("ran_olt_if", bootstrap && runOlt).
		Msg("pipeline SNMP concluído")
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
	return cycleDue(lastTel, cfg.TelemetrySeconds) ||
		cycleDue(lastIf, cfg.IfaceSeconds), nil
}
