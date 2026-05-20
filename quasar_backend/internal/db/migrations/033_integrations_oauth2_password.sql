-- +goose Up
ALTER TABLE integrations DROP CONSTRAINT IF EXISTS integrations_auth_type_check;
ALTER TABLE integrations ADD CONSTRAINT integrations_auth_type_check
    CHECK (auth_type IN ('none', 'bearer', 'basic', 'api_key', 'login', 'oauth2_password'));

-- +goose Down
ALTER TABLE integrations DROP CONSTRAINT IF EXISTS integrations_auth_type_check;
ALTER TABLE integrations ADD CONSTRAINT integrations_auth_type_check
    CHECK (auth_type IN ('none', 'bearer', 'basic', 'api_key', 'login'));
