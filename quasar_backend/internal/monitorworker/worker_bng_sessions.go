package monitorworker

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"github.com/netquasar/netquasar/quasar_backend/internal/bngcollect"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpdevicelock"
)

// TryStartParallelBngSessionsCycle dispara a coleta periódica das sessões PPPoE detalhadas
// (login/IP/MAC/uptime por assinante) numa goroutine separada, com cadência própria
// (bng_sessions_parallel_seconds, default 1800s = 30min) — bem mais espaçada que o ciclo leve
// de totais BNG (TryStartParallelBngCycle), porque um walk completo de sessões é pesado.
//
// Antes desta função, bng_session_snapshots só era populada pelo botão manual "Coletar
// sessões" da tela de BNG — nada automático corria (RunBngSweep só recolhe totais). Usa
// MergeSessionSnapshot em vez de StoreSessionSnapshot: uma leitura parcial (walk truncado,
// timeout a meio dos detalhes) actualiza só o que foi visto, sem apagar nem alterar os
// logins que não vieram nesta volta. Ver SessionsCollectionComplete para a mesma cautela
// aplicada ao inventário online/offline (SyncKnownLogins).
func TryStartParallelBngSessionsCycle(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, mode string, cfg intervalConfig, opts SweepOpts) bool {
	if pool == nil || mode != ModeFull {
		return false
	}
	var lastRun *time.Time
	if err := pool.QueryRow(ctx, `SELECT last_bng_sessions_parallel_cycle_at FROM monitoring_runtime WHERE id=1`).Scan(&lastRun); err != nil {
		return false
	}
	if !opts.Force && !cycleDue(lastRun, cfg.BngSessionsParallelSeconds) {
		return false
	}
	if !TryLockBngSessionsCycle() {
		if log != nil {
			log.Debug().Msg("BNG sessões adiado: ciclo anterior ainda em curso")
		}
		return false
	}

	devices, err := loadBngDevicesForCollect(ctx, pool, opts.DeviceID)
	if err != nil || len(devices) == 0 {
		UnlockBngSessionsCycle()
		if err == nil {
			_, _ = pool.Exec(ctx, `
				UPDATE monitoring_runtime SET last_bng_sessions_parallel_cycle_at = now(), updated_at = now()
				WHERE id = 1
			`)
		}
		return false
	}

	var defCommunity *string
	_ = pool.QueryRow(ctx, `SELECT snmp_community FROM settings_connection_defaults WHERE id=1`).Scan(&defCommunity)

	go func(devices []pingableDeviceRow, defCommunity *string) {
		defer UnlockBngSessionsCycle()
		l := log.With().Str("cycle", "bng_sessions_parallel").Logger()
		setActivity(ctx, pool, "BNG — sessões PPPoE detalhadas (ciclo periódico)")
		cycleCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Minute)
		defer cancel()

		// Um walk completo por device já é pesado — no máximo 2 em paralelo, mesmo que a
		// concorrência geral do sweep seja maior, para não sobrecarregar vários BNGs de vez.
		conc := 2
		if len(devices) < conc {
			conc = len(devices)
		}
		if conc < 1 {
			conc = 1
		}

		okN, failN, skipN := 0, 0, 0
		forEachLimited(cycleCtx, len(devices), conc, func(i int) {
			row := devices[i]
			comm := resolveSNMPCommunity(row, defCommunity)
			if comm == "" {
				skipN++
				return
			}
			unlock := snmpdevicelock.Acquire(row.id)
			defer unlock()
			n, synced, cErr := bngcollect.CollectAndMergeSessionsPeriodic(cycleCtx, pool, row.id, strings.TrimSpace(row.ip), comm, 5*time.Minute)
			if cErr != nil {
				failN++
				if log != nil {
					log.Warn().Err(cErr).Str("device", row.id.String()).Str("host", strings.TrimSpace(row.ip)).Msg("coleta periódica de sessões PPPoE falhou")
				}
				return
			}
			okN++
			if log != nil {
				log.Debug().Str("device", row.id.String()).Int("sessions", n).Bool("synced_online_offline", synced).Msg("coleta periódica de sessões PPPoE concluída")
			}
		})

		if log != nil && (okN > 0 || failN > 0) {
			l.Info().Int("ok", okN).Int("failed", failN).Int("skipped", skipN).Msg("ciclo de sessões PPPoE concluído")
		}
		setActivity(ctx, pool, "")
		_, _ = pool.Exec(context.WithoutCancel(ctx), `
			UPDATE monitoring_runtime SET last_bng_sessions_parallel_cycle_at = now(), updated_at = now()
			WHERE id = 1
		`)
	}(devices, defCommunity)
	return true
}
