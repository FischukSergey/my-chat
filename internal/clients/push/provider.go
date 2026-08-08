// Package push содержит абстракцию push-провайдера и его реализации.
package push

import (
	"context"
	"errors"

	"my-chat/internal/store"
)

// Message — данные для отправки одного push-уведомления на конкретное устройство.
type Message struct {
	Device         store.Device
	EventType      string
	UserID         string
	MessageID      string
	DialogID       string
	SenderID       string
	SenderUsername string // Sprint 8: title push = username; fallback "user"
	Preview        string // для логов/будущего; не мапится в notification title
	UnreadCount    int
	// Badge — число на иконке приложения. В Sprint 2 равно UnreadCount;
	// выделено в отдельное поле для совместимости с APNs/FCM.
	Badge    int
	DedupKey string
}

// Provider описывает абстракцию push-провайдера.
type Provider interface {
	// Name возвращает идентификатор провайдера для логирования.
	Name() string
	// Send отправляет push-уведомление на одно устройство.
	Send(ctx context.Context, msg Message) error
}

// ErrSubscriptionGone сигнализирует, что push-подписка устарела (HTTP 404/410 от push-сервера).
// Worker при получении этой ошибки деактивирует устройство без повторных попыток.
var ErrSubscriptionGone = errors.New("push subscription gone")
