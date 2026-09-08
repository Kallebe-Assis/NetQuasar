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

// TryStartParallelBngLoginWatchCycle dispara o ciclo rápido de presença online/offline dos
// logins PPPoE (só walk de access_login, sem GET de IPv4/IPv6/MAC/VLAN/etc. por índice — ver
// bngcollect.CollectAndSyncOnlineLoginsFast) numa goroutine separada, com cadência própria
// (bng_login_watch_seconds, default 90s) — bem mais frequente que o ciclo completo de sessões
// (TryStartParallelBngSessionsCycle, default 30min) porque este ciclo troca detalhe de sessão
// por velocidade: alimenta só bng_known_logins/bng_login_events (online/offline), nunca
// bng_session_snapshots (a tela "Sessões PPPoE" continua a depender do ciclo completo/botão
// manual para IP/MAC/VLAN/etc.).
func TryStartParallelBngLoginWatchCycle(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, mode string, cfg intervalConfig, opts SweepOpts) bool {
	if pool == nil || mode != ModeFull {
		return false
	}
	var lastRun *time.Time
	if err := pool.QueryRow(ctx, `SELECT last_bng_login_watch_cycle_at FROM monitoring_runtime WHERE id=1`).Scan(&lastRun); err != nil {
		return false
	}
	if !opts.Force && !cycleDue(lastRun, cfg.BngLoginWatchSeconds) {
		return false
	}
	if !TryLockBngLoginWatchCycle() {
		if log != nil {
			log.Debug().Msg("BNG presença online/offline adiada: ciclo anterior ainda em curso")
		}
		return false
	}

	devices, err := loadBngDevicesForCollect(ctx, pool, opts.DeviceID)
	if err != nil || len(devices) == 0 {
		UnlockBngLoginWatchCycle()
		if err == nil {
			_, _ = pool.Exec(ctx, `
				UPDATE monitoring_runtime SET last_bng_login_watch_cycle_at = now(), updated_at = now()
				WHERE id = 1
			`)
		}
		return false
	}

	var defCommunity *string
	_ = pool.QueryRow(ctx, `SELECT snmp_community FROM settings_connection_defaults WHERE id=1`).Scan(&defCommunity)

	go func(devices []pingableDeviceRow, defCommunity *string) {
		defer UnlockBngLoginWatchCycle()
		l := log.With().Str("cycle", "bng_login_watch").Logger()
		setActivity(ctx, pool, "BNG — presença online/offline (ciclo rápido)")
		cycleCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Minute)
		defer cancel()

		// Um walk de logins é leve, mas ainda assim evita martelar vários BNGs de vez.
		conc := 3
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
			n, cErr := bngcollect.CollectAndSyncOnlineLoginsFast(cycleCtx, pool, row.id, strings.TrimSpace(row.ip), comm, 30*time.Second)
			if cErr != nil {
				failN++
				if log != nil {
					log.Debug().Err(cErr).Str("device", row.id.String()).Str("host", strings.TrimSpace(row.ip)).Msg("ciclo rápido de presença PPPoE falhou")
				}
				return
			}
			okN++
			if log != nil {
				log.Debug().Str("device", row.id.String()).Int("logins", n).Msg("ciclo rápido de presença PPPoE concluído")
			}
		})

		if log != nil && (okN > 0 || failN > 0) {
			l.Debug().Int("ok", okN).Int("failed", failN).Int("skipped", skipN).Msg("ciclo de presença PPPoE concluído")
		}
		setActivity(ctx, pool, "")
		_, _ = pool.Exec(context.WithoutCancel(ctx), `
			UPDATE monitoring_runtime SET last_bng_login_watch_cycle_at = now(), updated_at = now()
			WHERE id = 1
		`)
	}(devices, defCommunity)
	return true
}
