CREATE TABLE IF NOT EXISTS messages (
    id UUID PRIMARY KEY,
    dialog_id UUID NOT NULL REFERENCES dialogs(id) ON DELETE CASCADE,
    sender_id UUID NOT NULL REFERENCES users(id),
    body TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT messages_body_not_empty CHECK (length(trim(body)) > 0),
    CONSTRAINT messages_body_max_len CHECK (length(body) <= 4000)
);

CREATE INDEX IF NOT EXISTS messages_dialog_created_at_idx
    ON messages (dialog_id, created_at DESC);
