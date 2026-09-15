package bootstrap

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// ResumeAfterUpdate fecha o ciclo de uma atualização disparada pela aba "Sobre" → "Versão do
// sistema" (ver internal/api/handlers_system_update.go e o serviço `updater` em
// docker-compose.yml): se este arranque encontrar system_update_state ainda marcado como
// "update_requested"/"updating", é porque o `docker compose up -d --build netquasar` do
// `updater` acabou de matar o processo anterior a meio da operação e este binário É o resultado
// dessa atualização — restaura monitoring_runtime ao estado de antes (monitorworker.Run, que
// arranca depois disto, lê essa linha a cada tick e retoma sozinho) e fecha o estado como
// concluído. Chamado em cmd/netquasar/main.go antes de api.NewServer (que é quem arranca o
// worker) — garante que o worker só começa a correr depois de sabermos qual deve ser o seu modo.
func ResumeAfterUpdate(ctx context.Context, pool *pgxpool.Pool) error {
	var status string
	var requestedAt *time.Time
	var prevRunning *bool
	var prevMode *string
	err := pool.QueryRow(ctx, `
		SELECT status, requested_at, previous_is_running, previous_monitoring_mode
		FROM system_update_state WHERE id = 1
	`).Scan(&status, &requestedAt, &prevRunning, &prevMode)
	if err != nil {
		// Tabela pode não existir ainda (migração não aplicada nesta ligação) ou faltar a linha —
		// não é fatal para o arranque, só significa "nada a retomar".
		return nil //nolint:nilerr
	}

	switch status {
	case "update_requested", "updating":
		if requestedAt != nil && time.Since(*requestedAt) > 10*time.Minute {
			// Reinício não relacionado a meio de um pedido muito antigo (ex.: o `updater` nunca
			// chegou a processá-lo) — não assumir que foi este arranque que o completou.
			_, _ = pool.Exec(ctx, `
				UPDATE system_update_state SET status = 'failed',
					error_message = 'Interrompido por reinício do servidor antes de concluir.',
					updated_at = now()
				WHERE id = 1
			`)
			return nil
		}
		if prevRunning != nil && *prevRunning {
			mode := "simple_ping"
			if prevMode != nil && *prevMode != "" && *prevMode != "off" {
				mode = *prevMode
			}
			if _, err := pool.Exec(ctx, `
				UPDATE monitoring_runtime SET is_running = true, monitoring_mode = $1, updated_at = now() WHERE id = 1
			`, mode); err != nil {
				return err
			}
			log.Info().Str("monitoring_mode", mode).Msg("atualização do sistema: monitoramento restaurado")
		}
		if _, err := pool.Exec(ctx, `
			UPDATE system_update_state SET status = 'completed', completed_at = now(), updated_at = now() WHERE id = 1
		`); err != nil {
			return err
		}
		log.Info().Msg("atualização do sistema concluída")
	case "check_requested", "checking":
		if requestedAt != nil && time.Since(*requestedAt) > 10*time.Minute {
			_, _ = pool.Exec(ctx, `
				UPDATE system_update_state SET status = 'failed',
					error_message = 'Interrompido por reinício do servidor antes de concluir.',
					updated_at = now()
				WHERE id = 1
			`)
		}
	}
	return nil
}
