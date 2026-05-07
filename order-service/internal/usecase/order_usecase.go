package usecase

import (
	"context"
	"log/slog"
	"order-service/internal/domain"
	"strings"
	"time"

	"github.com/google/uuid"
)

type OrderUsecase struct {
	repo          OrderRepository
	paymentClient PaymentAuthorizer
	cache         OrderCache
	cacheTTL      time.Duration
}

func NewOrderUsecase(repo OrderRepository, paymentClient PaymentAuthorizer, cache OrderCache, cacheTTL time.Duration) *OrderUsecase {
	return &OrderUsecase{repo: repo, paymentClient: paymentClient, cache: cache, cacheTTL: cacheTTL}
}

func (u *OrderUsecase) CreateOrder(
	ctx context.Context,
	customerID,
	itemName string,
	amount int64,
	idempotencyKey string,
) (*domain.Order, error) {
	if amount <= 0 {
		return nil, domain.ErrInvalidAmount
	}

	idempotencyKey = strings.TrimSpace(idempotencyKey)

	if idempotencyKey != "" {
		existing, err := u.repo.GetByIdempotencyKey(ctx, idempotencyKey)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			return existing, nil
		}
	}

	order := &domain.Order{
		ID:             uuid.NewString(),
		CustomerID:     customerID,
		ItemName:       itemName,
		Amount:         amount,
		Status:         domain.OrderStatusPending,
		CreatedAt:      time.Now().UTC(),
		IdempotencyKey: idempotencyKey,
	}

	if err := u.repo.Create(ctx, order); err != nil {
		return nil, err
	}

	_, paymentStatus, err := u.paymentClient.Authorize(ctx, order.ID, order.Amount)
	if err != nil {
		_ = u.repo.UpdateStatus(ctx, order.ID, domain.OrderStatusFailed)
		if delErr := u.cache.DeleteOrder(ctx, order.ID); delErr != nil {
			slog.Error("cache invalidation error", "order_id", order.ID, "error", delErr)
		}
		order.Status = domain.OrderStatusFailed
		return order, domain.ErrPaymentUnavailable
	}

	if paymentStatus == "Authorized" {
		order.Status = domain.OrderStatusPaid
	} else {
		order.Status = domain.OrderStatusFailed
	}

	if err := u.repo.UpdateStatus(ctx, order.ID, order.Status); err != nil {
		return nil, err
	}

	if delErr := u.cache.DeleteOrder(ctx, order.ID); delErr != nil {
		slog.Error("cache invalidation error", "order_id", order.ID, "error", delErr)
	}

	return order, nil
}

func (u *OrderUsecase) GetOrder(ctx context.Context, id string) (*domain.Order, error) {
	cached, cacheErr := u.cache.GetOrder(ctx, id)
	if cacheErr != nil {
		slog.Error("cache get error", "order_id", id, "error", cacheErr)
	} else if cached != nil {
		return cached, nil
	}

	order, err := u.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if setErr := u.cache.SetOrder(ctx, order, u.cacheTTL); setErr != nil {
		slog.Error("cache set error", "order_id", id, "error", setErr)
	}

	return order, nil
}

func (u *OrderUsecase) CancelOrder(ctx context.Context, id string) (*domain.Order, error) {
	order, err := u.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if order.Status == domain.OrderStatusPaid {
		return nil, domain.ErrCannotCancelPaid
	}
	if order.Status != domain.OrderStatusPending {
		return nil, domain.ErrCannotCancelStatus
	}

	if err := u.repo.UpdateStatus(ctx, id, domain.OrderStatusCancelled); err != nil {
		return nil, err
	}
	order.Status = domain.OrderStatusCancelled

	if delErr := u.cache.DeleteOrder(ctx, id); delErr != nil {
		slog.Error("cache invalidation error", "order_id", id, "error", delErr)
	}

	return order, nil
}
