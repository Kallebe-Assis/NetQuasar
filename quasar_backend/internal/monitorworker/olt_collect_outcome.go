package monitorworker

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// OltCollectOutcome resultado de uma tentativa de coleta ONU/PON.
type OltCollectOutcome struct {
	OK       bool
	Skipped  bool
	Reason   string
	PonCount int
	Mode     string // if_derived | vendor_profile
}

func recordOltCollectAttempt(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID, source string, out OltCollectOutcome) {
	if pool == nil || deviceID == uuid.Nil || out.OK {
		return
	}
	source = strings.TrimSpace(source)
	if source == "" {
		source = "worker"
	}
	patch := map[string]any{
		"last_collect_at":     time.Now().UTC().Format(time.RFC3339Nano),
		"last_collect_ok":       false,
		"last_collect_source":   source,
		"olt_collection_mode":   strings.TrimSpace(out.Mode),
		"last_collect_skipped":  out.Skipped,
	}
	if r := strings.TrimSpace(out.Reason); r != "" {
		patch["last_collect_error"] = r
	}
	sb, err := json.Marshal(patch)
	if err != nil {
		return
	}
	_, _ = pool.Exec(ctx, `
		INSERT INTO olt_snapshots (device_id, summary, pons) VALUES ($1, $2::jsonb, '[]'::jsonb)
		ON CONFLICT (device_id) DO UPDATE SET
			summary = COALESCE(olt_snapshots.summary, '{}'::jsonb) || $2::jsonb
	`, deviceID, sb)
}
