-- Тестовые пользователи для локальной разработки.
-- Пароль для обоих: password123  (bcrypt cost=12)
-- Запуск: psql "$DATABASE_DSN" -f deploy/local/seed-users.sql
--      или через docker compose:
--   docker compose -f deploy/local/docker-compose.local.yml exec postgres \
--     psql -U chat_service -d chat_service -f /dev/stdin < deploy/local/seed-users.sql

INSERT INTO users (id, status, username, password_hash)
VALUES
    (
        '11111111-1111-1111-1111-111111111111',
        'active',
        'alice',
        '$2a$12$KmAOkyDgSsP3MwHqDdpFouy7O04Oxw4O/s7HyhvdHZybFPkzo5mpW'
    ),
    (
        '22222222-2222-2222-2222-222222222222',
        'active',
        'bob',
        '$2a$12$99383/nHvAjzXuaB2vkgYeJCfjZ6i.pHR6miEgvl6rG6ycpZ7c/Km'
    )
ON CONFLICT (id) DO UPDATE
    SET username      = EXCLUDED.username,
        password_hash = EXCLUDED.password_hash,
        status        = EXCLUDED.status;
