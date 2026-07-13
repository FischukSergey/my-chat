// Package notification содержит логику обработки outbox-задач notification-worker.
package notification

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"time"

	"my-chat/internal/clients/push"
	"my-chat/internal/store"
)

const (
	backoffCap = 30 * time.Minute
	// exhaustedDelay — задержка для задач, исчерпавших попытки; эффективно выводит их из ротации.
	exhaustedDelay = 24 * time.Hour * 365
)

type outboxRepository interface {
	ClaimBatch(ctx context.Context, batchSize int) ([]store.NotificationOutbox, error)
	MarkSent(ctx context.Context, id string) error
	MarkFailed(ctx context.Context, id string, lastErr string, nextAttemptAt time.Time) error
}

type deviceRepository interface {
	ListActive(ctx context.Context, userID string) ([]store.Device, error)
}

// Config задаёт параметры работы Worker.
type Config struct {
	BatchSize   int
	MaxAttempts int
	BackoffBase time.Duration
}

// Worker обрабатывает outbox-задачи и отправляет push-уведомления через Provider.
type Worker struct {
	outbox   outboxRepository
	devices  deviceRepository
	provider push.Provider
	log      *slog.Logger
	cfg      Config
}

// NewWorker создаёт Worker.
func NewWorker(
	outbox outboxRepository,
	devices deviceRepository,
	provider push.Provider,
	log *slog.Logger,
	cfg Config,
) *Worker {
	return &Worker{
		outbox:   outbox,
		devices:  devices,
		provider: provider,
		log:      log,
		cfg:      cfg,
	}
}

// outboxPayload соответствует структуре payload из docs/api-sprint-2.md.
type outboxPayload struct {
	EventType   string `json:"event_type"`
	UserID      string `json:"user_id"`
	MessageID   string `json:"message_id"`
	DialogID    string `json:"dialog_id"`
	SenderID    string `json:"sender_id"`
	Preview     string `json:"preview"`
	UnreadCount int    `json:"unread_count"`
	DedupKey    string `json:"dedup_key"`
}

// RunOnce захватывает один батч задач и обрабатывает их.
// Возвращает количество обработанных задач (успешно или с ошибкой).
func (w *Worker) RunOnce(ctx context.Context) (int, error) {
	tasks, err := w.outbox.ClaimBatch(ctx, w.cfg.BatchSize)
	if err != nil {
		return 0, fmt.Errorf("claim outbox batch: %w", err)
	}

	for _, task := range tasks {
		w.processTask(ctx, task)
	}

	return len(tasks), nil
}

// Run запускает poll-loop с заданным интервалом до отмены ctx.
func (w *Worker) Run(ctx context.Context, pollInterval time.Duration) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	w.log.Info("notification worker запущен",
		slog.Duration("poll_interval", pollInterval),
		slog.Int("batch_size", w.cfg.BatchSize),
		slog.Int("max_attempts", w.cfg.MaxAttempts),
		slog.String("provider", w.provider.Name()),
	)

	for {
		select {
		case <-ctx.Done():
			w.log.Info("notification worker останавливается")
			return
		case <-ticker.C:
			n, err := w.RunOnce(ctx)
			if err != nil {
				w.log.Error("ошибка при обработке батча outbox", slog.String("error", err.Error()))
				continue
			}
			if n > 0 {
				w.log.Info("батч outbox обработан", slog.Int("processed", n))
			}
		}
	}
}

func (w *Worker) processTask(ctx context.Context, task store.NotificationOutbox) {
	start := time.Now()

	var payload outboxPayload
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		w.log.Error("не удалось распарсить payload outbox",
			slog.String("task_id", task.ID),
			slog.String("error", err.Error()),
		)
		w.markFailed(ctx, task, "unmarshal payload: "+err.Error(), start)
		return
	}

	devices, err := w.devices.ListActive(ctx, task.UserID)
	if err != nil {
		w.log.Error("ошибка при получении устройств пользователя",
			slog.String("task_id", task.ID),
			slog.String("user_id", task.UserID),
			slog.String("error", err.Error()),
		)
		w.markFailed(ctx, task, "list devices: "+err.Error(), start)
		return
	}

	if len(devices) == 0 {
		w.logAttempt(task, payload, "skipped_no_devices", "", time.Since(start))
		if err = w.outbox.MarkSent(ctx, task.ID); err != nil {
			w.log.Error("ошибка MarkSent (no devices)",
				slog.String("task_id", task.ID),
				slog.String("error", err.Error()),
			)
		}
		return
	}

	var sendErr error
	for _, device := range devices {
		msg := push.Message{
			Device:      device,
			EventType:   payload.EventType,
			UserID:      payload.UserID,
			MessageID:   payload.MessageID,
			DialogID:    payload.DialogID,
			SenderID:    payload.SenderID,
			Preview:     payload.Preview,
			UnreadCount: payload.UnreadCount,
			DedupKey:    payload.DedupKey,
		}
		if err = w.provider.Send(ctx, msg); err != nil {
			sendErr = fmt.Errorf("device %s (%s): %w", device.ID, device.Platform, err)
			break
		}
	}

	elapsed := time.Since(start)

	if sendErr != nil {
		w.logAttempt(task, payload, "failed", sendErr.Error(), elapsed)
		w.markFailed(ctx, task, sendErr.Error(), start)
		return
	}

	w.logAttempt(task, payload, "sent", "", elapsed)
	if err = w.outbox.MarkSent(ctx, task.ID); err != nil {
		w.log.Error("ошибка MarkSent",
			slog.String("task_id", task.ID),
			slog.String("error", err.Error()),
		)
	}
}

func (w *Worker) markFailed(ctx context.Context, task store.NotificationOutbox, errMsg string, start time.Time) {
	elapsed := time.Since(start)

	if task.Attempt >= w.cfg.MaxAttempts {
		w.log.Warn("push_attempt_exhausted",
			slog.String("task_id", task.ID),
			slog.String("event_type", task.EventType),
			slog.String("user_id", task.UserID),
			slog.Int("attempt", task.Attempt),
			slog.Int("max_attempts", w.cfg.MaxAttempts),
			slog.String("last_error", errMsg),
			slog.Int64("duration_ms", elapsed.Milliseconds()),
		)
		next := time.Now().UTC().Add(exhaustedDelay)
		if err := w.outbox.MarkFailed(ctx, task.ID, errMsg, next); err != nil {
			w.log.Error("ошибка MarkFailed (exhausted)",
				slog.String("task_id", task.ID),
				slog.String("error", err.Error()),
			)
		}
		return
	}

	backoff := calcBackoff(w.cfg.BackoffBase, task.Attempt)
	next := time.Now().UTC().Add(backoff)

	if err := w.outbox.MarkFailed(ctx, task.ID, errMsg, next); err != nil {
		w.log.Error("ошибка MarkFailed",
			slog.String("task_id", task.ID),
			slog.String("error", err.Error()),
		)
	}
}

func (w *Worker) logAttempt(task store.NotificationOutbox, payload outboxPayload, status, errMsg string, elapsed time.Duration) {
	args := []any{
		slog.String("task_id", task.ID),
		slog.String("event_type", task.EventType),
		slog.String("user_id", task.UserID),
		slog.String("message_id", payload.MessageID),
		slog.Int("attempt", task.Attempt),
		slog.String("provider", w.provider.Name()),
		slog.String("status", status),
		slog.Int64("duration_ms", elapsed.Milliseconds()),
	}
	if errMsg != "" {
		args = append(args, slog.String("error", errMsg))
	}
	w.log.Info("push_attempt", args...)
}

// calcBackoff вычисляет задержку по формуле base * 2^(attempt-1) с капом backoffCap.
func calcBackoff(base time.Duration, attempt int) time.Duration {
	if attempt <= 0 {
		return base
	}
	exp := math.Pow(2, float64(attempt-1))
	d := time.Duration(float64(base) * exp)
	if d > backoffCap {
		return backoffCap
	}
	return d
}
