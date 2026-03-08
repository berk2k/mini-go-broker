# Mini Go Broker

A learning-focused message broker written in Go to deeply understand:

- Message delivery guarantees
- Flow control (QoS / prefetch)
- Lease-based processing
- Failure isolation (DLQ)
- Consumer lifecycle management
- Concurrency primitives

This project is **not intended to replace RabbitMQ**,  
but to understand how such systems work internally by implementing core semantics from scratch.

---

# Executive Overview

This broker currently supports:

## Core Messaging Semantics

- At-least-once delivery
- Lease-based inflight tracking
- Visibility timeout
- Redelivery on failure
- Prefetch (per-consumer flow control)
- Ack / Nack (with requeue control)

## Failure Isolation

- Retry limit per message
- Dead Letter Queue (DLQ)
- Bounded DLQ (drop-oldest policy)

## Consumer Lifecycle

- Immediate inflight requeue on disconnect
- Timeout-based recovery fallback

This system demonstrates the fundamental behavior of modern message brokers.

---

# Architecture
Producer <br/>
↓<br/>
gRPC Publish<br/>
↓<br/>
Ready Queue<br/>
↓ (lease)<br/>
Inflight Map<br/>
↓<br/>
Consumer (gRPC streaming)<br/>

Timeout / Nack → Ready or DLQ


---

# Delivery Lifecycle
Ready<br/>
↓ (lease)<br/>
Inflight<br/>
↓ Ack → Done<br/>
↓ Nack (requeue=true) → Ready<br/>
↓ Timeout → Ready<br/>
↓ Attempts >= maxRetries → DLQ<br/>

Each lease has:

- deliveryID (ephemeral)
- messageID (stable)
- deadline
- attempt counter

---

# Design Decisions & Trade-offs

## 1: In-Memory Storage

**Why:**
- Focus on delivery semantics
- Avoid persistence complexity

**Trade-off:**
- Not crash-safe
- Memory-bound

---

## 2: Lease-Based Delivery

**Why:**
- Enables crash recovery
- Supports at-least-once semantics

**Trade-off:**
- Duplicate delivery possible
- Exactly-once not guaranteed

---

## 3: Visibility Timeout

**Why:**
- Recovers from consumer crashes
- Prevents infinite inflight lock

**Trade-off:**
- Short timeout → false redelivery
- Long timeout → slow recovery

---

## 4: Prefetch (QoS)

**Why:**
- Prevent slow consumers from blocking system
- Bound inflight per consumer

**Trade-off:**
- Potential fairness issues
- Requires inflight accounting

---

## 5: Bounded DLQ (Drop-Oldest Policy)

**Why:**
- Prevent memory exhaustion
- Ensure system stability

**Trade-off:**
- Historical failures may be lost
- Prioritizes stability over full history

---

## 6: Immediate Requeue on Disconnect

**Why:**
- Fast recovery
- Avoid waiting for timeout

**Trade-off:**
- Requires inflight scan (O(n))
- Could be optimized with per-consumer index

---

# Known Limitations

- In-memory only (no persistence)
- O(n) scan on consumer disconnect
- No partitioning
- No exchange/routing layer
- No metrics endpoint yet

---

# Roadmap to DONE

- [x] At-least-once semantics
- [x] Prefetch
- [x] Retry limit
- [x] Bounded DLQ
- [x] Consumer disconnect handling
- [ ] Graceful broker shutdown
- [ ] Metrics (ready/inflight/DLQ)
- [ ] Nack backoff strategy
- [ ] Optional persistence layer

---

# What This Project Demonstrates

- Concurrency control using mutex and condition variables
- Lease-based state machine design
- Flow control implementation
- Failure isolation strategies
- Trade-off-driven system design

---

# Systems Learning Focus

This project is part of a broader exploration into:

- Distributed systems fundamentals
- Backpressure propagation
- Async messaging design
- Reliability vs complexity trade-offs
