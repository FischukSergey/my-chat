CREATE TABLE IF NOT EXISTS devices (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    platform TEXT NOT NULL,
    push_token TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT devices_platform_allowed CHECK (platform IN ('ios', 'android', 'web')),
    CONSTRAINT devices_push_token_not_empty CHECK (length(trim(push_token)) > 0),
    CONSTRAINT devices_push_token_max_len CHECK (length(push_token) <= 1024),
    CONSTRAINT devices_user_platform_token_unique UNIQUE (user_id, platform, push_token)
);

CREATE INDEX IF NOT EXISTS devices_user_enabled_idx
    ON devices (user_id, enabled);
