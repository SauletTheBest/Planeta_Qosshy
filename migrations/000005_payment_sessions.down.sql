-- Rolls back 000005. Pending orders keep their reserved stock; cancel them first if needed.

BEGIN;

DROP INDEX uq_payments_one_pending_per_order;
ALTER TABLE payments
    DROP COLUMN provider,
    DROP COLUMN provider_payment_id,
    DROP COLUMN payment_url,
    DROP COLUMN card_last4,
    DROP COLUMN failure_reason,
    DROP COLUMN receipt_url;

DROP INDEX idx_orders_pending_expires_at;
ALTER TABLE orders DROP COLUMN expires_at;

COMMIT;
