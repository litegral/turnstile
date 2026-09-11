# Turnstile Progress

## Current

- [ ] Complete Phase 9 README, diagrams, and final report screenshots

## Completed

- [x] Phase 1 foundation
- [x] Go application entry points for API and migrations
- [x] PostgreSQL connection through bounded `pgxpool`
- [x] Embedded, ordered, transactional database migrations
- [x] Environment-based configuration with validation
- [x] JSON structured logging
- [x] Dockerfile and Docker Compose
- [x] `/health` liveness and database-backed `/ready` readiness
- [x] Signal handling and graceful HTTP shutdown
- [x] Phase 2 booking and race-condition handling
- [x] Ticket inventory, transaction, idempotency request, and outbox event schema
- [x] Atomic conditional inventory reservation inside a short PostgreSQL transaction
- [x] `POST /bookings` with strict JSON validation and required `Idempotency-Key`
- [x] Durable accounting and availability events committed with each booking
- [x] Concurrent one-ticket acceptance test and concurrent idempotency replay test
- [x] Phase 3 high traffic and scalability
- [x] Stateless API with bounded per-instance PostgreSQL pools and request-scoped booking deadlines
- [x] Kubernetes Deployment, Service, HPA, disruption budget, and readiness/liveness/startup probes
- [x] Reproducible k6 test for 10,001 bookings with PostgreSQL reconciliation
- [x] Load validation: 10,001 created responses and committed transactions in 38.924 seconds
- [x] Phase 4 transactional outbox engine
- [x] Atomic PostgreSQL claims with `FOR UPDATE SKIP LOCKED`
- [x] Durable processing leases with opaque claim tokens and stale-worker protection
- [x] Worker crash recovery through expired-lease reclamation
- [x] Bounded exponential backoff with cryptographic jitter
- [x] Persistent attempt count, last error, and terminal `FAILED` visibility
- [x] Concurrent claim, retry, stale claim, completion, and cancellation tests
- [x] Phase 5 accounting HTTP integration with explicit timeout and stable event-ID idempotency key
- [x] Temporary-failure retry through durable outbox and concurrency-safe circuit breaker
- [x] Accounting worker activation scoped to accounting event types
- [x] Runnable Docker Compose mock accounting service with configurable temporary failures
- [x] PostgreSQL end-to-end accounting test covering booking, durable retries, stable idempotency, and completion
- [x] Temporary accounting failures retry indefinitely with capped backoff; circuit-open postponements do not consume attempts
- [x] Phase 6 duplicate payment webhook handling
- [x] `POST /webhooks/{provider}/payments` with strict JSON validation and request deadline
- [x] Atomic webhook receipt and transaction payment persistence
- [x] PostgreSQL uniqueness on `(provider, event_id)` and `(provider, payment_id)`
- [x] Successful `2xx` replay for exact duplicates and conflict rejection for reused identifiers
- [x] Concurrent duplicate webhook acceptance test proving one receipt and one payment
- [x] Phase 7 availability synchronization
- [x] Monotonic inventory version included in every durable availability event
- [x] Destination projection guarded by atomic newer-version-only PostgreSQL upsert
- [x] Dedicated availability outbox worker with stale and duplicate delivery no-ops
- [x] Out-of-order and concurrent synchronization acceptance tests
- [x] Phase 8 reproducible validation evidence
- [x] One-command PostgreSQL acceptance validation with Go race detector
- [x] Numeric evidence for race, accounting retry, duplicate webhook, and stale availability scenarios
- [x] Fresh 10,001-request k6 run reconciled against committed PostgreSQL state in 40.920 seconds

## Remaining

- [ ] Phase 9 from `product_roadmap.md`

## Notes

- PostgreSQL remains system of record; liveness does not depend on database health.
- Migrations use a PostgreSQL advisory lock so concurrent deploys cannot apply the same migration twice.
- Booking correctness uses `UPDATE ... WHERE available_quantity >= quantity RETURNING`; PostgreSQL row locking serializes competing updates.
- Idempotency keys are claimed in PostgreSQL before inventory changes; concurrent matching retries replay one committed transaction.
- Booking, inventory change, and both outbox events commit atomically. External calls remain outside the transaction.
- Source-of-truth assessment is `docs/test_assessment.md`.
- Outbox delivery is at-least-once; handlers must use immutable outbox event IDs as idempotency keys.
- Accounting and availability workers independently claim only their destination event types.
- Availability destination applies `EXCLUDED.version > stored.version` atomically; stale and duplicate deliveries complete as successful no-ops.
- Expired leases recover crashed work; claim tokens prevent stale workers from completing or failing reclaimed events.
- Terminal failures remain queryable with attempt count and last error for operational visibility.
- Accounting retries network errors, HTTP 408/425/429, and 5xx indefinitely with bounded backoff; other non-2xx responses become visible terminal failures.
- Circuit breaker limits calls during accounting outages; open-circuit postponements do not consume attempts, and PostgreSQL outbox remains sole delivery-state owner.
- Payment webhooks rely on PostgreSQL unique constraints, not process-local locks; exact duplicates replay successfully across concurrent API replicas.
- Reused provider event or payment identifiers with different transaction data return conflict instead of silently accepting inconsistent state.
- Migration metadata uses `public.schema_migrations` to prevent search-path changes from replaying migrations.
- HPA permits 2-5 API replicas; 10 connections per replica caps application database connections at 50.
- Phase 8 validation used one hot inventory row, 200 VUs, PostgreSQL 17, and k6 1.3.0. Fresh result: 10,001 created responses in 40.920 seconds; reconciliation `0|10001|10001|10001|10001|20002|10001|10001|t`.
- `validate.ps1` runs isolated PostgreSQL acceptance tests sequentially because integration packages truncate shared test tables.
