-- +goose Up
-- Passos de coleta OLT configuráveis por perfil (marca + modelo).
ALTER TABLE olt_vendor_models
    ADD COLUMN IF NOT EXISTS collection_steps JSONB NOT NULL DEFAULT '[]'::jsonb;

-- VSOL: IF-MIB + tabela enterprise (snmpwalk gOnuAuthList)
UPDATE olt_vendor_models SET collection_steps = '[
  {"id":"if_refresh","method":"if_mib_refresh","enabled":true},
  {"id":"vsol","method":"vsol_onu_collect","enabled":true,"params":{"include_if_mib":false}}
]'::jsonb
WHERE brand = 'VSOL';

-- ZTE: walks SNMP + telnet (OIDs vêm das colunas do perfil ou do passo)
UPDATE olt_vendor_models SET collection_steps = '[
  {"id":"onu_walk","method":"snmp_walk","store_as":"zte_onu_online_table","oid_field":"onu_online_oid","enabled":true},
  {"id":"pon_walk","method":"snmp_walk","store_as":"zte_pon_status_table","oid_field":"pon_status_oid","enabled":true},
  {"id":"trx_walk","method":"snmp_walk","store_as":"zte_transceiver_table","oid_field":"transceiver_oid","enabled":true},
  {"id":"telnet","method":"telnet","command":"show gpon onu state","parser":"zte_gpon_onu_state","store_as":"pons","enabled":true,
   "pre_commands":["terminal length 0","terminal page-break disable","scroll 512"]}
]'::jsonb
WHERE brand = 'ZTE';

-- Datacom: walks + agregação de PONs
UPDATE olt_vendor_models SET collection_steps = '[
  {"id":"onu_walk","method":"snmp_walk","store_as":"datacom_onu_online_table","oid_field":"onu_online_oid","enabled":true},
  {"id":"pon_walk","method":"snmp_walk","store_as":"datacom_pon_status_table","oid_field":"pon_status_oid","enabled":true},
  {"id":"trx_walk","method":"snmp_walk","store_as":"datacom_transceiver_table","oid_field":"transceiver_oid","enabled":true},
  {"id":"pons","method":"datacom_build_pons","enabled":true}
]'::jsonb
WHERE brand = 'Datacom';

-- Demais marcas: apenas derivação IF-MIB (configurável na UI)
UPDATE olt_vendor_models SET collection_steps = '[
  {"id":"if_refresh","method":"if_mib_refresh","enabled":true},
  {"id":"merge","method":"if_mib_merge_pons","enabled":true},
  {"id":"stab","method":"stabilize_pons","enabled":true}
]'::jsonb
WHERE brand NOT IN ('VSOL', 'ZTE', 'Datacom');

-- +goose Down
ALTER TABLE olt_vendor_models DROP COLUMN IF EXISTS collection_steps;
