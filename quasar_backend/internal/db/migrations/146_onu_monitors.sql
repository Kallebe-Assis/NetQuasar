-- +goose Up
-- Monitoramento manual/temporário de uma ONU específica (tela OLT → ONUs → "Monitorar").
-- O sistema passa a acompanhar essa ONU e a levantar alertas (aba "ONUs" da tela de Alertas)
-- quando o status (online/offline) e/ou a potência RX degradam e quando normalizam. Cada
-- sub-monitor é opcional. Pode-se vincular um login PPPoE para o sistema confirmar, na
-- normalização, se esse login está online no BNG (via bng_known_logins).
CREATE TABLE IF NOT EXISTS onu_monitors (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    serial          TEXT NOT NULL,
    olt_device_id   UUID REFERENCES devices(id) ON DELETE CASCADE,
    olt_description TEXT NOT NULL DEFAULT '',
    pon             INT,
    onu             INT,
    client_name     TEXT NOT NULL DEFAULT '',
    watch_status    BOOLEAN NOT NULL DEFAULT true,
    watch_rx        BOOLEAN NOT NULL DEFAULT true,
    watch_login     BOOLEAN NOT NULL DEFAULT false,
    bng_login       TEXT NOT NULL DEFAULT '',
    notify_telegram BOOLEAN NOT NULL DEFAULT true,
    -- NULL = monitora até remoção manual; senão expira nesse instante.
    expires_at      TIMESTAMPTZ,
    -- Baseline do estado observado (para detectar transições sem falso-alarme).
    last_online       BOOLEAN,
    last_rx_dbm       NUMERIC,
    last_rx_class     TEXT,
    last_evaluated_at TIMESTAMPTZ,
    created_by      TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (serial)
);

CREATE INDEX IF NOT EXISTS idx_onu_monitors_expires ON onu_monitors (expires_at);
CREATE INDEX IF NOT EXISTS idx_onu_monitors_olt ON onu_monitors (olt_device_id);

-- +goose Down
DROP TABLE IF EXISTS onu_monitors;
