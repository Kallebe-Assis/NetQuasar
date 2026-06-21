-- +goose Up
ALTER TABLE network_ctos
    ADD COLUMN IF NOT EXISTS transmitter TEXT;

-- +goose Down
ALTER TABLE network_ctos DROP COLUMN IF EXISTS transmitter;
