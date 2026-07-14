package push

import (
	"context"
	"log/slog"
)

// DevLogProvider имитирует отправку push через структурированный лог.
// Используется в local/dev окружении; всегда возвращает успех.
type DevLogProvider struct {
	log *slog.Logger
}

// NewDevLogProvider создаёт DevLogProvider.
func NewDevLogProvider(log *slog.Logger) *DevLogProvider {
	return &DevLogProvider{log: log}
}

// Name возвращает идентификатор провайдера.
func (p *DevLogProvider) Name() string { return "dev-log" }

// Send логирует данные push-уведомления и возвращает nil.
func (p *DevLogProvider) Send(_ context.Context, msg Message) error {
	p.log.Info("dev_push_sent",
		slog.String("platform", msg.Device.Platform),
		slog.String("push_token", msg.Device.PushToken),
		slog.String("user_id", msg.UserID),
		slog.String("message_id", msg.MessageID),
		slog.String("dialog_id", msg.DialogID),
		slog.String("sender_id", msg.SenderID),
		slog.String("preview", msg.Preview),
		slog.Int("unread_count", msg.UnreadCount),
		slog.Int("badge", msg.Badge),
		slog.String("dedup_key", msg.DedupKey),
	)

	return nil
}
