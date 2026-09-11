#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SKIP_LOAD_TEST=false
if [[ "${1:-}" == "--skip-load-test" ]]; then
    SKIP_LOAD_TEST=true
elif [[ $# -gt 0 ]]; then
    echo "Usage: bash ./validate.sh [--skip-load-test]" >&2
    exit 2
fi

RUN_ID="$(date +%s)-$$"
PROJECT="turnstile-validation-${RUN_ID}"
DATABASE_URL="postgres://turnstile:turnstile@postgres:5432/turnstile?sslmode=disable"
TEST_LOG="$(mktemp)"
LOAD_EVIDENCE_PATH="${SCRIPT_DIR}/loadtest/results/evidence.txt"
STAGE_COUNT=2
if [[ "$SKIP_LOAD_TEST" == true ]]; then
    STAGE_COUNT=1
fi

export POSTGRES_DB=turnstile
export POSTGRES_USER=turnstile
export POSTGRES_PASSWORD=turnstile

cleanup() {
    echo "Cleaning up test database"
    docker compose -p "$PROJECT" -f "${SCRIPT_DIR}/compose.yaml" down -v >/dev/null 2>&1 || true
}
trap 'cleanup; rm -f "$TEST_LOG"' EXIT INT TERM

echo "Turnstile validation"
echo "Docker runs Go, PostgreSQL, backend services, mock accounting, and k6."
echo "No local Go, PostgreSQL, or k6 installation required."
printf '\n[1/%s] Running complete Go test suite with PostgreSQL and race detector\n' "$STAGE_COUNT"
docker compose -p "$PROJECT" -f "${SCRIPT_DIR}/compose.yaml" up -d postgres --wait
docker run --rm \
    --network "${PROJECT}_default" \
    --mount "type=bind,source=${SCRIPT_DIR},target=/src" \
    --workdir /src \
    -e "TEST_DATABASE_URL=${DATABASE_URL}" \
    golang:1.25.5 \
    go test -race -count=1 -p=1 -v ./... 2>&1 | tee "$TEST_LOG"

cleanup
trap 'rm -f "$TEST_LOG"' EXIT INT TERM

RACE_EVIDENCE="$(grep -m1 'race evidence:' "$TEST_LOG" | sed 's/^.*race evidence: //')"
ACCOUNTING_EVIDENCE="$(grep -m1 'accounting evidence:' "$TEST_LOG" | sed 's/^.*accounting evidence: //')"
DUPLICATE_EVIDENCE="$(grep -m1 'duplicate webhook evidence:' "$TEST_LOG" | sed 's/^.*duplicate webhook evidence: //')"
AVAILABILITY_EVIDENCE="$(grep -m1 'availability evidence:' "$TEST_LOG" | sed 's/^.*availability evidence: //')"
if [[ -z "$RACE_EVIDENCE" || -z "$ACCOUNTING_EVIDENCE" || -z "$DUPLICATE_EVIDENCE" || -z "$AVAILABILITY_EVIDENCE" ]]; then
    echo "Go tests passed but one or more assessment evidence markers are missing" >&2
    exit 1
fi

if [[ "$SKIP_LOAD_TEST" == false ]]; then
    printf '\n[2/%s] Running full backend and 10,001-request load test\n' "$STAGE_COUNT"
    bash "${SCRIPT_DIR}/loadtest/run.sh"
    if [[ ! -f "$LOAD_EVIDENCE_PATH" ]]; then
        echo "High-traffic evidence file is missing" >&2
        exit 1
    fi
    HIGH_TRAFFIC_EVIDENCE="$(cat "$LOAD_EVIDENCE_PATH")"
fi

printf '\nAssessment scenario summary\n'
echo "  1. Race Condition:             PASSED"
echo "     ${RACE_EVIDENCE}"
if [[ "$SKIP_LOAD_TEST" == true ]]; then
    echo "  2. High Traffic Processing:    SKIPPED"
else
    echo "  2. High Traffic Processing:    PASSED"
    echo "     ${HIGH_TRAFFIC_EVIDENCE}"
fi
echo "  3. External API Integration:   PASSED"
echo "     ${ACCOUNTING_EVIDENCE}"
echo "  4. Duplicate Request:          PASSED"
echo "     ${DUPLICATE_EVIDENCE}"
echo "  5. Data Synchronization:       PASSED"
echo "     ${AVAILABILITY_EVIDENCE}"
printf '\n'
if [[ "$SKIP_LOAD_TEST" == true ]]; then
    echo "SELECTED VALIDATION PASSED (load test skipped)"
else
    echo "ALL ASSESSMENT SCENARIOS PASSED"
fi
echo "All temporary Docker resources were removed."
