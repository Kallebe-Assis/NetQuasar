-- +goose Up
CREATE TABLE integrations (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            TEXT NOT NULL,
    slug            TEXT NOT NULL UNIQUE,
    description     TEXT,
    base_url        TEXT NOT NULL,
    enabled         BOOLEAN NOT NULL DEFAULT true,
    default_headers JSONB NOT NULL DEFAULT '{}'::jsonb,
    variables       JSONB NOT NULL DEFAULT '{}'::jsonb,
    auth_type       TEXT NOT NULL DEFAULT 'none'
        CHECK (auth_type IN ('none', 'bearer', 'basic', 'api_key', 'login')),
    auth_config     JSONB NOT NULL DEFAULT '{}'::jsonb,
    timeout_ms      INT NOT NULL DEFAULT 15000 CHECK (timeout_ms >= 1000 AND timeout_ms <= 120000),
    tls_insecure    BOOLEAN NOT NULL DEFAULT false,
    session_token   TEXT,
    session_expires_at TIMESTAMPTZ,
    last_test_at    TIMESTAMPTZ,
    last_test_ok    BOOLEAN,
    last_test_message TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_integrations_slug ON integrations (slug);
CREATE INDEX idx_integrations_enabled ON integrations (enabled);

CREATE TABLE integration_requests (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    integration_id  UUID NOT NULL REFERENCES integrations(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    description     TEXT,
    method          TEXT NOT NULL DEFAULT 'GET'
        CHECK (method IN ('GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'HEAD')),
    path            TEXT NOT NULL DEFAULT '/',
    path_params     JSONB NOT NULL DEFAULT '[]'::jsonb,
    query_params    JSONB NOT NULL DEFAULT '[]'::jsonb,
    headers         JSONB NOT NULL DEFAULT '{}'::jsonb,
    body_template   TEXT,
    body_type       TEXT NOT NULL DEFAULT 'json'
        CHECK (body_type IN ('none', 'json', 'form', 'text')),
    extract_json_path TEXT,
    is_login        BOOLEAN NOT NULL DEFAULT false,
    sort_order      INT NOT NULL DEFAULT 0,
    enabled         BOOLEAN NOT NULL DEFAULT true,
    last_run_at     TIMESTAMPTZ,
    last_run_ok     BOOLEAN,
    last_run_status INT,
    last_run_message TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_integration_requests_integration ON integration_requests (integration_id, sort_order);

CREATE TABLE integration_run_logs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    integration_id  UUID NOT NULL REFERENCES integrations(id) ON DELETE CASCADE,
    request_id      UUID REFERENCES integration_requests(id) ON DELETE SET NULL,
    run_kind        TEXT NOT NULL CHECK (run_kind IN ('test', 'login', 'request')),
    ok              BOOLEAN NOT NULL,
    status_code     INT,
    latency_ms      INT,
    request_url     TEXT,
    response_preview TEXT,
    error_message   TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_integration_run_logs_integration ON integration_run_logs (integration_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS integration_run_logs;
DROP TABLE IF EXISTS integration_requests;
DROP TABLE IF EXISTS integrations;
