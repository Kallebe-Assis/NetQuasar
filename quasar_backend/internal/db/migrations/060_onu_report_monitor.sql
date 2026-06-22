-- +goose Up
-- Campos opcionais em onu_report_commands (JSON): enabled, monitor_online_only, max_onus_per_cycle.
-- Sem alteração de schema — documentação de defaults para VSOL/ZTE.

UPDATE olt_vendor_models SET onu_report_commands = onu_report_commands || '{"monitor_online_only": true, "max_onus_per_cycle": 25}'::jsonb
WHERE onu_report_commands IS NOT NULL
  AND onu_report_commands <> '{}'::jsonb
  AND NOT (onu_report_commands ? 'max_onus_per_cycle');

-- +goose Down
-- Sem reversão.
