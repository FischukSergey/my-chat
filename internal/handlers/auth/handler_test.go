// Package auth_test contains unit tests for the auth handler.
package auth_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"my-chat/internal/handlers/auth"
	"my-chat/internal/jwt"
)

const (
	testSecret        = "test-jwt-secret"
	testAccessTTLSec  = 900
	testRefreshTTLSec = 604800
)

func newHandler() *auth.Handler {
	return auth.New(auth.Config{
		JWTSecret:          testSecret,
		AccessTokenTTLSec:  testAccessTTLSec,
		RefreshTokenTTLSec: testRefreshTTLSec,
	})
}

func jsonBody(t *testing.T, v any) *bytes.Reader {
	t.Helper()

	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}

	return bytes.NewReader(b)
}

func TestLogin_Success(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/auth/login",
		jsonBody(t, map[string]string{"user_id": "user-123"}),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	newHandler().Login(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if _, ok := resp["access_token"]; !ok {
		t.Error("missing access_token in response")
	}
	if _, ok := resp["refresh_token"]; !ok {
		t.Error("missing refresh_token in response")
	}
}

func TestLogin_EmptyUserID(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/auth/login",
		jsonBody(t, map[string]string{"user_id": ""}),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	newHandler().Login(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestRefresh_Success(t *testing.T) {
	t.Parallel()

	refreshToken, err := jwt.IssueRefresh("user-123", testSecret, time.Duration(testRefreshTTLSec)*time.Second)
	if err != nil {
		t.Fatalf("issue refresh token: %v", err)
	}

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/auth/refresh",
		jsonBody(t, map[string]string{"refresh_token": refreshToken}),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	newHandler().Refresh(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRefresh_InvalidToken(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/auth/refresh",
		jsonBody(t, map[string]string{"refresh_token": "not-a-valid-token"}),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	newHandler().Refresh(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestLogout_Success(t *testing.T) {
	t.Parallel()

	refreshToken, err := jwt.IssueRefresh("user-123", testSecret, time.Duration(testRefreshTTLSec)*time.Second)
	if err != nil {
		t.Fatalf("issue refresh token: %v", err)
	}

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/auth/logout",
		jsonBody(t, map[string]string{"refresh_token": refreshToken}),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	newHandler().Logout(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rec.Code)
	}
}
