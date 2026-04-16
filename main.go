package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"reverseproxy-poc/internal/app"
	"reverseproxy-poc/internal/config"
)

func main() {
	logger := log.New(os.Stdout, "[reverseproxy] ", log.LstdFlags|log.Lmicroseconds)

	configPath := "configs/app.json"
	if len(os.Args) > 1 {
		configPath = os.Args[1]
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		logger.Fatalf("load config: %v", err)
	}

	application, err := app.New(cfg, configPath, logger)
	if err != nil {
		logger.Fatalf("build app: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := application.Run(ctx); err != nil {
		logger.Fatalf("run app: %v", err)
	}
}
