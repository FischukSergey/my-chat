//go:build integration

// Package store_test содержит интеграционные тесты репозиториев devices и notification_outbox.
package store_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"my-chat/internal/store"
)

// setupDB подключается к тестовой БД, прогоняет миграции и возвращает *store.Store.

const (
	storeTestPlatformIOS = "ios"
	storeTestEventNew    = "message_new"
)

func setupDB(t *testing.T) *store.Store {
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

// insertUser создает пользователя и регистрирует его удаление в Cleanup.
func insertUser(t *testing.T, ctx context.Context, s *store.Store) string {
	t.Helper()

	id := uuid.NewString()
	_, err := s.DB().Exec(ctx, "INSERT INTO users (id) VALUES ($1)", id)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	t.Cleanup(func() {
		_, _ = s.DB().Exec(ctx, "DELETE FROM users WHERE id = $1", id)
	})

	return id
}

// --- DeviceRepository ---

func TestDeviceRepository_Upsert_NewDevice(t *testing.T) {
	s := setupDB(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, s)

	repo := store.NewDeviceRepository(s)

	d, err := repo.Upsert(ctx, store.Device{
		ID:        uuid.NewString(),
		UserID:    userID,
		Platform:  storeTestPlatformIOS,
		PushToken: "token-abc",
	})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if d.UserID != userID {
		t.Errorf("expected UserID %q, got %q", userID, d.UserID)
	}
	if !d.Enabled {
		t.Error("expected device to be enabled")
	}
}

func TestDeviceRepository_Upsert_Dedup_ReenablesDevice(t *testing.T) {
	s := setupDB(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, s)

	repo := store.NewDeviceRepository(s)

	base := store.Device{
		ID:        uuid.NewString(),
		UserID:    userID,
		Platform:  storeTestPlatformIOS,
		PushToken: "token-same",
	}

	// Первая регистрация.
	d1, err := repo.Upsert(ctx, base)
	if err != nil {
		t.Fatalf("first Upsert: %v", err)
	}

	// Деактивируем устройство вручную.
	if err = repo.Disable(ctx, userID, "token-same"); err != nil {
		t.Fatalf("Disable: %v", err)
	}

	// Повторная регистрация с тем же токеном — должна включить устройство обратно.
	base.ID = uuid.NewString() // новый UUID, но ON CONFLICT должен сработать
	d2, err := repo.Upsert(ctx, base)
	if err != nil {
		t.Fatalf("second Upsert: %v", err)
	}

	if d2.ID != d1.ID {
		t.Errorf("expected same device ID after conflict, got %q vs %q", d1.ID, d2.ID)
	}
	if !d2.Enabled {
		t.Error("expected device to be re-enabled after second upsert")
	}
}

func TestDeviceRepository_Disable(t *testing.T) {
	s := setupDB(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, s)

	repo := store.NewDeviceRepository(s)

	_, err := repo.Upsert(ctx, store.Device{
		ID:        uuid.NewString(),
		UserID:    userID,
		Platform:  "android",
		PushToken: "token-disable-test",
	})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	if err = repo.Disable(ctx, userID, "token-disable-test"); err != nil {
		t.Fatalf("Disable: %v", err)
	}

	devices, err := repo.ListActive(ctx, userID)
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if len(devices) != 0 {
		t.Errorf("expected 0 active devices after disable, got %d", len(devices))
	}
}

func TestDeviceRepository_Disable_NonexistentToken_NoError(t *testing.T) {
	s := setupDB(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, s)

	repo := store.NewDeviceRepository(s)

	// Отключение несуществующего токена не должно возвращать ошибку.
	if err := repo.Disable(ctx, userID, "no-such-token"); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestDeviceRepository_ListActive_FiltersDisabled(t *testing.T) {
	s := setupDB(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, s)

	repo := store.NewDeviceRepository(s)

	// Регистрируем два устройства.
	for _, token := range []string{"token-active", "token-to-disable"} {
		if _, err := repo.Upsert(ctx, store.Device{
			ID:        uuid.NewString(),
			UserID:    userID,
			Platform:  storeTestPlatformIOS,
			PushToken: token,
		}); err != nil {
			t.Fatalf("Upsert %q: %v", token, err)
		}
	}

	// Отключаем одно.
	if err := repo.Disable(ctx, userID, "token-to-disable"); err != nil {
		t.Fatalf("Disable: %v", err)
	}

	devices, err := repo.ListActive(ctx, userID)
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if len(devices) != 1 {
		t.Errorf("expected 1 active device, got %d", len(devices))
	}
	if devices[0].PushToken != "token-active" {
		t.Errorf("unexpected active token: %q", devices[0].PushToken)
	}
}

// --- NotificationOutboxRepository ---

func TestNotificationOutboxRepository_Enqueue_Success(t *testing.T) {
	s := setupDB(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, s)

	repo := store.NewNotificationOutboxRepository(s)

	task := store.NotificationOutbox{
		ID:        uuid.NewString(),
		EventType: storeTestEventNew,
		UserID:    userID,
		Payload:   []byte(`{"message_id":"m1"}`),
		DedupKey:  "message_new:" + uuid.NewString() + ":" + userID,
	}

	if err := repo.Enqueue(ctx, task); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
}

func TestNotificationOutboxRepository_Enqueue_DedupKey_IgnoresDuplicate(t *testing.T) {
	s := setupDB(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, s)

	repo := store.NewNotificationOutboxRepository(s)

	dedupKey := "message_new:" + uuid.NewString() + ":" + userID

	first := store.NotificationOutbox{
		ID:        uuid.NewString(),
		EventType: storeTestEventNew,
		UserID:    userID,
		Payload:   []byte(`{"message_id":"m1"}`),
		DedupKey:  dedupKey,
	}

	if err := repo.Enqueue(ctx, first); err != nil {
		t.Fatalf("first Enqueue: %v", err)
	}

	// Второй вызов с тем же dedupKey не должен возвращать ошибку.
	second := first
	second.ID = uuid.NewString()
	if err := repo.Enqueue(ctx, second); err != nil {
		t.Errorf("second Enqueue (dedup) must not fail, got: %v", err)
	}
}

func TestNotificationOutboxRepository_ClaimBatch_ReturnsPendingTasks(t *testing.T) {
	s := setupDB(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, s)

	repo := store.NewNotificationOutboxRepository(s)

	msgID := uuid.NewString()
	dedupKey := "message_new:" + msgID + ":" + userID

	if err := repo.Enqueue(ctx, store.NotificationOutbox{
		ID:        uuid.NewString(),
		EventType: storeTestEventNew,
		UserID:    userID,
		Payload:   []byte(`{"message_id":"` + msgID + `"}`),
		DedupKey:  dedupKey,
	}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	tasks, err := repo.ClaimBatch(ctx, 10)
	if err != nil {
		t.Fatalf("ClaimBatch: %v", err)
	}
	if len(tasks) == 0 {
		t.Fatal("expected at least one task in batch")
	}

	var found bool
	for _, task := range tasks {
		if task.DedupKey == dedupKey {
			found = true
			if task.Attempt != 1 {
				t.Errorf("expected attempt=1 after claim, got %d", task.Attempt)
			}
		}
	}
	if !found {
		t.Errorf("enqueued task not found in claimed batch")
	}
}

func TestNotificationOutboxRepository_MarkSent(t *testing.T) {
	s := setupDB(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, s)

	repo := store.NewNotificationOutboxRepository(s)

	taskID := uuid.NewString()
	dedupKey := "message_new:sent-test:" + userID

	if err := repo.Enqueue(ctx, store.NotificationOutbox{
		ID:        taskID,
		EventType: storeTestEventNew,
		UserID:    userID,
		Payload:   []byte(`{}`),
		DedupKey:  dedupKey,
	}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	if _, err := repo.ClaimBatch(ctx, 10); err != nil {
		t.Fatalf("ClaimBatch: %v", err)
	}

	if err := repo.MarkSent(ctx, taskID); err != nil {
		t.Fatalf("MarkSent: %v", err)
	}

	// Задача в статусе 'sent' не должна попасть в следующий батч.
	tasks, err := repo.ClaimBatch(ctx, 10)
	if err != nil {
		t.Fatalf("second ClaimBatch: %v", err)
	}
	for _, task := range tasks {
		if task.ID == taskID {
			t.Error("sent task must not appear in subsequent ClaimBatch")
		}
	}
}

func TestNotificationOutboxRepository_MarkFailed_PostponesRetry(t *testing.T) {
	s := setupDB(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, s)

	repo := store.NewNotificationOutboxRepository(s)

	taskID := uuid.NewString()
	dedupKey := "message_new:fail-test:" + userID

	if err := repo.Enqueue(ctx, store.NotificationOutbox{
		ID:        taskID,
		EventType: storeTestEventNew,
		UserID:    userID,
		Payload:   []byte(`{}`),
		DedupKey:  dedupKey,
	}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	if _, err := repo.ClaimBatch(ctx, 10); err != nil {
		t.Fatalf("ClaimBatch: %v", err)
	}

	// Откладываем следующую попытку на 1 час.
	nextAttempt := time.Now().UTC().Add(time.Hour)
	if err := repo.MarkFailed(ctx, taskID, "push provider error", nextAttempt); err != nil {
		t.Fatalf("MarkFailed: %v", err)
	}

	// Задача не должна попасть в батч, пока не наступит next_attempt_at.
	tasks, err := repo.ClaimBatch(ctx, 10)
	if err != nil {
		t.Fatalf("second ClaimBatch: %v", err)
	}
	for _, task := range tasks {
		if task.ID == taskID {
			t.Error("failed task with future next_attempt_at must not appear in ClaimBatch")
		}
	}
}
