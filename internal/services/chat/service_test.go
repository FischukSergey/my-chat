// Package chat_test contains unit tests for the chat service.
package chat_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"my-chat/internal/hub"
	chat "my-chat/internal/services/chat"
	"my-chat/internal/store"
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
	return m.countUnreadFn(ctx, userID)
}

type mockNotifier struct {
	sendFn func(ctx context.Context, userID string, event hub.Event) bool
}

func (m *mockNotifier) Send(ctx context.Context, userID string, event hub.Event) bool {
	return m.sendFn(ctx, userID, event)
}

// --- helpers ---

func noopNotifier() *mockNotifier {
	return &mockNotifier{
		sendFn: func(_ context.Context, _ string, _ hub.Event) bool { return false },
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
	if eventNames[0] != "message_new" {
		t.Errorf("expected event[0]=message_new, got %q", eventNames[0])
	}
	if eventNames[1] != "message_delivered" {
		t.Errorf("expected event[1]=message_delivered, got %q", eventNames[1])
	}
}

func TestMarkRead_NotifiesSender(t *testing.T) {
	t.Parallel()

	var notifiedUserID string
	var notifiedEventName string

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
				notifiedUserID = userID
				notifiedEventName = event.Event
				return true
			},
		},
	)

	if err := svc.MarkRead(context.Background(), "msg-1", "user-b", time.Now()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if notifiedUserID != "user-a" {
		t.Errorf("expected notification to sender user-a, got %q", notifiedUserID)
	}
	if notifiedEventName != "message_read" {
		t.Errorf("expected event message_read, got %q", notifiedEventName)
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
	)

	count, err := svc.UnreadCount(context.Background(), "user-a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 5 {
		t.Errorf("expected 5, got %d", count)
	}
}
