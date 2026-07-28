-- +goose Up
ALTER TABLE network_cables
    ADD COLUMN IF NOT EXISTS path JSONB;

COMMENT ON COLUMN network_cables.path IS 'Trajeto do cabo: [{lat, lng}, ...]';

-- +goose Down
ALTER TABLE network_cables DROP COLUMN IF EXISTS path;
