#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."
REPO_ROOT="$(pwd -P)"

TOTAL="${TOTAL:-28000}"
SHARDS_PER_BACKEND="${SHARDS_PER_BACKEND:-7}"
NETWORK="${NETWORK:-corporate-event-ticketing-system-team13_default}"
BACKEND_1_URL="${BACKEND_1_URL:-http://backend:8001/v1}"
BACKEND_2_URL="${BACKEND_2_URL:-http://ts_backend_wsl_2:8001/v1}"
BACKEND_1_HEALTH="${BACKEND_1_HEALTH:-http://backend:8001/health}"
BACKEND_2_HEALTH="${BACKEND_2_HEALTH:-http://ts_backend_wsl_2:8001/health}"
HTTP_TIMEOUT="${HTTP_TIMEOUT:-30s}"
K6_IMAGE="${K6_IMAGE:-grafana/k6:latest}"
TICKET_QUEUE_STREAM="${TICKET_QUEUE_STREAM:-ticket:applications}"
TICKET_QUEUE_GROUP="${TICKET_QUEUE_GROUP:-ticket-workers}"
SETTLE_TIMEOUT_SECONDS="${SETTLE_TIMEOUT_SECONDS:-660}"
RUN_ID="${RUN_ID:-wsl-two-backend-${TOTAL}-$(date +%Y%m%d-%H%M%S)}"
RESULT_DIR_INPUT="${RESULT_DIR:-load-test/results/${RUN_ID}}"
case "$RESULT_DIR_INPUT" in
  /*) RESULT_DIR="$RESULT_DIR_INPUT" ;;
  *) RESULT_DIR="${REPO_ROOT}/${RESULT_DIR_INPUT}" ;;
esac
SHARD_ROOT="${RESULT_DIR}/shards"

if [ $((TOTAL % 2)) -ne 0 ]; then
  echo "TOTAL must be even for two-backend split: ${TOTAL}" >&2
  exit 2
fi

mkdir -p "$SHARD_ROOT"
echo "$RESULT_DIR" > "${REPO_ROOT}/load-test/results/latest-wsl-28000.txt"

echo "RUN_ID=$RUN_ID"
echo "RESULT_DIR=$RESULT_DIR"

{
  echo "backend_1=${BACKEND_1_HEALTH}"
  docker run --rm --network "$NETWORK" curlimages/curl:latest -fsS "$BACKEND_1_HEALTH"
  echo
  echo "backend_2=${BACKEND_2_HEALTH}"
  docker run --rm --network "$NETWORK" curlimages/curl:latest -fsS "$BACKEND_2_HEALTH"
  echo
} | tee "$RESULT_DIR/health.log"

BACKEND_1_SECRET=$(docker inspect ts_backend --format '{{range .Config.Env}}{{println .}}{{end}}' | sed -n 's/^JWT_SECRET=//p' | tail -n 1)
BACKEND_2_SECRET=$(docker inspect ts_backend_wsl_2 --format '{{range .Config.Env}}{{println .}}{{end}}' | sed -n 's/^JWT_SECRET=//p' | tail -n 1)
if [ "$BACKEND_1_SECRET" != "$BACKEND_2_SECRET" ]; then
  echo "JWT_SECRET mismatch between ts_backend and ts_backend_wsl_2. Restart backend 2 with load-test/start_wsl_backend_2.sh." >&2
  exit 2
fi

export RUN_ID
export TOTAL_USERS="$TOTAL"
export TOTAL_QUOTA="$TOTAL"
export MAX_TICKETS_PER_PERSON="${MAX_TICKETS_PER_PERSON:-1}"

node load-test/generate_data.js | tee "$RESULT_DIR/generate-data.log"
cp load-test/setup_db.sql "$RESULT_DIR/setup_db.sql"
cp load-test/stress_env.json "$RESULT_DIR/stress_env.json"

EVENT_ID=$(node -e "console.log(require('./load-test/stress_env.json').event_id)")
TICKET_TYPE_ID=$(node -e "console.log(require('./load-test/stress_env.json').ticket_type_id)")

docker exec -i ts_postgres psql -q -U ts_user -d ticketing_system < load-test/setup_db.sql | tee "$RESULT_DIR/db-insert.log"
docker exec ts_redis redis-cli DEL "$TICKET_QUEUE_STREAM" >/dev/null
sleep 1

TOTAL_SHARDS=$((SHARDS_PER_BACKEND * 2))
PER_BACKEND=$((TOTAL / 2))
BASE=$((PER_BACKEND / SHARDS_PER_BACKEND))
REMAINDER=$((PER_BACKEND % SHARDS_PER_BACKEND))
OFFSET=0
SHARD_NO=0
pids=()

for ENDPOINT_INDEX in 0 1; do
  if [ "$ENDPOINT_INDEX" -eq 0 ]; then
    ENDPOINT="$BACKEND_1_URL"
  else
    ENDPOINT="$BACKEND_2_URL"
  fi

  for ((j = 0; j < SHARDS_PER_BACKEND; j++)); do
    SHARD_NO=$((SHARD_NO + 1))
    COUNT=$BASE
    if [ "$j" -lt "$REMAINDER" ]; then
      COUNT=$((COUNT + 1))
    fi

    SHARD_DIR="${SHARD_ROOT}/shard${SHARD_NO}"
    mkdir -p "$SHARD_DIR"
    cp load-test/book-ticket.js "$SHARD_DIR/book-ticket.js"
    echo "$ENDPOINT" > "$SHARD_DIR/endpoint.txt"

    node - "$RESULT_DIR/stress_env.json" "$SHARD_DIR/stress_env.json" "$RUN_ID-shard$SHARD_NO" "$OFFSET" "$COUNT" <<'NODE'
const fs = require('fs');
const [src, dst, runId, startText, countText] = process.argv.slice(2);
const data = JSON.parse(fs.readFileSync(src, 'utf8'));
const start = Number(startText);
const count = Number(countText);
data.run_id = runId;
data.total_users = count;
data.users = data.users.slice(start, start + count);
fs.writeFileSync(dst, JSON.stringify(data, null, 2));
NODE

    docker run --rm \
      --network "$NETWORK" \
      -v "${SHARD_DIR}:/scripts" \
      -w /scripts \
      -e "BASE_URL=${ENDPOINT}" \
      -e "VUS=${COUNT}" \
      -e ITERATIONS=1 \
      -e MAX_DURATION=5m \
      -e STRICT_THRESHOLDS=0 \
      -e ERROR_SAMPLE_RATE=0.001 \
      -e DISCARD_RESPONSE_BODIES=1 \
      -e "HTTP_TIMEOUT=${HTTP_TIMEOUT}" \
      "$K6_IMAGE" \
      run \
      --compatibility-mode=base \
      --summary-export /scripts/book-summary.json \
      book-ticket.js > "$SHARD_DIR/k6.out.log" 2> "$SHARD_DIR/k6.err.log" &

    pids+=("$!")
    OFFSET=$((OFFSET + COUNT))
  done
done

: > "$RESULT_DIR/k6-exit-code.txt"
for i in "${!pids[@]}"; do
  shard=$((i + 1))
  if wait "${pids[$i]}"; then
    code=0
  else
    code=$?
  fi
  echo "shard${shard}=${code}" | tee -a "$RESULT_DIR/k6-exit-code.txt"
done

cd "$REPO_ROOT"
node "$REPO_ROOT/load-test/aggregate_k6_shards.js" "$RESULT_DIR" "$TOTAL_SHARDS" > "$RESULT_DIR/k6-aggregate.txt"

EXPECTED_TICKETS=$(awk -F= '/^total_success=/{print $2}' "$RESULT_DIR/k6-aggregate.txt")
EXPECTED_TICKETS="${EXPECTED_TICKETS:-0}"

DEADLINE=$((SECONDS + SETTLE_TIMEOUT_SECONDS))
while [ "$SECONDS" -lt "$DEADLINE" ]; do
  STREAM_LEN=$(docker exec ts_redis redis-cli XLEN "$TICKET_QUEUE_STREAM" | tr -d '\r')
  PENDING=$(docker exec ts_redis redis-cli XPENDING "$TICKET_QUEUE_STREAM" "$TICKET_QUEUE_GROUP" | tr -d '\r' | tr '\n' ' ')
  TICKETS=$(docker exec ts_postgres psql -q -U ts_user -d ticketing_system -t -A -c "SELECT COUNT(*) FROM tickets WHERE event_id = '${EVENT_ID}';" | tr -d '[:space:]')
  if [ "$STREAM_LEN" = "0" ] && [[ "$PENDING" == 0* ]] && [ "$TICKETS" -ge "$EXPECTED_TICKETS" ]; then
    break
  fi
  sleep 2
done

DB_SUMMARY=$(docker exec ts_postgres psql -q -U ts_user -d ticketing_system -t -A -F '=' -c "SELECT 'applications_count', COUNT(*) FROM applications WHERE event_id = '${EVENT_ID}' UNION ALL SELECT 'tickets_count', COUNT(*) FROM tickets WHERE event_id = '${EVENT_ID}' UNION ALL SELECT 'approved_applications_count', COUNT(*) FROM applications WHERE event_id = '${EVENT_ID}' AND status = 'approved' UNION ALL SELECT 'pending_applications_count', COUNT(*) FROM applications WHERE event_id = '${EVENT_ID}' AND status = 'pending' UNION ALL SELECT 'ticket_type_total_quota', total_quota FROM ticket_types WHERE id = '${TICKET_TYPE_ID}' UNION ALL SELECT 'ticket_type_remaining', remaining FROM ticket_types WHERE id = '${TICKET_TYPE_ID}';")
REDIS_INVENTORY=$(docker exec ts_redis redis-cli GET "inventory:${TICKET_TYPE_ID}" | tr -d '\r')
STREAM_LEN=$(docker exec ts_redis redis-cli XLEN "$TICKET_QUEUE_STREAM" | tr -d '\r')
PENDING=$(docker exec ts_redis redis-cli XPENDING "$TICKET_QUEUE_STREAM" "$TICKET_QUEUE_GROUP" | tr -d '\r' | tr '\n' ' ')

{
  echo "run_id=${RUN_ID}"
  echo "topology=2_backend_1_postgres"
  echo "runner=wsl"
  echo "event_id=${EVENT_ID}"
  echo "ticket_type_id=${TICKET_TYPE_ID}"
  cat "$RESULT_DIR/k6-aggregate.txt"
  echo "$DB_SUMMARY"
  echo "redis_inventory=${REDIS_INVENTORY}"
  echo "redis_stream_length=${STREAM_LEN}"
  echo "redis_stream_pending=${PENDING}"
} | tee "$RESULT_DIR/db-summary.txt"

docker logs --tail 1000 ts_backend > "$RESULT_DIR/backend1-tail.log" 2>&1 || true
docker logs --tail 1000 ts_backend_wsl_2 > "$RESULT_DIR/backend2-tail.log" 2>&1 || true

echo "RESULT_DIR=$RESULT_DIR"
