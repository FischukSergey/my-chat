// Package notificationworker содержит bootstrap приложения notification-worker.
package notificationworker

import (
	"context"
	"log/slog"

	"my-chat/internal/config"
	"my-chat/internal/logger"
)

// App инкапсулирует bootstrap для сервиса notification-worker.
type App struct {
	logger *slog.Logger
}

// New создает bootstrap для сервиса notification-worker.
func New(cfg config.Config) *App {
	return &App{
		logger: logger.NewLogger(cfg.Log),
	}
}

// Run запускает сервис и завершает его по отмене контекста.
func (a *App) Run(ctx context.Context) error {
	a.logger.Info("запуск сервиса", slog.String("service", "notification-worker"))
	<-ctx.Done()

	cause := context.Cause(ctx)
	if cause == nil {
		cause = context.Canceled
	}

	a.logger.Info("остановка сервиса", slog.String("service", "notification-worker"), slog.String("cause", cause.Error()))

	return nil
}
