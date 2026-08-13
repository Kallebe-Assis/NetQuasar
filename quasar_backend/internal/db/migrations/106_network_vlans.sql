-- +goose Up
CREATE TABLE IF NOT EXISTS network_vlans (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    vlan_id TEXT NOT NULL,
    name TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    kind TEXT NOT NULL DEFAULT 'pppoe'
        CHECK (kind IN ('pppoe', 'gerencia', 'transporte')),
    status TEXT NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'inactive')),
    capacity INTEGER,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT network_vlans_vlan_id_unique UNIQUE (vlan_id),
    CONSTRAINT network_vlans_vlan_id_nonempty CHECK (length(trim(vlan_id)) > 0),
    CONSTRAINT network_vlans_capacity_chk CHECK (capacity IS NULL OR capacity > 0)
);

CREATE INDEX IF NOT EXISTS network_vlans_kind_idx ON network_vlans (kind);
CREATE INDEX IF NOT EXISTS network_vlans_status_idx ON network_vlans (status);
CREATE INDEX IF NOT EXISTS bng_known_logins_vlan_idx ON bng_known_logins (vlan)
    WHERE vlan IS NOT NULL AND trim(vlan) <> '';

-- +goose Down
DROP INDEX IF EXISTS bng_known_logins_vlan_idx;
DROP TABLE IF EXISTS network_vlans;
