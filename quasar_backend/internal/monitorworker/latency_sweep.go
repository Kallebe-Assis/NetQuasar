package monitorworker

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

// RunLatencySweep colecta apenas ICMP/TCP (latência/alcançabilidade), actualiza ping_history,
// preserva snmp_ok/detalle SNMP vindos dos outros ciclos e grava reach_ok por linha.
func RunLatencySweep(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, mode string, opts SweepOpts) error {
	cfg, err := loadClampMonitoringIntervals(ctx, pool)
	if err != nil {
		return err
	}

	perProbe := time.Duration(cfg.PingTimeoutMs) * time.Millisecond
	icmpPart := perProbe * 2 / 3
	tcpPart := perProbe - icmpPart
	if icmpPart < 500*time.Millisecond {
		icmpPart = 500 * time.Millisecond
	}
	if tcpPart < time.Second {
		tcpPart = time.Second
	}

	devices, err := loadPingableDevices(ctx, pool, opts.DeviceID)
	if err != nil {
		return err
	}

	src := opts.Source
	if src == "" {
		src = "worker"
	}

	setActivity(ctx, pool, "Requisitando ICMP/TCP nos equipamentos (ciclo latência)")
	defer setActivity(ctx, pool, "")

	okN, failN := 0, 0
	var recoveredPing []uuid.UUID

	for _, row := range devices {
		id := row.id
		host := strings.TrimSpace(row.ip)
		description := row.description
		if host == "" {
			continue
		}

		WithDeviceProbeRowLock(id, func() {
			prevSnap := loadPreviousPingSnapshot(ctx, pool, id)

			var snmpPrev sql.NullBool
			_ = pool.QueryRow(ctx, `SELECT snmp_ok FROM device_probe_cache WHERE device_id=$1`, id).Scan(&snmpPrev)

			var streak int
			_ = pool.QueryRow(ctx, `SELECT COALESCE(ping_fail_streak, 0) FROM device_probe_cache WHERE device_id=$1`, id).Scan(&streak)

			pctx, cancel := context.WithTimeout(ctx, perProbe+300*time.Millisecond)
			devLabel := monitoringDeviceLabel(description, host)
			setActivity(ctx, pool, fmt.Sprintf("ICMP/TCP [latência] · %s", devLabel))
			probe := probing.HostReachability(pctx, host, "443", icmpPart, tcpPart, cfg.ICMPPayloadBytes)
			cancel()

			reachOK, _ := probe["ok"].(bool)
			var streakAfter int
			if reachOK {
				streakAfter = 0
			} else {
				streakAfter = streak + 1
			}
			method, _ := probe["method"].(string)
			var lat int64
			switch v := probe["latency_ms"].(type) {
			case int64:
				lat = v
			case float64:
				lat = int64(v)
			case int:
				lat = int64(v)
			}

			detail := map[string]any{
				"reachability":   probe,
				"latency_source": src,
			}

			if shouldOpenPingUnreachableAlert(reachOK, streakAfter, cfg.OfflineThreshold) {
				InsertPingUnreachableIfNew(ctx, pool, log, id, description, host, probe, src)
			}
			if reachOK {
				recoveredPing = append(recoveredPing, id)
			}
			if reachOK {
				patchOpenLatencyHighMeta(ctx, pool, id, lat)
			}
			if !syncLatencyAlertByGlobalThreshold(ctx, pool, log, id, row.category, description, host, reachOK, lat) {
				insertLatencyHighIfNew(ctx, pool, log, id, description, host, prevSnap, lat, reachOK)
				resolveLatencyHighIfCalm(ctx, pool, log, id, reachOK, lat)
			}

			overallOK := compositeProbeOK(mode, reachOK, snmpPrev)
			if overallOK {
				okN++
			} else {
				failN++
			}

			dj, jerr := json.Marshal(detail)
			if jerr != nil || len(dj) == 0 {
				dj = []byte(`{"marshal_error":"detail_not_serializable","source":"latency_sweep"}`)
			}

			var snmpStored any
			if mode == ModeFull {
				if snmpPrev.Valid {
					snmpStored = snmpPrev.Bool
				} else {
					snmpStored = nil
				}
			} else {
				snmpStored = nil
			}

			tx, err := pool.Begin(ctx)
			if err != nil {
				if log != nil {
					log.Error().Err(err).Str("host", host).Msg("begin tx latency_sweep")
				}
				return
			}
			_, err = tx.Exec(ctx, `
			INSERT INTO device_probe_cache (
				device_id, checked_at, monitoring_mode, ok, reach_ok, latency_ms, method, snmp_ok, detail, ping_fail_streak
			)
			VALUES ($1, now(), $2, $3, $4, $5, $6, $7, ($8::jsonb), $9)
			ON CONFLICT (device_id) DO UPDATE SET
				checked_at = EXCLUDED.checked_at,
				monitoring_mode = EXCLUDED.monitoring_mode,
				reach_ok = EXCLUDED.reach_ok,
				latency_ms = EXCLUDED.latency_ms,
				method = EXCLUDED.method,
				detail = COALESCE(device_probe_cache.detail, '{}'::jsonb) || EXCLUDED.detail,
				ping_fail_streak = EXCLUDED.ping_fail_streak,
				snmp_ok = device_probe_cache.snmp_ok,
				ok = (EXCLUDED.reach_ok AND COALESCE(device_probe_cache.snmp_ok, true))
		`, id, mode, overallOK, reachOK, lat, method, snmpStored, string(dj), streakAfter)
			if err != nil {
				_ = tx.Rollback(ctx)
				if log != nil {
					log.Error().Err(err).Str("host", host).Msg("device_probe_cache latency_sweep")
				}
				return
			}
			_, err = tx.Exec(ctx, `
			INSERT INTO ping_history (device_id, checked_at, ok, latency_ms, method, source, detail)
			VALUES ($1, now(), $2, $3, $4, $5, $6::jsonb)
		`, id, overallOK, lat, method, src, string(dj))
			if err != nil {
				_ = tx.Rollback(ctx)
				if log != nil {
					log.Error().Err(err).Str("host", host).Msg("ping_history latency_sweep")
				}
				return
			}
			if err := tx.Commit(ctx); err != nil {
				if log != nil {
					log.Error().Err(err).Str("host", host).Msg("commit latency_sweep")
				}
			}
		})
	}

	resolvePingUnreachableForDevices(ctx, pool, log, recoveredPing)
	repairMissingPingUnreachableAlerts(ctx, pool, log, cfg.OfflineThreshold)

	_, err = pool.Exec(ctx, `
		UPDATE monitoring_runtime SET
			last_latency_cycle_at = now(),
			last_cycle_at = now(),
			last_cycle_ok_count = $1,
			last_cycle_fail_count = $2,
			updated_at = now()
		WHERE id = 1
	`, okN, failN)
	if log != nil {
		log.Info().Str("cycle", "latency").Str("mode", mode).Int("ok", okN).Int("fail", failN).Msg("ciclo latência")
	}
	return err
}

// UpsertSingleDeviceLatencyProbe aplica uma sonda ao equipamento, actualiza caches e preserva snmp_ok como RunLatencySweep.
func UpsertSingleDeviceLatencyProbe(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, deviceID uuid.UUID, mode string, probe map[string]any, streakAfter int, source string, attemptsMeta map[string]any) error {
	if pool == nil {
		return fmt.Errorf("pool nulo")
	}
	var outerErr error
	WithDeviceProbeRowLock(deviceID, func() {
		var cat, brand, model, description, ip string
		var snmpPrev sql.NullBool
		err := pool.QueryRow(ctx, `
		SELECT COALESCE(TRIM(category), ''), COALESCE(TRIM(brand), ''), COALESCE(TRIM(model), ''), COALESCE(TRIM(description), ''), host(ip)::text
		FROM devices WHERE id=$1
	`, deviceID).Scan(&cat, &brand, &model, &description, &ip)
		if err != nil {
			outerErr = err
			return
		}
		_ = pool.QueryRow(ctx, `SELECT snmp_ok FROM device_probe_cache WHERE device_id=$1`, deviceID).Scan(&snmpPrev)

		reachOK, _ := probe["ok"].(bool)
		method, _ := probe["method"].(string)
		var lat int64
		switch v := probe["latency_ms"].(type) {
		case int64:
			lat = v
		case float64:
			lat = int64(v)
		case int:
			lat = int64(v)
		}

		prevSnap := loadPreviousPingSnapshot(ctx, pool, deviceID)
		host := strings.TrimSpace(ip)

		detail := map[string]any{"reachability": probe, "latency_source": source}
		for k, v := range attemptsMeta {
			detail[k] = v
		}

		if reachOK {
			patchOpenLatencyHighMeta(ctx, pool, deviceID, lat)
		}
		if host != "" {
			if !syncLatencyAlertByGlobalThreshold(ctx, pool, log, deviceID, cat, description, host, reachOK, lat) {
				insertLatencyHighIfNew(ctx, pool, log, deviceID, description, host, prevSnap, lat, reachOK)
				resolveLatencyHighIfCalm(ctx, pool, log, deviceID, reachOK, lat)
			}
		}

		overallOK := compositeProbeOK(mode, reachOK, snmpPrev)
		dj, jerr := json.Marshal(detail)
		if jerr != nil || len(dj) == 0 {
			dj = []byte(`{}`)
		}
		var snmpStored any
		if mode == ModeFull {
			if snmpPrev.Valid {
				snmpStored = snmpPrev.Bool
			} else {
				snmpStored = nil
			}
		}

		_, err = pool.Exec(ctx, `
		INSERT INTO device_probe_cache (
			device_id, checked_at, monitoring_mode, ok, reach_ok, latency_ms, method, snmp_ok, detail, ping_fail_streak
		)
		VALUES ($1::uuid, now(), $2::text, $3, $4, $5, $6, $7, ($8)::jsonb, $9)
		ON CONFLICT (device_id) DO UPDATE SET
			checked_at = EXCLUDED.checked_at,
			monitoring_mode = EXCLUDED.monitoring_mode,
			reach_ok = EXCLUDED.reach_ok,
			latency_ms = EXCLUDED.latency_ms,
			method = EXCLUDED.method,
			detail = COALESCE(device_probe_cache.detail, '{}'::jsonb) || EXCLUDED.detail,
			ping_fail_streak = EXCLUDED.ping_fail_streak,
			snmp_ok = device_probe_cache.snmp_ok,
			ok = (EXCLUDED.reach_ok AND COALESCE(device_probe_cache.snmp_ok, true))
	`, deviceID, mode, overallOK, reachOK, lat, method, snmpStored, string(dj), streakAfter)
		if err != nil {
			outerErr = err
			return
		}

		_, err = pool.Exec(ctx, `
		INSERT INTO ping_history (device_id, checked_at, ok, latency_ms, method, source, detail)
		VALUES ($1::uuid, now(), $2, $3, $4, $5, $6::jsonb)
	`, deviceID, overallOK, lat, method, source, string(dj))
		outerErr = err
	})
	return outerErr
}
