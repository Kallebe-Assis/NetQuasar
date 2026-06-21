-- +goose Up
-- Sequência telnet para relatório individual de ONU (aba Pesquisa OLT).

ALTER TABLE olt_vendor_models
    ADD COLUMN IF NOT EXISTS onu_report_commands JSONB NOT NULL DEFAULT '{}'::jsonb;

UPDATE olt_vendor_models SET onu_report_commands = '{
  "pre_commands": ["terminal length 0", "terminal page-break disable", "scroll 512"],
  "command": "show gpon onu detail gpon-onu_{pon}/{onu}"
}'::jsonb
WHERE upper(trim(brand)) LIKE '%ZTE%' AND (onu_report_commands IS NULL OR onu_report_commands = '{}'::jsonb);

UPDATE olt_vendor_models SET onu_report_commands = '{
  "pre_commands": ["terminal length 0"],
  "command": "show onu {pon} {onu}"
}'::jsonb
WHERE upper(trim(brand)) LIKE '%VSOL%' AND (onu_report_commands IS NULL OR onu_report_commands = '{}'::jsonb);

-- +goose Down
ALTER TABLE olt_vendor_models DROP COLUMN IF EXISTS onu_report_commands;
