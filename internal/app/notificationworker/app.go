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
	env    string
}

// New создает bootstrap для сервиса notification-worker.
func New(cfg config.Config) *App {
	app := &App{
		logger: logger.NewLogger(cfg.Log),
		env:    cfg.Global.Env,
	}

	app.logger.Info("инициализация notification-worker", slog.String("env", app.env))

	return app
}

// Run запускает сервис и завершает его по отмене контекста.
func (a *App) Run(ctx context.Context) error {
	a.logger.Info("запуск сервиса", slog.String("service", "notification-worker"), slog.String("env", a.env))
	a.logger.Info("notification-worker готов к обработке задач")
	<-ctx.Done()

	cause := context.Cause(ctx)
	if cause == nil {
		cause = context.Canceled
	}

	a.logger.Info("остановка сервиса", slog.String("service", "notification-worker"), slog.String("cause", cause.Error()))

	return nil
}
