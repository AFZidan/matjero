package messaging

import (
	"context"
	"encoding/json"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"

	"matjero/packages/events"
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

func NewRabbitPublisher(channel *amqp.Channel) *RabbitPublisher {
	return &RabbitPublisher{channel: channel}
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

	return p.channel.PublishWithContext(ctx, publication.exchange, publication.routingKey, false, false, amqp.Publishing{
		ContentType:   "application/json",
		DeliveryMode:  amqp.Persistent,
		MessageId:     publication.messageID,
		Type:          publication.messageType,
		CorrelationId: publication.correlationID,
		Body:          body,
	})
}
