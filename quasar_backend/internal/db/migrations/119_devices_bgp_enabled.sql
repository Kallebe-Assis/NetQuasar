-- +goose Up
-- Flag explícita: equipamento participa na recolha BGP (perfil SNMP dedicado — ver
-- bgp_snmp_profiles). Ao contrário do bng_enabled (061), sem backfill heurístico: não há como
-- inferir BGP a partir de categoria/descrição existentes, fica desligado até o utilizador marcar.

ALTER TABLE devices
    ADD COLUMN IF NOT EXISTS bgp_enabled BOOLEAN NOT NULL DEFAULT false;

CREATE INDEX IF NOT EXISTS idx_devices_bgp_enabled ON devices (bgp_enabled) WHERE bgp_enabled = true;

-- +goose Down
DROP INDEX IF EXISTS idx_devices_bgp_enabled;
ALTER TABLE devices DROP COLUMN IF EXISTS bgp_enabled;
