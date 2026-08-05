package notification_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"my-chat/internal/clients/push"
	"my-chat/internal/services/notification"
	"my-chat/internal/store"
)

// --- fakes ---

const (
	eventTypeMessageNew = "message_new"
	platformIOSTest     = "ios"
	pushTokenTest       = "tok"
)

type fakeOutbox struct {
	mu        sync.Mutex
	tasks     []store.NotificationOutbox
	sentIDs   []string
	failedID  string
	failedAt  time.Time
	failedErr string
}

func (f *fakeOutbox) ClaimBatch(_ context.Context, batchSize int) ([]store.NotificationOutbox, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	n := batchSize
	if n > len(f.tasks) {
		n = len(f.tasks)
	}
	batch := f.tasks[:n]
	f.tasks = f.tasks[n:]
	return batch, nil
}

func (f *fakeOutbox) MarkSent(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sentIDs = append(f.sentIDs, id)
	return nil
}

func (f *fakeOutbox) MarkFailed(_ context.Context, id string, lastErr string, nextAttemptAt time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failedID = id
	f.failedErr = lastErr
	f.failedAt = nextAttemptAt
	return nil
}

type fakeDevices struct {
	devices    map[string][]store.Device
	disabledID string // последний ID, переданный в DisableByID
}

func (f *fakeDevices) ListActive(_ context.Context, userID string) ([]store.Device, error) {
	return f.devices[userID], nil
}

func (f *fakeDevices) DisableByID(_ context.Context, id string) error {
	f.disabledID = id
	return nil
}

// makeTask строит тестовую outbox-задачу с заданным числом attempt.
func makeTask(t *testing.T, userID string, attempt int) store.NotificationOutbox {
	t.Helper()
	msgID := uuid.NewString()
	raw, err := json.Marshal(map[string]any{
		"event_type":   eventTypeMessageNew,
		"user_id":      userID,
		"message_id":   msgID,
		"dialog_id":    uuid.NewString(),
		"sender_id":    uuid.NewString(),
		"preview":      "hello",
		"unread_count": 1,
		"dedup_key":    "message_new:" + msgID + ":" + userID,
	})
	if err != nil {
		t.Fatalf("marshal task payload: %v", err)
	}
	return store.NotificationOutbox{
		ID:        uuid.NewString(),
		EventType: eventTypeMessageNew,
		UserID:    userID,
		Payload:   raw,
		Attempt:   attempt,
		Status:    store.OutboxStatusPending,
	}
}

func newWorker(outbox *fakeOutbox, devices *fakeDevices, provider push.Provider) *notification.Worker {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	return notification.NewWorker(outbox, devices, provider, log, notification.Config{
		BatchSize:   10,
		MaxAttempts: 3,
		BackoffBase: 10 * time.Second,
	})
}

// --- tests ---

func TestWorker_Success_MarksSent(t *testing.T) {
	userID := uuid.NewString()
	task := makeTask(t, userID, 1)

	outbox := &fakeOutbox{tasks: []store.NotificationOutbox{task}}
	devices := &fakeDevices{devices: map[string][]store.Device{
		userID: {{ID: uuid.NewString(), UserID: userID, Platform: platformIOSTest, PushToken: pushTokenTest, Enabled: true}},
	}}
	provider := push.NewNoopProvider()

	w := newWorker(outbox, devices, provider)
	n, err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 processed, got %d", n)
	}
	if len(outbox.sentIDs) != 1 || outbox.sentIDs[0] != task.ID {
		t.Errorf("expected task %q in sentIDs, got %v", task.ID, outbox.sentIDs)
	}
	if outbox.failedID != "" {
		t.Errorf("expected no failed tasks, got failedID=%q", outbox.failedID)
	}
}

func TestWorker_ProviderError_BelowMaxAttempts_MarksFailedWithBackoff(t *testing.T) {
	userID := uuid.NewString()
	task := makeTask(t, userID, 1) // attempt=1, max=3 → должен быть повтор

	outbox := &fakeOutbox{tasks: []store.NotificationOutbox{task}}
	devices := &fakeDevices{devices: map[string][]store.Device{
		userID: {{ID: uuid.NewString(), UserID: userID, Platform: "android", PushToken: pushTokenTest, Enabled: true}},
	}}
	provider := push.NewNoopProvider()
	provider.SendFunc = func(_ context.Context, _ push.Message) error {
		return errors.New("provider unavailable")
	}

	w := newWorker(outbox, devices, provider)
	_, err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if outbox.failedID != task.ID {
		t.Errorf("expected task %q failed, got %q", task.ID, outbox.failedID)
	}
	if len(outbox.sentIDs) != 0 {
		t.Errorf("expected no sent tasks, got %v", outbox.sentIDs)
	}
	if !outbox.failedAt.After(time.Now()) {
		t.Errorf("expected future next_attempt_at, got %v", outbox.failedAt)
	}
	if outbox.failedAt.After(time.Now().Add(notification.BackoffCap + time.Minute)) {
		t.Errorf("backoff exceeded cap: %v", outbox.failedAt)
	}
}

func TestWorker_ProviderError_AtMaxAttempts_MarksFailedWithExhaustedDelay(t *testing.T) {
	userID := uuid.NewString()
	task := makeTask(t, userID, 3) // attempt=3 == MaxAttempts → исчерпан

	outbox := &fakeOutbox{tasks: []store.NotificationOutbox{task}}
	devices := &fakeDevices{devices: map[string][]store.Device{
		userID: {{ID: uuid.NewString(), UserID: userID, Platform: platformIOSTest, PushToken: pushTokenTest, Enabled: true}},
	}}
	provider := push.NewNoopProvider()
	provider.SendFunc = func(_ context.Context, _ push.Message) error {
		return errors.New("permanent failure")
	}

	w := newWorker(outbox, devices, provider)
	_, err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if outbox.failedID != task.ID {
		t.Errorf("expected task %q failed, got %q", task.ID, outbox.failedID)
	}
	// Задача с исчерпанными попытками уходит на exhaustedDelay — не менее суток.
	minExhausted := time.Now().Add(24 * time.Hour)
	if !outbox.failedAt.After(minExhausted) {
		t.Errorf("expected exhausted delay (>24h), got next_attempt_at=%v", outbox.failedAt)
	}
}

func TestWorker_NoDevices_MarksSentWithoutCallingProvider(t *testing.T) {
	userID := uuid.NewString()
	task := makeTask(t, userID, 1)

	outbox := &fakeOutbox{tasks: []store.NotificationOutbox{task}}
	devices := &fakeDevices{devices: map[string][]store.Device{}} // нет устройств

	var providerCalled bool
	provider := push.NewNoopProvider()
	provider.SendFunc = func(_ context.Context, _ push.Message) error {
		providerCalled = true
		return nil
	}

	w := newWorker(outbox, devices, provider)
	_, err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if providerCalled {
		t.Error("provider.Send must not be called when there are no devices")
	}
	if len(outbox.sentIDs) != 1 || outbox.sentIDs[0] != task.ID {
		t.Errorf("expected task marked sent, got sentIDs=%v", outbox.sentIDs)
	}
}

func TestCalcBackoff(t *testing.T) {
	base := 10 * time.Second
	cases := []struct {
		attempt  int
		expected time.Duration
	}{
		{1, 10 * time.Second},
		{2, 20 * time.Second},
		{3, 40 * time.Second},
		{4, 80 * time.Second},
		{9, notification.BackoffCap}, // 10s * 2^8 = 2560s > 30min → cap
	}
	for _, tc := range cases {
		got := notification.CalcBackoff(base, tc.attempt)
		if got != tc.expected {
			t.Errorf("CalcBackoff(base, %d) = %v, want %v", tc.attempt, got, tc.expected)
		}
	}
}
