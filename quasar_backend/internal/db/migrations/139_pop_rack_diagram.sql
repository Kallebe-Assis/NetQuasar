-- +goose Up
-- Diagrama 2D da topologia do POP (retângulo por porta de OLT/mikrotik/switch/DIO, ligações de
-- fibra coloridas entre portas) — mesmo padrão de documento opaco em JSONB já usado por
-- topology_projects (132_topology_projects.sql): o backend só garante JSON válido, quem
-- interpreta nodes/edges é o frontend. Um diagrama por POP (não N projectos nomeados como a
-- Topologia geral) — pop_id é a própria chave primária.
CREATE TABLE IF NOT EXISTS pop_rack_diagrams (
    pop_id     UUID PRIMARY KEY REFERENCES pops(id) ON DELETE CASCADE,
    canvas     JSONB NOT NULL DEFAULT '{"nodes":[],"edges":[]}'::jsonb,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS pop_rack_diagrams;
