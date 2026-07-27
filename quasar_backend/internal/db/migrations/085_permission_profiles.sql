-- +goose Up
CREATE TABLE permission_profiles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE,
    description TEXT,
    permissions JSONB NOT NULL DEFAULT '[]'::jsonb,
    is_system BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT permission_profiles_permissions_array CHECK (jsonb_typeof(permissions) = 'array')
);

INSERT INTO permission_profiles (name, slug, description, permissions, is_system)
VALUES
    ('Administrador', 'admin', 'Acesso total ao sistema.', '["*"]'::jsonb, true),
    ('Usuário', 'user', 'Acesso padrão somente para consulta.', '[
      "dashboard.view", "monitoring.view", "realtime.view", "integrations.view",
      "pops.view", "devices.view", "commercial.view", "connections.view",
      "alerts.view", "map.view", "tools.view", "olt.view", "mikrotik.view",
      "switch.view", "bng.view", "reports.view"
    ]'::jsonb, true)
ON CONFLICT (slug) DO NOTHING;

ALTER TABLE users
    ADD COLUMN permission_profile_id UUID REFERENCES permission_profiles(id) ON DELETE SET NULL;

UPDATE users u
SET permission_profile_id = p.id
FROM permission_profiles p
WHERE p.slug = CASE WHEN u.role = 'admin' THEN 'admin' ELSE 'user' END
  AND u.permission_profile_id IS NULL;

CREATE INDEX idx_users_permission_profile ON users(permission_profile_id);

-- +goose Down
DROP INDEX IF EXISTS idx_users_permission_profile;
ALTER TABLE users DROP COLUMN IF EXISTS permission_profile_id;
DROP TABLE IF EXISTS permission_profiles;
