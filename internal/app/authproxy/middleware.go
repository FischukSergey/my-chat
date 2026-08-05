package authproxy

import (
	"encoding/json"
	"math"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// --- Rate limiting (token bucket, stdlib only) ---

const (
	rateLimitMaxAttempts = 10              // максимум попыток за окно
	rateLimitWindowSec   = 60              // размер окна в секундах
	rateLimitCleanupTTL  = 5 * time.Minute // TTL неактивных IP-записей
)

// tokenBucket — простой токен-бакет на основе стандартной библиотеки.
// Потокобезопасен.
type tokenBucket struct {
	mu       sync.Mutex
	tokens   float64
	maxBurst float64
	rate     float64 // токенов в секунду
	lastTime time.Time
}

func newTokenBucket(ratePerSec, burst float64) *tokenBucket {
	return &tokenBucket{
		tokens:   burst,
		maxBurst: burst,
		rate:     ratePerSec,
		lastTime: time.Now(),
	}
}

// allow пытается изъять один токен.
// Возвращает (true, 0) если запрос разрешён, либо (false, retryAfter) если нет.
func (b *tokenBucket) allow() (bool, time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(b.lastTime).Seconds()
	b.lastTime = now

	b.tokens = math.Min(b.maxBurst, b.tokens+elapsed*b.rate)

	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}

	// Сколько секунд ждать до появления следующего токена.
	waitSec := math.Ceil((1 - b.tokens) / b.rate)
	return false, time.Duration(waitSec) * time.Second
}

type ipEntry struct {
	bucket   *tokenBucket
	lastSeen time.Time
}

// ipRateLimiter хранит per-IP бакеты и периодически очищает устаревшие.
type ipRateLimiter struct {
	mu      sync.Mutex
	entries map[string]*ipEntry
}

func newIPRateLimiter() *ipRateLimiter {
	rl := &ipRateLimiter{
		entries: make(map[string]*ipEntry),
	}
	go rl.cleanupLoop()
	return rl
}

func (rl *ipRateLimiter) get(ip string) *tokenBucket {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	e, ok := rl.entries[ip]
	if !ok {
		e = &ipEntry{
			bucket: newTokenBucket(
				float64(rateLimitMaxAttempts)/float64(rateLimitWindowSec),
				float64(rateLimitMaxAttempts),
			),
		}
		rl.entries[ip] = e
	}
	e.lastSeen = time.Now()
	return e.bucket
}

func (rl *ipRateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rateLimitCleanupTTL)
	defer ticker.Stop()
	for range ticker.C {
		rl.cleanup()
	}
}

func (rl *ipRateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	threshold := time.Now().Add(-rateLimitCleanupTTL)
	for ip, e := range rl.entries {
		if e.lastSeen.Before(threshold) {
			delete(rl.entries, ip)
		}
	}
}

// LoginRateLimitMiddleware ограничивает число запросов к /login по IP-адресу клиента.
// При превышении лимита возвращает 429 Too Many Requests с заголовком Retry-After.
func LoginRateLimitMiddleware(rl *ipRateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := clientIP(r)
			bucket := rl.get(ip)

			ok, retryAfter := bucket.allow()
			if !ok {
				writeRateLimitError(w, int(math.Ceil(retryAfter.Seconds())))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func clientIP(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

func writeRateLimitError(w http.ResponseWriter, retryAfterSec int) {
	type errBody struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	type resp struct {
		Error errBody `json:"error"`
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Retry-After", strconv.Itoa(retryAfterSec))
	w.WriteHeader(http.StatusTooManyRequests)
	_ = json.NewEncoder(w).Encode(resp{Error: errBody{ //nolint:errchkjson // payload is a concrete struct, marshal cannot fail
		Code:    "too_many_requests",
		Message: "too many login attempts, please try again later",
	}})
}

// --- CORS ---

// corsMiddleware устанавливает CORS-заголовки.
// Если allowedOrigins пуст — разрешает все origins (wildcard, только для local/dev).
// Иначе сверяет Origin запроса с разрешённым списком.
func corsMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
	allowAll := len(allowedOrigins) == 0
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		if o == "*" {
			allowAll = true
		}
		allowed[o] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			if origin != "" {
				if allowAll {
					w.Header().Set("Access-Control-Allow-Origin", origin)
				} else if _, ok := allowed[origin]; ok {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Add("Vary", "Origin")
				}
			} else if allowAll {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			}

			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Device-ID")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
