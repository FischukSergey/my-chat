// Package messageexpirer содержит bootstrap приложения message-expirer.
package messageexpirer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"my-chat/internal/config"
	"my-chat/internal/logger"
	"my-chat/internal/metrics"
	"my-chat/internal/services/expirer"
	"my-chat/internal/store"
)

const (
	defaultIntervalSeconds = 10
	defaultBatchSize       = 100
)

// App инкапсулирует bootstrap для сервиса message-expirer.
type App struct {
	logger  *slog.Logger
	env     string
	store   *store.Store
	expirer *expirer.Expirer
	cfg     config.Config
}

// New создает и инициализирует App, подключается к БД.
func New(ctx context.Context, cfg config.Config) (*App, error) {
	if !cfg.Database.IsConfigured() {
		return nil, errors.New("database.dsn is required for message-expirer")
	}

	log := logger.NewLogger(cfg.Log)
	log.Info("инициализация message-expirer", slog.String("env", cfg.Global.Env))

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

	messageRepo := store.NewMessageRepository(postgresStore)
	wsOutboxRepo := store.NewWSEventOutboxRepository(postgresStore)

	batchSize := defaultBatchSize
	if cfg.Expirer.BatchSize > 0 {
		batchSize = cfg.Expirer.BatchSize
	}

	exp := expirer.New(messageRepo, wsOutboxRepo, log, batchSize)
	exp.SetExpiredCounter(metrics.MessageExpiredTotal)

	return &App{
		logger:  log,
		env:     cfg.Global.Env,
		store:   postgresStore,
		expirer: exp,
		cfg:     cfg,
	}, nil
}

// Run запускает тикер expirer и завершает его по отмене контекста.
func (a *App) Run(ctx context.Context) error {
	defer func() {
		a.store.Close()
		a.logger.Info("подключение к PostgreSQL закрыто")
	}()

	// Сервер метрик на отдельном порту (только internal трафик).
	if a.cfg.Servers.Metrics.IsConfigured() {
		go func() {
			if err := metrics.Serve(ctx, a.cfg.Servers.Metrics.Addr, a.logger); err != nil {
				a.logger.Error("metrics сервер завершился с ошибкой", slog.String("error", err.Error()))
			}
		}()
	}

	interval := intervalFromCfg(a.cfg.Expirer.IntervalSeconds)

	a.logger.Info("запуск сервиса",
		slog.String("service", "message-expirer"),
		slog.String("env", a.env),
		slog.Duration("interval", interval),
		slog.Int("batch_size", batchSizeFromCfg(a.cfg.Expirer.BatchSize)),
	)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			cause := context.Cause(ctx)
			if cause == nil {
				cause = context.Canceled
			}
			a.logger.Info("остановка сервиса",
				slog.String("service", "message-expirer"),
				slog.String("cause", cause.Error()),
			)
			return nil
		case <-ticker.C:
			if _, err := a.expirer.Tick(ctx); err != nil {
				a.logger.Error("ошибка при обработке истёкших сообщений",
					slog.String("error", err.Error()),
				)
			}
		}
	}
}

func intervalFromCfg(seconds int) time.Duration {
	if seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	return defaultIntervalSeconds * time.Second
}

func batchSizeFromCfg(size int) int {
	if size > 0 {
		return size
	}
	return defaultBatchSize
}
