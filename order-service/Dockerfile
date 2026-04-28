FROM golang:1.24-alpine AS builder

WORKDIR /workspace
COPY my-user-service-gen /workspace/my-user-service-gen
COPY order-service /workspace/order-service

WORKDIR /workspace/order-service
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /order-service ./cmd/order-service

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /order-service .
EXPOSE 8080 50052
CMD ["/app/order-service"]
