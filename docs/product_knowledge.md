# Turnstile - Product Knowledge

## Overview

Turnstile is a backend system for a high-traffic concert ticket booking platform.

The system must remain correct and reliable when many users purchase tickets concurrently, external services fail, duplicate webhooks arrive, or synchronization messages arrive out of order.

## Source of Truth

The technical assessment PDF in this repository is the **primary source of truth**.

Expected filename:

```text
technical_assessment.pdf
```

If any documentation or implementation conflicts with the assessment, the assessment takes precedence.

## Core Challenges

Turnstile must solve:

1. **Race Condition**
   Prevent overselling when multiple users attempt to purchase the last ticket simultaneously.

2. **High Traffic Processing**
   Handle more than 10,000 transactions in under one minute while ensuring every reported successful transaction is durably stored.

3. **External API Reliability**
   Ensure every successful transaction is eventually delivered to the external accounting system even when it returns errors or becomes temporarily unavailable.

4. **Duplicate Webhook Requests**
   Prevent duplicate payment records when the provider retries or sends identical webhook requests concurrently.

5. **Out-of-Order Synchronization**
   Prevent stale ticket availability updates from overwriting newer data.

## Architecture

Turnstile uses a **modular monolith** written in Go.

Core stack:

* Go
* PostgreSQL
* `pgx` / `pgxpool`
* HTTP API
* background workers
* Docker / Docker Compose
* Kubernetes-ready deployment

The application layer is stateless so multiple API instances can run behind a load balancer.

PostgreSQL is the system of record and provides the main consistency guarantees.

## Key Design Decisions

### Race Condition

Use a PostgreSQL transaction with an **atomic conditional inventory update**.

PostgreSQL row-level locking ensures concurrent requests cannot oversell the same inventory.

### High Traffic

Keep the synchronous booking path short:

```text
validate
→ reserve inventory
→ save transaction
→ save outbox events
→ commit
→ return success
```

Scale stateless Go API instances horizontally behind a load balancer.

Use bounded database connection pooling.

Kubernetes may be used for replica management, health checks, recovery, and Horizontal Pod Autoscaling.

### External API

Use the **Transactional Outbox Pattern**.

Successful bookings and their outgoing events are committed atomically.

Background workers deliver events with:

* `FOR UPDATE SKIP LOCKED`
* retry
* exponential backoff + jitter
* processing lease
* circuit breaker
* stable idempotency key

Delivery semantics are:

```text
at-least-once delivery + idempotent processing
```

### Duplicate Webhooks

The provider should include stable identifiers such as:

```text
event_id
payment_id
```

Turnstile enforces uniqueness using PostgreSQL constraints.

Database-level uniqueness is the final concurrency guarantee.

### Data Synchronization

Each inventory change carries a monotonically increasing `version`.

The destination only accepts an update when:

```text
incoming_version > stored_version
```

This prevents stale messages from overwriting newer state.

## Engineering Philosophy

Prefer simple, explicit, maintainable solutions.

Do not introduce Kafka, Redis distributed locks, RabbitMQ, or microservices unless they solve a demonstrated requirement.

Correctness must not depend on a single application instance or in-memory state.

## Observability

Turnstile includes observability and validation as part of the system design. Prometheus collects application and business metrics, Grafana visualizes system behavior, and k6 is used for reproducible load testing. These tools are used to provide measurable evidence for reliability and scalability claims.