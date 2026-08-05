// Package push_test contains unit tests for the push HTTP handler.
package push_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"my-chat/internal/handlers/push"
)

func TestVapidPublicKey_ReturnsKey(t *testing.T) {
	t.Parallel()

	const key = "BNcRdreALRFXTkOOUHK1EtK2wtWelNhztlfMLU4ZN_nNqfSC7lNnBL0R6NFQKQ"

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/push/vapid-public-key", nil)
	rec := httptest.NewRecorder()

	push.New(key).VapidPublicKey(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		PublicKey string `json:"public_key"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.PublicKey != key {
		t.Errorf("expected public_key=%q, got %q", key, resp.PublicKey)
	}

	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type=application/json, got %q", ct)
	}
}

func TestVapidPublicKey_EmptyKey(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/push/vapid-public-key", nil)
	rec := httptest.NewRecorder()

	push.New("").VapidPublicKey(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		PublicKey string `json:"public_key"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.PublicKey != "" {
		t.Errorf("expected empty public_key, got %q", resp.PublicKey)
	}
}
