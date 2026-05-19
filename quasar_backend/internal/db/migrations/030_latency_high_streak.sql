-- +goose Up
ALTER TABLE device_probe_cache
    ADD COLUMN IF NOT EXISTS latency_high_streak INT NOT NULL DEFAULT 0
        CHECK (latency_high_streak >= 0);

-- +goose Down
ALTER TABLE device_probe_cache DROP COLUMN IF EXISTS latency_high_streak;
