// Package alertcorrelation agrupa alertas relacionados (POP offline, OLT offline em cascata).
package alertcorrelation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

const correlateWindow = 45 * time.Minute

// Reconcile analisa alertas abertos e cria/atualiza incidentes; resolve incidentes sem alertas abertos.
func Reconcile(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger) {
	if pool == nil {
		return
	}
	if err := reconcilePopOffline(ctx, pool, log); err != nil && log != nil {
		log.Warn().Err(err).Msg("correlação POP offline")
	}
	if err := reconcileOltOffline(ctx, pool, log); err != nil && log != nil {
		log.Warn().Err(err).Msg("correlação OLT offline")
	}
	if err := resolveStaleIncidents(ctx, pool); err != nil && log != nil {
		log.Warn().Err(err).Msg("resolver incidentes")
	}
}

// ShouldSkipMonitoringTelegram evita spam quando o alerta é efeito em cascata de um incidente.
func ShouldSkipMonitoringTelegram(ctx context.Context, pool *pgxpool.Pool, alertID uuid.UUID) bool {
	if pool == nil || alertID == uuid.Nil {
		return false
	}
	var role string
	var status string
	err := pool.QueryRow(ctx, `
		SELECT aia.role, i.status
		FROM alert_incident_alerts aia
		JOIN alert_incidents i ON i.id = aia.incident_id
		WHERE aia.alert_id = $1 AND i.status = 'open'
	`, alertID).Scan(&role, &status)
	if err != nil {
		return false
	}
	return role != "root"
}

func reconcilePopOffline(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger) error {
	rows, err := pool.Query(ctx, `
		SELECT d.pop_id, p.description, COUNT(DISTINCT ai.device_id)::int
		FROM alert_instances ai
		JOIN devices d ON d.id = ai.device_id
		JOIN pops p ON p.id = d.pop_id
		WHERE ai.closed_at IS NULL
		  AND ai.alert_type = 'ping_unreachable'
		  AND d.pop_id IS NOT NULL
		  AND ai.active_since >= now() - $1::interval
		GROUP BY d.pop_id, p.description
		HAVING COUNT(DISTINCT ai.device_id) >= 2
	`, correlateWindow.String())
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var popID uuid.UUID
		var popName string
		var cnt int
		if err := rows.Scan(&popID, &popName, &cnt); err != nil {
			continue
		}
		incID, err := findOrCreateIncident(ctx, pool, "pop_offline", popID, uuid.Nil,
			fmt.Sprintf("POP %s — %d equipamentos offline", strings.TrimSpace(popName), cnt),
			fmt.Sprintf("Correlação automática: %d equipamentos no mesmo POP sem resposta ICMP/TCP.", cnt))
		if err != nil {
			continue
		}
		alertRows, err := pool.Query(ctx, `
			SELECT ai.id
			FROM alert_instances ai
			JOIN devices d ON d.id = ai.device_id
			WHERE ai.closed_at IS NULL AND ai.alert_type = 'ping_unreachable'
			  AND d.pop_id = $1 AND ai.active_since >= now() - $2::interval
			ORDER BY ai.active_since ASC
		`, popID, correlateWindow.String())
		if err != nil {
			continue
		}
		linkAlertsFromRows(ctx, pool, incID, alertRows, log)
	}
	return rows.Err()
}

func reconcileOltOffline(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger) error {
	rows, err := pool.Query(ctx, `
		SELECT d.id, COALESCE(TRIM(d.description), 'OLT'), d.pop_id
		FROM devices d
		JOIN alert_instances ai ON ai.device_id = d.id
		WHERE lower(trim(d.category)) = 'olt'
		  AND ai.closed_at IS NULL
		  AND ai.alert_type = 'ping_unreachable'
	`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var oltID uuid.UUID
		var oltName string
		var popID *uuid.UUID
		if err := rows.Scan(&oltID, &oltName, &popID); err != nil {
			continue
		}
		var cascade int
		_ = pool.QueryRow(ctx, `
			SELECT COUNT(*)::int FROM alert_instances
			WHERE device_id = $1 AND closed_at IS NULL
			  AND alert_type IN ('olt_onu_drop', 'olt_onu_rise', 'telemetry_threshold')
		`, oltID).Scan(&cascade)
		title := fmt.Sprintf("OLT %s offline", oltName)
		summary := "OLT inalcançável (ICMP/TCP)."
		if cascade > 0 {
			summary = fmt.Sprintf("OLT inalcançável; %d alerta(s) associados (ONU/telemetria).", cascade)
		}
		var pop uuid.UUID
		if popID != nil {
			pop = *popID
		}
		incID, err := findOrCreateIncident(ctx, pool, "olt_offline", pop, oltID, title, summary)
		if err != nil {
			continue
		}
		// Root: ping na OLT
		var rootAlert uuid.UUID
		_ = pool.QueryRow(ctx, `
			SELECT id FROM alert_instances
			WHERE device_id = $1 AND alert_type = 'ping_unreachable' AND closed_at IS NULL
			ORDER BY active_since ASC LIMIT 1
		`, oltID).Scan(&rootAlert)
		if rootAlert != uuid.Nil {
			_ = linkAlert(ctx, pool, incID, rootAlert, "root")
		}
		cRows, err := pool.Query(ctx, `
			SELECT id FROM alert_instances
			WHERE device_id = $1 AND closed_at IS NULL
			  AND alert_type IN ('olt_onu_drop', 'olt_onu_rise', 'telemetry_threshold', 'latency_high')
		`, oltID)
		if err == nil {
			for cRows.Next() {
				var aid uuid.UUID
				if cRows.Scan(&aid) == nil {
					_ = linkAlert(ctx, pool, incID, aid, "cascade")
				}
			}
			cRows.Close()
		}
		// Equipamentos no mesmo POP (exceto a OLT) abertos por ping — efeito em cascata de infra
		if popID != nil {
			popRows, err := pool.Query(ctx, `
				SELECT ai.id FROM alert_instances ai
				JOIN devices d ON d.id = ai.device_id
				WHERE d.pop_id = $1 AND d.id <> $2
				  AND ai.closed_at IS NULL AND ai.alert_type = 'ping_unreachable'
				  AND ai.active_since >= now() - $3::interval
			`, *popID, oltID, correlateWindow.String())
			if err == nil {
				linkAlertsFromRows(ctx, pool, incID, popRows, log)
			}
		}
	}
	return rows.Err()
}

func findOrCreateIncident(ctx context.Context, pool *pgxpool.Pool, rootCause string, popID uuid.UUID, rootDeviceID uuid.UUID, title, summary string) (uuid.UUID, error) {
	var incID uuid.UUID
	var popArg any
	if popID != uuid.Nil {
		popArg = popID
	}
	var devArg any
	if rootDeviceID != uuid.Nil {
		devArg = rootDeviceID
	}
	q := `
		SELECT id FROM alert_incidents
		WHERE status = 'open' AND root_cause = $1
	`
	args := []any{rootCause}
	if rootDeviceID != uuid.Nil {
		q += ` AND root_device_id = $2`
		args = append(args, rootDeviceID)
	} else if popID != uuid.Nil {
		q += ` AND pop_id = $2`
		args = append(args, popID)
	}
	q += ` ORDER BY opened_at DESC LIMIT 1`
	err := pool.QueryRow(ctx, q, args...).Scan(&incID)
	if err == nil {
		_, _ = pool.Exec(ctx, `UPDATE alert_incidents SET title = $2, summary = $3, updated_at = now() WHERE id = $1`, incID, title, summary)
		return incID, nil
	}
	if err != pgx.ErrNoRows {
		return uuid.Nil, err
	}
	err = pool.QueryRow(ctx, `
		INSERT INTO alert_incidents (root_cause, title, summary, pop_id, root_device_id)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, rootCause, title, summary, popArg, devArg).Scan(&incID)
	return incID, err
}

func linkAlertsFromRows(ctx context.Context, pool *pgxpool.Pool, incID uuid.UUID, rows pgx.Rows, _ *zerolog.Logger) {
	defer rows.Close()
	first := true
	for rows.Next() {
		var aid uuid.UUID
		if err := rows.Scan(&aid); err != nil {
			continue
		}
		role := "member"
		if first {
			role = "root"
			first = false
		}
		_ = linkAlert(ctx, pool, incID, aid, role)
	}
}

func linkAlert(ctx context.Context, pool *pgxpool.Pool, incID, alertID uuid.UUID, role string) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO alert_incident_alerts (incident_id, alert_id, role)
		VALUES ($1, $2, $3)
		ON CONFLICT (alert_id) DO UPDATE SET incident_id = EXCLUDED.incident_id, role = EXCLUDED.role
	`, incID, alertID, role)
	if err != nil {
		return err
	}
	_, err = pool.Exec(ctx, `UPDATE alert_instances SET incident_id = $2 WHERE id = $1`, alertID, incID)
	return err
}

func resolveStaleIncidents(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
		UPDATE alert_incidents i SET
			status = 'resolved',
			resolved_at = now(),
			updated_at = now()
		WHERE i.status = 'open'
		  AND NOT EXISTS (
			SELECT 1 FROM alert_incident_alerts aia
			JOIN alert_instances ai ON ai.id = aia.alert_id
			WHERE aia.incident_id = i.id AND ai.closed_at IS NULL
		  )
	`)
	return err
}
