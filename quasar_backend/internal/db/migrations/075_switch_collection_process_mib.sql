-- +goose Up
-- OIDs confirmados via snmpwalk no Nexus (CISCO-PROCESS-MIB + ENTITY-SENSOR-MIB).
UPDATE settings_switch_collection
SET metrics = metrics
    || '{
      "cpu_load": {"enabled": true, "oid": "1.3.6.1.4.1.9.9.109.1.1.1.1.8.1", "collect_mode": "snmp_get"},
      "memory_used": {"enabled": true, "oid": "1.3.6.1.4.1.9.9.109.1.1.1.1.12.1", "collect_mode": "snmp_get"},
      "memory_free": {"enabled": true, "oid": "1.3.6.1.4.1.9.9.109.1.1.1.1.13.1", "collect_mode": "snmp_get"},
      "temperature": {"enabled": true, "oid": "1.3.6.1.4.1.9.9.91.1.1.1.1.4.21590", "collect_mode": "snmp_get"}
    }'::jsonb,
    updated_at = now()
WHERE id = 1;

-- +goose Down
-- Sem reversão automática dos OIDs anteriores.
