package user_test

import (
	"context"
	"errors"
	"testing"

	"my-chat/internal/services/user"
	"my-chat/internal/store"
)

const testUsernameAlice42 = "alice42"

// --- mock ---

type mockUserRepo struct {
	createFn func(ctx context.Context, u store.User) (store.User, error)
}

func (m *mockUserRepo) Create(ctx context.Context, u store.User) (store.User, error) {
	if m.createFn != nil {
		return m.createFn(ctx, u)
	}
	return u, nil
}

func newService(repo *mockUserRepo) *user.Service {
	return user.NewService(repo)
}

// --- Register tests ---

func TestRegister_Success(t *testing.T) {
	t.Parallel()

	var created store.User
	svc := newService(&mockUserRepo{
		createFn: func(_ context.Context, u store.User) (store.User, error) {
			created = u
			return u, nil
		},
	})

	result, err := svc.Register(context.Background(), testUsernameAlice42, "secret99")
	if err != nil {
		t.Fatalf("Register: unexpected error: %v", err)
	}

	if result.ID == "" {
		t.Error("expected non-empty user ID")
	}
	if result.Username != testUsernameAlice42 {
		t.Errorf("username: want %s, got %q", testUsernameAlice42, result.Username)
	}
	if result.Status != "active" {
		t.Errorf("status: want active, got %q", result.Status)
	}
	if created.PasswordHash == "" {
		t.Error("expected non-empty password hash")
	}
	if created.PasswordHash == "secret99" {
		t.Error("password must be hashed, not stored as plaintext")
	}
}

func TestRegister_DuplicateUsername_ReturnsErrUsernameTaken(t *testing.T) {
	t.Parallel()

	svc := newService(&mockUserRepo{
		createFn: func(_ context.Context, _ store.User) (store.User, error) {
			return store.User{}, store.ErrUsernameTaken
		},
	})

	_, err := svc.Register(context.Background(), testUsernameAlice42, "password123")
	if err == nil {
		t.Fatal("expected error for duplicate username, got nil")
	}
	if !errors.Is(err, user.ErrUsernameTaken) {
		t.Errorf("expected ErrUsernameTaken, got: %v", err)
	}
}

func TestRegister_ShortPassword_ReturnsErrPasswordTooShort(t *testing.T) {
	t.Parallel()

	svc := newService(&mockUserRepo{})

	_, err := svc.Register(context.Background(), testUsernameAlice42, "short")
	if err == nil {
		t.Fatal("expected error for short password, got nil")
	}
	if !errors.Is(err, user.ErrPasswordTooShort) {
		t.Errorf("expected ErrPasswordTooShort, got: %v", err)
	}
}

func TestRegister_InvalidUsername_ReturnsErrInvalidUsername(t *testing.T) {
	t.Parallel()

	cases := []string{
		"",          // пустой
		"ab",        // слишком короткий (< 3)
		"alice bob", // пробел
		"Алиса",     // кириллица
	}

	svc := newService(&mockUserRepo{})

	for _, username := range cases {
		t.Run(username, func(t *testing.T) {
			t.Parallel()

			_, err := svc.Register(context.Background(), username, "password123")
			if !errors.Is(err, user.ErrInvalidUsername) {
				t.Errorf("username %q: expected ErrInvalidUsername, got: %v", username, err)
			}
		})
	}
}

func TestRegister_NormalizesUsernameToLowercase(t *testing.T) {
	t.Parallel()

	var created store.User
	svc := newService(&mockUserRepo{
		createFn: func(_ context.Context, u store.User) (store.User, error) {
			created = u
			return u, nil
		},
	})

	result, err := svc.Register(context.Background(), "  Alice42  ", "secret99")
	if err != nil {
		t.Fatalf("Register: unexpected error: %v", err)
	}
	if result.Username != testUsernameAlice42 {
		t.Errorf("username: want %s, got %q", testUsernameAlice42, result.Username)
	}
	if created.Username != testUsernameAlice42 {
		t.Errorf("stored username: want %s, got %q", testUsernameAlice42, created.Username)
	}
}
