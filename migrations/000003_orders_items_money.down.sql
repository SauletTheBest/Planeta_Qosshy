-- Rolls back 000003. LOSSY by nature, read before running:
--   * orders whose product was deleted after the purchase can't exist in the old schema
--     (orders.clothes_id was NOT NULL) and are deleted;
--   * from multi-item orders only the first item survives;
--   * payment rows and order statuses are dropped.
-- In production prefer restoring a backup; this file is mainly for local development.

BEGIN;

DROP TABLE payments;
CREATE TABLE payments (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    clothes_id BIGINT REFERENCES clothes (id) ON DELETE SET NULL,
    amount     NUMERIC NOT NULL CHECK (amount >= 0),
    status     TEXT DEFAULT 'pending',
    created_at TIMESTAMPTZ DEFAULT now()
);
CREATE INDEX idx_payments_user_id ON payments (user_id);
CREATE INDEX idx_payments_clothes_id ON payments (clothes_id);

ALTER TABLE orders ADD COLUMN clothes_id BIGINT REFERENCES clothes (id) ON DELETE CASCADE;
UPDATE orders AS o
SET clothes_id = first_item.clothes_id
FROM (
    SELECT DISTINCT ON (order_id) order_id, clothes_id
    FROM order_items
    ORDER BY order_id, id
) AS first_item
WHERE first_item.order_id = o.id;
DELETE FROM orders WHERE clothes_id IS NULL;
ALTER TABLE orders ALTER COLUMN clothes_id SET NOT NULL;
CREATE INDEX idx_orders_clothes_id ON orders (clothes_id);

DROP TABLE order_items;
ALTER TABLE orders
    DROP COLUMN status,
    DROP COLUMN total_tiyn,
    DROP COLUMN paid_at,
    DROP COLUMN updated_at;

ALTER TABLE clothes ADD COLUMN price NUMERIC;
UPDATE clothes SET price = round(price_tiyn / 100.0, 2);  -- 1500050 → 15000.50, not 15000.500000000000
ALTER TABLE clothes
    ALTER COLUMN price SET NOT NULL,
    ADD CONSTRAINT clothes_price_check CHECK (price >= 0);
ALTER TABLE clothes DROP COLUMN price_tiyn;

COMMENT ON TABLE transactions IS NULL;

COMMIT;
