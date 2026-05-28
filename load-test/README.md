
# Load Test Guide

This folder contains k6 load tests for the corporate event ticketing system.

## Recommended quick start

```bash
docker compose up -d --build
chmod +x load-test/run_load_test.sh
TOTAL_USERS=10 TOTAL_QUOTA=10 VUS=10 ITERATIONS=1 KEEP_DATA=1 ./load-test/run_load_test.sh
```

The script writes every run to a timestamped folder:

```text
load-test/results/<RUN_ID>/
```

It also creates a zip automatically:

```text
load-test/results/<RUN_ID>.zip
```

Upload this generated zip when asking someone else to review the result. Do not manually copy old `book-summary.json` / `db-summary.txt`, because it is easy to mix files from different runs.

## Common commands

### 100 users, 100 tickets

```bash
RUN_LABEL=100v100q TOTAL_USERS=100 TOTAL_QUOTA=100 VUS=100 ITERATIONS=1 KEEP_DATA=0 ./load-test/run_load_test.sh
```

### 2000 users competing for 1000 tickets

```bash
RUN_LABEL=2000v1000q TOTAL_USERS=2000 TOTAL_QUOTA=1000 VUS=2000 ITERATIONS=1 MAX_DURATION=5m KEEP_DATA=1 ./load-test/run_load_test.sh
```

### 5000 users competing for 5000 tickets

```bash
RUN_LABEL=5000v5000q TOTAL_USERS=5000 TOTAL_QUOTA=5000 VUS=5000 ITERATIONS=1 MAX_DURATION=5m KEEP_DATA=1 ./load-test/run_load_test.sh
```

## Parameters

| Variable | Meaning | Default |
|---|---|---:|
| TOTAL_USERS | Number of fake employees inserted into DB | 2000 |
| TOTAL_QUOTA | Ticket quota for the test ticket type | 50000 |
| MAX_TICKETS_PER_PERSON | Per-user ticket limit | 1 |
| VUS | k6 virtual users | 1000 |
| ITERATIONS | Booking attempts per VU | 1 |
| MAX_DURATION | k6 max duration | 3m |
| KEEP_DATA | Keep DB rows after test for manual inspection | 1 |
| STRICT_THRESHOLDS | Enable strict threshold failure | 0 |
| FAIL_ON_THRESHOLD | Exit nonzero after collecting artifacts if k6 threshold fails | 0 |
| ERROR_SAMPLE_RATE | Probability to log a sample error response body | 0.02 |
| TEST_MODE | Test mode switch: `spike` (simultaneous burst) or `constant` (constant arrival rate) | spike |
| RATE | Requests per second (RPS) when `TEST_MODE=constant` | 1000 |
| PRE_ALLOCATED_VUS | Number of pre-allocated virtual users when `TEST_MODE=constant` | 500 |
| MAX_VUS | Maximum allowed virtual users under heavy system load when `TEST_MODE=constant` | 2000 |

## Output files

Each result folder contains:

```text
book-summary.json   k6 machine-readable summary
k6-console.log      full k6 console output, including sampled error bodies
db-summary.txt      DB / Redis verification
run-summary.md      human-readable report summary
stress_env.json     IDs used by this run
parameters.env      run parameters
```

## Important interpretation

If `VUS > TOTAL_USERS`, the test is invalid. Later VUs would not have real user IDs. The script now blocks this situation.

If `booking_server_error` is high while quota is not exhausted, the bottleneck is likely backend/DB throughput or transaction contention, not normal sold-out behavior.

If `ticket_type_consumed_by_db + ticket_type_remaining = ticket_type_total_quota`, the DB anti-oversell result is internally consistent.


## v3 notes: archive fallback and docker logs

If your environment does not have the `zip` command, `run_load_test.sh` now falls back to Python's built-in `zipfile` module. If Python is unavailable, it falls back to `tar.gz`.

Each result folder now also includes:

```text
backend-tail.log
postgres-tail.log
redis-tail.log
```

These logs are useful when k6 reports many HTTP 500 responses, because the API response body usually only says `INTERNAL_ERROR`, while the container logs may contain the real database or transaction error.
## Queue mode load-test notes

When `TICKET_QUEUE_ENABLED=true`, `POST /v1/applications` may return `202 Accepted` instead of `201 Created`. The k6 script counts both as successful business outcomes:

- `201`: synchronous booking created immediately
- `202`: request entered the Redis Stream waiting room
- `409`: normal sold-out response
- `5xx`: server-side failure

Recommended queue-mode run:

```bash
RUN_LABEL=10000v10000q-queue \
TOTAL_USERS=10000 \
TOTAL_QUOTA=10000 \
VUS=10000 \
ITERATIONS=1 \
MAX_DURATION=3m \
HTTP_TIMEOUT=60s \
SETTLE_SECONDS=120 \
KEEP_DATA=1 \
STRICT_THRESHOLDS=0 \
./load-test/run_load_test.sh
```

For larger conceptual tests such as 50,000 users, increase `TOTAL_USERS`, `TOTAL_QUOTA`, `VUS`, and `SETTLE_SECONDS`. In queue mode, the first metric to watch is not immediate DB completion; it is whether requests enter the waiting room without 5xx. Then inspect DB/Redis after enough settle time to confirm the worker pool drains the queue safely.

## Additional k6 scenarios added

### Smoke test

Use this before a large test to confirm the deployed API is reachable and demo login works.

```bash
BASE_ORIGIN=http://localhost:8001 BASE_URL=http://localhost:8001/v1 k6 run load-test/smoke.js
```

For Azure / HTTPS deployment, replace `BASE_ORIGIN` and `BASE_URL`:

```bash
BASE_ORIGIN=https://your-domain.example BASE_URL=https://your-domain.example/v1 k6 run load-test/smoke.js
```

### Read-heavy event browsing test

This tests high-concurrency read traffic such as employees refreshing the event list and opening event detail pages. It uses `constant-arrival-rate` by default.

```bash
RUN_LABEL=read-heavy \
K6_SCRIPT=read-heavy.js \
TOTAL_USERS=5000 \
TOTAL_QUOTA=5000 \
VUS=1000 \
RATE=500 \
DURATION=2m \
PRE_ALLOCATED_VUS=500 \
MAX_VUS=2000 \
DISCARD_RESPONSE_BODIES=1 \
KEEP_DATA=1 \
STRICT_THRESHOLDS=0 \
./load-test/run_load_test.sh
```

### Mixed user journey test

This simulates a more realistic flow: most iterations browse the event list, some open details, and a smaller portion submit booking requests.

```bash
RUN_LABEL=mixed-journey \
K6_SCRIPT=mixed-journey.js \
TOTAL_USERS=5000 \
TOTAL_QUOTA=5000 \
RATE=300 \
DURATION=2m \
PRE_ALLOCATED_VUS=300 \
MAX_VUS=1500 \
MIXED_BOOKING_RATIO=0.1 \
READ_DETAIL_RATIO=0.2 \
DISCARD_RESPONSE_BODIES=1 \
SETTLE_SECONDS=120 \
KEEP_DATA=1 \
STRICT_THRESHOLDS=0 \
./load-test/run_load_test.sh
```

### Constant-arrival-rate booking test

This is useful for sustained throughput testing. Unlike the burst test, k6 starts iterations at a fixed arrival rate independent of backend response speed.

```bash
RUN_LABEL=booking-arrival-rate \
K6_SCRIPT=constant-arrival-booking.js \
TOTAL_USERS=12000 \
TOTAL_QUOTA=12000 \
RATE=300 \
DURATION=2m \
PRE_ALLOCATED_VUS=500 \
MAX_VUS=3000 \
DISCARD_RESPONSE_BODIES=1 \
SETTLE_SECONDS=180 \
KEEP_DATA=1 \
STRICT_THRESHOLDS=0 \
./load-test/run_load_test.sh
```

## Performance mode for large tests

For large VU counts, use:

```bash
DISCARD_RESPONSE_BODIES=1 ERROR_SAMPLE_RATE=0
```

This reduces load-generator memory usage. For debugging smaller tests, keep response bodies and use a small `ERROR_SAMPLE_RATE` so failed responses can be inspected.

## Event list Redis cache

The backend can cache `GET /events` list responses in Redis using:

```env
EVENT_LIST_CACHE_TTL_SECONDS=15
```

Set it to `0` to disable the cache. The cache is short-lived and is invalidated on common event mutations. The scheduler also clears event-list cache after lifecycle updates.



# K8s Load Test Guide

Context below contains k6 load tests modified specifically for the Kubernetes (AKS) cluster environment.

## Recommended quick start

Ensure your local terminal is authenticated and connected to an active AKS cluster (`kubectl get nodes`).

```bash
chmod +x load-test/run_k8s_load_test.sh
TOTAL_USERS=10 TOTAL_QUOTA=10 VUS=10 ITERATIONS=1 KEEP_DATA=1 BASE_URL=https://nthu-team13.duckdns.org/v1 ./load-test/run_k8s_load_test.sh

```

The script dynamically detects Postgres and Redis pods via labels, provisions sandbox datasets isolated by a transient `RUN_ID`, and outputs results to:

```text
load-test/results/<RUN_ID>/
load-test/results/<RUN_ID>.zip

```

---

## Common K8s commands

### 100 users, 100 tickets baseline check

```bash
RUN_LABEL=100v100q-k8s TOTAL_USERS=100 TOTAL_QUOTA=100 VUS=100 ITERATIONS=1 KEEP_DATA=0 BASE_URL=https://nthu-team13.duckdns.org/v1 ./load-test/run_k8s_load_test.sh

```

### 2000 users competing under queue mode (Sustained Settle)

```bash
RUN_LABEL=2000v2000q-queue-k8s TOTAL_USERS=2000 TOTAL_QUOTA=2000 VUS=2000 ITERATIONS=1 MAX_DURATION=3m HTTP_TIMEOUT=60s SETTLE_SECONDS=120 KEEP_DATA=0 BASE_URL=https://nthu-team13.duckdns.org/v1 ./load-test/run_k8s_load_test.sh

```

### Mixed user journey test on remote ingress

```bash
RUN_LABEL=mixed-journey-k8s K6_SCRIPT=mixed-journey.js TOTAL_USERS=5000 TOTAL_QUOTA=5000 RATE=300 DURATION=2m PRE_ALLOCATED_VUS=300 MAX_VUS=1500 MIXED_BOOKING_RATIO=0.1 READ_DETAIL_RATIO=0.2 DISCARD_RESPONSE_BODIES=1 SETTLE_SECONDS=120 KEEP_DATA=1 BASE_URL=https://nthu-team13.duckdns.org/v1 ./load-test/run_k8s_load_test.sh

```

---

## Parameters

All configuration variables can be passed inline before the script execution path:

| Variable | Meaning | Default |
| --- | --- | --- |
| **`BASE_URL`** | **Crucial for K8s:** Targeted routing URL pointing to your public ingress gateway | `http://localhost:8001/v1` |
| `TOTAL_USERS` | Number of fake employees inserted into DB | `2000` |
| `TOTAL_QUOTA` | Ticket quota for the test ticket type | `50000` |
| `MAX_TICKETS_PER_PERSON` | Per-user ticket limit | `1` |
| `VUS` | k6 virtual users | `1000` |
| `ITERATIONS` | Booking attempts per VU | `1` |
| `MAX_DURATION` | k6 max duration | `3m` |
| `KEEP_DATA` | Keep DB rows after test for manual inspection (`0` triggers target deletion) | `1` |
| `HTTP_TIMEOUT` | Network timeout threshold for remote API evaluation | `30s` |
| `SETTLE_SECONDS` | Wait buffer to let asynchronous consumers drain Redis streams before metric verification | `0` |
| `RATE` | Requests per second (RPS) under constant arrival rates | `100` |
| `TIME_UNIT` | Base duration fraction for the defined arrival pacing | `1s` |
| `DURATION` | Total execution timespan for scenario runners | `1m` |
| `PRE_ALLOCATED_VUS` | Initial warm worker state footprint size for arrival modes | `100` |
| `MAX_VUS` | Allocation limit caps under sudden capacity expansions | `200` |
| `READ_DETAIL_RATIO` | Percentage weighting configuration for browse activities | `0.3` |
| `MIXED_BOOKING_RATIO` | Percentage weighting configuration for purchase interactions | `0.1` |
| `TICKET_QUEUE_STREAM` | Target Redis Stream topic identifier for applications routing | `ticket:applications` |
| `TICKET_QUEUE_GROUP` | Designated consumer group identity for worker matching | `ticket-workers` |

---

## Output files

Each result folder contains:

```text
book-summary.json   k6 machine-readable summary
k6-console.log      full k6 console output, including sampled error bodies
db-summary.txt      Verification metrics harvested via kubectl exec from Postgres/Redis
backend-tail.log    Standard output dump from target Go API pod replica
postgres-tail.log   Database infrastructure diagnostic logs from K8s cluster
redis-tail.log      In-memory store queue buffer runtime logs from K8s cluster
stress_env.json     IDs used by this run
parameters.env      run parameters

```

---

## Important K8s interpretation & safety

* **No Overwriting / No Corruptions**: This script entirely discards the destructive `TRUNCATE TABLE ... CASCADE;` statement. Data cleanup now switches to precision scopes via `DELETE WHERE event_id = '${EVENT_ID}'`. Concurrent manual testing from frontend remains unaffected.
* **Dynamic Identification**: If the shell reports target errors, check `kubectl get pods` to ensure labels (`app=postgres` and `app=redis`) are matched appropriately.
* **Budget Preservation**: To prevent accidental consumption of Azure for Students credits, always shut down your underlying node pools via `az aks stop` immediately after harvesting the results zip file.