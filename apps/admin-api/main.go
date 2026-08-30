package main

import (
	"context"
	"log"

	"matjero/internal/actorapi"
	"matjero/internal/commerce"
	"matjero/internal/markets"
	"matjero/internal/openapi"
	"matjero/internal/platformapi"
	"matjero/packages/auth"
	"matjero/packages/config"
	"matjero/packages/database"
	"matjero/packages/httpx"
	"matjero/packages/logging"
	"matjero/packages/observability"
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

	db, err := database.Connect(ctx, cfg)
	if err != nil {
		return err
	}
	defer db.Close()

	verifier, err := auth.NewOIDCVerifier(ctx, auth.Config{
		IssuerURL:  cfg.ZitadelIssuer,
		Audience:   cfg.ZitadelAudience,
		RolesClaim: auth.DefaultRolesClaim(),
	})
	if err != nil {
		return err
	}

	repo := commerce.NewRepository(db.Pool)
	service := commerce.NewService(repo)
	marketService := markets.NewService(markets.NewRepository(db.Pool))
	appCfg := httpx.ConfigFrom(cfg)
	router := httpx.NewRouter(httpx.App{
		Config: appCfg,
		Logger: logger,
		Ready: func(ctx context.Context) error {
			return db.Ping(ctx)
		},
	})
	if spec, err := openapi.BuildAdminSpec(); err == nil {
		if specBytes, err := openapi.MarshalDocument(spec); err == nil {
			router.Mount("/", openapi.NewRouter(openapi.RouterConfig{
				Enabled:   cfg.OpenAPIDocsEnabled,
				SpecPath:  "/openapi.json",
				DocsPath:  "/docs",
				SpecBytes: specBytes,
			}))
		} else {
			return err
		}
	} else {
		return err
	}
	router.Mount("/", actorapi.NewRouter(actorapi.Config{
		AppName:      "Admin API",
		Actor:        "admin",
		RequireAuth:  true,
		AllowedRoles: []string{auth.RolePlatformAdmin},
		Register:     platformapi.RegisterAdminRoutes(platformapi.Dependencies{Commerce: service, Repo: repo}),
	}, marketService, verifier))
	return httpx.Run(ctx, appCfg, logger, router)
}
