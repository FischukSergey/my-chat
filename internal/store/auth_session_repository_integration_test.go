//go:build integration

package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"my-chat/internal/store"
)

// newSession — вспомогательная функция для создания тестовой сессии с TTL 1 час.
func newSession(userID, familyID string) store.AuthSession {
	now := time.Now().UTC()
	return store.AuthSession{
		ID:        uuid.NewString(),
		UserID:    userID,
		FamilyID:  familyID,
		TokenHash: "hash-" + uuid.NewString(),
		ExpiresAt: now.Add(time.Hour),
		CreatedAt: now,
	}
}

// --- Create + FindByTokenHash ---

func TestAuthSessionRepository_Create_And_FindByTokenHash(t *testing.T) {
	s := setupDB(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, s)

	repo := store.NewAuthSessionRepository(s)
	sess := newSession(userID, uuid.NewString())

	if err := repo.Create(ctx, sess); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.FindByTokenHash(ctx, sess.TokenHash)
	if err != nil {
		t.Fatalf("FindByTokenHash: %v", err)
	}

	if got.ID != sess.ID {
		t.Errorf("ID mismatch: want %q, got %q", sess.ID, got.ID)
	}
	if got.UserID != sess.UserID {
		t.Errorf("UserID mismatch: want %q, got %q", sess.UserID, got.UserID)
	}
	if got.FamilyID != sess.FamilyID {
		t.Errorf("FamilyID mismatch: want %q, got %q", sess.FamilyID, got.FamilyID)
	}
	if got.IsRevoked() {
		t.Error("expected session to not be revoked after Create")
	}
}

func TestAuthSessionRepository_FindByTokenHash_NotFound(t *testing.T) {
	s := setupDB(t)
	ctx := context.Background()

	repo := store.NewAuthSessionRepository(s)

	_, err := repo.FindByTokenHash(ctx, "nonexistent-hash")
	if err == nil {
		t.Fatal("expected ErrSessionNotFound, got nil")
	}
	if err != store.ErrSessionNotFound {
		t.Errorf("expected ErrSessionNotFound, got %v", err)
	}
}

// --- RevokeSession ---

func TestAuthSessionRepository_RevokeSession(t *testing.T) {
	s := setupDB(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, s)

	repo := store.NewAuthSessionRepository(s)
	sess := newSession(userID, uuid.NewString())

	if err := repo.Create(ctx, sess); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := repo.RevokeSession(ctx, sess.ID); err != nil {
		t.Fatalf("RevokeSession: %v", err)
	}

	got, err := repo.FindByTokenHash(ctx, sess.TokenHash)
	if err != nil {
		t.Fatalf("FindByTokenHash after revoke: %v", err)
	}
	if !got.IsRevoked() {
		t.Error("expected session to be revoked")
	}
}

func TestAuthSessionRepository_RevokeSession_Idempotent(t *testing.T) {
	s := setupDB(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, s)

	repo := store.NewAuthSessionRepository(s)
	sess := newSession(userID, uuid.NewString())

	if err := repo.Create(ctx, sess); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Первый revoke.
	if err := repo.RevokeSession(ctx, sess.ID); err != nil {
		t.Fatalf("first RevokeSession: %v", err)
	}
	// Повторный revoke — не должен ошибаться.
	if err := repo.RevokeSession(ctx, sess.ID); err != nil {
		t.Errorf("second RevokeSession (idempotent) must not fail, got: %v", err)
	}
}

// --- RevokeFamily ---

func TestAuthSessionRepository_RevokeFamily(t *testing.T) {
	s := setupDB(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, s)

	repo := store.NewAuthSessionRepository(s)
	familyID := uuid.NewString()

	// Создаём две сессии одной family и одну из другой.
	s1 := newSession(userID, familyID)
	s2 := newSession(userID, familyID)
	other := newSession(userID, uuid.NewString())

	for _, sess := range []store.AuthSession{s1, s2, other} {
		if err := repo.Create(ctx, sess); err != nil {
			t.Fatalf("Create %q: %v", sess.ID, err)
		}
	}

	if err := repo.RevokeFamily(ctx, familyID); err != nil {
		t.Fatalf("RevokeFamily: %v", err)
	}

	// Обе сессии family должны быть revoked.
	for _, sess := range []store.AuthSession{s1, s2} {
		got, err := repo.FindByTokenHash(ctx, sess.TokenHash)
		if err != nil {
			t.Fatalf("FindByTokenHash %q: %v", sess.ID, err)
		}
		if !got.IsRevoked() {
			t.Errorf("session %q in family must be revoked", sess.ID)
		}
	}

	// Сессия другой family не затронута.
	got, err := repo.FindByTokenHash(ctx, other.TokenHash)
	if err != nil {
		t.Fatalf("FindByTokenHash other: %v", err)
	}
	if got.IsRevoked() {
		t.Error("session in other family must not be revoked")
	}
}

// --- RevokeAllForUser ---

func TestAuthSessionRepository_RevokeAllForUser(t *testing.T) {
	s := setupDB(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, s)
	otherUserID := insertUser(t, ctx, s)

	repo := store.NewAuthSessionRepository(s)

	// Две сессии целевого пользователя.
	ua := newSession(userID, uuid.NewString())
	ub := newSession(userID, uuid.NewString())
	// Сессия другого пользователя.
	oc := newSession(otherUserID, uuid.NewString())

	for _, sess := range []store.AuthSession{ua, ub, oc} {
		if err := repo.Create(ctx, sess); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	if err := repo.RevokeAllForUser(ctx, userID); err != nil {
		t.Fatalf("RevokeAllForUser: %v", err)
	}

	// Сессии целевого пользователя revoked.
	for _, sess := range []store.AuthSession{ua, ub} {
		got, err := repo.FindByTokenHash(ctx, sess.TokenHash)
		if err != nil {
			t.Fatalf("FindByTokenHash: %v", err)
		}
		if !got.IsRevoked() {
			t.Errorf("session %q must be revoked", sess.ID)
		}
	}

	// Сессия другого пользователя не затронута.
	got, err := repo.FindByTokenHash(ctx, oc.TokenHash)
	if err != nil {
		t.Fatalf("FindByTokenHash other: %v", err)
	}
	if got.IsRevoked() {
		t.Error("other user session must not be revoked")
	}
}

// --- RotateSession ---

func TestAuthSessionRepository_RotateSession(t *testing.T) {
	s := setupDB(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, s)

	repo := store.NewAuthSessionRepository(s)
	familyID := uuid.NewString()
	now := time.Now().UTC()

	old := newSession(userID, familyID)
	if err := repo.Create(ctx, old); err != nil {
		t.Fatalf("Create old: %v", err)
	}

	rotatedFrom := old.ID
	next := store.AuthSession{
		ID:          uuid.NewString(),
		UserID:      userID,
		FamilyID:    familyID,
		TokenHash:   "hash-" + uuid.NewString(),
		ExpiresAt:   now.Add(time.Hour),
		CreatedAt:   now,
		RotatedFrom: &rotatedFrom,
	}

	if err := repo.RotateSession(ctx, old.ID, next); err != nil {
		t.Fatalf("RotateSession: %v", err)
	}

	// Старая сессия должна быть revoked.
	gotOld, err := repo.FindByTokenHash(ctx, old.TokenHash)
	if err != nil {
		t.Fatalf("FindByTokenHash old: %v", err)
	}
	if !gotOld.IsRevoked() {
		t.Error("old session must be revoked after rotation")
	}

	// Новая сессия должна быть активна.
	gotNew, err := repo.FindByTokenHash(ctx, next.TokenHash)
	if err != nil {
		t.Fatalf("FindByTokenHash new: %v", err)
	}
	if gotNew.IsRevoked() {
		t.Error("new session must not be revoked")
	}
	if gotNew.RotatedFrom == nil || *gotNew.RotatedFrom != old.ID {
		t.Errorf("new session RotatedFrom must be %q, got %v", old.ID, gotNew.RotatedFrom)
	}
}

// --- IsExpired ---

func TestAuthSession_IsExpired(t *testing.T) {
	expired := store.AuthSession{ExpiresAt: time.Now().UTC().Add(-time.Minute)}
	if !expired.IsExpired() {
		t.Error("expected IsExpired() = true for past expires_at")
	}

	active := store.AuthSession{ExpiresAt: time.Now().UTC().Add(time.Hour)}
	if active.IsExpired() {
		t.Error("expected IsExpired() = false for future expires_at")
	}
}
