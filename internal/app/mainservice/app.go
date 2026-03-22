// Package mainservice содержит bootstrap приложения main-service.
package mainservice

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"my-chat/internal/config"
	debughandler "my-chat/internal/handlers/debug"
	"my-chat/internal/handlers/health"
	"my-chat/internal/logger"
)

// App инкапсулирует зависимости и жизненный цикл HTTP сервера.
type App struct {
	cfg    config.Config
	logger *slog.Logger
	server *http.Server
}

// New создает экземпляр приложения и инициализирует config/logger/server.
func New(cfg config.Config) (*App, error) {
	if !cfg.Servers.Client.IsConfigured() {
		return nil, errors.New("servers.client.addr is required for main-service")
	}

	log := logger.NewLogger(cfg.Log)

	router := chi.NewRouter()
	router.Get("/health", health.Handle)
	router.Get("/debug", debughandler.Handle)

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
		a.logger.Info("запуск main-service", slog.String("addr", a.cfg.Servers.Client.Addr))

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
			return fmt.Errorf("shutdown main-service: %w", err)
		}

		return nil
	case err := <-errCh:
		return err
	}
}
