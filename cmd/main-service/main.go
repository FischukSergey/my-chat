// Package main содержит точку входа сервиса.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"my-chat/internal/app/mainservice"
	"my-chat/internal/config"
)

var configPath = flag.String("config", "configs/config.main-service.local.example.yaml", "Путь к конфигу")

func main() {
	flag.Parse()

	cfg, err := config.ParseAndValidate(*configPath)
	if err != nil {
		log.Fatalf("parse config: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	application, err := mainservice.New(cfg)
	if err != nil {
		log.Printf("init app: %v", err)
		stop()
		os.Exit(1)
	}

	if err = application.Run(ctx); err != nil {
		log.Printf("run app: %v", err)
		stop()
		os.Exit(1)
	}

	stop()
}
