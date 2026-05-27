-- +goose Up
-- VSOL: coleta simples — um snmpwalk no OID do perfil e contagem de ONUs (~100s).
UPDATE olt_vendor_models SET
  onu_online_oid = COALESCE(NULLIF(trim(onu_online_oid), ''), '1.3.6.1.4.1.37950.1.1.6.1.1'),
  snmp_base_oid = COALESCE(NULLIF(trim(snmp_base_oid), ''), '1.3.6.1.4.1.37950'),
  collection_steps = '[
    {"id":"onu","method":"onu_snmp_walk","enabled":true,"oid_field":"onu_online_oid"}
  ]'::jsonb
WHERE brand = 'VSOL';

-- +goose Down
UPDATE olt_vendor_models SET collection_steps = '[
  {"id":"if_snap","method":"if_mib_snapshot","enabled":true},
  {"id":"vsol","method":"vsol_onu_collect","enabled":true,"params":{"include_if_mib":false}}
]'::jsonb
WHERE brand = 'VSOL';
