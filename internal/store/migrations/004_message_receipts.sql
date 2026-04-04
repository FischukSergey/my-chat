CREATE TABLE IF NOT EXISTS message_receipts (
    message_id UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id),
    delivered_at TIMESTAMPTZ NULL,
    read_at TIMESTAMPTZ NULL,
    PRIMARY KEY (message_id, user_id)
);

CREATE INDEX IF NOT EXISTS message_receipts_user_read_at_idx
    ON message_receipts (user_id, read_at);
