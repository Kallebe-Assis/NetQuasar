package monitorworker

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

