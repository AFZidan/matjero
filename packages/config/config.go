package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	ServiceName             string
	Environment             string
	HTTPAddr                string
	DatabaseURL             string
	RedisAddr               string
	RabbitMQURL             string
	ZitadelIssuer           string
	ZitadelAudience         string
	OpenAPIDocsEnabled      bool
	ShutdownTimeout         time.Duration
	CheckoutSessionLifetime time.Duration

	// PlatformDomain is the base domain under which platform-generated store
	// subdomains are allocated (e.g. "<store-code>.matjero.com"). It is
	// configuration-driven and never hardcoded in application code.
	PlatformDomain string
	// TrustedForwardedHost enables honoring the X-Forwarded-Host header for
	// tenant resolution. It must only be enabled behind an explicitly trusted
	// reverse proxy; otherwise the request Host header is authoritative.
	TrustedForwardedHost bool
	// ReservedSubdomains are subdomain labels that sellers may not claim as a
	// store code (e.g. www, api, admin).
	ReservedSubdomains []string
	// ThemePreviewSecret is the server-side HMAC signing key for short-lived,
	// store-scoped theme draft preview tokens. It must be configuration-driven
	// and is never hardcoded in application code.
	ThemePreviewSecret string

	// Internal service credentials for the Core internal API (ADR-017). Each
	// actor service presents its own bearer token; a caller with no configured
	// token cannot authenticate. These are secrets and must never be committed,
	// logged, or embedded in an image layer.
	InternalSellerToken   string
	InternalAdminToken    string
	InternalSupplierToken string
}

func Load(serviceName string) (Config, error) {
	if serviceName == "" {
		return Config{}, fmt.Errorf("service name is required")
	}

	timeoutSeconds, err := intEnv("SHUTDOWN_TIMEOUT_SECONDS", 10)
	if err != nil {
		return Config{}, err
	}
	checkoutSessionLifetime, err := durationEnv("CHECKOUT_SESSION_LIFETIME", 30*time.Minute)
	if err != nil {
		return Config{}, err
	}

	return Config{
		ServiceName:             serviceName,
		Environment:             stringEnv("APP_ENV", "development"),
		HTTPAddr:                stringEnv("HTTP_ADDR", ":8080"),
		DatabaseURL:             stringEnv("DATABASE_URL", "postgres://commerce:commerce@localhost:5432/commerce?sslmode=disable"),
		RedisAddr:               stringEnv("REDIS_ADDR", "localhost:6379"),
		RabbitMQURL:             stringEnv("RABBITMQ_URL", "amqp://commerce:commerce@localhost:5672/"),
		ZitadelIssuer:           stringEnv("ZITADEL_ISSUER", "http://localhost:8081"),
		ZitadelAudience:         stringEnv("ZITADEL_AUDIENCE", serviceName),
		OpenAPIDocsEnabled:      boolEnv("OPENAPI_DOCS_ENABLED", stringEnv("APP_ENV", "development") != "production"),
		ShutdownTimeout:         time.Duration(timeoutSeconds) * time.Second,
		CheckoutSessionLifetime: checkoutSessionLifetime,
		PlatformDomain:          stringEnv("PLATFORM_DOMAIN", "matjero.com"),
		TrustedForwardedHost:    boolEnv("TRUSTED_FORWARDED_HOST", false),
		ReservedSubdomains:      stringSliceEnv("RESERVED_SUBDOMAINS", []string{"www", "api", "admin", "app", "cdn", "mail", "seller", "supplier", "static", "assets"}),
		ThemePreviewSecret:      stringEnv("THEME_PREVIEW_SECRET", ""),

		InternalSellerToken:   stringEnv("CORE_INTERNAL_SELLER_TOKEN", ""),
		InternalAdminToken:    stringEnv("CORE_INTERNAL_ADMIN_TOKEN", ""),
		InternalSupplierToken: stringEnv("CORE_INTERNAL_SUPPLIER_TOKEN", ""),
	}, nil
}

func durationEnv(key string, fallback time.Duration) (time.Duration, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be a valid duration: %w", key, err)
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("%s must be greater than zero", key)
	}
	return parsed, nil
}

// stringSliceEnv reads a comma-separated environment variable, trimming
// whitespace from each entry and dropping empty values.
func stringSliceEnv(key string, fallback []string) []string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	if len(out) == 0 {
		return fallback
	}
	return out
}

func stringEnv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func intEnv(key string, fallback int) (int, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", key, err)
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("%s must be greater than zero", key)
	}

	return parsed, nil
}

func boolEnv(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}
