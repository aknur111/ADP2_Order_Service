package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"notification-service/internal/domain"
	"notification-service/internal/provider"
	"time"
)

type IdempotencyStore interface {
	IsProcessed(ctx context.Context, eventID string) (bool, error)
	MarkProcessed(ctx context.Context, eventID string) error
}

type NotificationWorker struct {
	sender      provider.EmailSender
	idempotency IdempotencyStore
	maxRetries  int
	backoffBase time.Duration
}

func NewNotificationWorker(
	sender provider.EmailSender,
	idempotency IdempotencyStore,
	maxRetries int,
	backoffBaseSec int,
) *NotificationWorker {
	return &NotificationWorker{
		sender:      sender,
		idempotency: idempotency,
		maxRetries:  maxRetries,
		backoffBase: time.Duration(backoffBaseSec) * time.Second,
	}
}

func (w *NotificationWorker) Handle(ctx context.Context, event domain.PaymentEvent) error {
	processed, err := w.idempotency.IsProcessed(ctx, event.EventID)
	if err != nil {
		slog.Warn("[NotificationWorker] idempotency check failed, proceeding with send", "event_id", event.EventID, "error", err)
	} else if processed {
		slog.Info("[NotificationWorker] duplicate event, skipping", "event_id", event.EventID)
		return nil
	}

	var lastErr error
	lastErr = w.sender.SendPaymentCompleted(ctx, event)
	if lastErr == nil {
		return w.markAndReturn(ctx, event.EventID)
	}

	for retry := 1; retry <= w.maxRetries; retry++ {
		backoff := w.backoffBase * (1 << uint(retry-1))
		slog.Warn("[NotificationWorker] provider error, retrying",
			"event_id", event.EventID,
			"retry", retry,
			"max_retries", w.maxRetries,
			"backoff_seconds", backoff.Seconds(),
			"error", lastErr,
		)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
		lastErr = w.sender.SendPaymentCompleted(ctx, event)
		if lastErr == nil {
			return w.markAndReturn(ctx, event.EventID)
		}
	}

	slog.Error("[NotificationWorker] all retries exhausted",
		"event_id", event.EventID,
		"error", lastErr,
	)
	return fmt.Errorf("all retries exhausted for event %s: %w", event.EventID, lastErr)
}

func (w *NotificationWorker) markAndReturn(ctx context.Context, eventID string) error {
	if err := w.idempotency.MarkProcessed(ctx, eventID); err != nil {
		slog.Error("[NotificationWorker] failed to mark processed", "event_id", eventID, "error", err)
		return fmt.Errorf("mark processed: %w", err)
	}
	return nil
}
