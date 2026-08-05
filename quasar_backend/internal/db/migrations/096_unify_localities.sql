-- Unifica Localidades (ex-comercial + POPs): endereço, UF, coords; POP opcional; VLANs.

ALTER TABLE commercial_localities
  ADD COLUMN IF NOT EXISTS address TEXT,
  ADD COLUMN IF NOT EXISTS uf TEXT,
  ADD COLUMN IF NOT EXISTS latitude DOUBLE PRECISION,
  ADD COLUMN IF NOT EXISTS longitude DOUBLE PRECISION,
  ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

UPDATE commercial_localities
SET uf = NULLIF(trim(region_code), '')
WHERE (uf IS NULL OR trim(uf) = '')
  AND region_code IS NOT NULL
  AND trim(region_code) <> '';

ALTER TABLE pops
  ADD COLUMN IF NOT EXISTS locality_id UUID REFERENCES commercial_localities(id) ON DELETE RESTRICT;

-- Backfill: cada POP sem localidade ganha (ou reutiliza) uma localidade com o mesmo nome.
DO $$
DECLARE
  r RECORD;
  lid UUID;
BEGIN
  FOR r IN
    SELECT id, description, address, latitude, longitude
    FROM pops
    WHERE locality_id IS NULL
  LOOP
    SELECT cl.id INTO lid
    FROM commercial_localities cl
    WHERE lower(trim(cl.name)) = lower(trim(r.description))
    LIMIT 1;

    IF lid IS NULL THEN
      INSERT INTO commercial_localities (name, address, latitude, longitude, updated_at)
      VALUES (
        COALESCE(NULLIF(trim(r.description), ''), 'POP ' || r.id::text),
        r.address,
        r.latitude,
        r.longitude,
        now()
      )
      RETURNING id INTO lid;
    ELSE
      UPDATE commercial_localities SET
        address = COALESCE(NULLIF(trim(address), ''), r.address),
        latitude = COALESCE(latitude, r.latitude),
        longitude = COALESCE(longitude, r.longitude),
        updated_at = now()
      WHERE id = lid;
    END IF;

    UPDATE pops SET locality_id = lid, updated_at = now() WHERE id = r.id;
  END LOOP;
END $$;

-- Localidade pode ter 0..N POPs; cada POP aponta para exactamente uma localidade.
CREATE INDEX IF NOT EXISTS pops_locality_id_idx ON pops (locality_id);

ALTER TABLE pops
  ALTER COLUMN locality_id SET NOT NULL;

CREATE TABLE IF NOT EXISTS locality_vlans (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  locality_id UUID NOT NULL REFERENCES commercial_localities(id) ON DELETE CASCADE,
  vlan TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT locality_vlans_vlan_nonempty CHECK (length(trim(vlan)) > 0),
  UNIQUE (locality_id, vlan)
);

CREATE INDEX IF NOT EXISTS locality_vlans_vlan_idx ON locality_vlans (vlan);
