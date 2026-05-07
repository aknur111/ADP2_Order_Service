package cache

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

const idempotencyKeyPrefix = "notification:processed:"

type RedisIdempotencyStore struct {
	client *redis.Client
	ttl    time.Duration
}

func NewRedisIdempotencyStore(client *redis.Client, ttl time.Duration) *RedisIdempotencyStore {
	return &RedisIdempotencyStore{client: client, ttl: ttl}
}

func (s *RedisIdempotencyStore) IsProcessed(ctx context.Context, eventID string) (bool, error) {
	key := idempotencyKeyPrefix + eventID
	_, err := s.client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return false, nil
		}
		return false, fmt.Errorf("redis get idempotency: %w", err)
	}
	slog.Info("idempotency check: already processed", "event_id", eventID)
	return true, nil
}

func (s *RedisIdempotencyStore) MarkProcessed(ctx context.Context, eventID string) error {
	key := idempotencyKeyPrefix + eventID
	if err := s.client.Set(ctx, key, "1", s.ttl).Err(); err != nil {
		return fmt.Errorf("redis set idempotency: %w", err)
	}
	slog.Info("idempotency marked", "event_id", eventID)
	return nil
}
