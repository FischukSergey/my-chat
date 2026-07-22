package auth_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	auth "my-chat/internal/services/auth"
	"my-chat/internal/store"
)

// --- mock ---

type mockSessionRepo struct {
	createFn          func(ctx context.Context, s store.AuthSession) error
	findByTokenHashFn func(ctx context.Context, hash string) (store.AuthSession, error)
	revokeSessionFn   func(ctx context.Context, sessionID string) error
	revokeFamilyFn    func(ctx context.Context, familyID string) error
	revokeAllFn       func(ctx context.Context, userID string) error
	rotateSessionFn   func(ctx context.Context, oldID string, newSess store.AuthSession) error
}

func (m *mockSessionRepo) Create(ctx context.Context, s store.AuthSession) error {
	if m.createFn != nil {
		return m.createFn(ctx, s)
	}

	return nil
}

func (m *mockSessionRepo) FindByTokenHash(ctx context.Context, hash string) (store.AuthSession, error) {
	if m.findByTokenHashFn != nil {
		return m.findByTokenHashFn(ctx, hash)
	}

	return store.AuthSession{}, store.ErrSessionNotFound
}

func (m *mockSessionRepo) RevokeSession(ctx context.Context, sessionID string) error {
	if m.revokeSessionFn != nil {
		return m.revokeSessionFn(ctx, sessionID)
	}

	return nil
}

func (m *mockSessionRepo) RevokeFamily(ctx context.Context, familyID string) error {
	if m.revokeFamilyFn != nil {
		return m.revokeFamilyFn(ctx, familyID)
	}

	return nil
}

func (m *mockSessionRepo) RevokeAllForUser(ctx context.Context, userID string) error {
	if m.revokeAllFn != nil {
		return m.revokeAllFn(ctx, userID)
	}

	return nil
}

func (m *mockSessionRepo) RotateSession(ctx context.Context, oldID string, newSess store.AuthSession) error {
	if m.rotateSessionFn != nil {
		return m.rotateSessionFn(ctx, oldID, newSess)
	}

	return nil
}

// --- helpers ---

func testConfig() auth.Config {
	return auth.Config{
		JWTSecret:       "test-secret",
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 24 * time.Hour,
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func newService(repo *mockSessionRepo) *auth.Service {
	return auth.NewService(repo, testConfig(), testLogger())
}

// --- Login ---

func TestLogin_ReturnsTokenPairAndCreatesSession(t *testing.T) {
	var created store.AuthSession
	repo := &mockSessionRepo{
		createFn: func(_ context.Context, s store.AuthSession) error {
			created = s
			return nil
		},
	}

	pair, err := newService(repo).Login(context.Background(), "user-1", nil)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	if pair.AccessToken == "" {
		t.Error("expected non-empty AccessToken")
	}
	if pair.RefreshToken == "" {
		t.Error("expected non-empty RefreshToken")
	}
	if pair.SessionID == "" {
		t.Error("expected non-empty SessionID")
	}
	if pair.ExpiresIn != int((15 * time.Minute).Seconds()) {
		t.Errorf("ExpiresIn: want %d, got %d", int((15 * time.Minute).Seconds()), pair.ExpiresIn)
	}
	if created.UserID != "user-1" {
		t.Errorf("created session UserID: want %q, got %q", "user-1", created.UserID)
	}
	if created.ID != pair.SessionID {
		t.Errorf("created session ID must equal pair.SessionID")
	}
	if created.TokenHash == "" {
		t.Error("expected non-empty TokenHash in created session")
	}
}

func TestLogin_RepoError_ReturnsError(t *testing.T) {
	repo := &mockSessionRepo{
		createFn: func(_ context.Context, _ store.AuthSession) error {
			return errors.New("db error")
		},
	}

	_, err := newService(repo).Login(context.Background(), "user-1", nil)
	if err == nil {
		t.Fatal("expected error from repo, got nil")
	}
}

// --- Refresh ---

func TestRefresh_ValidToken_RotatesSession(t *testing.T) {
	// Получим валидный refresh-токен через Login.
	var storedSession store.AuthSession
	repo := &mockSessionRepo{
		createFn: func(_ context.Context, s store.AuthSession) error {
			storedSession = s
			return nil
		},
	}
	svc := auth.NewService(repo, testConfig(), testLogger())

	pair, err := svc.Login(context.Background(), "user-1", nil)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	// Теперь используем тот же repo для Refresh, но с findByTokenHash.
	var rotatedOldID string
	var rotatedNewSession store.AuthSession

	repo2 := &mockSessionRepo{
		findByTokenHashFn: func(_ context.Context, _ string) (store.AuthSession, error) {
			return storedSession, nil
		},
		rotateSessionFn: func(_ context.Context, oldID string, newSess store.AuthSession) error {
			rotatedOldID = oldID
			rotatedNewSession = newSess
			return nil
		},
	}

	newPair, err := auth.NewService(repo2, testConfig(), testLogger()).Refresh(context.Background(), pair.RefreshToken)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	if newPair.AccessToken == "" || newPair.RefreshToken == "" {
		t.Error("expected non-empty tokens after refresh")
	}
	if newPair.SessionID == pair.SessionID {
		t.Error("new SessionID must differ from old")
	}
	if rotatedOldID != storedSession.ID {
		t.Errorf("RotateSession called with wrong oldID: %q", rotatedOldID)
	}
	if rotatedNewSession.FamilyID != storedSession.FamilyID {
		t.Error("new session must keep the same FamilyID")
	}
	if rotatedNewSession.RotatedFrom == nil || *rotatedNewSession.RotatedFrom != storedSession.ID {
		t.Error("new session RotatedFrom must point to old session")
	}
}

func TestRefresh_InvalidJWT_ReturnsErrRevoked(t *testing.T) {
	_, err := newService(&mockSessionRepo{}).Refresh(context.Background(), "not-a-jwt")
	if !errors.Is(err, auth.ErrSessionRevoked) {
		t.Errorf("expected ErrSessionRevoked, got %v", err)
	}
}

func TestRefresh_SessionNotFound_ReturnsErrRevoked(t *testing.T) {
	// Выпускаем валидный токен, но репо не найдёт сессию.
	pair, err := auth.NewService(&mockSessionRepo{}, testConfig(), testLogger()).Login(
		context.Background(), "user-1", nil,
	)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	repo := &mockSessionRepo{
		findByTokenHashFn: func(_ context.Context, _ string) (store.AuthSession, error) {
			return store.AuthSession{}, store.ErrSessionNotFound
		},
	}

	_, err = auth.NewService(repo, testConfig(), testLogger()).Refresh(context.Background(), pair.RefreshToken)
	if !errors.Is(err, auth.ErrSessionRevoked) {
		t.Errorf("expected ErrSessionRevoked, got %v", err)
	}
}

func TestRefresh_RevokedSession_ReturnsErrCompromised_AndRevokesFamily(t *testing.T) {
	pair, err := auth.NewService(&mockSessionRepo{}, testConfig(), testLogger()).Login(
		context.Background(), "user-1", nil,
	)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	revokedAt := time.Now().UTC()
	revokedSession := store.AuthSession{
		ID:        "sess-old",
		UserID:    "user-1",
		FamilyID:  "family-1",
		RevokedAt: &revokedAt,
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}

	var revokedFamilyID string
	repo := &mockSessionRepo{
		findByTokenHashFn: func(_ context.Context, _ string) (store.AuthSession, error) {
			return revokedSession, nil
		},
		revokeFamilyFn: func(_ context.Context, familyID string) error {
			revokedFamilyID = familyID
			return nil
		},
	}

	_, err = auth.NewService(repo, testConfig(), testLogger()).Refresh(context.Background(), pair.RefreshToken)
	if !errors.Is(err, auth.ErrSessionCompromised) {
		t.Errorf("expected ErrSessionCompromised, got %v", err)
	}
	if revokedFamilyID != "family-1" {
		t.Errorf("expected RevokeFamily called with %q, got %q", "family-1", revokedFamilyID)
	}
}

func TestRefresh_ExpiredSession_ReturnsErrExpired(t *testing.T) {
	pair, err := auth.NewService(&mockSessionRepo{}, testConfig(), testLogger()).Login(
		context.Background(), "user-1", nil,
	)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	expiredSession := store.AuthSession{
		ID:        "sess-1",
		UserID:    "user-1",
		FamilyID:  "family-1",
		ExpiresAt: time.Now().UTC().Add(-time.Minute), // в прошлом
	}

	repo := &mockSessionRepo{
		findByTokenHashFn: func(_ context.Context, _ string) (store.AuthSession, error) {
			return expiredSession, nil
		},
	}

	_, err = auth.NewService(repo, testConfig(), testLogger()).Refresh(context.Background(), pair.RefreshToken)
	if !errors.Is(err, auth.ErrSessionExpired) {
		t.Errorf("expected ErrSessionExpired, got %v", err)
	}
}

// --- Logout ---

func TestLogout_RevokesSession(t *testing.T) {
	pair, err := auth.NewService(&mockSessionRepo{}, testConfig(), testLogger()).Login(
		context.Background(), "user-1", nil,
	)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	activeSession := store.AuthSession{
		ID: "sess-1", UserID: "user-1", FamilyID: "family-1",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
	var revokedID string
	repo := &mockSessionRepo{
		findByTokenHashFn: func(_ context.Context, _ string) (store.AuthSession, error) {
			return activeSession, nil
		},
		revokeSessionFn: func(_ context.Context, id string) error {
			revokedID = id
			return nil
		},
	}

	if err = auth.NewService(repo, testConfig(), testLogger()).Logout(context.Background(), pair.RefreshToken); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if revokedID != "sess-1" {
		t.Errorf("expected RevokeSession with %q, got %q", "sess-1", revokedID)
	}
}

func TestLogout_InvalidToken_ReturnsErrRevoked(t *testing.T) {
	err := newService(&mockSessionRepo{}).Logout(context.Background(), "bad-token")
	if !errors.Is(err, auth.ErrSessionRevoked) {
		t.Errorf("expected ErrSessionRevoked, got %v", err)
	}
}

func TestLogout_SessionNotFound_ReturnsNil(t *testing.T) {
	pair, err := auth.NewService(&mockSessionRepo{}, testConfig(), testLogger()).Login(
		context.Background(), "user-1", nil,
	)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	repo := &mockSessionRepo{
		findByTokenHashFn: func(_ context.Context, _ string) (store.AuthSession, error) {
			return store.AuthSession{}, store.ErrSessionNotFound
		},
	}

	if err = auth.NewService(repo, testConfig(), testLogger()).Logout(context.Background(), pair.RefreshToken); err != nil {
		t.Errorf("Logout for not-found session must be idempotent, got: %v", err)
	}
}

func TestLogout_AlreadyRevoked_ReturnsNil(t *testing.T) {
	pair, err := auth.NewService(&mockSessionRepo{}, testConfig(), testLogger()).Login(
		context.Background(), "user-1", nil,
	)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	revokedAt := time.Now().UTC()
	repo := &mockSessionRepo{
		findByTokenHashFn: func(_ context.Context, _ string) (store.AuthSession, error) {
			return store.AuthSession{
				ID: "sess-1", RevokedAt: &revokedAt,
				ExpiresAt: time.Now().UTC().Add(time.Hour),
			}, nil
		},
	}

	if err = auth.NewService(repo, testConfig(), testLogger()).Logout(context.Background(), pair.RefreshToken); err != nil {
		t.Errorf("Logout for already-revoked session must be idempotent, got: %v", err)
	}
}

// --- RevokeAll ---

func TestRevokeAll_CallsRepoWithUserID(t *testing.T) {
	var calledWith string
	repo := &mockSessionRepo{
		revokeAllFn: func(_ context.Context, userID string) error {
			calledWith = userID
			return nil
		},
	}

	if err := newService(repo).RevokeAll(context.Background(), "user-42"); err != nil {
		t.Fatalf("RevokeAll: %v", err)
	}
	if calledWith != "user-42" {
		t.Errorf("expected RevokeAllForUser called with %q, got %q", "user-42", calledWith)
	}
}
