-- +goose Up
-- Rotula interfaces específicas de um equipamento BGP como pertencentes a uma operadora
-- (ex.: K2 no Eth-Trunk11, FORTE no Eth-Trunk10) — mesmo padrão de bng_uplink_interfaces
-- (116_bng_uplink_interfaces.sql). Usado para: (1) a tela BGP só somar no gráfico de tráfego
-- total as interfaces das operadoras configuradas (em vez de todas as interfaces walked),
-- (2) a futura exibição de tráfego por operadora. Casa por if_descr/if_name contra o relatório
-- pivotado mais recente (bgpcollect.BuildReportFromStoredMetrics), não por if_index (pode mudar
-- entre reinícios).
CREATE TABLE bgp_uplink_interfaces (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    device_id           UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    carrier_label       TEXT NOT NULL,      -- ex.: "K2", "FORTE"
    interface_label     TEXT NOT NULL,      -- ex.: "Uplink principal", "Membro Eth-Trunk10"
    if_descr            TEXT NOT NULL DEFAULT '',
    if_name             TEXT NOT NULL DEFAULT '',
    if_index_hint       INT,
    -- Evita somar tráfego em dobro: só a interface que representa o total real da operadora
    -- (o agregado Eth-Trunk, quando existir) conta para o gráfico; membros físicos ficam false.
    is_primary_traffic  BOOLEAN NOT NULL DEFAULT true,
    sort_order          INT NOT NULL DEFAULT 0,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_bgp_uplink_interfaces_device ON bgp_uplink_interfaces (device_id, sort_order);

-- +goose Down
DROP TABLE IF EXISTS bgp_uplink_interfaces;
