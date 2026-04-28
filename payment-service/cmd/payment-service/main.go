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

	"payment-service/internal/app"
	"payment-service/internal/repository"
	grpctransport "payment-service/internal/transport/grpc"
	httptransport "payment-service/internal/transport/http"
	"payment-service/internal/transport/mq"
	"payment-service/internal/usecase"

	paymentv1 "github.com/aknur111/my-user-service-gen/service/payment/v1"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"

	"google.golang.org/grpc"
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		slog.Error("failed to ping db", "error", err)
		os.Exit(1)
	}

	paymentRepo := repository.NewPostgresPaymentRepository(db)
	if err := paymentRepo.EnsureSchema(ctx); err != nil {
		slog.Error("failed to ensure schema", "error", err)
		os.Exit(1)
	}

	var publisher usecase.EventPublisher
	if cfg.RabbitMQURL != "" {
		pub, err := mq.NewRabbitPublisher(cfg.RabbitMQURL)
		if err != nil {
			slog.Error("failed to connect RabbitMQ publisher", "error", err)
		} else {
			publisher = pub
			defer pub.Close()
			slog.Info("RabbitMQ publisher connected")
		}
	}

	paymentUC := usecase.NewPaymentUsecase(paymentRepo, publisher)

	grpcHandler := grpctransport.NewPaymentGRPCHandler(paymentUC)

	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(grpctransport.LoggingInterceptor),
	)
	paymentv1.RegisterPaymentServiceServer(grpcServer, grpcHandler)

	grpcLis, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		slog.Error("failed to listen on gRPC address", "addr", cfg.GRPCAddr, "error", err)
		os.Exit(1)
	}

	go func() {
		slog.Info("payment gRPC server started", "addr", cfg.GRPCAddr)
		if err := grpcServer.Serve(grpcLis); err != nil {
			slog.Error("gRPC server error", "error", err)
			os.Exit(1)
		}
	}()

	httpHandler := httptransport.NewHandler(paymentUC)

	r := gin.New()
	r.Use(gin.Logger())
	r.Use(gin.Recovery())
	r.Use(httptransport.RequestIDMiddleware())

	httpHandler.Register(r)

	httpSrv := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: r,
	}

	go func() {
		slog.Info("payment HTTP server started", "addr", cfg.HTTPAddr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down payment service...")
	grpcServer.GracefulStop()

	ctxShutdown, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()

	if err := httpSrv.Shutdown(ctxShutdown); err != nil {
		slog.Error("HTTP shutdown failed", "error", err)
	}

	slog.Info("payment service exited properly")
}
