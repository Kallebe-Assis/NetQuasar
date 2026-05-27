-- +goose Up
-- Perfil SNMP por marca + modelo (configuração OLT na UI).
CREATE TABLE IF NOT EXISTS olt_vendor_models (
    brand TEXT NOT NULL,
    model TEXT NOT NULL,
    onu_online_oid TEXT,
    pon_status_oid TEXT,
    transceiver_oid TEXT,
    snmp_base_oid TEXT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (brand, model)
);

CREATE INDEX IF NOT EXISTS idx_olt_vendor_models_brand ON olt_vendor_models (brand);

INSERT INTO olt_vendor_models (brand, model, onu_online_oid, pon_status_oid, transceiver_oid, snmp_base_oid)
SELECT brand, 'Padrão', onu_online_oid, pon_status_oid, transceiver_oid, snmp_base_oid
FROM olt_vendor_profiles
ON CONFLICT (brand, model) DO NOTHING;

INSERT INTO olt_vendor_models (brand, model) VALUES
    ('VSOL', 'V1600G1'),
    ('VSOL', 'V1600G2'),
    ('VSOL', 'V1600D4'),
    ('ZTE', 'C320'),
    ('ZTE', 'C300'),
    ('Datacom', 'DM4610'),
    ('Datacom', 'DM4615'),
    ('Huawei', 'MA5608T'),
    ('Huawei', 'MA5800'),
    ('Fiberhome', 'AN5516-01')
ON CONFLICT (brand, model) DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS olt_vendor_models;
