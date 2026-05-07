package main

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"notification-service/internal/app"
	"notification-service/internal/cache"
	"notification-service/internal/provider"
	"notification-service/internal/repository"
	"notification-service/internal/transport/mq"
	"notification-service/internal/usecase"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
)

func main() {
	if err := godotenv.Load(); err != nil {
		slog.Warn("no .env file found, using OS environment variables")
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg := app.LoadConfig()

	db, err := sql.Open("postgres", cfg.DBDSN)
	if err != nil {
		slog.Error("failed to open db", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	initCtx, initCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer initCancel()

	if err := db.PingContext(initCtx); err != nil {
		slog.Error("failed to ping db", "error", err)
		os.Exit(1)
	}

	eventRepo := repository.NewPostgresEventRepository(db)
	if err := eventRepo.EnsureSchema(initCtx); err != nil {
		slog.Error("failed to ensure schema", "error", err)
		os.Exit(1)
	}

	rdb := redis.NewClient(&redis.Options{
		Addr: cfg.RedisAddr,
	})
	defer rdb.Close()

	pingCtx, pingCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer pingCancel()
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		slog.Warn("redis ping failed, idempotency store may be unavailable", "error", err)
	} else {
		slog.Info("redis connected", "addr", cfg.RedisAddr)
	}

	idempotencyTTL := time.Duration(cfg.IdempotencyTTLSeconds) * time.Second
	idempotencyStore := cache.NewRedisIdempotencyStore(rdb, idempotencyTTL)

	var emailSender provider.EmailSender
	switch cfg.ProviderMode {
	default:
		slog.Info("using simulated email provider")
		emailSender = provider.NewSimulatedEmailSender()
	}

	worker := usecase.NewNotificationWorker(emailSender, idempotencyStore, cfg.MaxRetries, cfg.BackoffBaseSeconds)

	consumer, err := mq.NewConsumer(cfg.RabbitMQURL, worker)
	if err != nil {
		slog.Error("failed to create RabbitMQ consumer", "error", err)
		os.Exit(1)
	}
	defer consumer.Close()

	ctx, cancel := context.WithCancel(context.Background())

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-quit
		slog.Info("shutting down notification service...")
		cancel()
	}()

	if err := consumer.Start(ctx); err != nil {
		slog.Error("consumer stopped with error", "error", err)
		os.Exit(1)
	}

	slog.Info("notification service exited properly")
}
