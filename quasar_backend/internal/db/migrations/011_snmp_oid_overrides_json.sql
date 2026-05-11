-- +goose Up
ALTER TABLE settings_connection_defaults
    ADD COLUMN IF NOT EXISTS snmp_oid_overrides JSONB NOT NULL DEFAULT '{}'::jsonb;

-- +goose Down
ALTER TABLE settings_connection_defaults
    DROP COLUMN IF EXISTS snmp_oid_overrides;
