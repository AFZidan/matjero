package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/matjeroapps/core/packages/config"
	"github.com/matjeroapps/core/packages/logging"
	"github.com/matjeroapps/core/packages/messaging"
	"github.com/matjeroapps/core/packages/observability"
	"github.com/matjeroapps/core/packages/outbox"
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

	dbPool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect db pool: %w", err)
	}
	defer dbPool.Close()

	if err := dbPool.Ping(ctx); err != nil {
		return fmt.Errorf("ping db pool: %w", err)
	}

	logger.Info("worker connected to postgres", slog.String("service", cfg.ServiceName))

	backoff := 1 * time.Second
	for {
		select {
		case <-ctx.Done():
			logger.Info("worker shutting down")
			return nil
		default:
		}

		conn, ch, pub, err := setupRabbit(cfg.RabbitMQURL)
		if err != nil {
			logger.Error("rabbitmq connection failed, retrying...", slog.String("error", err.Error()), slog.Duration("backoff", backoff))
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(backoff):
			}
			if backoff < 10*time.Second {
				backoff *= 2
			}
			continue
		}

		backoff = 1 * time.Second
		logger.Info("worker connected to rabbitmq and confirmed topology")

		proc := outbox.NewProcessor(cfg, dbPool, pub, logger)
		err = proc.Run(ctx)
		_ = ch.Close()
		_ = conn.Close()

		if errors.Is(ctx.Err(), context.Canceled) {
			logger.Info("worker shutdown complete")
			return nil
		}

		if err != nil {
			logger.Warn("processor stopped with error, reconnecting...", slog.String("error", err.Error()))
		}
	}
}

func setupRabbit(rabbitURL string) (*amqp.Connection, *amqp.Channel, messaging.Publisher, error) {
	conn, err := amqp.Dial(rabbitURL)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("dial rabbitmq: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, nil, nil, fmt.Errorf("open rabbitmq channel: %w", err)
	}

	err = ch.ExchangeDeclare(
		"commerce.events", // name
		"topic",           // type
		true,              // durable
		false,             // auto-deleted
		false,             // internal
		false,             // no-wait
		nil,               // arguments
	)
	if err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, nil, nil, fmt.Errorf("declare exchange commerce.events: %w", err)
	}

	pub, err := messaging.NewRabbitPublisher(ch)
	if err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, nil, nil, fmt.Errorf("new rabbit publisher: %w", err)
	}

	return conn, ch, pub, nil
}
