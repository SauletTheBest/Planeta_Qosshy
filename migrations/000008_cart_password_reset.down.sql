-- A hash can't be turned back into the token, so confirmation links sent before this
-- point stop working after a rollback; users can ask for a new letter.
ALTER TABLE users
    DROP COLUMN IF EXISTS verification_sent_at,
    DROP COLUMN IF EXISTS sessions_valid_after;

DROP TABLE IF EXISTS password_resets;
DROP TABLE IF EXISTS cart_items;
