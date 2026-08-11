-- +goose Up
ALTER TABLE network_ctos
    ADD COLUMN IF NOT EXISTS olt_device_id UUID REFERENCES devices(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS pon INTEGER;

CREATE INDEX IF NOT EXISTS idx_network_ctos_olt_device ON network_ctos(olt_device_id);
CREATE INDEX IF NOT EXISTS idx_network_ctos_olt_pon ON network_ctos(olt_device_id, pon);

COMMENT ON COLUMN network_ctos.olt_device_id IS 'OLT (transmissor) à qual a CTO está ligada';
COMMENT ON COLUMN network_ctos.pon IS 'Número da interface/PON da OLT; a VLAN vem de devices.pon_vlans';

-- +goose Down
DROP INDEX IF EXISTS idx_network_ctos_olt_pon;
DROP INDEX IF EXISTS idx_network_ctos_olt_device;
ALTER TABLE network_ctos DROP COLUMN IF EXISTS pon;
ALTER TABLE network_ctos DROP COLUMN IF EXISTS olt_device_id;
