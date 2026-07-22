# my-chat mobile client

Минимальный мобильный клиент на **Vanilla TypeScript + Vite + Capacitor 8**.

Реализует auth-flow Sprint 3:
- Login по `user_id` (dev-режим)
- Secure storage refresh token (iOS Keychain)
- Biometric gate (Face ID / Touch ID) перед чтением токена
- Auto-refresh access при 401
- Обработка `session_compromised` → wipe + redirect login
- Smoke: `GET /api/v1/me/unread-count` после unlock

## Требования

| Инструмент | Версия |
|---|---|
| Node.js | ≥ 22 (рекомендуется) или 18/20 |
| Xcode | ≥ 15 (для iOS Simulator) |
| CocoaPods | ≥ 1.14 |

Установить CocoaPods, если нет:
```bash
sudo gem install cocoapods
```

## Первый запуск на iOS Simulator

```bash
# 1. Установить зависимости
cd mobile/
npm install

# 2. Добавить iOS платформу (один раз)
npx cap add ios

# 3. Собрать web-часть + синхронизировать с Xcode-проектом
npm run build
npx cap sync ios

# 4. Открыть в Xcode (выбрать симулятор и нажать Run)
npx cap open ios
```

Или одной командой (сборка + синк + открыть Xcode):
```bash
npm run cap:ios
```

## Конфигурация URL

В `src/index.html` найдите блок `<script>` с `window.APP_CONFIG`:

```js
window.APP_CONFIG = {
  authUrl: "http://localhost:33081",  // auth-proxy
  apiUrl:  "http://localhost:8080",   // main-service
};
```

По умолчанию настроен на локальный бэкенд. Симулятор обращается к `localhost` хост-машины — менять не нужно.

## Структура проекта

```
mobile/
  src/
    index.html    — три экрана: Login, Unlock, Home
    main.ts       — routing и логика экранов
    auth.ts       — secure storage + biometric gate
    api.ts        — HTTP client с auto-refresh при 401
  capacitor.config.ts
  vite.config.ts
  tsconfig.json
  package.json
```

## Экраны и flow

```
[нет refresh token]
       ↓
   Login screen  ─── ввод user_id → POST /auth/login → сохранить токены
       ↓
[cold start / есть refresh token]
       ↓
  Unlock screen  ─── Face ID prompt → GET refresh из Keychain
       ↓
   POST /auth/refresh → обновить access + refresh
       ↓
   Home screen   ─── показать unread count
```

### Обработка ошибок

| Событие | Действие |
|---|---|
| `session_compromised` | wipe tokens + redirect Login |
| `session_expired` | wipe tokens + redirect Login |
| `session_revoked` | wipe tokens + redirect Login |
| Биометрия отменена | показать кнопку «Попробовать снова» |
| Биометрия недоступна (lockout / смена) | wipe tokens + redirect Login |

## Тестирование биометрии в симуляторе

В запущенном симуляторе:
```
Меню: Features → Face ID → Matching Face     ← успешная биометрия
Меню: Features → Face ID → Non-matching Face ← отказ
```

## Требования к бэкенду

Запустите локальный стек перед запуском приложения:
```bash
cd /path/to/my-chat
docker compose -f deploy/local/docker-compose.local.yml up -d --build --wait
```

Тестовые пользователи (предзагружены в БД):
- User A: `11111111-1111-1111-1111-111111111111`
- User B: `22222222-2222-2222-2222-222222222222`
