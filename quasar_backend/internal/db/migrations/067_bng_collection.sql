-- +goose Up
CREATE TABLE IF NOT EXISTS settings_bng_collection (
    id INT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    metrics JSONB NOT NULL DEFAULT '{}',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO settings_bng_collection (id, metrics)
VALUES (1, '{}'::jsonb)
ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS bng_stats_samples (
    id BIGSERIAL PRIMARY KEY,
    device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    collected_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    total_online INT,
    pppoe_online INT,
    ipv4_online INT,
    ipv6_online INT,
    dual_stack_online INT
);

CREATE INDEX IF NOT EXISTS idx_bng_stats_device_time ON bng_stats_samples (device_id, collected_at DESC);

ALTER TABLE bng_session_snapshots
    ADD COLUMN IF NOT EXISTS device_id UUID REFERENCES devices(id) ON DELETE CASCADE,
    ADD COLUMN IF NOT EXISTS session_count INT;

CREATE INDEX IF NOT EXISTS idx_bng_session_snapshots_device_time
    ON bng_session_snapshots (device_id, captured_at DESC)
    WHERE device_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_bng_session_snapshots_device_time;
ALTER TABLE bng_session_snapshots
    DROP COLUMN IF EXISTS session_count,
    DROP COLUMN IF EXISTS device_id;
DROP INDEX IF EXISTS idx_bng_stats_device_time;
DROP TABLE IF EXISTS bng_stats_samples;
DROP TABLE IF EXISTS settings_bng_collection;
