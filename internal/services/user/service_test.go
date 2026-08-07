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
	searchFn func(ctx context.Context, prefix, excludeUserID string, limit int) ([]store.User, error)
}

func (m *mockUserRepo) Create(ctx context.Context, u store.User) (store.User, error) {
	if m.createFn != nil {
		return m.createFn(ctx, u)
	}
	return u, nil
}

func (m *mockUserRepo) SearchByUsernamePrefix(
	ctx context.Context,
	prefix, excludeUserID string,
	limit int,
) ([]store.User, error) {
	if m.searchFn != nil {
		return m.searchFn(ctx, prefix, excludeUserID, limit)
	}
	return nil, nil
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

func TestSearch_Success(t *testing.T) {
	t.Parallel()

	callerID := "caller-1"
	svc := newService(&mockUserRepo{
		searchFn: func(_ context.Context, prefix, excludeUserID string, limit int) ([]store.User, error) {
			if prefix != "bo" || excludeUserID != callerID || limit != 20 {
				t.Errorf("args: prefix=%q exclude=%q limit=%d", prefix, excludeUserID, limit)
			}
			return []store.User{
				{ID: "u1", Username: "bob"},
				{ID: "u2", Username: "bobby"},
			}, nil
		},
	})

	hits, err := svc.Search(context.Background(), callerID, "  Bo ", 0)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 2 || hits[0].Username != "bob" || hits[1].UserID != "u2" {
		t.Errorf("unexpected hits: %+v", hits)
	}
}

func TestSearch_ShortQuery(t *testing.T) {
	t.Parallel()

	svc := newService(&mockUserRepo{})
	_, err := svc.Search(context.Background(), "caller", "a", 10)
	if !errors.Is(err, user.ErrInvalidSearchQuery) {
		t.Errorf("want ErrInvalidSearchQuery, got %v", err)
	}
}

func TestSearch_ClampsLimit(t *testing.T) {
	t.Parallel()

	var gotLimit int
	svc := newService(&mockUserRepo{
		searchFn: func(_ context.Context, _, _ string, limit int) ([]store.User, error) {
			gotLimit = limit
			return nil, nil
		},
	})

	if _, err := svc.Search(context.Background(), "caller", "bo", 100); err != nil {
		t.Fatalf("Search: %v", err)
	}
	if gotLimit != 50 {
		t.Errorf("limit: want 50, got %d", gotLimit)
	}
}
