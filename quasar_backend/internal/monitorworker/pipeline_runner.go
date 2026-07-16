package monitorworker

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

// RunConfiguredPipeline executa os passos configurados em sequência (cada um só inicia após o anterior terminar).
// Com SkipPingInPipeline, passos «ping» são ignorados (correm em paralelo via TryStartParallelPingCycle).
func RunConfiguredPipeline(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, mode string, opts SweepOpts) error {
	if pool == nil {
		return nil
	}
	steps, err := LoadPipelineSteps(ctx, pool)
	if err != nil && log != nil {
		log.Warn().Err(err).Msg("pipeline: carregar passos")
		steps = DefaultPipelineSteps()
	}
	enabled := EnabledPipelineSteps(steps)
	if len(enabled) == 0 {
		return nil
	}

	src := strings.TrimSpace(opts.Source)
	if src == "" {
		src = "pipeline"
	}
	total := len(enabled)
	ran := 0

	for _, step := range enabled {
		if ctx.Err() != nil {
			break
		}
		if opts.SkipPingInPipeline && step.Kind == StepKindPing {
			continue
		}
		if mode == ModeSimplePing && step.Kind != StepKindPing {
			continue
		}
		if mode != ModeFull && step.Kind != StepKindPing {
			continue
		}

		ran++
		label := fmt.Sprintf("%d/%d — %s", ran, total, pipelineStepLabel(step.Kind))
		setActivity(ctx, pool, label)

		devices, derr := loadDevicesForPipelineStep(ctx, pool, step, opts.DeviceID)
		if derr != nil {
			if log != nil {
				log.Warn().Err(derr).Str("step", step.ID).Msg("pipeline: equipamentos")
			}
			continue
		}

		stepOpts := opts
		stepOpts.Source = "pipeline"
		stepOpts.PipelineStep = &step
		stepOpts.ScopedDevices = devices
		if step.Kind == StepKindOltOnu {
			// Cadência própria por modo (PON status / contagens / full).
			stepOpts.Force = opts.Force
			onuMode := "full"
			if step.Options.OltOnuMode != "" {
				onuMode = step.Options.OltOnuMode
			}
			cfg, cerr := loadClampMonitoringIntervals(ctx, pool)
			if cerr == nil && !oltOnuStepDue(ctx, pool, cfg, onuMode, stepOpts.Force) {
				if log != nil {
					log.Debug().Str("step_id", step.ID).Str("mode", NormalizeOltOnuMode(onuMode)).
						Msg("pipeline: passo OLT adiado (intervalo/agenda)")
				}
				continue
			}
		} else {
			stepOpts.Force = true
		}

		if err := runPipelineStep(ctx, pool, log, mode, step, stepOpts); err != nil && log != nil {
			log.Warn().Err(err).Str("step_id", step.ID).Str("kind", step.Kind).Msg("pipeline: passo falhou")
		} else if step.Kind == StepKindOltOnu {
			onuMode := "full"
			if step.Options.OltOnuMode != "" {
				onuMode = step.Options.OltOnuMode
			}
			markOltOnuTierRan(ctx, pool, onuMode)
		}
	}

	setActivity(ctx, pool, "")
	_, _ = pool.Exec(ctx, `
		UPDATE monitoring_runtime SET
			last_pipeline_cycle_at = now(),
			last_cycle_at = now(),
			updated_at = now()
		WHERE id = 1
	`)
	if log != nil {
		log.Info().Int("steps", total).Str("source", src).Msg("pipeline de monitoramento concluído")
	}
	appendWorkerAudit(ctx, pool, log, "monitoring_cycle", "pipeline", "run", map[string]any{
		"source":           src,
		"steps":            total,
		"force":            opts.Force,
		"skip_ping":        opts.SkipPingInPipeline,
		"ping_parallel":    opts.SkipPingInPipeline,
	})
	return nil
}

func runPipelineStep(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, mode string, step PipelineStep, opts SweepOpts) error {
	switch step.Kind {
	case StepKindPing:
		return RunLatencySweep(ctx, pool, log, mode, opts)
	case StepKindTelemetry:
		return RunTelemetrySweep(ctx, pool, log, mode, opts)
	case StepKindBng:
		return RunBngSweep(ctx, pool, log, mode, opts)
	case StepKindOltOnu:
		return RunOltIfDerivedSweep(ctx, pool, log, mode, opts)
	case StepKindMikrotik:
		opts.InterfacePhase = InterfacePhaseMikrotik
		return RunInterfaceSnapshotSweep(ctx, pool, log, mode, opts)
	case StepKindSwitch:
		opts.InterfacePhase = InterfacePhaseSwitch
		return RunInterfaceSnapshotSweep(ctx, pool, log, mode, opts)
	case StepKindInterfacesOLT:
		opts.InterfacePhase = InterfacePhaseOLT
		return RunInterfaceSnapshotSweep(ctx, pool, log, mode, opts)
	case StepKindInterfacesMikrotik:
		opts.InterfacePhase = InterfacePhaseMikrotik
		return RunInterfaceSnapshotSweep(ctx, pool, log, mode, opts)
	case StepKindInterfacesSwitch:
		opts.InterfacePhase = InterfacePhaseSwitch
		return RunInterfaceSnapshotSweep(ctx, pool, log, mode, opts)
	default:
		return fmt.Errorf("tipo de passo desconhecido: %s", step.Kind)
	}
}
