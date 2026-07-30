ALTER TABLE messages
    ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ NULL;

CREATE INDEX IF NOT EXISTS idx_messages_expires_at
    ON messages (expires_at)
    WHERE deleted_at IS NULL;
