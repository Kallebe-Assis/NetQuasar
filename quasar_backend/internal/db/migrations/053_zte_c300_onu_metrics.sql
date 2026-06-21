-- +goose Up
-- ZTE C300: mesmo perfil SNMP de métricas ONU que C320 (coleta automática no worker).
UPDATE olt_vendor_models SET
  onu_metrics = (
    SELECT onu_metrics FROM olt_vendor_models WHERE brand = 'ZTE' AND model = 'C320' LIMIT 1
  ),
  onu_online_oid = COALESCE(NULLIF(trim(onu_online_oid), ''), '1.3.6.1.4.1.3902.1082.500.2.2.6.3.1.1'),
  pon_status_oid = COALESCE(NULLIF(trim(pon_status_oid), ''), '1.3.6.1.2.1.2.2.1.8'),
  collection_steps = '[{"id":"onu_metrics","method":"onu_metrics_collect","enabled":true}]'::jsonb
WHERE brand = 'ZTE' AND model = 'C300'
  AND EXISTS (SELECT 1 FROM olt_vendor_models WHERE brand = 'ZTE' AND model = 'C320');

-- +goose Down
UPDATE olt_vendor_models SET
  collection_steps = '[
    {"id":"onu_walk","method":"snmp_walk","store_as":"zte_onu_online_table","oid_field":"onu_online_oid","enabled":true},
    {"id":"pon_walk","method":"snmp_walk","store_as":"zte_pon_status_table","oid_field":"pon_status_oid","enabled":true},
    {"id":"trx_walk","method":"snmp_walk","store_as":"zte_transceiver_table","oid_field":"transceiver_oid","enabled":true},
    {"id":"telnet","method":"telnet","command":"show gpon onu state","parser":"zte_gpon_onu_state","store_as":"pons","enabled":true,
     "pre_commands":["terminal length 0","terminal page-break disable","scroll 512"]}
  ]'::jsonb,
  onu_metrics = '{}'::jsonb
WHERE brand = 'ZTE' AND model = 'C300';
