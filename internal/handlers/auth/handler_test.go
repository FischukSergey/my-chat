// Package auth_test contains unit tests for the auth HTTP handler.
package auth_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	authhandler "my-chat/internal/handlers/auth"
	authsvc "my-chat/internal/services/auth"
)

// --- mock ---

type mockAuthSvc struct {
	loginFn     func(ctx context.Context, userID string, deviceID *string) (authsvc.TokenPair, error)
	refreshFn   func(ctx context.Context, refreshToken string) (authsvc.TokenPair, error)
	logoutFn    func(ctx context.Context, refreshToken string) error
	revokeAllFn func(ctx context.Context, userID string) error
}

func (m *mockAuthSvc) Login(ctx context.Context, userID string, deviceID *string) (authsvc.TokenPair, error) {
	if m.loginFn != nil {
		return m.loginFn(ctx, userID, deviceID)
	}

	return authsvc.TokenPair{}, nil
}

func (m *mockAuthSvc) Refresh(ctx context.Context, refreshToken string) (authsvc.TokenPair, error) {
	if m.refreshFn != nil {
		return m.refreshFn(ctx, refreshToken)
	}

	return authsvc.TokenPair{}, nil
}

func (m *mockAuthSvc) Logout(ctx context.Context, refreshToken string) error {
	if m.logoutFn != nil {
		return m.logoutFn(ctx, refreshToken)
	}

	return nil
}

func (m *mockAuthSvc) RevokeAll(ctx context.Context, userID string) error {
	if m.revokeAllFn != nil {
		return m.revokeAllFn(ctx, userID)
	}

	return nil
}

// --- helpers ---

func jsonBody(v any) *bytes.Reader {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}

	return bytes.NewReader(b)
}

func decodeTokenResponse(t *testing.T, body *bytes.Buffer) map[string]any {
	t.Helper()
	var resp map[string]any
	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		t.Fatalf("decode token response: %v", err)
	}
	return resp
}

func decodeErrorCode(t *testing.T, body *bytes.Buffer) string {
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

// --- Login ---

func TestLogin_Success(t *testing.T) {
	t.Parallel()

	svc := &mockAuthSvc{
		loginFn: func(_ context.Context, _ string, _ *string) (authsvc.TokenPair, error) {
			return authsvc.TokenPair{
				AccessToken:  "acc",
				RefreshToken: "ref",
				SessionID:    "sess-1",
				ExpiresIn:    900,
			}, nil
		},
	}

	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
		jsonBody(map[string]string{"user_id": "user-1"}))
	w := httptest.NewRecorder()

	authhandler.New(svc).Login(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status: want 200, got %d", w.Code)
	}

	resp := decodeTokenResponse(t, w.Body)
	if resp["access_token"] != "acc" {
		t.Errorf("access_token mismatch: %v", resp["access_token"])
	}
	if resp["session_id"] != "sess-1" {
		t.Errorf("session_id mismatch: %v", resp["session_id"])
	}
	if resp["token_type"] != "Bearer" {
		t.Errorf("token_type mismatch: %v", resp["token_type"])
	}
}

func TestLogin_MissingUserID_Returns400(t *testing.T) {
	t.Parallel()

	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
		jsonBody(map[string]string{}))
	w := httptest.NewRecorder()

	authhandler.New(&mockAuthSvc{}).Login(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status: want 400, got %d", w.Code)
	}
	if code := decodeErrorCode(t, w.Body); code != "invalid_argument" {
		t.Errorf("error code: want invalid_argument, got %q", code)
	}
}

func TestLogin_ServiceError_Returns500(t *testing.T) {
	t.Parallel()

	svc := &mockAuthSvc{
		loginFn: func(_ context.Context, _ string, _ *string) (authsvc.TokenPair, error) {
			return authsvc.TokenPair{}, errors.New("db error")
		},
	}

	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
		jsonBody(map[string]string{"user_id": "user-1"}))
	w := httptest.NewRecorder()

	authhandler.New(svc).Login(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status: want 500, got %d", w.Code)
	}
}

func TestLogin_InactiveUser_Returns403WithUserInactiveCode(t *testing.T) {
	t.Parallel()

	svc := &mockAuthSvc{
		loginFn: func(_ context.Context, _ string, _ *string) (authsvc.TokenPair, error) {
			return authsvc.TokenPair{}, authsvc.ErrUserInactive
		},
	}

	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
		jsonBody(map[string]string{"user_id": "blocked-user"}))
	w := httptest.NewRecorder()

	authhandler.New(svc).Login(w, r)

	if w.Code != http.StatusForbidden {
		t.Errorf("status: want 403, got %d", w.Code)
	}
	if code := decodeErrorCode(t, w.Body); code != "user_inactive" {
		t.Errorf("error code: want user_inactive, got %q", code)
	}
}

// --- Refresh ---

func TestRefresh_Success(t *testing.T) {
	t.Parallel()

	svc := &mockAuthSvc{
		refreshFn: func(_ context.Context, _ string) (authsvc.TokenPair, error) {
			return authsvc.TokenPair{AccessToken: "new-acc", RefreshToken: "new-ref", SessionID: "sess-2"}, nil
		},
	}

	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh",
		jsonBody(map[string]string{"refresh_token": "old-ref"}))
	w := httptest.NewRecorder()

	authhandler.New(svc).Refresh(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status: want 200, got %d", w.Code)
	}
}

func TestRefresh_MissingToken_Returns400(t *testing.T) {
	t.Parallel()

	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh",
		jsonBody(map[string]string{}))
	w := httptest.NewRecorder()

	authhandler.New(&mockAuthSvc{}).Refresh(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status: want 400, got %d", w.Code)
	}
}

func TestRefresh_SessionRevoked_Returns401WithCorrectCode(t *testing.T) {
	t.Parallel()

	svc := &mockAuthSvc{
		refreshFn: func(_ context.Context, _ string) (authsvc.TokenPair, error) {
			return authsvc.TokenPair{}, authsvc.ErrSessionRevoked
		},
	}

	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh",
		jsonBody(map[string]string{"refresh_token": "tok"}))
	w := httptest.NewRecorder()

	authhandler.New(svc).Refresh(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status: want 401, got %d", w.Code)
	}
	if code := decodeErrorCode(t, w.Body); code != "session_revoked" {
		t.Errorf("error code: want session_revoked, got %q", code)
	}
}

func TestRefresh_SessionExpired_Returns401WithCorrectCode(t *testing.T) {
	t.Parallel()

	svc := &mockAuthSvc{
		refreshFn: func(_ context.Context, _ string) (authsvc.TokenPair, error) {
			return authsvc.TokenPair{}, authsvc.ErrSessionExpired
		},
	}

	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh",
		jsonBody(map[string]string{"refresh_token": "tok"}))
	w := httptest.NewRecorder()

	authhandler.New(svc).Refresh(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status: want 401, got %d", w.Code)
	}
	if code := decodeErrorCode(t, w.Body); code != "session_expired" {
		t.Errorf("error code: want session_expired, got %q", code)
	}
}

func TestRefresh_SessionCompromised_Returns401WithCorrectCode(t *testing.T) {
	t.Parallel()

	svc := &mockAuthSvc{
		refreshFn: func(_ context.Context, _ string) (authsvc.TokenPair, error) {
			return authsvc.TokenPair{}, authsvc.ErrSessionCompromised
		},
	}

	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh",
		jsonBody(map[string]string{"refresh_token": "tok"}))
	w := httptest.NewRecorder()

	authhandler.New(svc).Refresh(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status: want 401, got %d", w.Code)
	}
	if code := decodeErrorCode(t, w.Body); code != "session_compromised" {
		t.Errorf("error code: want session_compromised, got %q", code)
	}
}

// --- Logout ---

func TestLogout_Success(t *testing.T) {
	t.Parallel()

	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout",
		jsonBody(map[string]string{"refresh_token": "tok"}))
	w := httptest.NewRecorder()

	authhandler.New(&mockAuthSvc{}).Logout(w, r)

	if w.Code != http.StatusNoContent {
		t.Errorf("status: want 204, got %d", w.Code)
	}
}

func TestLogout_MissingToken_Returns400(t *testing.T) {
	t.Parallel()

	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout",
		jsonBody(map[string]string{}))
	w := httptest.NewRecorder()

	authhandler.New(&mockAuthSvc{}).Logout(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status: want 400, got %d", w.Code)
	}
}

func TestLogout_SessionRevoked_Returns401(t *testing.T) {
	t.Parallel()

	svc := &mockAuthSvc{
		logoutFn: func(_ context.Context, _ string) error {
			return authsvc.ErrSessionRevoked
		},
	}

	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout",
		jsonBody(map[string]string{"refresh_token": "tok"}))
	w := httptest.NewRecorder()

	authhandler.New(svc).Logout(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status: want 401, got %d", w.Code)
	}
	if code := decodeErrorCode(t, w.Body); code != "session_revoked" {
		t.Errorf("error code: want session_revoked, got %q", code)
	}
}
