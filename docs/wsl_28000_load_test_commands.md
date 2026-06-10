# WSL 28000 VU Load Test Commands

Verified WSL result:

```text
RUN_ID=wsl-two-backend-28000-20260610-232007
total_reqs=28000
total_success=28000
success_rate_percent=100.0000
applications_count=28000
tickets_count=28000
redis_inventory=0
redis_stream_length=0
redis_stream_pending=0
```

Run the following commands inside a WSL shell.

## 1. Enter Repo

```bash
cd /mnt/d/code/Corporate-Event-Ticketing-System-Team13
docker --version
node --version
```

## 2. Start Compose Backend

```bash
cd /mnt/d/code/Corporate-Event-Ticketing-System-Team13

export TICKET_QUEUE_ENABLED=true
export TICKET_QUEUE_MAX_WAITING=100000
export TICKET_QUEUE_WORKERS=16

docker compose up -d --build backend
docker ps --format '{{.Names}} {{.Status}}'
```

## 3. Start Second Backend

```bash
cd /mnt/d/code/Corporate-Event-Ticketing-System-Team13
bash load-test/start_wsl_backend_2.sh
```

## 4. Check Both Backends

```bash
docker run --rm --network corporate-event-ticketing-system-team13_default curlimages/curl:latest -fsS http://backend:8001/health
docker run --rm --network corporate-event-ticketing-system-team13_default curlimages/curl:latest -fsS http://ts_backend_wsl_2:8001/health
```

## 5. Run 28000 VU Test

```bash
cd /mnt/d/code/Corporate-Event-Ticketing-System-Team13
bash load-test/run_wsl_28000.sh
```

Optional overrides:

```bash
TOTAL=28000 SHARDS_PER_BACKEND=7 HTTP_TIMEOUT=30s bash load-test/run_wsl_28000.sh
```

要測試超賣的話，可以去修改 load-test/run_wsl_28000.sh 裡面的 TOTAL_QUOTA (line 56)

## 6. Read Results

```bash
cd /mnt/d/code/Corporate-Event-Ticketing-System-Team13

RESULT_DIR=$(cat load-test/results/latest-wsl-28000.txt)
cat "$RESULT_DIR/k6-exit-code.txt"
cat "$RESULT_DIR/db-summary.txt"
cat "$RESULT_DIR/book-summary.json"
```

## 7. Stop Second Backend

```bash
docker stop ts_backend_wsl_2 >/dev/null 2>&1 || true
```
