// Package sysevents grava a linha do tempo operacional na tabela events.
package sysevents

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	TypeAlertOpened = "alert.opened"
	TypeAlertClosed = "alert.closed"
	TypeDeviceChecks = "device.checks"
)

// Emit insere um evento na tabela events. Falhas de escrita são ignoradas pelo
// chamador tipicamente (best-effort); aqui devolvemos o erro para testes.
func Emit(ctx context.Context, pool *pgxpool.Pool, eventType, severity string, deviceID *uuid.UUID, payload map[string]any) error {
	if pool == nil {
		return nil
	}
	eventType = strings.TrimSpace(eventType)
	if eventType == "" {
		return nil
	}
	if payload == nil {
		payload = map[string]any{}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		raw = []byte("{}")
	}
	sev := strings.TrimSpace(severity)
	var sevArg any
	if sev != "" {
		sevArg = sev
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO events (event_type, severity, device_id, payload)
		VALUES ($1, $2, $3, COALESCE($4::jsonb, '{}'::jsonb))
	`, eventType, sevArg, deviceID, raw)
	return err
}

// EmitAlertOpened regista abertura de alerta (best-effort).
func EmitAlertOpened(ctx context.Context, pool *pgxpool.Pool, alertID, deviceID uuid.UUID, alertType, severity, message string, meta map[string]any) {
	did := deviceID
	payload := map[string]any{
		"alert_id":   alertID,
		"alert_type": alertType,
		"message":    message,
	}
	if len(meta) > 0 {
		payload["meta"] = meta
	}
	_ = Emit(ctx, pool, TypeAlertOpened, severity, &did, payload)
}

// EmitAlertClosed regista resolução de alerta (best-effort).
func EmitAlertClosed(ctx context.Context, pool *pgxpool.Pool, alertID, deviceID uuid.UUID, alertType, message string) {
	did := deviceID
	_ = Emit(ctx, pool, TypeAlertClosed, "info", &did, map[string]any{
		"alert_id":   alertID,
		"alert_type": alertType,
		"message":    message,
	})
}
