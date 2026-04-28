package grpctransport

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"payment-service/internal/domain"
	"payment-service/internal/usecase"

	paymentv1 "github.com/aknur111/my-user-service-gen/service/payment/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type PaymentGRPCHandler struct {
	paymentv1.UnimplementedPaymentServiceServer
	uc *usecase.PaymentUsecase
}

func NewPaymentGRPCHandler(uc *usecase.PaymentUsecase) *PaymentGRPCHandler {
	return &PaymentGRPCHandler{uc: uc}
}

func (h *PaymentGRPCHandler) ProcessPayment(
	ctx context.Context,
	req *paymentv1.PaymentRequest,
) (*paymentv1.PaymentResponse, error) {
	slog.Info("gRPC ProcessPayment called",
		"order_id", req.GetOrderId(),
		"amount", req.GetAmount(),
	)

	if req.GetOrderId() == "" {
		return nil, status.Error(codes.InvalidArgument, "order_id is required")
	}
	if req.GetAmount() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "amount must be greater than 0")
	}

	payment, err := h.uc.CreatePayment(ctx, req.GetOrderId(), req.GetAmount(), "customer@example.com")
	if err != nil {
		if errors.Is(err, domain.ErrInvalidAmount) {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		slog.Error("ProcessPayment use case error", "error", err)
		return nil, status.Error(codes.Internal, "internal payment processing error")
	}

	slog.Info("gRPC ProcessPayment completed",
		"payment_id", payment.ID,
		"status", payment.Status,
	)

	return &paymentv1.PaymentResponse{
		TransactionId: payment.TransactionID,
		Status:        string(payment.Status),
		PaymentId:     payment.ID,
		CreatedAt:     timestamppb.New(time.Now().UTC()),
	}, nil
}
