-- +goose Up
ALTER TABLE devices
    ADD COLUMN IF NOT EXISTS telemetry_oid_strategy TEXT NOT NULL DEFAULT 'default'
        CHECK (telemetry_oid_strategy IN ('default', 'manual')),
    ADD COLUMN IF NOT EXISTS telemetry_oid_overrides JSONB NOT NULL DEFAULT '{}'::jsonb;

-- +goose Down
ALTER TABLE devices
    DROP COLUMN IF EXISTS telemetry_oid_overrides,
    DROP COLUMN IF EXISTS telemetry_oid_strategy;
