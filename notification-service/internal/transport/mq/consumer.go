package mq

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"notification-service/internal/domain"
	"notification-service/internal/usecase"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	mainExchange = "payment.events"
	dlxExchange  = "payment.dlx"
	mainQueue    = "payment.completed"
	dlqName      = "payment.completed.dlq"
	routingKey   = "payment.completed"
)

type Consumer struct {
	conn    *amqp.Connection
	channel *amqp.Channel
	worker  *usecase.NotificationWorker
}

func NewConsumer(url string, worker *usecase.NotificationWorker) (*Consumer, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("dial rabbitmq: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("open channel: %w", err)
	}

	if err := ch.Qos(1, 0, false); err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("set qos: %w", err)
	}

	if err := ch.ExchangeDeclare(dlxExchange, "direct", true, false, false, false, nil); err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("declare dlx exchange: %w", err)
	}

	if _, err := ch.QueueDeclare(dlqName, true, false, false, false, nil); err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("declare dlq: %w", err)
	}
	if err := ch.QueueBind(dlqName, routingKey, dlxExchange, false, nil); err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("bind dlq: %w", err)
	}

	if err := ch.ExchangeDeclare(mainExchange, "direct", true, false, false, false, nil); err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("declare main exchange: %w", err)
	}

	qArgs := amqp.Table{
		"x-dead-letter-exchange":    dlxExchange,
		"x-dead-letter-routing-key": routingKey,
	}
	if _, err := ch.QueueDeclare(mainQueue, true, false, false, false, qArgs); err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("declare main queue: %w", err)
	}
	if err := ch.QueueBind(mainQueue, routingKey, mainExchange, false, nil); err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("bind main queue: %w", err)
	}

	return &Consumer{
		conn:    conn,
		channel: ch,
		worker:  worker,
	}, nil
}

func (c *Consumer) Start(ctx context.Context) error {
	msgs, err := c.channel.Consume(
		mainQueue,
		"",
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return fmt.Errorf("start consume: %w", err)
	}

	slog.Info("notification consumer started", "queue", mainQueue)

	for {
		select {
		case <-ctx.Done():
			return nil
		case msg, ok := <-msgs:
			if !ok {
				return fmt.Errorf("rabbitmq channel closed unexpectedly")
			}
			c.processMessage(ctx, msg)
		}
	}
}

func (c *Consumer) processMessage(ctx context.Context, msg amqp.Delivery) {
	var event domain.PaymentEvent
	if err := json.Unmarshal(msg.Body, &event); err != nil {
		slog.Error("malformed event payload, routing to DLQ", "error", err)
		msg.Nack(false, false)
		return
	}

	if err := c.worker.Handle(ctx, event); err != nil {
		slog.Error("notification worker failed, routing to DLQ", "event_id", event.EventID, "error", err)
		msg.Nack(false, false)
		return
	}

	msg.Ack(false)
}

func (c *Consumer) Close() error {
	if err := c.channel.Close(); err != nil {
		return err
	}
	return c.conn.Close()
}
