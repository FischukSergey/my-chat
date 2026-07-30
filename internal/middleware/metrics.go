package middleware

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"my-chat/internal/metrics"
)

// statusRecorder перехватывает HTTP статус-код из handler-а.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (sr *statusRecorder) WriteHeader(status int) {
	sr.status = status
	sr.ResponseWriter.WriteHeader(status)
}

// Hijack реализует http.Hijacker — необходим для WebSocket-апгрейда.
// Делегирует вызов к оборачиваемому ResponseWriter.
func (sr *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := sr.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("underlying ResponseWriter does not implement http.Hijacker")
	}
	return h.Hijack()
}

// PrometheusMiddleware собирает метрики http_requests_total и http_request_duration_seconds.
// Использует chi RoutePattern в качестве лейбла пути, чтобы избежать высокой кардинальности от ID в URL.
func PrometheusMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sr := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(sr, r)

		pattern := "unknown"
		if rc := chi.RouteContext(r.Context()); rc != nil {
			if p := rc.RoutePattern(); p != "" {
				pattern = p
			}
		}

		duration := time.Since(start).Seconds()
		status := strconv.Itoa(sr.status)

		metrics.HTTPRequestsTotal.WithLabelValues(r.Method, pattern, status).Inc()
		metrics.HTTPRequestDurationSeconds.WithLabelValues(r.Method, pattern).Observe(duration)
	})
}
