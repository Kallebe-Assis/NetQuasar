-- +goose Up
-- Agendamentos adicionais (resumo alertas, base comercial) + SMTP + incidentes correlacionados.

CREATE TABLE settings_smtp (
    id SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    enabled BOOLEAN NOT NULL DEFAULT false,
    host TEXT,
    port INT NOT NULL DEFAULT 587,
    username TEXT,
    password TEXT,
    from_address TEXT,
    use_tls BOOLEAN NOT NULL DEFAULT true,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO settings_smtp (id) VALUES (1);

CREATE TABLE automation_alerts_digest (
    id SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    enabled BOOLEAN NOT NULL DEFAULT false,
    frequency TEXT NOT NULL DEFAULT 'daily' CHECK (frequency IN ('daily', 'weekly')),
    day_of_week INT CHECK (day_of_week IS NULL OR (day_of_week >= 0 AND day_of_week <= 6)),
    time_hhmm TEXT NOT NULL DEFAULT '07:30' CHECK (time_hhmm ~ '^\d{2}:\d{2}$'),
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
INSERT INTO automation_alerts_digest (id) VALUES (1);

CREATE TABLE automation_commercial_report (
    id SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    enabled BOOLEAN NOT NULL DEFAULT false,
    day_of_month INT CHECK (day_of_month IS NULL OR (day_of_month >= 1 AND day_of_month <= 31)),
    time_hhmm TEXT NOT NULL DEFAULT '09:00' CHECK (time_hhmm ~ '^\d{2}:\d{2}$'),
    timezone TEXT NOT NULL DEFAULT 'America/Sao_Paulo',
    channel_telegram BOOLEAN NOT NULL DEFAULT true,
    channel_email BOOLEAN NOT NULL DEFAULT false,
    email_to TEXT,
    last_run_at TIMESTAMPTZ,
    last_run_period TEXT,
    last_status TEXT,
    last_error TEXT,
    running BOOLEAN NOT NULL DEFAULT false,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO automation_commercial_report (id) VALUES (1);

CREATE TABLE alert_incidents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'resolved')),
    root_cause TEXT NOT NULL CHECK (root_cause IN ('pop_offline', 'olt_offline', 'device_offline')),
    title TEXT NOT NULL,
    summary TEXT,
    pop_id UUID REFERENCES pops(id) ON DELETE SET NULL,
    root_device_id UUID REFERENCES devices(id) ON DELETE SET NULL,
    meta JSONB NOT NULL DEFAULT '{}'::jsonb,
    opened_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_alert_incidents_open ON alert_incidents (opened_at DESC) WHERE status = 'open';
CREATE INDEX idx_alert_incidents_pop ON alert_incidents (pop_id) WHERE status = 'open';

CREATE TABLE alert_incident_alerts (
    incident_id UUID NOT NULL REFERENCES alert_incidents(id) ON DELETE CASCADE,
    alert_id UUID NOT NULL REFERENCES alert_instances(id) ON DELETE CASCADE,
    role TEXT NOT NULL DEFAULT 'member' CHECK (role IN ('root', 'cascade', 'member')),
    linked_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (incident_id, alert_id)
);
CREATE UNIQUE INDEX idx_alert_incident_alerts_alert ON alert_incident_alerts (alert_id);

ALTER TABLE alert_instances ADD COLUMN IF NOT EXISTS incident_id UUID REFERENCES alert_incidents(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_alert_instances_incident ON alert_instances (incident_id) WHERE incident_id IS NOT NULL;

-- +goose Down
ALTER TABLE alert_instances DROP COLUMN IF EXISTS incident_id;
DROP TABLE IF EXISTS alert_incident_alerts CASCADE;
DROP TABLE IF EXISTS alert_incidents CASCADE;
DROP TABLE IF EXISTS automation_commercial_report CASCADE;
DROP TABLE IF EXISTS automation_alerts_digest CASCADE;
DROP TABLE IF EXISTS settings_smtp CASCADE;
