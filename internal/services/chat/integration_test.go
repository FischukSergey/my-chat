//go:build integration

// Package chat_test contains integration tests for the chat service.
package chat_test

import (
	"context"
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

	if err = s.Migrate(ctx); err != nil {
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

	svc := chat.NewService(dialogRepo, messageRepo, receiptRepo, noopNotifier())

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
