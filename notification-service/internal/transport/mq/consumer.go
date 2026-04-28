package mq

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"notification-service/internal/domain"
	"notification-service/internal/repository"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	mainExchange = "payment.events"
	dlxExchange  = "payment.dlx"
	mainQueue    = "payment.completed"
	dlqName      = "payment.completed.dlq"
	routingKey   = "payment.completed"
	maxRetries   = 3
)

type Consumer struct {
	conn    *amqp.Connection
	channel *amqp.Channel
	repo    *repository.PostgresEventRepository

	mu       sync.Mutex
	attempts map[string]int
}

func NewConsumer(url string, repo *repository.PostgresEventRepository) (*Consumer, error) {
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
		conn:     conn,
		channel:  ch,
		repo:     repo,
		attempts: make(map[string]int),
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
		slog.Error("malformed event payload, sending to DLQ", "error", err)
		msg.Nack(false, false)
		return
	}

	if event.CustomerEmail == "fail@example.com" {
		c.mu.Lock()
		c.attempts[event.EventID]++
		attempt := c.attempts[event.EventID]
		c.mu.Unlock()

		slog.Warn("[Notification] Simulated failure",
			"event_id", event.EventID,
			"attempt", attempt,
			"max_retries", maxRetries,
		)

		if attempt < maxRetries {
			msg.Nack(false, true)
		} else {
			slog.Error("[Notification] Max retries exceeded — routing to DLQ",
				"event_id", event.EventID,
			)
			c.mu.Lock()
			delete(c.attempts, event.EventID)
			c.mu.Unlock()
			msg.Nack(false, false)
		}
		return
	}

	inserted, err := c.repo.MarkProcessed(ctx, event.EventID)
	if err != nil {
		slog.Error("idempotency DB error, requeuing", "event_id", event.EventID, "error", err)
		msg.Nack(false, true)
		return
	}
	if !inserted {
		slog.Info("[Notification] Duplicate event — skipping", "event_id", event.EventID)
		msg.Ack(false)
		return
	}

	slog.Info(fmt.Sprintf(
		"[Notification] Sent email to %s for Order #%s. Amount: $%.2f",
		event.CustomerEmail,
		event.OrderID,
		float64(event.Amount)/100.0,
	))

	msg.Ack(false) 
}

func (c *Consumer) Close() error {
	if err := c.channel.Close(); err != nil {
		return err
	}
	return c.conn.Close()
}
