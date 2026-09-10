-- +goose Up
-- Permissões por aba do Dashboard — cada perfil pode ver só as abas que lhe forem concedidas.
-- A permissão base "dashboard.view" continua a controlar o acesso à tela; estas chaves filtram
-- quais abas aparecem. O perfil de sistema "Usuário" recebe todas (comportamento antigo mantido).
UPDATE permission_profiles
SET permissions = permissions || '[
      "dashboard.tab.geral","dashboard.tab.equipamentos","dashboard.tab.fibra",
      "dashboard.tab.infra","dashboard.tab.sessoes","dashboard.tab.servidor","dashboard.tab.frota"
    ]'::jsonb
WHERE slug = 'user'
  AND NOT (permissions @> '["dashboard.tab.geral"]'::jsonb);

-- +goose Down
UPDATE permission_profiles
SET permissions = (
      SELECT COALESCE(jsonb_agg(v), '[]'::jsonb)
      FROM jsonb_array_elements(permissions) v
      WHERE v::text NOT LIKE '"dashboard.tab.%'
    )
WHERE slug = 'user';
