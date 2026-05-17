package monitorworker

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"github.com/netquasar/netquasar/quasar_backend/internal/alertthresholds"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpdevicelock"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpdiscovery"
	"github.com/netquasar/netquasar/quasar_backend/internal/telemetryengine"
)

// RunTelemetrySweep colecta apenas telemetria SNMP (CPU, memória, etc.) quando o host está reach_ok no cache.
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

	devices, err := loadPingableDevices(ctx, pool, opts.DeviceID)
	if err != nil {
		return err
	}
	if len(devices) == 0 {
		_, err = pool.Exec(ctx, `UPDATE monitoring_runtime SET last_telemetry_cycle_at = now(), updated_at = now(), last_cycle_at = now() WHERE id=1`)
		return err
	}

	var defCommunity *string
	_ = pool.QueryRow(ctx, `SELECT snmp_community FROM settings_connection_defaults WHERE id=1`).Scan(&defCommunity)

	telIDs := make([]uuid.UUID, len(devices))
	for i := range devices {
		telIDs[i] = devices[i].id
	}

	lastTel := map[uuid.UUID]time.Time{}
	q := `
		SELECT device_id, max(collected_at) AS last_at
		FROM telemetry_samples
		WHERE device_id = ANY($1::uuid[])
		GROUP BY device_id
	`
	trows, err := pool.Query(ctx, q, telIDs)
	if err != nil {
		return err
	}
	defer trows.Close()
	for trows.Next() {
		var did uuid.UUID
		var t time.Time
		if err := trows.Scan(&did, &t); err != nil {
			return err
		}
		lastTel[did] = t
	}
	if err := trows.Err(); err != nil {
		return err
	}

	telDur := time.Duration(cfg.TelemetrySeconds) * time.Second

	for _, row := range devices {
		if !row.telemetryEnabled {
			continue
		}
		comm := resolveSNMPCommunity(row, defCommunity)
		if comm == "" {
			continue
		}
		var reachOK bool
		qerr := pool.QueryRow(ctx, `
			SELECT reach_ok FROM device_probe_cache WHERE device_id = $1
		`, row.id).Scan(&reachOK)
		if qerr != nil {
			continue
		}
		if !reachOK {
			continue
		}

		lastAt, had := lastTel[row.id]
		need := opts.Force || !had || time.Since(lastAt) >= telDur
		if !need {
			continue
		}

		unlock := snmpdevicelock.Acquire(row.id)
		func() {
			defer unlock()
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
			var snmpDetail any
			if telErr != nil {
				snmpDetail = map[string]any{"ok": false, "error": telErr.Error(), "source": "telemetryengine"}
			} else {
				snmpDetail = c.SNMP
			}
			snippet, _ := json.Marshal(map[string]any{
				"snmp": snmpDetail,
				"telemetry_cycle": map[string]any{"source": src},
			})
			WithDeviceProbeRowLock(row.id, func() {
				if _, uerr := pool.Exec(sctx, `
				UPDATE device_probe_cache SET
					snmp_ok = $2,
					ok = CASE WHEN monitoring_mode = $3 THEN reach_ok ELSE (reach_ok AND $2::bool) END,
					detail = COALESCE(detail, '{}'::jsonb) || $4::jsonb,
					checked_at = now()
				WHERE device_id = $1
			`, row.id, snmpOK, ModeSimplePing, string(snippet)); uerr != nil && log != nil {
					log.Warn().Err(uerr).Str("device", row.id.String()).Msg("telemetry_sweep probe cache")
				}
			})

			lastTel[row.id] = time.Now()
			if telErr == nil && c.OK {
				RunPostTelemetryAlertEval(sctx, pool, log, row.id, row.description, strings.TrimSpace(row.ip), comm, row.category, row.brand, row.model, c)
			}
		}()
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

// RunInterfaceSnapshotSweep grava apenas interface_snapshots (IF-MIB) para equipamentos alcançáveis.
func RunInterfaceSnapshotSweep(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, mode string, opts SweepOpts) error {
	if mode != ModeFull {
		return nil
	}
	cfg, err := loadClampMonitoringIntervals(ctx, pool)
	if err != nil {
		return err
	}
	devices, err := loadPingableDevices(ctx, pool, opts.DeviceID)
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

	ph := strings.TrimSpace(strings.ToLower(opts.InterfacePhase))
	oltEligible := 0
	oltProcessed := 0
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
			continue
		}

		var reachOK bool
		qerr := pool.QueryRow(ctx, `SELECT reach_ok FROM device_probe_cache WHERE device_id=$1`, row.id).Scan(&reachOK)
		if qerr != nil || !reachOK {
			continue
		}

		lastIf := lastIfaceByDevice[row.id]
		need := opts.Force || lastIf.IsZero() || time.Since(lastIf) >= ifaceDur
		if !need {
			continue
		}

		unlock := snmpdevicelock.Acquire(row.id)
		func() {
			defer unlock()
			perDeviceTimeout := cfg.interfaceTimeout(ph == InterfacePhaseOLT)
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
	return err
}

// RunOltIfDerivedSweep apenas para OLT com PON derivada por IF-MIB (ex.: VSOL/Mikrotik óptica); ZTE/datacom ficam omitidos pela guard.
func RunOltIfDerivedSweep(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, mode string, opts SweepOpts) error {
	if mode != ModeFull {
		return nil
	}
	cfg, err := loadClampMonitoringIntervals(ctx, pool)
	if err != nil {
		return err
	}
	if !alertthresholds.OltOnuQuantityAlertsEnabled(ctx, pool) {
		if log != nil {
			log.Debug().Msg("OLT IF-derived: omitido — nenhum limiar global de quantidade/queda de ONUs activo")
		}
		_, err = pool.Exec(ctx, `UPDATE monitoring_runtime SET last_olt_if_derived_cycle_at = now(), last_cycle_at = now(), updated_at = now() WHERE id=1`)
		return err
	}
	devices, err := loadPingableDevices(ctx, pool, opts.DeviceID)
	if err != nil {
		return err
	}
	if len(devices) == 0 {
		_, err = pool.Exec(ctx, `UPDATE monitoring_runtime SET last_olt_if_derived_cycle_at = now(), updated_at = now(), last_cycle_at = now() WHERE id=1`)
		return err
	}

	lastOltByDevice, err := loadOltSnapshotByDevice(ctx, pool)
	if err != nil {
		return err
	}

	var defCommunity *string
	_ = pool.QueryRow(ctx, `SELECT snmp_community FROM settings_connection_defaults WHERE id=1`).Scan(&defCommunity)

	oltDur := time.Duration(cfg.OltDerivedSeconds) * time.Second

	for _, row := range devices {
		if !strings.EqualFold(strings.TrimSpace(row.category), "olt") {
			continue
		}
		if !OltUsesIfDerivedPonSnapshots(row.category, row.brand, row.model) {
			// Evita alerta "preso" de queda PON por pipeline IF-MIB em vendors não suportados (ZTE/Datacom).
			if _, cerr := pool.Exec(ctx, `
				UPDATE alert_instances
				SET closed_at = now(),
					meta = COALESCE(meta,'{}'::jsonb) || '{"resolved":"incompatible_if_mib_vendor","source":"monitor_worker"}'::jsonb
				WHERE device_id = $1::uuid
				  AND alert_type = 'olt_onu_drop'
				  AND closed_at IS NULL
			`, row.id); cerr != nil && log != nil {
				log.Warn().Err(cerr).Str("device", row.id.String()).Msg("close incompatible IF-MIB olt_onu_drop")
			}
			continue
		}
		if !row.telemetryEnabled {
			continue
		}
		comm := resolveSNMPCommunity(row, defCommunity)
		if comm == "" {
			continue
		}

		var reachOK bool
		qerr := pool.QueryRow(ctx, `SELECT reach_ok FROM device_probe_cache WHERE device_id=$1`, row.id).Scan(&reachOK)
		if qerr != nil || !reachOK {
			continue
		}

		lastOt := lastOltByDevice[row.id]
		need := opts.Force || lastOt.IsZero() || time.Since(lastOt) >= oltDur
		if !need {
			continue
		}

		unlock := snmpdevicelock.Acquire(row.id)
		func() {
			defer unlock()
			// Mantém limite curto por OLT para não bloquear ciclo por muitos minutos.
			sctx, scancel := context.WithTimeout(ctx, cfg.oltIfDerivedTimeout())
			defer scancel()

			invEmptyBefore, _ := snmpInventoryEmpty(sctx, pool, row.id)
			if invEmptyBefore {
				_, _ = snmpdiscovery.EnsureFreshInventory(sctx, pool, log, row.id, snmpdiscovery.DefaultInventoryMaxAge)
			}

			CollectOltPonAndEvaluate(sctx, pool, log, row.id, strings.TrimSpace(row.ip), comm, row.description, row.category, row.brand, row.model)
			lastOltByDevice[row.id] = time.Now()
		}()
	}

	_, err = pool.Exec(ctx, `
		UPDATE monitoring_runtime SET last_olt_if_derived_cycle_at = now(), last_cycle_at = now(), updated_at = now()
		WHERE id = 1
	`)
	return err
}
