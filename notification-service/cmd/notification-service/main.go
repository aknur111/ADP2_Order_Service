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
	"notification-service/internal/repository"
	"notification-service/internal/transport/mq"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
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

	consumer, err := mq.NewConsumer(cfg.RabbitMQURL, eventRepo)
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
