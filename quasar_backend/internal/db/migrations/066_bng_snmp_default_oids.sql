-- +goose Up
ALTER TABLE settings_connection_defaults
    ADD COLUMN IF NOT EXISTS bng_cpu_oid TEXT,
    ADD COLUMN IF NOT EXISTS bng_cpu_available_oid TEXT,
    ADD COLUMN IF NOT EXISTS bng_memory_used_oid TEXT,
    ADD COLUMN IF NOT EXISTS bng_memory_size_oid TEXT,
    ADD COLUMN IF NOT EXISTS bng_temp_oid TEXT,
    ADD COLUMN IF NOT EXISTS bng_uptime_oid TEXT;

-- +goose Down
ALTER TABLE settings_connection_defaults
    DROP COLUMN IF EXISTS bng_uptime_oid,
    DROP COLUMN IF EXISTS bng_temp_oid,
    DROP COLUMN IF EXISTS bng_memory_size_oid,
    DROP COLUMN IF EXISTS bng_memory_used_oid,
    DROP COLUMN IF EXISTS bng_cpu_available_oid,
    DROP COLUMN IF EXISTS bng_cpu_oid;
