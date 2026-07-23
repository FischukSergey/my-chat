// Package chat_test contains unit tests for the chat service.
package chat_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"my-chat/internal/hub"
	chat "my-chat/internal/services/chat"
	"my-chat/internal/store"
)

const noTTL = 0 // используется в тестах без TTL

const (
	eventMessageRead = "message_read"
	eventMessageNew  = "message_new"
)

// --- mock types ---

type mockDialogRepo struct {
	getByIDFn func(ctx context.Context, dialogID string) (store.Dialog, error)
}

func (m *mockDialogRepo) GetByID(ctx context.Context, dialogID string) (store.Dialog, error) {
	return m.getByIDFn(ctx, dialogID)
}

type mockMessageRepo struct {
	createFn       func(ctx context.Context, msg store.Message) (store.Message, error)
	getByIDFn      func(ctx context.Context, msgID string) (store.Message, error)
	listByDialogFn func(ctx context.Context, dialogID string, limit int, before *time.Time) ([]store.Message, error)
}

func (m *mockMessageRepo) Create(ctx context.Context, msg store.Message) (store.Message, error) {
	return m.createFn(ctx, msg)
}

func (m *mockMessageRepo) GetByID(ctx context.Context, msgID string) (store.Message, error) {
	return m.getByIDFn(ctx, msgID)
}

func (m *mockMessageRepo) ListByDialog(
	ctx context.Context,
	dialogID string,
	limit int,
	before *time.Time,
) ([]store.Message, error) {
	return m.listByDialogFn(ctx, dialogID, limit, before)
}

type mockReceiptRepo struct {
	ensureFn      func(ctx context.Context, messageID, userID string) error
	markReadFn    func(ctx context.Context, messageID, userID string, readAt time.Time) error
	countUnreadFn func(ctx context.Context, userID string) (int, error)
}

func (m *mockReceiptRepo) Ensure(ctx context.Context, messageID, userID string) error {
	return m.ensureFn(ctx, messageID, userID)
}

func (m *mockReceiptRepo) MarkRead(ctx context.Context, messageID, userID string, readAt time.Time) error {
	return m.markReadFn(ctx, messageID, userID, readAt)
}

func (m *mockReceiptRepo) CountUnread(ctx context.Context, userID string) (int, error) {
	if m.countUnreadFn == nil {
		return 0, nil
	}

	return m.countUnreadFn(ctx, userID)
}

type mockNotifier struct {
	sendFn func(ctx context.Context, userID string, event hub.Event) bool
}

func (m *mockNotifier) Send(ctx context.Context, userID string, event hub.Event) bool {
	return m.sendFn(ctx, userID, event)
}

type mockOutbox struct {
	enqueueFn func(ctx context.Context, task store.NotificationOutbox) error
}

func (m *mockOutbox) Enqueue(ctx context.Context, task store.NotificationOutbox) error {
	return m.enqueueFn(ctx, task)
}

// --- helpers ---

func noopNotifier() *mockNotifier {
	return &mockNotifier{
		sendFn: func(_ context.Context, _ string, _ hub.Event) bool { return false },
	}
}

func noopOutbox() *mockOutbox {
	return &mockOutbox{
		enqueueFn: func(_ context.Context, _ store.NotificationOutbox) error { return nil },
	}
}

// --- tests ---

func TestSendMessage_EmptyBody(t *testing.T) {
	t.Parallel()

	svc := chat.NewService(
		&mockDialogRepo{},
		&mockMessageRepo{},
		&mockReceiptRepo{},
		noopNotifier(),
		noopOutbox(),
		noTTL,
	)

	_, err := svc.SendMessage(context.Background(), store.Message{
		ID:       "msg-1",
		DialogID: "dialog-1",
		SenderID: "user-a",
		Body:     "   ",
	})
	if !errors.Is(err, chat.ErrInvalidMessageBody) {
		t.Errorf("expected ErrInvalidMessageBody, got %v", err)
	}
}

func TestSendMessage_ForbiddenDialog(t *testing.T) {
	t.Parallel()

	svc := chat.NewService(
		&mockDialogRepo{
			getByIDFn: func(_ context.Context, _ string) (store.Dialog, error) {
				return store.Dialog{ID: "d1", UserAID: "user-a", UserBID: "user-b"}, nil
			},
		},
		&mockMessageRepo{},
		&mockReceiptRepo{},
		noopNotifier(),
		noopOutbox(),
		noTTL,
	)

	_, err := svc.SendMessage(context.Background(), store.Message{
		ID:       "msg-1",
		DialogID: "d1",
		SenderID: "intruder",
		Body:     "hello",
	})
	if !errors.Is(err, chat.ErrForbiddenDialogAccess) {
		t.Errorf("expected ErrForbiddenDialogAccess, got %v", err)
	}
}

func TestSendMessage_ReceiverOffline(t *testing.T) {
	t.Parallel()

	notifyCount := 0

	svc := chat.NewService(
		&mockDialogRepo{
			getByIDFn: func(_ context.Context, _ string) (store.Dialog, error) {
				return store.Dialog{ID: "d1", UserAID: "user-a", UserBID: "user-b"}, nil
			},
		},
		&mockMessageRepo{
			createFn: func(_ context.Context, msg store.Message) (store.Message, error) {
				msg.CreatedAt = time.Now()
				return msg, nil
			},
		},
		&mockReceiptRepo{
			ensureFn: func(_ context.Context, _, _ string) error { return nil },
		},
		&mockNotifier{
			sendFn: func(_ context.Context, _ string, _ hub.Event) bool {
				notifyCount++
				return false
			},
		},
		noopOutbox(),
		noTTL,
	)

	msg, err := svc.SendMessage(context.Background(), store.Message{
		ID:       "msg-1",
		DialogID: "d1",
		SenderID: "user-a",
		Body:     "hello",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Body != "hello" {
		t.Errorf("unexpected body: %q", msg.Body)
	}
	if notifyCount != 1 {
		t.Errorf("expected 1 notification (message_new only), got %d", notifyCount)
	}
}

func TestSendMessage_ReceiverOnline(t *testing.T) {
	t.Parallel()

	var eventNames []string

	svc := chat.NewService(
		&mockDialogRepo{
			getByIDFn: func(_ context.Context, _ string) (store.Dialog, error) {
				return store.Dialog{ID: "d1", UserAID: "user-a", UserBID: "user-b"}, nil
			},
		},
		&mockMessageRepo{
			createFn: func(_ context.Context, msg store.Message) (store.Message, error) {
				msg.CreatedAt = time.Now()
				return msg, nil
			},
		},
		&mockReceiptRepo{
			ensureFn: func(_ context.Context, _, _ string) error { return nil },
		},
		&mockNotifier{
			sendFn: func(_ context.Context, _ string, event hub.Event) bool {
				eventNames = append(eventNames, event.Event)
				return true
			},
		},
		noopOutbox(),
		noTTL,
	)

	_, err := svc.SendMessage(context.Background(), store.Message{
		ID:       "msg-1",
		DialogID: "d1",
		SenderID: "user-a",
		Body:     "hello",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(eventNames) != 2 {
		t.Fatalf("expected 2 events, got %d: %v", len(eventNames), eventNames)
	}
	if eventNames[0] != eventMessageNew {
		t.Errorf("expected event[0]=message_new, got %q", eventNames[0])
	}
	if eventNames[1] != "message_delivered" {
		t.Errorf("expected event[1]=message_delivered, got %q", eventNames[1])
	}
}

func TestMarkRead_NotifiesSender(t *testing.T) {
	t.Parallel()

	type sentEvent struct {
		userID string
		name   string
	}
	var events []sentEvent

	svc := chat.NewService(
		&mockDialogRepo{},
		&mockMessageRepo{
			getByIDFn: func(_ context.Context, _ string) (store.Message, error) {
				return store.Message{
					ID:       "msg-1",
					DialogID: "d1",
					SenderID: "user-a",
					Body:     "hello",
				}, nil
			},
		},
		&mockReceiptRepo{
			markReadFn: func(_ context.Context, _, _ string, _ time.Time) error { return nil },
		},
		&mockNotifier{
			sendFn: func(_ context.Context, userID string, event hub.Event) bool {
				events = append(events, sentEvent{userID: userID, name: event.Event})
				return true
			},
		},
		noopOutbox(),
		noTTL,
	)

	if err := svc.MarkRead(context.Background(), "msg-1", "user-b", time.Now()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Должно быть хотя бы одно событие message_read, адресованное отправителю.
	var found bool
	for _, e := range events {
		if e.userID == "user-a" && e.name == eventMessageRead {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected message_read sent to sender user-a, got events: %v", events)
	}
}

func TestListMessages_ForbiddenDialog(t *testing.T) {
	t.Parallel()

	svc := chat.NewService(
		&mockDialogRepo{
			getByIDFn: func(_ context.Context, _ string) (store.Dialog, error) {
				return store.Dialog{ID: "d1", UserAID: "user-a", UserBID: "user-b"}, nil
			},
		},
		&mockMessageRepo{},
		&mockReceiptRepo{},
		noopNotifier(),
		noopOutbox(),
		noTTL,
	)

	_, err := svc.ListMessages(context.Background(), "intruder", "d1", 10, nil)
	if !errors.Is(err, chat.ErrForbiddenDialogAccess) {
		t.Errorf("expected ErrForbiddenDialogAccess, got %v", err)
	}
}

func TestListMessages_Success(t *testing.T) {
	t.Parallel()

	want := []store.Message{
		{ID: "msg-1", DialogID: "d1", SenderID: "user-a", Body: "hello"},
	}

	svc := chat.NewService(
		&mockDialogRepo{
			getByIDFn: func(_ context.Context, _ string) (store.Dialog, error) {
				return store.Dialog{ID: "d1", UserAID: "user-a", UserBID: "user-b"}, nil
			},
		},
		&mockMessageRepo{
			listByDialogFn: func(_ context.Context, _ string, _ int, _ *time.Time) ([]store.Message, error) {
				return want, nil
			},
		},
		&mockReceiptRepo{},
		noopNotifier(),
		noopOutbox(),
		noTTL,
	)

	got, err := svc.ListMessages(context.Background(), "user-a", "d1", 10, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != len(want) {
		t.Errorf("expected %d messages, got %d", len(want), len(got))
	}
}

func TestUnreadCount(t *testing.T) {
	t.Parallel()

	svc := chat.NewService(
		&mockDialogRepo{},
		&mockMessageRepo{},
		&mockReceiptRepo{
			countUnreadFn: func(_ context.Context, _ string) (int, error) {
				return 5, nil
			},
		},
		noopNotifier(),
		noopOutbox(),
		noTTL,
	)

	count, err := svc.UnreadCount(context.Background(), "user-a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 5 {
		t.Errorf("expected 5, got %d", count)
	}
}

func TestSendMessage_ReceiverOffline_EnqueuesOutbox(t *testing.T) {
	t.Parallel()

	var enqueuedTask store.NotificationOutbox

	svc := chat.NewService(
		&mockDialogRepo{
			getByIDFn: func(_ context.Context, _ string) (store.Dialog, error) {
				return store.Dialog{ID: "d1", UserAID: "user-a", UserBID: "user-b"}, nil
			},
		},
		&mockMessageRepo{
			createFn: func(_ context.Context, msg store.Message) (store.Message, error) {
				msg.CreatedAt = time.Now()
				return msg, nil
			},
		},
		&mockReceiptRepo{
			ensureFn: func(_ context.Context, _, _ string) error { return nil },
			countUnreadFn: func(_ context.Context, _ string) (int, error) {
				return 3, nil
			},
		},
		&mockNotifier{
			sendFn: func(_ context.Context, _ string, _ hub.Event) bool { return false },
		},
		&mockOutbox{
			enqueueFn: func(_ context.Context, task store.NotificationOutbox) error {
				enqueuedTask = task
				return nil
			},
		},
		noTTL,
	)

	_, err := svc.SendMessage(context.Background(), store.Message{
		ID:       "msg-1",
		DialogID: "d1",
		SenderID: "user-a",
		Body:     "hello outbox",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if enqueuedTask.ID == "" {
		t.Fatal("expected outbox task to be enqueued, but it was not")
	}
	if enqueuedTask.EventType != eventMessageNew {
		t.Errorf("expected event_type=message_new, got %q", enqueuedTask.EventType)
	}
	if enqueuedTask.UserID != "user-b" {
		t.Errorf("expected task user_id=user-b (receiver), got %q", enqueuedTask.UserID)
	}
	expectedDedupKey := "message_new:msg-1:user-b"
	if enqueuedTask.DedupKey != expectedDedupKey {
		t.Errorf("expected dedup_key=%q, got %q", expectedDedupKey, enqueuedTask.DedupKey)
	}
	if len(enqueuedTask.Payload) == 0 {
		t.Error("expected non-empty payload")
	}
}

func TestSendMessage_ReceiverOnline_NoOutbox(t *testing.T) {
	t.Parallel()

	outboxCalled := false

	svc := chat.NewService(
		&mockDialogRepo{
			getByIDFn: func(_ context.Context, _ string) (store.Dialog, error) {
				return store.Dialog{ID: "d1", UserAID: "user-a", UserBID: "user-b"}, nil
			},
		},
		&mockMessageRepo{
			createFn: func(_ context.Context, msg store.Message) (store.Message, error) {
				msg.CreatedAt = time.Now()
				return msg, nil
			},
		},
		&mockReceiptRepo{
			ensureFn: func(_ context.Context, _, _ string) error { return nil },
		},
		&mockNotifier{
			sendFn: func(_ context.Context, _ string, _ hub.Event) bool { return true },
		},
		&mockOutbox{
			enqueueFn: func(_ context.Context, _ store.NotificationOutbox) error {
				outboxCalled = true
				return nil
			},
		},
		noTTL,
	)

	_, err := svc.SendMessage(context.Background(), store.Message{
		ID:       "msg-1",
		DialogID: "d1",
		SenderID: "user-a",
		Body:     "hello online",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outboxCalled {
		t.Error("outbox must NOT be called when receiver is online")
	}
}

func TestBuildPreview_Truncates(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("а", 200)
	got := chat.BuildPreview(long)
	if len([]rune(got)) != 120 {
		t.Errorf("expected 120 runes, got %d", len([]rune(got)))
	}
}

func TestSendMessage_WithTTL_SetsExpiresAt(t *testing.T) {
	t.Parallel()

	var capturedMsg store.Message

	svc := chat.NewService(
		&mockDialogRepo{
			getByIDFn: func(_ context.Context, _ string) (store.Dialog, error) {
				return store.Dialog{ID: "d1", UserAID: "user-a", UserBID: "user-b"}, nil
			},
		},
		&mockMessageRepo{
			createFn: func(_ context.Context, msg store.Message) (store.Message, error) {
				capturedMsg = msg
				msg.CreatedAt = time.Now()
				return msg, nil
			},
		},
		&mockReceiptRepo{
			ensureFn: func(_ context.Context, _, _ string) error { return nil },
		},
		noopNotifier(),
		noopOutbox(),
		5*time.Minute,
	)

	before := time.Now()
	_, err := svc.SendMessage(context.Background(), store.Message{
		ID:       "msg-ttl",
		DialogID: "d1",
		SenderID: "user-a",
		Body:     "hello ttl",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedMsg.ExpiresAt == nil {
		t.Fatal("expected ExpiresAt to be set when TTL > 0")
	}
	if !capturedMsg.ExpiresAt.After(before) {
		t.Errorf("ExpiresAt %v must be after send time %v", *capturedMsg.ExpiresAt, before)
	}
	minExp := before.Add(5 * time.Minute)
	if capturedMsg.ExpiresAt.Before(minExp) {
		t.Errorf("ExpiresAt %v must be >= now+5min (%v)", *capturedMsg.ExpiresAt, minExp)
	}
}

func TestSendMessage_WithoutTTL_ExpiresAtIsNil(t *testing.T) {
	t.Parallel()

	var capturedMsg store.Message

	svc := chat.NewService(
		&mockDialogRepo{
			getByIDFn: func(_ context.Context, _ string) (store.Dialog, error) {
				return store.Dialog{ID: "d1", UserAID: "user-a", UserBID: "user-b"}, nil
			},
		},
		&mockMessageRepo{
			createFn: func(_ context.Context, msg store.Message) (store.Message, error) {
				capturedMsg = msg
				msg.CreatedAt = time.Now()
				return msg, nil
			},
		},
		&mockReceiptRepo{
			ensureFn: func(_ context.Context, _, _ string) error { return nil },
		},
		noopNotifier(),
		noopOutbox(),
		noTTL,
	)

	_, err := svc.SendMessage(context.Background(), store.Message{
		ID:       "msg-no-ttl",
		DialogID: "d1",
		SenderID: "user-a",
		Body:     "no ttl",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedMsg.ExpiresAt != nil {
		t.Errorf("expected ExpiresAt to be nil when TTL=0, got %v", *capturedMsg.ExpiresAt)
	}
}

func TestSendMessage_WithTTL_IncludesExpiresAtInWSEvent(t *testing.T) {
	t.Parallel()

	var receivedEvent hub.Event

	svc := chat.NewService(
		&mockDialogRepo{
			getByIDFn: func(_ context.Context, _ string) (store.Dialog, error) {
				return store.Dialog{ID: "d1", UserAID: "user-a", UserBID: "user-b"}, nil
			},
		},
		&mockMessageRepo{
			createFn: func(_ context.Context, msg store.Message) (store.Message, error) {
				msg.CreatedAt = time.Now()
				return msg, nil
			},
		},
		&mockReceiptRepo{
			ensureFn: func(_ context.Context, _, _ string) error { return nil },
		},
		&mockNotifier{
			sendFn: func(_ context.Context, _ string, event hub.Event) bool {
				if event.Event == "message_new" {
					receivedEvent = event
				}
				return false
			},
		},
		noopOutbox(),
		5*time.Minute,
	)

	_, err := svc.SendMessage(context.Background(), store.Message{
		ID:       "msg-ws",
		DialogID: "d1",
		SenderID: "user-a",
		Body:     "ws ttl test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, ok := receivedEvent.Data.(map[string]any)
	if !ok {
		t.Fatalf("event.Data is not map[string]any: %T", receivedEvent.Data)
	}
	expiresAt, exists := data["expires_at"]
	if !exists {
		t.Fatal("expected expires_at in message_new WS event")
	}
	if expiresAt == nil {
		t.Error("expires_at must not be nil when TTL > 0")
	}
}

func TestBuildPreview_NormalizesNewlines(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input string
		want  string
	}{
		{"hello\nworld", "hello world"},
		{"hello\r\nworld", "hello world"},
		{"hello\rworld", "hello world"},
		{"no newlines", "no newlines"},
	}

	for _, c := range cases {
		got := chat.BuildPreview(c.input)
		if got != c.want {
			t.Errorf("BuildPreview(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

func TestMarkRead_SendsBadgeUpdatedToReader(t *testing.T) {
	t.Parallel()

	type sentEvent struct {
		userID string
		event  hub.Event
	}
	var sent []sentEvent

	svc := chat.NewService(
		&mockDialogRepo{},
		&mockMessageRepo{
			getByIDFn: func(_ context.Context, _ string) (store.Message, error) {
				return store.Message{
					ID:       "msg-1",
					DialogID: "d1",
					SenderID: "user-a",
					Body:     "hello",
				}, nil
			},
		},
		&mockReceiptRepo{
			markReadFn: func(_ context.Context, _, _ string, _ time.Time) error { return nil },
			countUnreadFn: func(_ context.Context, _ string) (int, error) {
				return 2, nil
			},
		},
		&mockNotifier{
			sendFn: func(_ context.Context, userID string, event hub.Event) bool {
				sent = append(sent, sentEvent{userID: userID, event: event})
				return true
			},
		},
		noopOutbox(),
		noTTL,
	)

	if err := svc.MarkRead(context.Background(), "msg-1", "user-b", time.Now()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(sent) != 2 {
		t.Fatalf("expected 2 events (message_read + badge_updated), got %d", len(sent))
	}

	// Первое событие — message_read отправителю.
	if sent[0].event.Event != eventMessageRead {
		t.Errorf("expected event[0]=message_read, got %q", sent[0].event.Event)
	}
	if sent[0].userID != "user-a" {
		t.Errorf("expected message_read sent to sender user-a, got %q", sent[0].userID)
	}

	// Второе событие — badge_updated читателю.
	if sent[1].event.Event != "badge_updated" {
		t.Errorf("expected event[1]=badge_updated, got %q", sent[1].event.Event)
	}
	if sent[1].userID != "user-b" {
		t.Errorf("expected badge_updated sent to reader user-b, got %q", sent[1].userID)
	}

	data, ok := sent[1].event.Data.(map[string]any)
	if !ok {
		t.Fatalf("badge_updated data is not map[string]any: %T", sent[1].event.Data)
	}
	if data["unread_count"] != 2 {
		t.Errorf("expected unread_count=2, got %v", data["unread_count"])
	}
	if data["badge"] != 2 {
		t.Errorf("expected badge=2, got %v", data["badge"])
	}
	if data["reason"] != eventMessageRead {
		t.Errorf("expected reason=message_read, got %v", data["reason"])
	}
}
