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
	chathandler "my-chat/internal/handlers/chat"
	debughandler "my-chat/internal/handlers/debug"
	"my-chat/internal/handlers/health"
	"my-chat/internal/logger"
	mw "my-chat/internal/middleware"
	chatservice "my-chat/internal/services/chat"
	"my-chat/internal/store"
)

// App инкапсулирует зависимости и жизненный цикл HTTP сервера.
type App struct {
	cfg    config.Config
	logger *slog.Logger
	server *http.Server
	store  *store.Store
}

// New создает экземпляр приложения и инициализирует config/logger/server.
func New(ctx context.Context, cfg config.Config) (*App, error) {
	if !cfg.Servers.Client.IsConfigured() {
		return nil, errors.New("servers.client.addr is required for main-service")
	}
	if !cfg.Database.IsConfigured() {
		return nil, errors.New("database.dsn is required for main-service")
	}
	if !cfg.JWT.IsConfigured() {
		return nil, errors.New("jwt.secret is required for main-service")
	}

	log := logger.NewLogger(cfg.Log)
	postgresStore, err := store.New(ctx, cfg.Database.DSN)
	if err != nil {
		return nil, fmt.Errorf("init store: %w", err)
	}

	if cfg.Database.AutoMigrate {
		if err = postgresStore.Migrate(ctx); err != nil {
			postgresStore.Close()
			return nil, fmt.Errorf("run migrations: %w", err)
		}
	}

	dialogRepo := store.NewDialogRepository(postgresStore)
	messageRepo := store.NewMessageRepository(postgresStore)
	receiptRepo := store.NewReceiptRepository(postgresStore)
	chatSvc := chatservice.NewService(dialogRepo, messageRepo, receiptRepo)
	chatHandler := chathandler.New(chatSvc)

	router := chi.NewRouter()
	router.Get("/health", health.Handle)
	router.Get("/debug", debughandler.Handle)

	router.Group(func(r chi.Router) {
		r.Use(mw.Authenticate(cfg.JWT.Secret))

		r.Post("/api/v1/dialogs/{id}/messages", chatHandler.SendMessage)
		r.Get("/api/v1/dialogs/{id}/messages", chatHandler.ListMessages)
		r.Post("/api/v1/messages/{id}/read", chatHandler.MarkRead)
		r.Get("/api/v1/me/unread-count", chatHandler.UnreadCount)
	})

	server := &http.Server{
		Addr:              cfg.Servers.Client.Addr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	return &App{
		cfg:    cfg,
		logger: log,
		server: server,
		store:  postgresStore,
	}, nil
}

// Run запускает сервер и корректно завершает его по сигналу отмены контекста.
func (a *App) Run(ctx context.Context) error {
	defer a.store.Close()

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
