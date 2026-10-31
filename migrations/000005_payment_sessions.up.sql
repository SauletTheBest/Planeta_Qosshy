-- Real payment flow: an order waits for the bank as 'pending' with a reservation deadline,
-- and each payment attempt remembers the bank's session.

BEGIN;

-- When the reservation of a pending order ends; after it the stock goes back on sale.
ALTER TABLE orders ADD COLUMN expires_at TIMESTAMPTZ;

-- The expiry job asks "pending orders whose time is up" every minute. A partial index holds
-- only pending orders, so it stays tiny no matter how many paid orders pile up.
CREATE INDEX idx_orders_pending_expires_at ON orders (expires_at) WHERE status = 'pending';

ALTER TABLE payments
    ADD COLUMN provider            TEXT NOT NULL DEFAULT 'simulated', -- rows so far were simulated inside the shop
    ADD COLUMN provider_payment_id TEXT UNIQUE,  -- the bank's ID; webhooks find the payment by it
    ADD COLUMN payment_url         TEXT,         -- the bank page where the buyer enters the card
    ADD COLUMN card_last4          TEXT,
    ADD COLUMN failure_reason      TEXT,
    ADD COLUMN receipt_url         TEXT;
ALTER TABLE payments ALTER COLUMN provider DROP DEFAULT; -- new rows must say which provider they use

-- At most one attempt in progress per order: a double click can't open two bank sessions.
CREATE UNIQUE INDEX uq_payments_one_pending_per_order ON payments (order_id) WHERE status = 'pending';

COMMIT;
