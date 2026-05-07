package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"order-service/internal/domain"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisOrderCache struct {
	client *redis.Client
}

func NewRedisOrderCache(client *redis.Client) *RedisOrderCache {
	return &RedisOrderCache{client: client}
}

func orderKey(id string) string {
	return fmt.Sprintf("order:%s", id)
}

func (c *RedisOrderCache) GetOrder(ctx context.Context, id string) (*domain.Order, error) {
	val, err := c.client.Get(ctx, orderKey(id)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			slog.Info("cache miss", "order_id", id)
			return nil, nil
		}
		return nil, fmt.Errorf("redis get: %w", err)
	}
	var order domain.Order
	if err := json.Unmarshal([]byte(val), &order); err != nil {
		return nil, fmt.Errorf("unmarshal order: %w", err)
	}
	slog.Info("cache hit", "order_id", id)
	return &order, nil
}

func (c *RedisOrderCache) SetOrder(ctx context.Context, order *domain.Order, ttl time.Duration) error {
	data, err := json.Marshal(order)
	if err != nil {
		return fmt.Errorf("marshal order: %w", err)
	}
	if err := c.client.Set(ctx, orderKey(order.ID), data, ttl).Err(); err != nil {
		return fmt.Errorf("redis set: %w", err)
	}
	slog.Info("cache set", "order_id", order.ID, "ttl_seconds", ttl.Seconds())
	return nil
}

func (c *RedisOrderCache) DeleteOrder(ctx context.Context, id string) error {
	if err := c.client.Del(ctx, orderKey(id)).Err(); err != nil {
		return fmt.Errorf("redis del: %w", err)
	}
	slog.Info("cache invalidated", "order_id", id)
	return nil
}
