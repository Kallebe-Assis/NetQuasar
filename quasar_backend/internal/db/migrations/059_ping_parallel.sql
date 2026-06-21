-- +goose Up
ALTER TABLE monitoring_intervals
    ADD COLUMN IF NOT EXISTS ping_parallel BOOLEAN NOT NULL DEFAULT true;

-- +goose Down
ALTER TABLE monitoring_intervals DROP COLUMN IF EXISTS ping_parallel;
