-- Logins PPPoE já vistos no BNG + histórico de sessões (online/offline).
CREATE TABLE IF NOT EXISTS bng_known_logins (
    id BIGSERIAL PRIMARY KEY,
    device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    login TEXT NOT NULL,
    is_online BOOLEAN NOT NULL DEFAULT false,
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_offline_at TIMESTAMPTZ,
    session_index TEXT,
    vlan TEXT,
    ipv4 TEXT,
    ipv6 TEXT,
    ipv6_pd TEXT,
    mac TEXT,
    interface_name TEXT,
    domain TEXT,
    ip_type TEXT,
    ip_type_raw TEXT,
    car_up_cir_kbps TEXT,
    car_dn_cir_kbps TEXT,
    online_time_sec BIGINT,
    auth_state TEXT,
    current_event_id BIGINT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (device_id, login)
);

CREATE INDEX IF NOT EXISTS idx_bng_known_logins_device_online
    ON bng_known_logins (device_id, is_online, login);

CREATE INDEX IF NOT EXISTS idx_bng_known_logins_device_seen
    ON bng_known_logins (device_id, last_seen_at DESC);

CREATE TABLE IF NOT EXISTS bng_login_events (
    id BIGSERIAL PRIMARY KEY,
    device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    login TEXT NOT NULL,
    connected_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    disconnected_at TIMESTAMPTZ,
    duration_sec BIGINT,
    session_index TEXT,
    vlan TEXT,
    ipv4 TEXT,
    ipv6 TEXT,
    ipv6_pd TEXT,
    mac TEXT,
    interface_name TEXT,
    domain TEXT,
    ip_type TEXT,
    car_up_cir_kbps TEXT,
    car_dn_cir_kbps TEXT,
    online_time_sec BIGINT
);

CREATE INDEX IF NOT EXISTS idx_bng_login_events_device_login_time
    ON bng_login_events (device_id, login, connected_at DESC);

CREATE INDEX IF NOT EXISTS idx_bng_login_events_device_open
    ON bng_login_events (device_id, disconnected_at)
    WHERE disconnected_at IS NULL;

ALTER TABLE bng_known_logins
    DROP CONSTRAINT IF EXISTS bng_known_logins_current_event_id_fkey;
ALTER TABLE bng_known_logins
    ADD CONSTRAINT bng_known_logins_current_event_id_fkey
    FOREIGN KEY (current_event_id) REFERENCES bng_login_events(id) ON DELETE SET NULL;
