// Package push содержит абстракцию push-провайдера и его реализации.
package push

import (
	"context"

	"my-chat/internal/store"
)

// Message — данные для отправки одного push-уведомления на конкретное устройство.
type Message struct {
	Device      store.Device
	EventType   string
	UserID      string
	MessageID   string
	DialogID    string
	SenderID    string
	Preview     string
	UnreadCount int
	DedupKey    string
}

// Provider описывает абстракцию push-провайдера.
type Provider interface {
	// Name возвращает идентификатор провайдера для логирования.
	Name() string
	// Send отправляет push-уведомление на одно устройство.
	Send(ctx context.Context, msg Message) error
}
