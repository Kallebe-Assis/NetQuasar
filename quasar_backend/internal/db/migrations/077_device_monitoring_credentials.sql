-- +goose Up
ALTER TABLE devices
    ADD COLUMN IF NOT EXISTS telnet_user TEXT,
    ADD COLUMN IF NOT EXISTS telnet_password TEXT,
    ADD COLUMN IF NOT EXISTS telnet_enable TEXT,
    ADD COLUMN IF NOT EXISTS ssh_user TEXT,
    ADD COLUMN IF NOT EXISTS ssh_password TEXT;

-- +goose Down
ALTER TABLE devices
    DROP COLUMN IF EXISTS telnet_user,
    DROP COLUMN IF EXISTS telnet_password,
    DROP COLUMN IF EXISTS telnet_enable,
    DROP COLUMN IF EXISTS ssh_user,
    DROP COLUMN IF EXISTS ssh_password;
