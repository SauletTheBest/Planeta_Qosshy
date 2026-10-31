-- Orders with a lifecycle, order items as a purchase snapshot, money as integer tiyn.
-- The whole file runs in one transaction: if any statement fails, nothing is changed.

BEGIN;

-- 1. Money: integer tiyn (1 ₸ = 100 tiyn) instead of NUMERIC read into Go float64.
--    round() because price * 100 of a NUMERIC like 1999.995 must not be truncated silently.
ALTER TABLE clothes ADD COLUMN price_tiyn BIGINT;
UPDATE clothes SET price_tiyn = round(price * 100);
ALTER TABLE clothes
    ALTER COLUMN price_tiyn SET NOT NULL,
    ADD CONSTRAINT clothes_price_tiyn_check CHECK (price_tiyn >= 0);
ALTER TABLE clothes DROP COLUMN price;

-- 2. Orders get a status and a total.
--    Existing rows were created only after a (simulated) payment, so they start as 'paid';
--    new rows default to 'pending'.
ALTER TABLE orders
    ADD COLUMN status     TEXT NOT NULL DEFAULT 'paid',
    ADD COLUMN total_tiyn BIGINT,
    ADD COLUMN paid_at    TIMESTAMPTZ,
    ADD COLUMN updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE orders
    ALTER COLUMN status SET DEFAULT 'pending',
    ADD CONSTRAINT orders_status_check
        CHECK (status IN ('pending', 'paid', 'cancelled', 'shipped', 'delivered'));

-- 3. Order items: WHAT was bought and FOR HOW MUCH, copied at purchase time.
--    Editing the price or deleting the product later must not rewrite order history,
--    hence ON DELETE SET NULL and the title/price copies.
CREATE TABLE order_items (
    id              BIGSERIAL PRIMARY KEY,
    order_id        BIGINT NOT NULL REFERENCES orders (id) ON DELETE CASCADE,
    clothes_id      BIGINT REFERENCES clothes (id) ON DELETE SET NULL,
    title           TEXT   NOT NULL,
    size            TEXT,
    unit_price_tiyn BIGINT NOT NULL CHECK (unit_price_tiyn >= 0),
    quantity        INT    NOT NULL CHECK (quantity > 0)
);
CREATE INDEX idx_order_items_order_id ON order_items (order_id);
CREATE INDEX idx_order_items_clothes_id ON order_items (clothes_id);

-- Backfill: every old order was exactly one item. The price at purchase time was never
-- stored, so the current price is the best available value.
INSERT INTO order_items (order_id, clothes_id, title, size, unit_price_tiyn, quantity)
SELECT o.id, c.id, c.title, NULL, c.price_tiyn, 1
FROM orders AS o
JOIN clothes AS c ON c.id = o.clothes_id;

UPDATE orders AS o
SET total_tiyn = i.unit_price_tiyn * i.quantity,
    paid_at    = o.created_at
FROM order_items AS i
WHERE i.order_id = o.id;

ALTER TABLE orders
    ALTER COLUMN total_tiyn SET NOT NULL,
    ADD CONSTRAINT orders_total_tiyn_check CHECK (total_tiyn >= 0);

-- orders.clothes_id is replaced by order_items. Its ON DELETE CASCADE also meant that
-- deleting a product silently deleted every order that contained it.
ALTER TABLE orders DROP COLUMN clothes_id;

-- 4. Payments, rebuilt around orders. The old table was never written by the code;
--    if it unexpectedly has rows, stop instead of losing them.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM payments) THEN
        RAISE EXCEPTION 'payments has rows; it was assumed unused, migrate it by hand';
    END IF;
END $$;
DROP TABLE payments;

CREATE TABLE payments (
    id          BIGSERIAL PRIMARY KEY,
    -- RESTRICT (the default): an order with payments can't be deleted, money records stay.
    order_id    BIGINT NOT NULL REFERENCES orders (id),
    amount_tiyn BIGINT NOT NULL CHECK (amount_tiyn >= 0),
    method      TEXT   NOT NULL,
    status      TEXT   NOT NULL CHECK (status IN ('pending', 'succeeded', 'failed')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_payments_order_id ON payments (order_id);

-- The old "transactions" table stays as a read-only archive: its rows have no link to
-- users or orders (only a user name and a title), so they can't be moved into payments.
COMMENT ON TABLE transactions IS 'legacy, read-only archive; replaced by payments in migration 000003';

COMMIT;
