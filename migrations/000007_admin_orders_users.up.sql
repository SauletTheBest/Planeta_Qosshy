-- Phase 4: what the admin panel needs.

-- 1. Refunds. A payment can now end as 'refunded' (the bank gave the money back).
ALTER TABLE payments DROP CONSTRAINT payments_status_check;
ALTER TABLE payments
    ADD CONSTRAINT payments_status_check
        CHECK (status IN ('pending', 'succeeded', 'failed', 'refunded')),
    -- When we found out that we owe this money back: the order was cancelled after it was
    -- paid, or a late payment came for an item that is already sold out. Before this column
    -- such cases existed only as a "REFUND NEEDED" line in the log, easy to miss.
    ADD COLUMN refund_needed_at TIMESTAMPTZ,
    ADD COLUMN refunded_at      TIMESTAMPTZ;

-- 2. Order history: who did what and when. "Why was my order cancelled?" gets an answer
--    from the database instead of from someone's memory. Rows are only ever added.
CREATE TABLE order_events (
    id         BIGSERIAL   PRIMARY KEY,
    order_id   BIGINT      NOT NULL REFERENCES orders (id) ON DELETE CASCADE,
    action     TEXT        NOT NULL,          -- paid, expired, shipped, delivered, cancelled, refunded...
    actor_id   BIGINT      REFERENCES users (id) ON DELETE SET NULL,  -- NULL = the system or the bank
    note       TEXT        NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_order_events_order ON order_events (order_id, created_at);

-- 3. Users: blocking and the sign-up date.
ALTER TABLE users
    ADD COLUMN blocked_at TIMESTAMPTZ,
    -- Existing users keep NULL ("unknown"): writing now() would claim they all signed up today.
    ADD COLUMN created_at TIMESTAMPTZ;
ALTER TABLE users ALTER COLUMN created_at SET DEFAULT now();

-- Only these two roles exist; a typo in the admin form must not create a third.
UPDATE users SET role = 'user' WHERE role IS NULL OR role NOT IN ('user', 'admin');
ALTER TABLE users
    ALTER COLUMN role SET NOT NULL,
    ALTER COLUMN role SET DEFAULT 'user',
    ADD CONSTRAINT users_role_check CHECK (role IN ('user', 'admin'));
