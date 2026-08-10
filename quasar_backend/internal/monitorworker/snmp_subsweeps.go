package monitorworker

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"github.com/netquasar/netquasar/quasar_backend/internal/mikrotikcollect"
	"github.com/netquasar/netquasar/quasar_backend/internal/monitorview"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpdevicelock"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpdiscovery"
	"github.com/netquasar/netquasar/quasar_backend/internal/telemetryengine"
)

const (
	healthPerDeviceTimeout = 22 * time.Second
	snmpLockWaitMax        = 4 * time.Second
)

// RunTelemetrySweep runs the parallel health telemetry cycle (CPU/mem/temp/uptime).
// Per device: TryAcquire SNMP lock -> CollectHealthAndStore (<=22s) -> PatchProbeKPIs.
// last_telemetry_cycle_at advances only when every eligible device was covered.
func RunTelemetrySweep(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, mode string, opts SweepOpts) error {
	if mode != ModeFull {
		return nil
	}
	cfg, err := loadClampMonitoringIntervals(ctx, pool)
	if err != nil {
		return err
	}
	src := opts.Source
	if src == "" {
		src = "worker"
	}

	devices, err := resolveTelemetryDevices(ctx, pool, opts)
	if err != nil {
		return err
	}
	if len(devices) == 0 {
		_, err = pool.Exec(ctx, `UPDATE monitoring_runtime SET last_telemetry_cycle_at = now(), updated_at = now(), last_cycle_at = now() WHERE id=1`)
		return err
	}

	var defCommunity *string
	_ = pool.QueryRow(ctx, `SELECT snmp_community FROM settings_connection_defaults WHERE id=1`).Scan(&defCommunity)

	var ctr sweepCounters
	limit := sweepConcurrency()
	if limit > 8 {
		limit = 8
	}

	forEachLimited(ctx, len(devices), limit, func(i int) {
		row := devices[i]
		if isBngDevice(row) {
			ctr.addSkip()
			recordTelemetryCycleOutcome(ctx, pool, row.id, src, telemetryCycleOutcome{
				Skipped: true, Reason: "coleta BNG no passo dedicado do pipeline",
			})
			return
		}
		if !row.telemetryEnabled {
			ctr.addSkip()
			recordTelemetryCycleOutcome(ctx, pool, row.id, src, telemetryCycleOutcome{
				Skipped: true, Reason: "telemetria desativada no equipamento",
			})
			return
		}
		comm := resolveSNMPCommunity(row, defCommunity)
		if comm == "" {
			ctr.addSkip()
			recordTelemetryCycleOutcome(ctx, pool, row.id, src, telemetryCycleOutcome{
				Skipped: true, Reason: "community SNMP nao configurada",
			})
			patchProbeSNMPHealth(ctx, pool, row.id, ModeSimplePing, false, "failed",
				"community SNMP nao configurada",
				probeDetailFromTelemetry(src, map[string]any{"ok": false, "skipped": true, "reason": "snmp_community_missing"}, nil))
			return
		}

		ctr.addEligible()
		unlock, gotLock := snmpdevicelock.TryAcquire(ctx, row.id, snmpLockWaitMax)
		if !gotLock {
			ctr.addSkip()
			recordTelemetryCycleOutcome(ctx, pool, row.id, src, telemetryCycleOutcome{
				Skipped: true, Reason: "snmp_lock_busy",
			})
			return
		}
		defer unlock()
		ctr.addProcessed()

		hctx, hcancel := context.WithTimeout(ctx, healthPerDeviceTimeout)
		defer hcancel()

		c, telErr := telemetryengine.CollectHealthAndStore(hctx, pool, row.id, strings.TrimSpace(row.ip), comm)
		snmpOK := telErr == nil && c.OK
		healthStatus := "ok"
		healthReason := ""
		if telErr != nil {
			ctr.addFail()
			healthStatus = "failed"
			healthReason = strings.TrimSpace(telErr.Error())
			recordTelemetryCycleOutcome(ctx, pool, row.id, src, telemetryCycleOutcome{
				OK: false, Reason: healthReason,
			})
		} else if !c.OK {
			ctr.addFail()
			healthStatus = "partial"
			healthReason = strings.TrimSpace(c.SNMP.Error)
			if healthReason == "" {
				healthReason = "SNMP sem retorno util"
			}
		}
		if collectedFromMikrotikOrSwitch(c.Metrics) {
			snmpOK = true
			if healthStatus == "failed" {
				healthStatus = "partial"
			}
		}

		var snmpDetail any
		if telErr != nil && c.Metrics == nil {
			snmpDetail = map[string]any{"ok": false, "error": telErr.Error(), "source": "telemetry_health"}
		} else {
			snmpDetail = c.SNMP
		}
		var mikrotikDetail any
		if c.Metrics != nil {
			mikrotikDetail = c.Metrics["mikrotik_collection"]
			if mikrotikDetail == nil {
				mikrotikDetail = c.Metrics["switch_collection"]
			}
		}

		patchCtx := hctx
		var patchCancel context.CancelFunc
		if hctx.Err() != nil {
			patchCtx, patchCancel = context.WithTimeout(context.WithoutCancel(ctx), 12*time.Second)
			defer patchCancel()
		}

		patchProbeSNMPHealth(patchCtx, pool, row.id, ModeSimplePing, snmpOK, healthStatus, healthReason,
			probeDetailFromTelemetry(src, snmpDetail, mikrotikDetail))

		if c.Metrics != nil {
			if mb, err := json.Marshal(c.Metrics); err == nil {
				WithDeviceProbeRowLock(row.id, func() {
					monitorview.PatchProbeKPIs(patchCtx, pool, row.id, mb, time.Now())
				})
			}
		}

		if snmpOK {
			ctr.addOK()
			// Avaliar limiares globais (CPU/mem/temp/uptime) no mesmo ciclo do worker —
			// antes isto só corria no refresh manual da API.
			RunPostTelemetryAlertEval(patchCtx, pool, log, row.id, row.description, strings.TrimSpace(row.ip), comm,
				row.category, row.brand, row.model, c)
			NudgeMonitoringRuntimeRefresh(patchCtx, pool)
		} else if telErr != nil && log != nil {
			log.Warn().Err(telErr).Str("device", row.id.String()).Str("host", strings.TrimSpace(row.ip)).
				Msg("telemetria health falhou")
		}
	})

	incomplete := ctx.Err() != nil || (ctr.eligible > 0 && ctr.processed < ctr.eligible)
	if log != nil {
		ev := log.Info()
		if incomplete {
			ev = log.Warn()
		}
		ev.Int("eligible", ctr.eligible).Int("processed", ctr.processed).
			Int("ok", ctr.ok).Int("failed", ctr.fail).Int("skipped", ctr.skip).
			Bool("incomplete", incomplete).Str("source", src).
			Msg("ciclo telemetria health concluido")
	}

	appendWorkerAudit(ctx, pool, log, "monitoring_cycle", CycleSlugTelemetry, "run", map[string]any{
		"source":      src,
		"phase":       "health",
		"eligible":    ctr.eligible,
		"processed":   ctr.processed,
		"ok":          ctr.ok,
		"failed":      ctr.fail,
		"skipped":     ctr.skip,
		"incomplete":  incomplete,
		"concurrency": limit,
	})

	if incomplete {
		// Nao avancar o intervalo completo: reagendar em ~15s para cobrir devices
		// que falharam por lock SNMP / timeout, sem busy-loop a cada tick de 1s.
		_, _ = pool.Exec(ctx, `
			UPDATE monitoring_runtime SET
				last_telemetry_cycle_at = now() - make_interval(secs => greatest($1 - 15, 0)),
				updated_at = now()
			WHERE id = 1
		`, cfg.TelemetrySeconds)
		return ctx.Err()
	}
	_, err = pool.Exec(ctx, `
		UPDATE monitoring_runtime SET
			last_telemetry_cycle_at = now(),
			last_cycle_at = now(),
			updated_at = now()
		WHERE id = 1
	`)
	return err
}

func collectedFromMikrotikOrSwitch(metrics map[string]any) bool {
	if metrics == nil {
		return false
	}
	for _, key := range []string{"mikrotik_collection", "switch_collection", "mikrotik_telnet_collection", "switch_telnet_collection"} {
		raw, ok := metrics[key]
		if !ok || raw == nil {
			continue
		}
		switch v := raw.(type) {
		case mikrotikcollect.CollectOutput:
			if v.Status.Collected > 0 {
				return true
			}
		case map[string]any:
			if st, ok := v["status"].(map[string]any); ok {
				switch n := st["collected"].(type) {
				case float64:
					if n > 0 {
						return true
					}
				case int:
					if n > 0 {
						return true
					}
				}
			}
		default:
			b, err := json.Marshal(raw)
			if err != nil {
				continue
			}
			var doc map[string]any
			if json.Unmarshal(b, &doc) != nil {
				continue
			}
			if st, ok := doc["status"].(map[string]any); ok {
				if n, ok := st["collected"].(float64); ok && n > 0 {
					return true
				}
			}
		}
	}
	return false
}

func resolveTelemetryDevices(ctx context.Context, pool *pgxpool.Pool, opts SweepOpts) ([]pingableDeviceRow, error) {
	if len(opts.ScopedDevices) > 0 {
		return opts.ScopedDevices, nil
	}
	return loadTelemetryDevices(ctx, pool, opts.DeviceID)
}

// RunInterfaceSnapshotSweep stores IF-MIB interface_snapshots for reachable devices.
func RunInterfaceSnapshotSweep(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, mode string, opts SweepOpts) error {
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
		_, err = pool.Exec(ctx, `UPDATE monitoring_runtime SET last_interface_snapshot_cycle_at = now(), updated_at = now(), last_cycle_at = now() WHERE id=1`)
		return err
	}

	lastIfaceByDevice, err := loadLatestIfaceByDevice(ctx, pool)
	if err != nil {
		return err
	}

	var defCommunity *string
	_ = pool.QueryRow(ctx, `SELECT snmp_community FROM settings_connection_defaults WHERE id=1`).Scan(&defCommunity)

	ifaceDur := time.Duration(cfg.IfaceSeconds) * time.Second
	src := opts.Source
	if src == "" {
		src = "worker"
	}

	ph := strings.TrimSpace(strings.ToLower(opts.InterfacePhase))
	oltEligible := 0
	var oltProcessed atomicInt
	ifaceAttempted := 0
	ifaceSkipped := 0
	var ifaceMu sync.Mutex
	if ph == InterfacePhaseOLT {
		for _, row := range devices {
			if strings.EqualFold(strings.TrimSpace(row.category), "olt") && row.telemetryEnabled {
				oltEligible++
			}
		}
	}

	limit := sweepConcurrency()
	forEachLimited(ctx, len(devices), limit, func(i int) {
		row := devices[i]
		if !row.telemetryEnabled {
			return
		}
		if ph == InterfacePhaseMikrotik && !workerLikelyMikrotik(row.category, row.brand, row.model, row.description) {
			return
		}
		if ph == InterfacePhaseSwitch && !workerLikelySwitch(row.category) {
			return
		}
		if ph == InterfacePhaseOLT && !strings.EqualFold(strings.TrimSpace(row.category), "olt") {
			return
		}
		comm := resolveSNMPCommunity(row, defCommunity)
		if comm == "" {
			ifaceMu.Lock()
			ifaceSkipped++
			ifaceMu.Unlock()
			return
		}

		ifaceMu.Lock()
		lastIf := lastIfaceByDevice[row.id]
		ifaceMu.Unlock()
		if !sweepShouldCollectDevice(opts, lastIf, ifaceDur) {
			return
		}

		unlock := snmpdevicelock.Acquire(row.id)
		defer unlock()
		ifaceMu.Lock()
		ifaceAttempted++
		ifaceMu.Unlock()
		perDeviceTimeout := cfg.interfaceTimeout(ph == InterfacePhaseOLT, ph == InterfacePhaseMikrotik)
		sctx, scancel := context.WithTimeout(ctx, perDeviceTimeout)
		defer scancel()
		t0 := time.Now()

		CollectInterfaceSnapshotWorker(sctx, pool, log, row.id, strings.TrimSpace(row.ip), comm,
			row.category, row.brand, row.model, row.description)
		if ph != InterfacePhaseOLT && sctx.Err() == nil {
			invEmptyBefore, _ := snmpInventoryEmpty(sctx, pool, row.id)
			if invEmptyBefore {
				invCtx, invCancel := context.WithTimeout(sctx, 15*time.Second)
				_, _ = snmpdiscovery.EnsureFreshInventory(invCtx, pool, log, row.id, snmpdiscovery.DefaultInventoryMaxAge)
				invCancel()
			}
		}
		ifaceMu.Lock()
		lastIfaceByDevice[row.id] = time.Now()
		ifaceMu.Unlock()
		NudgeMonitoringRuntimeRefresh(sctx, pool)
		if ph == InterfacePhaseOLT {
			done := oltProcessed.inc()
			setActivity(ctx, pool, "4/5 - Interfaces SNMP (OLT) ["+strconv.Itoa(done)+"/"+strconv.Itoa(oltEligible)+"]")
			if log != nil {
				log.Info().
					Str("phase", "interfaces_olt").
					Int("progress_done", done).
					Int("progress_total", oltEligible).
					Str("device_id", row.id.String()).
					Str("host", strings.TrimSpace(row.ip)).
					Int64("timeout_ms", perDeviceTimeout.Milliseconds()).
					Int64("device_collect_ms", time.Since(t0).Milliseconds()).
					Msg("interface sweep OLT concluido")
			}
		}
	})

	_, err = pool.Exec(ctx, `
		UPDATE monitoring_runtime SET last_interface_snapshot_cycle_at = now(), last_cycle_at = now(), updated_at = now()
		WHERE id = 1
	`)
	appendWorkerAudit(ctx, pool, log, "monitoring_cycle", CycleSlugInterfaces, "run", map[string]any{
		"source":      src,
		"phase":       ph,
		"attempted":   ifaceAttempted,
		"skipped":     ifaceSkipped,
		"olt_done":    oltProcessed.n,
		"olt_total":   oltEligible,
		"concurrency": limit,
	})
	return err
}

// RunOltIfDerivedSweep collects ONUs/PON on all OLTs (delegates to RunOltCollectAll).
func RunOltIfDerivedSweep(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, mode string, opts SweepOpts) error {
	_, err := RunOltCollectAll(ctx, pool, log, mode, opts)
	return err
}
