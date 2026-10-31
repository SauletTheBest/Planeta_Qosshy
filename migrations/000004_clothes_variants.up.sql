-- Per-size stock in its own table instead of a text column like "S:2, M:0".
-- Before: buying an M decreased only the total clothes.stock, the M count never changed,
-- and the catalog size filter compared size = 'M' with "S:2, M:3" (never matched).

BEGIN;

CREATE TABLE clothes_variants (
    id         BIGSERIAL PRIMARY KEY,
    clothes_id BIGINT NOT NULL REFERENCES clothes (id) ON DELETE CASCADE,
    size       TEXT   NOT NULL CHECK (size <> ''),
    stock      INT    NOT NULL CHECK (stock >= 0),
    UNIQUE (clothes_id, size)   -- one row per size of a product; also used by the admin upsert
);

-- Backfill from the old text column. Formats found in the code:
--   "S:2, M:0"   written by the admin form
--   "Распродано" written by the admin form when every size is 0
--   "M"          a plain size: then clothes.stock is its stock
WITH parts AS (
    SELECT c.id    AS clothes_id,
           c.stock AS total,
           trim(p) AS part,
           count(*) OVER (PARTITION BY c.id) AS parts_count
    FROM clothes AS c,
         unnest(string_to_array(c.size, ',')) AS p     -- one row per comma-separated part
    WHERE trim(p) NOT IN ('', 'Распродано')
)
INSERT INTO clothes_variants (clothes_id, size, stock)
SELECT clothes_id,
       trim(split_part(part, ':', 1)),
       sum(CASE
               WHEN position(':' IN part) > 0
                   THEN COALESCE(NULLIF(regexp_replace(split_part(part, ':', 2), '[^0-9]', '', 'g'), '')::int, 0)
               WHEN parts_count = 1 THEN total  -- a single plain size owns the whole stock
               ELSE 0                           -- several plain sizes: per-size stock unknown, don't oversell
           END)
FROM parts
WHERE trim(split_part(part, ':', 1)) <> ''
GROUP BY clothes_id, trim(split_part(part, ':', 1));   -- "M:1, M:2" becomes one M with 3

-- Products that were in stock but had no readable size keep their stock as "One size".
INSERT INTO clothes_variants (clothes_id, size, stock)
SELECT c.id, 'One size', c.stock
FROM clothes AS c
WHERE c.stock > 0
  AND NOT EXISTS (SELECT 1 FROM clothes_variants AS v WHERE v.clothes_id = c.id);

-- From now on the variants are the truth: the total is their sum.
-- (If the old text and clothes.stock disagreed, the per-size numbers win.)
UPDATE clothes AS c
SET stock = v.total, in_stock = v.total > 0
FROM (SELECT clothes_id, sum(stock) AS total FROM clothes_variants GROUP BY clothes_id) AS v
WHERE v.clothes_id = c.id;

UPDATE clothes
SET stock = 0, in_stock = false
WHERE id NOT IN (SELECT clothes_id FROM clothes_variants);

-- A new product starts empty: its stock is the sum of the variants added after it.
ALTER TABLE clothes
    ALTER COLUMN stock SET DEFAULT 0,
    ALTER COLUMN in_stock SET DEFAULT false,
    ALTER COLUMN in_stock SET NOT NULL;

ALTER TABLE clothes DROP COLUMN size;

-- clothes.stock / in_stock are now a cached total maintained by the database itself.
-- The trigger applies the DIFFERENCE (stock + delta) instead of recomputing SUM():
-- two purchases of different sizes running at the same time both lock the clothes row,
-- and "stock + delta" is re-evaluated on the newest row version, so no update is lost.
-- A recomputed SUM() would use the second transaction's older snapshot and could drift.
CREATE FUNCTION clothes_variants_sync_stock() RETURNS trigger AS $$
DECLARE
    delta INT;
    target BIGINT;
BEGIN
    IF TG_OP = 'INSERT' THEN
        delta := NEW.stock;
        target := NEW.clothes_id;
    ELSIF TG_OP = 'DELETE' THEN
        delta := -OLD.stock;
        target := OLD.clothes_id;
    ELSE
        IF NEW.clothes_id <> OLD.clothes_id THEN
            RAISE EXCEPTION 'a variant cannot be moved to another product';
        END IF;
        delta := NEW.stock - OLD.stock;
        target := NEW.clothes_id;
    END IF;

    IF delta <> 0 THEN
        UPDATE clothes
        SET stock = stock + delta,
            in_stock = stock + delta > 0
        WHERE id = target;
    END IF;
    RETURN NULL;  -- AFTER trigger: the return value is ignored
END
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_clothes_variants_sync_stock
AFTER INSERT OR DELETE OR UPDATE OF stock, clothes_id ON clothes_variants
FOR EACH ROW EXECUTE FUNCTION clothes_variants_sync_stock();

COMMIT;
