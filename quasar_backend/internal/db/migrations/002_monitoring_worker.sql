-- +goose Up
ALTER TABLE monitoring_runtime ADD COLUMN IF NOT EXISTS monitoring_mode TEXT NOT NULL DEFAULT 'off';
ALTER TABLE monitoring_runtime ADD COLUMN IF NOT EXISTS last_cycle_at TIMESTAMPTZ;
ALTER TABLE monitoring_runtime ADD COLUMN IF NOT EXISTS last_cycle_ok_count INT;
ALTER TABLE monitoring_runtime ADD COLUMN IF NOT EXISTS last_cycle_fail_count INT;

UPDATE monitoring_runtime SET monitoring_mode = 'off' WHERE monitoring_mode IS NULL OR monitoring_mode = '';

CREATE TABLE IF NOT EXISTS device_probe_cache (
    device_id UUID PRIMARY KEY REFERENCES devices(id) ON DELETE CASCADE,
    checked_at TIMESTAMPTZ NOT NULL,
    monitoring_mode TEXT NOT NULL,
    ok BOOLEAN NOT NULL,
    latency_ms BIGINT,
    method TEXT,
    snmp_ok BOOLEAN,
    detail JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX IF NOT EXISTS idx_device_probe_cache_checked ON device_probe_cache (checked_at DESC);

-- +goose Down
DROP TABLE IF EXISTS device_probe_cache;
ALTER TABLE monitoring_runtime DROP COLUMN IF EXISTS last_cycle_fail_count;
ALTER TABLE monitoring_runtime DROP COLUMN IF EXISTS last_cycle_ok_count;
ALTER TABLE monitoring_runtime DROP COLUMN IF EXISTS last_cycle_at;
ALTER TABLE monitoring_runtime DROP COLUMN IF EXISTS monitoring_mode;
