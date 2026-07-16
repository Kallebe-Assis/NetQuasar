package monitorview

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PatchProbeKPIs grava KPIs compactos no detail do probe cache (evita leituras em telemetry_samples).
func PatchProbeKPIs(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID, metricsJSON []byte, collectedAt time.Time) {
	if pool == nil || deviceID == uuid.Nil {
		return
	}
	kpis := ExtractDeviceKPIs(metricsJSON, nil, &collectedAt)
	patch := KPIsDetailPatch(kpis)
	_, _ = pool.Exec(ctx, `
		UPDATE device_probe_cache SET
			detail = COALESCE(detail, '{}'::jsonb) || $2::jsonb
		WHERE device_id = $1
	`, deviceID, string(patch))
}
