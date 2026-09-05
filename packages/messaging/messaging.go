package messaging

import (
	"context"
	"encoding/json"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/matjeroapps/core/packages/events"
)

type Publisher interface {
	PublishEvent(ctx context.Context, exchange, routingKey string, event events.EventEnvelope) error
	PublishMessage(ctx context.Context, exchange, routingKey string, message events.MessageEnvelope) error
}

type RabbitPublisher struct {
	channel *amqp.Channel
}

type rabbitPublication struct {
	exchange      string
	routingKey    string
	messageID     string
	messageType   string
	correlationID string
	payload       any
}

func NewRabbitPublisher(channel *amqp.Channel) (*RabbitPublisher, error) {
	if channel == nil {
		return nil, fmt.Errorf("rabbitmq channel is required")
	}
	if err := channel.Confirm(false); err != nil {
		return nil, fmt.Errorf("enable channel confirm mode: %w", err)
	}
	return &RabbitPublisher{channel: channel}, nil
}

func (p *RabbitPublisher) PublishEvent(ctx context.Context, exchange, routingKey string, event events.EventEnvelope) error {
	if err := event.Validate(); err != nil {
		return err
	}
	return p.publish(ctx, rabbitPublication{
		exchange:      exchange,
		routingKey:    routingKey,
		messageID:     event.EventID,
		messageType:   event.EventType,
		correlationID: event.CorrelationID,
		payload:       event,
	})
}

func (p *RabbitPublisher) PublishMessage(ctx context.Context, exchange, routingKey string, message events.MessageEnvelope) error {
	if err := message.Validate(); err != nil {
		return err
	}
	return p.publish(ctx, rabbitPublication{
		exchange:      exchange,
		routingKey:    routingKey,
		messageID:     message.MessageID,
		messageType:   message.MessageType,
		correlationID: message.CorrelationID,
		payload:       message,
	})
}

func (p *RabbitPublisher) publish(ctx context.Context, publication rabbitPublication) error {
	if p.channel == nil {
		return fmt.Errorf("rabbitmq channel is required")
	}

	body, err := json.Marshal(publication.payload)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	confirmation, err := p.channel.PublishWithDeferredConfirmWithContext(
		ctx,
		publication.exchange,
		publication.routingKey,
		false,
		false,
		amqp.Publishing{
			ContentType:   "application/json",
			DeliveryMode:  amqp.Persistent,
			MessageId:     publication.messageID,
			Type:          publication.messageType,
			CorrelationId: publication.correlationID,
			Body:          body,
		},
	)
	if err != nil {
		return fmt.Errorf("publish message: %w", err)
	}

	if confirmation == nil {
		return fmt.Errorf("channel is not in confirm mode")
	}

	acked, err := confirmation.WaitContext(ctx)
	if err != nil {
		return fmt.Errorf("wait publish confirm: %w", err)
	}
	if !acked {
		return fmt.Errorf("rabbitmq publish nacked by broker for message_id %s", publication.messageID)
	}

	return nil
}
