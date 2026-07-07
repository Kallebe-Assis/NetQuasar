-- +goose Up
CREATE TABLE IF NOT EXISTS settings_switch_collection (
    id INT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    metrics JSONB NOT NULL DEFAULT '{}',
    collection_steps JSONB NOT NULL DEFAULT '[]',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO settings_switch_collection (id, metrics, collection_steps)
VALUES (1, '{}'::jsonb, '[]'::jsonb)
ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS switch_telnet_profiles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    metrics JSONB NOT NULL DEFAULT '{}',
    pre_commands JSONB NOT NULL DEFAULT '[]',
    is_default BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_switch_telnet_profiles_name
    ON switch_telnet_profiles (lower(trim(name)));

ALTER TABLE devices
    ADD COLUMN IF NOT EXISTS switch_telnet_profile_id UUID
        REFERENCES switch_telnet_profiles(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_devices_switch_telnet_profile
    ON devices (switch_telnet_profile_id)
    WHERE switch_telnet_profile_id IS NOT NULL;

INSERT INTO switch_telnet_profiles (name, metrics, pre_commands, is_default)
SELECT 'Padrão', '{}'::jsonb, '[]'::jsonb, true
WHERE NOT EXISTS (SELECT 1 FROM switch_telnet_profiles WHERE is_default = true);

-- +goose Down
ALTER TABLE devices DROP COLUMN IF EXISTS switch_telnet_profile_id;
DROP TABLE IF EXISTS switch_telnet_profiles;
DROP TABLE IF EXISTS settings_switch_collection;
