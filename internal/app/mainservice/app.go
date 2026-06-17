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
	wshandler "my-chat/internal/handlers/ws"
	"my-chat/internal/hub"
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
	log.Info(
		"инициализация main-service",
		slog.String("env", cfg.Global.Env),
		slog.String("addr", cfg.Servers.Client.Addr),
		slog.Bool("auto_migrate", cfg.Database.AutoMigrate),
	)

	log.Info("подключение к PostgreSQL")
	postgresStore, err := store.New(ctx, cfg.Database.DSN)
	if err != nil {
		return nil, fmt.Errorf("init store: %w", err)
	}
	log.Info("подключение к PostgreSQL успешно")

	if cfg.Database.AutoMigrate {
		log.Info("запуск миграций БД")
		migrationReport, migrationErr := postgresStore.Migrate(ctx)
		if migrationErr != nil {
			postgresStore.Close()
			return nil, fmt.Errorf("run migrations: %w", migrationErr)
		}
		log.Info(
			"миграции БД применены успешно",
			slog.Int("migrations_count", len(migrationReport.Applied)),
			slog.Any("migrations", migrationReport.Applied),
		)
	} else {
		log.Info("автоматические миграции отключены")
	}

	dialogRepo := store.NewDialogRepository(postgresStore)
	messageRepo := store.NewMessageRepository(postgresStore)
	receiptRepo := store.NewReceiptRepository(postgresStore)
	log.Info("инициализированы репозитории хранилища", slog.Int("repositories_count", 3))

	connHub := hub.New(log)
	chatSvc := chatservice.NewService(dialogRepo, messageRepo, receiptRepo, connHub)
	chatHandler := chathandler.New(chatSvc)
	wsHandler := wshandler.New(connHub, cfg.JWT.Secret, log)

	router := chi.NewRouter()
	router.Get("/health", health.Handle)
	router.Get("/debug", debughandler.Handle)
	router.Get("/ws/connect", wsHandler.Connect)

	router.Group(func(r chi.Router) {
		r.Use(mw.Authenticate(cfg.JWT.Secret))

		r.Post("/api/v1/dialogs/{id}/messages", chatHandler.SendMessage)
		r.Get("/api/v1/dialogs/{id}/messages", chatHandler.ListMessages)
		r.Post("/api/v1/messages/{id}/read", chatHandler.MarkRead)
		r.Get("/api/v1/me/unread-count", chatHandler.UnreadCount)
	})
	log.Info("маршруты main-service зарегистрированы")

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
	defer func() {
		a.store.Close()
		a.logger.Info("подключение к PostgreSQL закрыто")
	}()

	errCh := make(chan error, 1)

	go func() {
		a.logger.Info("запуск HTTP сервера main-service", slog.String("addr", a.cfg.Servers.Client.Addr))
		a.logger.Info("main-service готов принимать запросы")

		if err := a.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			a.logger.Error("HTTP сервер main-service завершился с ошибкой", slog.String("error", err.Error()))
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
		a.logger.Info("получен сигнал остановки main-service", slog.String("cause", cause.Error()))

		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()

		if err := a.server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown main-service: %w", err)
		}
		a.logger.Info("HTTP сервер main-service остановлен")

		return nil
	case err := <-errCh:
		return err
	}
}
