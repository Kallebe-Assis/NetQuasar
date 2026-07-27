-- +goose Up
ALTER TABLE network_cables
    ADD COLUMN IF NOT EXISTS fiber_ports JSONB;

COMMENT ON COLUMN network_cables.fiber_ports IS 'Fibras do cabo: [{port, color, color_hex, label, status, note, destination}]';

-- +goose Down
ALTER TABLE network_cables DROP COLUMN IF EXISTS fiber_ports;
