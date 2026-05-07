package provider

import (
	"context"
	"notification-service/internal/domain"
)

type EmailSender interface {
	SendPaymentCompleted(ctx context.Context, event domain.PaymentEvent) error
}
