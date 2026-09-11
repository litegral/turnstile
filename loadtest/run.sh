#!/usr/bin/env bash
set -euo pipefail

REQUEST_COUNT="${REQUEST_COUNT:-10001}"
VUS="${VUS:-200}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RUN_ID="$(date +%s)-$$"
PROJECT="turnstile-load-${RUN_ID}"
KEY_PREFIX="load-${RUN_ID}-"
RESULTS_DIR="${SCRIPT_DIR}/results"
COUNTS_PATH="${RESULTS_DIR}/counts.txt"
SUMMARY_PATH="${RESULTS_DIR}/summary.json"
EVIDENCE_PATH="${RESULTS_DIR}/evidence.txt"

if ! [[ "$REQUEST_COUNT" =~ ^[0-9]+$ ]] || (( REQUEST_COUNT < 10001 || REQUEST_COUNT > 1000000 )); then
    echo "REQUEST_COUNT must be between 10001 and 1000000" >&2
    exit 2
fi
if ! [[ "$VUS" =~ ^[0-9]+$ ]] || (( VUS < 1 || VUS > 2000 )); then
    echo "VUS must be between 1 and 2000" >&2
    exit 2
fi

export POSTGRES_DB=turnstile
export POSTGRES_USER=turnstile
export POSTGRES_PASSWORD=turnstile
export API_PORT=0
export LOG_LEVEL=warn

mkdir -p "$RESULTS_DIR"
rm -f "$COUNTS_PATH" "$SUMMARY_PATH" "$EVIDENCE_PATH"

cleanup() {
    echo "Cleaning up load-test containers and database"
    docker compose -p "$PROJECT" down -v >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

echo "[1/4] Starting full backend stack"
docker compose -p "$PROJECT" -f "${SCRIPT_DIR}/../compose.yaml" up -d --build --wait

echo "[2/4] Creating inventory for ${REQUEST_COUNT} bookings"
INVENTORY_ID="$(docker compose -p "$PROJECT" -f "${SCRIPT_DIR}/../compose.yaml" exec -T postgres psql \
    -qAt -v ON_ERROR_STOP=1 -v "run_id=${RUN_ID}" -v "request_count=${REQUEST_COUNT}" \
    -U turnstile -d turnstile < "${SCRIPT_DIR}/setup.sql")"
if ! [[ "$INVENTORY_ID" =~ ^[0-9]+$ ]]; then
    echo "Inventory setup failed: ${INVENTORY_ID}" >&2
    exit 1
fi

echo "[3/4] Sending ${REQUEST_COUNT} bookings with ${VUS} virtual users"
docker pull grafana/k6:1.3.0 >/dev/null
set +e
docker run --rm \
    --network "${PROJECT}_default" \
    --mount "type=bind,source=${SCRIPT_DIR},target=/work" \
    -e "BASE_URL=http://api:8080" \
    -e "RUN_ID=${RUN_ID}" \
    -e "INVENTORY_ID=${INVENTORY_ID}" \
    -e "REQUEST_COUNT=${REQUEST_COUNT}" \
    -e "VUS=${VUS}" \
    grafana/k6:1.3.0 run /work/bookings.js
K6_EXIT=$?
set -e

if [[ ! -f "$COUNTS_PATH" || ! -f "$SUMMARY_PATH" ]]; then
    echo "k6 did not produce files in ${RESULTS_DIR}" >&2
    exit 1
fi
IFS='|' read -r HTTP_REQUESTS HTTP_SUCCESSES ELAPSED_MILLISECONDS < "$COUNTS_PATH"

echo "[4/4] Reconciling HTTP results with PostgreSQL"
VERIFICATION="$(docker compose -p "$PROJECT" -f "${SCRIPT_DIR}/../compose.yaml" exec -T postgres psql \
    -qAt -F '|' -v ON_ERROR_STOP=1 -v "inventory_id=${INVENTORY_ID}" \
    -v "key_prefix=${KEY_PREFIX}" -v "expected=${REQUEST_COUNT}" -v "http_successes=${HTTP_SUCCESSES}" \
    -U turnstile -d turnstile < "${SCRIPT_DIR}/verify.sql")"

PASSED=false
IFS='|' read -r _ _ _ _ _ _ _ _ DATABASE_PASSED <<< "$VERIFICATION"
if (( K6_EXIT == 0 )) && [[ "$HTTP_REQUESTS" == "$REQUEST_COUNT" ]] \
    && [[ "$HTTP_SUCCESSES" == "$REQUEST_COUNT" ]] \
    && awk "BEGIN { exit !(${ELAPSED_MILLISECONDS} < 60000) }" \
    && [[ "$DATABASE_PASSED" == "t" ]]; then
    PASSED=true
fi

printf '\nHigh-traffic evidence\n'
printf '  Requests:              %s\n' "$HTTP_REQUESTS"
printf '  Created responses:     %s\n' "$HTTP_SUCCESSES"
printf '  Elapsed seconds:       %.3f\n' "$(awk "BEGIN { print ${ELAPSED_MILLISECONDS} / 1000 }")"
printf '  Database verification: %s\n' "$VERIFICATION"
printf '  Passed:                %s\n' "$PASSED"

if [[ "$PASSED" != true ]]; then
    echo "High-traffic validation failed" >&2
    exit 1
fi
printf 'requests=%s created=%s elapsed_seconds=%.3f database=%s' \
    "$HTTP_REQUESTS" "$HTTP_SUCCESSES" "$(awk "BEGIN { print ${ELAPSED_MILLISECONDS} / 1000 }")" "$VERIFICATION" \
    > "$EVIDENCE_PATH"
