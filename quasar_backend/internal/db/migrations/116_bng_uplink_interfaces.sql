-- +goose Up
-- Rotula interfaces específicas de um equipamento BNG como "uplink de operadora" (ex.: K2 na
-- interface 0/1/6, FORTE no Eth-Trunk10) para poderem ter tráfego (ifHCInOctets/ifHCOutOctets,
-- IF-MIB padrão — confirmado no MIB reference do Huawei NE8000 M8) monitorado e mostrado na
-- aba Relatório da tela de BNG. Casa por if_descr/if_name (não só por if_index, que pode mudar
-- entre reinícios) contra o snapshot mais recente de interface_snapshots.
CREATE TABLE bng_uplink_interfaces (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    device_id           UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    carrier_label       TEXT NOT NULL,      -- ex.: "K2", "FORTE"
    interface_label     TEXT NOT NULL,      -- ex.: "Uplink principal", "Membro Eth-Trunk10 (0/1/7)"
    if_descr            TEXT NOT NULL DEFAULT '',
    if_name             TEXT NOT NULL DEFAULT '',
    if_index_hint        INT,                -- último ifIndex resolvido — acelera o casamento, não é a chave
    -- Marca a interface cujo contador já representa o total da operadora (o agregado
    -- Eth-Trunk, ou a física quando não há trunk) — soma-se ao total; membros físicos de um
    -- trunk (só para inspeção/distribuição de carga) ficam com is_primary_traffic = false para
    -- não contar tráfego em dobro.
    is_primary_traffic  BOOLEAN NOT NULL DEFAULT true,
    sort_order          INT NOT NULL DEFAULT 0,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_bng_uplink_interfaces_device ON bng_uplink_interfaces (device_id, sort_order);

-- +goose Down
DROP TABLE IF EXISTS bng_uplink_interfaces;
