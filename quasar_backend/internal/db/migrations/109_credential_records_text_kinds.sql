-- +goose Up
ALTER TABLE credential_records ADD COLUMN IF NOT EXISTS content TEXT;

ALTER TABLE credential_records ALTER COLUMN password_blob DROP NOT NULL;

ALTER TABLE credential_records DROP CONSTRAINT IF EXISTS credential_records_kind_check;
ALTER TABLE credential_records
    ADD CONSTRAINT credential_records_kind_check
    CHECK (kind IN ('equipment', 'server', 'site', 'orcamento', 'informacao', 'texto', 'outros'));

ALTER TABLE credential_records DROP CONSTRAINT IF EXISTS credential_records_kind_target;
ALTER TABLE credential_records
    ADD CONSTRAINT credential_records_kind_target CHECK (
        (kind = 'equipment' AND device_id IS NOT NULL)
        OR (kind = 'server' AND host IS NOT NULL AND length(trim(host)) > 0)
        OR (kind = 'site' AND domain IS NOT NULL AND length(trim(domain)) > 0)
        OR (kind IN ('orcamento', 'informacao', 'texto', 'outros')
            AND length(trim(title)) > 0
            AND content IS NOT NULL AND length(trim(content)) > 0)
    );

ALTER TABLE credential_records DROP CONSTRAINT IF EXISTS credential_records_password_required;
ALTER TABLE credential_records
    ADD CONSTRAINT credential_records_password_required CHECK (
        (kind IN ('equipment', 'server', 'site') AND password_blob IS NOT NULL)
        OR (kind IN ('orcamento', 'informacao', 'texto', 'outros'))
    );

-- +goose Down
ALTER TABLE credential_records DROP CONSTRAINT IF EXISTS credential_records_password_required;
ALTER TABLE credential_records DROP CONSTRAINT IF EXISTS credential_records_kind_target;
ALTER TABLE credential_records DROP CONSTRAINT IF EXISTS credential_records_kind_check;

ALTER TABLE credential_records
    ADD CONSTRAINT credential_records_kind_check
    CHECK (kind IN ('equipment', 'server', 'site'));

ALTER TABLE credential_records
    ADD CONSTRAINT credential_records_kind_target CHECK (
        (kind = 'equipment' AND device_id IS NOT NULL)
        OR (kind = 'server' AND host IS NOT NULL AND length(trim(host)) > 0)
        OR (kind = 'site' AND domain IS NOT NULL AND length(trim(domain)) > 0)
    );

DELETE FROM credential_records WHERE kind IN ('orcamento', 'informacao', 'texto', 'outros');

ALTER TABLE credential_records ALTER COLUMN password_blob SET NOT NULL;
ALTER TABLE credential_records DROP COLUMN IF EXISTS content;
