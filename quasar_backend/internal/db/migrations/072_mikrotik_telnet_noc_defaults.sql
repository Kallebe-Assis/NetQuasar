-- +goose Up
-- Repõe métricas do perfil padrão para o pacote NOC (defaults aplicados em runtime).
UPDATE mikrotik_telnet_profiles
SET metrics = '{}'::jsonb, updated_at = now()
WHERE is_default = true;

-- +goose Down
UPDATE mikrotik_telnet_profiles
SET metrics = '{}'::jsonb, updated_at = now()
WHERE is_default = true;
