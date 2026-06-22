-- +goose Up
-- Comando telnet para pesquisa de ONU por número de série (aba Pesquisa OLT).

UPDATE olt_vendor_models SET onu_report_commands = onu_report_commands || '{"serial_search_command": "show gpon onu by sn {serial}"}'::jsonb,
    updated_at = now()
WHERE upper(trim(brand)) LIKE '%ZTE%'
  AND NOT (onu_report_commands ? 'serial_search_command');

UPDATE olt_vendor_models SET onu_report_commands = onu_report_commands || '{"serial_search_command": "show onu sn {serial}"}'::jsonb,
    updated_at = now()
WHERE upper(trim(brand)) LIKE '%VSOL%'
  AND NOT (onu_report_commands ? 'serial_search_command');

-- +goose Down
-- Sem reversão automática dos valores anteriores.
