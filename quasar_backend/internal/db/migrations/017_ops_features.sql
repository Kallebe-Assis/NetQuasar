-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS maintenance_windows (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title TEXT NOT NULL,
    scope_type TEXT NOT NULL CHECK (scope_type IN ('global', 'pop', 'device')),
    pop_id UUID REFERENCES pops(id) ON DELETE SET NULL,
    device_id UUID REFERENCES devices(id) ON DELETE SET NULL,
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ NOT NULL,
    checklist JSONB NOT NULL DEFAULT '[]'::jsonb,
    notes TEXT,
    status TEXT NOT NULL DEFAULT 'planned' CHECK (status IN ('planned', 'running', 'completed', 'cancelled')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_maintenance_windows_period ON maintenance_windows (starts_at, ends_at);
CREATE INDEX IF NOT EXISTS idx_maintenance_windows_scope_pop ON maintenance_windows (pop_id) WHERE pop_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_maintenance_windows_scope_device ON maintenance_windows (device_id) WHERE device_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS pop_contacts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    pop_id UUID NOT NULL REFERENCES pops(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    contact TEXT NOT NULL,
    shift_label TEXT,
    is_primary BOOLEAN NOT NULL DEFAULT false,
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_pop_contacts_pop ON pop_contacts (pop_id);

CREATE TABLE IF NOT EXISTS ops_audit_log (
    id BIGSERIAL PRIMARY KEY,
    entity_type TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    action TEXT NOT NULL,
    actor TEXT,
    before_data JSONB,
    after_data JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_ops_audit_entity ON ops_audit_log (entity_type, entity_id, created_at DESC);

CREATE TABLE IF NOT EXISTS nightly_collection_settings (
    id SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    enabled BOOLEAN NOT NULL DEFAULT false,
    run_time_hhmm TEXT NOT NULL DEFAULT '02:30' CHECK (run_time_hhmm ~ '^\d{2}:\d{2}$'),
    timezone TEXT NOT NULL DEFAULT 'America/Sao_Paulo',
    last_run_at TIMESTAMPTZ,
    last_status TEXT,
    last_summary JSONB,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO nightly_collection_settings (id) VALUES (1) ON CONFLICT (id) DO NOTHING;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS nightly_collection_settings;
DROP TABLE IF EXISTS ops_audit_log;
DROP TABLE IF EXISTS pop_contacts;
DROP TABLE IF EXISTS maintenance_windows;
-- +goose StatementEnd
