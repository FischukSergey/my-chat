//go:build integration

package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"my-chat/internal/store"
)

// insertDialog создаёт диалог между двумя пользователями и регистрирует удаление.
// Порядок user_a_id < user_b_id соблюдается автоматически (constraint dialogs_ordered_pair).
func insertDialog(t *testing.T, ctx context.Context, s *store.Store, userAID, userBID string) string {
	t.Helper()
	if userAID > userBID {
		userAID, userBID = userBID, userAID
	}
	id := uuid.NewString()
	_, err := s.DB().Exec(ctx,
		"INSERT INTO dialogs (id, user_a_id, user_b_id) VALUES ($1, $2, $3)",
		id, userAID, userBID,
	)
	if err != nil {
		t.Fatalf("insert dialog: %v", err)
	}
	t.Cleanup(func() {
		_, _ = s.DB().Exec(ctx, "DELETE FROM dialogs WHERE id = $1", id)
	})
	return id
}

// insertMessage вставляет сообщение напрямую через репозиторий.
func insertMessage(
	t *testing.T,
	ctx context.Context,
	repo *store.MessageRepository,
	dialogID, senderID string,
	expiresAt *time.Time,
) store.Message {
	t.Helper()
	m, err := repo.Create(ctx, store.Message{
		ID:        uuid.NewString(),
		DialogID:  dialogID,
		SenderID:  senderID,
		Body:      "test body",
		ExpiresAt: expiresAt,
	})
	if err != nil {
		t.Fatalf("Create message: %v", err)
	}
	return m
}

// --- MessageRepository.Create ---

func TestMessageRepository_Create_WithExpiresAt(t *testing.T) {
	s := setupDB(t)
	ctx := context.Background()
	userA := insertUser(t, ctx, s)
	userB := insertUser(t, ctx, s)
	dialogID := insertDialog(t, ctx, s, userA, userB)

	repo := store.NewMessageRepository(s)
	exp := time.Now().UTC().Add(5 * time.Minute).Truncate(time.Microsecond)

	m := insertMessage(t, ctx, repo, dialogID, userA, &exp)

	if m.ExpiresAt == nil {
		t.Fatal("expected ExpiresAt to be set")
	}
	if !m.ExpiresAt.Equal(exp) {
		t.Errorf("ExpiresAt mismatch: want %v, got %v", exp, *m.ExpiresAt)
	}
	if m.DeletedAt != nil {
		t.Error("expected DeletedAt to be nil on creation")
	}
}

func TestMessageRepository_Create_WithoutExpiresAt(t *testing.T) {
	s := setupDB(t)
	ctx := context.Background()
	userA := insertUser(t, ctx, s)
	userB := insertUser(t, ctx, s)
	dialogID := insertDialog(t, ctx, s, userA, userB)

	repo := store.NewMessageRepository(s)
	m := insertMessage(t, ctx, repo, dialogID, userA, nil)

	if m.ExpiresAt != nil {
		t.Errorf("expected ExpiresAt to be nil, got %v", *m.ExpiresAt)
	}
}

// --- MessageRepository.ListByDialog ---

func TestMessageRepository_ListByDialog_ExcludesDeleted(t *testing.T) {
	s := setupDB(t)
	ctx := context.Background()
	userA := insertUser(t, ctx, s)
	userB := insertUser(t, ctx, s)
	dialogID := insertDialog(t, ctx, s, userA, userB)

	repo := store.NewMessageRepository(s)

	// Активное сообщение.
	active := insertMessage(t, ctx, repo, dialogID, userA, nil)

	// Сообщение с истёкшим TTL — soft-delete руками.
	deleted := insertMessage(t, ctx, repo, dialogID, userA, nil)
	_, err := s.DB().Exec(ctx, "UPDATE messages SET deleted_at = now() WHERE id = $1", deleted.ID)
	if err != nil {
		t.Fatalf("manual soft-delete: %v", err)
	}

	msgs, err := repo.ListByDialog(ctx, dialogID, 50, nil)
	if err != nil {
		t.Fatalf("ListByDialog: %v", err)
	}

	for _, m := range msgs {
		if m.ID == deleted.ID {
			t.Errorf("deleted message %q must not appear in ListByDialog", deleted.ID)
		}
	}

	found := false
	for _, m := range msgs {
		if m.ID == active.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("active message %q must appear in ListByDialog", active.ID)
	}
}

// --- MessageRepository.ExpireMessages ---

func TestMessageRepository_ExpireMessages_MarksExpired(t *testing.T) {
	s := setupDB(t)
	ctx := context.Background()
	userA := insertUser(t, ctx, s)
	userB := insertUser(t, ctx, s)
	dialogID := insertDialog(t, ctx, s, userA, userB)

	repo := store.NewMessageRepository(s)

	// Сообщение с истёкшим TTL (в прошлом).
	pastExp := time.Now().UTC().Add(-time.Second)
	expired := insertMessage(t, ctx, repo, dialogID, userA, &pastExp)

	// Сообщение с будущим TTL — не должно быть затронуто.
	futureExp := time.Now().UTC().Add(time.Hour)
	future := insertMessage(t, ctx, repo, dialogID, userA, &futureExp)

	// Сообщение без TTL — не должно быть затронуто.
	noTTL := insertMessage(t, ctx, repo, dialogID, userA, nil)

	result, err := repo.ExpireMessages(ctx, time.Now().UTC(), 100)
	if err != nil {
		t.Fatalf("ExpireMessages: %v", err)
	}

	// Только expired должен попасть в результат.
	if len(result) < 1 {
		t.Fatal("expected at least 1 expired message")
	}
	var foundExpired bool
	for _, m := range result {
		if m.ID == expired.ID {
			foundExpired = true
			if m.DialogID != dialogID {
				t.Errorf("DialogID mismatch: want %q, got %q", dialogID, m.DialogID)
			}
		}
		if m.ID == future.ID {
			t.Errorf("future message %q must not be expired", future.ID)
		}
		if m.ID == noTTL.ID {
			t.Errorf("no-TTL message %q must not be expired", noTTL.ID)
		}
	}
	if !foundExpired {
		t.Errorf("expected expired message %q in result", expired.ID)
	}
}

func TestMessageRepository_ExpireMessages_Idempotent(t *testing.T) {
	s := setupDB(t)
	ctx := context.Background()
	userA := insertUser(t, ctx, s)
	userB := insertUser(t, ctx, s)
	dialogID := insertDialog(t, ctx, s, userA, userB)

	repo := store.NewMessageRepository(s)

	pastExp := time.Now().UTC().Add(-time.Second)
	insertMessage(t, ctx, repo, dialogID, userA, &pastExp)

	now := time.Now().UTC()

	first, err := repo.ExpireMessages(ctx, now, 100)
	if err != nil {
		t.Fatalf("first ExpireMessages: %v", err)
	}
	if len(first) == 0 {
		t.Fatal("expected at least 1 expired message on first call")
	}

	second, err := repo.ExpireMessages(ctx, now, 100)
	if err != nil {
		t.Fatalf("second ExpireMessages: %v", err)
	}
	for _, m := range second {
		for _, f := range first {
			if m.ID == f.ID {
				t.Errorf("message %q expired twice — ExpireMessages is not idempotent", m.ID)
			}
		}
	}
}

func TestMessageRepository_ExpireMessages_ListExcludesExpired(t *testing.T) {
	s := setupDB(t)
	ctx := context.Background()
	userA := insertUser(t, ctx, s)
	userB := insertUser(t, ctx, s)
	dialogID := insertDialog(t, ctx, s, userA, userB)

	repo := store.NewMessageRepository(s)

	pastExp := time.Now().UTC().Add(-time.Second)
	expired := insertMessage(t, ctx, repo, dialogID, userA, &pastExp)

	if _, err := repo.ExpireMessages(ctx, time.Now().UTC(), 100); err != nil {
		t.Fatalf("ExpireMessages: %v", err)
	}

	msgs, err := repo.ListByDialog(ctx, dialogID, 50, nil)
	if err != nil {
		t.Fatalf("ListByDialog after expire: %v", err)
	}
	for _, m := range msgs {
		if m.ID == expired.ID {
			t.Errorf("expired message %q must not appear in ListByDialog after ExpireMessages", expired.ID)
		}
	}
}

// --- MessageRepository.SetExpiresAt ---

func TestMessageRepository_SetExpiresAt_SetsOnFirstRead(t *testing.T) {
	s := setupDB(t)
	ctx := context.Background()
	userA := insertUser(t, ctx, s)
	userB := insertUser(t, ctx, s)
	dialogID := insertDialog(t, ctx, s, userA, userB)

	repo := store.NewMessageRepository(s)
	m := insertMessage(t, ctx, repo, dialogID, userA, nil) // создаём без TTL

	if m.ExpiresAt != nil {
		t.Fatalf("expected ExpiresAt nil at creation, got %v", *m.ExpiresAt)
	}

	exp := time.Now().UTC().Add(5 * time.Minute).Truncate(time.Microsecond)
	if err := repo.SetExpiresAt(ctx, m.ID, exp); err != nil {
		t.Fatalf("SetExpiresAt: %v", err)
	}

	// Проверяем через GetByID
	got, err := repo.GetByID(ctx, m.ID)
	if err != nil {
		t.Fatalf("GetByID after SetExpiresAt: %v", err)
	}
	if got.ExpiresAt == nil {
		t.Fatal("expected ExpiresAt to be set after SetExpiresAt")
	}
	if !got.ExpiresAt.Equal(exp) {
		t.Errorf("ExpiresAt mismatch: want %v, got %v", exp, *got.ExpiresAt)
	}
}

func TestMessageRepository_SetExpiresAt_Idempotent(t *testing.T) {
	s := setupDB(t)
	ctx := context.Background()
	userA := insertUser(t, ctx, s)
	userB := insertUser(t, ctx, s)
	dialogID := insertDialog(t, ctx, s, userA, userB)

	repo := store.NewMessageRepository(s)
	m := insertMessage(t, ctx, repo, dialogID, userA, nil)

	first := time.Now().UTC().Add(5 * time.Minute).Truncate(time.Microsecond)
	if err := repo.SetExpiresAt(ctx, m.ID, first); err != nil {
		t.Fatalf("first SetExpiresAt: %v", err)
	}

	// Повторный вызов с другим временем не должен перезаписывать.
	second := time.Now().UTC().Add(10 * time.Minute).Truncate(time.Microsecond)
	if err := repo.SetExpiresAt(ctx, m.ID, second); err != nil {
		t.Fatalf("second SetExpiresAt: %v", err)
	}

	got, err := repo.GetByID(ctx, m.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.ExpiresAt == nil || !got.ExpiresAt.Equal(first) {
		t.Errorf("expected first expires_at %v to be preserved, got %v", first, got.ExpiresAt)
	}
}

// --- UserRepository.FindByID ---

func TestUserRepository_FindByID_Found(t *testing.T) {
	s := setupDB(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, s)

	repo := store.NewUserRepository(s)

	u, err := repo.FindByID(ctx, userID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if u.ID != userID {
		t.Errorf("ID mismatch: want %q, got %q", userID, u.ID)
	}
	if u.Status != "active" {
		t.Errorf("expected status 'active', got %q", u.Status)
	}
}

func TestUserRepository_FindByID_NotFound(t *testing.T) {
	s := setupDB(t)
	ctx := context.Background()

	repo := store.NewUserRepository(s)

	_, err := repo.FindByID(ctx, uuid.NewString())
	if err == nil {
		t.Fatal("expected ErrUserNotFound, got nil")
	}
	if err != store.ErrUserNotFound {
		t.Errorf("expected ErrUserNotFound, got %v", err)
	}
}
