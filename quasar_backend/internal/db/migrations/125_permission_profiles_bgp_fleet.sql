-- +goose Up
-- O perfil "Usuário" (padrão para novas contas não-admin) nunca ganhou bgp.view nem fleet.view
-- quando esses módulos foram adicionados — ficaram sem acesso às telas BGP e Frota mesmo
-- estando activos no catálogo (internal/api/permission_catalog.go). Mesmo padrão retroactivo já
-- usado em 104_network_events.sql para network_events.view.
UPDATE permission_profiles
SET permissions = permissions || '["bgp.view"]'::jsonb,
    updated_at = now()
WHERE slug = 'user'
  AND jsonb_typeof(permissions) = 'array'
  AND NOT (permissions @> '["bgp.view"]'::jsonb)
  AND NOT (permissions @> '["*"]'::jsonb);

UPDATE permission_profiles
SET permissions = permissions || '["fleet.view"]'::jsonb,
    updated_at = now()
WHERE slug = 'user'
  AND jsonb_typeof(permissions) = 'array'
  AND NOT (permissions @> '["fleet.view"]'::jsonb)
  AND NOT (permissions @> '["*"]'::jsonb);

-- +goose Down
UPDATE permission_profiles
SET permissions = (
    SELECT COALESCE(jsonb_agg(value), '[]'::jsonb)
    FROM jsonb_array_elements(permissions) AS t(value)
    WHERE value::text NOT IN ('"bgp.view"', '"fleet.view"')
),
    updated_at = now()
WHERE slug = 'user'
  AND jsonb_typeof(permissions) = 'array';
