-- +goose Up
CREATE TABLE settings_ui (
    id SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    theme TEXT NOT NULL DEFAULT 'dark' CHECK (theme IN ('dark', 'light')),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO settings_ui (id, theme) VALUES (1, 'dark');

-- +goose Down
DROP TABLE IF EXISTS settings_ui;
