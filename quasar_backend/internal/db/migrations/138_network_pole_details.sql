-- +goose Up
-- Dados mais completos do poste — altura, se tem transformador, material (madeira/concreto).
ALTER TABLE network_poles
    ADD COLUMN IF NOT EXISTS height_m DOUBLE PRECISION,
    ADD COLUMN IF NOT EXISTS has_transformer BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS material TEXT;

ALTER TABLE network_poles DROP CONSTRAINT IF EXISTS network_poles_material_chk;
ALTER TABLE network_poles ADD CONSTRAINT network_poles_material_chk
    CHECK (material IS NULL OR material IN ('madeira', 'concreto'));

ALTER TABLE network_poles DROP CONSTRAINT IF EXISTS network_poles_height_chk;
ALTER TABLE network_poles ADD CONSTRAINT network_poles_height_chk
    CHECK (height_m IS NULL OR (height_m > 0 AND height_m < 100));

-- +goose Down
ALTER TABLE network_poles DROP CONSTRAINT IF EXISTS network_poles_height_chk;
ALTER TABLE network_poles DROP CONSTRAINT IF EXISTS network_poles_material_chk;
ALTER TABLE network_poles DROP COLUMN IF EXISTS material;
ALTER TABLE network_poles DROP COLUMN IF EXISTS has_transformer;
ALTER TABLE network_poles DROP COLUMN IF EXISTS height_m;
