package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/config"
	"golang.org/x/crypto/bcrypt"
)

// EnsureDefaultUsers cria admin + visitante (viewer) se tabela vazia (senhas bcrypt).
func EnsureDefaultUsers(ctx context.Context, pool *pgxpool.Pool) error {
	var n int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
		return fmt.Errorf("count users: %w", err)
	}
	if n > 0 {
		return nil
	}
	adminHash, err := bcrypt.GenerateFromPassword([]byte("admin"), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	viewerHash, err := bcrypt.GenerateFromPassword([]byte("viewer"), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO users (display_name, email, phone, password_hash, role) VALUES
			('Administrador', 'admin@admin.com', '11999998888', $1, 'admin'),
			('Visitante', 'viewer@netquasar.local', '21988887777', $2, 'viewer')
	`, string(adminHash), string(viewerHash))
	if err != nil {
		return fmt.Errorf("seed users: %w", wrapReadOnlyDB(err))
	}
	return nil
}

// EnsureDatabaseMetaRow garante linha id=1 e espelha parâmetros não sensíveis do processo (senha não é gravada aqui).
func EnsureDatabaseMetaRow(ctx context.Context, pool *pgxpool.Pool, cfg *config.Config) error {
	var exists bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM settings_database_meta WHERE id=1)`).Scan(&exists); err != nil {
		return fmt.Errorf("check database meta: %w", err)
	}
	if exists && cfg.DatabaseURL != "" {
		// Já há linha meta e a DSN vem de URL — nada a gravar.
		return nil
	}

	if cfg.DatabaseURL != "" {
		_, err := pool.Exec(ctx, `
			INSERT INTO settings_database_meta (id, host, port, db_user, db_name, ssl_mode, updated_at)
			VALUES (1, NULL, NULL, NULL, NULL, 'disable', now())
			ON CONFLICT (id) DO NOTHING
		`)
		if err != nil {
			return wrapReadOnlyDB(err)
		}
		return nil
	}
	_, err := pool.Exec(ctx, `
		INSERT INTO settings_database_meta (id, host, port, db_user, db_name, ssl_mode, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, now())
		ON CONFLICT (id) DO UPDATE SET
			host = COALESCE(EXCLUDED.host, settings_database_meta.host),
			port = COALESCE(EXCLUDED.port, settings_database_meta.port),
			db_user = COALESCE(EXCLUDED.db_user, settings_database_meta.db_user),
			db_name = COALESCE(EXCLUDED.db_name, settings_database_meta.db_name),
			ssl_mode = COALESCE(NULLIF(EXCLUDED.ssl_mode, ''), settings_database_meta.ssl_mode),
			updated_at = now()
	`, 1, cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBName, cfg.DBSSLMode)
	if err != nil {
		return fmt.Errorf("ensure database meta: %w", wrapReadOnlyDB(err))
	}
	return nil
}

// AssertWritablePostgres confirma que a sessão aceita escrita (não é réplica / hot standby).
func AssertWritablePostgres(ctx context.Context, pool *pgxpool.Pool) error {
	var readOnly string
	if err := pool.QueryRow(ctx, `SHOW transaction_read_only`).Scan(&readOnly); err != nil {
		return fmt.Errorf("verificar transaction_read_only: %w", err)
	}
	if strings.EqualFold(strings.TrimSpace(readOnly), "on") {
		return errors.New(readOnlyDBHint)
	}
	var inRecovery bool
	if err := pool.QueryRow(ctx, `SELECT pg_is_in_recovery()`).Scan(&inRecovery); err != nil {
		return fmt.Errorf("verificar pg_is_in_recovery: %w", err)
	}
	if inRecovery {
		return errors.New(readOnlyDBHint)
	}
	return nil
}

const readOnlyDBHint = `PostgreSQL está em modo só-leitura (réplica / hot standby / default_transaction_read_only). ` +
	`O NetQuasar precisa do primary com escrita. Verifique NETQUASAR_DATABASE_URL: ` +
	`use o endpoint de escrita (não "reader"/"replica"), no Supabase prefira Session pooler ou db.<ref>.supabase.co (primary), ` +
	`e confirme com: SHOW transaction_read_only; SELECT pg_is_in_recovery();`

func wrapReadOnlyDB(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "25006" {
		return fmt.Errorf("%s (detalhe: %w)", readOnlyDBHint, err)
	}
	if strings.Contains(strings.ToLower(err.Error()), "read-only") {
		return fmt.Errorf("%s (detalhe: %w)", readOnlyDBHint, err)
	}
	return err
}
