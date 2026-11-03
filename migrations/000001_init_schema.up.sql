-- Baseline: the schema the Go models had while GORM AutoMigrate created the tables.
-- IF NOT EXISTS lets this same file adopt a database that AutoMigrate already built:
-- existing tables are left alone, a fresh database gets all of them.
-- (The old database/migrations/*.sql files were never executed and don't match the models.)

BEGIN;

CREATE TABLE IF NOT EXISTS users (
    id                 BIGSERIAL PRIMARY KEY,
    username           TEXT NOT NULL UNIQUE,
    email              TEXT NOT NULL UNIQUE,
    password           TEXT NOT NULL,           -- bcrypt hash, never the password itself
    role               TEXT DEFAULT 'user',
    verification_token TEXT,
    verified           BOOLEAN DEFAULT FALSE
);

CREATE TABLE IF NOT EXISTS clothes (
    id          BIGSERIAL PRIMARY KEY,
    title       TEXT NOT NULL,
    brand       TEXT NOT NULL,
    category    TEXT NOT NULL,
    size        TEXT NOT NULL,
    color       TEXT NOT NULL,
    price       NUMERIC NOT NULL CHECK (price >= 0),
    stock       BIGINT NOT NULL DEFAULT 1 CHECK (stock >= 0),  -- the DB itself refuses negative stock
    image_url   TEXT,
    description TEXT,
    in_stock    BOOLEAN DEFAULT TRUE,
    created_at  TIMESTAMPTZ DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_clothes_category ON clothes (category);

CREATE TABLE IF NOT EXISTS orders (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    clothes_id BIGINT NOT NULL REFERENCES clothes (id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_orders_user_id ON orders (user_id);
CREATE INDEX IF NOT EXISTS idx_orders_clothes_id ON orders (clothes_id);

-- Not used by the code yet; reworked together with the payment flow.
CREATE TABLE IF NOT EXISTS payments (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    clothes_id BIGINT REFERENCES clothes (id) ON DELETE SET NULL,
    amount     NUMERIC NOT NULL CHECK (amount >= 0),
    status     TEXT DEFAULT 'pending',
    created_at TIMESTAMPTZ DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_payments_user_id ON payments (user_id);
CREATE INDEX IF NOT EXISTS idx_payments_clothes_id ON payments (clothes_id);

CREATE TABLE IF NOT EXISTS transactions (
    id             BIGSERIAL PRIMARY KEY,
    title          TEXT NOT NULL,
    user_name      TEXT NOT NULL,
    price          NUMERIC NOT NULL,
    quantity       BIGINT DEFAULT 1,
    total_amount   NUMERIC NOT NULL,
    payment_method TEXT NOT NULL,
    status         TEXT DEFAULT 'pending',
    created_at     TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE IF NOT EXISTS chats (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    admin_id   BIGINT,
    status     TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    closed_at  TIMESTAMP
);

-- chat_id/sender_id are TEXT because the Go model stores them as strings ("Admin", username).
CREATE TABLE IF NOT EXISTS messages (
    id         BIGSERIAL PRIMARY KEY,
    sender_id  TEXT,
    chat_id    TEXT,
    content    TEXT,
    created_at TIMESTAMPTZ DEFAULT now()
);

COMMIT;
