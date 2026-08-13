-- +goose Up
CREATE TABLE IF NOT EXISTS credential_records (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    kind TEXT NOT NULL CHECK (kind IN ('equipment', 'server', 'site')),
    title TEXT NOT NULL DEFAULT '',
    device_id UUID REFERENCES devices(id) ON DELETE SET NULL,
    host TEXT,
    domain TEXT,
    username TEXT,
    password_blob BYTEA NOT NULL,
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT credential_records_kind_target CHECK (
        (kind = 'equipment' AND device_id IS NOT NULL)
        OR (kind = 'server' AND host IS NOT NULL AND length(trim(host)) > 0)
        OR (kind = 'site' AND domain IS NOT NULL AND length(trim(domain)) > 0)
    )
);

CREATE INDEX IF NOT EXISTS credential_records_owner_idx ON credential_records (owner_user_id);
CREATE INDEX IF NOT EXISTS credential_records_kind_idx ON credential_records (kind);
CREATE INDEX IF NOT EXISTS credential_records_device_idx ON credential_records (device_id)
    WHERE device_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS credential_record_reveals (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    record_id UUID NOT NULL REFERENCES credential_records(id) ON DELETE CASCADE,
    revealed_by UUID REFERENCES users(id) ON DELETE SET NULL,
    revealed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS credential_record_reveals_record_idx
    ON credential_record_reveals (record_id, revealed_at DESC);

-- +goose Down
DROP TABLE IF EXISTS credential_record_reveals;
DROP TABLE IF EXISTS credential_records;
