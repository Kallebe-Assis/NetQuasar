-- +goose Up
-- Histórico unificado de execuções automáticas (relatórios agendados).

CREATE TABLE automation_execution_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_type TEXT NOT NULL,
    actor TEXT NOT NULL DEFAULT 'scheduler',
    trigger_type TEXT NOT NULL DEFAULT 'scheduled',
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ok BOOLEAN NOT NULL DEFAULT false,
    status_message TEXT NOT NULL DEFAULT '',
    error_message TEXT,
    summary JSONB NOT NULL DEFAULT '{}'::jsonb,
    run_key TEXT
);

CREATE INDEX idx_automation_execution_log_started ON automation_execution_log (started_at DESC);
CREATE INDEX idx_automation_execution_log_job ON automation_execution_log (job_type, started_at DESC);

-- +goose Down
DROP TABLE IF EXISTS automation_execution_log;
