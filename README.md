# my-chat

[![CI](https://github.com/FischukSergey/my-chat/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/FischukSergey/my-chat/actions/workflows/ci.yml)

Бэкенд чат-сервиса с WebSocket, JWT-аутентификацией и prod-деплоем на VPS через HTTPS/WSS.

## Сервисы

| Сервис | Описание |
|---|---|
| `main-service` | REST API + WebSocket, бизнес-логика чата |
| `auth-proxy` | Аутентификация (login/refresh/logout) |
| `notification-worker` | Фоновая рассылка push-уведомлений |
| `message-expirer` | Удаление истёкших сообщений |

## Быстрый старт (локально)

```bash
cp configs/config.main-service.local.example.yaml configs/config.main-service.local.yaml
# заполнить конфиг

task local:up      # поднять стек через Docker Compose
task test          # юнит-тесты
task test:integration  # интеграционные тесты
task lint          # линтер
```

## Prod-деплой

Домен: **https://beepru.ru**

```bash
# На VPS: первичное получение SSL-сертификата
bash deploy/prod/init-ssl.sh --staging   # тест
bash deploy/prod/init-ssl.sh             # боевой сертификат
```

Подробнее: [`docs/sprint-5-plan.md`](docs/sprint-5-plan.md)
