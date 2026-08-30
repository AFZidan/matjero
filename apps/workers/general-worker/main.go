package main

import (
	"context"
	"log"
	"log/slog"
	"os/signal"
	"syscall"

	"matjero/packages/config"
	"matjero/packages/logging"
	"matjero/packages/observability"
)

func main() {
	if err := run(context.Background()); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context) error {
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load("general-worker")
	if err != nil {
		return err
	}
	logger := logging.New(cfg)
	shutdown, err := observability.Init(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() { _ = shutdown(context.Background()) }()

	logger.Info("worker ready", slog.String("service", cfg.ServiceName))
	<-ctx.Done()
	logger.Info("worker shutting down")
	return nil
}
