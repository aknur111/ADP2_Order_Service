package httptransport

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func RateLimiterMiddleware(rdb *redis.Client, maxRequests int, windowSeconds int) gin.HandlerFunc {
	window := time.Duration(windowSeconds) * time.Second
	return func(c *gin.Context) {
		ip := c.ClientIP()
		key := fmt.Sprintf("rate_limit:%s", ip)
		ctx := context.Background()

		count, err := rdb.Incr(ctx, key).Result()
		if err != nil {
			slog.Error("rate limiter redis error", "error", err)
			c.Next()
			return
		}

		if count == 1 {
			rdb.Expire(ctx, key, window)
		}

		if count > int64(maxRequests) {
			slog.Warn("rate limit exceeded", "ip", ip, "count", count, "limit", maxRequests)
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "rate limit exceeded",
				"code":  "RATE_LIMIT_EXCEEDED",
			})
			return
		}

		slog.Info("rate limit allowed", "ip", ip, "count", count, "limit", maxRequests)
		c.Next()
	}
}
