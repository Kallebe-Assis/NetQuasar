-- +goose Up
-- Placa deixa de ser obrigatória: permite cadastrar/importar veículos sem placa
-- (ex.: aguardando emplacamento). Índice único já trata múltiplos NULL como distintos.
ALTER TABLE fleet_vehicles ALTER COLUMN plate DROP NOT NULL;

-- +goose Down
UPDATE fleet_vehicles SET plate = 'SEM-' || upper(substr(replace(id::text, '-', ''), 1, 4)) WHERE plate IS NULL;
ALTER TABLE fleet_vehicles ALTER COLUMN plate SET NOT NULL;
