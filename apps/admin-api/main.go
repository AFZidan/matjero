package main

import (
	"context"
	"log"

	"dropshipping/packages/config"
	"dropshipping/packages/httpx"
	"dropshipping/packages/logging"
	"dropshipping/packages/observability"
)

func main() {
	if err := run(context.Background()); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context) error {
	cfg, err := config.Load("admin-api")
	if err != nil {
		return err
	}

	logger := logging.New(cfg)
	shutdown, err := observability.Init(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() { _ = shutdown(context.Background()) }()

	appCfg := httpx.ConfigFrom(cfg)
	router := httpx.NewRouter(httpx.App{Config: appCfg, Logger: logger})
	return httpx.Run(ctx, appCfg, logger, router)
}
