-- +goose Up
CREATE TABLE IF NOT EXISTS mikrotik_telnet_profiles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    metrics JSONB NOT NULL DEFAULT '{}',
    pre_commands JSONB NOT NULL DEFAULT '[]',
    is_default BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_mikrotik_telnet_profiles_name
    ON mikrotik_telnet_profiles (lower(trim(name)));

ALTER TABLE devices
    ADD COLUMN IF NOT EXISTS mikrotik_telnet_profile_id UUID
        REFERENCES mikrotik_telnet_profiles(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_devices_mikrotik_telnet_profile
    ON devices (mikrotik_telnet_profile_id)
    WHERE mikrotik_telnet_profile_id IS NOT NULL;

INSERT INTO mikrotik_telnet_profiles (name, metrics, pre_commands, is_default)
SELECT 'Padrão', '{}'::jsonb, '[]'::jsonb, true
WHERE NOT EXISTS (SELECT 1 FROM mikrotik_telnet_profiles WHERE is_default = true);

-- +goose Down
ALTER TABLE devices DROP COLUMN IF EXISTS mikrotik_telnet_profile_id;
DROP TABLE IF EXISTS mikrotik_telnet_profiles;
