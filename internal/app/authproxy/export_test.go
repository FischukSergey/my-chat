package authproxy

import "net/http"

// RateLimitMaxAttempts экспортирует константу лимита для тестов.
const RateLimitMaxAttempts = rateLimitMaxAttempts

// NewTestLoginMiddleware создаёт http.Handler с rate-limiting для тестов.
// Каждый вызов создаёт отдельный, независимый limiter.
func NewTestLoginMiddleware(next http.Handler) http.Handler {
	return LoginRateLimitMiddleware(newIPRateLimiter())(next)
}
