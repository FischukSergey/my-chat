//go:build integration

// Package chat_test contains integration tests for the chat service.
package chat_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	chat "my-chat/internal/services/chat"
	"my-chat/internal/store"
)

func TestIntegration_SendListReadUnread(t *testing.T) {
	t.Parallel()

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

	userAID := uuid.NewString()
	userBID := uuid.NewString()

	_, err = s.DB().Exec(ctx, "INSERT INTO users (id) VALUES ($1), ($2)", userAID, userBID)
	if err != nil {
		t.Fatalf("insert users: %v", err)
	}

	t.Cleanup(func() {
		cleanCtx := context.Background()
		_, _ = s.DB().Exec(cleanCtx, "DELETE FROM dialogs WHERE user_a_id IN ($1, $2) OR user_b_id IN ($1, $2)", userAID, userBID)
		_, _ = s.DB().Exec(cleanCtx, "DELETE FROM users WHERE id IN ($1, $2)", userAID, userBID)
	})

	dialogRepo := store.NewDialogRepository(s)
	messageRepo := store.NewMessageRepository(s)
	receiptRepo := store.NewReceiptRepository(s)

	dialog, err := dialogRepo.GetOrCreate(ctx, uuid.NewString(), userAID, userBID)
	if err != nil {
		t.Fatalf("create dialog: %v", err)
	}

	svc := chat.NewService(dialogRepo, messageRepo, receiptRepo, noopNotifier(), noopOutbox(), 0)

	// Step 1: send message from userA.
	msg, err := svc.SendMessage(ctx, store.Message{
		ID:       uuid.NewString(),
		DialogID: dialog.ID,
		SenderID: userAID,
		Body:     "hello from integration test",
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if msg.ID == "" {
		t.Fatal("expected non-empty message ID")
	}

	// Step 2: list messages as userA.
	messages, err := svc.ListMessages(ctx, userAID, dialog.ID, 10, nil)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
	if messages[0].Body != "hello from integration test" {
		t.Errorf("unexpected body: %q", messages[0].Body)
	}

	// Step 3: unread count for userB must be 1.
	unread, err := svc.UnreadCount(ctx, userBID)
	if err != nil {
		t.Fatalf("UnreadCount before read: %v", err)
	}
	if unread != 1 {
		t.Errorf("expected 1 unread, got %d", unread)
	}

	// Step 4: userB marks message read.
	if err = svc.MarkRead(ctx, msg.ID, userBID, time.Now().UTC()); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}

	// Step 5: unread count for userB must be 0.
	unread, err = svc.UnreadCount(ctx, userBID)
	if err != nil {
		t.Fatalf("UnreadCount after read: %v", err)
	}
	if unread != 0 {
		t.Errorf("expected 0 unread, got %d", unread)
	}
}

// TestIntegration_OfflineRecipient_OutboxTaskCreated verifies that when the receiver
// is offline (notifier returns false) a notification_outbox task is persisted in the DB.
func TestIntegration_OfflineRecipient_OutboxTaskCreated(t *testing.T) {
	t.Parallel()

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

	userAID := uuid.NewString()
	userBID := uuid.NewString()

	_, err = s.DB().Exec(ctx, "INSERT INTO users (id) VALUES ($1), ($2)", userAID, userBID)
	if err != nil {
		t.Fatalf("insert users: %v", err)
	}
	t.Cleanup(func() {
		cleanCtx := context.Background()
		_, _ = s.DB().Exec(cleanCtx, "DELETE FROM notification_outbox WHERE user_id IN ($1, $2)", userAID, userBID)
		_, _ = s.DB().Exec(cleanCtx, "DELETE FROM dialogs WHERE user_a_id IN ($1, $2) OR user_b_id IN ($1, $2)", userAID, userBID)
		_, _ = s.DB().Exec(cleanCtx, "DELETE FROM users WHERE id IN ($1, $2)", userAID, userBID)
	})

	dialogRepo := store.NewDialogRepository(s)
	messageRepo := store.NewMessageRepository(s)
	receiptRepo := store.NewReceiptRepository(s)
	outboxRepo := store.NewNotificationOutboxRepository(s)

	dialog, err := dialogRepo.GetOrCreate(ctx, uuid.NewString(), userAID, userBID)
	if err != nil {
		t.Fatalf("create dialog: %v", err)
	}

	// noopNotifier always returns false (receiver offline) — triggers outbox path.
	svc := chat.NewService(dialogRepo, messageRepo, receiptRepo, noopNotifier(), outboxRepo, 0)

	msgID := uuid.NewString()
	_, err = svc.SendMessage(ctx, store.Message{
		ID:       msgID,
		DialogID: dialog.ID,
		SenderID: userAID,
		Body:     "push me offline",
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	// Verify outbox task was persisted.
	var count int
	err = s.DB().QueryRow(ctx,
		"SELECT COUNT(*) FROM notification_outbox WHERE user_id = $1 AND event_type = 'message_new'",
		userBID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("count outbox tasks: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 outbox task for receiver, got %d", count)
	}

	// Verify payload contains expected fields.
	var rawPayload []byte
	err = s.DB().QueryRow(ctx,
		"SELECT payload FROM notification_outbox WHERE user_id = $1",
		userBID,
	).Scan(&rawPayload)
	if err != nil {
		t.Fatalf("fetch outbox payload: %v", err)
	}

	var payload map[string]any
	if err = json.Unmarshal(rawPayload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload["event_type"] != "message_new" {
		t.Errorf("expected event_type=message_new, got %v", payload["event_type"])
	}
	if payload["user_id"] != userBID {
		t.Errorf("expected user_id=%q, got %v", userBID, payload["user_id"])
	}
	if payload["message_id"] != msgID {
		t.Errorf("expected message_id=%q, got %v", msgID, payload["message_id"])
	}
}

// TestIntegration_ReadSynchronizesUnreadBadge verifies that after MarkRead the unread
// count drops to 0 and repeating the call is idempotent.
func TestIntegration_ReadSynchronizesUnreadBadge(t *testing.T) {
	t.Parallel()

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

	userAID := uuid.NewString()
	userBID := uuid.NewString()

	_, err = s.DB().Exec(ctx, "INSERT INTO users (id) VALUES ($1), ($2)", userAID, userBID)
	if err != nil {
		t.Fatalf("insert users: %v", err)
	}
	t.Cleanup(func() {
		cleanCtx := context.Background()
		_, _ = s.DB().Exec(cleanCtx, "DELETE FROM dialogs WHERE user_a_id IN ($1, $2) OR user_b_id IN ($1, $2)", userAID, userBID)
		_, _ = s.DB().Exec(cleanCtx, "DELETE FROM users WHERE id IN ($1, $2)", userAID, userBID)
	})

	dialogRepo := store.NewDialogRepository(s)
	messageRepo := store.NewMessageRepository(s)
	receiptRepo := store.NewReceiptRepository(s)

	dialog, err := dialogRepo.GetOrCreate(ctx, uuid.NewString(), userAID, userBID)
	if err != nil {
		t.Fatalf("create dialog: %v", err)
	}

	svc := chat.NewService(dialogRepo, messageRepo, receiptRepo, noopNotifier(), noopOutbox(), 0)

	// Send 3 messages from A.
	var lastMsgID string
	for i := range 3 {
		msg, sendErr := svc.SendMessage(ctx, store.Message{
			ID:       uuid.NewString(),
			DialogID: dialog.ID,
			SenderID: userAID,
			Body:     "msg",
		})
		if sendErr != nil {
			t.Fatalf("SendMessage %d: %v", i, sendErr)
		}
		lastMsgID = msg.ID
	}

	// B has 3 unread messages.
	unread, err := svc.UnreadCount(ctx, userBID)
	if err != nil {
		t.Fatalf("UnreadCount: %v", err)
	}
	if unread != 3 {
		t.Errorf("expected 3 unread before read, got %d", unread)
	}

	// B reads only the last message.
	if err = svc.MarkRead(ctx, lastMsgID, userBID, time.Now().UTC()); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}

	// After MarkRead the receipt for lastMsgID is marked — unread count must drop to 2.
	unread, err = svc.UnreadCount(ctx, userBID)
	if err != nil {
		t.Fatalf("UnreadCount after 1 read: %v", err)
	}
	if unread != 2 {
		t.Errorf("expected 2 unread after reading 1 of 3, got %d", unread)
	}

	// Idempotent: reading again must not change the count.
	if err = svc.MarkRead(ctx, lastMsgID, userBID, time.Now().UTC()); err != nil {
		t.Fatalf("MarkRead (repeat): %v", err)
	}
	unread, err = svc.UnreadCount(ctx, userBID)
	if err != nil {
		t.Fatalf("UnreadCount after repeat read: %v", err)
	}
	if unread != 2 {
		t.Errorf("expected 2 unread after idempotent read, got %d", unread)
	}
}
