-- +goose Up
-- POP pode existir sem localidade (0..N POPs por localidade; desvincular = locality_id NULL).

ALTER TABLE pops
  ALTER COLUMN locality_id DROP NOT NULL;

COMMENT ON COLUMN pops.locality_id IS 'Localidade associada; NULL = POP sem localidade';

-- +goose Down
-- Reanexa POPs órfãos a uma localidade placeholder antes de reimpor NOT NULL.
-- +goose StatementBegin
DO $$
DECLARE
  lid UUID;
BEGIN
  IF EXISTS (SELECT 1 FROM pops WHERE locality_id IS NULL) THEN
    SELECT id INTO lid FROM commercial_localities WHERE lower(trim(name)) = 'sem localidade' LIMIT 1;
    IF lid IS NULL THEN
      INSERT INTO commercial_localities (name, updated_at)
      VALUES ('Sem localidade', now())
      RETURNING id INTO lid;
    END IF;
    UPDATE pops SET locality_id = lid, updated_at = now() WHERE locality_id IS NULL;
  END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE pops
  ALTER COLUMN locality_id SET NOT NULL;
