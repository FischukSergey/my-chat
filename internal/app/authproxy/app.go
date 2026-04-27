// Package authproxy содержит bootstrap приложения auth-proxy.
package authproxy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"my-chat/internal/config"
	authhandler "my-chat/internal/handlers/auth"
	"my-chat/internal/logger"
)

// App инкапсулирует зависимости и жизненный цикл HTTP сервера auth-proxy.
type App struct {
	cfg    config.Config
	logger *slog.Logger
	server *http.Server
}

// New создает bootstrap для сервиса auth-proxy.
func New(cfg config.Config) (*App, error) {
	if !cfg.Servers.Client.IsConfigured() {
		return nil, errors.New("servers.client.addr is required for auth-proxy")
	}
	if !cfg.JWT.IsConfigured() {
		return nil, errors.New("jwt.secret is required for auth-proxy")
	}

	log := logger.NewLogger(cfg.Log)
	log.Info(
		"инициализация auth-proxy",
		slog.String("env", cfg.Global.Env),
		slog.String("addr", cfg.Servers.Client.Addr),
		slog.Int("access_token_ttl_sec", cfg.JWT.AccessTokenTTL),
		slog.Int("refresh_token_ttl_sec", cfg.JWT.RefreshTokenTTL),
	)

	authHandler := authhandler.New(authhandler.Config{
		JWTSecret:          cfg.JWT.Secret,
		AccessTokenTTLSec:  cfg.JWT.AccessTokenTTL,
		RefreshTokenTTLSec: cfg.JWT.RefreshTokenTTL,
	})

	router := chi.NewRouter()
	router.Use(corsMiddleware)
	router.Post("/api/v1/auth/login", authHandler.Login)
	router.Post("/api/v1/auth/refresh", authHandler.Refresh)
	router.Post("/api/v1/auth/logout", authHandler.Logout)
	log.Info("маршруты auth-proxy зарегистрированы", slog.Int("routes_count", 3))

	server := &http.Server{
		Addr:              cfg.Servers.Client.Addr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	return &App{
		cfg:    cfg,
		logger: log,
		server: server,
	}, nil
}

// corsMiddleware разрешает cross-origin запросы для локальной отладки через /debug.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Run запускает сервер и корректно завершает его по сигналу отмены контекста.
func (a *App) Run(ctx context.Context) error {
	errCh := make(chan error, 1)

	go func() {
		a.logger.Info("запуск HTTP сервера auth-proxy", slog.String("addr", a.cfg.Servers.Client.Addr))
		a.logger.Info("auth-proxy готов принимать запросы")

		if err := a.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			a.logger.Error("HTTP сервер auth-proxy завершился с ошибкой", slog.String("error", err.Error()))
			errCh <- err
			return
		}

		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		cause := context.Cause(ctx)
		if cause == nil {
			cause = context.Canceled
		}
		a.logger.Info("получен сигнал остановки auth-proxy", slog.String("cause", cause.Error()))

		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()

		if err := a.server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown auth-proxy: %w", err)
		}
		a.logger.Info("HTTP сервер auth-proxy остановлен")

		return nil
	case err := <-errCh:
		return err
	}
}
