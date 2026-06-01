-- +goose Up
ALTER TABLE device_probe_cache
    ADD COLUMN IF NOT EXISTS snmp_health_status TEXT NOT NULL DEFAULT 'unknown',
    ADD COLUMN IF NOT EXISTS snmp_health_reason TEXT,
    ADD COLUMN IF NOT EXISTS snmp_health_checked_at TIMESTAMPTZ;

ALTER TABLE device_probe_cache
    ADD CONSTRAINT chk_device_probe_cache_snmp_health_status
    CHECK (snmp_health_status IN ('unknown', 'ok', 'partial', 'failed'));

-- +goose Down
ALTER TABLE device_probe_cache DROP CONSTRAINT IF EXISTS chk_device_probe_cache_snmp_health_status;
ALTER TABLE device_probe_cache
    DROP COLUMN IF EXISTS snmp_health_checked_at,
    DROP COLUMN IF EXISTS snmp_health_reason,
    DROP COLUMN IF EXISTS snmp_health_status;
