-- +goose Up
-- VSOL: pesquisa por série via listagem por PON (sem lookup directo por serial).

UPDATE olt_vendor_models
SET onu_report_commands = jsonb_set(
        onu_report_commands,
        '{serial_search_command}',
        '"show onu info {pon}"'::jsonb,
        true
    ),
    updated_at = now()
WHERE upper(trim(brand)) LIKE '%VSOL%'
  AND (
    NOT (onu_report_commands ? 'serial_search_command')
    OR onu_report_commands->>'serial_search_command' = 'show onu sn {serial}'
  );

-- +goose Down
-- Sem reversão automática.
