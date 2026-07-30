package expirer_test

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

	"my-chat/internal/services/expirer"
	"my-chat/internal/store"
)

// --- fakes ---

type fakeMessageRepo struct {
	mu      sync.Mutex
	expired []store.ExpiredMessage
	err     error
}

func (f *fakeMessageRepo) ExpireMessages(_ context.Context, _ time.Time, _ int) ([]store.ExpiredMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.expired, f.err
}

type fakePublisher struct {
	mu      sync.Mutex
	batches [][]store.WSEventOutbox
	err     error
}

func (f *fakePublisher) EnqueueBatch(_ context.Context, events []store.WSEventOutbox) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	cp := make([]store.WSEventOutbox, len(events))
	copy(cp, events)
	f.batches = append(f.batches, cp)
	return nil
}

func (f *fakePublisher) allEvents() []store.WSEventOutbox {
	f.mu.Lock()
	defer f.mu.Unlock()
	var all []store.WSEventOutbox
	for _, b := range f.batches {
		all = append(all, b...)
	}
	return all
}

func newExpirer(repo *fakeMessageRepo, pub *fakePublisher) *expirer.Expirer {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	return expirer.New(repo, pub, log, 100)
}

// --- tests ---

func TestExpirer_Tick_EmptyRepo_ReturnsZero(t *testing.T) {
	repo := &fakeMessageRepo{expired: nil}
	pub := &fakePublisher{}
	e := newExpirer(repo, pub)

	n, err := e.Tick(context.Background())
	if err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 expired, got %d", n)
	}
	if len(pub.allEvents()) != 0 {
		t.Errorf("expected no events published, got %d", len(pub.allEvents()))
	}
}

func TestExpirer_Tick_PublishesTwoEventsPerMessage(t *testing.T) {
	userA := uuid.NewString()
	userB := uuid.NewString()
	msgID := uuid.NewString()
	dialogID := uuid.NewString()

	repo := &fakeMessageRepo{
		expired: []store.ExpiredMessage{
			{ID: msgID, DialogID: dialogID, SenderID: userA, UserAID: userA, UserBID: userB},
		},
	}
	pub := &fakePublisher{}
	e := newExpirer(repo, pub)

	n, err := e.Tick(context.Background())
	if err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 expired, got %d", n)
	}

	events := pub.allEvents()
	if len(events) != 2 {
		t.Fatalf("expected 2 events (one per participant), got %d", len(events))
	}

	userIDs := map[string]bool{}
	for _, ev := range events {
		if ev.EventType != "message_deleted" {
			t.Errorf("expected event_type 'message_deleted', got %q", ev.EventType)
		}
		if ev.ID == "" {
			t.Error("expected non-empty event ID")
		}
		userIDs[ev.UserID] = true

		var payload struct {
			Type      string `json:"type"`
			MessageID string `json:"message_id"`
			DialogID  string `json:"dialog_id"`
		}
		if err = json.Unmarshal(ev.Payload, &payload); err != nil {
			t.Fatalf("unmarshal payload: %v", err)
		}
		if payload.Type != "message_deleted" {
			t.Errorf("payload.type: want 'message_deleted', got %q", payload.Type)
		}
		if payload.MessageID != msgID {
			t.Errorf("payload.message_id: want %q, got %q", msgID, payload.MessageID)
		}
		if payload.DialogID != dialogID {
			t.Errorf("payload.dialog_id: want %q, got %q", dialogID, payload.DialogID)
		}
	}

	if !userIDs[userA] {
		t.Errorf("expected event for userA %q, not found in events", userA)
	}
	if !userIDs[userB] {
		t.Errorf("expected event for userB %q, not found in events", userB)
	}
}

func TestExpirer_Tick_MultipleMessages_PublishesAllEvents(t *testing.T) {
	makeMsg := func() store.ExpiredMessage {
		return store.ExpiredMessage{
			ID:       uuid.NewString(),
			DialogID: uuid.NewString(),
			SenderID: uuid.NewString(),
			UserAID:  uuid.NewString(),
			UserBID:  uuid.NewString(),
		}
	}

	msgs := []store.ExpiredMessage{makeMsg(), makeMsg(), makeMsg()}
	repo := &fakeMessageRepo{expired: msgs}
	pub := &fakePublisher{}
	e := newExpirer(repo, pub)

	n, err := e.Tick(context.Background())
	if err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if n != len(msgs) {
		t.Errorf("expected %d expired, got %d", len(msgs), n)
	}
	if got := len(pub.allEvents()); got != len(msgs)*2 {
		t.Errorf("expected %d events, got %d", len(msgs)*2, got)
	}
}

func TestExpirer_Tick_RepoError_ReturnsError(t *testing.T) {
	repoErr := errors.New("db connection lost")
	repo := &fakeMessageRepo{err: repoErr}
	pub := &fakePublisher{}
	e := newExpirer(repo, pub)

	_, err := e.Tick(context.Background())
	if err == nil {
		t.Fatal("expected error from Tick, got nil")
	}
	if !errors.Is(err, repoErr) {
		t.Errorf("expected wrapped repoErr, got %v", err)
	}
	if len(pub.allEvents()) != 0 {
		t.Error("publisher must not be called when repo fails")
	}
}

func TestExpirer_Tick_PublisherError_ReturnsError(t *testing.T) {
	pubErr := errors.New("outbox write failed")
	repo := &fakeMessageRepo{
		expired: []store.ExpiredMessage{
			{
				ID:       uuid.NewString(),
				DialogID: uuid.NewString(),
				SenderID: uuid.NewString(),
				UserAID:  uuid.NewString(),
				UserBID:  uuid.NewString(),
			},
		},
	}
	pub := &fakePublisher{err: pubErr}
	e := newExpirer(repo, pub)

	_, err := e.Tick(context.Background())
	if err == nil {
		t.Fatal("expected error from Tick, got nil")
	}
	if !errors.Is(err, pubErr) {
		t.Errorf("expected wrapped pubErr, got %v", err)
	}
}
