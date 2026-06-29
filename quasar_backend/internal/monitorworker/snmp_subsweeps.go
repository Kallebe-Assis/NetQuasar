package monitorworker

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"github.com/netquasar/netquasar/quasar_backend/internal/snmpdevicelock"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpdiscovery"
	"github.com/netquasar/netquasar/quasar_backend/internal/telemetryengine"
)

// RunTelemetrySweep coleta telemetria SNMP (CPU, memória, uptime, etc.) para todos os equipamentos elegíveis.
// Cada ciclo grava amostra ou motivo de falha/skip em telemetry_samples e actualiza snmp_health_* no cache.
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

	devices, err := resolveSweepDevices(ctx, pool, opts, false)
	if err != nil {
		return err
	}
	if len(devices) == 0 {
		_, err = pool.Exec(ctx, `UPDATE monitoring_runtime SET last_telemetry_cycle_at = now(), updated_at = now(), last_cycle_at = now() WHERE id=1`)
		return err
	}

	var defCommunity *string
	_ = pool.QueryRow(ctx, `SELECT snmp_community FROM settings_connection_defaults WHERE id=1`).Scan(&defCommunity)

	eligible := 0
	processed := 0
	okN := 0
	failN := 0
	skipN := 0

	for _, row := range devices {
		if !row.telemetryEnabled {
			skipN++
			recordTelemetryCycleOutcome(ctx, pool, row.id, src, telemetryCycleOutcome{
				Skipped: true,
				Reason:  "telemetria desativada no equipamento",
			})
			continue
		}
		if isBngDevice(row) {
			skipN++
			recordTelemetryCycleOutcome(ctx, pool, row.id, src, telemetryCycleOutcome{
				Skipped: true,
				Reason:  "coleta BNG no passo dedicado do pipeline",
			})
			continue
		}
		comm := resolveSNMPCommunity(row, defCommunity)
		if comm == "" {
			skipN++
			recordTelemetryCycleOutcome(ctx, pool, row.id, src, telemetryCycleOutcome{
				Skipped: true,
				Reason:  "community SNMP não configurada",
			})
			patchProbeSNMPHealth(ctx, pool, row.id, ModeSimplePing, false, "failed",
				"community SNMP não configurada",
				probeDetailFromTelemetry(src, map[string]any{"ok": false, "skipped": true, "reason": "snmp_community_missing"}, nil))
			continue
		}
		eligible++

		unlock := snmpdevicelock.Acquire(row.id)
		func() {
			defer unlock()
			processed++
			sctx, scancel := context.WithTimeout(ctx, cfg.telemetryTimeout())
			defer scancel()

			invEmptyBefore, invQErr := snmpInventoryEmpty(sctx, pool, row.id)
			if invQErr != nil {
				invEmptyBefore = true
			}
			if invEmptyBefore {
				_, invErr := snmpdiscovery.EnsureFreshInventory(sctx, pool, log, row.id, snmpdiscovery.DefaultInventoryMaxAge)
				if invErr != nil && log != nil {
					log.Warn().Err(invErr).Str("device", row.id.String()).Msg("telemetry_sweep inventário")
				}
			}

			c, telErr := telemetryengine.CollectAndStore(sctx, pool, row.id, strings.TrimSpace(row.ip), comm)
			snmpOK := telErr == nil && c.OK
			healthStatus := "ok"
			healthReason := ""
			if telErr != nil {
				failN++
				healthStatus = "failed"
				healthReason = strings.TrimSpace(telErr.Error())
				recordTelemetryCycleOutcome(sctx, pool, row.id, src, telemetryCycleOutcome{
					OK: false, Reason: healthReason,
				})
			} else if !c.OK {
				failN++
				healthStatus = "partial"
				healthReason = strings.TrimSpace(c.SNMP.Error)
				if healthReason == "" {
					healthReason = "SNMP sem retorno útil"
				}
			}
			if telErr == nil && c.Metrics != nil {
				if mk, ok := c.Metrics["mikrotik_collection"]; ok {
					if doc, ok := mk.(map[string]any); ok {
						if st, ok := doc["status"].(map[string]any); ok {
							if coll, _ := st["collected"].(float64); coll > 0 {
								snmpOK = true
							}
						}
					}
				}
				if bn, ok := c.Metrics["bng_collection"]; ok {
					if doc, ok := bn.(map[string]any); ok {
						if st, ok := doc["status"].(map[string]any); ok {
							if coll, _ := st["collected"].(float64); coll > 0 {
								snmpOK = true
							}
						}
					}
				}
			}
			var snmpDetail any
			if telErr != nil {
				snmpDetail = map[string]any{"ok": false, "error": telErr.Error(), "source": "telemetryengine"}
			} else {
				snmpDetail = c.SNMP
			}
			var mikrotikDetail any
			if telErr == nil && c.Metrics != nil {
				mikrotikDetail = c.Metrics["mikrotik_collection"]
			}
			patchProbeSNMPHealth(sctx, pool, row.id, ModeSimplePing, snmpOK, healthStatus, healthReason,
				probeDetailFromTelemetry(src, snmpDetail, mikrotikDetail))

			if snmpOK {
				okN++
				RunPostTelemetryAlertEval(sctx, pool, log, row.id, row.description, strings.TrimSpace(row.ip), comm, row.category, row.brand, row.model, c)
				NudgeMonitoringRuntimeRefresh(sctx, pool)
			} else if telErr != nil && log != nil {
				log.Warn().Err(telErr).Str("device", row.id.String()).Str("host", strings.TrimSpace(row.ip)).
					Msg("telemetria SNMP falhou")
			}
		}()
	}

	if log != nil && eligible > 0 {
		log.Info().Int("eligible", eligible).Int("processed", processed).Str("source", src).
			Msg("ciclo telemetria SNMP concluído")
	}
	if log != nil && eligible > processed {
		log.Warn().Int("eligible", eligible).Int("processed", processed).
			Msg("ciclo telemetria incompleto (equipamentos não processados)")
	}

	appendWorkerAudit(ctx, pool, log, "monitoring_cycle", CycleSlugTelemetry, "run", map[string]any{
		"source":    src,
		"eligible":  eligible,
		"processed": processed,
		"ok":        okN,
		"failed":    failN,
		"skipped":   skipN,
	})

	_, err = pool.Exec(ctx, `
		UPDATE monitoring_runtime SET
			last_telemetry_cycle_at = now(),
			last_cycle_at = now(),
			updated_at = now()
		WHERE id = 1
	`)
	return err
}

// RunInterfaceSnapshotSweep grava apenas interface_snapshots (IF-MIB) para equipamentos alcançáveis.
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
	oltProcessed := 0
	ifaceAttempted := 0
	ifaceSkipped := 0
	if ph == InterfacePhaseOLT {
		for _, row := range devices {
			if strings.EqualFold(strings.TrimSpace(row.category), "olt") && row.telemetryEnabled {
				oltEligible++
			}
		}
	}

	for _, row := range devices {
		if !row.telemetryEnabled {
			continue
		}
		if ph == InterfacePhaseMikrotik && !workerLikelyMikrotik(row.category, row.brand, row.model, row.description) {
			continue
		}
		if ph == InterfacePhaseOLT && !strings.EqualFold(strings.TrimSpace(row.category), "olt") {
			continue
		}
		comm := resolveSNMPCommunity(row, defCommunity)
		if comm == "" {
			ifaceSkipped++
			continue
		}

		lastIf := lastIfaceByDevice[row.id]
		if !sweepShouldCollectDevice(opts, lastIf, ifaceDur) {
			continue
		}

		unlock := snmpdevicelock.Acquire(row.id)
		func() {
			defer unlock()
			ifaceAttempted++
			perDeviceTimeout := cfg.interfaceTimeout(ph == InterfacePhaseOLT, ph == InterfacePhaseMikrotik)
			sctx, scancel := context.WithTimeout(ctx, perDeviceTimeout)
			defer scancel()
			t0 := time.Now()

			if ph != InterfacePhaseOLT {
				invEmptyBefore, _ := snmpInventoryEmpty(sctx, pool, row.id)
				if invEmptyBefore {
					_, _ = snmpdiscovery.EnsureFreshInventory(sctx, pool, log, row.id, snmpdiscovery.DefaultInventoryMaxAge)
				}
			}

			CollectInterfaceSnapshotWorker(sctx, pool, log, row.id, strings.TrimSpace(row.ip), comm,
				row.category, row.brand, row.model, row.description)
			lastIfaceByDevice[row.id] = time.Now()
			NudgeMonitoringRuntimeRefresh(sctx, pool)
			if ph == InterfacePhaseOLT {
				oltProcessed++
				setActivity(ctx, pool, "4/5 — Interfaces SNMP (OLT) ["+strconv.Itoa(oltProcessed)+"/"+strconv.Itoa(oltEligible)+"]")
				if log != nil {
					log.Info().
						Str("phase", "interfaces_olt").
						Int("progress_done", oltProcessed).
						Int("progress_total", oltEligible).
						Str("device_id", row.id.String()).
						Str("host", strings.TrimSpace(row.ip)).
						Int64("timeout_ms", perDeviceTimeout.Milliseconds()).
						Int64("device_collect_ms", time.Since(t0).Milliseconds()).
						Msg("interface sweep OLT concluído")
				}
			}
		}()
	}

	_, err = pool.Exec(ctx, `
		UPDATE monitoring_runtime SET last_interface_snapshot_cycle_at = now(), last_cycle_at = now(), updated_at = now()
		WHERE id = 1
	`)
	appendWorkerAudit(ctx, pool, log, "monitoring_cycle", CycleSlugInterfaces, "run", map[string]any{
		"source":    src,
		"phase":     ph,
		"attempted": ifaceAttempted,
		"skipped":   ifaceSkipped,
		"olt_done":  oltProcessed,
		"olt_total": oltEligible,
	})
	return err
}

// RunOltIfDerivedSweep coleta ONUs/PON em todas as OLTs (delega para RunOltCollectAll).
func RunOltIfDerivedSweep(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, mode string, opts SweepOpts) error {
	_, err := RunOltCollectAll(ctx, pool, log, mode, opts)
	return err
}
