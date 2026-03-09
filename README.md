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

- JSON metrics endpoint (`/metrics/json`)
- Prometheus-compatible metrics endpoint (`/metrics`)
- Ready / Inflight / DLQ gauges
- Delivery counters (published, acked, nacked, redelivered)
- Processing latency measurement (average)
- Total successfully processed messages

---

# Architecture

Producer </br>
↓</br>
gRPC Publish</br>
↓</br>
Ready Queue</br>
↓ (lease)</br>
Inflight Map</br>
↓</br>
Consumer (gRPC streaming)</br>

Timeout / Nack → Ready or DLQ


---

# Delivery Model

Each delivery creates a lease:

- `messageID` → stable message identity
- `deliveryID` → ephemeral lease identity
- deadline → visibility timeout
- attempt counter → retry tracking

Lifecycle:

Ready → Inflight → Ack → Done

↓ Timeout

↓ Nack

↓ MaxRetries → DLQ


The broker guarantees **at-least-once delivery**.

---

# Current Limitations

- In-memory only (no persistence)
- No partitioning
- No exchange/routing model
- No histogram-based latency buckets (average only)
- No structured logging yet

See `DESIGN.md` for detailed trade-offs and architectural reasoning.

---

# Roadmap

- [x] Core delivery semantics
- [x] Retry limit + DLQ
- [x] Consumer disconnect handling
- [x] Graceful shutdown (drain + timeout)
- [X] Observability / metrics
- [ ] Backoff strategy
- [ ] Optional persistence layer

---

# Learning Focus

This project explores:

- Lease-based message processing
- Backpressure and flow control
- Failure isolation patterns
- Distributed shutdown strategies
- Concurrency design in Go

For detailed design decisions and trade-offs, see:

👉 `DESIGN.md`
