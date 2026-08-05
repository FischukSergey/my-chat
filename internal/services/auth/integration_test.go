//go:build integration

// Package auth_test содержит интеграционные тесты сервиса аутентификации.
package auth_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	authsvc "my-chat/internal/services/auth"
	"my-chat/internal/store"
)

// setupAuthService подключается к БД, прогоняет миграции и возвращает готовый сервис.
func setupAuthService(t *testing.T) (*authsvc.Service, *store.Store) {
	t.Helper()

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

	sessionRepo := store.NewAuthSessionRepository(s)
	userRepo := store.NewUserRepository(s)
	cfg := authsvc.Config{
		JWTSecret:       "integration-test-secret",
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 7 * 24 * time.Hour,
	}
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	svc := authsvc.NewService(sessionRepo, userRepo, cfg, log)

	return svc, s
}

// insertTestUser вставляет пользователя с учётными данными и удаляет его в Cleanup.
// Возвращает username и plaintext-пароль.
func insertTestUser(t *testing.T, ctx context.Context, s *store.Store) (username, password string) {
	t.Helper()

	id := uuid.NewString()
	username = "testuser_" + id[:8]
	password = "testpass1"

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}

	_, err = s.DB().Exec(ctx,
		"INSERT INTO users (id, username, password_hash) VALUES ($1, $2, $3)",
		id, username, string(hash),
	)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	t.Cleanup(func() {
		_, _ = s.DB().Exec(ctx, "DELETE FROM users WHERE id = $1", id)
	})

	return username, password
}

// TestIntegration_Login_Refresh_OldRefreshInvalid проверяет сценарий:
// login → refresh → попытка повторно использовать старый refresh → ошибка.
func TestIntegration_Login_Refresh_OldRefreshInvalid(t *testing.T) {
	t.Parallel()

	svc, db := setupAuthService(t)
	ctx := context.Background()
	username, password := insertTestUser(t, ctx, db)

	// Login — получаем первую пару токенов.
	pair1, err := svc.Login(ctx, username, password, nil)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	// Refresh — ротируем. Старый refresh должен стать невалидным.
	pair2, err := svc.Refresh(ctx, pair1.RefreshToken, nil)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	if pair2.RefreshToken == pair1.RefreshToken {
		t.Error("Refresh must return a new token, not the same")
	}
	if pair2.SessionID == pair1.SessionID {
		t.Error("Refresh must return a new session ID")
	}

	// Попытка использовать старый refresh → reuse detection → session_compromised.
	_, err = svc.Refresh(ctx, pair1.RefreshToken, nil)
	if !errors.Is(err, authsvc.ErrSessionCompromised) {
		t.Errorf("expected ErrSessionCompromised, got %v", err)
	}

	// Новый refresh тоже должен быть отозван (вся family revoked).
	_, err = svc.Refresh(ctx, pair2.RefreshToken, nil)
	if !errors.Is(err, authsvc.ErrSessionRevoked) && !errors.Is(err, authsvc.ErrSessionCompromised) {
		t.Errorf("expected ErrSessionRevoked or ErrSessionCompromised for new token after family revoke, got %v", err)
	}
}

// TestIntegration_Logout_RefreshFails проверяет сценарий:
// login → logout → попытка refresh → ошибка.
func TestIntegration_Logout_RefreshFails(t *testing.T) {
	t.Parallel()

	svc, db := setupAuthService(t)
	ctx := context.Background()
	username, password := insertTestUser(t, ctx, db)

	pair, err := svc.Login(ctx, username, password, nil)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	// Logout — отзываем сессию.
	if err = svc.Logout(ctx, pair.RefreshToken); err != nil {
		t.Fatalf("Logout: %v", err)
	}

	// Refresh после logout должен вернуть ошибку.
	_, err = svc.Refresh(ctx, pair.RefreshToken, nil)
	if err == nil {
		t.Fatal("expected error after logout, got nil")
	}
	if !errors.Is(err, authsvc.ErrSessionRevoked) && !errors.Is(err, authsvc.ErrSessionCompromised) {
		t.Errorf("expected ErrSessionRevoked or ErrSessionCompromised, got %v", err)
	}

	// Повторный logout должен быть идемпотентным.
	if err = svc.Logout(ctx, pair.RefreshToken); err != nil {
		t.Errorf("second Logout must be idempotent, got: %v", err)
	}
}

// TestIntegration_ReuseDetection_FamilyRevoked проверяет reuse detection:
// login → refresh × 2 → reuse первого токена → вся family отзывается.
func TestIntegration_ReuseDetection_FamilyRevoked(t *testing.T) {
	t.Parallel()

	svc, db := setupAuthService(t)
	ctx := context.Background()
	username, password := insertTestUser(t, ctx, db)

	// Login.
	pair1, err := svc.Login(ctx, username, password, nil)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	// Первый refresh → пара 2.
	pair2, err := svc.Refresh(ctx, pair1.RefreshToken, nil)
	if err != nil {
		t.Fatalf("first Refresh: %v", err)
	}

	// Второй refresh → пара 3.
	pair3, err := svc.Refresh(ctx, pair2.RefreshToken, nil)
	if err != nil {
		t.Fatalf("second Refresh: %v", err)
	}

	// Reuse: повторно используем пару 1 (старый, уже отозванный токен).
	_, err = svc.Refresh(ctx, pair1.RefreshToken, nil)
	if !errors.Is(err, authsvc.ErrSessionCompromised) {
		t.Fatalf("expected ErrSessionCompromised on reuse, got %v", err)
	}

	// Все токены из той же family должны быть аннулированы.
	for name, token := range map[string]string{
		"pair2": pair2.RefreshToken,
		"pair3": pair3.RefreshToken,
	} {
		_, err = svc.Refresh(ctx, token, nil)
		if err == nil {
			t.Errorf("%s: expected error after family revoke, got nil", name)
			continue
		}
		if !errors.Is(err, authsvc.ErrSessionRevoked) && !errors.Is(err, authsvc.ErrSessionCompromised) {
			t.Errorf("%s: expected ErrSessionRevoked or ErrSessionCompromised, got %v", name, err)
		}
	}
}
