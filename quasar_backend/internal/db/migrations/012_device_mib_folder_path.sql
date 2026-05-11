-- +goose Up
ALTER TABLE devices
    ADD COLUMN IF NOT EXISTS mib_folder_path TEXT;

-- +goose Down
ALTER TABLE devices
    DROP COLUMN IF EXISTS mib_folder_path;
