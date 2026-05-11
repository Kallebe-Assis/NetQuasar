-- +goose Up
-- +goose StatementBegin
CREATE EXTENSION IF NOT EXISTS "pgcrypto";
-- +goose StatementEnd

CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    login TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('admin', 'user')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE pops (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    description TEXT NOT NULL,
    address TEXT,
    latitude DOUBLE PRECISION,
    longitude DOUBLE PRECISION,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE devices (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    pop_id UUID REFERENCES pops(id) ON DELETE SET NULL,
    category TEXT NOT NULL,
    description TEXT NOT NULL,
    ip INET,
    network_status TEXT NOT NULL DEFAULT 'Normal' CHECK (network_status IN ('Bridge', 'Normal')),
    access_mode TEXT,
    telemetry_mode TEXT,
    ping_enabled BOOLEAN NOT NULL DEFAULT true,
    telemetry_enabled BOOLEAN NOT NULL DEFAULT false,
    operational_mode TEXT NOT NULL DEFAULT 'Ativo' CHECK (operational_mode IN ('Ativo', 'Inativo', 'Manutenção', 'Reserva')),
    latitude DOUBLE PRECISION,
    longitude DOUBLE PRECISION,
    brand TEXT,
    model TEXT,
    mac TEXT,
    serial_number TEXT,
    software_version TEXT,
    acquired_at DATE,
    locality_id UUID,
    snmp_community TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT telemetry_requires_ping CHECK (NOT telemetry_enabled OR ping_enabled)
);

CREATE INDEX idx_devices_pop ON devices(pop_id);
CREATE INDEX idx_devices_category ON devices(category);

CREATE TABLE monitoring_intervals (
    id SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    ping_seconds INT NOT NULL DEFAULT 30 CHECK (ping_seconds >= 30),
    telemetry_minutes INT NOT NULL DEFAULT 2 CHECK (telemetry_minutes >= 2),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO monitoring_intervals (id) VALUES (1);

CREATE TABLE monitoring_runtime (
    id SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    is_running BOOLEAN NOT NULL DEFAULT false,
    last_started_at TIMESTAMPTZ,
    last_stopped_at TIMESTAMPTZ,
    last_internet_check_at TIMESTAMPTZ,
    last_internet_check_ok BOOLEAN,
    last_internet_check_detail JSONB,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO monitoring_runtime (id) VALUES (1);

CREATE TABLE monitoring_settings (
    id SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    vps_latency_offset_ms INT NOT NULL DEFAULT 0 CHECK (vps_latency_offset_ms >= 0),
    internet_check_targets JSONB NOT NULL DEFAULT '["https://1.1.1.1","https://www.google.com/generate_204"]'::jsonb,
    internet_check_timeout_ms INT NOT NULL DEFAULT 3500 CHECK (internet_check_timeout_ms > 0 AND internet_check_timeout_ms <= 30000),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO monitoring_settings (id) VALUES (1);

CREATE TABLE settings_connection_defaults (
    id SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    snmp_community TEXT,
    telnet_user TEXT,
    telnet_password TEXT,
    telnet_enable TEXT,
    ssh_user TEXT,
    ssh_password TEXT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO settings_connection_defaults (id) VALUES (1);

CREATE TABLE settings_telegram (
    id TEXT PRIMARY KEY CHECK (id IN ('monitoring', 'reports')),
    bot_token TEXT,
    chat_id TEXT,
    topic_id TEXT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE settings_database_meta (
    id SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    host TEXT,
    port INT,
    db_user TEXT,
    db_name TEXT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE commercial_localities (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL UNIQUE,
    region_code TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE commercial_monthly_records (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    locality_id UUID NOT NULL REFERENCES commercial_localities(id) ON DELETE CASCADE,
    year_month TEXT NOT NULL CHECK (year_month ~ '^\d{4}-\d{2}$'),
    client_count INT NOT NULL CHECK (client_count >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (locality_id, year_month)
);

CREATE TABLE alert_suppressions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    scope_type TEXT NOT NULL,
    scope_ref TEXT NOT NULL,
    reason TEXT NOT NULL,
    created_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE automation_onu_report (
    id SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    enabled BOOLEAN NOT NULL DEFAULT false,
    mode TEXT NOT NULL DEFAULT 'disabled' CHECK (mode IN ('monthly', 'weekly', 'disabled')),
    day_of_month INT CHECK (day_of_month IS NULL OR (day_of_month >= 1 AND day_of_month <= 31)),
    day_of_week INT CHECK (day_of_week IS NULL OR (day_of_week >= 0 AND day_of_week <= 6)),
    time_hhmm TEXT NOT NULL DEFAULT '08:00' CHECK (time_hhmm ~ '^\d{2}:\d{2}$'),
    last_run_at TIMESTAMPTZ,
    last_status TEXT,
    last_error TEXT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO automation_onu_report (id) VALUES (1);

CREATE TABLE olt_vendor_profiles (
    brand TEXT PRIMARY KEY,
    onu_online_oid TEXT,
    pon_status_oid TEXT,
    transceiver_oid TEXT,
    snmp_base_oid TEXT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO olt_vendor_profiles (brand) VALUES
    ('ZTE'), ('VSOL'), ('Huawei'), ('Nokia'), ('Datacom')
ON CONFLICT (brand) DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS olt_vendor_profiles CASCADE;
DROP TABLE IF EXISTS automation_onu_report CASCADE;
DROP TABLE IF EXISTS alert_suppressions CASCADE;
DROP TABLE IF EXISTS commercial_monthly_records CASCADE;
DROP TABLE IF EXISTS commercial_localities CASCADE;
DROP TABLE IF EXISTS settings_database_meta CASCADE;
DROP TABLE IF EXISTS settings_telegram CASCADE;
DROP TABLE IF EXISTS settings_connection_defaults CASCADE;
DROP TABLE IF EXISTS monitoring_settings CASCADE;
DROP TABLE IF EXISTS monitoring_runtime CASCADE;
DROP TABLE IF EXISTS monitoring_intervals CASCADE;
DROP TABLE IF EXISTS devices CASCADE;
DROP TABLE IF EXISTS pops CASCADE;
DROP TABLE IF EXISTS users CASCADE;
