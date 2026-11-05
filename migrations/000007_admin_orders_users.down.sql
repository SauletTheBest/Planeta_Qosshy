ALTER TABLE users
    DROP CONSTRAINT IF EXISTS users_role_check,
    ALTER COLUMN role DROP NOT NULL,
    DROP COLUMN IF EXISTS created_at,
    DROP COLUMN IF EXISTS blocked_at;

DROP TABLE IF EXISTS order_events;

-- A refunded payment did take money once, so going back it becomes 'succeeded' again.
UPDATE payments SET status = 'succeeded' WHERE status = 'refunded';
ALTER TABLE payments
    DROP COLUMN IF EXISTS refunded_at,
    DROP COLUMN IF EXISTS refund_needed_at,
    DROP CONSTRAINT payments_status_check;
ALTER TABLE payments
    ADD CONSTRAINT payments_status_check CHECK (status IN ('pending', 'succeeded', 'failed'));
