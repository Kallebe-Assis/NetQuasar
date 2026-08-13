-- +goose Up
CREATE TABLE network_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    category_code TEXT NOT NULL,
    type_code TEXT NOT NULL,
    impact TEXT NOT NULL DEFAULT 'none'
        CHECK (impact IN ('none', 'low', 'medium', 'high', 'critical')),
    notes TEXT,
    pop_id UUID REFERENCES pops(id) ON DELETE SET NULL,
    device_id UUID REFERENCES devices(id) ON DELETE SET NULL,
    technician_id UUID REFERENCES users(id) ON DELETE SET NULL,
    project_id UUID REFERENCES network_projects(id) ON DELETE SET NULL,
    cto_id UUID REFERENCES network_ctos(id) ON DELETE SET NULL,
    cable_id UUID REFERENCES network_cables(id) ON DELETE SET NULL,
    splice_box_id UUID REFERENCES network_splice_boxes(id) ON DELETE SET NULL,
    pole_id UUID REFERENCES network_poles(id) ON DELETE SET NULL,
    interface_name TEXT,
    vlan TEXT,
    extra JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    updated_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_network_events_occurred ON network_events (occurred_at DESC);
CREATE INDEX idx_network_events_category ON network_events (category_code, occurred_at DESC);
CREATE INDEX idx_network_events_type ON network_events (type_code, occurred_at DESC);
CREATE INDEX idx_network_events_pop ON network_events (pop_id) WHERE pop_id IS NOT NULL;
CREATE INDEX idx_network_events_device ON network_events (device_id) WHERE device_id IS NOT NULL;
CREATE INDEX idx_network_events_tech ON network_events (technician_id) WHERE technician_id IS NOT NULL;
CREATE INDEX idx_network_events_impact ON network_events (impact);
CREATE INDEX idx_network_events_project ON network_events (project_id) WHERE project_id IS NOT NULL;

UPDATE permission_profiles
SET permissions = permissions || '["network_events.view"]'::jsonb,
    updated_at = now()
WHERE slug = 'user'
  AND jsonb_typeof(permissions) = 'array'
  AND NOT (permissions @> '["network_events.view"]'::jsonb)
  AND NOT (permissions @> '["*"]'::jsonb);

-- +goose Down
UPDATE permission_profiles
SET permissions = (
    SELECT COALESCE(jsonb_agg(value), '[]'::jsonb)
    FROM jsonb_array_elements(permissions) AS t(value)
    WHERE value::text <> '"network_events.view"'
),
    updated_at = now()
WHERE slug = 'user'
  AND jsonb_typeof(permissions) = 'array';

DROP TABLE IF EXISTS network_events;
