# Turnstile Progress

## Current

- [ ] Wire outbox worker to real accounting and availability handlers in Phases 5 and 7

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

## Remaining

- [ ] Phase 4 production activation after destination handlers exist
- [ ] Phases 5-9 from `product_roadmap.md`

## Notes

- PostgreSQL remains system of record; liveness does not depend on database health.
- Migrations use a PostgreSQL advisory lock so concurrent deploys cannot apply the same migration twice.
- Booking correctness uses `UPDATE ... WHERE available_quantity >= quantity RETURNING`; PostgreSQL row locking serializes competing updates.
- Idempotency keys are claimed in PostgreSQL before inventory changes; concurrent matching retries replay one committed transaction.
- Booking, inventory change, and both outbox events commit atomically. External calls remain outside the transaction.
- Source-of-truth assessment is `docs/technical_assessment.pdf`.
- Outbox delivery is at-least-once; handlers must use immutable outbox event IDs as idempotency keys.
- Worker engine remains unwired until real destination handlers exist; placeholder handlers would falsely complete undelivered events.
- Expired leases recover crashed work; claim tokens prevent stale workers from completing or failing reclaimed events.
- Terminal failures remain queryable with attempt count and last error for operational visibility.
- Migration metadata uses `public.schema_migrations` to prevent search-path changes from replaying migrations.
- HPA permits 2-5 API replicas; 10 connections per replica caps application database connections at 50.
- Phase 3 load validation used one hot inventory row, 200 VUs, PostgreSQL 17, and k6 1.3.0. Reconciliation result: `0|10001|10001|10001|10001|20002|10001|10001|t`.
