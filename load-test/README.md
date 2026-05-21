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
RUN_LABEL=2000v2000q-queue \
TOTAL_USERS=2000 \
TOTAL_QUOTA=2000 \
VUS=2000 \
ITERATIONS=1 \
MAX_DURATION=3m \
HTTP_TIMEOUT=60s \
SETTLE_SECONDS=120 \
KEEP_DATA=0 \
STRICT_THRESHOLDS=0 \
./load-test/run_load_test.sh
```

For larger conceptual tests such as 50,000 users, increase `TOTAL_USERS`, `TOTAL_QUOTA`, `VUS`, and `SETTLE_SECONDS`. In queue mode, the first metric to watch is not immediate DB completion; it is whether requests enter the waiting room without 5xx. Then inspect DB/Redis after enough settle time to confirm the worker pool drains the queue safely.

## Constant Arrival Rate Test

```bash
TEST_MODE=constant \
RATE=1000 \
MAX_DURATION=5s \
PRE_ALLOCATED_VUS=1000 \
MAX_VUS=2000 \
RUN_LABEL=3s-burst-test \
TOTAL_USERS=2000 \
TOTAL_QUOTA=5000 \
HTTP_TIMEOUT=60s \
SETTLE_SECONDS=10 \
KEEP_DATA=0 \
STRICT_THRESHOLDS=0 \
MAX_TICKETS_PER_PERSON=10 \
./load-test/run_load_test.sh
```