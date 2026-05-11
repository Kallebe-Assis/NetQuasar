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

// wakeSeconds devolve o menor intervalo ≥30 s para o worker acordar o ciclo (latência/telemetria/interfaces/PON).
func wakeSeconds(ping, tel, iface, olt int) int {
	w := clampInt(ping, 30, 86400)
	for _, v := range []int{tel, iface, olt} {
		if v < 30 {
			continue
		}
		vv := clampInt(v, 30, 86400)
		if vv < w {
			w = vv
		}
	}
	return w
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

func loadOltSnapshotByDevice(ctx context.Context, pool *pgxpool.Pool) (map[uuid.UUID]time.Time, error) {
	out := make(map[uuid.UUID]time.Time)
	rows, err := pool.Query(ctx, `SELECT device_id, updated_at FROM olt_snapshots`)
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
