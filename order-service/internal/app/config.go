package app

import (
	"os"
	"strconv"
)

type Config struct {
	HTTPAddr             string
	GRPCAddr             string
	DBDSNorder           string
	PaymentGRPCAddr      string
	HTTPTimeoutSeconds   int
	StreamPollIntervalMs int
	RedisAddr            string
	CacheTTLSeconds      int
	RateLimitEnabled     bool
	RateLimitRequests    int
	RateLimitWindowSec   int
}

func LoadConfig() Config {
	timeoutSeconds := 5
	if v := os.Getenv("HTTP_TIMEOUT_SECONDS"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			timeoutSeconds = parsed
		}
	}

	pollIntervalMs := 500
	if v := os.Getenv("STREAM_POLL_INTERVAL_MS"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			pollIntervalMs = parsed
		}
	}

	httpAddr := os.Getenv("HTTP_ADDR")
	if httpAddr == "" {
		httpAddr = ":8080"
	}

	grpcAddr := os.Getenv("GRPC_ADDR")
	if grpcAddr == "" {
		grpcAddr = ":50052"
	}

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	cacheTTL := 300
	if v := os.Getenv("ORDER_CACHE_TTL_SECONDS"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			cacheTTL = parsed
		}
	}

	rateLimitEnabled := os.Getenv("RATE_LIMIT_ENABLED") == "true"

	rateLimitRequests := 10
	if v := os.Getenv("RATE_LIMIT_REQUESTS"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			rateLimitRequests = parsed
		}
	}

	rateLimitWindowSec := 60
	if v := os.Getenv("RATE_LIMIT_WINDOW_SECONDS"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			rateLimitWindowSec = parsed
		}
	}

	return Config{
		HTTPAddr:             httpAddr,
		GRPCAddr:             grpcAddr,
		DBDSNorder:           os.Getenv("DB_DSN"),
		PaymentGRPCAddr:      os.Getenv("PAYMENT_GRPC_ADDR"),
		HTTPTimeoutSeconds:   timeoutSeconds,
		StreamPollIntervalMs: pollIntervalMs,
		RedisAddr:            redisAddr,
		CacheTTLSeconds:      cacheTTL,
		RateLimitEnabled:     rateLimitEnabled,
		RateLimitRequests:    rateLimitRequests,
		RateLimitWindowSec:   rateLimitWindowSec,
	}
}
