package alertignore

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const SQLActiveIgnoreNotExists = `
	AND NOT EXISTS (
		SELECT 1 FROM alert_ignores ig
		WHERE ig.active = true
		  AND ig.device_id = a.device_id
		  AND ig.alert_type = a.alert_type
		  AND ig.meta_key = COALESCE(
			NULLIF(trim(a.meta->>'key'), ''),
			CASE a.alert_type
			  WHEN 'olt_onu_drop' THEN COALESCE(NULLIF(trim(a.meta->>'pon'), ''), '')
			  WHEN 'olt_onu_rise' THEN COALESCE(NULLIF(trim(a.meta->>'pon'), ''), '')
			  WHEN 'olt_onu_rx' THEN COALESCE(NULLIF(trim(a.meta->>'metric_id') || ':' || trim(a.meta->>'pon'), ':'), NULLIF(trim(a.meta->>'pon'), ''))
			  WHEN 'olt_onu_tx' THEN COALESCE(NULLIF(trim(a.meta->>'metric_id') || ':' || trim(a.meta->>'pon'), ':'), NULLIF(trim(a.meta->>'pon'), ''))
			  WHEN 'telemetry_threshold' THEN CASE WHEN NULLIF(trim(a.meta->>'metric_id'), '') IS NOT NULL THEN 'telemetry:' || trim(a.meta->>'metric_id') ELSE '' END
			  WHEN 'interface_down_transition' THEN COALESCE('if:' || NULLIF(trim(a.meta->>'if_index'), ''), 'if:' || NULLIF(trim(a.meta->>'if_name'), ''))
			  WHEN 'interface_down' THEN COALESCE('if:' || NULLIF(trim(a.meta->>'if_index'), ''), 'if:' || NULLIF(trim(a.meta->>'if_name'), ''))
			  WHEN 'mikrotik_sfp_tx' THEN COALESCE(NULLIF(trim(a.meta->>'if_name'), ''), '')
			  WHEN 'mikrotik_sfp_rx' THEN COALESCE(NULLIF(trim(a.meta->>'if_name'), ''), '')
			  ELSE ''
			END,
			''
		  )
	)`

// IsMuted indica se alertas novos deste padrão devem ser suprimidos (UI + Telegram).
func IsMuted(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID, alertType, metaKey string) bool {
	if pool == nil || deviceID == uuid.Nil {
		return false
	}
	alertType = strings.TrimSpace(alertType)
	metaKey = strings.TrimSpace(metaKey)
	var ok bool
	_ = pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM alert_ignores
			WHERE active = true AND device_id = $1::uuid AND alert_type = $2 AND meta_key = $3
		)
	`, deviceID, alertType, metaKey).Scan(&ok)
	return ok
}

type IgnoreRow struct {
	ID              uuid.UUID
	DeviceID        uuid.UUID
	AlertType       string
	MetaKey         string
	DeviceName      string
	IP              string
	Severity        string
	ProblemTitle    string
	LastMessage     string
	LastMeta        map[string]any
	Reason          string
	IgnoredAt       time.Time
	SourceAlertID   *uuid.UUID
	LastVerifiedAt  *time.Time
	LastVerifyResult map[string]any
}

func IgnoreFromAlert(ctx context.Context, pool *pgxpool.Pool, alertID uuid.UUID, reason string, ignoredBy *uuid.UUID) (uuid.UUID, error) {
	if pool == nil {
		return uuid.Nil, fmt.Errorf("db indisponível")
	}
	var deviceID uuid.UUID
	var alertType, msg, sev string
	var ip, dname *string
	var metaRaw []byte
	err := pool.QueryRow(ctx, `
		SELECT device_id, alert_type, message, severity, ip, device_name, COALESCE(meta::text,'{}')
		FROM alert_instances WHERE id = $1 AND closed_at IS NULL
	`, alertID).Scan(&deviceID, &alertType, &msg, &sev, &ip, &dname, &metaRaw)
	if err != nil {
		return uuid.Nil, err
	}
	metaKey := MetaKeyFromJSON(alertType, metaRaw)
	var meta map[string]any
	_ = json.Unmarshal(metaRaw, &meta)

	_, _ = pool.Exec(ctx, `
		UPDATE alert_instances SET
			closed_at = now(),
			meta = COALESCE(meta,'{}'::jsonb) || '{"resolved":"user_ignored","source":"alert_ignore"}'::jsonb
		WHERE device_id = $1::uuid AND alert_type = $2 AND closed_at IS NULL
		  AND COALESCE(NULLIF(trim(meta->>'key'), ''), '') = $3
	`, deviceID, alertType, metaKey)

	_, _ = pool.Exec(ctx, `
		UPDATE alert_ignores SET active = false, reactivated_at = now()
		WHERE device_id = $1::uuid AND alert_type = $2 AND meta_key = $3 AND active = true
	`, deviceID, alertType, metaKey)

	var ignoreID uuid.UUID
	err = pool.QueryRow(ctx, `
		INSERT INTO alert_ignores (
			device_id, alert_type, meta_key, device_name, ip, severity, last_message, last_meta,
			reason, ignored_by, source_alert_id, active
		) VALUES (
			$1::uuid, $2, $3, $4, $5, $6, $7, COALESCE($8::jsonb,'{}'::jsonb),
			NULLIF(trim($9),''), $10::uuid, $11::uuid, true
		)
		RETURNING id
	`, deviceID, alertType, metaKey, ptrStr(dname), ptrStr(ip), sev, msg, metaRaw, reason, ignoredBy, alertID).Scan(&ignoreID)
	return ignoreID, err
}

func Reactivate(ctx context.Context, pool *pgxpool.Pool, ignoreID uuid.UUID) error {
	_, err := pool.Exec(ctx, `
		UPDATE alert_ignores SET active = false, reactivated_at = now()
		WHERE id = $1::uuid AND active = true
	`, ignoreID)
	return err
}

func ListActive(ctx context.Context, pool *pgxpool.Pool, limit int) ([]IgnoreRow, error) {
	if limit <= 0 || limit > 5000 {
		limit = 5000
	}
	rows, err := pool.Query(ctx, `
		SELECT id, device_id, alert_type, meta_key,
			COALESCE(device_name,''), COALESCE(ip,''), COALESCE(severity,''),
			COALESCE(last_message,''), COALESCE(last_meta::text,'{}'),
			COALESCE(reason,''), ignored_at, source_alert_id,
			last_verified_at, COALESCE(last_verify_result::text,'{}')
		FROM alert_ignores
		WHERE active = true
		ORDER BY ignored_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []IgnoreRow
	for rows.Next() {
		var r IgnoreRow
		var metaRaw, verifyRaw []byte
		if err := rows.Scan(
			&r.ID, &r.DeviceID, &r.AlertType, &r.MetaKey,
			&r.DeviceName, &r.IP, &r.Severity, &r.LastMessage, &metaRaw,
			&r.Reason, &r.IgnoredAt, &r.SourceAlertID, &r.LastVerifiedAt, &verifyRaw,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(metaRaw, &r.LastMeta)
		_ = json.Unmarshal(verifyRaw, &r.LastVerifyResult)
		out = append(out, r)
	}
	return out, rows.Err()
}

func PatchVerifyResult(ctx context.Context, pool *pgxpool.Pool, ignoreID *uuid.UUID, deviceID uuid.UUID, alertType, metaKey string, result map[string]any) {
	if pool == nil {
		return
	}
	raw, _ := json.Marshal(result)
	now := time.Now().UTC()
	if ignoreID != nil && *ignoreID != uuid.Nil {
		_, _ = pool.Exec(ctx, `
			UPDATE alert_ignores SET last_verified_at = $2, last_verify_result = COALESCE($3::jsonb,'{}'::jsonb)
			WHERE id = $1::uuid
		`, *ignoreID, now, raw)
		return
	}
	_, _ = pool.Exec(ctx, `
		UPDATE alert_ignores SET last_verified_at = $4, last_verify_result = COALESCE($5::jsonb,'{}'::jsonb)
		WHERE active = true AND device_id = $1::uuid AND alert_type = $2 AND meta_key = $3
	`, deviceID, alertType, metaKey, now, raw)
}

func FindActiveIgnoreID(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID, alertType, metaKey string) *uuid.UUID {
	var id uuid.UUID
	err := pool.QueryRow(ctx, `
		SELECT id FROM alert_ignores
		WHERE active = true AND device_id = $1::uuid AND alert_type = $2 AND meta_key = $3
	`, deviceID, alertType, metaKey).Scan(&id)
	if err != nil {
		return nil
	}
	return &id
}

func ptrStr(p *string) string {
	if p == nil {
		return ""
	}
	return strings.TrimSpace(*p)
}

// GuardNewAlert retorna true se o alerta deve ser suprimido (ignorado).
func GuardNewAlert(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID, alertType string, meta map[string]any) bool {
	return IsMuted(ctx, pool, deviceID, alertType, MetaKeyFromAlert(alertType, meta))
}

// ErrNotFound alerta não encontrado.
var ErrNotFound = pgx.ErrNoRows
