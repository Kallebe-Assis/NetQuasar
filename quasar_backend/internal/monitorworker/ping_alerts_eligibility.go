package monitorworker

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SQLDeviceEligibleForPingAlerts is a predicate on devices alias d (already joined).
// Same criteria as GET /monitoring/active-equipment.
const SQLDeviceEligibleForPingAlerts = `
	d.ping_enabled = true
	AND TRIM(BOTH FROM COALESCE(d.operational_mode, '')) = 'Ativo'
	AND TRIM(BOTH FROM COALESCE(d.network_status, '')) = 'Normal'
	AND d.ip IS NOT NULL
	AND TRIM(BOTH FROM host(d.ip)::text) <> ''
`

// SQLDeviceEligibleForPingAlertsByID is an EXISTS subquery when devices is not joined.
func SQLDeviceEligibleForPingAlertsByID(deviceIDCol string) string {
	return `
		EXISTS (
			SELECT 1 FROM devices d
			WHERE d.id = ` + deviceIDCol + `
			AND ` + SQLDeviceEligibleForPingAlerts + `
		)`
}

// SQLDeviceEligibleForPingAlertsNotMet negates the joined predicate (device not monitored for ping alerts).
const SQLDeviceEligibleForPingAlertsNotMet = `NOT (` + SQLDeviceEligibleForPingAlerts + `)`

// DeviceEligibleForPingAlerts reports whether the device should participate in ping_unreachable alerts.
func DeviceEligibleForPingAlerts(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID) (bool, error) {
	if pool == nil {
		return false, nil
	}
	var ok bool
	err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM devices d
			WHERE d.id = $1
			AND `+SQLDeviceEligibleForPingAlerts+`
		)
	`, deviceID).Scan(&ok)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	return ok, err
}
