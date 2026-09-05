package outbox

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/matjeroapps/core/packages/config"
	"github.com/matjeroapps/core/packages/events"
)

type EventPublisher interface {
	PublishEvent(ctx context.Context, exchange, routingKey string, event events.EventEnvelope) error
}

type Processor struct {
	cfg       config.Config
	db        DBExecutor
	publisher EventPublisher
	logger    *slog.Logger
	store     Store
}

func NewProcessor(cfg config.Config, db DBExecutor, publisher EventPublisher, logger *slog.Logger) *Processor {
	if logger == nil {
		logger = slog.Default()
	}
	return &Processor{
		cfg:       cfg,
		db:        db,
		publisher: publisher,
		logger:    logger,
		store:     NewStore(),
	}
}

// Run executes the outbox processing loop until ctx is canceled.
func (p *Processor) Run(ctx context.Context) error {
	p.logger.Info("outbox processor loop started",
		slog.Duration("poll_interval", p.cfg.OutboxPollInterval),
		slog.Duration("lease_duration", p.cfg.OutboxClaimLeaseDuration),
		slog.Int("batch_size", p.cfg.OutboxBatchSize),
	)

	ticker := time.NewTicker(p.cfg.OutboxPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			p.logger.Info("outbox processor loop stopping")
			return nil
		default:
		}

		processedCount, err := p.ProcessBatch(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			p.logger.Error("outbox batch processing error", slog.String("error", err.Error()))
		}

		if processedCount == 0 || err != nil {
			select {
			case <-ctx.Done():
				return nil
			case <-ticker.C:
			}
		}
	}
}

func (p *Processor) ProcessBatch(ctx context.Context) (int, error) {
	claimID := newClaimID()
	claimedIDs, err := p.store.ClaimBatch(ctx, p.db, claimID, p.cfg.OutboxBatchSize, p.cfg.OutboxClaimLeaseDuration)
	if err != nil {
		return 0, fmt.Errorf("claim batch: %w", err)
	}
	if len(claimedIDs) == 0 {
		return 0, nil
	}

	p.logger.Info("claimed outbox batch",
		slog.String("claim_id", claimID),
		slog.Int("batch_size", len(claimedIDs)),
	)

	batchStartTime := time.Now()

	for _, eventID := range claimedIDs {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		default:
		}

		if time.Since(batchStartTime) >= (p.cfg.OutboxClaimLeaseDuration - p.cfg.OutboxClaimRenewalMargin) {
			if err := p.store.RenewBatchNearExpiry(ctx, p.db, claimedIDs, claimID, p.cfg.OutboxClaimLeaseDuration); err != nil {
				p.logger.Warn("batch lease renewal failed", slog.String("claim_id", claimID), slog.String("error", err.Error()))
			} else {
				batchStartTime = time.Now()
			}
		}

		envelope, err := p.store.RenewAndLoadEvent(ctx, p.db, eventID, claimID, p.cfg.OutboxClaimLeaseDuration)
		if err != nil {
			if errors.Is(err, ErrClaimLost) {
				p.logger.Warn("outbox claim lost before publish",
					slog.String("event_id", eventID),
					slog.String("claim_id", claimID),
				)
				continue
			}
			p.logger.Error("failed to renew and load outbox event",
				slog.String("event_id", eventID),
				slog.String("error", err.Error()),
			)
			continue
		}

		exchange, routingKey, err := ResolveRoutingKey(envelope.EventType)
		if err != nil {
			p.logger.Error("failed to resolve routing key",
				slog.String("event_id", eventID),
				slog.String("event_type", envelope.EventType),
				slog.String("error", err.Error()),
			)
			_, _ = p.store.ReleaseWithBackoff(ctx, p.db, eventID, claimID)
			continue
		}

		pubCtx, cancel := context.WithTimeout(ctx, p.cfg.RabbitMQPublishConfirmTimeout)
		pubErr := p.publisher.PublishEvent(pubCtx, exchange, routingKey, *envelope)
		cancel()

		if pubErr != nil {
			p.logger.Error("outbox publish failed",
				slog.String("event_id", eventID),
				slog.String("event_type", envelope.EventType),
				slog.String("error", pubErr.Error()),
			)
			released, relErr := p.store.ReleaseWithBackoff(ctx, p.db, eventID, claimID)
			if relErr != nil {
				p.logger.Error("failed to release outbox event with backoff",
					slog.String("event_id", eventID),
					slog.String("error", relErr.Error()),
				)
			} else if !released {
				p.logger.Warn("stale release with backoff (claim lost)",
					slog.String("event_id", eventID),
					slog.String("claim_id", claimID),
				)
			}
			continue
		}

		marked, markErr := p.store.MarkPublished(ctx, p.db, eventID, claimID)
		if markErr != nil {
			p.logger.Error("failed to mark outbox event published",
				slog.String("event_id", eventID),
				slog.String("error", markErr.Error()),
			)
		} else if !marked {
			p.logger.Warn("stale mark published (claim lost or already published)",
				slog.String("event_id", eventID),
				slog.String("claim_id", claimID),
			)
		} else {
			p.logger.Info("outbox event published",
				slog.String("event_id", eventID),
				slog.String("event_type", envelope.EventType),
			)
		}
	}

	return len(claimedIDs), nil
}

func newClaimID() string {
	var b [16]byte
	_, err := io.ReadFull(rand.Reader, b[:])
	if err != nil {
		return fmt.Sprintf("claim-%d", time.Now().UnixNano())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
