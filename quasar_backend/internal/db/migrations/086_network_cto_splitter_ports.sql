-- +goose Up
ALTER TABLE network_ctos
    ADD COLUMN IF NOT EXISTS splitter_ports JSONB;

COMMENT ON COLUMN network_ctos.splitter_ports IS 'Portas do splitter: [{port, color, label, status, note, destination}]';

-- +goose Down
ALTER TABLE network_ctos DROP COLUMN IF EXISTS splitter_ports;
