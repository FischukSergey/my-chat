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
	devicehandler "my-chat/internal/handlers/device"
	"my-chat/internal/handlers/health"
	pushhandler "my-chat/internal/handlers/push"
	userhandler "my-chat/internal/handlers/user"
	wshandler "my-chat/internal/handlers/ws"
	"my-chat/internal/hub"
	"my-chat/internal/logger"
	"my-chat/internal/metrics"
	mw "my-chat/internal/middleware"
	chatservice "my-chat/internal/services/chat"
	deviceservice "my-chat/internal/services/device"
	userservice "my-chat/internal/services/user"
	"my-chat/internal/services/wsdelivery"
	"my-chat/internal/store"
)

const (
	wsDeliveryPollInterval = 5 * time.Second
	wsDeliveryBatchSize    = 50
)

// App инкапсулирует зависимости и жизненный цикл HTTP сервера.
type App struct {
	cfg        config.Config
	logger     *slog.Logger
	server     *http.Server
	store      *store.Store
	wsDelivery *wsdelivery.Delivery
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
	deviceRepo := store.NewDeviceRepository(postgresStore)
	userRepo := store.NewUserRepository(postgresStore)
	outboxRepo := store.NewNotificationOutboxRepository(postgresStore)
	wsOutboxRepo := store.NewWSEventOutboxRepository(postgresStore)
	log.Info("инициализированы репозитории хранилища", slog.Int("repositories_count", 7))

	connHub := hub.New(log)
	connHub.SetConnGauge(metrics.WSConnectionsActive)
	messageTTL := time.Duration(cfg.Chat.MessageTTLSeconds) * time.Second
	chatSvc := chatservice.NewService(dialogRepo, messageRepo, receiptRepo, connHub, outboxRepo, messageTTL)
	chatSvc.SetMessageCounter(metrics.MessageSendTotal)
	chatSvc.SetUsersRepository(userRepo)
	deviceSvc := deviceservice.NewService(deviceRepo)
	userSvc := userservice.NewService(userRepo)
	chatHandler := chathandler.New(chatSvc)
	deviceHandler := devicehandler.New(deviceSvc)
	userHandler := userhandler.New(userSvc)
	vapidHandler := pushhandler.New(cfg.NotificationWorker.WebPush.VAPIDPublicKey)
	wsHandler := wshandler.New(connHub, cfg.JWT.Secret, log)
	wsDelivery := wsdelivery.New(wsOutboxRepo, connHub, log, wsDeliveryBatchSize)

	router := chi.NewRouter()
	router.Use(corsMiddleware)
	router.Use(mw.PrometheusMiddleware)
	router.Get("/health", health.Handle)
	router.Get("/debug", debughandler.Handle)
	router.Get("/ws/connect", wsHandler.Connect)

	// Публичные маршруты (без auth middleware).
	router.Post("/api/v1/users/register", userHandler.Register)
	router.Get("/api/v1/push/vapid-public-key", vapidHandler.VapidPublicKey)

	router.Group(func(r chi.Router) {
		r.Use(mw.Authenticate(cfg.JWT.Secret))

		r.Post("/api/v1/devices/register", deviceHandler.Register)
		r.Post("/api/v1/devices/unregister", deviceHandler.Unregister)

		r.Get("/api/v1/users/search", userHandler.Search)

		r.Get("/api/v1/dialogs", chatHandler.ListDialogs)
		r.Post("/api/v1/dialogs", chatHandler.CreateDialog)
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
		cfg:        cfg,
		logger:     log,
		server:     server,
		store:      postgresStore,
		wsDelivery: wsDelivery,
	}, nil
}

// Run запускает сервер и корректно завершает его по сигналу отмены контекста.
func (a *App) Run(ctx context.Context) error {
	defer func() {
		a.store.Close()
		a.logger.Info("подключение к PostgreSQL закрыто")
	}()

	// Горутина доставки WS-событий из outbox подключённым клиентам.
	go a.wsDelivery.Run(ctx, wsDeliveryPollInterval)

	// Сервер метрик на отдельном порту (только internal трафик).
	if a.cfg.Servers.Metrics.IsConfigured() {
		go func() {
			if err := metrics.Serve(ctx, a.cfg.Servers.Metrics.Addr, a.logger); err != nil {
				a.logger.Error("metrics сервер завершился с ошибкой", slog.String("error", err.Error()))
			}
		}()
	}

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

// corsMiddleware разрешает cross-origin запросы для debug UI и мобильного клиента.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Device-ID")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
