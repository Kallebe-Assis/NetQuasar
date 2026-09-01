-- +goose Up
-- Cadastro de operadoras (fornecedores de link BGP) — CNPJ, endereço, 1+ AS (Autonomous System)
-- e limite de banda contratado. Substitui o campo livre bgp_uplink_interfaces.carrier_label
-- (texto digitado à mão) por um FK para esta tabela, e MOVE o limite de banda que hoje vive em
-- bgp_uplink_carrier_limits (por device+label) para cá — a operadora é uma entidade real (mesmo
-- CNPJ, mesmo AS), o limite contratado é dela, não de um par device+rótulo. Global (sem
-- device_id): a mesma operadora pode servir mais de um equipamento.
CREATE TABLE bgp_carriers (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name                   TEXT NOT NULL,
    document               TEXT NOT NULL DEFAULT '', -- CNPJ
    address                TEXT NOT NULL DEFAULT '',
    bandwidth_limit_mbps   NUMERIC,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX idx_bgp_carriers_name ON bgp_carriers (lower(name));

-- Uma operadora pode ter mais de um AS (ex.: um para cada bloco/upstream).
CREATE TABLE bgp_carrier_as_numbers (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    carrier_id   UUID NOT NULL REFERENCES bgp_carriers(id) ON DELETE CASCADE,
    as_number    BIGINT NOT NULL,
    sort_order   INT NOT NULL DEFAULT 0
);
CREATE INDEX idx_bgp_carrier_as_numbers_carrier ON bgp_carrier_as_numbers (carrier_id);

-- Backfill: 1 operadora por carrier_label distinto já em uso (interfaces já configuradas e/ou
-- limites já configurados), herdando o limite de banda existente quando houver.
INSERT INTO bgp_carriers (name, bandwidth_limit_mbps)
SELECT DISTINCT ON (lower(lbl)) lbl, lim.bandwidth_limit_mbps
FROM (
    SELECT carrier_label AS lbl FROM bgp_uplink_interfaces
    UNION
    SELECT carrier_label AS lbl FROM bgp_uplink_carrier_limits
) x
LEFT JOIN bgp_uplink_carrier_limits lim ON lower(lim.carrier_label) = lower(x.lbl)
ORDER BY lower(lbl), lim.updated_at DESC NULLS LAST;

ALTER TABLE bgp_uplink_interfaces ADD COLUMN carrier_id UUID REFERENCES bgp_carriers(id) ON DELETE CASCADE;
UPDATE bgp_uplink_interfaces u SET carrier_id = c.id FROM bgp_carriers c WHERE lower(c.name) = lower(u.carrier_label);
ALTER TABLE bgp_uplink_interfaces ALTER COLUMN carrier_id SET NOT NULL;
ALTER TABLE bgp_uplink_interfaces DROP COLUMN carrier_label;
CREATE INDEX idx_bgp_uplink_interfaces_carrier ON bgp_uplink_interfaces (carrier_id);

DROP TABLE bgp_uplink_carrier_limits;

-- +goose Down
CREATE TABLE bgp_uplink_carrier_limits (
    device_id             UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    carrier_label         TEXT NOT NULL,
    bandwidth_limit_mbps  NUMERIC,
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (device_id, carrier_label)
);
INSERT INTO bgp_uplink_carrier_limits (device_id, carrier_label, bandwidth_limit_mbps)
SELECT DISTINCT u.device_id, c.name, c.bandwidth_limit_mbps
FROM bgp_uplink_interfaces u
JOIN bgp_carriers c ON c.id = u.carrier_id;

ALTER TABLE bgp_uplink_interfaces ADD COLUMN carrier_label TEXT;
UPDATE bgp_uplink_interfaces u SET carrier_label = c.name FROM bgp_carriers c WHERE c.id = u.carrier_id;
ALTER TABLE bgp_uplink_interfaces ALTER COLUMN carrier_label SET NOT NULL;
DROP INDEX IF EXISTS idx_bgp_uplink_interfaces_carrier;
ALTER TABLE bgp_uplink_interfaces DROP COLUMN carrier_id;

DROP TABLE bgp_carrier_as_numbers;
DROP TABLE bgp_carriers;
