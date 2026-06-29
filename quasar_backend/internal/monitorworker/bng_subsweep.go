package monitorworker

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"github.com/netquasar/netquasar/quasar_backend/internal/alertthresholds"
	"github.com/netquasar/netquasar/quasar_backend/internal/bngcollect"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpdevicelock"
)

// RunBngSweep coleta SNMP periódica de BNG (totais de logins, saúde, etc.) conforme perfil global.
func RunBngSweep(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, mode string, opts SweepOpts) error {
	if mode != ModeFull {
		return nil
	}
	cfg, err := loadClampMonitoringIntervals(ctx, pool)
	if err != nil {
		return err
	}
	devices, err := resolveSweepDevices(ctx, pool, opts, false)
	if err != nil {
		return err
	}
	if len(devices) == 0 {
		_, err = pool.Exec(ctx, `UPDATE monitoring_runtime SET last_bng_cycle_at = now(), updated_at = now(), last_cycle_at = now() WHERE id=1`)
		return err
	}

	bngMode := "totals"
	if opts.PipelineStep != nil {
		bngMode = strings.TrimSpace(opts.PipelineStep.Options.BngMode)
		if bngMode == "" {
			bngMode = "totals"
		}
	}

	var defCommunity *string
	_ = pool.QueryRow(ctx, `SELECT snmp_community FROM settings_connection_defaults WHERE id=1`).Scan(&defCommunity)

	src := opts.Source
	if src == "" {
		src = "worker"
	}

	eligible, processed, okN, failN, skipN := 0, 0, 0, 0, 0
	timeout := cfg.bngTimeout()

	for _, row := range devices {
		if !isBngDevice(row) {
			continue
		}
		if !row.telemetryEnabled {
			skipN++
			continue
		}
		comm := resolveSNMPCommunity(row, defCommunity)
		if comm == "" {
			skipN++
			continue
		}
		eligible++

		unlock := snmpdevicelock.Acquire(row.id)
		func() {
			defer unlock()
			processed++
			sctx, scancel := context.WithTimeout(ctx, timeout)
			defer scancel()

			out, telErr := bngcollect.CollectAndStorePeriodicMode(sctx, pool, row.id, strings.TrimSpace(row.ip), comm, timeout, bngMode)
			snmpOK := telErr == nil && out.Status.Collected > 0
			if out.Status.Enabled > 0 && out.Status.Collected == 0 && out.Status.Failed == 0 && len(out.Status.MissingOID) > 0 {
				snmpOK = false
			}
			if snmpOK {
				okN++
				alertthresholds.EvaluateBngSubscriberDropAlerts(sctx, pool, log, row.id, row.description, strings.TrimSpace(row.ip), "monitoring_bng")
				NudgeMonitoringRuntimeRefresh(sctx, pool)
			} else {
				failN++
				if telErr != nil && log != nil {
					log.Warn().Err(telErr).Str("device", row.id.String()).Str("host", strings.TrimSpace(row.ip)).Msg("coleta BNG falhou")
				}
			}
		}()
	}

	if log != nil && eligible > 0 {
		log.Info().Int("eligible", eligible).Int("processed", processed).Str("mode", bngMode).Str("source", src).Msg("ciclo BNG concluído")
	}

	appendWorkerAudit(ctx, pool, log, "monitoring_cycle", CycleSlugBng, "run", map[string]any{
		"source":    src,
		"mode":      bngMode,
		"eligible":  eligible,
		"processed": processed,
		"ok":        okN,
		"failed":    failN,
		"skipped":   skipN,
	})

	_, err = pool.Exec(ctx, `
		UPDATE monitoring_runtime SET last_bng_cycle_at = now(), last_cycle_at = now(), updated_at = now()
		WHERE id = 1
	`)
	return err
}
