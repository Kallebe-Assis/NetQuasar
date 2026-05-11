package bootstrap

import (
	"context"
	"fmt"

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
	userHash, err := bcrypt.GenerateFromPassword([]byte("viewer"), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO users (display_name, email, phone, password_hash, role) VALUES
			('Administrador', 'admin@admin.com', '11999998888', $1, 'admin'),
			('Visitante', 'viewer@netquasar.local', '21988887777', $2, 'viewer')
	`, string(adminHash), string(userHash))
	if err != nil {
		return fmt.Errorf("seed users: %w", err)
	}
	return nil
}

// EnsureDatabaseMetaRow garante linha id=1 e espelha parâmetros não sensíveis do processo (senha não é gravada aqui).
func EnsureDatabaseMetaRow(ctx context.Context, pool *pgxpool.Pool, cfg *config.Config) error {
	if cfg.DatabaseURL != "" {
		_, err := pool.Exec(ctx, `
			INSERT INTO settings_database_meta (id, host, port, db_user, db_name, ssl_mode, updated_at)
			VALUES (1, NULL, NULL, NULL, NULL, 'disable', now())
			ON CONFLICT (id) DO NOTHING
		`)
		return err
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
		return fmt.Errorf("ensure database meta: %w", err)
	}
	return nil
}
