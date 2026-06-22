-- +goose Up
-- Comandos telnet por PON/SFP (voltagem, TX, temperatura, bias) no perfil OLT.

ALTER TABLE olt_vendor_models
    ADD COLUMN IF NOT EXISTS pon_telnet_commands JSONB NOT NULL DEFAULT '{}'::jsonb;

UPDATE olt_vendor_models SET pon_telnet_commands = '{
  "enabled": false,
  "max_pons_per_cycle": 16,
  "pre_commands": ["terminal length 0", "terminal page-break disable", "scroll 512"],
  "commands": [
    "show pon power olt-tx gpon-olt_1/1/{pon}",
    "show pon power olt-rx gpon-olt_1/1/{pon}",
    "show optical-module-info gpon-olt_1/1/{pon}"
  ]
}'::jsonb, updated_at = now()
WHERE upper(trim(brand)) LIKE '%ZTE%'
  AND (pon_telnet_commands IS NULL OR pon_telnet_commands = '{}'::jsonb);

UPDATE olt_vendor_models SET pon_telnet_commands = '{
  "enabled": false,
  "max_pons_per_cycle": 16,
  "pre_commands": ["enable", "{enable}", "conf terminal"],
  "commands": [
    "show pon optical-transceiver-diagnosis slot 0 pon {pon}"
  ]
}'::jsonb, updated_at = now()
WHERE upper(trim(brand)) LIKE '%VSOL%'
  AND (pon_telnet_commands IS NULL OR pon_telnet_commands = '{}'::jsonb);

-- +goose Down
ALTER TABLE olt_vendor_models DROP COLUMN IF EXISTS pon_telnet_commands;
