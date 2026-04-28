package usecase

import (
	"context"
	"payment-service/internal/domain"
)

type PaymentRepository interface {
	Create(ctx context.Context, payment *domain.Payment) error
	GetByOrderID(ctx context.Context, orderID string) (*domain.Payment, error)
	EnsureSchema(ctx context.Context) error
}

type EventPublisher interface {
	PublishPaymentCompleted(ctx context.Context, event domain.PaymentEvent) error
	Close() error
}
