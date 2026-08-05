-- X-Device-ID — клиентский идентификатор привязки сессии (localStorage UUID),
-- а не FK на devices.id. Устройство регистрируется позже (после login + push),
-- поэтому FK ломал login с 500 (violates foreign key constraint).

ALTER TABLE auth_sessions
    DROP CONSTRAINT IF EXISTS auth_sessions_device_id_fkey;

-- Username case-insensitive: нормализуем существующие значения.
UPDATE users
SET username = lower(username)
WHERE username <> '' AND username <> lower(username);

-- Пересоздаём unique index на lower(username) только если ещё старый
-- индекс по (username) или индекса нет. Не DROP на каждом повторном Migrate().
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM pg_indexes
        WHERE schemaname = 'public'
          AND indexname = 'idx_users_username'
          AND indexdef NOT ILIKE '%lower(%'
    ) THEN
        DROP INDEX idx_users_username;
    END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_username
    ON users (lower(username))
    WHERE username <> '';
