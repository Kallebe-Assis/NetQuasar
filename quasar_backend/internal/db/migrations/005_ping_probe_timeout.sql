-- +goose Up
-- Tempo máximo por sondagem ICMP+TCP fallback (milisegundos), usado pelo worker de ping.

ALTER TABLE monitoring_intervals
    ADD COLUMN IF NOT EXISTS ping_timeout_ms INTEGER NOT NULL DEFAULT 5500
        CHECK (ping_timeout_ms >= 1000 AND ping_timeout_ms <= 30000);

-- +goose Down
ALTER TABLE monitoring_intervals DROP COLUMN IF EXISTS ping_timeout_ms;
