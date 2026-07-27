// Package main содержит точку входа сервиса.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"my-chat/internal/app/messageexpirer"
	"my-chat/internal/config"
)

var configPath = flag.String("config", "configs/config.message-expirer.local.example.yaml", "Путь к конфигу")

func main() {
	flag.Parse()

	cfg, err := config.ParseAndValidate(*configPath)
	if err != nil {
		log.Fatalf("parse config: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	application, err := messageexpirer.New(ctx, cfg)
	if err != nil {
		stop()
		log.Fatalf("init app: %v", err)
	}

	if err = application.Run(ctx); err != nil {
		stop()
		log.Printf("run app: %v", err)
		os.Exit(1)
	}

	stop()
}
