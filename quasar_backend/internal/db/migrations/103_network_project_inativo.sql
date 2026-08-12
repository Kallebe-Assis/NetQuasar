-- +goose Up
ALTER TABLE network_projects DROP CONSTRAINT IF EXISTS network_projects_status_chk;
ALTER TABLE network_projects ADD CONSTRAINT network_projects_status_chk CHECK (
    status IN ('planejamento', 'em_andamento', 'concluido', 'pausado', 'cancelado', 'inativo')
);

-- +goose Down
ALTER TABLE network_projects DROP CONSTRAINT IF EXISTS network_projects_status_chk;
ALTER TABLE network_projects ADD CONSTRAINT network_projects_status_chk CHECK (
    status IN ('planejamento', 'em_andamento', 'concluido', 'pausado', 'cancelado')
);
