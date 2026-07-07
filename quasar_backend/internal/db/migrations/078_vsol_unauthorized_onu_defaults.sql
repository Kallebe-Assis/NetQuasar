-- +goose Up
-- VSOL: consulta de ONUs não autorizadas (show onu auto-find por porta PON).

UPDATE olt_vendor_models
SET onu_report_commands = jsonb_set(
        jsonb_set(
            coalesce(onu_report_commands, '{}'::jsonb),
            '{unauthorized_onu_pre_commands}',
            '["enable", "{enable}", "configure terminal", "interface gpon 0/{pon}"]'::jsonb,
            true
        ),
        '{unauthorized_onu_query_command}',
        '"show onu auto-find"'::jsonb,
        true
    ),
    updated_at = now()
WHERE upper(trim(brand)) LIKE '%VSOL%'
  AND (
    NOT (onu_report_commands ? 'unauthorized_onu_query_command')
    OR trim(coalesce(onu_report_commands->>'unauthorized_onu_query_command', '')) = ''
  );

-- +goose Down
-- Sem reversão automática.
