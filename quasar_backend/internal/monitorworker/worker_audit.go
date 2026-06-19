package monitorworker

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

const workerAuditActor = "SISTEMA"

// appendWorkerAudit regista ciclos automáticos (ping, SNMP, ONU/PON) em ops_audit_log.
func appendWorkerAudit(ctx context.Context, pool *pgxpool.Pool, entityType, entityID, action string, detail map[string]any) {
	if pool == nil {
		return
	}
	if detail != nil {
		if src, _ := detail["source"].(string); strings.TrimSpace(src) == "api" {
			return
		}
	}
	if detail == nil {
		detail = map[string]any{}
	}
	ab, err := json.Marshal(detail)
	if err != nil {
		return
	}
	_, _ = pool.Exec(ctx, `
		INSERT INTO ops_audit_log (entity_type, entity_id, action, actor, before_data, after_data)
		VALUES ($1, $2, $3, $4, NULL, $5::jsonb)
	`, entityType, entityID, action, workerAuditActor, ab)
}
