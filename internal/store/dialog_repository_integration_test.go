//go:build integration

package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"my-chat/internal/store"
)

func insertUserWithUsername(t *testing.T, ctx context.Context, s *store.Store, username string) string {
	t.Helper()

	id := uuid.NewString()
	_, err := s.DB().Exec(ctx,
		"INSERT INTO users (id, username) VALUES ($1, $2)",
		id, username,
	)
	if err != nil {
		t.Fatalf("insert user with username: %v", err)
	}

	t.Cleanup(func() {
		_, _ = s.DB().Exec(ctx, "DELETE FROM users WHERE id = $1", id)
	})

	return id
}

func TestDialogRepository_GetOrCreate_RejectsSameUser(t *testing.T) {
	s := setupDB(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, s)

	repo := store.NewDialogRepository(s)
	_, err := repo.GetOrCreate(ctx, uuid.NewString(), userID, userID)
	if err == nil {
		t.Fatal("expected error for same users")
	}
}

func TestDialogRepository_ListByUserID_Empty(t *testing.T) {
	s := setupDB(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, s)

	repo := store.NewDialogRepository(s)
	items, err := repo.ListByUserID(ctx, userID)
	if err != nil {
		t.Fatalf("ListByUserID: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected empty list, got %d", len(items))
	}
}

func TestDialogRepository_ListByUserID_PeerUsernameAndEmptyLastMessage(t *testing.T) {
	s := setupDB(t)
	ctx := context.Background()

	alice := insertUserWithUsername(t, ctx, s, "alice_"+uuid.NewString()[:8])
	bob := insertUserWithUsername(t, ctx, s, "bob_"+uuid.NewString()[:8])
	dialogID := insertDialog(t, ctx, s, alice, bob)

	repo := store.NewDialogRepository(s)
	items, err := repo.ListByUserID(ctx, alice)
	if err != nil {
		t.Fatalf("ListByUserID: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 dialog, got %d", len(items))
	}

	item := items[0]
	if item.DialogID != dialogID {
		t.Errorf("DialogID: want %q, got %q", dialogID, item.DialogID)
	}
	if item.PeerUserID != bob {
		t.Errorf("PeerUserID: want %q, got %q", bob, item.PeerUserID)
	}
	if item.PeerUsername == "" || item.PeerUsername[:4] != "bob_" {
		t.Errorf("PeerUsername unexpected: %q", item.PeerUsername)
	}
	if item.LastMessageID != nil || item.LastMessageBody != nil {
		t.Error("expected nil last message for empty dialog")
	}
	if item.UnreadCount != 0 {
		t.Errorf("UnreadCount: want 0, got %d", item.UnreadCount)
	}
	if item.UpdatedAt.IsZero() {
		t.Error("UpdatedAt must be set from dialog.created_at")
	}
}

func TestDialogRepository_ListByUserID_LastMessageExcludesSoftDeleted(t *testing.T) {
	s := setupDB(t)
	ctx := context.Background()

	alice := insertUserWithUsername(t, ctx, s, "alice_"+uuid.NewString()[:8])
	bob := insertUserWithUsername(t, ctx, s, "bob_"+uuid.NewString()[:8])
	dialogID := insertDialog(t, ctx, s, alice, bob)

	msgRepo := store.NewMessageRepository(s)
	active, err := msgRepo.Create(ctx, store.Message{
		ID:       uuid.NewString(),
		DialogID: dialogID,
		SenderID: bob,
		Body:     "active preview",
	})
	if err != nil {
		t.Fatalf("Create active: %v", err)
	}

	// Более новое, но soft-deleted — не должно стать preview.
	time.Sleep(5 * time.Millisecond)
	deleted, err := msgRepo.Create(ctx, store.Message{
		ID:       uuid.NewString(),
		DialogID: dialogID,
		SenderID: bob,
		Body:     "deleted preview",
	})
	if err != nil {
		t.Fatalf("Create deleted: %v", err)
	}
	if _, err = s.DB().Exec(ctx, "UPDATE messages SET deleted_at = now() WHERE id = $1", deleted.ID); err != nil {
		t.Fatalf("soft-delete: %v", err)
	}

	repo := store.NewDialogRepository(s)
	items, err := repo.ListByUserID(ctx, alice)
	if err != nil {
		t.Fatalf("ListByUserID: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 dialog, got %d", len(items))
	}

	item := items[0]
	if item.LastMessageID == nil || *item.LastMessageID != active.ID {
		t.Errorf("last message id: want %q, got %v", active.ID, item.LastMessageID)
	}
	if item.LastMessageBody == nil || *item.LastMessageBody != "active preview" {
		t.Errorf("last message body: want %q, got %v", "active preview", item.LastMessageBody)
	}
	if item.LastMessageSenderID == nil || *item.LastMessageSenderID != bob {
		t.Errorf("last message sender: want %q, got %v", bob, item.LastMessageSenderID)
	}
	if item.LastMessageCreatedAt == nil || !item.LastMessageCreatedAt.Equal(active.CreatedAt) {
		t.Errorf("LastMessageCreatedAt: want %v, got %v", active.CreatedAt, item.LastMessageCreatedAt)
	}
	if !item.UpdatedAt.Equal(active.CreatedAt) {
		t.Errorf("UpdatedAt: want %v, got %v", active.CreatedAt, item.UpdatedAt)
	}
}

func TestDialogRepository_ListByUserID_UnreadAndSort(t *testing.T) {
	s := setupDB(t)
	ctx := context.Background()

	alice := insertUserWithUsername(t, ctx, s, "alice_"+uuid.NewString()[:8])
	bob := insertUserWithUsername(t, ctx, s, "bob_"+uuid.NewString()[:8])
	carol := insertUserWithUsername(t, ctx, s, "carol_"+uuid.NewString()[:8])

	dialogOld := insertDialog(t, ctx, s, alice, carol)
	dialogNew := insertDialog(t, ctx, s, alice, bob)

	msgRepo := store.NewMessageRepository(s)
	receipts := store.NewReceiptRepository(s)

	// Старый диалог с сообщением раньше.
	oldMsg, err := msgRepo.Create(ctx, store.Message{
		ID:       uuid.NewString(),
		DialogID: dialogOld,
		SenderID: carol,
		Body:     "old",
	})
	if err != nil {
		t.Fatalf("Create old: %v", err)
	}
	if err = receipts.Ensure(ctx, oldMsg.ID, alice); err != nil {
		t.Fatalf("Ensure receipt: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	// Новый диалог — более свежее сообщение + unread.
	newMsg, err := msgRepo.Create(ctx, store.Message{
		ID:       uuid.NewString(),
		DialogID: dialogNew,
		SenderID: bob,
		Body:     "new",
	})
	if err != nil {
		t.Fatalf("Create new: %v", err)
	}
	if err = receipts.Ensure(ctx, newMsg.ID, alice); err != nil {
		t.Fatalf("Ensure receipt new: %v", err)
	}

	// Soft-deleted unread не должен считаться.
	deletedUnread, err := msgRepo.Create(ctx, store.Message{
		ID:       uuid.NewString(),
		DialogID: dialogNew,
		SenderID: bob,
		Body:     "gone",
	})
	if err != nil {
		t.Fatalf("Create deleted unread: %v", err)
	}
	if _, err = s.DB().Exec(ctx, "UPDATE messages SET deleted_at = now() WHERE id = $1", deletedUnread.ID); err != nil {
		t.Fatalf("soft-delete unread: %v", err)
	}

	repo := store.NewDialogRepository(s)
	items, err := repo.ListByUserID(ctx, alice)
	if err != nil {
		t.Fatalf("ListByUserID: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 dialogs, got %d", len(items))
	}

	if items[0].DialogID != dialogNew {
		t.Errorf("first dialog should be newest: want %q, got %q", dialogNew, items[0].DialogID)
	}
	if items[1].DialogID != dialogOld {
		t.Errorf("second dialog should be older: want %q, got %q", dialogOld, items[1].DialogID)
	}

	if items[0].UnreadCount != 1 {
		t.Errorf("dialogNew unread: want 1, got %d", items[0].UnreadCount)
	}
	if items[1].UnreadCount != 1 {
		t.Errorf("dialogOld unread: want 1, got %d", items[1].UnreadCount)
	}

	// Mark new as read → unread 0.
	if err = receipts.MarkRead(ctx, newMsg.ID, alice, time.Now().UTC()); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}

	items, err = repo.ListByUserID(ctx, alice)
	if err != nil {
		t.Fatalf("ListByUserID after read: %v", err)
	}
	for _, item := range items {
		if item.DialogID == dialogNew && item.UnreadCount != 0 {
			t.Errorf("after MarkRead unread: want 0, got %d", item.UnreadCount)
		}
	}
}

func TestDialogRepository_ListByUserID_OnlyCallerDialogs(t *testing.T) {
	s := setupDB(t)
	ctx := context.Background()

	alice := insertUser(t, ctx, s)
	bob := insertUser(t, ctx, s)
	carol := insertUser(t, ctx, s)

	mine := insertDialog(t, ctx, s, alice, bob)
	_ = insertDialog(t, ctx, s, bob, carol)

	repo := store.NewDialogRepository(s)
	items, err := repo.ListByUserID(ctx, alice)
	if err != nil {
		t.Fatalf("ListByUserID: %v", err)
	}
	if len(items) != 1 || items[0].DialogID != mine {
		t.Fatalf("expected only alice dialog %q, got %+v", mine, items)
	}
}
