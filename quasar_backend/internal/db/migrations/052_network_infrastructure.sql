-- +goose Up
-- Infraestrutura de rede óptica (CTOs, emendas, cabos, postes, projetos)

CREATE TABLE IF NOT EXISTS network_projects (    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    display_number SERIAL UNIQUE,
    description TEXT NOT NULL,
    locality_id UUID REFERENCES commercial_localities(id) ON DELETE SET NULL,
    color TEXT,
    status TEXT NOT NULL DEFAULT 'planejamento',
    latitude DOUBLE PRECISION,
    longitude DOUBLE PRECISION,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT network_projects_status_chk CHECK (
        status IN ('planejamento', 'em_andamento', 'concluido', 'pausado', 'cancelado')
    ),
    CONSTRAINT network_projects_coords_chk CHECK (
        (latitude IS NULL AND longitude IS NULL)
        OR (latitude BETWEEN -90 AND 90 AND longitude BETWEEN -180 AND 180)
    )
);

CREATE INDEX IF NOT EXISTS idx_network_projects_locality ON network_projects(locality_id);
CREATE INDEX IF NOT EXISTS idx_network_projects_status ON network_projects(status);

CREATE TABLE IF NOT EXISTS network_ctos (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    display_number SERIAL UNIQUE,
    description TEXT NOT NULL,
    latitude DOUBLE PRECISION,
    longitude DOUBLE PRECISION,
    splitter TEXT,
    fiber_color TEXT,
    notes TEXT,
    needs_maintenance BOOLEAN NOT NULL DEFAULT false,
    project_id UUID REFERENCES network_projects(id) ON DELETE SET NULL,
    locality_id UUID REFERENCES commercial_localities(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT network_ctos_fiber_color_chk CHECK (
        fiber_color IS NULL OR fiber_color IN (
            'Verde', 'Amarelo', 'Branco', 'Azul', 'Vermelho', 'Violeta',
            'Marrom', 'Rosa', 'Preto', 'Cinza', 'Laranja', 'Aqua (Turquesa)'
        )
    ),
    CONSTRAINT network_ctos_coords_chk CHECK (
        (latitude IS NULL AND longitude IS NULL)
        OR (latitude BETWEEN -90 AND 90 AND longitude BETWEEN -180 AND 180)
    )
);

CREATE INDEX IF NOT EXISTS idx_network_ctos_project ON network_ctos(project_id);
CREATE INDEX IF NOT EXISTS idx_network_ctos_locality ON network_ctos(locality_id);
CREATE INDEX IF NOT EXISTS idx_network_ctos_maintenance ON network_ctos(needs_maintenance) WHERE needs_maintenance = true;

CREATE TABLE IF NOT EXISTS network_splice_boxes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    display_number SERIAL UNIQUE,
    description TEXT NOT NULL,
    latitude DOUBLE PRECISION,
    longitude DOUBLE PRECISION,
    fiber_count INTEGER,
    needs_maintenance BOOLEAN NOT NULL DEFAULT false,
    notes TEXT,
    project_id UUID REFERENCES network_projects(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT network_splice_boxes_fiber_count_chk CHECK (fiber_count IS NULL OR fiber_count >= 0),
    CONSTRAINT network_splice_boxes_coords_chk CHECK (
        (latitude IS NULL AND longitude IS NULL)
        OR (latitude BETWEEN -90 AND 90 AND longitude BETWEEN -180 AND 180)
    )
);

CREATE INDEX IF NOT EXISTS idx_network_splice_boxes_project ON network_splice_boxes(project_id);

CREATE TABLE IF NOT EXISTS network_cables (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    display_number SERIAL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    cable_type TEXT,
    fiber_count INTEGER,
    status TEXT NOT NULL DEFAULT 'ativo',
    project_id UUID REFERENCES network_projects(id) ON DELETE SET NULL,
    latitude DOUBLE PRECISION,
    longitude DOUBLE PRECISION,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT network_cables_fiber_count_chk CHECK (fiber_count IS NULL OR fiber_count >= 0),
    CONSTRAINT network_cables_status_chk CHECK (
        status IN ('ativo', 'planejado', 'inativo', 'manutencao')
    )
);

CREATE INDEX IF NOT EXISTS idx_network_cables_project ON network_cables(project_id);

CREATE TABLE IF NOT EXISTS network_poles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    display_number SERIAL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    pole_type TEXT,
    project_id UUID REFERENCES network_projects(id) ON DELETE SET NULL,
    locality_id UUID REFERENCES commercial_localities(id) ON DELETE SET NULL,
    latitude DOUBLE PRECISION,
    longitude DOUBLE PRECISION,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT network_poles_coords_chk CHECK (
        (latitude IS NULL AND longitude IS NULL)
        OR (latitude BETWEEN -90 AND 90 AND longitude BETWEEN -180 AND 180)
    )
);

CREATE INDEX IF NOT EXISTS idx_network_poles_project ON network_poles(project_id);
CREATE INDEX IF NOT EXISTS idx_network_poles_locality ON network_poles(locality_id);

-- +goose Down
DROP TABLE IF EXISTS network_poles;
DROP TABLE IF EXISTS network_cables;
DROP TABLE IF EXISTS network_splice_boxes;
DROP TABLE IF EXISTS network_ctos;
DROP TABLE IF EXISTS network_projects;
