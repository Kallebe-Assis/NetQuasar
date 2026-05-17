-- +goose Up
CREATE TABLE IF NOT EXISTS olt_onu_samples (
    id BIGSERIAL PRIMARY KEY,
    device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    onu_total INT NOT NULL DEFAULT 0,
    onu_online INT NOT NULL DEFAULT 0,
    onu_offline INT NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_olt_onu_samples_device_time ON olt_onu_samples (device_id, recorded_at DESC);
CREATE INDEX IF NOT EXISTS idx_olt_onu_samples_recorded_at ON olt_onu_samples (recorded_at DESC);

-- +goose Down
DROP TABLE IF EXISTS olt_onu_samples;
