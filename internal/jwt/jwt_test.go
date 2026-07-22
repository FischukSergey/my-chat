package jwt_test

import (
	"testing"
	"time"

	internaljwt "my-chat/internal/jwt"
)

const (
	testSecret  = "test-secret-key"
	testUserID  = "11111111-1111-1111-1111-111111111111"
	testSession = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
)

// --- IssueAccess / ParseAccess (обратная совместимость) ---

func TestIssueAccess_ParseAccess_RoundTrip(t *testing.T) {
	token, err := internaljwt.IssueAccess(testUserID, testSecret, time.Hour)
	if err != nil {
		t.Fatalf("IssueAccess: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	gotUserID, err := internaljwt.ParseAccess(token, testSecret)
	if err != nil {
		t.Fatalf("ParseAccess: %v", err)
	}
	if gotUserID != testUserID {
		t.Errorf("UserID mismatch: want %q, got %q", testUserID, gotUserID)
	}
}

func TestIssueAccess_WrongSecret_ReturnsInvalidToken(t *testing.T) {
	token, err := internaljwt.IssueAccess(testUserID, testSecret, time.Hour)
	if err != nil {
		t.Fatalf("IssueAccess: %v", err)
	}

	_, err = internaljwt.ParseAccess(token, "wrong-secret")
	if err != internaljwt.ErrInvalidToken {
		t.Errorf("expected ErrInvalidToken, got %v", err)
	}
}

func TestIssueAccess_Expired_ReturnsInvalidToken(t *testing.T) {
	token, err := internaljwt.IssueAccess(testUserID, testSecret, -time.Second)
	if err != nil {
		t.Fatalf("IssueAccess: %v", err)
	}

	_, err = internaljwt.ParseAccess(token, testSecret)
	if err != internaljwt.ErrInvalidToken {
		t.Errorf("expected ErrInvalidToken for expired token, got %v", err)
	}
}

// --- IssueRefresh / ParseRefresh (обратная совместимость) ---

func TestIssueRefresh_ParseRefresh_RoundTrip(t *testing.T) {
	token, err := internaljwt.IssueRefresh(testUserID, testSecret, time.Hour)
	if err != nil {
		t.Fatalf("IssueRefresh: %v", err)
	}

	gotUserID, err := internaljwt.ParseRefresh(token, testSecret)
	if err != nil {
		t.Fatalf("ParseRefresh: %v", err)
	}
	if gotUserID != testUserID {
		t.Errorf("UserID mismatch: want %q, got %q", testUserID, gotUserID)
	}
}

func TestParseRefresh_AccessToken_ReturnsWrongTokenType(t *testing.T) {
	accessToken, err := internaljwt.IssueAccess(testUserID, testSecret, time.Hour)
	if err != nil {
		t.Fatalf("IssueAccess: %v", err)
	}

	_, err = internaljwt.ParseRefresh(accessToken, testSecret)
	if err != internaljwt.ErrWrongTokenType {
		t.Errorf("expected ErrWrongTokenType, got %v", err)
	}
}

// --- IssueAccessWithSession ---

func TestIssueAccessWithSession_ClaimsContainSessionID(t *testing.T) {
	token, err := internaljwt.IssueAccessWithSession(testUserID, testSession, testSecret, time.Hour)
	if err != nil {
		t.Fatalf("IssueAccessWithSession: %v", err)
	}

	// ParseAccess должен по-прежнему работать и вернуть userID.
	gotUserID, err := internaljwt.ParseAccess(token, testSecret)
	if err != nil {
		t.Fatalf("ParseAccess: %v", err)
	}
	if gotUserID != testUserID {
		t.Errorf("UserID mismatch: want %q, got %q", testUserID, gotUserID)
	}
}

func TestIssueAccessWithSession_WrongType_ParseRefreshFails(t *testing.T) {
	token, err := internaljwt.IssueAccessWithSession(testUserID, testSession, testSecret, time.Hour)
	if err != nil {
		t.Fatalf("IssueAccessWithSession: %v", err)
	}

	_, err = internaljwt.ParseRefreshClaims(token, testSecret)
	if err != internaljwt.ErrWrongTokenType {
		t.Errorf("expected ErrWrongTokenType parsing access as refresh, got %v", err)
	}
}

// --- IssueRefreshWithSession / ParseRefreshClaims ---

func TestIssueRefreshWithSession_ParseRefreshClaims_RoundTrip(t *testing.T) {
	token, err := internaljwt.IssueRefreshWithSession(testUserID, testSession, testSecret, time.Hour)
	if err != nil {
		t.Fatalf("IssueRefreshWithSession: %v", err)
	}

	claims, err := internaljwt.ParseRefreshClaims(token, testSecret)
	if err != nil {
		t.Fatalf("ParseRefreshClaims: %v", err)
	}

	if claims.UserID != testUserID {
		t.Errorf("UserID mismatch: want %q, got %q", testUserID, claims.UserID)
	}
	if claims.SessionID != testSession {
		t.Errorf("SessionID mismatch: want %q, got %q", testSession, claims.SessionID)
	}
}

func TestParseRefreshClaims_Expired_ReturnsInvalidToken(t *testing.T) {
	token, err := internaljwt.IssueRefreshWithSession(testUserID, testSession, testSecret, -time.Second)
	if err != nil {
		t.Fatalf("IssueRefreshWithSession: %v", err)
	}

	_, err = internaljwt.ParseRefreshClaims(token, testSecret)
	if err != internaljwt.ErrInvalidToken {
		t.Errorf("expected ErrInvalidToken for expired token, got %v", err)
	}
}

func TestParseRefreshClaims_WrongSecret_ReturnsInvalidToken(t *testing.T) {
	token, err := internaljwt.IssueRefreshWithSession(testUserID, testSession, testSecret, time.Hour)
	if err != nil {
		t.Fatalf("IssueRefreshWithSession: %v", err)
	}

	_, err = internaljwt.ParseRefreshClaims(token, "wrong-secret")
	if err != internaljwt.ErrInvalidToken {
		t.Errorf("expected ErrInvalidToken for wrong secret, got %v", err)
	}
}

// --- Обратная совместимость: старый токен без session_id ---

func TestParseRefreshClaims_OldToken_EmptySessionID(t *testing.T) {
	// Токен, выпущенный старой функцией без session_id.
	oldToken, err := internaljwt.IssueRefresh(testUserID, testSecret, time.Hour)
	if err != nil {
		t.Fatalf("IssueRefresh: %v", err)
	}

	claims, err := internaljwt.ParseRefreshClaims(oldToken, testSecret)
	if err != nil {
		t.Fatalf("ParseRefreshClaims on old token: %v", err)
	}
	if claims.UserID != testUserID {
		t.Errorf("UserID mismatch: want %q, got %q", testUserID, claims.UserID)
	}
	if claims.SessionID != "" {
		t.Errorf("expected empty SessionID for legacy token, got %q", claims.SessionID)
	}
}
