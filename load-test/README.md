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
RUN_LABEL=100v100q TOTAL_USERS=100 TOTAL_QUOTA=100 VUS=100 ITERATIONS=1 KEEP_DATA=1 ./load-test/run_load_test.sh
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
