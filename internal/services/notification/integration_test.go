//go:build integration

// Package notification_test contains integration tests for the notification worker.
package notification_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"my-chat/internal/clients/push"
	"my-chat/internal/services/notification"
	"my-chat/internal/store"
)

// setupIntegrationDB подключается к тестовой БД, прогоняет миграции и возвращает *store.Store.
func setupIntegrationDB(t *testing.T) *store.Store {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	ctx := context.Background()

	s, err := store.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect to db: %v", err)
	}
	t.Cleanup(s.Close)

	if _, err = s.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	return s
}

// insertUserForWorker создаёт пользователя в БД и регистрирует его удаление.
func insertUserForWorker(t *testing.T, ctx context.Context, s *store.Store) string {
	t.Helper()

	id := uuid.NewString()
	if _, err := s.DB().Exec(ctx, "INSERT INTO users (id) VALUES ($1)", id); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = s.DB().Exec(ctx, "DELETE FROM users WHERE id = $1", id)
	})

	return id
}

// makeOutboxTask формирует и вставляет outbox-задачу в БД напрямую.
func makeOutboxTask(t *testing.T, ctx context.Context, s *store.Store, userID string) store.NotificationOutbox {
	t.Helper()

	msgID := uuid.NewString()
	rawPayload, err := json.Marshal(map[string]any{
		"event_type":   "message_new",
		"user_id":      userID,
		"message_id":   msgID,
		"dialog_id":    uuid.NewString(),
		"sender_id":    uuid.NewString(),
		"preview":      "integration test push",
		"unread_count": 1,
		"dedup_key":    "message_new:" + msgID + ":" + userID,
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	task := store.NotificationOutbox{
		ID:        uuid.NewString(),
		EventType: "message_new",
		UserID:    userID,
		Payload:   rawPayload,
		DedupKey:  "message_new:" + msgID + ":" + userID,
	}

	outboxRepo := store.NewNotificationOutboxRepository(s)
	if err = outboxRepo.Enqueue(ctx, task); err != nil {
		t.Fatalf("enqueue outbox task: %v", err)
	}

	t.Cleanup(func() {
		_, _ = s.DB().Exec(ctx, "DELETE FROM notification_outbox WHERE id = $1", task.ID)
	})

	return task
}

// TestIntegration_WorkerProcessesOutbox_MarksSent verifies the end-to-end path:
// outbox task in DB -> Worker.RunOnce -> task status = 'sent'.
func TestIntegration_WorkerProcessesOutbox_MarksSent(t *testing.T) {
	s := setupIntegrationDB(t)
	ctx := context.Background()

	userID := insertUserForWorker(t, ctx, s)

	// Register a device for the user so the worker actually calls the provider.
	deviceRepo := store.NewDeviceRepository(s)
	_, err := deviceRepo.Upsert(ctx, store.Device{
		ID:        uuid.NewString(),
		UserID:    userID,
		Platform:  "ios",
		PushToken: "integration-test-token",
	})
	if err != nil {
		t.Fatalf("upsert device: %v", err)
	}
	t.Cleanup(func() {
		_, _ = s.DB().Exec(ctx, "DELETE FROM devices WHERE user_id = $1", userID)
	})

	task := makeOutboxTask(t, ctx, s, userID)

	outboxRepo := store.NewNotificationOutboxRepository(s)
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	provider := push.NewNoopProvider()

	worker := notification.NewWorker(outboxRepo, deviceRepo, provider, log, notification.Config{
		BatchSize:   10,
		MaxAttempts: 3,
		BackoffBase: 10 * time.Second,
	})

	n, err := worker.RunOnce(ctx)
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 task processed, got %d", n)
	}

	// Verify the task is now 'sent' in the DB.
	var status string
	err = s.DB().QueryRow(ctx,
		"SELECT status FROM notification_outbox WHERE id = $1",
		task.ID,
	).Scan(&status)
	if err != nil {
		t.Fatalf("fetch task status: %v", err)
	}
	if status != string(store.OutboxStatusSent) {
		t.Errorf("expected task status=sent, got %q", status)
	}
}

// TestIntegration_WorkerProcessesOutbox_NoDevices_MarksSent verifies that when a user
// has no registered devices the task is still marked 'sent' (skipped gracefully).
func TestIntegration_WorkerProcessesOutbox_NoDevices_MarksSent(t *testing.T) {
	s := setupIntegrationDB(t)
	ctx := context.Background()

	userID := insertUserForWorker(t, ctx, s)
	// No device registered for this user.

	task := makeOutboxTask(t, ctx, s, userID)

	outboxRepo := store.NewNotificationOutboxRepository(s)
	deviceRepo := store.NewDeviceRepository(s)
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	provider := push.NewNoopProvider()

	worker := notification.NewWorker(outboxRepo, deviceRepo, provider, log, notification.Config{
		BatchSize:   10,
		MaxAttempts: 3,
		BackoffBase: 10 * time.Second,
	})

	n, err := worker.RunOnce(ctx)
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 task processed, got %d", n)
	}

	var status string
	err = s.DB().QueryRow(ctx,
		"SELECT status FROM notification_outbox WHERE id = $1",
		task.ID,
	).Scan(&status)
	if err != nil {
		t.Fatalf("fetch task status: %v", err)
	}
	if status != string(store.OutboxStatusSent) {
		t.Errorf("expected status=sent (no-devices skipped gracefully), got %q", status)
	}
}
