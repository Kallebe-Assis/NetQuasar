-- +goose Up
ALTER TABLE monitoring_runtime
    ADD COLUMN IF NOT EXISTS activity_started_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS last_activity TEXT,
    ADD COLUMN IF NOT EXISTS last_activity_finished_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE monitoring_runtime
    DROP COLUMN IF EXISTS last_activity_finished_at,
    DROP COLUMN IF EXISTS last_activity,
    DROP COLUMN IF EXISTS activity_started_at;
