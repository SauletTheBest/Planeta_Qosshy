-- Fixes that apply both to fresh databases and to ones AutoMigrate created earlier.

BEGIN;

-- AutoMigrate created messages without a primary key (the Go model has no ID field).
-- On a fresh database the column already exists and this line does nothing.
ALTER TABLE messages ADD COLUMN IF NOT EXISTS id BIGSERIAL PRIMARY KEY;

-- Every chat page loads "messages WHERE chat_id = ? ORDER BY created_at".
-- Without an index Postgres reads the whole table each time.
CREATE INDEX IF NOT EXISTS idx_messages_chat_id_created_at ON messages (chat_id, created_at);

-- At most one active chat per user. Without this, two quick clicks (or two tabs)
-- could create two "active" chats. First close duplicates that may already exist,
-- keeping the newest one, otherwise the unique index below can't be created.
UPDATE chats AS c
SET status = 'inactive', closed_at = now()
WHERE c.status = 'active'
  AND EXISTS (
      SELECT 1 FROM chats AS newer
      WHERE newer.user_id = c.user_id
        AND newer.status = 'active'
        AND newer.id > c.id
  );

-- Partial index: uniqueness is enforced only for rows WHERE status = 'active',
-- so a user can still have many closed chats.
CREATE UNIQUE INDEX IF NOT EXISTS uq_chats_one_active_per_user ON chats (user_id) WHERE status = 'active';

-- Profile and order history: orders of one user, newest first.
CREATE INDEX IF NOT EXISTS idx_orders_user_id_created_at ON orders (user_id, created_at DESC);

COMMIT;
