-- +goose Up
-- Tamanho de payload ICMP, limiar de falhas consecutivas para alertar offline,
-- e contagem persistente entre ciclos do worker.

ALTER TABLE monitoring_intervals
    ADD COLUMN IF NOT EXISTS icmp_payload_bytes INT NOT NULL DEFAULT 32
        CHECK (icmp_payload_bytes >= 0 AND icmp_payload_bytes <= 65507);

ALTER TABLE monitoring_intervals
    ADD COLUMN IF NOT EXISTS offline_ping_fail_threshold INT NOT NULL DEFAULT 3
        CHECK (offline_ping_fail_threshold >= 1 AND offline_ping_fail_threshold <= 50);

ALTER TABLE device_probe_cache
    ADD COLUMN IF NOT EXISTS ping_fail_streak INT NOT NULL DEFAULT 0
        CHECK (ping_fail_streak >= 0);

-- +goose Down
ALTER TABLE device_probe_cache DROP COLUMN IF EXISTS ping_fail_streak;
ALTER TABLE monitoring_intervals DROP COLUMN IF EXISTS offline_ping_fail_threshold;
ALTER TABLE monitoring_intervals DROP COLUMN IF EXISTS icmp_payload_bytes;
