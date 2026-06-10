-- +goose Up
CREATE TABLE client_connections (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    client_name TEXT NOT NULL,
    address TEXT,
    neighborhood TEXT,
    login TEXT NOT NULL,
    password TEXT,
    ip_address TEXT,
    connection_kind TEXT NOT NULL DEFAULT 'pppoe'
        CHECK (connection_kind IN ('pppoe', 'dhcp')),
    medium_type TEXT
        CHECK (medium_type IS NULL OR medium_type IN ('fibra', 'radio', 'cabo_utp')),
    sales_plan TEXT,
    onu_mac_sn TEXT,
    rx_dbm TEXT,
    tx_dbm TEXT,
    transmitter TEXT,
    cto TEXT,
    port TEXT,
    latitude DOUBLE PRECISION,
    longitude DOUBLE PRECISION,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_client_connections_login_lower ON client_connections (lower(trim(login)));
CREATE INDEX idx_client_connections_kind ON client_connections (connection_kind);
CREATE INDEX idx_client_connections_coords ON client_connections (latitude, longitude)
    WHERE latitude IS NOT NULL AND longitude IS NOT NULL;

-- +goose Down
DROP TABLE IF EXISTS client_connections;
