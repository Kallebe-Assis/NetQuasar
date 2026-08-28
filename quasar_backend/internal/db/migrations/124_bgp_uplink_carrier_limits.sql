-- +goose Up
-- Limite de banda por operadora (não por interface — uma operadora pode ter várias interfaces
-- em bgp_uplink_interfaces, o limite é dela como um todo, não de uma linha específica). Usado
-- pela tela BGP para definir o teto do eixo Y do gráfico de tráfego por operadora. Sempre
-- normalizado para Mbps (o formulário aceita Mbps ou Gbps e converte antes de gravar).
CREATE TABLE bgp_uplink_carrier_limits (
    device_id             UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    carrier_label         TEXT NOT NULL,
    bandwidth_limit_mbps  NUMERIC,
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (device_id, carrier_label)
);

-- +goose Down
DROP TABLE IF EXISTS bgp_uplink_carrier_limits;
