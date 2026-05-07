package provider

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"notification-service/internal/domain"
	"time"
)

type SimulatedEmailSender struct{}

func NewSimulatedEmailSender() *SimulatedEmailSender {
	return &SimulatedEmailSender{}
}

func (s *SimulatedEmailSender) SendPaymentCompleted(ctx context.Context, event domain.PaymentEvent) error {
	time.Sleep(time.Duration(50+rand.Intn(200)) * time.Millisecond)

	if event.CustomerEmail == "fail@example.com" {
		return errors.New("simulated permanent failure for fail@example.com")
	}

	if rand.Intn(10) < 2 {
		return fmt.Errorf("simulated transient provider error for order %s", event.OrderID)
	}

	slog.Info("[SimulatedEmailSender] email sent",
		"to", event.CustomerEmail,
		"order_id", event.OrderID,
		"amount", fmt.Sprintf("$%.2f", float64(event.Amount)/100.0),
	)
	return nil
}
