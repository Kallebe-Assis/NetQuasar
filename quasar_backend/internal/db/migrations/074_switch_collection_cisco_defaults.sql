-- +goose Up
-- Perfil SNMP Switch alinhado ao Nexus 5000 (IF-MIB + Cisco PROCESS/MEMPOOL).
UPDATE settings_switch_collection
SET metrics = '{
  "sys_descr": {"enabled": true, "oid": "1.3.6.1.2.1.1.1.0", "collect_mode": "snmp_get"},
  "sys_uptime": {"enabled": true, "oid": "1.3.6.1.2.1.1.3.0", "collect_mode": "snmp_get"},
  "sys_name": {"enabled": true, "oid": "1.3.6.1.2.1.1.5.0", "collect_mode": "snmp_get"},
  "cpu_load": {"enabled": true, "oid": "1.3.6.1.4.1.9.9.109.1.1.1.1.8.1", "collect_mode": "snmp_get"},
  "memory_used": {"enabled": true, "oid": "1.3.6.1.4.1.9.9.109.1.1.1.1.12.1", "collect_mode": "snmp_get"},
  "memory_free": {"enabled": true, "oid": "1.3.6.1.4.1.9.9.109.1.1.1.1.13.1", "collect_mode": "snmp_get"},
  "temperature": {"enabled": true, "oid": "1.3.6.1.4.1.9.9.91.1.1.1.1.4.21590", "collect_mode": "snmp_get"},
  "if_mib_table": {"enabled": true, "oid": "1.3.6.1.2.1.2.2.1", "collect_mode": "if_mib_table"},
  "if_x_table": {"enabled": true, "oid": "1.3.6.1.2.1.31.1.1.1", "collect_mode": "if_mib_table"},
  "if_oper_status": {"enabled": true, "oid": "1.3.6.1.2.1.2.2.1.8", "collect_mode": "if_mib_status"},
  "vlan_membership_table": {"enabled": true, "oid": "1.3.6.1.4.1.9.9.68.1.2.2.1", "collect_mode": "snmp_walk"},
  "vlan_name_table": {"enabled": true, "oid": "1.3.6.1.4.1.9.9.46.1.3.1.1.4", "collect_mode": "snmp_walk"}
}'::jsonb,
    updated_at = now()
WHERE id = 1
  AND (metrics = '{}'::jsonb OR metrics IS NULL);

-- +goose Down
UPDATE settings_switch_collection
SET metrics = '{}'::jsonb, updated_at = now()
WHERE id = 1;
