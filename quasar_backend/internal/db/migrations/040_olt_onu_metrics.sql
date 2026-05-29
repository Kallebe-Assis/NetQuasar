-- +goose Up
ALTER TABLE olt_vendor_models
    ADD COLUMN IF NOT EXISTS onu_metrics JSONB NOT NULL DEFAULT '{}'::jsonb;

-- VSOL V1600G1: OIDs confirmados pelo utilizador (tabela + .PON.ONU)
UPDATE olt_vendor_models SET
  onu_metrics = '{
    "serial": {"enabled": true, "oid": "1.3.6.1.4.1.37950.1.1.6.1.1.2.1.5"},
    "status": {"enabled": true, "oid": "1.3.6.1.4.1.37950.1.1.6.1.1.1.1.5", "online_values": [3], "offline_values": [0, 1, 2, 4, 5, 6]},
    "vlan": {"enabled": false, "oid": "1.3.6.1.4.1.37950.1.1.6.1.1.7.5.8"},
    "rx_power": {"enabled": true, "oid": "1.3.6.1.4.1.37950.1.1.6.1.1.3.1.7"},
    "tx_power": {"enabled": true, "oid": "1.3.6.1.4.1.37950.1.1.6.1.1.3.1.6"},
    "temperature": {"enabled": true, "oid": "1.3.6.1.4.1.37950.1.1.6.1.1.3.1.3"},
    "model": {"enabled": true, "oid": "1.3.6.1.4.1.37950.1.1.6.1.1.2.1.6"}
  }'::jsonb,
  collection_steps = '[{"id":"onu_metrics","method":"onu_metrics_collect","enabled":true}]'::jsonb
WHERE brand = 'VSOL' AND model IN ('V1600G1', 'Padrão');

-- +goose Down
ALTER TABLE olt_vendor_models DROP COLUMN IF EXISTS onu_metrics;
