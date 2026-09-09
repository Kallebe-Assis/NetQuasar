-- +goose Up
ALTER TABLE monitoring_settings
    ADD COLUMN onu_rx_good_dbm NUMERIC NOT NULL DEFAULT -23,
    ADD COLUMN onu_rx_bad_dbm NUMERIC NOT NULL DEFAULT -27;

-- +goose Down
ALTER TABLE monitoring_settings
    DROP COLUMN IF EXISTS onu_rx_good_dbm,
    DROP COLUMN IF EXISTS onu_rx_bad_dbm;
