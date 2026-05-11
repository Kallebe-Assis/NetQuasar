package monitorworker

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func snmpInventoryEmpty(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID) (bool, error) {
	var rc *int
	err := pool.QueryRow(ctx, `SELECT di.row_count FROM device_snmp_inventory di WHERE di.device_id=$1`, deviceID).Scan(&rc)
	if err == pgx.ErrNoRows {
		return true, nil
	}
	if err != nil {
		return true, err
	}
	if rc == nil || *rc <= 0 {
		return true, nil
	}
	return false, nil
}
