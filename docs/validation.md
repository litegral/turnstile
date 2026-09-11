# Validation Evidence

Phase 8 evidence comes from PostgreSQL-backed acceptance tests and the k6 load test. Tests print numeric database results; failed assertions return a non-zero exit code.

## Complete Validation

Prerequisites: Docker Desktop with Docker Compose and PowerShell 7+.

```powershell
pwsh ./validate.ps1
```

This command creates disposable PostgreSQL environments, runs acceptance tests with the Go race detector, executes 10,001 bookings through the HTTP API, reconciles HTTP successes against committed PostgreSQL rows, then removes its containers and volumes.

Use this faster command while changing acceptance tests:

```powershell
pwsh ./validate.ps1 -SkipLoadTest
```

## Scenario Evidence

### Race Condition

Test: `TestConcurrentBookingDoesNotOversell`

Expected numeric evidence:

```text
buyers=32 succeeded=1 sold_out=31 available=0 transactions=1 outbox_events=2
```

PostgreSQL values come from `ticket_inventory`, `transactions`, and `outbox_events` after all buyers start concurrently.

### High Traffic

Command:

```powershell
pwsh ./loadtest/run.ps1 -RequestCount 10001 -VUs 200
```

Pass criteria:

```text
Requests=10001
CreatedResponses=10001
ElapsedSeconds<60
DatabaseVerification=0|10001|10001|10001|10001|20002|10001|10001|t
Passed=True
```

`DatabaseVerification` fields are remaining inventory, inventory version, transaction count, booked quantity, completed idempotency requests, total outbox events, accounting events, availability events, and combined reconciliation result.

Recorded validation on PostgreSQL 17 and k6 1.3.0 on September 11, 2026:

```text
Requests=10001
CreatedResponses=10001
ElapsedSeconds=40.920
DatabaseVerification=0|10001|10001|10001|10001|20002|10001|10001|t
Passed=True
```

### External API

Test: `TestBookingEventuallyReachesAccounting`

Expected numeric evidence:

```text
http_requests=3 failed_requests=2 successful_requests=1 recorded_attempts=2 status=COMPLETED stable_idempotency_key=true committed_transactions=1
```

Mock destination returns HTTP 500 twice, then HTTP 200. PostgreSQL outbox status and attempt count prove durable retry and eventual completion.

### Duplicate Request

Test: `TestConcurrentDuplicateWebhookCreatesOnePayment`

Expected numeric evidence:

```text
concurrent_requests=16 webhook_events=1 transaction_payments=1
```

PostgreSQL unique constraints remain final concurrency guard.

### Data Synchronization

Test: `TestOutOfOrderAvailabilityKeepsNewestVersion`

Expected numeric evidence:

```text
delivered_versions=12,11 final_quantity=2 final_version=12
```

PostgreSQL destination receives version 12 before version 11. Newer-version-only upsert prevents stale overwrite.
