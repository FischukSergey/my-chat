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

	authhandler "my-chat/internal/handlers/auth"

	"my-chat/internal/config"
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

	authHandler := authhandler.New(authhandler.Config{
		JWTSecret:          cfg.JWT.Secret,
		AccessTokenTTLSec:  cfg.JWT.AccessTokenTTL,
		RefreshTokenTTLSec: cfg.JWT.RefreshTokenTTL,
	})

	router := chi.NewRouter()
	router.Post("/api/v1/auth/login", authHandler.Login)
	router.Post("/api/v1/auth/refresh", authHandler.Refresh)
	router.Post("/api/v1/auth/logout", authHandler.Logout)

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

// Run запускает сервер и корректно завершает его по сигналу отмены контекста.
func (a *App) Run(ctx context.Context) error {
	errCh := make(chan error, 1)

	go func() {
		a.logger.Info("запуск auth-proxy", slog.String("addr", a.cfg.Servers.Client.Addr))

		if err := a.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}

		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()

		if err := a.server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown auth-proxy: %w", err)
		}

		return nil
	case err := <-errCh:
		return err
	}
}
