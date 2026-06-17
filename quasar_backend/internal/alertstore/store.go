package alertstore

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertignore"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertnotify"
	"github.com/rs/zerolog"
)

// OpenSpec dados para criar ou actualizar um alerta aberto.
type OpenSpec struct {
	DeviceID   uuid.UUID
	Severity   string
	AlertType  string
	Message    string
	IP         string
	DeviceName string
	Meta       map[string]any
	Match      Match
}

// NotifyCreate envia Telegram apenas quando o alerta é criado (não em actualizações).
type NotifyCreate struct {
	Log      *zerolog.Logger
	Level    string
	Headline string
}

// OpenResult resultado de OpenOrUpdate.
type OpenResult struct {
	Created bool
	ID      uuid.UUID
}

// OpenOrUpdate insere alerta novo ou actualiza severity/message/meta se já existir (mesmo Match).
func OpenOrUpdate(ctx context.Context, pool *pgxpool.Pool, spec OpenSpec, notify *NotifyCreate) (OpenResult, error) {
	var out OpenResult
	if pool == nil {
		return out, fmt.Errorf("alertstore: pool nulo")
	}
	if spec.Meta == nil {
		spec.Meta = map[string]any{}
	}
	muteKey := alertignore.MetaKeyFromAlert(spec.AlertType, spec.Meta)
	if spec.Match.Kind == MatchMetaKey && spec.Match.MetaKey != "" {
		muteKey = spec.Match.MetaKey
	}
	if alertignore.IsMuted(ctx, pool, spec.DeviceID, spec.AlertType, muteKey) {
		return out, nil
	}

	metaRaw, err := json.Marshal(spec.Meta)
	if err != nil {
		metaRaw = []byte("{}")
	}

	insertArgs := []any{
		spec.DeviceID, spec.Severity, spec.AlertType, spec.Message,
		spec.IP, spec.DeviceName, metaRaw,
	}
	keyParam := len(insertArgs) + 1
	insertArgs = spec.Match.appendKeyArg(insertArgs)

	insertQ := fmt.Sprintf(`
		INSERT INTO alert_instances (device_id, severity, alert_type, message, ip, device_name, meta)
		SELECT $1::uuid, $2::text, $3::text, $4, NULLIF(trim($5), ''), NULLIF(trim($6), ''),
			COALESCE($7::jsonb, '{}'::jsonb)
		WHERE NOT EXISTS (
			SELECT 1 FROM alert_instances ai
			WHERE ai.device_id = $1::uuid
			  AND ai.alert_type = $3::text
			  AND ai.closed_at IS NULL%s
		)
		RETURNING id
	`, spec.Match.notExistsClause(keyParam))

	err = pool.QueryRow(ctx, insertQ, insertArgs...).Scan(&out.ID)
	if err == nil {
		out.Created = true
		if notify != nil && notify.Log != nil {
			alertnotify.SendMonitoringTelegramAndPatchMeta(
				ctx, pool, notify.Log, out.ID, notify.Level, notify.Headline, spec.Message,
			)
		}
		return out, nil
	}
	if err != pgx.ErrNoRows {
		return out, err
	}

	updateArgs := []any{spec.DeviceID, spec.AlertType, spec.Severity, spec.Message, metaRaw}
	whereParam := len(updateArgs) + 1
	updateArgs = spec.Match.appendKeyArg(updateArgs)

	updateQ := fmt.Sprintf(`
		UPDATE alert_instances SET
			severity = $3::text,
			message = $4,
			meta = COALESCE(meta, '{}'::jsonb) || $5::jsonb
		WHERE device_id = $1::uuid
		  AND alert_type = $2::text
		  AND closed_at IS NULL%s
	`, spec.Match.whereClause(whereParam))

	_, err = pool.Exec(ctx, updateQ, updateArgs...)
	return out, err
}

// PatchOpenMeta actualiza meta (e opcionalmente message) de um alerta aberto sem criar novo.
func PatchOpenMeta(ctx context.Context, pool *pgxpool.Pool, spec OpenSpec) error {
	if pool == nil {
		return fmt.Errorf("alertstore: pool nulo")
	}
	if spec.Meta == nil {
		spec.Meta = map[string]any{}
	}
	metaRaw, err := json.Marshal(spec.Meta)
	if err != nil {
		metaRaw = []byte("{}")
	}
	args := []any{spec.DeviceID, spec.AlertType, metaRaw}
	if strings.TrimSpace(spec.Message) != "" {
		args = []any{spec.DeviceID, spec.AlertType, spec.Message, metaRaw}
		whereParam := len(args) + 1
		args = spec.Match.appendKeyArg(args)
		q := fmt.Sprintf(`
			UPDATE alert_instances SET
				message = $3,
				meta = COALESCE(meta, '{}'::jsonb) || $4::jsonb
			WHERE device_id = $1::uuid AND alert_type = $2::text AND closed_at IS NULL%s
		`, spec.Match.whereClause(whereParam))
		_, err = pool.Exec(ctx, q, args...)
		return err
	}
	whereParam := len(args) + 1
	args = spec.Match.appendKeyArg(args)
	q := fmt.Sprintf(`
		UPDATE alert_instances SET
			meta = COALESCE(meta, '{}'::jsonb) || $3::jsonb
		WHERE device_id = $1::uuid AND alert_type = $2::text AND closed_at IS NULL%s
	`, spec.Match.whereClause(whereParam))
	_, err = pool.Exec(ctx, q, args...)
	return err
}

// CloseSpec fecha alertas abertos que correspondem ao Match.
type CloseSpec struct {
	DeviceID  uuid.UUID
	AlertType string
	Match     Match
	Resolved  map[string]any
}

// Close fecha alerta(s) e opcionalmente notifica resolução via Telegram.
func Close(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, spec CloseSpec) (bool, uuid.UUID, error) {
	if pool == nil {
		return false, uuid.Nil, fmt.Errorf("alertstore: pool nulo")
	}
	if spec.Resolved == nil {
		spec.Resolved = map[string]any{}
	}
	metaRaw, _ := json.Marshal(spec.Resolved)

	args := []any{spec.DeviceID, spec.AlertType, metaRaw}
	whereParam := len(args) + 1
	args = spec.Match.appendKeyArg(args)

	q := fmt.Sprintf(`
		UPDATE alert_instances SET
			closed_at = now(),
			meta = COALESCE(meta, '{}'::jsonb) || $3::jsonb
		WHERE device_id = $1::uuid
		  AND alert_type = $2::text
		  AND closed_at IS NULL%s
		RETURNING id, message
	`, spec.Match.whereClause(whereParam))

	var id uuid.UUID
	var msg string
	err := pool.QueryRow(ctx, q, args...).Scan(&id, &msg)
	if err != nil {
		if err == pgx.ErrNoRows {
			return false, uuid.Nil, nil
		}
		return false, uuid.Nil, err
	}
	if log != nil {
		head := alertnotify.ResolutionHeadlineForAlertType(spec.AlertType)
		alertnotify.SendResolutionTelegramAndPatchMeta(ctx, pool, log, id, head, msg)
	}
	return true, id, nil
}
