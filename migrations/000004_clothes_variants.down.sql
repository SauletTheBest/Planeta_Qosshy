-- Rolls back 000004: variants are folded back into the "S:2, M:0" text column.

BEGIN;

DROP TRIGGER trg_clothes_variants_sync_stock ON clothes_variants;
DROP FUNCTION clothes_variants_sync_stock();

ALTER TABLE clothes ADD COLUMN size TEXT;
UPDATE clothes AS c
SET size = COALESCE(
    (SELECT string_agg(v.size || ':' || v.stock, ', ' ORDER BY v.id)
     FROM clothes_variants AS v
     WHERE v.clothes_id = c.id),
    'Распродано');
ALTER TABLE clothes ALTER COLUMN size SET NOT NULL;

ALTER TABLE clothes
    ALTER COLUMN stock SET DEFAULT 1,
    ALTER COLUMN in_stock DROP NOT NULL,
    ALTER COLUMN in_stock SET DEFAULT true;

DROP TABLE clothes_variants;

COMMIT;
