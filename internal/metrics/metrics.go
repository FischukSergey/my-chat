// Package metrics содержит определения Prometheus-метрик и вспомогательные функции.
package metrics

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Метрики HTTP-слоя.
var (
	HTTPRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests by method, path and status.",
		},
		[]string{"method", "path", "status"},
	)

	HTTPRequestDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request duration in seconds by method and path.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)
)

// WSConnectionsActive tracks the number of currently active WebSocket connections.
var WSConnectionsActive = prometheus.NewGauge(prometheus.GaugeOpts{
	Name: "ws_connections_active",
	Help: "Number of currently active WebSocket connections.",
})

// Метрики бизнес-логики.
var (
	MessageSendTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "message_send_total",
		Help: "Total number of messages successfully sent.",
	})

	MessageExpiredTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "message_expired_total",
		Help: "Total number of messages expired by TTL.",
	})
)

func init() {
	prometheus.MustRegister(
		HTTPRequestsTotal,
		HTTPRequestDurationSeconds,
		WSConnectionsActive,
		MessageSendTotal,
		MessageExpiredTotal,
	)
}

// Serve запускает HTTP сервер с /metrics эндпоинтом на указанном адресе.
// Завершается при отмене ctx.
func Serve(ctx context.Context, addr string, log *slog.Logger) error {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	log.Info("запуск metrics сервера", slog.String("addr", addr))
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("metrics server: %w", err)
	}

	return nil
}
