// Package monitorworker executa ciclos de sondagem enquanto monitoring_runtime.is_running = true,
// independentemente de conexões HTTP do front (troca de tela não interrompe). Parar só via POST /monitoring/stop.
//
// O worker dispara ciclos independentes: ping (paralelo), telemetria SNMP (paralelo), pipeline de interfaces
// (MikroTik + OLT), e ONU/PON. Ao iniciar modo full, o HTTP arranca um bootstrap sequencial com todos os passos forçados.
// Escritas em device_probe_cache por equipamento são serializadas com WithDeviceProbeRowLock; SNMP por snmpdevicelock.
package monitorworker

import (
	"context"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

const (
	ModeOff        = "off"
	ModeSimplePing = "simple_ping"
	ModeFull       = "full"
)

// Run loop até ctx cancelado (shutdown do processo). Estado on/off vem exclusivamente do Postgres.
// dbHolder aponta para o mesmo atomic do servidor HTTP para que troca de pool (settings/database) seja vista aqui.
func Run(ctx context.Context, dbHolder *atomic.Pointer[pgxpool.Pool], log zerolog.Logger) {
	l := log.With().Str("component", "monitor_worker").Logger()
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			l.Info().Msg("monitor worker encerrado (shutdown)")
			return
		case <-ticker.C:
			pool := dbHolder.Load()
			if pool == nil {
				continue
			}
			if err := tick(ctx, pool, &l); err != nil {
				l.Debug().Err(err).Msg("monitor tick")
			}
		}
	}
}

func cycleDue(last *time.Time, sec int) bool {
	sec = clampInt(sec, 30, 86400)
	if last == nil || last.IsZero() {
		return true
	}
	return time.Since(*last) >= time.Duration(sec)*time.Second
}

func tick(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger) error {
	var running bool
	var mode string
	err := pool.QueryRow(ctx, `
		SELECT is_running,
			COALESCE(NULLIF(TRIM(monitoring_mode), ''), 'off')
		FROM monitoring_runtime WHERE id = 1
	`).Scan(&running, &mode)
	if err != nil {
		return err
	}
	if !running || mode == ModeOff || mode == "" {
		return nil
	}
	if mode != ModeSimplePing && mode != ModeFull {
		return nil
	}

	cfg, err := loadClampMonitoringIntervals(ctx, pool)
	if err != nil {
		return err
	}
	var lastLat, lastTel, lastIf, lastOlt *time.Time
	if err := pool.QueryRow(ctx, `
		SELECT last_latency_cycle_at, last_telemetry_cycle_at, last_interface_snapshot_cycle_at,
			last_olt_if_derived_cycle_at
		FROM monitoring_runtime WHERE id=1
	`).Scan(&lastLat, &lastTel, &lastIf, &lastOlt); err != nil {
		return err
	}

	latencyDue := cycleDue(lastLat, cfg.PingSeconds)
	if latencyDue && TryLockLatencyCycle() {
		go func(mode string, log *zerolog.Logger) {
			defer UnlockLatencyCycle()
			l := log.With().Str("component", "monitor_worker").Str("cycle", "latency").Logger()
			if err := RunLatencySweep(ctx, pool, &l, mode, SweepOpts{Source: "worker"}); err != nil {
				l.Warn().Err(err).Msg("ciclo latência")
			}
		}(mode, log)
	}

	if mode == ModeFull {
		oltDue := cycleDue(lastOlt, cfg.OltDerivedSeconds)
		if oltDue && TryLockOltPonCycle() {
			go func(log *zerolog.Logger) {
				defer UnlockOltPonCycle()
				l := log.With().Str("cycle", "olt_pon").Logger()
				if err := RunOltIfDerivedSweep(ctx, pool, &l, mode, SweepOpts{Source: "worker"}); err != nil {
					l.Warn().Err(err).Msg("ciclo ONU/PON")
				}
			}(log)
		} else if oltDue && log != nil {
			log.Warn().Msg("ciclo ONU/PON adiado: execução anterior ainda em curso")
		}

		telDue := cycleDue(lastTel, cfg.TelemetrySeconds)
		if telDue {
			if TryLockTelemetryCycle() {
				go func(log *zerolog.Logger) {
					defer UnlockTelemetryCycle()
					l := log.With().Str("component", "monitor_worker").Str("cycle", "telemetry").Logger()
					if err := RunTelemetrySweep(ctx, pool, &l, mode, SweepOpts{Source: "worker"}); err != nil {
						l.Warn().Err(err).Msg("ciclo telemetria")
					}
				}(log)
			} else if log != nil {
				log.Warn().Msg("ciclo telemetria adiado: execução anterior ainda em curso")
			}
		}

		ifDue := cycleDue(lastIf, cfg.IfaceSeconds)
		if ifDue {
			if TryLockMonitoringPipeline() {
				go func(log *zerolog.Logger) {
					defer UnlockMonitoringPipeline()
					l := log.With().Str("component", "monitor_worker").Str("cycle", "interfaces").Logger()
					RunWorkerInterfaceSteps(ctx, pool, &l, mode, false)
				}(log)
			} else if log != nil {
				log.Warn().Msg("ciclo interfaces adiado: pipeline SNMP ainda em execução")
			}
		}
	}

	return nil
}

// monitoringDeviceLabel devolve a descrição do equipamento ou, em falta, o IP/host.
func monitoringDeviceLabel(description, host string) string {
	d := strings.TrimSpace(description)
	if d != "" {
		return d
	}
	return strings.TrimSpace(host)
}

// NudgeMonitoringRuntimeRefresh sinaliza ao frontend que novas leituras foram gravadas (telemetria, OLT, etc.).
func NudgeMonitoringRuntimeRefresh(ctx context.Context, pool *pgxpool.Pool) {
	if pool == nil {
		return
	}
	_, _ = pool.Exec(ctx, `
		UPDATE monitoring_runtime
		SET updated_at = now(), activity_updated_at = now()
		WHERE id = 1
	`)
}

func setActivity(ctx context.Context, pool *pgxpool.Pool, activity string) {
	if pool == nil {
		return
	}
	activity = strings.TrimSpace(activity)
	_, _ = pool.Exec(ctx, `
		UPDATE monitoring_runtime
		SET last_activity = CASE
				WHEN NULLIF($1, '') IS NULL AND current_activity IS NOT NULL THEN current_activity
				ELSE last_activity
			END,
			last_activity_finished_at = CASE
				WHEN NULLIF($1, '') IS NULL AND current_activity IS NOT NULL THEN now()
				ELSE last_activity_finished_at
			END,
			activity_started_at = CASE
				WHEN NULLIF($1, '') IS NULL THEN NULL
				WHEN current_activity IS DISTINCT FROM NULLIF($1, '') THEN now()
				ELSE activity_started_at
			END,
			current_activity = NULLIF($1, ''),
			activity_updated_at = CASE WHEN NULLIF($1, '') IS NULL THEN NULL ELSE now() END,
			updated_at = now()
		WHERE id = 1
	`, activity)
}
