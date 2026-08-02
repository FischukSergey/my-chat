-- Sprint 6: добавляем push_subscription (JSONB) для Web Push и делаем push_token nullable.
-- Старый push_token оставляем для возможного будущего использования (ios/android).

-- 1. Добавить колонку push_subscription (идемпотентно).
ALTER TABLE devices ADD COLUMN IF NOT EXISTS push_subscription JSONB;

-- 2. Снять NOT NULL с push_token: web-устройства не имеют push_token.
--    CHECK-ограничения корректно пропускают NULL (NULL > 0 → NULL → не FALSE).
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
          FROM information_schema.columns
         WHERE table_schema = 'public'
           AND table_name   = 'devices'
           AND column_name  = 'push_token'
           AND is_nullable  = 'NO'
    ) THEN
        ALTER TABLE devices ALTER COLUMN push_token DROP NOT NULL;
    END IF;
END $$;

-- 3. Partial unique index для web-подписок: уникальность по (user_id, endpoint).
--    Позволяет использовать ON CONFLICT при upsert для platform='web'.
CREATE UNIQUE INDEX IF NOT EXISTS devices_web_endpoint_unique
    ON devices (user_id, (push_subscription->>'endpoint'))
    WHERE platform = 'web';
