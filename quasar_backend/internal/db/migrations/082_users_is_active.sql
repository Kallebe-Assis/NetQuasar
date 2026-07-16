-- +goose Up
ALTER TABLE users
  ADD COLUMN IF NOT EXISTS is_active BOOLEAN NOT NULL DEFAULT true;

-- +goose Down
ALTER TABLE users DROP COLUMN IF EXISTS is_active;
