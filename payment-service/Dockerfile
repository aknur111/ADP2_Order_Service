FROM golang:1.24-alpine AS builder

WORKDIR /workspace

COPY my-user-service-gen /workspace/my-user-service-gen
COPY payment-service /workspace/payment-service

WORKDIR /workspace/payment-service
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /payment-service ./cmd/payment-service

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /payment-service .
EXPOSE 8081 50051
CMD ["/app/payment-service"]
