-- +goose Up
-- Perfil Telnet Switch: comandos Cisco NX-OS (em vez de defaults RouterOS).
UPDATE switch_telnet_profiles
SET
  name = CASE WHEN is_default THEN 'Cisco NX-OS' ELSE name END,
  metrics = '{}'::jsonb,
  pre_commands = '["terminal length 0"]'::jsonb,
  updated_at = now()
WHERE is_default = true;

INSERT INTO switch_telnet_profiles (name, metrics, pre_commands, is_default)
SELECT 'Cisco NX-OS', '{}'::jsonb, '["terminal length 0"]'::jsonb, true
WHERE NOT EXISTS (SELECT 1 FROM switch_telnet_profiles WHERE is_default = true);

-- +goose Down
UPDATE switch_telnet_profiles
SET metrics = '{}'::jsonb, pre_commands = '[]'::jsonb, updated_at = now()
WHERE is_default = true;
