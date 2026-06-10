-- +goose Up
-- Ignorar alertas por equipamento + tipo + chave (ex.: PON) — persiste em BD e bloqueia UI/Telegram.

CREATE TABLE alert_ignores (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    alert_type TEXT NOT NULL,
    meta_key TEXT NOT NULL DEFAULT '',
    device_name TEXT,
    ip TEXT,
    severity TEXT,
    problem_title TEXT,
    last_message TEXT,
    last_meta JSONB NOT NULL DEFAULT '{}'::jsonb,
    reason TEXT,
    ignored_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ignored_by UUID REFERENCES users(id) ON DELETE SET NULL,
    source_alert_id UUID REFERENCES alert_instances(id) ON DELETE SET NULL,
    active BOOLEAN NOT NULL DEFAULT true,
    last_verified_at TIMESTAMPTZ,
    last_verify_result JSONB NOT NULL DEFAULT '{}'::jsonb,
    reactivated_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX idx_alert_ignores_active_unique
    ON alert_ignores (device_id, alert_type, meta_key)
    WHERE active = true;

CREATE INDEX idx_alert_ignores_active_list ON alert_ignores (ignored_at DESC) WHERE active = true;

-- +goose Down
DROP TABLE IF EXISTS alert_ignores;
