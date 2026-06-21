package monitorworker

import (
	"context"

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
	persistOltCollectStatus(ctx, pool, deviceID, source, out)
}
