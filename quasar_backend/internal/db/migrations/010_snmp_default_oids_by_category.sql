-- +goose Up
ALTER TABLE settings_connection_defaults
    ADD COLUMN IF NOT EXISTS olt_cpu_oid TEXT,
    ADD COLUMN IF NOT EXISTS olt_memory_used_oid TEXT,
    ADD COLUMN IF NOT EXISTS olt_memory_size_oid TEXT,
    ADD COLUMN IF NOT EXISTS olt_temp_oid TEXT,
    ADD COLUMN IF NOT EXISTS olt_uptime_oid TEXT,
    ADD COLUMN IF NOT EXISTS mikrotik_cpu_oid TEXT,
    ADD COLUMN IF NOT EXISTS mikrotik_memory_used_oid TEXT,
    ADD COLUMN IF NOT EXISTS mikrotik_memory_size_oid TEXT,
    ADD COLUMN IF NOT EXISTS mikrotik_temp_oid TEXT,
    ADD COLUMN IF NOT EXISTS mikrotik_uptime_oid TEXT,
    ADD COLUMN IF NOT EXISTS server_cpu_oid TEXT,
    ADD COLUMN IF NOT EXISTS server_memory_used_oid TEXT,
    ADD COLUMN IF NOT EXISTS server_memory_size_oid TEXT,
    ADD COLUMN IF NOT EXISTS server_temp_oid TEXT,
    ADD COLUMN IF NOT EXISTS server_uptime_oid TEXT;

-- +goose Down
ALTER TABLE settings_connection_defaults
    DROP COLUMN IF EXISTS server_uptime_oid,
    DROP COLUMN IF EXISTS server_temp_oid,
    DROP COLUMN IF EXISTS server_memory_size_oid,
    DROP COLUMN IF EXISTS server_memory_used_oid,
    DROP COLUMN IF EXISTS server_cpu_oid,
    DROP COLUMN IF EXISTS mikrotik_uptime_oid,
    DROP COLUMN IF EXISTS mikrotik_temp_oid,
    DROP COLUMN IF EXISTS mikrotik_memory_size_oid,
    DROP COLUMN IF EXISTS mikrotik_memory_used_oid,
    DROP COLUMN IF EXISTS mikrotik_cpu_oid,
    DROP COLUMN IF EXISTS olt_uptime_oid,
    DROP COLUMN IF EXISTS olt_temp_oid,
    DROP COLUMN IF EXISTS olt_memory_size_oid,
    DROP COLUMN IF EXISTS olt_memory_used_oid,
    DROP COLUMN IF EXISTS olt_cpu_oid;
