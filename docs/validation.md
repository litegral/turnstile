# Validation Evidence

Company testers need only Docker Desktop or Docker Engine. Go, PostgreSQL, and k6 run inside containers.

## Run Everything

From repository root on Linux or macOS:

```bash
bash ./validate.sh
```

From repository root on Windows PowerShell:

```powershell
pwsh ./validate.ps1
```

Both commands run same validation flow:

1. Start disposable PostgreSQL.
2. Run complete Go test suite with race detector.
3. Run PostgreSQL-backed concurrency and reliability tests.
4. Remove test database.
5. Build and start full backend stack: PostgreSQL, migrations, Go API, outbox workers, and mock accounting.
6. Send 10,001 HTTP booking requests with k6.
7. Reconcile successful HTTP responses with committed PostgreSQL records.
8. Remove all temporary containers and volumes.

Successful full run exits with code `0` and ends with an explicit assessment summary:

```text
Assessment scenario summary
  1. Race Condition:             PASSED
  2. High Traffic Processing:    PASSED
  3. External API Integration:   PASSED
  4. Duplicate Request:          PASSED
  5. Data Synchronization:       PASSED

ALL ASSESSMENT SCENARIOS PASSED
All temporary Docker resources were removed.
```

Expected duration: about 1-3 minutes after Docker images are cached. First run takes longer because Docker downloads images and builds backend.

## Fast Validation

Skip 10,001-request load test while developing.

Linux or macOS:

```bash
bash ./validate.sh --skip-load-test
```

Windows PowerShell:

```powershell
pwsh ./validate.ps1 -SkipLoadTest
```

Fast mode still runs complete Go suite, PostgreSQL integration tests, and race detector. It does not start full API stack. Summary reports `High Traffic Processing: SKIPPED` and never claims all assessment scenarios passed.

## Load Test Only

Linux or macOS:

```bash
bash ./loadtest/run.sh
```

Optional workload configuration:

```bash
REQUEST_COUNT=20000 VUS=300 bash ./loadtest/run.sh
```

Windows PowerShell:

```powershell
pwsh ./loadtest/run.ps1 -RequestCount 10001 -VUs 200
```

Load test succeeds only when all requests return created responses within 60 seconds and PostgreSQL reconciliation passes.

## Evidence

Race condition:

```text
buyers=32 succeeded=1 sold_out=31 available=0 transactions=1 outbox_events=2
```

Accounting retry:

```text
http_requests=3 failed_requests=2 successful_requests=1 recorded_attempts=2 status=COMPLETED stable_idempotency_key=true committed_transactions=1
```

Duplicate webhook:

```text
concurrent_requests=16 webhook_events=1 transaction_payments=1
```

Out-of-order availability:

```text
delivered_versions=12,11 final_quantity=2 final_version=12
```

High traffic:

```text
Requests:              10001
Created responses:     10001
Elapsed seconds:       39.663
Database verification: 0|10001|10001|10001|10001|20002|10001|10001|t
Passed:                True
```

Database verification fields represent remaining inventory, inventory version, transaction count, booked quantity, completed idempotency requests, total outbox events, accounting events, availability events, and final pass result.

Recorded on PostgreSQL 17 and k6 1.3.0 on September 11, 2026.

## Troubleshooting

Confirm Docker works:

```bash
docker info
docker compose version
```

Scripts are invoked through `bash`, so executable file permissions are not required.

A failed test returns non-zero exit code and prints failed stage. Cleanup still runs. Detailed k6 result remains in `loadtest/results/summary.json`.
