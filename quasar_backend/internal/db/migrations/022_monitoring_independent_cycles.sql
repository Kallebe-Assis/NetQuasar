-- +goose Up
ALTER TABLE device_probe_cache ADD COLUMN IF NOT EXISTS reach_ok BOOLEAN;

UPDATE device_probe_cache SET reach_ok = ok WHERE reach_ok IS NULL;

ALTER TABLE device_probe_cache ALTER COLUMN reach_ok SET DEFAULT true;

ALTER TABLE device_probe_cache ALTER COLUMN reach_ok SET NOT NULL;

ALTER TABLE monitoring_runtime ADD COLUMN IF NOT EXISTS last_latency_cycle_at TIMESTAMPTZ;
ALTER TABLE monitoring_runtime ADD COLUMN IF NOT EXISTS last_telemetry_cycle_at TIMESTAMPTZ;
ALTER TABLE monitoring_runtime ADD COLUMN IF NOT EXISTS last_interface_snapshot_cycle_at TIMESTAMPTZ;
ALTER TABLE monitoring_runtime ADD COLUMN IF NOT EXISTS last_olt_if_derived_cycle_at TIMESTAMPTZ;

UPDATE monitoring_runtime SET
	last_latency_cycle_at = COALESCE(last_latency_cycle_at, last_cycle_at),
	last_telemetry_cycle_at = COALESCE(last_telemetry_cycle_at, last_cycle_at),
	last_interface_snapshot_cycle_at = COALESCE(last_interface_snapshot_cycle_at, last_cycle_at),
	last_olt_if_derived_cycle_at = COALESCE(last_olt_if_derived_cycle_at, last_cycle_at)
WHERE last_cycle_at IS NOT NULL;

-- +goose Down
ALTER TABLE monitoring_runtime DROP COLUMN IF EXISTS last_olt_if_derived_cycle_at;
ALTER TABLE monitoring_runtime DROP COLUMN IF EXISTS last_interface_snapshot_cycle_at;
ALTER TABLE monitoring_runtime DROP COLUMN IF EXISTS last_telemetry_cycle_at;
ALTER TABLE monitoring_runtime DROP COLUMN IF EXISTS last_latency_cycle_at;
ALTER TABLE device_probe_cache DROP COLUMN IF EXISTS reach_ok;
