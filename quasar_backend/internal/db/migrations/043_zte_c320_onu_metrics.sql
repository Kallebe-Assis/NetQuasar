-- +goose Up
-- ZTE C320: ONUs não aparecem no IF-MIB (ifName só tem gpon_olt-1/1/N). Estado por MIB enterprise .ifIndex.ONU.
UPDATE olt_vendor_models SET
  onu_metrics = '{
    "status": {
      "enabled": true,
      "status_mode": "rx_power_threshold",
      "oid": "1.3.6.1.4.1.3902.1082.500.1.2.4.2.1.2",
      "value_divisor": 1000,
      "offline_rx_dbm": -70
    },
    "rx_power": {
      "enabled": true,
      "oid": "1.3.6.1.4.1.3902.1082.500.1.2.4.2.1.2",
      "value_divisor": 1000
    },
    "pon_status": {
      "enabled": true,
      "status_mode": "if_mib_index",
      "oid": "1.3.6.1.2.1.2.2.1.8",
      "ifdescr_oid": "1.3.6.1.2.1.2.2.1.2",
      "ifoper_oid": "1.3.6.1.2.1.2.2.1.8",
      "online_values": [1],
      "offline_values": [2]
    },
    "pon_tx_power": {
      "enabled": true,
      "oid": "1.3.6.1.4.1.3902.1082.30.40.2.4.1.3",
      "value_divisor": 1000
    },
    "pon_voltage": {
      "enabled": true,
      "oid": "1.3.6.1.4.1.3902.1082.30.40.2.4.1.5",
      "value_divisor": 1000
    },
    "pon_temperature": {
      "enabled": true,
      "oid": "1.3.6.1.4.1.3902.1082.30.40.2.4.1.8",
      "value_divisor": 1000
    }
  }'::jsonb,
  onu_online_oid = COALESCE(NULLIF(trim(onu_online_oid), ''), '1.3.6.1.4.1.3902.1082.500.2.2.6.3.1.1'),
  pon_status_oid = COALESCE(NULLIF(trim(pon_status_oid), ''), '1.3.6.1.2.1.2.2.1.8'),
  collection_steps = '[{"id":"onu_metrics","method":"onu_metrics_collect","enabled":true}]'::jsonb
WHERE brand = 'ZTE' AND model IN ('C320', 'Padrão');

-- +goose Down
UPDATE olt_vendor_models SET onu_metrics = '{}'::jsonb WHERE brand = 'ZTE' AND model IN ('C320', 'Padrão');
