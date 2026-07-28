-- +goose Up
ALTER TABLE settings_ui
    ADD COLUMN IF NOT EXISTS map_cto_color TEXT NOT NULL DEFAULT '#0d0663'
        CHECK (map_cto_color ~ '^#[0-9A-Fa-f]{6}$'),
    ADD COLUMN IF NOT EXISTS map_splice_color TEXT NOT NULL DEFAULT '#d97706'
        CHECK (map_splice_color ~ '^#[0-9A-Fa-f]{6}$'),
    ADD COLUMN IF NOT EXISTS map_equipment_icon TEXT NOT NULL DEFAULT 'pin',
    ADD COLUMN IF NOT EXISTS map_connection_icon TEXT NOT NULL DEFAULT 'user',
    ADD COLUMN IF NOT EXISTS map_cto_icon TEXT NOT NULL DEFAULT 'pin',
    ADD COLUMN IF NOT EXISTS map_splice_icon TEXT NOT NULL DEFAULT 'rocket';

-- +goose Down
ALTER TABLE settings_ui
    DROP COLUMN IF EXISTS map_splice_icon,
    DROP COLUMN IF EXISTS map_cto_icon,
    DROP COLUMN IF EXISTS map_connection_icon,
    DROP COLUMN IF EXISTS map_equipment_icon,
    DROP COLUMN IF EXISTS map_splice_color,
    DROP COLUMN IF EXISTS map_cto_color;
