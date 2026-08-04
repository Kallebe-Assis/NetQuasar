package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/netquasar/netquasar/quasar_backend/internal/config"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var embedMigrations embed.FS

const readOnlyConnectHint = `PostgreSQL recusou a ligação porque a sessão é só-leitura (réplica, hot standby ou default_transaction_read_only=on). ` +
	`No Supabase SQL Editor execute: SHOW default_transaction_read_only; SHOW transaction_read_only; SELECT pg_is_in_recovery(); ` +
	`Se estiver on/true: ALTER DATABASE postgres SET default_transaction_read_only = off; ` +
	`e no role (ex. postgres): ALTER ROLE "postgres" SET default_transaction_read_only = off; ` +
	`Depois reconecte / reinicie o backend. Use sempre o endpoint de escrita (Session pooler ou db.<ref>.supabase.co), nunca reader/replica.`

func wrapConnectErr(err error) error {
	if err == nil {
		return nil
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "read only connection") ||
		strings.Contains(msg, "read-only") ||
		strings.Contains(msg, "hot standby") ||
		strings.Contains(msg, "sqlstate 25006") {
		return fmt.Errorf("%s\n(detalhe: %w)", readOnlyConnectHint, err)
	}
	return err
}

// Modo transação Supabase (porta 6543 no host db.*) não suporta prepared statements; pgx deve usar protocolo simples.
// Ver: https://supabase.com/docs/guides/database/connecting-to-postgres
func applySupabaseTxnPoolerCompat(pcfg *pgxpool.Config) {
	if pcfg == nil || pcfg.ConnConfig == nil {
		return
	}
	h := strings.ToLower(strings.TrimSpace(pcfg.ConnConfig.Host))
	if pcfg.ConnConfig.Port == 6543 && config.IsSupabaseDirectDBHost(h) {
		pcfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	}
}

// NewPool cria pool pgx (alto desempenho, conexões multiplexadas).
func NewPool(ctx context.Context, cfg *config.Config) (*pgxpool.Pool, error) {
	pcfg, err := pgxpool.ParseConfig(cfg.PostgresDSN())
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}
	applySupabaseTxnPoolerCompat(pcfg)
	pcfg.MaxConns = 32
	pcfg.MinConns = 2
	pcfg.MaxConnLifetime = time.Hour
	pcfg.MaxConnIdleTime = 10 * time.Minute
	pcfg.HealthCheckPeriod = time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, pcfg)
	if err != nil {
		return nil, fmt.Errorf("pgx pool: %w", wrapConnectErr(err))
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", wrapConnectErr(err))
	}
	return pool, nil
}

// NewEphemeralPool abre pool temporário (ex.: POST /settings/database/test) e deve ser fechado pelo chamador.
func NewEphemeralPool(ctx context.Context, cfg *config.Config) (*pgxpool.Pool, error) {
	return NewPool(ctx, cfg)
}

// Migrate aplica migrações embutidas.
func Migrate(ctx context.Context, cfg *config.Config) error {
	pgxCfg, err := pgxpool.ParseConfig(cfg.PostgresDSN())
	if err != nil {
		return err
	}
	applySupabaseTxnPoolerCompat(pgxCfg)
	var sqlDB *sql.DB = stdlib.OpenDB(*pgxCfg.ConnConfig)
	defer sqlDB.Close()

	if pingErr := sqlDB.PingContext(ctx); pingErr != nil {
		return fmt.Errorf("migrate ping: %w", wrapConnectErr(pingErr))
	}

	goose.SetBaseFS(embedMigrations)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	if err := goose.UpContext(ctx, sqlDB, "migrations"); err != nil {
		return fmt.Errorf("goose up: %w", wrapConnectErr(err))
	}
	return nil
}
