-- +goose Up
-- Superfície de dados alinhada ao README (históricos, alertas, telemetria, jobs, eventos).

CREATE TABLE ping_history (
    id BIGSERIAL PRIMARY KEY,
    device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    checked_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ok BOOLEAN NOT NULL,
    latency_ms BIGINT,
    method TEXT,
    source TEXT NOT NULL DEFAULT 'worker',
    detail JSONB NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX idx_ping_history_device_time ON ping_history (device_id, checked_at DESC);

CREATE TABLE alert_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT true,
    condition_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    channels_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE alert_instances (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    device_id UUID REFERENCES devices(id) ON DELETE SET NULL,
    severity TEXT NOT NULL CHECK (severity IN ('critical', 'warning', 'info')),
    alert_type TEXT NOT NULL,
    message TEXT NOT NULL,
    ip TEXT,
    device_name TEXT,
    active_since TIMESTAMPTZ NOT NULL DEFAULT now(),
    closed_at TIMESTAMPTZ,
    meta JSONB NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX idx_alert_instances_open ON alert_instances (active_since DESC) WHERE closed_at IS NULL;
CREATE INDEX idx_alert_instances_device ON alert_instances (device_id);

CREATE TABLE telemetry_samples (
    id BIGSERIAL PRIMARY KEY,
    device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    collected_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    metrics JSONB NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX idx_telemetry_samples_device_time ON telemetry_samples (device_id, collected_at DESC);

CREATE TABLE interface_snapshots (
    id BIGSERIAL PRIMARY KEY,
    device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    collected_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    interfaces JSONB NOT NULL DEFAULT '[]'::jsonb
);
CREATE INDEX idx_interface_snapshots_device_time ON interface_snapshots (device_id, collected_at DESC);

CREATE TABLE olt_snapshots (
    device_id UUID PRIMARY KEY REFERENCES devices(id) ON DELETE CASCADE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    summary JSONB NOT NULL DEFAULT '{}'::jsonb,
    pons JSONB NOT NULL DEFAULT '[]'::jsonb
);

CREATE TABLE snmp_walk_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    device_id UUID REFERENCES devices(id) ON DELETE SET NULL,
    host TEXT,
    community TEXT,
    scope TEXT,
    status TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'done', 'failed')),
    result JSONB,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at TIMESTAMPTZ
);
CREATE INDEX idx_snmp_walk_jobs_created ON snmp_walk_jobs (created_at DESC);

CREATE TABLE onu_report_runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at TIMESTAMPTZ,
    status TEXT NOT NULL,
    error_message TEXT,
    summary JSONB NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX idx_onu_report_runs_started ON onu_report_runs (started_at DESC);

CREATE TABLE events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    event_type TEXT NOT NULL,
    severity TEXT,
    device_id UUID REFERENCES devices(id) ON DELETE SET NULL,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX idx_events_created ON events (created_at DESC);

CREATE TABLE bng_session_snapshots (
    id BIGSERIAL PRIMARY KEY,
    captured_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    label TEXT NOT NULL DEFAULT 'stub',
    data JSONB NOT NULL DEFAULT '[]'::jsonb
);

-- +goose Down
DROP TABLE IF EXISTS bng_session_snapshots;
DROP TABLE IF EXISTS events;
DROP TABLE IF EXISTS onu_report_runs;
DROP TABLE IF EXISTS snmp_walk_jobs;
DROP TABLE IF EXISTS olt_snapshots;
DROP TABLE IF EXISTS interface_snapshots;
DROP TABLE IF EXISTS telemetry_samples;
DROP TABLE IF EXISTS alert_instances;
DROP TABLE IF EXISTS alert_rules;
DROP TABLE IF EXISTS ping_history;
