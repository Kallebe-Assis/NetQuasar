package monitorworker

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func resolveSweepDevices(ctx context.Context, pool *pgxpool.Pool, opts SweepOpts, oltOnu bool) ([]pingableDeviceRow, error) {
	if len(opts.ScopedDevices) > 0 {
		return opts.ScopedDevices, nil
	}
	if oltOnu {
		return loadOltDevicesForCollect(ctx, pool, opts.DeviceID)
	}
	return loadPingableDevices(ctx, pool, opts.DeviceID)
}
