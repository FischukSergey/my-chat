CREATE TABLE IF NOT EXISTS dialogs (
    id UUID PRIMARY KEY,
    user_a_id UUID NOT NULL REFERENCES users(id),
    user_b_id UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT dialogs_users_different CHECK (user_a_id <> user_b_id),
    CONSTRAINT dialogs_ordered_pair CHECK (user_a_id < user_b_id),
    CONSTRAINT dialogs_unique_pair UNIQUE (user_a_id, user_b_id)
);
