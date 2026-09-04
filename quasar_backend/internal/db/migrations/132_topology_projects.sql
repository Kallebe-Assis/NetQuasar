-- +goose Up
-- Substitui o documento único (singleton) de topologia por N projectos nomeados — pedido do
-- utilizador: "o sistema pode criar PROJETOS para cada topologia". Mesmo padrão de documento
-- opaco em JSONB já usado por topology_canvas (ver 122_topology_canvas.sql) — o backend só
-- garante que é JSON válido, quem interpreta nodes/edges/groups/settings é o frontend.
CREATE TABLE topology_projects (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT NOT NULL,
    canvas     JSONB NOT NULL DEFAULT '{"nodes":[],"edges":[],"groups":[]}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Migra o diagrama único já existente para o primeiro projecto, para ninguém perder o que já
-- tinha desenhado — só corre se topology_canvas existir e tiver conteúdo (instalação nova não
-- tem nada para migrar).
INSERT INTO topology_projects (name, canvas)
SELECT 'Topologia principal', canvas
FROM topology_canvas
WHERE id = 1;

-- Instalação nova (sem linha em topology_canvas ainda, ex.: goose a correr pela primeira vez
-- numa base recém-criada) — garante pelo menos 1 projecto para a tela nunca ficar sem nada para
-- mostrar/seleccionar.
INSERT INTO topology_projects (name)
SELECT 'Topologia principal'
WHERE NOT EXISTS (SELECT 1 FROM topology_projects);

-- +goose Down
DROP TABLE IF EXISTS topology_projects;
