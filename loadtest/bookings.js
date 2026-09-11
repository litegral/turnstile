import http from "k6/http";
import { check } from "k6";
import { Counter } from "k6/metrics";
import exec from "k6/execution";

const requestCount = Number(__ENV.REQUEST_COUNT || "10001");
const vus = Number(__ENV.VUS || "200");
const runID = __ENV.RUN_ID;
const inventoryID = Number(__ENV.INVENTORY_ID);

if (!runID || !Number.isInteger(inventoryID) || inventoryID < 1) {
  throw new Error("RUN_ID and positive INVENTORY_ID are required");
}

const created = new Counter("booking_created");

export const options = {
  scenarios: {
    bookings: {
      executor: "shared-iterations",
      vus,
      iterations: requestCount,
      maxDuration: "55s",
      gracefulStop: "0s",
    },
  },
  thresholds: {
    booking_created: [`count==${requestCount}`],
    checks: ["rate==1"],
    http_req_failed: ["rate==0"],
    http_reqs: [`count==${requestCount}`],
  },
};

export default function () {
  const iteration = exec.scenario.iterationInTest;
  const response = http.post(
    `${__ENV.BASE_URL || "http://api:8080"}/bookings`,
    JSON.stringify({
      inventory_id: inventoryID,
      customer_id: `load-${runID}-${iteration}`,
      quantity: 1,
    }),
    {
      headers: {
        "Content-Type": "application/json",
        "Idempotency-Key": `load-${runID}-${iteration}`,
      },
      tags: { name: "POST /bookings" },
      timeout: "10s",
    },
  );

  let body = null;
  try {
    body = response.json();
  } catch (_) {
    // Checks below report malformed responses without hiding request outcome.
  }

  created.add(response.status === 201);
  check(response, {
    "status is 201": (r) => r.status === 201,
    "transaction ID exists": () => Number.isInteger(body?.transaction_id) && body.transaction_id > 0,
    "inventory matches": () => body?.inventory_id === inventoryID,
    "quantity is one": () => body?.quantity === 1,
    "response is not replay": () => body?.idempotent_replay === false,
  });
}

export function handleSummary(data) {
  return {
    stdout: `requests=${data.metrics.http_reqs?.values?.count || 0} created=${data.metrics.booking_created?.values?.count || 0}\n`,
    "/work/results/summary.json": JSON.stringify(data, null, 2),
  };
}
