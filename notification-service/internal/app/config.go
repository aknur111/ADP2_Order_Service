package app

import (
	"os"
	"strconv"
)

type Config struct {
	DBDSN                 string
	RabbitMQURL           string
	RedisAddr             string
	IdempotencyTTLSeconds int
	MaxRetries            int
	BackoffBaseSeconds    int
	ProviderMode          string
}

func LoadConfig() Config {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	idempotencyTTL := 86400
	if v := os.Getenv("NOTIFICATION_IDEMPOTENCY_TTL_SECONDS"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			idempotencyTTL = parsed
		}
	}

	maxRetries := 3
	if v := os.Getenv("NOTIFICATION_MAX_RETRIES"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			maxRetries = parsed
		}
	}

	backoffBase := 2
	if v := os.Getenv("NOTIFICATION_BACKOFF_BASE_SECONDS"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			backoffBase = parsed
		}
	}

	providerMode := os.Getenv("PROVIDER_MODE")
	if providerMode == "" {
		providerMode = "SIMULATED"
	}

	return Config{
		DBDSN:                 os.Getenv("DB_DSN"),
		RabbitMQURL:           os.Getenv("RABBITMQ_URL"),
		RedisAddr:             redisAddr,
		IdempotencyTTLSeconds: idempotencyTTL,
		MaxRetries:            maxRetries,
		BackoffBaseSeconds:    backoffBase,
		ProviderMode:          providerMode,
	}
}
