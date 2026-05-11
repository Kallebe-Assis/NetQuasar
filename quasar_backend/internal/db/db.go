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
		return nil, fmt.Errorf("pgx pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
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
		return fmt.Errorf("migrate ping: %w", pingErr)
	}

	goose.SetBaseFS(embedMigrations)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	if err := goose.UpContext(ctx, sqlDB, "migrations"); err != nil {
		return fmt.Errorf("goose up: %w", err)
	}
	return nil
}
