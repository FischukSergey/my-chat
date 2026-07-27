// Package expirer содержит логику обнаружения и мягкого удаления истёкших сообщений.
package expirer

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"my-chat/internal/store"
)

type messageRepository interface {
	ExpireMessages(ctx context.Context, now time.Time, batchSize int) ([]store.ExpiredMessage, error)
}

type eventPublisher interface {
	EnqueueBatch(ctx context.Context, events []store.WSEventOutbox) error
}

// Expirer обнаруживает истёкшие сообщения и публикует WS-события в outbox.
type Expirer struct {
	repo      messageRepository
	publisher eventPublisher
	log       *slog.Logger
	batchSize int
}

// New создаёт Expirer.
func New(repo messageRepository, publisher eventPublisher, log *slog.Logger, batchSize int) *Expirer {
	return &Expirer{
		repo:      repo,
		publisher: publisher,
		log:       log,
		batchSize: batchSize,
	}
}

// messageDeletedPayload — payload WS-события message_deleted.
// Формат зафиксирован в docs/api-sprint-4.md.
type messageDeletedPayload struct {
	Type      string `json:"type"`
	MessageID string `json:"message_id"`
	DialogID  string `json:"dialog_id"`
}

// Tick выполняет одну итерацию: помечает истёкшие сообщения удалёнными
// и публикует события message_deleted в ws_event_outbox для обоих участников диалога.
// Возвращает число обработанных сообщений.
func (e *Expirer) Tick(ctx context.Context) (int, error) {
	start := time.Now()

	expired, err := e.repo.ExpireMessages(ctx, time.Now(), e.batchSize)
	if err != nil {
		return 0, fmt.Errorf("expire messages: %w", err)
	}

	if len(expired) == 0 {
		return 0, nil
	}

	events, err := buildEvents(expired)
	if err != nil {
		return 0, fmt.Errorf("build ws events: %w", err)
	}

	if err = e.publisher.EnqueueBatch(ctx, events); err != nil {
		return 0, fmt.Errorf("enqueue ws events: %w", err)
	}

	e.log.Info("message_expired",
		slog.Int("count", len(expired)),
		slog.Int64("duration_ms", time.Since(start).Milliseconds()),
	)

	return len(expired), nil
}

// buildEvents формирует два WS-события (для каждого участника диалога) на каждое истёкшее сообщение.
func buildEvents(expired []store.ExpiredMessage) ([]store.WSEventOutbox, error) {
	events := make([]store.WSEventOutbox, 0, len(expired)*2)

	for _, msg := range expired {
		payload, err := json.Marshal(messageDeletedPayload{
			Type:      "message_deleted",
			MessageID: msg.ID,
			DialogID:  msg.DialogID,
		})
		if err != nil {
			return nil, fmt.Errorf("marshal payload for message %s: %w", msg.ID, err)
		}

		events = append(events,
			store.WSEventOutbox{ID: uuid.NewString(), EventType: "message_deleted", UserID: msg.UserAID, Payload: payload},
			store.WSEventOutbox{ID: uuid.NewString(), EventType: "message_deleted", UserID: msg.UserBID, Payload: payload},
		)
	}

	return events, nil
}
