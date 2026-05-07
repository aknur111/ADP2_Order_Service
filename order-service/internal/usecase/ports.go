package usecase

import (
	"context"
	"order-service/internal/domain"
	"time"
)

type OrderRepository interface {
	Create(ctx context.Context, order *domain.Order) error
	GetByID(ctx context.Context, id string) (*domain.Order, error)
	UpdateStatus(ctx context.Context, id string, status domain.OrderStatus) error
	EnsureSchema(ctx context.Context) error
	GetByIdempotencyKey(ctx context.Context, key string) (*domain.Order, error)
}

type PaymentAuthorizer interface {
	Authorize(ctx context.Context, orderID string, amount int64) (transactionID string, paymentStatus string, err error)
}

type OrderCache interface {
	GetOrder(ctx context.Context, id string) (*domain.Order, error)
	SetOrder(ctx context.Context, order *domain.Order, ttl time.Duration) error
	DeleteOrder(ctx context.Context, id string) error
}
