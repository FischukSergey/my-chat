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

	userA = "user-a"
	userB = "user-b"

	msgID1    = "msg-1"
	msgBody   = "hello"
	msgBodyNL = "hello world"

	statusActive   = "active"
	peerUsername   = "bob"
	sharedDialogID = "d-shared"
	blockedUser    = "blocked"
)

// --- mock types ---

type mockDialogRepo struct {
	getByIDFn      func(ctx context.Context, dialogID string) (store.Dialog, error)
	getOrCreateFn  func(ctx context.Context, dialogID, user1ID, user2ID string) (store.Dialog, error)
	listByUserIDFn func(ctx context.Context, userID string) ([]store.DialogListItem, error)
}

func (m *mockDialogRepo) GetByID(ctx context.Context, dialogID string) (store.Dialog, error) {
	return m.getByIDFn(ctx, dialogID)
}

func (m *mockDialogRepo) GetOrCreate(ctx context.Context, dialogID, user1ID, user2ID string) (store.Dialog, error) {
	if m.getOrCreateFn == nil {
		return store.Dialog{}, errors.New("GetOrCreate not stubbed")
	}
	return m.getOrCreateFn(ctx, dialogID, user1ID, user2ID)
}

func (m *mockDialogRepo) ListByUserID(ctx context.Context, userID string) ([]store.DialogListItem, error) {
	if m.listByUserIDFn == nil {
		return nil, errors.New("ListByUserID not stubbed")
	}
	return m.listByUserIDFn(ctx, userID)
}

type mockUserRepo struct {
	findByUsernameFn func(ctx context.Context, username string) (store.User, error)
}

func (m *mockUserRepo) FindByUsername(ctx context.Context, username string) (store.User, error) {
	if m.findByUsernameFn == nil {
		return store.User{}, errors.New("FindByUsername not stubbed")
	}
	return m.findByUsernameFn(ctx, username)
}

type mockMessageRepo struct {
	createFn       func(ctx context.Context, msg store.Message) (store.Message, error)
	getByIDFn      func(ctx context.Context, msgID string) (store.Message, error)
	listByDialogFn func(ctx context.Context, dialogID string, limit int, before *time.Time) ([]store.Message, error)
	setExpiresAtFn func(ctx context.Context, messageID string, expiresAt time.Time) error
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

func (m *mockMessageRepo) SetExpiresAt(ctx context.Context, messageID string, expiresAt time.Time) error {
	if m.setExpiresAtFn != nil {
		return m.setExpiresAtFn(ctx, messageID, expiresAt)
	}
	return nil
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
		ID:       msgID1,
		DialogID: "dialog-1",
		SenderID: userA,
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
				return store.Dialog{ID: "d1", UserAID: userA, UserBID: userB}, nil
			},
		},
		&mockMessageRepo{},
		&mockReceiptRepo{},
		noopNotifier(),
		noopOutbox(),
		noTTL,
	)

	_, err := svc.SendMessage(context.Background(), store.Message{
		ID:       msgID1,
		DialogID: "d1",
		SenderID: "intruder",
		Body:     msgBody,
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
				return store.Dialog{ID: "d1", UserAID: userA, UserBID: userB}, nil
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
		ID:       msgID1,
		DialogID: "d1",
		SenderID: userA,
		Body:     msgBody,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Body != msgBody {
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
				return store.Dialog{ID: "d1", UserAID: userA, UserBID: userB}, nil
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
		ID:       msgID1,
		DialogID: "d1",
		SenderID: userA,
		Body:     msgBody,
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
					ID:       msgID1,
					DialogID: "d1",
					SenderID: userA,
					Body:     msgBody,
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

	if err := svc.MarkRead(context.Background(), msgID1, userB, time.Now()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Должно быть хотя бы одно событие message_read, адресованное отправителю.
	var found bool
	for _, e := range events {
		if e.userID == userA && e.name == eventMessageRead {
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
				return store.Dialog{ID: "d1", UserAID: userA, UserBID: userB}, nil
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
		{ID: msgID1, DialogID: "d1", SenderID: userA, Body: msgBody},
	}

	svc := chat.NewService(
		&mockDialogRepo{
			getByIDFn: func(_ context.Context, _ string) (store.Dialog, error) {
				return store.Dialog{ID: "d1", UserAID: userA, UserBID: userB}, nil
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

	got, err := svc.ListMessages(context.Background(), userA, "d1", 10, nil)
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

	count, err := svc.UnreadCount(context.Background(), userA)
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
				return store.Dialog{ID: "d1", UserAID: userA, UserBID: userB}, nil
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
		ID:       msgID1,
		DialogID: "d1",
		SenderID: userA,
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
	if enqueuedTask.UserID != userB {
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
				return store.Dialog{ID: "d1", UserAID: userA, UserBID: userB}, nil
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
		ID:       msgID1,
		DialogID: "d1",
		SenderID: userA,
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

func TestSendMessage_ExpiresAtAlwaysNilAtSend(t *testing.T) {
	t.Parallel()

	var capturedMsg store.Message

	// Даже с TTL > 0 expires_at при отправке должен быть nil:
	// таймер запускается только при прочтении получателем.
	svc := chat.NewService(
		&mockDialogRepo{
			getByIDFn: func(_ context.Context, _ string) (store.Dialog, error) {
				return store.Dialog{ID: "d1", UserAID: userA, UserBID: userB}, nil
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

	_, err := svc.SendMessage(context.Background(), store.Message{
		ID:       "msg-ttl",
		DialogID: "d1",
		SenderID: userA,
		Body:     "hello ttl",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedMsg.ExpiresAt != nil {
		t.Errorf("expected ExpiresAt nil at send time, got %v", *capturedMsg.ExpiresAt)
	}
}

func TestSendMessage_MessageNew_ExpiresAtIsNil(t *testing.T) {
	t.Parallel()

	var receivedEvent hub.Event

	svc := chat.NewService(
		&mockDialogRepo{
			getByIDFn: func(_ context.Context, _ string) (store.Dialog, error) {
				return store.Dialog{ID: "d1", UserAID: userA, UserBID: userB}, nil
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
		SenderID: userA,
		Body:     "ws ttl test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, ok := receivedEvent.Data.(map[string]any)
	if !ok {
		t.Fatalf("event.Data is not map[string]any: %T", receivedEvent.Data)
	}
	// expires_at должен быть nil в message_new — таймер ещё не запущен
	expiresAt := data["expires_at"]
	if expiresAt != nil {
		t.Errorf("expected expires_at=nil in message_new before read, got %v", expiresAt)
	}
}

func TestMarkRead_WithTTL_StartsCountdown(t *testing.T) {
	t.Parallel()

	var capturedExpiresAt time.Time

	svc := chat.NewService(
		&mockDialogRepo{},
		&mockMessageRepo{
			getByIDFn: func(_ context.Context, _ string) (store.Message, error) {
				return store.Message{
					ID:        msgID1,
					DialogID:  "d1",
					SenderID:  userA,
					Body:      msgBody,
					ExpiresAt: nil, // ещё не прочитано
				}, nil
			},
			setExpiresAtFn: func(_ context.Context, _ string, expiresAt time.Time) error {
				capturedExpiresAt = expiresAt
				return nil
			},
		},
		&mockReceiptRepo{
			markReadFn: func(_ context.Context, _, _ string, _ time.Time) error { return nil },
		},
		noopNotifier(),
		noopOutbox(),
		5*time.Minute,
	)

	readAt := time.Now().UTC()
	if err := svc.MarkRead(context.Background(), msgID1, userB, readAt); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}

	expected := readAt.Add(5 * time.Minute)
	if capturedExpiresAt.IsZero() {
		t.Fatal("expected SetExpiresAt to be called")
	}
	// Допускаем 1 секунду погрешности
	diff := capturedExpiresAt.Sub(expected)
	if diff < -time.Second || diff > time.Second {
		t.Errorf("expected expires_at ≈ %v, got %v", expected, capturedExpiresAt)
	}
}

func TestMarkRead_WithTTL_SendsTTLStartedEventToBoth(t *testing.T) {
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
					ID:        msgID1,
					DialogID:  "d1",
					SenderID:  userA,
					Body:      msgBody,
					ExpiresAt: nil,
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
		5*time.Minute,
	)

	if err := svc.MarkRead(context.Background(), msgID1, userB, time.Now()); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}

	var senderGotTTL, readerGotTTL bool
	for _, e := range events {
		if e.name == "message_ttl_started" {
			if e.userID == userA {
				senderGotTTL = true
			}
			if e.userID == userB {
				readerGotTTL = true
			}
		}
	}
	if !senderGotTTL {
		t.Error("sender (user-a) must receive message_ttl_started")
	}
	if !readerGotTTL {
		t.Error("reader (user-b) must receive message_ttl_started")
	}
}

func TestMarkRead_WithTTL_AlreadyExpiring_NoSecondStart(t *testing.T) {
	t.Parallel()

	setExpiresAtCalled := false
	alreadyExpiring := time.Now().UTC().Add(3 * time.Minute)

	svc := chat.NewService(
		&mockDialogRepo{},
		&mockMessageRepo{
			getByIDFn: func(_ context.Context, _ string) (store.Message, error) {
				return store.Message{
					ID:        msgID1,
					DialogID:  "d1",
					SenderID:  userA,
					Body:      msgBody,
					ExpiresAt: &alreadyExpiring, // уже прочитано ранее
				}, nil
			},
			setExpiresAtFn: func(_ context.Context, _ string, _ time.Time) error {
				setExpiresAtCalled = true
				return nil
			},
		},
		&mockReceiptRepo{
			markReadFn: func(_ context.Context, _, _ string, _ time.Time) error { return nil },
		},
		noopNotifier(),
		noopOutbox(),
		5*time.Minute,
	)

	if err := svc.MarkRead(context.Background(), msgID1, userB, time.Now()); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}

	if setExpiresAtCalled {
		t.Error("SetExpiresAt must NOT be called if message is already expiring (ExpiresAt != nil)")
	}
}

func TestMarkRead_WithoutTTL_NoExpiresAt(t *testing.T) {
	t.Parallel()

	setExpiresAtCalled := false

	svc := chat.NewService(
		&mockDialogRepo{},
		&mockMessageRepo{
			getByIDFn: func(_ context.Context, _ string) (store.Message, error) {
				return store.Message{
					ID:        msgID1,
					DialogID:  "d1",
					SenderID:  userA,
					Body:      msgBody,
					ExpiresAt: nil,
				}, nil
			},
			setExpiresAtFn: func(_ context.Context, _ string, _ time.Time) error {
				setExpiresAtCalled = true
				return nil
			},
		},
		&mockReceiptRepo{
			markReadFn: func(_ context.Context, _, _ string, _ time.Time) error { return nil },
		},
		noopNotifier(),
		noopOutbox(),
		noTTL,
	)

	if err := svc.MarkRead(context.Background(), msgID1, userB, time.Now()); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}

	if setExpiresAtCalled {
		t.Error("SetExpiresAt must NOT be called when TTL=0")
	}
}

func TestBuildPreview_NormalizesNewlines(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input string
		want  string
	}{
		{"hello\nworld", msgBodyNL},
		{"hello\r\nworld", msgBodyNL},
		{"hello\rworld", msgBodyNL},
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
					ID:       msgID1,
					DialogID: "d1",
					SenderID: userA,
					Body:     msgBody,
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

	if err := svc.MarkRead(context.Background(), msgID1, userB, time.Now()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(sent) != 2 {
		t.Fatalf("expected 2 events (message_read + badge_updated), got %d", len(sent))
	}

	// Первое событие — message_read отправителю.
	if sent[0].event.Event != eventMessageRead {
		t.Errorf("expected event[0]=message_read, got %q", sent[0].event.Event)
	}
	if sent[0].userID != userA {
		t.Errorf("expected message_read sent to sender user-a, got %q", sent[0].userID)
	}

	// Второе событие — badge_updated читателю.
	if sent[1].event.Event != "badge_updated" {
		t.Errorf("expected event[1]=badge_updated, got %q", sent[1].event.Event)
	}
	if sent[1].userID != userB {
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

func TestMarkRead_EnqueuesBadgeSyncForReader(t *testing.T) {
	t.Parallel()

	var enqueuedTasks []store.NotificationOutbox

	svc := chat.NewService(
		&mockDialogRepo{},
		&mockMessageRepo{
			getByIDFn: func(_ context.Context, _ string) (store.Message, error) {
				return store.Message{
					ID:       msgID1,
					DialogID: "d1",
					SenderID: userA,
					Body:     msgBody,
				}, nil
			},
		},
		&mockReceiptRepo{
			markReadFn: func(_ context.Context, _, _ string, _ time.Time) error { return nil },
		},
		noopNotifier(),
		&mockOutbox{
			enqueueFn: func(_ context.Context, task store.NotificationOutbox) error {
				enqueuedTasks = append(enqueuedTasks, task)
				return nil
			},
		},
		noTTL,
	)

	if err := svc.MarkRead(context.Background(), msgID1, userB, time.Now()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var badgeSyncTasks []store.NotificationOutbox
	for _, task := range enqueuedTasks {
		if task.EventType == "badge_sync" {
			badgeSyncTasks = append(badgeSyncTasks, task)
		}
	}

	if len(badgeSyncTasks) == 0 {
		t.Fatal("expected at least one badge_sync outbox task, got none")
	}

	task := badgeSyncTasks[0]
	if task.UserID != userB {
		t.Errorf("expected badge_sync task for reader %q, got user_id=%q", userB, task.UserID)
	}
	if task.DedupKey == "" {
		t.Error("expected non-empty dedup_key")
	}
}

func TestListDialogs_MapsPreviewAndNullLastMessage(t *testing.T) {
	t.Parallel()

	body := msgBody
	msgID := "m1"
	sender := userB
	created := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)

	svc := chat.NewService(
		&mockDialogRepo{
			listByUserIDFn: func(_ context.Context, userID string) ([]store.DialogListItem, error) {
				if userID != userA {
					t.Errorf("userID: want %q, got %q", userA, userID)
				}
				return []store.DialogListItem{
					{
						DialogID:             "d1",
						PeerUserID:           userB,
						PeerUsername:         peerUsername,
						LastMessageID:        &msgID,
						LastMessageSenderID:  &sender,
						LastMessageBody:      &body,
						LastMessageCreatedAt: &created,
						UnreadCount:          2,
						UpdatedAt:            created,
					},
					{
						DialogID:     "d2",
						PeerUserID:   "user-c",
						PeerUsername: "carol",
						UnreadCount:  0,
						UpdatedAt:    created.Add(-time.Hour),
					},
				}, nil
			},
		},
		&mockMessageRepo{},
		&mockReceiptRepo{},
		noopNotifier(),
		noopOutbox(),
		noTTL,
	)

	items, err := svc.ListDialogs(context.Background(), userA)
	if err != nil {
		t.Fatalf("ListDialogs: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len: want 2, got %d", len(items))
	}
	if items[0].LastMessage == nil || items[0].LastMessage.BodyPreview != body {
		t.Errorf("first last_message unexpected: %+v", items[0].LastMessage)
	}
	if items[1].LastMessage != nil {
		t.Errorf("second last_message: want nil, got %+v", items[1].LastMessage)
	}
	if items[0].Peer.Username != peerUsername || items[0].UnreadCount != 2 {
		t.Errorf("first item: %+v", items[0])
	}
}

func TestCreateDialogByUsername_SuccessAndIdempotent(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2026, 8, 6, 12, 5, 0, 0, time.UTC)
	getOrCreateCalls := 0

	dialogs := &mockDialogRepo{
		getOrCreateFn: func(_ context.Context, _, user1ID, user2ID string) (store.Dialog, error) {
			getOrCreateCalls++
			return store.Dialog{
				ID:        sharedDialogID,
				UserAID:   user1ID,
				UserBID:   user2ID,
				CreatedAt: createdAt,
			}, nil
		},
		listByUserIDFn: func(_ context.Context, _ string) ([]store.DialogListItem, error) {
			return []store.DialogListItem{{
				DialogID:     sharedDialogID,
				PeerUserID:   userB,
				PeerUsername: peerUsername,
				UnreadCount:  0,
				UpdatedAt:    createdAt,
			}}, nil
		},
	}

	users := &mockUserRepo{
		findByUsernameFn: func(_ context.Context, username string) (store.User, error) {
			if username != peerUsername {
				t.Errorf("username: want %q, got %q", peerUsername, username)
			}
			return store.User{ID: userB, Status: statusActive, Username: peerUsername}, nil
		},
	}

	svc := chat.NewService(dialogs, &mockMessageRepo{}, &mockReceiptRepo{}, noopNotifier(), noopOutbox(), noTTL)
	svc.SetUsersRepository(users)

	first, err := svc.CreateDialogByUsername(context.Background(), userA, "  Bob ")
	if err != nil {
		t.Fatalf("first create: %v", err)
	}
	second, err := svc.CreateDialogByUsername(context.Background(), userA, peerUsername)
	if err != nil {
		t.Fatalf("second create: %v", err)
	}

	if first.DialogID != sharedDialogID || second.DialogID != first.DialogID {
		t.Errorf("idempotent dialog ids: %q vs %q", first.DialogID, second.DialogID)
	}
	if getOrCreateCalls != 2 {
		t.Errorf("GetOrCreate calls: want 2, got %d", getOrCreateCalls)
	}
}

func TestCreateDialogByUsername_Errors(t *testing.T) {
	t.Parallel()

	users := &mockUserRepo{
		findByUsernameFn: func(_ context.Context, username string) (store.User, error) {
			switch username {
			case "alice":
				return store.User{ID: userA, Status: statusActive, Username: "alice"}, nil
			case "ghost":
				return store.User{}, store.ErrUserNotFound
			case blockedUser:
				return store.User{ID: userB, Status: blockedUser, Username: blockedUser}, nil
			default:
				return store.User{ID: userB, Status: statusActive, Username: username}, nil
			}
		},
	}

	svc := chat.NewService(
		&mockDialogRepo{},
		&mockMessageRepo{},
		&mockReceiptRepo{},
		noopNotifier(),
		noopOutbox(),
		noTTL,
	)
	svc.SetUsersRepository(users)

	if _, err := svc.CreateDialogByUsername(context.Background(), userA, "   "); !errors.Is(err, chat.ErrInvalidDialogUsername) {
		t.Errorf("empty: want ErrInvalidDialogUsername, got %v", err)
	}
	if _, err := svc.CreateDialogByUsername(context.Background(), userA, "alice"); !errors.Is(err, chat.ErrCannotDialogWithSelf) {
		t.Errorf("self: want ErrCannotDialogWithSelf, got %v", err)
	}
	if _, err := svc.CreateDialogByUsername(context.Background(), userA, "ghost"); !errors.Is(err, chat.ErrDialogUserNotFound) {
		t.Errorf("missing: want ErrDialogUserNotFound, got %v", err)
	}
	if _, err := svc.CreateDialogByUsername(context.Background(), userA, blockedUser); !errors.Is(err, chat.ErrDialogUserNotFound) {
		t.Errorf("blocked: want ErrDialogUserNotFound, got %v", err)
	}
}
