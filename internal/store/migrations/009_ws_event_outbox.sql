CREATE TABLE IF NOT EXISTS ws_event_outbox (
    id           UUID        PRIMARY KEY,
    event_type   TEXT        NOT NULL,
    user_id      UUID        NOT NULL REFERENCES users(id),
    payload      JSONB       NOT NULL,
    processed_at TIMESTAMPTZ NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ws_event_outbox_pending
    ON ws_event_outbox (created_at)
    WHERE processed_at IS NULL;
