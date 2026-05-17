# Waiting Room + Redis Stream Queue Design

This project keeps the original synchronous booking path by default. For high-concurrency experiments, set `TICKET_QUEUE_ENABLED=true` to enable a queue-based ticket application path.

## Why this exists

The synchronous path is correct but high-latency under flash-sale style traffic because many requests eventually update the same `ticket_types.remaining` row. The queue path changes the peak-time behavior:

1. API validates the basic request.
2. Redis atomically reserves inventory and appends the application job to a Redis Stream.
3. API returns `202 Accepted` with application status `queued`.
4. Worker goroutines consume the Redis Stream and write approved applications/tickets into PostgreSQL at a controlled rate.
5. The employee can check queue status via `GET /v1/applications/queue/{idempotency_key}` or later see the final ticket/application state.

This is a simplified implementation of Queue-Based Load Leveling. It does not replace a real CDN/vendor waiting room, but it demonstrates the same cloud-native idea: absorb burst traffic quickly and let workers process database writes at a sustainable speed.

## Environment variables

```env
TICKET_QUEUE_ENABLED=true
TICKET_QUEUE_STREAM=ticket:applications
TICKET_QUEUE_GROUP=ticket-workers
TICKET_QUEUE_WORKERS=8
TICKET_QUEUE_MAX_WAITING=100000
TICKET_QUEUE_RESERVATION_TTL_SECONDS=900
TICKET_QUEUE_STATUS_TTL_SECONDS=3600
```

## Expected API behavior

### Synchronous mode

`POST /v1/applications` returns:

- `201 Created` when a ticket is created immediately.
- `200 OK` for an idempotent duplicate request.
- `409` when sold out.

### Queue mode

`POST /v1/applications` returns:

- `202 Accepted` with status `queued` after Redis reservation succeeds.
- `200 OK` if the same idempotency key already produced a DB application.
- `409` when Redis inventory is already sold out.
- `429 WAITING_ROOM_FULL` if the Redis Stream waiting room is above `TICKET_QUEUE_MAX_WAITING`.

Check queue status:

```http
GET /v1/applications/queue/{idempotency_key}
```

Possible statuses:

- `queued`
- `processing`
- `approved`
- `failed`

## Testing suggestion

1. First run the existing synchronous baseline.
2. Then set `TICKET_QUEUE_ENABLED=true` in `.env` and recreate backend:

```bash
docker compose down
docker compose up -d --build --force-recreate
```

3. Confirm workers started:

```bash
docker compose logs backend | grep "Ticket queue workers started"
```

4. Run the high-concurrency test. In queue mode, k6 should count `202` as successful queue acceptance. Use a longer settle time so workers can drain the queue:

```bash
RUN_LABEL=5000v5000q-queue \
TOTAL_USERS=5000 \
TOTAL_QUOTA=5000 \
VUS=5000 \
ITERATIONS=1 \
MAX_DURATION=3m \
HTTP_TIMEOUT=60s \
SETTLE_SECONDS=120 \
KEEP_DATA=1 \
STRICT_THRESHOLDS=0 \
./load-test/run_load_test.sh
```

For a 50,000-user conceptual test, do not expect free-tier infrastructure to process all DB writes immediately. The important evidence is that the API can accept requests into the waiting room without 5xx, while workers process the queue steadily.
