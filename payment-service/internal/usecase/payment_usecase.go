package usecase

import (
	"context"
	"log/slog"
	"payment-service/internal/domain"
	"time"

	"github.com/google/uuid"
)

type PaymentUsecase struct {
	repo      PaymentRepository
	publisher EventPublisher
}

func NewPaymentUsecase(repo PaymentRepository, publisher EventPublisher) *PaymentUsecase {
	return &PaymentUsecase{repo: repo, publisher: publisher}
}

func (u *PaymentUsecase) CreatePayment(ctx context.Context, orderID string, amount int64, customerEmail string) (*domain.Payment, error) {
	if amount <= 0 {
		return nil, domain.ErrInvalidAmount
	}

	status := domain.PaymentStatusAuthorized
	if amount > 100000 {
		status = domain.PaymentStatusDeclined
	}

	payment := &domain.Payment{
		ID:            uuid.NewString(),
		OrderID:       orderID,
		TransactionID: uuid.NewString(),
		Amount:        amount,
		Status:        status,
		CustomerEmail: customerEmail,
	}

	if err := u.repo.Create(ctx, payment); err != nil {
		return nil, err
	}

	if u.publisher != nil {
		event := domain.PaymentEvent{
			EventID:       uuid.NewString(),
			OrderID:       payment.OrderID,
			Amount:        payment.Amount,
			CustomerEmail: payment.CustomerEmail,
			Status:        string(payment.Status),
			OccurredAt:    time.Now().UTC(),
		}
		if err := u.publisher.PublishPaymentCompleted(ctx, event); err != nil {
			slog.Error("failed to publish payment event", "order_id", orderID, "error", err)
		}
	}

	return payment, nil
}

func (u *PaymentUsecase) GetByOrderID(ctx context.Context, orderID string) (*domain.Payment, error) {
	return u.repo.GetByOrderID(ctx, orderID)
}
