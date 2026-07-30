package wsdelivery_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"sync"
	"testing"

	"github.com/google/uuid"

	"my-chat/internal/hub"
	"my-chat/internal/services/wsdelivery"
	"my-chat/internal/store"
)

// --- fakes ---

type fakeOutbox struct {
	mu           sync.Mutex
	pending      []store.WSEventOutbox
	processedIDs []string
	claimErr     error
	markErr      error
}

func (f *fakeOutbox) ClaimBatch(_ context.Context, batchSize int) ([]store.WSEventOutbox, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.claimErr != nil {
		return nil, f.claimErr
	}
	n := batchSize
	if n > len(f.pending) {
		n = len(f.pending)
	}
	batch := f.pending[:n]
	f.pending = f.pending[n:]
	return batch, nil
}

func (f *fakeOutbox) MarkProcessedBatch(_ context.Context, ids []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.markErr != nil {
		return f.markErr
	}
	f.processedIDs = append(f.processedIDs, ids...)
	return nil
}

type sentEvent struct {
	userID string
	event  hub.Event
}

type fakeHub struct {
	mu     sync.Mutex
	sent   []sentEvent
	online map[string]bool
}

func (f *fakeHub) Send(_ context.Context, userID string, event hub.Event) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, sentEvent{userID: userID, event: event})
	return f.online[userID]
}

func newDelivery(outbox *fakeOutbox, h *fakeHub) *wsdelivery.Delivery {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	return wsdelivery.New(outbox, h, log, 20)
}

func makeOutboxEvent(t *testing.T, userID string) store.WSEventOutbox {
	t.Helper()
	payload, err := json.Marshal(map[string]string{
		"type":       hub.EventMessageDeleted,
		"message_id": uuid.NewString(),
		"dialog_id":  uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("marshal test payload: %v", err)
	}
	return store.WSEventOutbox{
		ID:        uuid.NewString(),
		EventType: hub.EventMessageDeleted,
		UserID:    userID,
		Payload:   payload,
	}
}

// --- tests ---

func TestDelivery_RunOnce_EmptyOutbox_ReturnsZero(t *testing.T) {
	outbox := &fakeOutbox{}
	h := &fakeHub{online: map[string]bool{}}
	d := newDelivery(outbox, h)

	n, err := d.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0, got %d", n)
	}
	if len(h.sent) != 0 {
		t.Errorf("expected no Hub.Send calls, got %d", len(h.sent))
	}
}

func TestDelivery_RunOnce_SendsEventToCorrectUser(t *testing.T) {
	userID := uuid.NewString()
	ev := makeOutboxEvent(t, userID)

	outbox := &fakeOutbox{pending: []store.WSEventOutbox{ev}}
	h := &fakeHub{online: map[string]bool{userID: true}}
	d := newDelivery(outbox, h)

	n, err := d.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1, got %d", n)
	}
	if len(h.sent) != 1 {
		t.Fatalf("expected 1 Hub.Send call, got %d", len(h.sent))
	}
	if h.sent[0].userID != userID {
		t.Errorf("userID mismatch: want %q, got %q", userID, h.sent[0].userID)
	}
	if h.sent[0].event.Event != hub.EventMessageDeleted {
		t.Errorf("event name mismatch: want %q, got %q", hub.EventMessageDeleted, h.sent[0].event.Event)
	}
}

func TestDelivery_RunOnce_MarksEventsProcessed(t *testing.T) {
	userA := uuid.NewString()
	userB := uuid.NewString()
	evA := makeOutboxEvent(t, userA)
	evB := makeOutboxEvent(t, userB)

	outbox := &fakeOutbox{pending: []store.WSEventOutbox{evA, evB}}
	h := &fakeHub{online: map[string]bool{}}
	d := newDelivery(outbox, h)

	if _, err := d.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	if len(outbox.processedIDs) != 2 {
		t.Errorf("expected 2 processed IDs, got %d", len(outbox.processedIDs))
	}

	processed := map[string]bool{}
	for _, id := range outbox.processedIDs {
		processed[id] = true
	}
	if !processed[evA.ID] {
		t.Errorf("event %q was not marked processed", evA.ID)
	}
	if !processed[evB.ID] {
		t.Errorf("event %q was not marked processed", evB.ID)
	}
}

func TestDelivery_RunOnce_OfflineUser_EventStillMarkedProcessed(t *testing.T) {
	userID := uuid.NewString()
	ev := makeOutboxEvent(t, userID)

	outbox := &fakeOutbox{pending: []store.WSEventOutbox{ev}}
	h := &fakeHub{online: map[string]bool{}} // пользователь оффлайн
	d := newDelivery(outbox, h)

	n, err := d.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1, got %d", n)
	}
	if len(outbox.processedIDs) != 1 || outbox.processedIDs[0] != ev.ID {
		t.Errorf("expected event %q to be marked processed even for offline user", ev.ID)
	}
}

func TestDelivery_RunOnce_ClaimError_ReturnsError(t *testing.T) {
	claimErr := errors.New("db unavailable")
	outbox := &fakeOutbox{claimErr: claimErr}
	h := &fakeHub{online: map[string]bool{}}
	d := newDelivery(outbox, h)

	_, err := d.RunOnce(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, claimErr) {
		t.Errorf("expected wrapped claimErr, got %v", err)
	}
}
