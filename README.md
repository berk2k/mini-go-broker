# Mini Go Broker

A learning-focused message broker written in Go to understand how modern messaging systems (e.g. RabbitMQ, Kafka consumers) work internally.

This project implements core messaging semantics from scratch to explore:

- Delivery guarantees
- Flow control (QoS / prefetch)
- Lease-based processing
- Failure isolation
- Consumer lifecycle management
- Controlled shutdown patterns

This is not intended to replace production brokers.  
The goal is to make distributed messaging mechanics explicit instead of hidden behind abstractions.

---

# Features

## Core Messaging Semantics

- At-least-once delivery
- Lease-based inflight tracking
- Visibility timeout
- Redelivery on failure
- Exponential backoff on retry
- Prefetch (per-consumer flow control)
- Ack / Nack support

## Failure Isolation

- Retry limit per message
- Dead Letter Queue (DLQ)
- Bounded DLQ (drop-oldest policy)

## Consumer Lifecycle

- Immediate inflight requeue on disconnect
- Graceful shutdown with draining
- Timeout-based forced requeue on shutdown

## Observability

- Structured JSON logging (slog)
- JSON metrics endpoint (`/metrics/json`)
- Prometheus-compatible metrics endpoint (`/metrics`)
- Ready / Inflight / DLQ gauges
- Delivery counters (published, acked, nacked, redelivered)
- Processing latency measurement (average)
- Total successfully processed messages

---

# How to Run

### Prerequisites

- Go 1.21+

### 1. Clone the repository

```bash
git clone https://github.com/berk2k/mini-go-broker.git
cd mini-go-broker
```

### 2. Configure (optional)

Copy the example env file and adjust values if needed:

```bash
cp .env.example .env
```

Default values work out of the box — no configuration required.

### 3. Run the broker

```bash
go run cmd/broker/main.go
```

Expected output:

```json
{"time":"...","level":"INFO","msg":"metrics_server_started","port":":8080"}
{"time":"...","level":"INFO","msg":"broker_started","port":":50051"}
```

---

# Configuration

All parameters are configurable via environment variables. Defaults are applied if not set.

| Variable | Default | Description |
|---|---|---|
| GRPC_PORT | :50051 | gRPC server listen address |
| METRICS_PORT | :8080 | HTTP metrics server address |
| MAX_RETRIES | 3 | Max delivery attempts before DLQ |
| MAX_DLQ_SIZE | 100 | Maximum DLQ capacity (drop-oldest) |
| VISIBILITY_TIMEOUT_SEC | 5 | Lease deadline in seconds |
| DRAIN_TIMEOUT_SEC | 10 | Graceful shutdown drain window |
| DEFAULT_PREFETCH | 1 | Default per-consumer prefetch limit |

---

# Usage Examples

### Publish a message

```bash
grpcurl -plaintext -d '{"payload": "aGVsbG8="}' \
  localhost:50051 broker.v1.BrokerService/Publish
```

### Consume messages (streaming)

```bash
grpcurl -plaintext -d '{"consumer_id": "consumer-1", "prefetch": 5}' \
  localhost:50051 broker.v1.BrokerService/Consume
```

### Ack a delivery

```bash
grpcurl -plaintext -d '{"delivery_id": "<deliveryID>", "consumer_id": "consumer-1"}' \
  localhost:50051 broker.v1.BrokerService/Ack
```

### Nack a delivery (requeue)

```bash
grpcurl -plaintext -d '{"delivery_id": "<deliveryID>", "consumer_id": "consumer-1", "requeue": true}' \
  localhost:50051 broker.v1.BrokerService/Nack
```

### Check metrics

```bash
# JSON snapshot
curl http://localhost:8080/metrics/json

# Prometheus format
curl http://localhost:8080/metrics
```

---

# Architecture

```
Producer
↓
gRPC Publish
↓
Ready Queue
↓ (lease)
Inflight Map
↓
Consumer (gRPC streaming)

Timeout / Nack → Ready or DLQ
```

---

# Delivery Model

Each delivery creates a lease:

- `messageID` → stable message identity
- `deliveryID` → ephemeral lease identity
- `deadline` → visibility timeout
- `attempt counter` → retry tracking

Lifecycle:

```
Ready → Inflight → Ack → Done
            ↓
          Timeout / Nack
            ↓
        Ready (with backoff)
            ↓
        MaxRetries → DLQ
```

The broker guarantees **at-least-once delivery**.

---

# Current Limitations

- In-memory only (no persistence)
- No partitioning
- No exchange/routing model
- No histogram-based latency buckets (average only)

See `DESIGN.md` for detailed trade-offs and architectural reasoning.

---

# Roadmap

- [x] Core delivery semantics
- [x] Retry limit + DLQ
- [x] Exponential backoff on retry
- [x] Consumer disconnect handling
- [x] Graceful shutdown (drain + timeout)
- [x] Observability / metrics
- [x] Structured logging (slog)
- [x] Environment-based configuration
- [ ] Optional persistence layer
- [ ] Per-consumer inflight index (O(k) disconnect)

---

# Learning Focus

This project explores:

- Lease-based message processing
- Backpressure and flow control
- Failure isolation patterns
- Distributed shutdown strategies
- Concurrency design in Go (`sync.Cond` vs channels)
- Structured observability

For detailed design decisions and trade-offs, see:

👉 `DESIGN.md`
