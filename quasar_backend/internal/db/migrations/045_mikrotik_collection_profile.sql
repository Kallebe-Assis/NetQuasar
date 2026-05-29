-- +goose Up
CREATE TABLE IF NOT EXISTS settings_mikrotik_collection (
    id INT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    metrics JSONB NOT NULL DEFAULT '{}',
    collection_steps JSONB NOT NULL DEFAULT '[]',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO settings_mikrotik_collection (id, metrics, collection_steps)
VALUES (1, '{}'::jsonb, '[]'::jsonb)
ON CONFLICT (id) DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS settings_mikrotik_collection;
