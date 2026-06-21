package monitorworker

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

// RunBootstrapPipeline primeira sequência ao iniciar monitoramento full (ping paralelo + pipeline SNMP/OLT).
func RunBootstrapPipeline(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger) {
	if pool == nil {
		return
	}
	cfg, err := loadClampMonitoringIntervals(ctx, pool)
	if err != nil {
		if log != nil {
			log.Error().Err(err).Msg("bootstrap: intervalos")
		}
		return
	}
	TryStartParallelPingCycle(ctx, pool, log, ModeFull, cfg, SweepOpts{Source: "bootstrap", Force: true})
	LockMonitoringPipeline()
	defer UnlockMonitoringPipeline()
	if err := RunConfiguredPipeline(ctx, pool, log, ModeFull, SweepOpts{
		Source:             "bootstrap",
		Force:              true,
		SkipPingInPipeline: cfg.PingParallel,
	}); err != nil && log != nil {
		log.Warn().Err(err).Msg("bootstrap pipeline")
	}
}
