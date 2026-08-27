package monitorworker

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

// TryRunHistoryRetention purga linhas antigas de ping_history, telemetry_samples e
// interface_snapshots quando history_retention_days > 0 (monitoring_intervals) e a
// última purga (monitoring_runtime.last_retention_run_at) já tem mais de 24h.
//
// Antes desta função, a única forma de limpar estas tabelas era manual, em
// Configurações -> Base de dados. Sem purga automática, elas crescem indefinidamente
// e tendem a degradar consultas de relatório com o tempo. Ver
// DIAGNOSTICO-PERFORMANCE-ARQUITETURA.md (achado "retenção automática").
//
// Corre no máximo uma vez por dia, em lotes (para não segurar uma transação longa
// nem competir por conexões do pool com o resto do worker), e nunca é chamada para
// o banco Supabase de produção sem o operador ter definido explicitamente
// history_retention_days > 0 (o default de coluna é 90, mas é sempre uma decisão
// explícita do operador manter ou desligar via Configurações/valor na BD).
func TryRunHistoryRetention(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, cfg intervalConfig) {
	if pool == nil || cfg.HistoryRetentionDays <= 0 {
		return
	}
	var last *time.Time
	if err := pool.QueryRow(ctx, `SELECT last_retention_run_at FROM monitoring_runtime WHERE id=1`).Scan(&last); err != nil {
		return
	}
	if last != nil && time.Since(*last) < 24*time.Hour {
		return
	}
	if !TryLockRetentionCycle() {
		return
	}
	go func(days int) {
		defer UnlockRetentionCycle()
		l := zerolog.Nop()
		if log != nil {
			l = log.With().Str("component", "history_retention").Logger()
		}
		rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Minute)
		defer cancel()
		runHistoryRetentionBatches(rctx, pool, &l, days)
		_, _ = pool.Exec(rctx, `UPDATE monitoring_runtime SET last_retention_run_at = now(), updated_at = now() WHERE id=1`)
	}(cfg.HistoryRetentionDays)
}

// runHistoryRetentionBatches apaga em lotes de até 10000 linhas por tabela/iteração,
// com uma pequena pausa entre lotes, para não segurar locks longos nem saturar o pool
// enquanto o resto do sistema (API, worker) continua a usá-lo.
func runHistoryRetentionBatches(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, retentionDays int) {
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	tables := []struct {
		name, column string
	}{
		{"ping_history", "checked_at"},
		{"telemetry_samples", "collected_at"},
		{"interface_snapshots", "collected_at"},
	}
	for _, t := range tables {
		totalDeleted := int64(0)
		for {
			if ctx.Err() != nil {
				return
			}
			tag, err := pool.Exec(ctx, `
				DELETE FROM `+t.name+`
				WHERE ctid IN (
					SELECT ctid FROM `+t.name+`
					WHERE `+t.column+` < $1
					LIMIT 10000
				)
			`, cutoff)
			if err != nil {
				if log != nil {
					log.Warn().Err(err).Str("table", t.name).Msg("retenção de histórico: falha ao apagar lote")
				}
				break
			}
			n := tag.RowsAffected()
			totalDeleted += n
			if n < 10000 {
				break
			}
			time.Sleep(200 * time.Millisecond)
		}
		if totalDeleted > 0 && log != nil {
			log.Info().Str("table", t.name).Int64("deleted", totalDeleted).
				Time("cutoff", cutoff).Msg("retenção de histórico: linhas antigas removidas")
		}
	}
}
