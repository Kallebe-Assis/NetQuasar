package monitorworker

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"github.com/netquasar/netquasar/quasar_backend/internal/mikrotikcollect"
	"github.com/netquasar/netquasar/quasar_backend/internal/panicguard"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpdevicelock"
	"github.com/netquasar/netquasar/quasar_backend/internal/telemetryengine"
)

// TryStartParallelMikrotikFullCycle dispara a coleta MikroTik COMPLETA (disco/óptica/telnet/
// PPPoE) numa goroutine separada, com cadência própria (mikrotik_full_parallel_seconds, default
// 900s = 15min) — bem mais espaçada que o ciclo rápido de saúde (telemetry_seconds,
// TryStartParallelTelemetryCycle), que só coleta CPU/memória/temperatura/uptime.
//
// Antes deste ciclo, telemetryengine.CollectAndStore (o único caminho que faz walk de disco/
// óptica e sincroniza PPPoE via telnet — ver mikrotikcollect.CollectAndStore) só corria pelo
// botão manual "Coletar agora" (handlers_ping_telemetry.go) ou pela tela de sessões BNG. Como
// telemetry_samples é sempre INSERT (nunca upsert — necessário para os gráficos de histórico) e
// a leitura de telemetria "atual" lia só a última linha, esses campos apareciam uma vez após uma
// coleta manual e desapareciam assim que o próximo tick de saúde gravasse uma amostra mais magra
// — mesmo bug relatado como "os dados são coletados, fica 1 minutinho e depois todos somem".
// telemetryengine.LoadLatestMergedTelemetry cobre o lado da leitura (preenche campos ausentes a
// partir de amostras recentes); este ciclo cobre o lado da escrita, garantindo que a coleta
// completa de facto se repete sozinha — sem depender de alguém clicar "Coletar agora" a cada
// poucos minutos.
//
// Cobre TODOS os MikroTik elegíveis para telemetria, independente de bng_enabled: o ciclo rápido
// de saúde ignora equipamentos BNG de propósito ("coleta BNG no passo dedicado do pipeline" —
// ver isBngDevice em snmp_subsweeps.go), e o ciclo BNG dedicado (worker_bng.go) é orientado a
// hwAccessTable (Huawei) — não existe em hardware MikroTik. Sem este ciclo, um MikroTik com
// bng_enabled=true nunca tinha as sessões PPPoE por telnet sincronizadas automaticamente.
func TryStartParallelMikrotikFullCycle(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, mode string, cfg intervalConfig, opts SweepOpts) bool {
	if pool == nil || mode != ModeFull {
		return false
	}
	var lastRun *time.Time
	if err := pool.QueryRow(ctx, `SELECT last_mikrotik_full_parallel_cycle_at FROM monitoring_runtime WHERE id=1`).Scan(&lastRun); err != nil {
		return false
	}
	if !opts.Force && !cycleDue(lastRun, cfg.MikrotikFullParallelSeconds) {
		return false
	}
	if !TryLockMikrotikFullCycle() {
		if log != nil {
			log.Debug().Msg("MikroTik completo adiado: ciclo anterior ainda em curso")
		}
		return false
	}

	all, err := loadTelemetryDevices(ctx, pool, opts.DeviceID)
	if err != nil {
		UnlockMikrotikFullCycle()
		return false
	}
	devices := make([]pingableDeviceRow, 0, len(all))
	for _, row := range all {
		if mikrotikcollect.IsMikrotikDevice(row.category, row.brand, row.model, row.description) {
			devices = append(devices, row)
		}
	}
	if len(devices) == 0 {
		UnlockMikrotikFullCycle()
		_, _ = pool.Exec(ctx, `
			UPDATE monitoring_runtime SET last_mikrotik_full_parallel_cycle_at = now(), updated_at = now()
			WHERE id = 1
		`)
		return false
	}

	var defCommunity *string
	_ = pool.QueryRow(ctx, `SELECT snmp_community FROM settings_connection_defaults WHERE id=1`).Scan(&defCommunity)

	go func(devices []pingableDeviceRow, defCommunity *string) {
		defer panicguard.Recover("monitor_worker_mikrotik_full")
		defer UnlockMikrotikFullCycle()
		l := log.With().Str("cycle", "mikrotik_full_parallel").Logger()
		setActivity(ctx, pool, "MikroTik — coleta completa (disco/óptica/PPPoE)")
		cycleCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Minute)
		defer cancel()

		conc := sweepConcurrency()
		if conc > 4 {
			// Walks ópticos + telnet são pesados por device — teto menor que o ciclo de saúde.
			conc = 4
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
			unlock, gotLock := snmpdevicelock.TryAcquire(cycleCtx, row.id, snmpLockWaitMax)
			if !gotLock {
				skipN++
				return
			}
			defer unlock()
			devCtx, devCancel := context.WithTimeout(cycleCtx, 90*time.Second)
			col, cErr := telemetryengine.CollectAndStore(devCtx, pool, row.id, strings.TrimSpace(row.ip), comm)
			devCancel()
			if cErr != nil {
				failN++
				if log != nil {
					log.Warn().Err(cErr).Str("device", row.id.String()).Str("host", strings.TrimSpace(row.ip)).Msg("coleta MikroTik completa (periódica) falhou")
				}
				return
			}
			okN++
			_ = col
		})

		if log != nil && (okN > 0 || failN > 0) {
			l.Info().Int("ok", okN).Int("failed", failN).Int("skipped", skipN).Msg("ciclo MikroTik completo concluído")
		}
		setActivity(ctx, pool, "")
		_, _ = pool.Exec(context.WithoutCancel(ctx), `
			UPDATE monitoring_runtime SET last_mikrotik_full_parallel_cycle_at = now(), updated_at = now()
			WHERE id = 1
		`)
	}(devices, defCommunity)
	return true
}
