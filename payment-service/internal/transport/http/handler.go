package httptransport

import (
	"errors"
	"log/slog"
	"net/http"
	"payment-service/internal/domain"
	"payment-service/internal/usecase"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	uc *usecase.PaymentUsecase
}

type createPaymentRequest struct {
	OrderID       string `json:"order_id" binding:"required"`
	Amount        int64  `json:"amount"`
	CustomerEmail string `json:"customer_email"`
}

type errorResponse struct {
	Error     string `json:"error"`
	Code      string `json:"code"`
	RequestID string `json:"request_id,omitempty"`
}

func NewHandler(uc *usecase.PaymentUsecase) *Handler {
	return &Handler{uc: uc}
}

func (h *Handler) Register(r *gin.Engine) {
	r.POST("/payments", h.CreatePayment)
	r.GET("/payments/:order_id", h.GetPaymentByOrderID)
	r.GET("/health", h.Health)
	r.GET("/ready", h.Ready)
}

func (h *Handler) respondError(c *gin.Context, statusCode int, err error, code string) {
	requestID := c.GetString("request_id")
	slog.Error("request failed",
		"request_id", requestID,
		"code", code,
		"error", err,
	)
	c.JSON(statusCode, errorResponse{
		Error:     err.Error(),
		Code:      code,
		RequestID: requestID,
	})
}

func (h *Handler) CreatePayment(c *gin.Context) {
	requestID := c.GetString("request_id")
	var req createPaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.respondError(c, http.StatusBadRequest, err, "INVALID_REQUEST")
		return
	}
	if req.CustomerEmail == "" {
		req.CustomerEmail = "customer@example.com"
	}
	slog.Info("create payment request", "request_id", requestID, "order_id", req.OrderID)

	payment, err := h.uc.CreatePayment(c.Request.Context(), req.OrderID, req.Amount, req.CustomerEmail)
	if err != nil {
		if errors.Is(err, domain.ErrInvalidAmount) {
			h.respondError(c, http.StatusBadRequest, err, "INVALID_AMOUNT")
			return
		}
		h.respondError(c, http.StatusInternalServerError, err, "INTERNAL_ERROR")
		return
	}
	c.JSON(http.StatusCreated, payment)
}

func (h *Handler) GetPaymentByOrderID(c *gin.Context) {
	payment, err := h.uc.GetByOrderID(c.Request.Context(), c.Param("order_id"))
	if err != nil {
		if errors.Is(err, domain.ErrPaymentNotFound) {
			h.respondError(c, http.StatusNotFound, err, "PAYMENT_NOT_FOUND")
			return
		}
		h.respondError(c, http.StatusInternalServerError, err, "INTERNAL_ERROR")
		return
	}
	c.JSON(http.StatusOK, payment)
}

func (h *Handler) Health(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) }
func (h *Handler) Ready(c *gin.Context)  { c.JSON(http.StatusOK, gin.H{"status": "ready"}) }
