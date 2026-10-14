package monitorworker

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func loadLatestIfaceByDevice(ctx context.Context, pool *pgxpool.Pool) (map[uuid.UUID]time.Time, error) {
	out := make(map[uuid.UUID]time.Time)
	rows, err := pool.Query(ctx, `
		SELECT device_id, max(collected_at) FROM interface_snapshots GROUP BY device_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		var t time.Time
		if err := rows.Scan(&id, &t); err != nil {
			return nil, err
		}
		out[id] = t
	}
	return out, rows.Err()
}

