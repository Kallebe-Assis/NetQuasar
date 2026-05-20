-- +goose Up
-- Partial index for monitor worker device selection (ping_enabled + Ativo + Normal + IP).
CREATE INDEX IF NOT EXISTS idx_devices_ping_monitorable
    ON devices (id)
    WHERE ping_enabled
      AND operational_mode = 'Ativo'
      AND network_status = 'Normal'
      AND ip IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_devices_ping_monitorable;
