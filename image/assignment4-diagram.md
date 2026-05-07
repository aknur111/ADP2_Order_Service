# Assignment 4 — Performance Optimization & External Integrations Diagram

```mermaid
flowchart TD
    Client([Client / Browser])

    subgraph OrderSvc["Order Service :8080"]
        RL[Rate Limiter Middleware\nRedis counter / IP]
        OH[HTTP Handler]
        OUC[Order Usecase]
        OCache[Redis Cache\norder:{id} TTL 300s]
        OR[Repository]
        ODB[(order-db\nPostgres)]
    end

    subgraph Redis["Redis :6379"]
        RCache["Order Cache Keys\norder:{id}"]
        RRate["Rate Limit Keys\nrate_limit:{ip}"]
        RIdem["Idempotency Keys\nnotification:processed:{event_id}"]
    end

    subgraph PaymentSvc["Payment Service :8081 / :50051"]
        PH[HTTP + gRPC Handler]
        PUC[Use Case]
        PR[Repository]
        PDB[(payment-db\nPostgres)]
        PUB[RabbitMQ Publisher]
    end

    subgraph MQ["RabbitMQ :5672 / UI :15672"]
        EX["Exchange: payment.events\n(direct, durable)"]
        Q["Queue: payment.completed\n(durable, DLX → payment.dlx)"]
        DLX["DLX Exchange: payment.dlx"]
        DLQ["DLQ: payment.completed.dlq"]
    end

    subgraph NotifSvc["Notification Service"]
        CON[RabbitMQ Consumer\nmanual ACK / QoS=1]
        NW[Notification Worker\nexponential backoff retries]
        EA[EmailSender Adapter]
        SIM["SimulatedEmailSender\nlatency + random failures"]
    end

    Client -->|POST /orders| RL
    RL -->|allowed| OH
    RL -->|429 Too Many Requests| Client
    RL <-->|INCR rate_limit:ip| RRate

    OH --> OUC
    OUC <-->|GetOrder cache-aside| OCache
    OCache <-->|GET/SET/DEL order:id| RCache
    OUC -->|cache miss| OR --> ODB
    OUC -->|UpdateStatus → DEL cache| OCache
    OUC -->|gRPC ProcessPayment| PH

    PH --> PUC
    PUC --> PR --> PDB
    PUC --> PUB
    PUB -->|publish + confirm| EX
    EX --> Q

    Q -->|consume| CON
    CON --> NW
    NW <-->|IsProcessed / MarkProcessed| RIdem
    NW -->|send| EA
    EA --> SIM

    SIM -->|success| NW
    SIM -->|failure / fail@example.com| NW
    NW -->|all retries exhausted| CON
    CON -->|Nack requeue=false| DLX --> DLQ

    style DLQ fill:#f55,color:#fff
    style SIM fill:#5a5,color:#fff
    style RCache fill:#c33,color:#fff
    style RRate fill:#c33,color:#fff
    style RIdem fill:#c33,color:#fff
```
