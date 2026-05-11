-- +goose Up
CREATE TABLE IF NOT EXISTS oid_definitions (
    oid_base TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    category TEXT NOT NULL,
    description TEXT,
    vendor TEXT,
    unit TEXT,
    mib TEXT,
    data_type TEXT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS discovered_oids (
    id BIGSERIAL PRIMARY KEY,
    equipment_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    oid TEXT NOT NULL,
    normalized_oid TEXT NOT NULL,
    value TEXT,
    type TEXT,
    category TEXT NOT NULL DEFAULT 'other',
    last_seen TIMESTAMPTZ NOT NULL DEFAULT now(),
    vendor TEXT,
    model TEXT
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_discovered_oids_equipment_oid ON discovered_oids (equipment_id, oid);
CREATE INDEX IF NOT EXISTS idx_discovered_oids_norm ON discovered_oids (normalized_oid);
CREATE INDEX IF NOT EXISTS idx_discovered_oids_last_seen ON discovered_oids (last_seen DESC);

CREATE TABLE IF NOT EXISTS snmp_profiles (
    id BIGSERIAL PRIMARY KEY,
    vendor TEXT NOT NULL,
    model TEXT NOT NULL,
    cpu_oid TEXT,
    temp_oid TEXT,
    memory_used_oid TEXT,
    memory_size_oid TEXT,
    uptime_oid TEXT,
    interface_oid TEXT,
    rx_oid TEXT,
    tx_oid TEXT,
    pon_oid TEXT,
    onu_oid TEXT,
    optical_oid TEXT,
    source TEXT NOT NULL DEFAULT 'auto_discovery',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (vendor, model)
);

INSERT INTO oid_definitions (oid_base, name, category, description, vendor, unit, mib, data_type) VALUES
('1.3.6.1.2.1.1.3.0', 'sysUpTime', 'uptime', 'Uptime do sistema', NULL, 'ticks', 'SNMPv2-MIB', 'TimeTicks'),
('1.3.6.1.2.1.1.5.0', 'sysName', 'system', 'Hostname do equipamento', NULL, NULL, 'SNMPv2-MIB', 'OctetString'),
('1.3.6.1.2.1.25.3.3.1.2', 'hrProcessorLoad', 'cpu', 'Carga de CPU por processador', NULL, '%', 'HOST-RESOURCES-MIB', 'Integer'),
('1.3.6.1.4.1.2021.4.5.0', 'memTotalReal', 'memory', 'Memória total (UCD-SNMP)', NULL, 'kB', 'UCD-SNMP-MIB', 'Integer'),
('1.3.6.1.4.1.2021.4.6.0', 'memAvailReal', 'memory', 'Memória disponível (UCD-SNMP)', NULL, 'kB', 'UCD-SNMP-MIB', 'Integer'),
('1.3.6.1.2.1.2.2.1.8', 'ifOperStatus', 'interfaces', 'Status operacional da interface', NULL, NULL, 'IF-MIB', 'Integer'),
('1.3.6.1.2.1.31.1.1.1.6', 'ifHCInOctets', 'traffic_rx', 'Tráfego RX (64-bit)', NULL, 'octets', 'IF-MIB', 'Counter64'),
('1.3.6.1.2.1.31.1.1.1.10', 'ifHCOutOctets', 'traffic_tx', 'Tráfego TX (64-bit)', NULL, 'octets', 'IF-MIB', 'Counter64'),
('1.3.6.1.4.1.14988.1.1.3.10.0', 'mtxrHlCpuLoad', 'cpu', 'CPU MikroTik', 'mikrotik', '%', 'MIKROTIK-MIB', 'Integer'),
('1.3.6.1.4.1.14988.1.1.3.14.0', 'mtxrHlTemperature', 'temperature', 'Temperatura MikroTik', 'mikrotik', 'C', 'MIKROTIK-MIB', 'Integer'),
('1.3.6.1.2.1.99.1.1.1.4', 'entPhySensorValue', 'temperature', 'Leitura de sensor físico', NULL, NULL, 'ENTITY-SENSOR-MIB', 'Integer'),
('1.3.6.1.4.1.9.9.13.1.3.1.3', 'ciscoEnvMonTemperatureStatusValue', 'temperature', 'Temperatura Cisco', 'cisco', 'C', 'CISCO-ENVMON-MIB', 'Integer')
ON CONFLICT (oid_base) DO UPDATE SET
    name = EXCLUDED.name,
    category = EXCLUDED.category,
    description = EXCLUDED.description,
    vendor = EXCLUDED.vendor,
    unit = EXCLUDED.unit,
    mib = EXCLUDED.mib,
    data_type = EXCLUDED.data_type,
    updated_at = now();

-- +goose Down
DROP TABLE IF EXISTS snmp_profiles;
DROP TABLE IF EXISTS discovered_oids;
DROP TABLE IF EXISTS oid_definitions;
