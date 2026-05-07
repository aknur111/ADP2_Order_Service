package main

import (
	"context"
	"database/sql"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"order-service/internal/app"
	"order-service/internal/cache"
	"order-service/internal/client"
	"order-service/internal/repository"
	grpctransport "order-service/internal/transport/grpc"
	httptransport "order-service/internal/transport/http"
	"order-service/internal/usecase"

	orderv1 "github.com/aknur111/my-user-service-gen/service/order/v1"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"

	"google.golang.org/grpc"
)

func main() {
	if err := godotenv.Load(); err != nil {
		slog.Warn("no .env file found, using OS environment variables")
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg := app.LoadConfig()

	db, err := sql.Open("postgres", cfg.DBDSNorder)
	if err != nil {
		slog.Error("failed to open db", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		slog.Error("ping db", "error", err)
		os.Exit(1)
	}

	rdb := redis.NewClient(&redis.Options{
		Addr: cfg.RedisAddr,
	})
	defer rdb.Close()

	pingCtx, pingCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer pingCancel()
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		slog.Warn("redis ping failed, cache will be unavailable", "error", err)
	} else {
		slog.Info("redis connected", "addr", cfg.RedisAddr)
	}

	orderCache := cache.NewRedisOrderCache(rdb)

	orderRepo := repository.NewPostgresOrderRepository(db)
	if err := orderRepo.EnsureSchema(ctx); err != nil {
		slog.Error("ensure schema", "error", err)
		os.Exit(1)
	}

	paymentGRPCClient, err := client.NewGRPCPaymentClient(cfg.PaymentGRPCAddr)
	if err != nil {
		slog.Error("failed to create gRPC payment client", "error", err)
		os.Exit(1)
	}
	defer paymentGRPCClient.Close()

	cacheTTL := time.Duration(cfg.CacheTTLSeconds) * time.Second
	orderUC := usecase.NewOrderUsecase(orderRepo, paymentGRPCClient, orderCache, cacheTTL)
	grpcHandler := grpctransport.NewOrderGRPCHandler(db, cfg.StreamPollIntervalMs)

	grpcServer := grpc.NewServer()
	orderv1.RegisterOrderServiceServer(grpcServer, grpcHandler)

	grpcLis, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		slog.Error("failed to listen gRPC", "addr", cfg.GRPCAddr, "error", err)
		os.Exit(1)
	}

	go func() {
		slog.Info("order gRPC server started", "addr", cfg.GRPCAddr)
		if err := grpcServer.Serve(grpcLis); err != nil {
			slog.Error("gRPC server error", "error", err)
			os.Exit(1)
		}
	}()

	httpHandler := httptransport.NewHandler(orderUC)

	r := gin.New()
	r.Use(gin.Logger())
	r.Use(gin.Recovery())
	r.Use(httptransport.RequestIDMiddleware())

	if cfg.RateLimitEnabled {
		slog.Info("rate limiter enabled", "requests", cfg.RateLimitRequests, "window_seconds", cfg.RateLimitWindowSec)
		r.Use(httptransport.RateLimiterMiddleware(rdb, cfg.RateLimitRequests, cfg.RateLimitWindowSec))
	}

	httpHandler.Register(r)

	httpSrv := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: r,
	}

	go func() {
		slog.Info("order HTTP server started", "addr", cfg.HTTPAddr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP listen error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down order service...")
	grpcServer.GracefulStop()

	ctxShutdown, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()

	if err := httpSrv.Shutdown(ctxShutdown); err != nil {
		slog.Error("HTTP shutdown failed", "error", err)
	}

	slog.Info("order service exited properly")
}
