-- +goose Up
-- Cor própria de um cabo específico no mapa — sobrepõe a cor por função (network_cables.funcao /
-- Configurações → Mapa). NULL (padrão) continua a usar a cor da função, como sempre.
ALTER TABLE network_cables
    ADD COLUMN IF NOT EXISTS color TEXT;

-- +goose Down
ALTER TABLE network_cables DROP COLUMN IF EXISTS color;
