package monitorworker

import (
	"context"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"github.com/netquasar/netquasar/quasar_backend/internal/bgpcollect"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpdevicelock"
)

// RunBgpSweep coleta SNMP periódica de BGP (peers/interfaces/tráfego/saúde, conforme o perfil
// is_default de bgp_snmp_profiles) — mirror directo de RunBngSweep, para equipamentos com
// bgp_enabled=true. Guarda em telemetry_samples (ver bgpcollect.CollectAndStoreForDevice).
func RunBgpSweep(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, mode string, opts SweepOpts) error {
	if mode != ModeFull {
		return nil
	}
	cfg, err := loadClampMonitoringIntervals(ctx, pool)
	if err != nil {
		return err
	}
	var devices []pingableDeviceRow
	if len(opts.ScopedDevices) > 0 {
		devices = opts.ScopedDevices
	} else {
		devices, err = loadBgpDevicesForCollect(ctx, pool, opts.DeviceID)
		if err != nil {
			return err
		}
	}
	if len(devices) == 0 {
		_, err = pool.Exec(ctx, `UPDATE monitoring_runtime SET last_bgp_cycle_at = now(), updated_at = now(), last_cycle_at = now() WHERE id=1`)
		return err
	}

	profile := bgpcollect.LoadDefaultProfile(ctx, pool)
	var defCommunity *string
	_ = pool.QueryRow(ctx, `SELECT snmp_community FROM settings_connection_defaults WHERE id=1`).Scan(&defCommunity)

	src := opts.Source
	if src == "" {
		src = "worker"
	}

	eligible, processed, okN, failN, skipN := 0, 0, 0, 0, 0
	timeout := cfg.bngTimeout()
	var ctrMu sync.Mutex
	limit := sweepConcurrency()

	forEachLimited(ctx, len(devices), limit, func(i int) {
		row := devices[i]
		comm := resolveSNMPCommunity(row, defCommunity)
		if comm == "" {
			ctrMu.Lock()
			skipN++
			ctrMu.Unlock()
			return
		}
		ctrMu.Lock()
		eligible++
		ctrMu.Unlock()

		unlock := snmpdevicelock.Acquire(row.id)
		defer unlock()
		ctrMu.Lock()
		processed++
		ctrMu.Unlock()
		sctx, scancel := context.WithTimeout(ctx, timeout)
		defer scancel()

		out, cErr := bgpcollect.CollectAndStoreForDevice(sctx, pool, row.id, strings.TrimSpace(row.ip), comm, timeout, profile)
		if cErr == nil && out.CollectedCount > 0 {
			ctrMu.Lock()
			okN++
			ctrMu.Unlock()
			NudgeMonitoringRuntimeRefresh(sctx, pool)
		} else {
			ctrMu.Lock()
			failN++
			ctrMu.Unlock()
			if cErr != nil && log != nil {
				log.Warn().Err(cErr).Str("device", row.id.String()).Str("host", strings.TrimSpace(row.ip)).Msg("coleta BGP falhou")
			}
		}
	})

	if log != nil && eligible > 0 {
		log.Info().Int("eligible", eligible).Int("processed", processed).Str("source", src).
			Int("concurrency", limit).Msg("ciclo BGP concluído")
	}

	appendWorkerAudit(ctx, pool, log, "monitoring_cycle", CycleSlugBgp, "run", map[string]any{
		"source":      src,
		"eligible":    eligible,
		"processed":   processed,
		"ok":          okN,
		"failed":      failN,
		"skipped":     skipN,
		"concurrency": limit,
	})

	_, err = pool.Exec(ctx, `
		UPDATE monitoring_runtime SET last_bgp_cycle_at = now(), last_cycle_at = now(), updated_at = now()
		WHERE id = 1
	`)
	return err
}
