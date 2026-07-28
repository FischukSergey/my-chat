// Package notificationworker содержит bootstrap приложения notification-worker.
package notificationworker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"my-chat/internal/clients/push"
	"my-chat/internal/config"
	"my-chat/internal/logger"
	"my-chat/internal/services/notification"
	"my-chat/internal/store"
)

const (
	defaultPollInterval    = 5 * time.Second
	defaultBatchSize       = 20
	defaultMaxAttempts     = 5
	defaultBackoffBase     = 30 * time.Second
	defaultProvider        = "dev-log"
	defaultOutboxRetention = 7 * 24 * time.Hour
	housekeepingInterval   = 24 * time.Hour
)

// App инкапсулирует зависимости и жизненный цикл notification-worker.
type App struct {
	log        *slog.Logger
	store      *store.Store
	worker     *notification.Worker
	outboxRepo *store.NotificationOutboxRepository
	cfg        config.Config
}

// New создаёт и инициализирует App.
func New(ctx context.Context, cfg config.Config) (*App, error) {
	if !cfg.Database.IsConfigured() {
		return nil, errors.New("database.dsn is required for notification-worker")
	}

	log := logger.NewLogger(cfg.Log)
	log.Info(
		"инициализация notification-worker",
		slog.String("env", cfg.Global.Env),
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
		report, migErr := postgresStore.Migrate(ctx)
		if migErr != nil {
			postgresStore.Close()
			return nil, fmt.Errorf("run migrations: %w", migErr)
		}
		log.Info("миграции БД применены",
			slog.Int("count", len(report.Applied)),
			slog.Any("migrations", report.Applied),
		)
	}

	outboxRepo := store.NewNotificationOutboxRepository(postgresStore)
	deviceRepo := store.NewDeviceRepository(postgresStore)

	provider := buildProvider(cfg.NotificationWorker.Provider, log)
	workerCfg := buildWorkerConfig(cfg.NotificationWorker)

	log.Info("конфигурация worker",
		slog.String("provider", provider.Name()),
		slog.Int("batch_size", workerCfg.BatchSize),
		slog.Int("max_attempts", workerCfg.MaxAttempts),
		slog.Duration("backoff_base", workerCfg.BackoffBase),
	)

	w := notification.NewWorker(outboxRepo, deviceRepo, provider, log, workerCfg)

	return &App{
		log:        log,
		store:      postgresStore,
		worker:     w,
		outboxRepo: outboxRepo,
		cfg:        cfg,
	}, nil
}

// Run запускает worker и ждёт отмены контекста.
func (a *App) Run(ctx context.Context) error {
	defer func() {
		a.store.Close()
		a.log.Info("подключение к PostgreSQL закрыто")
	}()

	pollInterval := pollIntervalFromCfg(a.cfg.NotificationWorker.PollIntervalSeconds)
	retention := retentionFromCfg(a.cfg.NotificationWorker.OutboxRetentionSeconds)

	done := make(chan struct{})
	go func() {
		defer close(done)
		a.worker.Run(ctx, pollInterval)
	}()

	go a.runHousekeepingLoop(ctx, retention)

	<-ctx.Done()
	cause := context.Cause(ctx)
	if cause == nil {
		cause = context.Canceled
	}
	a.log.Info("получен сигнал остановки notification-worker", slog.String("cause", cause.Error()))

	<-done
	a.log.Info("notification-worker остановлен")

	return nil
}

func buildProvider(name string, log *slog.Logger) push.Provider {
	switch name {
	case "noop":
		return push.NewNoopProvider()
	default:
		return push.NewDevLogProvider(log)
	}
}

func buildWorkerConfig(cfg config.NotificationWorkerConfig) notification.Config {
	wCfg := notification.Config{
		BatchSize:   defaultBatchSize,
		MaxAttempts: defaultMaxAttempts,
		BackoffBase: defaultBackoffBase,
	}
	if cfg.BatchSize > 0 {
		wCfg.BatchSize = cfg.BatchSize
	}
	if cfg.MaxAttempts > 0 {
		wCfg.MaxAttempts = cfg.MaxAttempts
	}
	if cfg.BackoffBaseSeconds > 0 {
		wCfg.BackoffBase = time.Duration(cfg.BackoffBaseSeconds) * time.Second
	}
	return wCfg
}

func pollIntervalFromCfg(seconds int) time.Duration {
	if seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	return defaultPollInterval
}

func retentionFromCfg(seconds int) time.Duration {
	if seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	return defaultOutboxRetention
}

// runHousekeepingLoop выполняет очистку outbox при старте, затем раз в housekeepingInterval.
func (a *App) runHousekeepingLoop(ctx context.Context, retention time.Duration) {
	a.log.Info("outbox_housekeeping: запуск", slog.Duration("retention", retention))
	a.runHousekeeping(ctx, retention)

	ticker := time.NewTicker(housekeepingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.runHousekeeping(ctx, retention)
		}
	}
}

func (a *App) runHousekeeping(ctx context.Context, retention time.Duration) {
	n, err := a.outboxRepo.DeleteSent(ctx, retention)
	if err != nil {
		a.log.Error("outbox_housekeeping: ошибка очистки", slog.String("error", err.Error()))
		return
	}
	if n > 0 {
		a.log.Info("outbox_housekeeping: удалено записей", slog.Int64("count", n))
	}
}
