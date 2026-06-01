-- +goose Up
CREATE TABLE IF NOT EXISTS olt_onu_optical_streak (
    device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    pon_key TEXT NOT NULL,
    metric_id TEXT NOT NULL,
    streak INT NOT NULL DEFAULT 0 CHECK (streak >= 0),
    last_value DOUBLE PRECISION,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (device_id, pon_key, metric_id)
);

-- +goose Down
DROP TABLE IF EXISTS olt_onu_optical_streak;
