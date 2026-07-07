-- +goose Up
-- VLAN por interface (CISCO-VLAN-MEMBERSHIP + VTP names) — confirmado em switch_cisco_II.txt
UPDATE settings_switch_collection
SET metrics = metrics
    || '{
      "vlan_membership_table": {"enabled": true, "oid": "1.3.6.1.4.1.9.9.68.1.2.2.1", "collect_mode": "snmp_walk"},
      "vlan_name_table": {"enabled": true, "oid": "1.3.6.1.4.1.9.9.46.1.3.1.1.4", "collect_mode": "snmp_walk"}
    }'::jsonb,
    updated_at = now()
WHERE id = 1;

-- +goose Down
-- Sem reversão automática.
