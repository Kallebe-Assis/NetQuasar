-- +goose Up
-- Múltiplos IPs por equipamento: devices.ip continua a ser o IP primário/padrão, usado sem
-- alteração em todo o resto do sistema (relatórios, busca, telnet, sniffer, etc — dezenas de
-- pontos). device_ips guarda só IPs EXTRA, cada um com descrição, se entra no ciclo de ping
-- (monitored) e para que propósito serve (for_telemetry/for_bng/for_bgp) — usado pelos
-- coletores periódicos para escolher qual IP falar em vez do primário, quando marcado.
CREATE TABLE device_ips (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    device_id     UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    ip            INET NOT NULL,
    description   TEXT,
    monitored     BOOLEAN NOT NULL DEFAULT true,
    for_telemetry BOOLEAN NOT NULL DEFAULT false,
    for_bng       BOOLEAN NOT NULL DEFAULT false,
    for_bgp       BOOLEAN NOT NULL DEFAULT false,
    sort_order    INT NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (device_id, ip)
);
CREATE INDEX idx_device_ips_device ON device_ips (device_id, sort_order);
-- Um só IP por propósito por equipamento — evita ambiguidade nos loaders de coleta.
CREATE UNIQUE INDEX idx_device_ips_one_telemetry ON device_ips (device_id) WHERE for_telemetry;
CREATE UNIQUE INDEX idx_device_ips_one_bng ON device_ips (device_id) WHERE for_bng;
CREATE UNIQUE INDEX idx_device_ips_one_bgp ON device_ips (device_id) WHERE for_bgp;

-- Estado de ping por IP extra (mesma lógica de streak do device_probe_cache, mas por IP —
-- device_probe_cache/ping_history do IP primário não mudam em nada).
CREATE TABLE device_ip_probe_state (
    device_ip_id UUID PRIMARY KEY REFERENCES device_ips(id) ON DELETE CASCADE,
    ok           BOOLEAN,
    fail_streak  INT NOT NULL DEFAULT 0,
    latency_ms   INT,
    checked_at   TIMESTAMPTZ
);

-- Lógica de combinação quando há 2+ IPs monitorados (primário + extras): 'any' (padrão) =
-- offline se qualquer IP monitorado falhar — idêntico ao comportamento actual de sempre
-- quando só há 1 IP monitorado; 'all' = offline só quando TODOS falharem.
ALTER TABLE devices
    ADD COLUMN offline_alert_logic TEXT NOT NULL DEFAULT 'any'
        CHECK (offline_alert_logic IN ('any', 'all'));

-- +goose Down
ALTER TABLE devices DROP COLUMN IF EXISTS offline_alert_logic;
DROP TABLE IF EXISTS device_ip_probe_state;
DROP TABLE IF EXISTS device_ips;
