FROM golang:1.24-alpine AS builder

WORKDIR /workspace
COPY notification-service /workspace/notification-service

WORKDIR /workspace/notification-service
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /notification-service ./cmd/notification-service

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /notification-service .
CMD ["/app/notification-service"]
