-- +goose Up
-- Versão de hardware (cadastro), distinta de software_version (firmware/software).
ALTER TABLE devices
  ADD COLUMN IF NOT EXISTS hardware_version TEXT;

-- +goose Down
ALTER TABLE devices
  DROP COLUMN IF EXISTS hardware_version;
