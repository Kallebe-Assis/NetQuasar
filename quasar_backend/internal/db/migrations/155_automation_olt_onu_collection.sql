-- +goose Up
-- Automação: coleta em segundo plano dos dados das ONUs de todas as OLTs, em duas cadências —
-- "leve" (status + RX, frequente) e "completa" (serial, temperatura, TX, telnet…, espaçada).
-- Existe porque a coleta completa só corria manualmente (olt_full_collect_seconds = 0 por padrão)
-- e a linha-base periódica nunca lê serial, então ONUs novas ficavam sem serial indefinidamente.

CREATE TABLE automation_olt_onu_collection (
    id SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    enabled BOOLEAN NOT NULL DEFAULT false,
    light_enabled BOOLEAN NOT NULL DEFAULT true,
    light_interval_minutes INT NOT NULL DEFAULT 5 CHECK (light_interval_minutes BETWEEN 1 AND 1440),
    full_enabled BOOLEAN NOT NULL DEFAULT true,
    full_interval_minutes INT NOT NULL DEFAULT 360 CHECK (full_interval_minutes BETWEEN 15 AND 10080),
    last_light_at TIMESTAMPTZ,
    last_full_at TIMESTAMPTZ,
    last_run_at TIMESTAMPTZ,
    last_status TEXT,
    last_error TEXT,
    last_light_summary JSONB,
    last_full_summary JSONB,
    running_light BOOLEAN NOT NULL DEFAULT false,
    running_full BOOLEAN NOT NULL DEFAULT false,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO automation_olt_onu_collection (id) VALUES (1);

-- +goose Down
DROP TABLE IF EXISTS automation_olt_onu_collection CASCADE;
