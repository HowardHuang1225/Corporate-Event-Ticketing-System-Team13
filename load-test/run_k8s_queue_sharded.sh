#!/usr/bin/env bash
set -uo pipefail

cd "$(dirname "$0")/.." || exit 1
REPO_ROOT="$(pwd -P)"

GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

HTTP_TIMEOUT=${HTTP_TIMEOUT:-30s}
TOTAL_USERS=${TOTAL_USERS:-6000}
TOTAL_QUOTA=${TOTAL_QUOTA:-$TOTAL_USERS}
MAX_TICKETS_PER_PERSON=${MAX_TICKETS_PER_PERSON:-1}
ITERATIONS=${ITERATIONS:-1}
MAX_DURATION=${MAX_DURATION:-5m}
BASE_URL=${BASE_URL:-http://localhost:8001/v1}
BACKEND_URLS=${BACKEND_URLS:-$BASE_URL}
NUM_BACKENDS=${NUM_BACKENDS:-1}
SHARDS_PER_BACKEND=${SHARDS_PER_BACKEND:-7}
KEEP_DATA=${KEEP_DATA:-1}
RESET_QUEUE=${RESET_QUEUE:-1}
STRICT_THRESHOLDS=${STRICT_THRESHOLDS:-0}
FAIL_ON_THRESHOLD=${FAIL_ON_THRESHOLD:-0}
ERROR_SAMPLE_RATE=${ERROR_SAMPLE_RATE:-0.001}
DISCARD_RESPONSE_BODIES=${DISCARD_RESPONSE_BODIES:-1}
K6_SCRIPT=${K6_SCRIPT:-book-ticket.js}
TICKET_QUEUE_STREAM=${TICKET_QUEUE_STREAM:-ticket:applications}
TICKET_QUEUE_GROUP=${TICKET_QUEUE_GROUP:-ticket-workers}
POSTGRES_USER=${POSTGRES_USER:-ts_user}
POSTGRES_DB=${POSTGRES_DB:-ticketing_system}
DRAIN_TIMEOUT_SECONDS=${DRAIN_TIMEOUT_SECONDS:-900}
DRAIN_POLL_SECONDS=${DRAIN_POLL_SECONDS:-2}
CLEANUP_ON_DRAIN_TIMEOUT=${CLEANUP_ON_DRAIN_TIMEOUT:-0}
RUN_LABEL=${RUN_LABEL:-booking}

RUN_ID=${RUN_ID:-$(date +%Y%m%d-%H%M%S)-${RUN_LABEL}-u${TOTAL_USERS}-b${NUM_BACKENDS}-s${SHARDS_PER_BACKEND}}
RESULT_DIR="${REPO_ROOT}/load-test/results/${RUN_ID}"
LATEST_DIR="${REPO_ROOT}/load-test/results/latest"
ZIP_PATH="${RESULT_DIR}.zip"
SHARD_ROOT="${RESULT_DIR}/shards"

IFS=',' read -r -a BACKEND_URL_ARRAY <<< "$BACKEND_URLS"
if [ "${#BACKEND_URL_ARRAY[@]}" -lt "$NUM_BACKENDS" ]; then
  echo -e "${RED}BACKEND_URLS must contain at least NUM_BACKENDS comma-separated URLs.${NC}" >&2
  exit 2
fi

POSTGRES_POD=$(kubectl get pods -l app=postgres -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")
REDIS_POD=$(kubectl get pods -l app=redis -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")
if [ -z "$POSTGRES_POD" ] || [ -z "$REDIS_POD" ]; then
  echo -e "${RED}Error: Postgres or Redis pod not found in cluster.${NC}" >&2
  exit 1
fi

mkdir -p "$SHARD_ROOT"

redis_value() {
  kubectl exec "$REDIS_POD" -- redis-cli "$@" 2>/dev/null | tr -d '\r' || true
}

redis_pending_summary() {
  kubectl exec "$REDIS_POD" -- redis-cli XPENDING "$TICKET_QUEUE_STREAM" "$TICKET_QUEUE_GROUP" 2>/dev/null | tr -d '\r' | tr '\n' ' ' || true
}

redis_pending_count() {
  local raw first
  raw=$(kubectl exec "$REDIS_POD" -- redis-cli XPENDING "$TICKET_QUEUE_STREAM" "$TICKET_QUEUE_GROUP" 2>/dev/null | tr -d '\r' || true)
  first=$(printf '%s\n' "$raw" | head -n 1)
  if [[ "$first" =~ ^[0-9]+$ ]]; then
    echo "$first"
  else
    echo "0"
  fi
}

db_scalar() {
  kubectl exec "$POSTGRES_POD" -- psql -q -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -A -c "$1" 2>/dev/null | tr -d '[:space:]' || true
}

echo -e "${BLUE}=====================================================${NC}"
echo -e "${BLUE}   K8s Queue Sharded Load Test                       ${NC}"
echo -e "${BLUE}=====================================================${NC}"
echo "RUN_ID=${RUN_ID}"
echo "RESULT_DIR=${RESULT_DIR}"
echo "TOTAL_USERS=${TOTAL_USERS}"
echo "NUM_BACKENDS=${NUM_BACKENDS}"
echo "SHARDS_PER_BACKEND=${SHARDS_PER_BACKEND}"
echo "BACKEND_URLS=${BACKEND_URLS}"

cat > "$RESULT_DIR/parameters.env" <<PARAMS
RUN_ID=${RUN_ID}
TOTAL_USERS=${TOTAL_USERS}
TOTAL_QUOTA=${TOTAL_QUOTA}
MAX_TICKETS_PER_PERSON=${MAX_TICKETS_PER_PERSON}
NUM_BACKENDS=${NUM_BACKENDS}
SHARDS_PER_BACKEND=${SHARDS_PER_BACKEND}
ITERATIONS=${ITERATIONS}
MAX_DURATION=${MAX_DURATION}
BASE_URL=${BASE_URL}
BACKEND_URLS=${BACKEND_URLS}
KEEP_DATA=${KEEP_DATA}
RESET_QUEUE=${RESET_QUEUE}
STRICT_THRESHOLDS=${STRICT_THRESHOLDS}
FAIL_ON_THRESHOLD=${FAIL_ON_THRESHOLD}
ERROR_SAMPLE_RATE=${ERROR_SAMPLE_RATE}
DISCARD_RESPONSE_BODIES=${DISCARD_RESPONSE_BODIES}
K6_SCRIPT=${K6_SCRIPT}
TICKET_QUEUE_STREAM=${TICKET_QUEUE_STREAM}
TICKET_QUEUE_GROUP=${TICKET_QUEUE_GROUP}
HTTP_TIMEOUT=${HTTP_TIMEOUT}
DRAIN_TIMEOUT_SECONDS=${DRAIN_TIMEOUT_SECONDS}
DRAIN_POLL_SECONDS=${DRAIN_POLL_SECONDS}
CLEANUP_ON_DRAIN_TIMEOUT=${CLEANUP_ON_DRAIN_TIMEOUT}
PARAMS

echo -e "\n${YELLOW}[1/8] Generating test data ...${NC}"
RUN_ID="$RUN_ID" TOTAL_USERS="$TOTAL_USERS" TOTAL_QUOTA="$TOTAL_QUOTA" MAX_TICKETS_PER_PERSON="$MAX_TICKETS_PER_PERSON" \
  node load-test/generate_data.js | tee "$RESULT_DIR/generate-data.log"
cp load-test/setup_db.sql "$RESULT_DIR/setup_db.sql"
cp load-test/stress_env.json "$RESULT_DIR/stress_env.json"

EVENT_ID=$(node -e "console.log(require('./load-test/stress_env.json').event_id)")
TICKET_TYPE_ID=$(node -e "console.log(require('./load-test/stress_env.json').ticket_type_id)")

echo -e "\n${YELLOW}[2/8] Inserting data into K8s PostgreSQL ...${NC}"
kubectl exec -i "$POSTGRES_POD" -- psql -q -U "$POSTGRES_USER" -d "$POSTGRES_DB" < load-test/setup_db.sql | tee "$RESULT_DIR/db-insert.log"

PRE_RESET_STREAM_LEN=$(redis_value XLEN "$TICKET_QUEUE_STREAM")
PRE_RESET_PENDING=$(redis_pending_summary)
{
  echo "pre_reset_stream_length=${PRE_RESET_STREAM_LEN:-NULL}"
  echo "pre_reset_pending=${PRE_RESET_PENDING:-NULL}"
} | tee "$RESULT_DIR/pre-reset-queue.txt"

if [ "$RESET_QUEUE" = "1" ]; then
  echo -e "\n${YELLOW}[3/8] Resetting Redis stream before this run ...${NC}"
  redis_value DEL "$TICKET_QUEUE_STREAM" >/dev/null
  sleep 1
else
  echo -e "\n${YELLOW}[3/8] RESET_QUEUE=0, leaving existing Redis stream intact ...${NC}"
fi

STREAM_LEN_BEFORE=$(redis_value XLEN "$TICKET_QUEUE_STREAM")
STREAM_LEN_BEFORE=${STREAM_LEN_BEFORE:-0}

echo -e "\n${YELLOW}[4/8] Running sharded k6 ...${NC}"
TOTAL_SHARDS=$((NUM_BACKENDS * SHARDS_PER_BACKEND))
OFFSET=0
SHARD_NO=0
pids=()

for ((b = 0; b < NUM_BACKENDS; b++)); do
  ENDPOINT="${BACKEND_URL_ARRAY[$b]}"
  BACKEND_TOTAL=$((TOTAL_USERS / NUM_BACKENDS))
  if [ "$b" -lt $((TOTAL_USERS % NUM_BACKENDS)) ]; then
    BACKEND_TOTAL=$((BACKEND_TOTAL + 1))
  fi
  BASE_COUNT=$((BACKEND_TOTAL / SHARDS_PER_BACKEND))
  REMAINDER=$((BACKEND_TOTAL % SHARDS_PER_BACKEND))

  for ((j = 0; j < SHARDS_PER_BACKEND; j++)); do
    SHARD_NO=$((SHARD_NO + 1))
    COUNT=$BASE_COUNT
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

    (
      cd "$SHARD_DIR" || exit 1
      BASE_URL="$ENDPOINT" \
      VUS="$COUNT" \
      ITERATIONS="$ITERATIONS" \
      MAX_DURATION="$MAX_DURATION" \
      STRICT_THRESHOLDS="$STRICT_THRESHOLDS" \
      ERROR_SAMPLE_RATE="$ERROR_SAMPLE_RATE" \
      HTTP_TIMEOUT="$HTTP_TIMEOUT" \
      DISCARD_RESPONSE_BODIES="$DISCARD_RESPONSE_BODIES" \
        k6 run --compatibility-mode=base --summary-export=book-summary.json book-ticket.js > k6.out.log 2> k6.err.log
    ) &

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

node load-test/aggregate_k6_shards.js "$RESULT_DIR" "$TOTAL_SHARDS" > "$RESULT_DIR/k6-aggregate.txt"
EXPECTED_SUCCESS=$(awk -F= '/^total_success=/{print $2}' "$RESULT_DIR/k6-aggregate.txt" | tail -n 1)
EXPECTED_SUCCESS=${EXPECTED_SUCCESS:-0}

echo -e "\n${YELLOW}[5/8] Waiting for worker drain ...${NC}"
DRAIN_OK=0
DRAIN_START=$SECONDS
DRAIN_DEADLINE=$((SECONDS + DRAIN_TIMEOUT_SECONDS))
while [ "$SECONDS" -lt "$DRAIN_DEADLINE" ]; do
  APPLICATIONS_COUNT=$(db_scalar "SELECT COUNT(*) FROM applications WHERE event_id = '${EVENT_ID}';")
  TICKETS_COUNT=$(db_scalar "SELECT COUNT(*) FROM tickets WHERE event_id = '${EVENT_ID}';")
  STREAM_LEN_NOW=$(redis_value XLEN "$TICKET_QUEUE_STREAM")
  PENDING_COUNT=$(redis_pending_count)

  APPLICATIONS_COUNT=${APPLICATIONS_COUNT:-0}
  TICKETS_COUNT=${TICKETS_COUNT:-0}
  STREAM_LEN_NOW=${STREAM_LEN_NOW:-0}
  PENDING_COUNT=${PENDING_COUNT:-0}

  printf 'drain applications=%s tickets=%s expected=%s stream=%s pending=%s\n' \
    "$APPLICATIONS_COUNT" "$TICKETS_COUNT" "$EXPECTED_SUCCESS" "$STREAM_LEN_NOW" "$PENDING_COUNT" | tee -a "$RESULT_DIR/drain.log"

  if [ "$APPLICATIONS_COUNT" -ge "$EXPECTED_SUCCESS" ] &&
     [ "$TICKETS_COUNT" -ge "$EXPECTED_SUCCESS" ] &&
     [ "$STREAM_LEN_NOW" = "0" ] &&
     [ "$PENDING_COUNT" = "0" ]; then
    DRAIN_OK=1
    break
  fi

  sleep "$DRAIN_POLL_SECONDS"
done
DRAIN_ELAPSED=$((SECONDS - DRAIN_START))

echo -e "\n${YELLOW}[6/8] Querying final DB and Redis metrics ...${NC}"
{
  echo "run_id=${RUN_ID}"
  echo "topology=k8s-${NUM_BACKENDS}backend-${TOTAL_SHARDS}shards"
  echo "event_id=${EVENT_ID}"
  echo "ticket_type_id=${TICKET_TYPE_ID}"
  echo "drain_ok=${DRAIN_OK}"
  echo "drain_elapsed_seconds=${DRAIN_ELAPSED}"
  echo "drain_expected_success=${EXPECTED_SUCCESS}"
  cat "$RESULT_DIR/k6-aggregate.txt"

  kubectl exec "$POSTGRES_POD" -- psql -q -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -A -F '=' -c "
SELECT 'applications_count', COUNT(*) FROM applications WHERE event_id = '${EVENT_ID}'
UNION ALL SELECT 'tickets_count', COUNT(*) FROM tickets WHERE event_id = '${EVENT_ID}'
UNION ALL SELECT 'approved_applications_count', COUNT(*) FROM applications WHERE event_id = '${EVENT_ID}' AND status = 'approved'
UNION ALL SELECT 'pending_applications_count', COUNT(*) FROM applications WHERE event_id = '${EVENT_ID}' AND status = 'pending'
UNION ALL SELECT 'ticket_type_total_quota', total_quota FROM ticket_types WHERE id = '${TICKET_TYPE_ID}'
UNION ALL SELECT 'ticket_type_remaining', remaining FROM ticket_types WHERE id = '${TICKET_TYPE_ID}'
UNION ALL SELECT 'ticket_type_consumed_by_db', total_quota - remaining FROM ticket_types WHERE id = '${TICKET_TYPE_ID}';
" | sed '/^$/d'

  REDIS_INV=$(redis_value GET "inventory:${TICKET_TYPE_ID}")
  STREAM_LEN_AFTER=$(redis_value XLEN "$TICKET_QUEUE_STREAM")
  PENDING_SUMMARY=$(redis_pending_summary)
  echo "redis_inventory=${REDIS_INV:-NULL}"
  echo "redis_stream_length_before=${STREAM_LEN_BEFORE:-NULL}"
  echo "redis_stream_length_after=${STREAM_LEN_AFTER:-NULL}"
  echo "redis_stream_pending=${PENDING_SUMMARY:-NULL}"
} | tee "$RESULT_DIR/db-summary.txt"

echo -e "\n${YELLOW}[6.5/8] Archiving pod logs ...${NC}"
BACKEND_PODS=($(kubectl get pods -l app=backend -o jsonpath='{.items[*].metadata.name}' 2>/dev/null || echo ""))
for pod in "${BACKEND_PODS[@]}"; do
  kubectl logs --tail=500 "$pod" > "$RESULT_DIR/backend-${pod}.log" 2>&1 || true
done
kubectl logs --tail=300 "$POSTGRES_POD" > "$RESULT_DIR/postgres-tail.log" 2>&1 || true
kubectl logs --tail=200 "$REDIS_POD" > "$RESULT_DIR/redis-tail.log" 2>&1 || true

echo -e "\n${YELLOW}[7/8] Generating analysis report ...${NC}"
TOTAL_USERS="$TOTAL_USERS" TOTAL_QUOTA="$TOTAL_QUOTA" MAX_TICKETS_PER_PERSON="$MAX_TICKETS_PER_PERSON" \
  BASE_URL="$BACKEND_URLS" node load-test/analyze_results.js "$RESULT_DIR"

if [[ "$KEEP_DATA" != "1" ]]; then
  if [ "$DRAIN_OK" = "1" ] || [ "$CLEANUP_ON_DRAIN_TIMEOUT" = "1" ]; then
    echo -e "\n${YELLOW}[7.5/8] Cleaning scoped test data ...${NC}"
    kubectl exec -i "$POSTGRES_POD" -- psql -q -U "$POSTGRES_USER" -d "$POSTGRES_DB" <<SQL | tee "$RESULT_DIR/db-cleanup.log"
DELETE FROM checkins WHERE ticket_id IN (SELECT id FROM tickets WHERE event_id = '${EVENT_ID}');
DELETE FROM tickets WHERE event_id = '${EVENT_ID}';
DELETE FROM applications WHERE event_id = '${EVENT_ID}';
DELETE FROM ticket_types WHERE event_id = '${EVENT_ID}';
DELETE FROM events WHERE id = '${EVENT_ID}';
DELETE FROM users WHERE email LIKE 'stress_${RUN_ID}_%' OR email = 'stress_mgr_${RUN_ID}@company.com';
SQL
    redis_value DEL \
      "inventory:${TICKET_TYPE_ID}" \
      "inventory_loaded:${TICKET_TYPE_ID}" \
      "queue:ticket_type_meta:${TICKET_TYPE_ID}" \
      "lock:init_lock:${TICKET_TYPE_ID}" >/dev/null
  else
    echo "Skipped cleanup because drain did not finish. Set CLEANUP_ON_DRAIN_TIMEOUT=1 to force cleanup." | tee "$RESULT_DIR/cleanup-skipped.log"
  fi
else
  echo -e "\n${YELLOW}[7.5/8] KEEP_DATA=1, preserving test data for inspection.${NC}"
fi

echo -e "\n${YELLOW}[8/8] Creating latest copy and archive ...${NC}"
rm -rf "$LATEST_DIR"
mkdir -p "$(dirname "$LATEST_DIR")"
cp -R "$RESULT_DIR" "$LATEST_DIR"

ARCHIVE_PATH="$ZIP_PATH"
if command -v zip >/dev/null 2>&1; then
  (cd "$(dirname "$RESULT_DIR")" && zip -qr "$(basename "$ZIP_PATH")" "$(basename "$RESULT_DIR")")
elif command -v tar >/dev/null 2>&1; then
  ARCHIVE_PATH="${RESULT_DIR}.tar.gz"
  (cd "$(dirname "$RESULT_DIR")" && tar -czf "$(basename "$ARCHIVE_PATH")" "$(basename "$RESULT_DIR")")
else
  ARCHIVE_PATH=""
fi

echo -e "\n${GREEN}Pipeline complete.${NC}"
echo "Metrics folder: ${RESULT_DIR}"
echo "Latest copy: ${LATEST_DIR}"
if [[ -n "$ARCHIVE_PATH" ]]; then
  echo "Archive: ${ARCHIVE_PATH}"
fi

K6_EXIT=$(awk -F= '/^shard/ {if($2!=0) exitcode=1} END{print exitcode+0}' "$RESULT_DIR/k6-exit-code.txt" 2>/dev/null || echo 0)
if (( K6_EXIT != 0 && FAIL_ON_THRESHOLD == 1 )); then
  exit "$K6_EXIT"
fi
if (( DRAIN_OK != 1 )); then
  exit 3
fi
