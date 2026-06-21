-- +goose Up
-- VSOL: enable + config terminal antes dos comandos show onu.

UPDATE olt_vendor_models SET onu_report_commands = '{
  "pre_commands": ["enable", "conf terminal"],
  "commands": [
    "show onu info {pon} {onu}",
    "show onu state {pon} {onu}"
  ]
}'::jsonb, updated_at = now()
WHERE upper(trim(brand)) LIKE '%VSOL%';

-- +goose Down
-- Sem reversão automática.
