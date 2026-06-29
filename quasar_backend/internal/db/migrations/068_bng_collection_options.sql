-- +goose Up
ALTER TABLE settings_bng_collection
    ADD COLUMN IF NOT EXISTS options JSONB NOT NULL DEFAULT '{}'::jsonb;

-- +goose Down
ALTER TABLE settings_bng_collection
    DROP COLUMN IF EXISTS options;
