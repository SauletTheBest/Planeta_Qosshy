-- Kafka: the transactional outbox and the notifier's memory.

-- 1. Events waiting to be published to Kafka. A row is written IN THE SAME TRANSACTION
--    as the change it describes (order paid, shipped…): either both are saved or neither.
--    The relay in the shop reads unpublished rows, sends them to Kafka and sets published_at.
CREATE TABLE outbox_events (
    id           BIGSERIAL   PRIMARY KEY,           -- also the publishing order
    topic        TEXT        NOT NULL,
    msg_key      TEXT        NOT NULL,              -- Kafka key: same key → same partition → kept in order
    payload      JSONB       NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ,                       -- NULL = not in Kafka yet
    attempts     INT         NOT NULL DEFAULT 0,
    last_error   TEXT        NOT NULL DEFAULT ''
);
-- The relay always asks "what is not published yet?". A partial index holds only those
-- rows, so it stays tiny even when the table has millions of published events.
CREATE INDEX idx_outbox_unpublished ON outbox_events (id) WHERE published_at IS NULL;

-- 2. Events the notifier has already handled. Kafka delivers "at least once": after a
--    restart some messages come again, and without this table the customer would get
--    the same letter twice. (A notifier in a separate company would keep this in its own
--    database; here both services share one PostgreSQL to keep the stand small.)
CREATE TABLE notifier_processed_events (
    event_id     TEXT        PRIMARY KEY,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
