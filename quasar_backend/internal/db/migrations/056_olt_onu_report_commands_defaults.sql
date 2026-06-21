-- +goose Up
-- Comandos telnet reais para relatório ONU (VSOL / ZTE).

UPDATE olt_vendor_models SET onu_report_commands = '{
  "pre_commands": [],
  "commands": [
    "show onu info {pon} {onu}",
    "show onu state {pon} {onu}"
  ]
}'::jsonb, updated_at = now()
WHERE upper(trim(brand)) LIKE '%VSOL%';

UPDATE olt_vendor_models SET onu_report_commands = '{
  "pre_commands": ["terminal length 0", "terminal page-break disable", "scroll 512"],
  "commands": [
    "show gpon onu detail-info {gpon_onu}",
    "show pon onu information {gpon_onu}",
    "show pon power onu-rx {gpon_onu}",
    "show pon power onu-tx {gpon_onu}"
  ]
}'::jsonb, updated_at = now()
WHERE upper(trim(brand)) LIKE '%ZTE%';

-- +goose Down
-- Sem reversão automática dos valores anteriores.
