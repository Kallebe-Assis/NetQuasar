package monitorworker

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type telemetryCycleOutcome struct {
	OK      bool
	Skipped bool
	Reason  string
	Extra   map[string]any
}

// recordTelemetryCycleOutcome regista skip/falha no detail do probe cache (sem INSERT em telemetry_samples).
func recordTelemetryCycleOutcome(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID, source string, out telemetryCycleOutcome) {
	if pool == nil || deviceID == uuid.Nil {
		return
	}
	source = strings.TrimSpace(source)
	if source == "" {
		source = "worker"
	}
	cycle := map[string]any{
		"ok":      out.OK,
		"skipped": out.Skipped,
		"source":  source,
		"at":      time.Now().UTC().Format(time.RFC3339Nano),
	}
	if r := strings.TrimSpace(out.Reason); r != "" {
		cycle["reason"] = r
	}
	patch, err := json.Marshal(map[string]any{"telemetry_cycle": cycle})
	if err != nil {
		return
	}
	WithDeviceProbeRowLock(deviceID, func() {
		_, _ = pool.Exec(ctx, `
			UPDATE device_probe_cache SET
				detail = COALESCE(detail, '{}'::jsonb) || $2::jsonb,
				checked_at = now()
			WHERE device_id = $1
		`, deviceID, string(patch))
	})
}

func patchProbeSNMPHealth(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID, mode string, snmpOK bool, healthStatus, healthReason string, detailJSON []byte) {
	if pool == nil || deviceID == uuid.Nil {
		return
	}
	healthStatus = strings.TrimSpace(healthStatus)
	if healthStatus == "" {
		healthStatus = "unknown"
	}
	WithDeviceProbeRowLock(deviceID, func() {
		_, _ = pool.Exec(ctx, `
			UPDATE device_probe_cache SET
				snmp_ok = $2,
				ok = CASE WHEN monitoring_mode = $3 THEN reach_ok ELSE (reach_ok AND $2::bool) END,
				detail = COALESCE(detail, '{}'::jsonb) || $4::jsonb,
				snmp_health_status = $5::text,
				snmp_health_reason = NULLIF(trim($6::text), ''),
				snmp_health_checked_at = now(),
				checked_at = now()
			WHERE device_id = $1
		`, deviceID, snmpOK, mode, string(detailJSON), healthStatus, healthReason)
	})
}

func probeDetailFromTelemetry(source string, snmpDetail any, mikrotik any) []byte {
	snippet, _ := json.Marshal(map[string]any{
		"snmp":                snmpDetail,
		"telemetry_cycle":     map[string]any{"source": source},
		"mikrotik_collection": mikrotik,
	})
	return snippet
}
