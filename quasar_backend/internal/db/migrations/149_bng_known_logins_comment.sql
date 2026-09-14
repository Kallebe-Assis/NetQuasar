-- +goose Up
-- Rótulo/nome do cliente por login — em equipamentos Mikrotik vem do comentário do /ppp secret
-- (não existe por SNMP; só telnet CLI dá acesso a isto). NULL continua a significar "sem
-- comentário configurado" (ex.: logins vindos de BNGs Huawei via SNMP, ou secrets sem comentário).
ALTER TABLE bng_known_logins
    ADD COLUMN IF NOT EXISTS comment TEXT;

-- +goose Down
ALTER TABLE bng_known_logins DROP COLUMN IF EXISTS comment;
