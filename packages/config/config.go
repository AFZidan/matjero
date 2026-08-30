package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	ServiceName     string
	Environment     string
	HTTPAddr        string
	DatabaseURL     string
	RedisAddr       string
	RabbitMQURL     string
	ZitadelIssuer   string
	ShutdownTimeout time.Duration
}

func Load(serviceName string) (Config, error) {
	if serviceName == "" {
		return Config{}, fmt.Errorf("service name is required")
	}

	timeoutSeconds, err := intEnv("SHUTDOWN_TIMEOUT_SECONDS", 10)
	if err != nil {
		return Config{}, err
	}

	return Config{
		ServiceName:     serviceName,
		Environment:     stringEnv("APP_ENV", "development"),
		HTTPAddr:        stringEnv("HTTP_ADDR", ":8080"),
		DatabaseURL:     stringEnv("DATABASE_URL", "postgres://commerce:commerce@localhost:5432/commerce?sslmode=disable"),
		RedisAddr:       stringEnv("REDIS_ADDR", "localhost:6379"),
		RabbitMQURL:     stringEnv("RABBITMQ_URL", "amqp://commerce:commerce@localhost:5672/"),
		ZitadelIssuer:   stringEnv("ZITADEL_ISSUER", "http://localhost:8081"),
		ShutdownTimeout: time.Duration(timeoutSeconds) * time.Second,
	}, nil
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
