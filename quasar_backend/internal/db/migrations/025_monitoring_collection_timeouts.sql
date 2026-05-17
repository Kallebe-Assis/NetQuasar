-- +goose Up
-- Timeouts por tipo de coleta (ms), além de ping_timeout_ms já existente.
ALTER TABLE monitoring_intervals ADD COLUMN IF NOT EXISTS telemetry_timeout_ms INT NOT NULL DEFAULT 120000;
ALTER TABLE monitoring_intervals ADD COLUMN IF NOT EXISTS interface_snapshot_timeout_ms INT NOT NULL DEFAULT 120000;
ALTER TABLE monitoring_intervals ADD COLUMN IF NOT EXISTS olt_if_derived_pon_timeout_ms INT NOT NULL DEFAULT 180000;

-- +goose Down
ALTER TABLE monitoring_intervals DROP COLUMN IF EXISTS olt_if_derived_pon_timeout_ms;
ALTER TABLE monitoring_intervals DROP COLUMN IF EXISTS interface_snapshot_timeout_ms;
ALTER TABLE monitoring_intervals DROP COLUMN IF EXISTS telemetry_timeout_ms;
