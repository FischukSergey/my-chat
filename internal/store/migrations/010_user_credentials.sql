-- Добавляем поля username и password_hash в таблицу users.
-- DEFAULT '' позволяет существующим строкам пережить миграцию;
-- уникальность обеспечивается partial index, исключающим пустые строки,
-- чтобы миграция была идемпотентной на БД с несколькими legacy-пользователями.

ALTER TABLE users ADD COLUMN IF NOT EXISTS username      TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN IF NOT EXISTS password_hash TEXT NOT NULL DEFAULT '';

-- Partial unique index: пустые username не участвуют в уникальности (legacy/migrated rows).
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_username
    ON users (username)
    WHERE username != '';
