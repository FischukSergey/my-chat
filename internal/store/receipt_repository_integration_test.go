//go:build integration

package store_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"my-chat/internal/store"
)

func TestReceiptRepository_CountUnread_ExcludesSoftDeleted(t *testing.T) {
	s := setupDB(t)
	ctx := context.Background()
	alice := insertUser(t, ctx, s)
	bob := insertUser(t, ctx, s)
	dialogID := insertDialog(t, ctx, s, alice, bob)

	messages := store.NewMessageRepository(s)
	receipts := store.NewReceiptRepository(s)

	keep, err := messages.Create(ctx, store.Message{
		ID:       uuid.NewString(),
		DialogID: dialogID,
		SenderID: bob,
		Body:     "keep unread",
	})
	if err != nil {
		t.Fatalf("Create keep: %v", err)
	}
	if err = receipts.Ensure(ctx, keep.ID, alice); err != nil {
		t.Fatalf("Ensure keep: %v", err)
	}

	gone, err := messages.Create(ctx, store.Message{
		ID:       uuid.NewString(),
		DialogID: dialogID,
		SenderID: bob,
		Body:     "deleted unread",
	})
	if err != nil {
		t.Fatalf("Create gone: %v", err)
	}
	if err = receipts.Ensure(ctx, gone.ID, alice); err != nil {
		t.Fatalf("Ensure gone: %v", err)
	}
	ok, err := messages.SoftDelete(ctx, gone.ID)
	if err != nil || !ok {
		t.Fatalf("SoftDelete: ok=%v err=%v", ok, err)
	}

	got, err := receipts.CountUnread(ctx, alice)
	if err != nil {
		t.Fatalf("CountUnread: %v", err)
	}
	if got != 1 {
		t.Errorf("CountUnread after unsend: want 1, got %d", got)
	}
}
