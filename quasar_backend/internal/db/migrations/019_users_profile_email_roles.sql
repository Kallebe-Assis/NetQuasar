-- +goose Up
-- +goose StatementBegin

ALTER TABLE users ADD COLUMN IF NOT EXISTS display_name TEXT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS email TEXT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS phone TEXT;

UPDATE users SET display_name = COALESCE(NULLIF(trim(login), ''), 'Utilizador') WHERE display_name IS NULL;
UPDATE users SET email = lower(trim(login)) || '@seed.netquasar.local' WHERE email IS NULL;

ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_check;
UPDATE users SET role = 'viewer' WHERE role = 'user';
ALTER TABLE users ADD CONSTRAINT users_role_check CHECK (role IN ('admin', 'viewer'));

ALTER TABLE users DROP CONSTRAINT IF EXISTS users_login_key;

ALTER TABLE users DROP COLUMN IF EXISTS login;

ALTER TABLE users ALTER COLUMN display_name SET NOT NULL;
ALTER TABLE users ALTER COLUMN email SET NOT NULL;

DROP INDEX IF EXISTS users_email_lower_idx;
CREATE UNIQUE INDEX users_email_lower_idx ON users (lower(trim(email)));

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS users_email_lower_idx;

ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_check;
UPDATE users SET role = 'user' WHERE role = 'viewer';
ALTER TABLE users ADD CONSTRAINT users_role_check CHECK (role IN ('admin', 'user'));

ALTER TABLE users ADD COLUMN IF NOT EXISTS login TEXT;
UPDATE users SET login = email WHERE login IS NULL OR trim(login) = '';
ALTER TABLE users ALTER COLUMN login SET NOT NULL;
ALTER TABLE users ADD CONSTRAINT users_login_key UNIQUE (login);

ALTER TABLE users DROP COLUMN IF EXISTS phone;
ALTER TABLE users DROP COLUMN IF EXISTS email;
ALTER TABLE users DROP COLUMN IF EXISTS display_name;

-- +goose StatementEnd
