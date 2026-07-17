-- +goose Up
CREATE TABLE IF NOT EXISTS device_interface_metadata (
    device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    if_index INTEGER NOT NULL CHECK (if_index > 0),
    if_name TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    interface_type TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (device_id, if_index),
    CONSTRAINT device_interface_metadata_type_chk
        CHECK (interface_type IN ('', 'ether', 'sfp'))
);

CREATE INDEX IF NOT EXISTS idx_device_interface_metadata_device
    ON device_interface_metadata(device_id);

-- +goose Down
DROP TABLE IF EXISTS device_interface_metadata;
