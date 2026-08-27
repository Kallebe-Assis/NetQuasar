package monitorworker

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"github.com/netquasar/netquasar/quasar_backend/internal/snmpdevicelock"
)

// TryStartParallelBngInterfaceCycle dispara o snapshot periódico de interfaces (IF-MIB —
// ifDescr/ifName/ifHCInOctets/ifHCOutOctets) dos equipamentos BNG numa goroutine separada.
//
// Antes desta função, interface_snapshots só recebia dados de um BNG quando alguém clicava
// "Atualizar" manualmente na tela de equipamento — o pipeline periódico configurado
// (StepKindInterfacesMikrotik/Switch/OLT em pipeline_runner.go) nunca incluía a categoria BNG.
// Sem isto, não há histórico de tráfego para os uplinks de operadora (K2/FORTE) mostrados na
// aba Relatório do BNG. Reaproveita a mesma cadência de interface_snapshot_seconds já usada
// para Mikrotik/switch/OLT — é o mesmo tipo de operação (um walk IF-MIB), só que agora também
// para BNG.
func TryStartParallelBngInterfaceCycle(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, mode string, cfg intervalConfig, opts SweepOpts) bool {
	if pool == nil || mode != ModeFull {
		return false
	}
	var lastRun *time.Time
	if err := pool.QueryRow(ctx, `SELECT last_bng_interfaces_parallel_cycle_at FROM monitoring_runtime WHERE id=1`).Scan(&lastRun); err != nil {
		return false
	}
	if !opts.Force && !cycleDue(lastRun, cfg.IfaceSeconds) {
		return false
	}
	if !TryLockBngInterfacesCycle() {
		if log != nil {
			log.Debug().Msg("BNG interfaces adiado: ciclo anterior ainda em curso")
		}
		return false
	}

	devices, err := loadBngDevicesForCollect(ctx, pool, opts.DeviceID)
	if err != nil || len(devices) == 0 {
		UnlockBngInterfacesCycle()
		if err == nil {
			_, _ = pool.Exec(ctx, `
				UPDATE monitoring_runtime SET last_bng_interfaces_parallel_cycle_at = now(), updated_at = now()
				WHERE id = 1
			`)
		}
		return false
	}

	var defCommunity *string
	_ = pool.QueryRow(ctx, `SELECT snmp_community FROM settings_connection_defaults WHERE id=1`).Scan(&defCommunity)

	go func(devices []pingableDeviceRow, defCommunity *string) {
		defer UnlockBngInterfacesCycle()
		l := log.With().Str("cycle", "bng_interfaces_parallel").Logger()
		setActivity(ctx, pool, "BNG — snapshot de interfaces (ciclo periódico)")
		cycleCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 6*time.Minute)
		defer cancel()

		conc := sweepConcurrency()
		if conc < 1 {
			conc = 1
		}
		okN, skipN := 0, 0
		forEachLimited(cycleCtx, len(devices), conc, func(i int) {
			row := devices[i]
			comm := resolveSNMPCommunity(row, defCommunity)
			if comm == "" {
				skipN++
				return
			}
			unlock := snmpdevicelock.Acquire(row.id)
			defer unlock()
			sctx, scancel := context.WithTimeout(cycleCtx, cfg.interfaceTimeout(false, false))
			defer scancel()
			CollectInterfaceSnapshotWorker(sctx, pool, &l, row.id, strings.TrimSpace(row.ip), comm,
				row.category, row.brand, row.model, row.description)
			okN++
		})

		if log != nil && (okN > 0 || skipN > 0) {
			l.Info().Int("ok", okN).Int("skipped", skipN).Msg("ciclo de interfaces BNG concluído")
		}
		setActivity(ctx, pool, "")
		_, _ = pool.Exec(context.WithoutCancel(ctx), `
			UPDATE monitoring_runtime SET last_bng_interfaces_parallel_cycle_at = now(), updated_at = now()
			WHERE id = 1
		`)
	}(devices, defCommunity)
	return true
}
