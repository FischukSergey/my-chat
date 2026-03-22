// Package messageexpirer содержит bootstrap приложения message-expirer.
package messageexpirer

import (
	"context"
	"log/slog"

	"my-chat/internal/config"
	"my-chat/internal/logger"
)

// App инкапсулирует bootstrap для сервиса message-expirer.
type App struct {
	logger *slog.Logger
}

// New создает bootstrap для сервиса message-expirer.
func New(cfg config.Config) *App {
	return &App{
		logger: logger.NewLogger(cfg.Log),
	}
}

// Run запускает сервис и завершает его по отмене контекста.
func (a *App) Run(ctx context.Context) error {
	a.logger.Info("запуск сервиса", slog.String("service", "message-expirer"))
	<-ctx.Done()

	cause := context.Cause(ctx)
	if cause == nil {
		cause = context.Canceled
	}

	a.logger.Info("остановка сервиса", slog.String("service", "message-expirer"), slog.String("cause", cause.Error()))

	return nil
}
