-- +goose Up
ALTER TABLE devices
    ADD COLUMN IF NOT EXISTS max_pons INTEGER;

ALTER TABLE devices
    DROP CONSTRAINT IF EXISTS devices_max_pons_positive;

ALTER TABLE devices
    ADD CONSTRAINT devices_max_pons_positive
    CHECK (max_pons IS NULL OR max_pons >= 1);

-- +goose Down
ALTER TABLE devices
    DROP CONSTRAINT IF EXISTS devices_max_pons_positive;

ALTER TABLE devices
    DROP COLUMN IF EXISTS max_pons;
