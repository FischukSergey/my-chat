-- X-Device-ID — клиентский идентификатор привязки сессии (localStorage UUID),
-- а не FK на devices.id. Устройство регистрируется позже (после login + push),
-- поэтому FK ломал login с 500 (violates foreign key constraint).

ALTER TABLE auth_sessions
    DROP CONSTRAINT IF EXISTS auth_sessions_device_id_fkey;

-- Username case-insensitive: нормализуем существующие значения и уникальность.
UPDATE users
SET username = lower(username)
WHERE username <> '' AND username <> lower(username);

DROP INDEX IF EXISTS idx_users_username;

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_username
    ON users (lower(username))
    WHERE username <> '';
