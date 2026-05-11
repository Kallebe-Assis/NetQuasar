-- +goose Up
-- Inventário SNMP por equipamento: walk MIB-II (1.3.6.1.2.1) + perfil para colectas (CPU, memória, temperatura, uptime).

CREATE TABLE IF NOT EXISTS device_snmp_inventory (
    device_id UUID PRIMARY KEY REFERENCES devices(id) ON DELETE CASCADE,
    discovered_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    root_oid TEXT NOT NULL DEFAULT '1.3.6.1.2.1',
    row_count INT NOT NULL DEFAULT 0,
    truncated BOOLEAN NOT NULL DEFAULT false,
    walk_rows JSONB NOT NULL DEFAULT '[]'::jsonb,
    walk_summary JSONB NOT NULL DEFAULT '{}'::jsonb,
    collect_profile JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX IF NOT EXISTS idx_device_snmp_inventory_discovered ON device_snmp_inventory (discovered_at DESC);

-- +goose Down
DROP TABLE IF EXISTS device_snmp_inventory;
