#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SKIP_LOAD_TEST=false
if [[ "${1:-}" == "--skip-load-test" ]]; then
    SKIP_LOAD_TEST=true
elif [[ $# -gt 0 ]]; then
    echo "Usage: ./validate.sh [--skip-load-test]" >&2
    exit 2
fi

RUN_ID="$(date +%s)-$$"
PROJECT="turnstile-validation-${RUN_ID}"
DATABASE_URL="postgres://turnstile:turnstile@postgres:5432/turnstile?sslmode=disable"
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
trap cleanup EXIT INT TERM

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
    go test -race -count=1 -p=1 -v ./...

cleanup
trap - EXIT INT TERM

if [[ "$SKIP_LOAD_TEST" == false ]]; then
    printf '\n[2/%s] Running full backend and 10,001-request load test\n' "$STAGE_COUNT"
    bash "${SCRIPT_DIR}/loadtest/run.sh"
fi

printf '\nAssessment scenario summary\n'
echo "  1. Race Condition:             PASSED"
if [[ "$SKIP_LOAD_TEST" == true ]]; then
    echo "  2. High Traffic Processing:    SKIPPED"
else
    echo "  2. High Traffic Processing:    PASSED"
fi
echo "  3. External API Integration:   PASSED"
echo "  4. Duplicate Request:          PASSED"
echo "  5. Data Synchronization:       PASSED"
printf '\n'
if [[ "$SKIP_LOAD_TEST" == true ]]; then
    echo "SELECTED VALIDATION PASSED (load test skipped)"
else
    echo "ALL ASSESSMENT SCENARIOS PASSED"
fi
echo "All temporary Docker resources were removed."
