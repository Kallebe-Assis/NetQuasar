-- +goose Up
ALTER TABLE devices
    ADD COLUMN IF NOT EXISTS pon_descriptions JSONB NOT NULL DEFAULT '{}'::jsonb;

-- +goose Down
ALTER TABLE devices
    DROP COLUMN IF EXISTS pon_descriptions;
