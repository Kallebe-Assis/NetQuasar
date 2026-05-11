-- +goose Up
-- Intervalos independentes em segundos (latência, telemetria, interfaces, ONUs/PON por OLT derivada IF-MIB).
ALTER TABLE monitoring_intervals ADD COLUMN IF NOT EXISTS telemetry_seconds INT NOT NULL DEFAULT 120;
ALTER TABLE monitoring_intervals ADD COLUMN IF NOT EXISTS interface_snapshot_seconds INT NOT NULL DEFAULT 300;
ALTER TABLE monitoring_intervals ADD COLUMN IF NOT EXISTS olt_if_derived_pon_seconds INT NOT NULL DEFAULT 240;

UPDATE monitoring_intervals
SET telemetry_seconds = GREATEST(120, COALESCE(telemetry_minutes, 2) * 60)
WHERE id = 1;

-- +goose Down
ALTER TABLE monitoring_intervals DROP COLUMN IF EXISTS olt_if_derived_pon_seconds;
ALTER TABLE monitoring_intervals DROP COLUMN IF EXISTS interface_snapshot_seconds;
ALTER TABLE monitoring_intervals DROP COLUMN IF EXISTS telemetry_seconds;
