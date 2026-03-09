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

---

# 9: Known Limitations

- In-memory only (no persistence)
- O(n) inflight scan on disconnect
- No partitioning or sharding
- No exchange/routing model
- No metrics endpoint
- No backoff strategy for retries
- No idempotency key tracking

These are intentional omissions to keep the focus on delivery semantics.

---

# 10: Future Improvements

Potential extensions:

- Persistent storage layer
- Partitioned queues
- Exchange & routing model
- Prometheus metrics
- Exponential backoff on Nack
- Per-consumer inflight index
- Priority queues
- Message TTL

---

# Systems Learning Outcomes

This project demonstrates understanding of:

- Delivery guarantees
- Lease-based processing
- Failure isolation
- Backpressure control
- Controlled shutdown lifecycle
- Trade-off driven design

The implementation focuses on clarity of semantics over feature completeness.

---

# Closing Note

This broker is intentionally minimal.

Its purpose is to expose the internal mechanics behind:

- RabbitMQ
- Kafka consumer semantics
- Distributed async systems

By building these primitives manually, the underlying systems behavior becomes explicit instead of hidden behind abstractions.

