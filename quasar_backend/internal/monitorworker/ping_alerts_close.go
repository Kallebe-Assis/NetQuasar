package monitorworker

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

// ClosePingUnreachableOnMonitoringDisabled encerra alertas ping_unreachable sem notificar «equipamento online».
func ClosePingUnreachableOnMonitoringDisabled(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, deviceID uuid.UUID) {
	if pool == nil || deviceID == uuid.Nil {
		return
	}
	tag, err := pool.Exec(ctx, `
		UPDATE alert_instances SET
			closed_at = now(),
			meta = COALESCE(meta, '{}'::jsonb) || '{"resolved":"ping_monitoring_disabled","source":"device_patch"}'::jsonb
		WHERE device_id = $1::uuid
		  AND alert_type = $2
		  AND closed_at IS NULL
	`, deviceID, alertTypePingUnreachable)
	if err != nil && log != nil {
		log.Warn().Err(err).Str("device", deviceID.String()).Msg("fechar ping_unreachable ao desativar monitoramento")
		return
	}
	if tag.RowsAffected() > 0 && log != nil {
		log.Info().Str("device", deviceID.String()).Int64("closed", tag.RowsAffected()).Msg("alertas ping encerrados (monitoramento desativado, sem notificação online)")
	}
}
