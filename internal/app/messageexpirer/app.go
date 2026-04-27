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
	env    string
}

// New создает bootstrap для сервиса message-expirer.
func New(cfg config.Config) *App {
	app := &App{
		logger: logger.NewLogger(cfg.Log),
		env:    cfg.Global.Env,
	}

	app.logger.Info("инициализация message-expirer", slog.String("env", app.env))

	return app
}

// Run запускает сервис и завершает его по отмене контекста.
func (a *App) Run(ctx context.Context) error {
	a.logger.Info("запуск сервиса", slog.String("service", "message-expirer"), slog.String("env", a.env))
	a.logger.Info("message-expirer готов к обработке задач")
	<-ctx.Done()

	cause := context.Cause(ctx)
	if cause == nil {
		cause = context.Canceled
	}

	a.logger.Info("остановка сервиса", slog.String("service", "message-expirer"), slog.String("cause", cause.Error()))

	return nil
}
