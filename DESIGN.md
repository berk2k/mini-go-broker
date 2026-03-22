# Mini Go Broker – Design & Trade-offs

This document explains the internal design decisions, trade-offs, and alternative approaches considered while building the broker.

The goal of this project is not to build a production-ready RabbitMQ clone, but to deeply understand the mechanics behind message brokers.

---

# 1: Delivery Semantics

## Why At-Least-Once?

The broker implements **at-least-once delivery**.

### Why?
- Simpler to reason about
- Works well with lease-based processing
- Crash recovery becomes manageable

### Trade-off:
- Duplicate delivery is possible
- Consumers must be idempotent

---

## Why Not Exactly-Once?

Exactly-once requires:

- Persistent storage
- Transaction coordination
- Deduplication tracking
- Distributed consensus (in some cases)

This significantly increases complexity.

The goal of this project is to understand delivery semantics, not to build a full transactional system.

---

# 2: Lease-Based Processing Model

Each message delivery creates a **lease**:

- deliveryID (ephemeral)
- messageID (stable)
- deadline (visibility timeout)
- attempt counter

Lifecycle:
Ready → Inflight (lease) → Ack → Done </br>
↓ Timeout</br>
↓ Nack</br>


### Why lease-based?

- Enables crash recovery
- Supports visibility timeout
- Prevents permanent message loss

### Trade-off:
- Requires inflight tracking
- Introduces duplicate risk
- Adds state complexity

---

# 3: Visibility Timeout

A leased message has a deadline.

If the consumer does not acknowledge before deadline:

→ The message is redelivered.

### Why needed?

- Consumer crash recovery
- Prevent infinite inflight lock
- Enable automatic retry

### Trade-offs:

Short timeout:
- Faster recovery
- Higher false redelivery risk

Long timeout:
- Slower recovery
- Lower duplicate probability

Timeout tuning is workload-dependent.

---

# 4: Prefetch (QoS)

Each consumer has a prefetch limit.

Inflight per consumer is bounded.

### Why?

- Prevent slow consumers from overwhelming the system
- Implement backpressure
- Control concurrency

### Trade-offs:

- Potential fairness imbalance between consumers
- Requires per-consumer inflight accounting

Alternative design:
- Global inflight limit
- Weighted scheduling

Current implementation prioritizes simplicity and clarity.

### Observed Impact

Load testing confirmed that prefetch tuning has a significant effect on throughput:

| Prefetch | Consumers | Throughput |
|----------|-----------|------------|
| 5        | 3         | ~16 msg/sec |
| 50       | 3         | ~1,225 msg/sec |

With prefetch=5, consumers spend most of their time waiting for the next delivery slot to open up — the queue is underutilized. With prefetch=50, consumers can hold more messages in flight simultaneously and throughput scales accordingly.

This is a direct demonstration of why prefetch tuning is critical in real broker deployments (e.g. RabbitMQ `basic.qos`).

---

# 5: Retry & Dead Letter Queue (DLQ)

Each message tracks its retry attempts.

If:
Attempts >= maxRetries


→ Message is moved to DLQ.

### Why?

- Prevent infinite retry loops
- Isolate poison messages
- Protect system stability

---

## Bounded DLQ (Drop-Oldest Policy)

DLQ has a maximum size.

When full:
- Oldest message is dropped
- New failed message is inserted

### Why bounded?

- Prevent memory exhaustion
- Avoid OOM crashes

### Trade-off:

- Historical failure records may be lost
- Stability prioritized over perfect retention

Alternative options considered:
- Block publish when DLQ full (rejected)
- Reject new DLQ insertions (rejected)

System stability is prioritized.

---

# 6: Consumer Disconnect Handling

When a consumer disconnects:

→ All inflight messages for that consumer are immediately requeued.

### Why?

- Fast recovery
- Avoid waiting for visibility timeout
- Free prefetch slots immediately

### Trade-off:

Current implementation performs:

- O(n) scan over inflight map

Alternative design:
- Maintain `inflightByConsumer` index
- O(k) cleanup (k = inflight per consumer)

Complexity vs simplicity trade-off favored simplicity.

---

# 7: Graceful Shutdown Model

The broker implements a 3-state lifecycle:

Running → Draining → Stopped


Shutdown behavior:

1. Stop issuing new leases
2. Wait for inflight messages to finish
3. If drain timeout exceeded:
   - Force requeue remaining inflight
4. GracefulStop gRPC server

---

## Why Drain First?

Drain reduces:

- Duplicate delivery risk
- Partial execution issues

Immediate requeue during shutdown would increase duplicates.

---

## Why Timeout-Based Drain?

Infinite drain is unsafe in production.

If a consumer hangs:

- Shutdown could block forever.

Timeout ensures:

- Predictable shutdown time
- Kubernetes-friendly behavior

Trade-off:
- Some duplicate risk remains after timeout.

---

# 8: Concurrency Model

The broker uses:

- Mutex (`sync.Mutex`)
- Condition variables (`sync.Cond`)
- Map-based inflight tracking
- Blocking dequeue via cond.Wait()

### Why cond instead of channels?

- Precise wake-up control
- Multi-condition waiting (ready + prefetch + shutdown)
- Clear state machine reasoning

Trade-off:
- Slightly more complex than channel-only design
- Requires careful locking discipline

### Observed Behavior Under Load

CPU profiling via pprof at 50 concurrent consumers showed mutex acquisition (runtime.lock2) at 0.6% of CPU time — well below expectations. However, the profile was captured with only 13.4% sample coverage because the workload completed too quickly for meaningful sampling.

The flat throughput curve across 3–50 consumers (less than 8% variance) is a stronger signal: if mutex contention were significant, throughput would degrade as consumer count increases. At this message volume and hardware, the single-mutex design does not produce observable contention.

---

# 9: Observability Design

The broker exposes two metrics endpoints:

- `/metrics/json` → Human-readable debug snapshot
- `/metrics` → Prometheus exposition format (text/plain; version=0.0.4)

### Metrics Categories

**Gauges**
- ready queue size
- inflight count
- DLQ size
- average processing latency (ms)

**Counters**
- total published
- total acked
- total nacked
- total redelivered
- total processed
- total DLQ moves

### Processing Latency

Processing latency is measured as:

Lease.StartedAt → Ack time

This provides average end-to-end processing time per message.

### Trade-off

Current implementation:
- Uses snapshot-based metrics (derived from internal state)
- Protected by queue mutex
- Lightweight and dependency-free

Alternative (future):
- Prometheus Go client library
- Histogram buckets
- Atomic counters

---

# 10: Structured Logging

The broker uses `log/slog` from the Go standard library.

### Why slog?

- Zero external dependencies
- Structured, typed key-value fields
- JSON output format by default
- Part of Go stdlib since 1.21

### Why Dependency Injection?

Logger is passed via constructor (`NewQueue(logger, cfg)`) and struct fields (`Server.Logger`).

- No global state
- Testable — mock logger can be injected in tests
- Each component owns its logger reference

Alternative rejected:
- Global `slog.SetDefault()` — harder to test, implicit dependency

### Why JSON Format?

```json
{"time":"2026-03-11T12:00:00Z","level":"INFO","msg":"message_published","messageID":"abc123","queueSize":5}
```

JSON logs are machine-readable. Production log systems (Datadog, Grafana Loki, Elasticsearch) parse JSON natively — enabling field-level filtering and querying without regex.

Plain string logs require fragile regex parsing.

### What Is Not Logged — And Why

Message payload is never logged.

- Payload may contain PII, tokens, or sensitive data
- Logging payload would be a security violation in most production environments

Only metadata is logged: messageID, deliveryID, consumerID, attempt count, latency.

### Key Log Events

| Event | Level | Where |
|---|---|---|
| broker_started | INFO | main.go |
| broker_stopped | INFO | main.go |
| shutdown_signal_received | INFO | main.go |
| drain_complete | INFO | main.go |
| drain_timeout_reached | WARN | main.go |
| metrics_server_started | INFO | observability/metrics.go |
| consumer_connected | INFO | server.go |
| consumer_disconnected | INFO | server.go |
| message_published | INFO | server.go |
| lease_created | INFO | delivery.go |
| message_acked | INFO | delivery.go |
| message_nacked | INFO | delivery.go |
| message_requeued | INFO | delivery.go, reaper.go, lifecycle.go |
| message_dlq | WARN | delivery.go, reaper.go, lifecycle.go |

---

# 11: Configuration

All runtime parameters are externalized via environment variables.

### Loading Strategy

```
Environment Variable → Default Value
```

If the environment variable is set, it is used. Otherwise, the hardcoded default is applied. No config file is required to run the broker.

### Why godotenv?

`godotenv` is used only for local development — it loads a `.env` file into the process environment before `config.Load()` runs.

In production (Docker, Kubernetes), real environment variables are used directly. `godotenv.Load()` silently does nothing if no `.env` file is present.

### Configurable Parameters

| Variable | Default | Description |
|---|---|---|
| GRPC_PORT | :50051 | gRPC server listen address |
| METRICS_PORT | :8080 | HTTP metrics server address |
| MAX_RETRIES | 3 | Max delivery attempts before DLQ |
| MAX_DLQ_SIZE | 100 | Maximum DLQ capacity (drop-oldest) |
| VISIBILITY_TIMEOUT_SEC | 5 | Lease deadline in seconds |
| DRAIN_TIMEOUT_SEC | 10 | Graceful shutdown drain window |
| DEFAULT_PREFETCH | 1 | Default per-consumer prefetch limit |

### Trade-off

This approach avoids flag parsing complexity and config file formats. The broker remains stateless and container-friendly with zero required configuration.

---

# 12: Known Limitations

- In-memory only (no persistence)
- O(n) inflight scan on consumer disconnect
- No partitioning or sharding
- No exchange or routing model
- No idempotency key tracking

These are intentional omissions to keep the focus on delivery semantics.

---

# 13: Future Improvements

Potential extensions:

- Persistent storage layer
- Partitioned queues
- Exchange & routing model
- Prometheus client library (histogram buckets)
- Per-consumer inflight index (O(k) disconnect)
- Priority queues
- Message TTL

---

# 14: Benchmark Results

Load tests were run on a local machine using `go run ./cmd/loadtest`.

## Scaling Test — 500 messages, prefetch 50

Consumer count increased from 3 to 50 to observe scaling behavior.

| Consumers | msg/sec | avg(ms) | p50(ms) | p99(ms) | Loss |
|-----------|---------|---------|---------|---------|------|
| 3         | 1,225   | 0.777   | 0.999   | 2.000   | 0%   |
| 5         | 1,220   | 0.741   | 0.609   | 1.660   | 0%   |
| 10        | 1,176   | 0.861   | 0.999   | 2.001   | 0%   |
| 20        | 1,232   | 0.784   | 0.999   | 2.000   | 0%   |
| 50        | 1,139   | 0.873   | 0.999   | 2.002   | 0%   |

Throughput is stable across all consumer counts (~8% variance). There is no degradation cliff. This confirms that the single-mutex design is not a bottleneck at this scale — see Section 8.

## Sustained Load — 5,000 messages, 50 consumers, prefetch 50

| Metric | Result |
|--------|--------|
| Messages published | 5,000 |
| Messages acked | 5,000 |
| Throughput | 1,287 msg/sec |
| Avg ack latency | 0.875ms |
| p99 ack latency | 2.002ms |
| Message loss | 0% |
| Errors | 0 |

Zero message loss under sustained load with 50 concurrent consumers. At-least-once delivery guarantee held throughout.

## pprof CPU Profile — 50 consumers, 5,000 messages

Captured with `go tool pprof http://localhost:6060/debug/pprof/profile` during the sustained load test.

| Function | CPU% | Interpretation |
|----------|------|----------------|
| runtime.cgocall | 63% | OS network syscalls — gRPC transport layer |
| runtime.procyield | 2.4% | Mutex spinning under contention |
| runtime.lock2 | 0.6% | Actual mutex acquisition cost |

The bottleneck is gRPC's network I/O layer, not the queue's mutex. Each ack requires a separate TCP round-trip, which dominates CPU time. The single-mutex design produces negligible lock overhead at this message volume.

---

# Systems Learning Outcomes

This project demonstrates understanding of:

- Delivery guarantees
- Lease-based processing
- Failure isolation
- Backpressure control
- Controlled shutdown lifecycle
- Structured observability
- Trade-off driven design
- Performance profiling and bottleneck identification

The implementation focuses on clarity of semantics over feature completeness.

---

# Closing Note

This broker is intentionally minimal.

Its purpose is to expose the internal mechanics behind:

- RabbitMQ
- Kafka consumer semantics
- Distributed async systems

By building these primitives manually, the underlying systems behavior becomes explicit instead of hidden behind abstractions.
