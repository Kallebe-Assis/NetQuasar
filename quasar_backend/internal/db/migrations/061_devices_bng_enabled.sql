-- +goose Up
-- Flag explícita: equipamento participa na recolha BNG/PPPoE (Configurações → Equipamentos).

ALTER TABLE devices
    ADD COLUMN IF NOT EXISTS bng_enabled BOOLEAN NOT NULL DEFAULT false;

-- Migrar equipamentos já identificados como BNG/concentrador pelo nome/categoria.
UPDATE devices SET bng_enabled = true
WHERE bng_enabled = false AND (
    lower(coalesce(category, '')) LIKE '%bng%'
    OR lower(coalesce(category, '')) LIKE '%concentrador%'
    OR lower(coalesce(description, '')) LIKE '%bng%'
);

CREATE INDEX IF NOT EXISTS idx_devices_bng_enabled ON devices (bng_enabled) WHERE bng_enabled = true;

-- +goose Down
DROP INDEX IF EXISTS idx_devices_bng_enabled;
ALTER TABLE devices DROP COLUMN IF EXISTS bng_enabled;
