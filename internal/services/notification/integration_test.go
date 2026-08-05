//go:build integration

// Package notification_test contains integration tests for the notification worker.
package notification_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

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
		"event_type":   eventTypeMessageNew,
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
		EventType: eventTypeMessageNew,
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
		Platform:  platformIOSTest,
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
	receiptRepo := store.NewReceiptRepository(s)
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	provider := push.NewNoopProvider()

	worker := notification.NewWorker(outboxRepo, deviceRepo, receiptRepo, provider, log, notification.Config{
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

// TestIntegration_NotificationOutbox_DeleteSent проверяет, что DeleteSent удаляет
// только задачи со статусом 'sent', чей updated_at старше указанного порога.
func TestIntegration_NotificationOutbox_DeleteSent(t *testing.T) {
	s := setupIntegrationDB(t)
	ctx := context.Background()

	userID := insertUserForWorker(t, ctx, s)
	outboxRepo := store.NewNotificationOutboxRepository(s)

	// Вставляем 'sent' задачу с updated_at = 8 дней назад (должна удалиться).
	oldID := uuid.NewString()
	_, err := s.DB().Exec(ctx, `
		INSERT INTO notification_outbox
		            (id, event_type, user_id, payload, dedup_key, status,
		             next_attempt_at, created_at, updated_at)
		VALUES ($1, 'message_new', $2, '{}', $3, 'sent',
		        NOW(), NOW() - INTERVAL '8 days', NOW() - INTERVAL '8 days')
	`, oldID, userID, "dedup-old-"+oldID)
	if err != nil {
		t.Fatalf("insert old task: %v", err)
	}
	t.Cleanup(func() {
		_, _ = s.DB().Exec(ctx, "DELETE FROM notification_outbox WHERE id = $1", oldID)
	})

	// Вставляем 'sent' задачу с updated_at = 1 день назад (не должна удалиться).
	recentID := uuid.NewString()
	_, err = s.DB().Exec(ctx, `
		INSERT INTO notification_outbox
		            (id, event_type, user_id, payload, dedup_key, status,
		             next_attempt_at, created_at, updated_at)
		VALUES ($1, 'message_new', $2, '{}', $3, 'sent',
		        NOW(), NOW() - INTERVAL '1 day', NOW() - INTERVAL '1 day')
	`, recentID, userID, "dedup-recent-"+recentID)
	if err != nil {
		t.Fatalf("insert recent task: %v", err)
	}
	t.Cleanup(func() {
		_, _ = s.DB().Exec(ctx, "DELETE FROM notification_outbox WHERE id = $1", recentID)
	})

	// Вставляем 'pending' задачу с old updated_at (не должна удалиться — статус != 'sent').
	pendingID := uuid.NewString()
	_, err = s.DB().Exec(ctx, `
		INSERT INTO notification_outbox
		            (id, event_type, user_id, payload, dedup_key, status,
		             next_attempt_at, created_at, updated_at)
		VALUES ($1, 'message_new', $2, '{}', $3, 'pending',
		        NOW(), NOW() - INTERVAL '8 days', NOW() - INTERVAL '8 days')
	`, pendingID, userID, "dedup-pending-"+pendingID)
	if err != nil {
		t.Fatalf("insert pending task: %v", err)
	}
	t.Cleanup(func() {
		_, _ = s.DB().Exec(ctx, "DELETE FROM notification_outbox WHERE id = $1", pendingID)
	})

	// Запускаем очистку с порогом 7 суток.
	n, err := outboxRepo.DeleteSent(ctx, 7*24*time.Hour)
	if err != nil {
		t.Fatalf("DeleteSent: %v", err)
	}
	if n < 1 {
		t.Errorf("expected at least 1 deleted, got %d", n)
	}

	countRow := func(id string) int {
		var c int
		if scanErr := s.DB().QueryRow(ctx,
			"SELECT COUNT(*) FROM notification_outbox WHERE id = $1", id,
		).Scan(&c); scanErr != nil {
			t.Fatalf("count row %s: %v", id, scanErr)
		}
		return c
	}

	if countRow(oldID) != 0 {
		t.Errorf("old 'sent' task must be deleted, but still exists")
	}
	if countRow(recentID) != 1 {
		t.Errorf("recent 'sent' task must still exist")
	}
	if countRow(pendingID) != 1 {
		t.Errorf("'pending' task must not be deleted by DeleteSent")
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
	receiptRepo := store.NewReceiptRepository(s)
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	provider := push.NewNoopProvider()

	worker := notification.NewWorker(outboxRepo, deviceRepo, receiptRepo, provider, log, notification.Config{
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

// TestIntegration_RegisterLoginDeviceWeb_WebPushSent verifies the full flow:
// register user → register web device → enqueue outbox task → worker calls web push provider.
func TestIntegration_RegisterLoginDeviceWeb_WebPushSent(t *testing.T) {
	s := setupIntegrationDB(t)
	ctx := context.Background()

	// 1. Register user with username and bcrypt password hash.
	userID := uuid.NewString()
	username := "webtest_" + userID[:8]
	passwordHash, err := bcrypt.GenerateFromPassword([]byte("testpass99"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}

	_, err = s.DB().Exec(ctx,
		"INSERT INTO users (id, username, password_hash) VALUES ($1, $2, $3)",
		userID, username, string(passwordHash),
	)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = s.DB().Exec(ctx, "DELETE FROM users WHERE id = $1", userID)
	})

	// 2. Register a web device with push_subscription JSON.
	pushSub := `{"endpoint":"https://push.example.com/test-` + userID[:8] + `","keys":{"p256dh":"dGVzdA","auth":"dGVzdA"}}`
	deviceRepo := store.NewDeviceRepository(s)

	_, err = deviceRepo.Upsert(ctx, store.Device{
		ID:               uuid.NewString(),
		UserID:           userID,
		Platform:         "web",
		PushSubscription: pushSub,
	})
	if err != nil {
		t.Fatalf("upsert web device: %v", err)
	}
	t.Cleanup(func() {
		_, _ = s.DB().Exec(ctx, "DELETE FROM devices WHERE user_id = $1", userID)
	})

	// 3. Enqueue an outbox task targeting this user.
	task := makeOutboxTask(t, ctx, s, userID)

	// 4. Run worker with a tracking NoopProvider.
	var mu sync.Mutex
	var sentMessages []push.Message

	provider := push.NewNoopProvider()
	provider.SendFunc = func(_ context.Context, msg push.Message) error {
		mu.Lock()
		sentMessages = append(sentMessages, msg)
		mu.Unlock()
		return nil
	}

	outboxRepo := store.NewNotificationOutboxRepository(s)
	receiptRepo := store.NewReceiptRepository(s)
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	worker := notification.NewWorker(outboxRepo, deviceRepo, receiptRepo, provider, log, notification.Config{
		BatchSize:   10,
		MaxAttempts: 3,
		BackoffBase: 10 * time.Second,
	})

	n, runErr := worker.RunOnce(ctx)
	if runErr != nil {
		t.Fatalf("RunOnce: %v", runErr)
	}
	if n != 1 {
		t.Errorf("expected 1 task processed, got %d", n)
	}

	// 5. Verify task status is 'sent'.
	var taskStatus string
	if scanErr := s.DB().QueryRow(ctx,
		"SELECT status FROM notification_outbox WHERE id = $1", task.ID,
	).Scan(&taskStatus); scanErr != nil {
		t.Fatalf("fetch task status: %v", scanErr)
	}
	if taskStatus != string(store.OutboxStatusSent) {
		t.Errorf("expected task status=sent, got %q", taskStatus)
	}

	// 6. Verify web push provider.Send was called with the web device.
	mu.Lock()
	msgs := sentMessages
	mu.Unlock()

	if len(msgs) == 0 {
		t.Fatal("expected NoopProvider.Send to be called for web device, but it was not")
	}
	if msgs[0].Device.Platform != "web" {
		t.Errorf("expected device platform=web, got %q", msgs[0].Device.Platform)
	}
	if msgs[0].EventType != eventTypeMessageNew {
		t.Errorf("expected event_type=%q, got %q", eventTypeMessageNew, msgs[0].EventType)
	}
}
