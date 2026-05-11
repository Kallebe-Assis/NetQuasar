-- +goose Up
ALTER TABLE settings_connection_defaults
    ADD COLUMN IF NOT EXISTS olt_cpu_available_oid TEXT,
    ADD COLUMN IF NOT EXISTS mikrotik_cpu_available_oid TEXT,
    ADD COLUMN IF NOT EXISTS server_cpu_available_oid TEXT;

-- +goose Down
ALTER TABLE settings_connection_defaults
    DROP COLUMN IF EXISTS server_cpu_available_oid,
    DROP COLUMN IF EXISTS mikrotik_cpu_available_oid,
    DROP COLUMN IF EXISTS olt_cpu_available_oid;
