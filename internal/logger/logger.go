// Package logger содержит компоненты логирования.
package logger

import (
	"log/slog"
	"os"
	"time"

	"github.com/lmittmann/tint"

	"my-chat/internal/config"
)

// NewLogger создает структурированный логгер на основе конфигурации.
func NewLogger(cfg config.LogConfig) *slog.Logger {
	var handler slog.Handler

	level := parseLevel(cfg.Level)
	handlerOptions := &slog.HandlerOptions{
		Level:     level,
		AddSource: true,
	}

	switch cfg.Format {
	case "json":
		handler = slog.NewJSONHandler(os.Stdout, handlerOptions)
	case "text":
		handler = tint.NewHandler(os.Stdout, &tint.Options{
			Level:      level,
			TimeFormat: time.TimeOnly,
			AddSource:  true,
			NoColor:    false,
		})
	default:
		handler = slog.NewJSONHandler(os.Stdout, handlerOptions)
	}

	return slog.New(handler).With("service", cfg.ServiceName)
}

func parseLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
