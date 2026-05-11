-- +goose Up
ALTER TABLE monitoring_runtime
    ADD COLUMN IF NOT EXISTS current_activity TEXT,
    ADD COLUMN IF NOT EXISTS activity_updated_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE monitoring_runtime
    DROP COLUMN IF EXISTS activity_updated_at,
    DROP COLUMN IF EXISTS current_activity;
