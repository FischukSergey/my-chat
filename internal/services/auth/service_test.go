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

// --- mocks ---

const (
	testUser1ID  = "user-1"
	testFamily1  = "family-1"
	testSession1 = "sess-1"
)

type mockUserRepo struct {
	findByIDFn func(ctx context.Context, userID string) (store.User, error)
}

func (m *mockUserRepo) FindByID(ctx context.Context, userID string) (store.User, error) {
	if m.findByIDFn != nil {
		return m.findByIDFn(ctx, userID)
	}
	// По умолчанию возвращаем активного пользователя.
	return store.User{ID: userID, Status: "active"}, nil
}

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
	return auth.NewService(repo, &mockUserRepo{}, testConfig(), testLogger())
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

	pair, err := newService(repo).Login(context.Background(), testUser1ID, nil)
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
	if created.UserID != testUser1ID {
		t.Errorf("created session UserID: want %q, got %q", testUser1ID, created.UserID)
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

	_, err := newService(repo).Login(context.Background(), testUser1ID, nil)
	if err == nil {
		t.Fatal("expected error from repo, got nil")
	}
}

func TestLogin_InactiveUser_ReturnsErrUserInactive(t *testing.T) {
	userRepo := &mockUserRepo{
		findByIDFn: func(_ context.Context, userID string) (store.User, error) {
			return store.User{ID: userID, Status: "blocked"}, nil
		},
	}
	svc := auth.NewService(&mockSessionRepo{}, userRepo, testConfig(), testLogger())

	_, err := svc.Login(context.Background(), "blocked-user", nil)
	if err == nil {
		t.Fatal("expected ErrUserInactive, got nil")
	}
	if !errors.Is(err, auth.ErrUserInactive) {
		t.Errorf("expected ErrUserInactive, got %v", err)
	}
}

func TestLogin_UserNotFound_ReturnsError(t *testing.T) {
	userRepo := &mockUserRepo{
		findByIDFn: func(_ context.Context, _ string) (store.User, error) {
			return store.User{}, store.ErrUserNotFound
		},
	}
	svc := auth.NewService(&mockSessionRepo{}, userRepo, testConfig(), testLogger())

	_, err := svc.Login(context.Background(), "unknown-user", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
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
	svc := auth.NewService(repo, &mockUserRepo{}, testConfig(), testLogger())

	pair, err := svc.Login(context.Background(), testUser1ID, nil)
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

	svc2 := auth.NewService(repo2, &mockUserRepo{}, testConfig(), testLogger())
	newPair, err := svc2.Refresh(context.Background(), pair.RefreshToken)
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
	pair, err := auth.NewService(&mockSessionRepo{}, &mockUserRepo{}, testConfig(), testLogger()).Login(
		context.Background(), testUser1ID, nil,
	)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	repo := &mockSessionRepo{
		findByTokenHashFn: func(_ context.Context, _ string) (store.AuthSession, error) {
			return store.AuthSession{}, store.ErrSessionNotFound
		},
	}

	_, err = auth.NewService(repo, &mockUserRepo{}, testConfig(), testLogger()).Refresh(context.Background(), pair.RefreshToken)
	if !errors.Is(err, auth.ErrSessionRevoked) {
		t.Errorf("expected ErrSessionRevoked, got %v", err)
	}
}

func TestRefresh_RevokedSession_ReturnsErrCompromised_AndRevokesFamily(t *testing.T) {
	pair, err := auth.NewService(&mockSessionRepo{}, &mockUserRepo{}, testConfig(), testLogger()).Login(
		context.Background(), testUser1ID, nil,
	)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	revokedAt := time.Now().UTC()
	revokedSession := store.AuthSession{
		ID:        "sess-old",
		UserID:    testUser1ID,
		FamilyID:  testFamily1,
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

	_, err = auth.NewService(repo, &mockUserRepo{}, testConfig(), testLogger()).Refresh(context.Background(), pair.RefreshToken)
	if !errors.Is(err, auth.ErrSessionCompromised) {
		t.Errorf("expected ErrSessionCompromised, got %v", err)
	}
	if revokedFamilyID != testFamily1 {
		t.Errorf("expected RevokeFamily called with %q, got %q", testFamily1, revokedFamilyID)
	}
}

func TestRefresh_ExpiredSession_ReturnsErrExpired(t *testing.T) {
	pair, err := auth.NewService(&mockSessionRepo{}, &mockUserRepo{}, testConfig(), testLogger()).Login(
		context.Background(), testUser1ID, nil,
	)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	expiredSession := store.AuthSession{
		ID:        testSession1,
		UserID:    testUser1ID,
		FamilyID:  testFamily1,
		ExpiresAt: time.Now().UTC().Add(-time.Minute), // в прошлом
	}

	repo := &mockSessionRepo{
		findByTokenHashFn: func(_ context.Context, _ string) (store.AuthSession, error) {
			return expiredSession, nil
		},
	}

	_, err = auth.NewService(repo, &mockUserRepo{}, testConfig(), testLogger()).Refresh(context.Background(), pair.RefreshToken)
	if !errors.Is(err, auth.ErrSessionExpired) {
		t.Errorf("expected ErrSessionExpired, got %v", err)
	}
}

// --- Logout ---

func TestLogout_RevokesSession(t *testing.T) {
	pair, err := auth.NewService(&mockSessionRepo{}, &mockUserRepo{}, testConfig(), testLogger()).Login(
		context.Background(), testUser1ID, nil,
	)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	activeSession := store.AuthSession{
		ID: testSession1, UserID: testUser1ID, FamilyID: testFamily1,
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

	svcLogout := auth.NewService(repo, &mockUserRepo{}, testConfig(), testLogger())
	if err = svcLogout.Logout(context.Background(), pair.RefreshToken); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if revokedID != testSession1 {
		t.Errorf("expected RevokeSession with %q, got %q", testSession1, revokedID)
	}
}

func TestLogout_InvalidToken_ReturnsErrRevoked(t *testing.T) {
	err := newService(&mockSessionRepo{}).Logout(context.Background(), "bad-token")
	if !errors.Is(err, auth.ErrSessionRevoked) {
		t.Errorf("expected ErrSessionRevoked, got %v", err)
	}
}

func TestLogout_SessionNotFound_ReturnsNil(t *testing.T) {
	pair, err := auth.NewService(&mockSessionRepo{}, &mockUserRepo{}, testConfig(), testLogger()).Login(
		context.Background(), testUser1ID, nil,
	)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	repo := &mockSessionRepo{
		findByTokenHashFn: func(_ context.Context, _ string) (store.AuthSession, error) {
			return store.AuthSession{}, store.ErrSessionNotFound
		},
	}

	svcIdempotent := auth.NewService(repo, &mockUserRepo{}, testConfig(), testLogger())
	if err = svcIdempotent.Logout(context.Background(), pair.RefreshToken); err != nil {
		t.Errorf("Logout for not-found session must be idempotent, got: %v", err)
	}
}

func TestLogout_AlreadyRevoked_ReturnsNil(t *testing.T) {
	pair, err := auth.NewService(&mockSessionRepo{}, &mockUserRepo{}, testConfig(), testLogger()).Login(
		context.Background(), testUser1ID, nil,
	)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	revokedAt := time.Now().UTC()
	repo := &mockSessionRepo{
		findByTokenHashFn: func(_ context.Context, _ string) (store.AuthSession, error) {
			return store.AuthSession{
				ID: testSession1, RevokedAt: &revokedAt,
				ExpiresAt: time.Now().UTC().Add(time.Hour),
			}, nil
		},
	}

	svcRevoked := auth.NewService(repo, &mockUserRepo{}, testConfig(), testLogger())
	if err = svcRevoked.Logout(context.Background(), pair.RefreshToken); err != nil {
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
