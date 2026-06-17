CREATE TABLE IF NOT EXISTS notification_outbox (
    id UUID PRIMARY KEY,
    event_type TEXT NOT NULL,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    payload JSONB NOT NULL,
    dedup_key TEXT NOT NULL,
    attempt INT NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'pending',
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_error TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT notification_outbox_event_type_allowed CHECK (event_type IN ('message_new')),
    CONSTRAINT notification_outbox_status_allowed CHECK (status IN ('pending', 'sent', 'failed')),
    CONSTRAINT notification_outbox_attempt_non_negative CHECK (attempt >= 0),
    CONSTRAINT notification_outbox_dedup_key_not_empty CHECK (length(trim(dedup_key)) > 0),
    CONSTRAINT notification_outbox_dedup_key_unique UNIQUE (dedup_key)
);

CREATE INDEX IF NOT EXISTS notification_outbox_status_next_attempt_idx
    ON notification_outbox (status, next_attempt_at);
