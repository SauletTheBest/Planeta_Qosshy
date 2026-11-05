-- Block C: shopping cart, password reset, safer email links.

-- 1. Cart: what a logged-in user is about to buy. One row per size of a product.
CREATE TABLE cart_items (
    user_id    BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- A size the admin deletes disappears from carts too.
    variant_id BIGINT      NOT NULL REFERENCES clothes_variants (id) ON DELETE CASCADE,
    quantity   INT         NOT NULL CHECK (quantity BETWEEN 1 AND 10),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- The same size added twice is one row with a bigger quantity, not two rows.
    PRIMARY KEY (user_id, variant_id)
);

-- 2. Password reset links. Only the SHA-256 hash of the token is stored: the token is a
--    key to the account, and a leaked database must not contain working keys.
CREATE TABLE password_resets (
    id         BIGSERIAL   PRIMARY KEY,
    user_id    BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash TEXT        NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at    TIMESTAMPTZ,            -- a link works once
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_password_resets_user ON password_resets (user_id);

-- 3. Users.
ALTER TABLE users
    -- Sessions started before this moment are logged out (set by a password change/reset).
    ADD COLUMN sessions_valid_after TIMESTAMPTZ,
    -- The email confirmation link works for 24 hours after this.
    ADD COLUMN verification_sent_at TIMESTAMPTZ;

-- Email confirmation tokens were stored as plain text. Store their hash instead: links
-- already sent keep working (the app now hashes the token that arrives in the link),
-- but the table no longer holds working links. They get a fresh 24 hours.
UPDATE users
SET verification_token   = encode(sha256(convert_to(verification_token, 'UTF8')), 'hex'),
    verification_sent_at = now()
WHERE verification_token IS NOT NULL AND verification_token <> '';
