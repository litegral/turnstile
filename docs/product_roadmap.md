
# Turnstile - Product Roadmap

## Goal

Implement all technical-assessment scenarios as **one cohesive Go application** that demonstrates:

* correctness
* reliability
* scalability
* maintainability
* failure recovery
* automated testing

The assessment PDF remains the source of truth.

---

## Progress Tracking

Development progress is tracked in:

```text
progress.md
```

`progress.md` is the operational checklist for implementation.

Every meaningful completed task should be reflected there.

Rules:

* update it after completing a feature or milestone;
* do not mark something complete unless it is actually implemented and tested;
* record important design decisions or deviations;
* record blockers or remaining issues;
* keep the current next task obvious.

Suggested format:

```md
# Turnstile Progress

## Current
- [ ] Implement transactional outbox worker

## Completed
- [x] PostgreSQL migrations
- [x] Atomic ticket reservation
- [x] Purchase endpoint

## Remaining
- [ ] Duplicate webhook handling
- [ ] Availability versioning
- [ ] Kubernetes manifests
- [ ] Load testing

## Notes
- Booking correctness uses PostgreSQL atomic conditional update.
```

`product_roadmap.md` defines **what must be built**.

`progress.md` records **what has actually been built**.

---

# Phase 1 - Foundation

* [x] Initialize Go project
* [x] PostgreSQL + `pgxpool`
* [x] Database migrations
* [x] Configuration via environment variables
* [x] Structured logging
* [x] Dockerfile
* [x] Docker Compose
* [x] `/health`
* [x] `/ready`
* [x] Graceful shutdown

---

# Phase 2 - Booking & Race Condition

Implement the core purchase flow.

Requirements:

* [x] ticket inventory table
* [x] transactions table
* [x] atomic conditional inventory update
* [x] PostgreSQL transaction boundary
* [x] row-level concurrency safety
* [x] sold-out handling
* [x] client `Idempotency-Key`

The booking transaction should atomically:

```text
reserve inventory
+
create transaction
+
create accounting outbox event
+
create availability outbox event
```

Acceptance test:

```text
1 ticket
many concurrent buyers
→ exactly 1 successful purchase
→ inventory = 0
→ no overselling
```

---

# Phase 3 - High Traffic & Scalability

* [x] stateless API
* [x] bounded `pgxpool`
* [x] short database transactions
* [x] external calls outside booking transaction
* [x] multiple application instances supported
* [x] Kubernetes Deployment
* [x] Kubernetes Service
* [x] Horizontal Pod Autoscaler
* [x] readiness/liveness probes

Kubernetes handles compute scaling and process recovery.

PostgreSQL remains responsible for transaction correctness.

Add a reproducible load test:

```text
10,000+ requests
within < 60 seconds
```

Verify successful responses match committed transactions.

---

# Phase 4 - Transactional Outbox

Implement:

* [x] `outbox_events`
* [x] durable event creation inside booking transaction
* [x] background worker
* [x] `FOR UPDATE SKIP LOCKED`
* [x] processing lease / `locked_until`
* [x] worker crash recovery
* [x] exponential backoff
* [x] jitter
* [x] failed event visibility

Required events:

```text
ACCOUNTING_TRANSACTION_CREATED
AVAILABILITY_CHANGED
```

No successful booking may lose its outgoing event.

---

# Phase 5 - Accounting Integration

* [x] accounting HTTP client
* [x] explicit timeout
* [x] stable outgoing idempotency key
* [x] retry temporary failures
* [x] circuit breaker

Test scenario:

```text
500
→ 500
→ 200
```

Expected result:

```text
booking remains successful
event remains durable
event eventually completes
```

Delivery model:

```text
at-least-once + idempotency
```

---

# Phase 6 - Duplicate Webhooks

Provider contract should include:

```text
event_id
payment_id
transaction_id
```

Implement:

* [x] webhook endpoint
* [x] `webhook_events`
* [x] `transaction_payments`
* [x] `UNIQUE(provider, event_id)`
* [x] `UNIQUE(provider, payment_id)`
* [x] idempotent processing
* [x] duplicate requests return successful `2xx`

Acceptance test:

```text
2 identical concurrent webhook requests
→ 1 webhook event
→ 1 payment
→ no duplicate side effect
```

---

# Phase 7 - Availability Synchronization

Each inventory update must include:

```text
quantity
version
```

Implement:

* [ ] monotonically increasing inventory version
* [ ] version included in availability event
* [ ] destination stores latest version
* [ ] update only when incoming version is newer

Acceptance test:

```text
version 12, quantity 2
arrives first

version 11, quantity 5
arrives second

final state:
quantity = 2
version = 12
```

---

## Phase 8 - Observability & Validation

Turnstile must provide measurable evidence that the proposed reliability and scalability mechanisms work correctly.

### Monitoring

Use:

* Prometheus for metric collection
* Grafana for visualization
* k6 for load generation

Expose application metrics through:

```text
GET /metrics
```

Monitor at minimum:

* HTTP throughput
* HTTP success/error rate
* request latency (p50/p95/p99)
* active database connections
* booking success and sold-out count
* pending/retried outbox events
* accounting delivery success/failure
* duplicate webhook detection
* stale synchronization events

### Validation

Each assessment challenge must have reproducible validation:

* Race Condition → concurrent buyers cannot oversell inventory.
* High Traffic → process more than 10,000 requests within one minute while monitoring throughput and latency.
* External API → demonstrate failed delivery followed by retry and eventual success.
* Duplicate Request → concurrent duplicate webhooks result in only one payment record.
* Data Synchronization → an older version cannot overwrite a newer state.

Relevant automated test output, Grafana dashboards, and database results should be captured as evidence for the final technical report.


# Phase 9 - Documentation & Final Validation

Required documentation:

* [ ] README
* [ ] architecture diagram
* [ ] race-condition flow
* [ ] high-traffic flow
* [ ] accounting/retry flow
* [ ] duplicate-webhook flow
* [ ] synchronization flow
* [ ] combined system flow

README must explain:

* how to run the application;
* how to run migrations;
* how to run workers;
* how to run tests;
* how to run load testing;
* major architectural decisions and trade-offs.

---

# Definition of Done

Turnstile is ready for submission when:

* [ ] all five assessment scenarios are implemented;
* [ ] race conditions cannot oversell inventory;
* [ ] purchase retries are idempotent;
* [ ] API is stateless and horizontally scalable;
* [ ] database connections are bounded;
* [ ] successful transactions are durably stored;
* [ ] Transactional Outbox is implemented;
* [ ] workers support safe concurrent processing;
* [ ] retry uses exponential backoff + jitter;
* [ ] worker crash recovery exists;
* [ ] accounting integration uses idempotency and circuit breaker;
* [x] duplicate webhooks cannot duplicate payments;
* [ ] availability uses monotonic versioning;
* [ ] stale updates cannot overwrite newer state;
* [ ] concurrency scenarios are integration tested;
* [ ] 10,000+ request load test is reproducible;
* [ ] Kubernetes manifests exist;
* [ ] required diagrams exist;
* [ ] README fully explains run and test instructions;
* [ ] `progress.md` reflects the real implementation state;
* [ ] full automated test suite passes.