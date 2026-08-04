// Package user_test contains unit tests for the user registration handler.
package user_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"my-chat/internal/handlers/user"
	usersvc "my-chat/internal/services/user"
	"my-chat/internal/store"
)

const (
	testUsername = "alice"
	testPassword = "secret99"
	keyUsername  = "username"
	keyPassword  = "password"
)

// --- mock ---

type mockUserSvc struct {
	registerFn func(ctx context.Context, username, password string) (store.User, error)
}

func (m *mockUserSvc) Register(ctx context.Context, username, password string) (store.User, error) {
	return m.registerFn(ctx, username, password)
}

// --- helpers ---

func jsonBody(t *testing.T, v any) *bytes.Reader {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}

	return bytes.NewReader(b)
}

func decodeError(t *testing.T, body *bytes.Buffer) string {
	t.Helper()
	var resp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}

	return resp.Error.Code
}

func decodeUserID(t *testing.T, body *bytes.Buffer) string {
	t.Helper()
	var resp struct {
		UserID string `json:"user_id"`
	}
	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		t.Fatalf("decode success response: %v", err)
	}

	return resp.UserID
}

// --- tests ---

func TestRegister_Success(t *testing.T) {
	t.Parallel()

	svc := &mockUserSvc{
		registerFn: func(_ context.Context, _, _ string) (store.User, error) {
			return store.User{ID: "uuid-1", Status: "active", Username: testUsername}, nil
		},
	}

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/users/register",
		jsonBody(t, map[string]string{keyUsername: testUsername, keyPassword: testPassword}))
	rec := httptest.NewRecorder()

	user.New(svc).Register(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if id := decodeUserID(t, rec.Body); id != "uuid-1" {
		t.Errorf("expected user_id=uuid-1, got %q", id)
	}
}

func TestRegister_DuplicateUsername_409(t *testing.T) {
	t.Parallel()

	svc := &mockUserSvc{
		registerFn: func(_ context.Context, _, _ string) (store.User, error) {
			return store.User{}, usersvc.ErrUsernameTaken
		},
	}

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/users/register",
		jsonBody(t, map[string]string{keyUsername: testUsername, keyPassword: testPassword}))
	rec := httptest.NewRecorder()

	user.New(svc).Register(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rec.Code)
	}
	if code := decodeError(t, rec.Body); code != "username_taken" {
		t.Errorf("expected code=username_taken, got %q", code)
	}
}

func TestRegister_PasswordTooShort_400(t *testing.T) {
	t.Parallel()

	svc := &mockUserSvc{
		registerFn: func(_ context.Context, _, _ string) (store.User, error) {
			return store.User{}, usersvc.ErrPasswordTooShort
		},
	}

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/users/register",
		jsonBody(t, map[string]string{keyUsername: testUsername, keyPassword: "short"}))
	rec := httptest.NewRecorder()

	user.New(svc).Register(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if code := decodeError(t, rec.Body); code != "invalid_argument" {
		t.Errorf("expected code=invalid_argument, got %q", code)
	}
}

func TestRegister_InvalidUsername_400(t *testing.T) {
	t.Parallel()

	svc := &mockUserSvc{
		registerFn: func(_ context.Context, _, _ string) (store.User, error) {
			return store.User{}, usersvc.ErrInvalidUsername
		},
	}

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/users/register",
		jsonBody(t, map[string]string{keyUsername: "a", keyPassword: testPassword}))
	rec := httptest.NewRecorder()

	user.New(svc).Register(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if code := decodeError(t, rec.Body); code != "invalid_argument" {
		t.Errorf("expected code=invalid_argument, got %q", code)
	}
}

func TestRegister_InvalidJSON_400(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/users/register",
		bytes.NewBufferString("{bad json"))
	rec := httptest.NewRecorder()

	user.New(&mockUserSvc{}).Register(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestRegister_ServiceError_500(t *testing.T) {
	t.Parallel()

	svc := &mockUserSvc{
		registerFn: func(_ context.Context, _, _ string) (store.User, error) {
			return store.User{}, errors.New("db error")
		},
	}

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/users/register",
		jsonBody(t, map[string]string{keyUsername: testUsername, keyPassword: testPassword}))
	rec := httptest.NewRecorder()

	user.New(svc).Register(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if code := decodeError(t, rec.Body); code != "internal" {
		t.Errorf("expected code=internal, got %q", code)
	}
}
