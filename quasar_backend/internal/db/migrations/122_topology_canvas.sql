-- +goose Up
-- Documento único (singleton) do diagrama livre de topologia (menu Mapa → Topologia) —
-- mesmo padrão de monitoring_intervals/users.preferences: uma linha, uma coluna JSONB
-- evolutiva. O conteúdo (nodes/edges/groups) é opaco para o backend, só o frontend
-- interpreta a estrutura (ver internal/api/handlers_topology.go).
CREATE TABLE IF NOT EXISTS topology_canvas (
    id SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    canvas JSONB NOT NULL DEFAULT '{"nodes":[],"edges":[],"groups":[]}'::jsonb,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO topology_canvas (id)
SELECT 1
WHERE NOT EXISTS (SELECT 1 FROM topology_canvas WHERE id = 1);

-- +goose Down
DROP TABLE IF EXISTS topology_canvas;
