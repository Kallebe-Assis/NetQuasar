-- +goose Up
-- Automação: relatório agendado de totais BNG (PPPoE, IPv4, etc.).

CREATE TABLE automation_bng_stats_report (
    id SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    enabled BOOLEAN NOT NULL DEFAULT false,
    frequency TEXT NOT NULL DEFAULT 'daily' CHECK (frequency IN ('daily', 'weekly')),
    day_of_week INT CHECK (day_of_week IS NULL OR (day_of_week >= 0 AND day_of_week <= 6)),
    time_hhmm TEXT NOT NULL DEFAULT '08:00' CHECK (time_hhmm ~ '^\d{2}:\d{2}$'),
    timezone TEXT NOT NULL DEFAULT 'America/Sao_Paulo',
    channel_telegram BOOLEAN NOT NULL DEFAULT true,
    channel_email BOOLEAN NOT NULL DEFAULT false,
    email_to TEXT,
    last_run_at TIMESTAMPTZ,
    last_run_key TEXT,
    last_status TEXT,
    last_error TEXT,
    running BOOLEAN NOT NULL DEFAULT false,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO automation_bng_stats_report (id) VALUES (1);

-- +goose Down
DROP TABLE IF EXISTS automation_bng_stats_report CASCADE;
