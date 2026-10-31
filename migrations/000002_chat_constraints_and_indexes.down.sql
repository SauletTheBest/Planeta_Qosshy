-- Rolls back 000002. Closed duplicate chats stay closed and messages.id stays:
-- dropping a primary key that other code may already rely on is riskier than keeping it.

BEGIN;

DROP INDEX IF EXISTS idx_orders_user_id_created_at;
DROP INDEX IF EXISTS uq_chats_one_active_per_user;
DROP INDEX IF EXISTS idx_messages_chat_id_created_at;

COMMIT;
