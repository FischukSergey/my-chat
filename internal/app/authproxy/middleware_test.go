// Package authproxy_test содержит unit-тесты для middleware rate-limiting.
package authproxy_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"my-chat/internal/app/authproxy"
)

// okHandler — заглушка, всегда возвращает 200.
var okHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
})

func loginRequest(ip string) *http.Request {
	r := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/auth/login", nil)
	r.RemoteAddr = ip
	return r
}

// TestRateLimit_AllowsUpToMaxAttempts проверяет, что первые RateLimitMaxAttempts
// запросов с одного IP проходят без ограничений.
func TestRateLimit_AllowsUpToMaxAttempts(t *testing.T) {
	t.Parallel()

	handler := authproxy.NewTestLoginMiddleware(okHandler)
	for i := range authproxy.RateLimitMaxAttempts {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, loginRequest("10.0.0.1:1234"))
		if w.Code != http.StatusOK {
			t.Errorf("request %d: want 200, got %d", i+1, w.Code)
		}
	}
}

// TestRateLimit_Returns429AfterExceedingLimit проверяет, что (RateLimitMaxAttempts+1)-й
// запрос с одного IP получает 429 с заголовком Retry-After.
func TestRateLimit_Returns429AfterExceedingLimit(t *testing.T) {
	t.Parallel()

	handler := authproxy.NewTestLoginMiddleware(okHandler)
	const ip = "10.0.0.2:5678"

	// Исчерпываем все токены.
	for range authproxy.RateLimitMaxAttempts {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, loginRequest(ip))
	}

	// Следующий запрос должен получить 429.
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, loginRequest(ip))

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("want 429 after exceeding limit, got %d", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Error("expected Retry-After header to be set")
	}

	var resp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Error.Code != "too_many_requests" {
		t.Errorf("error code: want too_many_requests, got %q", resp.Error.Code)
	}
}

// TestRateLimit_DifferentIPsAreIndependent проверяет, что лимит
// не переносится между разными IP-адресами.
func TestRateLimit_DifferentIPsAreIndependent(t *testing.T) {
	t.Parallel()

	handler := authproxy.NewTestLoginMiddleware(okHandler)
	const ip1 = "192.168.1.1:1"
	const ip2 = "192.168.1.2:2"

	// Исчерпываем лимит ip1.
	for range authproxy.RateLimitMaxAttempts {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, loginRequest(ip1))
	}

	// ip2 должен быть пропущен.
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, loginRequest(ip2))

	if w.Code != http.StatusOK {
		t.Errorf("different IP should not be rate-limited, got %d", w.Code)
	}
}
