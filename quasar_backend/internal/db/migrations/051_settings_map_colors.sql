-- +goose Up
ALTER TABLE settings_ui
    ADD COLUMN IF NOT EXISTS map_equipment_color TEXT NOT NULL DEFAULT '#3388ff'
        CHECK (map_equipment_color ~ '^#[0-9A-Fa-f]{6}$'),
    ADD COLUMN IF NOT EXISTS map_connection_color TEXT NOT NULL DEFAULT '#3b82f6'
        CHECK (map_connection_color ~ '^#[0-9A-Fa-f]{6}$');

-- +goose Down
ALTER TABLE settings_ui
    DROP COLUMN IF EXISTS map_connection_color,
    DROP COLUMN IF EXISTS map_equipment_color;
