-- +goose Up
ALTER TABLE client_connections ADD COLUMN IF NOT EXISTS display_number SERIAL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_client_connections_display_number ON client_connections (display_number);

-- +goose Down
DROP INDEX IF EXISTS idx_client_connections_display_number;
ALTER TABLE client_connections DROP COLUMN IF EXISTS display_number;
