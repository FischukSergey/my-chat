// Package wsdelivery содержит логику доставки WS-событий из outbox подключённым клиентам.
package wsdelivery

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"my-chat/internal/hub"
	"my-chat/internal/store"
)

type wsOutboxRepository interface {
	ClaimBatch(ctx context.Context, batchSize int) ([]store.WSEventOutbox, error)
	MarkProcessedBatch(ctx context.Context, ids []string) error
}

type eventSender interface {
	Send(ctx context.Context, userID string, event hub.Event) bool
}

// Delivery читает события из ws_event_outbox и рассылает их подключённым клиентам через Hub.
type Delivery struct {
	outbox    wsOutboxRepository
	hub       eventSender
	log       *slog.Logger
	batchSize int
}

// New создаёт Delivery.
func New(outbox wsOutboxRepository, h eventSender, log *slog.Logger, batchSize int) *Delivery {
	return &Delivery{
		outbox:    outbox,
		hub:       h,
		log:       log,
		batchSize: batchSize,
	}
}

// RunOnce читает один батч из outbox, отправляет через Hub и помечает обработанными.
// Возвращает количество доставленных событий.
func (d *Delivery) RunOnce(ctx context.Context) (int, error) {
	events, err := d.outbox.ClaimBatch(ctx, d.batchSize)
	if err != nil {
		return 0, fmt.Errorf("claim ws event batch: %w", err)
	}

	if len(events) == 0 {
		return 0, nil
	}

	ids := make([]string, 0, len(events))
	for _, e := range events {
		wsEvent, parseErr := parseEvent(e)
		if parseErr != nil {
			d.log.Warn("ws_delivery: не удалось разобрать payload события",
				slog.String("event_id", e.ID),
				slog.String("event_type", e.EventType),
				slog.String("error", parseErr.Error()),
			)
			// Помечаем как обработанное, чтобы не застревать на битом событии.
			ids = append(ids, e.ID)
			continue
		}

		online := d.hub.Send(ctx, e.UserID, wsEvent)
		if !online {
			d.log.Debug("ws_delivery: пользователь оффлайн, событие пропущено",
				slog.String("event_type", e.EventType),
				slog.String("user_id", e.UserID),
			)
		}

		ids = append(ids, e.ID)
	}

	if err = d.outbox.MarkProcessedBatch(ctx, ids); err != nil {
		return 0, fmt.Errorf("mark ws events processed: %w", err)
	}

	return len(events), nil
}

// Run запускает poll-loop с заданным интервалом до отмены ctx.
func (d *Delivery) Run(ctx context.Context, pollInterval time.Duration) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	d.log.Info("ws_delivery: поллер запущен",
		slog.Duration("poll_interval", pollInterval),
		slog.Int("batch_size", d.batchSize),
	)

	for {
		select {
		case <-ctx.Done():
			d.log.Info("ws_delivery: поллер остановлен")
			return
		case <-ticker.C:
			n, err := d.RunOnce(ctx)
			if err != nil {
				d.log.Error("ws_delivery: ошибка обработки батча", slog.String("error", err.Error()))
				continue
			}
			if n > 0 {
				d.log.Info("ws_delivery: событий доставлено", slog.Int("count", n))
			}
		}
	}
}

// parseEvent преобразует outbox-запись в hub.Event.
func parseEvent(e store.WSEventOutbox) (hub.Event, error) {
	var payload json.RawMessage
	if err := json.Unmarshal(e.Payload, &payload); err != nil {
		return hub.Event{}, fmt.Errorf("unmarshal payload: %w", err)
	}

	return hub.NewEvent(e.EventType, payload), nil
}
