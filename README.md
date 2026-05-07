# ADP2 — Order Service (Assignments 2, 3 & 4)

## Repository Links

| Repository | Purpose | URL |
|---|---|---|
| **Proto Repository** | Source `.proto` files | `https://github.com/aknur111/my-user-service-protos` |
| **Generated Code Repository** | Auto-generated `.pb.go` files (v1.2.0) | `https://github.com/aknur111/my-user-service-gen` |

---

## Assignment 4 — Performance Optimization & External Integrations

### What was added on top of Assignment 3

| Component | Assignment 3 | Assignment 4 |
|---|---|---|
| Order GET | always queries Postgres | **Redis cache-aside, TTL 300 s** |
| Cache invalidation | — | **DEL on status change (paid, failed, cancelled)** |
| Rate limiter | — | **Redis counter per IP, 10 req / 60 s, HTTP 429** |
| Notification idempotency | Postgres `processed_events` table | **Redis key `notification:processed:{event_id}`** |
| Notification retries | RabbitMQ requeue loop | **exponential backoff in worker (2 s, 4 s, 8 s)** |
| Provider abstraction | inline log statement | **`EmailSender` interface + `SimulatedEmailSender`** |
| DLQ | requeue × 3 then NACK | **worker exhausts retries, consumer NACKs to DLQ** |
| Redis | — | **Redis 7 container, exposed on localhost:6379** |

---

### Redis Cache-aside Pattern (Order Service)

Every `GET /orders/:id` follows the cache-aside pattern:

1. Check Redis key `order:{order_id}`.
2. **Cache hit** → return the cached JSON order immediately (logs `cache hit`).
3. **Cache miss** → query Postgres (logs `cache miss`), store result in Redis with TTL, return order.

**Cache key format:** `order:{order_id}`
**TTL:** configurable via `ORDER_CACHE_TTL_SECONDS` (default 300 s).

**Cache invalidation** happens immediately after any `UpdateStatus` call:
- After payment (Pending → Paid or Failed): `DEL order:{id}`
- After cancel (Pending → Cancelled): `DEL order:{id}`

This prevents serving stale order status after a payment completes.

Cache operations are non-blocking: if Redis is unavailable, the service falls back to Postgres transparently.

---

### Redis Rate Limiter (Order Service)

A Redis-based rate limiter middleware limits requests by client IP using the INCR + EXPIRE pattern.

- **Config:** `RATE_LIMIT_ENABLED=true`, `RATE_LIMIT_REQUESTS=10`, `RATE_LIMIT_WINDOW_SECONDS=60`
- **Key:** `rate_limit:{ip}`
- **Behavior:** increments counter per IP; if count exceeds limit returns `HTTP 429 Too Many Requests`.
- Logs `rate limit allowed` or `rate limit exceeded` on each request.

---

### Provider Adapter Pattern (Notification Service)

The `EmailSender` interface decouples the RabbitMQ consumer from any specific provider:

```go
type EmailSender interface {
    SendPaymentCompleted(ctx context.Context, event domain.PaymentEvent) error
}
```

The `SimulatedEmailSender` (selected when `PROVIDER_MODE=SIMULATED`):
- Simulates network latency with `time.Sleep(50–250 ms)`.
- Simulates random transient failures (~20 % of requests).
- Always fails for `customer_email = fail@example.com`.
- Logs a success line on successful delivery.

The consumer calls the `NotificationWorker` which calls the `EmailSender`; no provider-specific code lives in the consumer.

---

### Reliable Background Jobs (Notification Service)

**Flow:** RabbitMQ Consumer → NotificationWorker → EmailSender Adapter

**Idempotency via Redis:**

Before sending, the worker checks `notification:processed:{event_id}` in Redis.
- Key exists → duplicate, skip send, ACK message.
- Key absent → proceed with send.
After successful send, the worker writes the key with TTL (`NOTIFICATION_IDEMPOTENCY_TTL_SECONDS=86400`).

The existing Postgres `processed_events` table is retained from Assignment 3.

**Retry policy with exponential backoff:**

| Retry | Wait before attempt |
|---|---|
| 1 | 2 s |
| 2 | 4 s |
| 3 | 8 s |

Formula: `backoffBase * 2^(retry-1)`, configurable via `NOTIFICATION_BACKOFF_BASE_SECONDS` and `NOTIFICATION_MAX_RETRIES`.

**ACK/NACK rules:**
- ACK only after successful send AND `MarkProcessed` succeeds.
- Duplicate event: ACK without sending.
- All retries exhausted: NACK without requeue → message routes to `payment.completed.dlq`.

---

### Architecture Diagram (Assignment 4)

See [`image/assignment4-diagram.md`](image/assignment4-diagram.md) for the Mermaid diagram.

---

### How to Run (Assignment 4)

```bash
docker compose down -v
docker compose up --build
```

Check all containers are running:
```bash
docker compose ps
```

Services:

| Service | Address |
|---|---|
| order-service HTTP | `localhost:8080` |
| order-service gRPC | `localhost:50052` |
| payment-service HTTP | `localhost:8081` |
| payment-service gRPC | `localhost:50051` |
| notification-service | (consumer only) |
| RabbitMQ AMQP | `localhost:5672` |
| RabbitMQ Management UI | `localhost:15672` — **guest / guest** |
| Redis | `localhost:6379` |
| order-db | `localhost:5435` |
| payment-db | `localhost:5433` |
| notification-db | `localhost:5434` |

---

### Testing Cache Miss and Hit

Create an order:
```bash
curl -v -X POST http://localhost:8080/orders \
  -H "Content-Type: application/json" \
  -d '{"customer_id":"cust-1","item_name":"Laptop","amount":50000}'
```

Copy the `id` from the response. Then run GET twice:
```bash
curl -v http://localhost:8080/orders/<ORDER_ID>
curl -v http://localhost:8080/orders/<ORDER_ID>
```

Expected in order-service logs:
- First GET: `cache miss`
- Second GET: `cache hit`

---

### Checking Redis Keys

```bash
docker exec -it redis redis-cli
KEYS *
GET order:<ORDER_ID>
```

---

### Testing Notification Retry and DLQ

Send a payment with the always-failing email:
```bash
curl -v -X POST http://localhost:8081/payments \
  -H "Content-Type: application/json" \
  -d '{"order_id":"ord-fail-1","amount":5000,"customer_email":"fail@example.com"}'
```

Watch notification logs:
```bash
docker compose logs -f notification-service
```

Expected output:
```
[NotificationWorker] provider error, retrying  retry=1  backoff_seconds=2
[NotificationWorker] provider error, retrying  retry=2  backoff_seconds=4
[NotificationWorker] provider error, retrying  retry=3  backoff_seconds=8
[NotificationWorker] all retries exhausted
notification worker failed, routing to DLQ
```

Then open `http://localhost:15672` → Queues → `payment.completed.dlq` — you will see 1 message.

---

### Testing Idempotency

After a successful notification is processed, publish the same event again to RabbitMQ. The worker will:
1. Find `notification:processed:{event_id}` key in Redis.
2. Log `duplicate event, skipping`.
3. ACK without sending again.

---

### Environment Variables Reference (Assignment 4 additions)

#### Order Service
| Variable | Default | Description |
|---|---|---|
| `REDIS_ADDR` | `localhost:6379` | Redis address |
| `ORDER_CACHE_TTL_SECONDS` | `300` | Cache TTL for order objects |
| `RATE_LIMIT_ENABLED` | `false` | Enable rate limiter middleware |
| `RATE_LIMIT_REQUESTS` | `10` | Max requests per window per IP |
| `RATE_LIMIT_WINDOW_SECONDS` | `60` | Rate limit window duration |

#### Notification Service
| Variable | Default | Description |
|---|---|---|
| `REDIS_ADDR` | `localhost:6379` | Redis address |
| `NOTIFICATION_IDEMPOTENCY_TTL_SECONDS` | `86400` | TTL for processed event keys |
| `NOTIFICATION_MAX_RETRIES` | `3` | Max retries on provider failure |
| `NOTIFICATION_BACKOFF_BASE_SECONDS` | `2` | Base seconds for exponential backoff |
| `PROVIDER_MODE` | `SIMULATED` | Email provider mode |

---

## Assignment 3 — Event-Driven Architecture (RabbitMQ)

### What was added on top of Assignment 2

| Component | Assignment 2 | Assignment 3 |
|---|---|---|
| Order → Payment | gRPC `ProcessPayment` | **unchanged** |
| Payment → downstream | nothing | **publishes `PaymentCompleted` event to RabbitMQ** |
| Notification | — | **new `notification-service` consumes events** |
| Delivery guarantee | — | **at-least-once, durable queue, persistent messages** |
| Idempotency | — | **`processed_events` Postgres table** |
| DLQ | — | **`payment.completed.dlq` via DLX after 3 failures** |

### Event Flow

```
Client
  │  POST /payments  (HTTP)
  ▼
Payment Service
  │  INSERT payments + customer_email
  │  PublishWithContext → publisher confirms
  ▼
RabbitMQ Exchange: payment.events (direct, durable)
  ▼
Queue: payment.completed  (durable, x-dead-letter-exchange=payment.dlx)
  ▼
Notification Service consumer  (manual ACK, QoS=1)
  │  1. Check processed_events (idempotency)
  │  2. Log: [Notification] Sent email to … for Order #…. Amount: $…
  │  3. msg.Ack()
  │
  └── On failure (fail@example.com): Nack requeue=true × 3 → Nack requeue=false
        ▼
      DLX: payment.dlx  →  Queue: payment.completed.dlq
```

Mermaid diagram including Notification service: ![Architecture Diagram](image/Screenshot%202026-04-28%20at%2023.18.23.png)
Also you can see there [`image/assignment3-diagram.md`](image/assignment3-diagram.md)

Original diagram: ![Architecture Diagram](image/Screenshot%202026-04-10%20at%2009.26.48.png)

---

## How to Run

### Prerequisites
- Docker & Docker Compose
- Go 1.23+
- `curl` (for testing)

### 1. Start everything

```bash
docker compose up --build
```

Services started:
| Service | Address |
|---|---|
| order-service HTTP | `localhost:8080` |
| order-service gRPC | `localhost:50052` |
| payment-service HTTP | `localhost:8081` |
| payment-service gRPC | `localhost:50051` |
| notification-service | (consumer only) |
| RabbitMQ AMQP | `localhost:5672` |
| RabbitMQ Management UI | `localhost:15672` — **guest / guest** |
| order-db | `localhost:5432` |
| payment-db | `localhost:5433` |
| notification-db | `localhost:5434` |

### 2. Test normal flow

```bash
curl -s -X POST http://localhost:8080/orders \
  -H "Content-Type: application/json" \
  -d '{"customer_id":"cust-1","item_name":"Laptop","amount":50000}' | jq .
```

Then watch notification logs:
```bash
docker compose logs -f notification-service
```

Expected output:
```
{"level":"INFO","msg":"[Notification] Sent email to customer@example.com for Order #<order-id>. Amount: $500.00"}
```

### 3. Test with explicit customer_email via Payment HTTP API

```bash
curl -s -X POST http://localhost:8081/payments \
  -H "Content-Type: application/json" \
  -d '{"order_id":"ord-abc","amount":9999,"customer_email":"alice@example.com"}' | jq .
```

Notification service will log:
```
[Notification] Sent email to alice@example.com for Order #ord-abc. Amount: $99.99
```

### 4. Test idempotency (duplicate events)

Send the same `order_id` twice (second will fail at DB due to unique constraint on payment-service), but if you manually re-publish the same `event_id` twice to RabbitMQ, the second delivery is silently ACKed:
```
[Notification] Duplicate event — skipping  event_id=<uuid>
```

### 5. Test Dead Letter Queue

Send a payment with `customer_email: fail@example.com`:
```bash
curl -s -X POST http://localhost:8081/payments \
  -H "Content-Type: application/json" \
  -d '{"order_id":"ord-fail-1","amount":5000,"customer_email":"fail@example.com"}' | jq .
```

Watch notification logs:
```bash
docker compose logs -f notification-service
```

After 3 attempts you will see:
```
{"level":"WARN","msg":"[Notification] Simulated failure","attempt":1,"max_retries":3}
{"level":"WARN","msg":"[Notification] Simulated failure","attempt":2,"max_retries":3}
{"level":"WARN","msg":"[Notification] Simulated failure","attempt":3,"max_retries":3}
{"level":"ERROR","msg":"[Notification] Max retries exceeded — routing to DLQ"}
```

Then open RabbitMQ UI → `http://localhost:15672` (guest/guest) → Queues → `payment.completed.dlq` — you will see 1 message queued there.

---

## Architecture Details

### How manual ACK works

The notification consumer starts with `autoAck=false`:
```go
ch.Consume(queue, "", false /* autoAck */, ...)
```
`msg.Ack(false)` is called **only after** the email log is successfully printed and the `processed_events` row is committed. If the service crashes before ACK, RabbitMQ re-delivers the message (at-least-once).

### How idempotency works

Before processing, the consumer runs:
```sql
INSERT INTO processed_events (event_id) VALUES ($1) ON CONFLICT (event_id) DO NOTHING
```
- **0 rows affected** → duplicate → `msg.Ack()` without resending email.
- **1 row affected** → new event → send email then `msg.Ack()`.

`event_id` is a UUID generated by payment-service at publish time. Even if RabbitMQ re-delivers, the second delivery is a no-op.

### How DLQ works

Queue topology:
```
payment.completed
  x-dead-letter-exchange:     payment.dlx
  x-dead-letter-routing-key:  payment.completed

payment.dlx (direct exchange)
  → payment.completed.dlq (durable)
```

When `msg.Nack(false, false)` (requeue=false) is called, the broker routes the message through `payment.dlx` into `payment.completed.dlq`. The consumer tracks per-event attempt counts in memory and requeues (requeue=true) up to `maxRetries=3` times; on the 3rd failure it nacks with requeue=false.

### RabbitMQ UI

- URL: `http://localhost:15672`
- Username: `guest`
- Password: `guest`

---

## Assignment 2 — gRPC Migration (preserved)

### What Changed from Assignment 1

| Component | Assignment 1 | Assignment 2 |
|---|---|---|
| Order → Payment call | HTTP REST | **gRPC** `ProcessPayment` |
| Payment Service transport | HTTP only | HTTP + **gRPC Server** `:50051` |
| Order Service transport | HTTP only | HTTP + **gRPC Streaming Server** `:50052` |

### Proto Contracts

```protobuf
// service/payment/v1/payment.proto
service PaymentService {
  rpc ProcessPayment(PaymentRequest) returns (PaymentResponse);
}

// service/order/v1/order.proto
service OrderService {
  rpc SubscribeToOrderUpdates(OrderRequest) returns (stream OrderStatusUpdate);
}
```

### Subscribe to real-time order updates

```bash
grpcurl -plaintext \
  -d '{"order_id":"<ORDER_ID>"}' \
  localhost:50052 \
  service.order.v1.OrderService/SubscribeToOrderUpdates
```

---

## Environment Variables Reference

### Order Service
| Variable | Default | Description |
|---|---|---|
| `HTTP_ADDR` | `:8080` | REST API listen address |
| `GRPC_ADDR` | `:50052` | gRPC streaming server address |
| `DB_DSN` | — | PostgreSQL connection string |
| `PAYMENT_GRPC_ADDR` | — | Payment Service gRPC address |
| `HTTP_TIMEOUT_SECONDS` | `5` | HTTP client timeout |
| `STREAM_POLL_INTERVAL_MS` | `500` | DB poll interval for streaming |

### Payment Service
| Variable | Default | Description |
|---|---|---|
| `HTTP_ADDR` | `:8081` | REST API listen address |
| `GRPC_ADDR` | `:50051` | gRPC server address |
| `DB_DSN` | — | PostgreSQL connection string |
| `RABBITMQ_URL` | — | RabbitMQ AMQP URL |

### Notification Service
| Variable | Default | Description |
|---|---|---|
| `DB_DSN` | — | PostgreSQL connection string |
| `RABBITMQ_URL` | — | RabbitMQ AMQP URL |

---

## Git Commands

```bash
git add .
git commit -m "Implement Assignment 3 event-driven notifications"
git push -u origin feature/assignment-3
```
