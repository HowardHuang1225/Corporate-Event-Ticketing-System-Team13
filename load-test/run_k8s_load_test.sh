#!/usr/bin/env bash
set -uo pipefail

cd "$(dirname "$0")/.." || exit 1

GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

HTTP_TIMEOUT=${HTTP_TIMEOUT:-30s}
SETTLE_SECONDS=${SETTLE_SECONDS:-0}
TOTAL_USERS=${TOTAL_USERS:-5000}
TOTAL_QUOTA=${TOTAL_QUOTA:-5000}
MAX_TICKETS_PER_PERSON=${MAX_TICKETS_PER_PERSON:-1}
ITERATIONS=${ITERATIONS:-1}
MAX_DURATION=${MAX_DURATION:-5m}
KEEP_DATA=${KEEP_DATA:-0}
STRICT_THRESHOLDS=${STRICT_THRESHOLDS:-0}
FAIL_ON_THRESHOLD=${FAIL_ON_THRESHOLD:-0}
ERROR_SAMPLE_RATE=${ERROR_SAMPLE_RATE:-0.001}
DISCARD_RESPONSE_BODIES=${DISCARD_RESPONSE_BODIES:-1}
K6_SCRIPT=${K6_SCRIPT:-book-ticket.js}
TICKET_QUEUE_STREAM=${TICKET_QUEUE_STREAM:-ticket:applications}
TICKET_QUEUE_GROUP=${TICKET_QUEUE_GROUP:-ticket-workers}
RUN_LABEL=${RUN_LABEL:-booking}

NUM_BACKENDS=${NUM_BACKENDS:-1}                                      
BACKEND_URLS=${BACKEND_URLS:-"http://backend:8001/v1"}             
SHARDS_PER_BACKEND=${SHARDS_PER_BACKEND:-7}

# RUN_ID generation
RUN_ID=${RUN_ID:-$(date +%Y%m%d-%H%M%S)-${RUN_LABEL}-u${TOTAL_USERS}-b${NUM_BACKENDS}-s${SHARDS_PER_BACKEND}}
RESULT_DIR="load-test/results/${RUN_ID}"
LATEST_DIR="load-test/results/latest"
ZIP_PATH="${RESULT_DIR}.zip"

# Locate K8s pods
POSTGRES_POD=$(kubectl get pods -l app=postgres -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")
REDIS_POD=$(kubectl get pods -l app=redis -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")

if [ -z "$POSTGRES_POD" ] || [ -z "$REDIS_POD" ]; then
  echo -e "${RED}Error: Postgres or Redis pod not found in cluster.${NC}"
  exit 1
fi

mkdir -p "$RESULT_DIR"
SHARD_ROOT="${RESULT_DIR}/shards"
mkdir -p "$SHARD_ROOT"

if (( VUS > TOTAL_USERS )); then
  echo -e "${RED}Invalid test: VUS=${VUS} but TOTAL_USERS=${TOTAL_USERS}.${NC}"
  exit 2
fi

# Export test configuration
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
KEEP_DATA=${KEEP_DATA}
STRICT_THRESHOLDS=${STRICT_THRESHOLDS}
ERROR_SAMPLE_RATE=${ERROR_SAMPLE_RATE}
DISCARD_RESPONSE_BODIES=${DISCARD_RESPONSE_BODIES}
K6_SCRIPT=${K6_SCRIPT}
TICKET_QUEUE_STREAM=${TICKET_QUEUE_STREAM}
TICKET_QUEUE_GROUP=${TICKET_QUEUE_GROUP}
HTTP_TIMEOUT=${HTTP_TIMEOUT}
SETTLE_SECONDS=${SETTLE_SECONDS}
PARAMS

echo -e "${BLUE}=====================================================${NC}"
echo -e "${BLUE}   Corporate Event Ticketing System Load Test (K8s)  ${NC}"
echo -e "${BLUE}   Topology: ${NUM_BACKENDS} Backend(s) × ${SHARDS_PER_BACKEND} Shards  ${NC}"
echo -e "${BLUE}=====================================================${NC}"
echo "RUN_ID=${RUN_ID}"
echo "RESULT_DIR=${RESULT_DIR}"

# Generate data
echo -e "\n${YELLOW}[1/7] Generating testing data ...${NC}"
RUN_ID="$RUN_ID" TOTAL_USERS="$TOTAL_USERS" TOTAL_QUOTA="$TOTAL_QUOTA" MAX_TICKETS_PER_PERSON="$MAX_TICKETS_PER_PERSON" node load-test/generate_data.js | tee "$RESULT_DIR/generate-data.log"
cp load-test/setup_db.sql "$RESULT_DIR/setup_db.sql"
cp load-test/stress_env.json "$RESULT_DIR/stress_env.json"

EVENT_ID=$(node -e "console.log(require('./load-test/stress_env.json').event_id)")
TICKET_TYPE_ID=$(node -e "console.log(require('./load-test/stress_env.json').ticket_type_id)")

# Insert DB data
echo -e "\n${YELLOW}[2/7] Inserting data into K8s PostgreSQL ...${NC}"
cat load-test/setup_db.sql | kubectl exec -i $POSTGRES_POD -- psql -q -U ts_user -d ticketing_system | tee "$RESULT_DIR/db-insert.log"

sleep 2
STREAM_LEN_BEFORE=$(kubectl exec -i $REDIS_POD -- redis-cli XLEN "${TICKET_QUEUE_STREAM}" 2>/dev/null | tr -d '\r' || true)
STREAM_LEN_BEFORE=${STREAM_LEN_BEFORE:-0}

echo -e "\n${YELLOW}[3/7] Running k6 with ${NUM_BACKENDS} backends, ${SHARDS_PER_BACKEND} shards each ...${NC}"

TOTAL_SHARDS=$((SHARDS_PER_BACKEND * NUM_BACKENDS))
PER_BACKEND=$((TOTAL_USERS / NUM_BACKENDS))
BASE=$((PER_BACKEND / SHARDS_PER_BACKEND))
REMAINDER=$((PER_BACKEND % SHARDS_PER_BACKEND))

pids=()
OFFSET=0
SHARD_NO=0

for ((b=0; b<NUM_BACKENDS; b++)); do
  ENDPOINT=$(echo "$BACKEND_URLS" | cut -d',' -f$((b+1)))

  for ((j=0; j<SHARDS_PER_BACKEND; j++)); do
    SHARD_NO=$((SHARD_NO + 1))
    COUNT=$BASE
    if [ "$j" -lt "$REMAINDER" ]; then
      COUNT=$((COUNT + 1))
    fi

    SHARD_DIR="${SHARD_ROOT}/shard${SHARD_NO}"
    mkdir -p "$SHARD_DIR"
    cp load-test/book-ticket.js "$SHARD_DIR/"

    # 生成 shard 專屬 stress_env.json
    node - "$RESULT_DIR/stress_env.json" "$SHARD_DIR/stress_env.json" "${RUN_ID}-shard${SHARD_NO}" "$OFFSET" "$COUNT" <<'NODE'
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

    BASE_URL="$ENDPOINT" \
    VUS="$COUNT" \
    ITERATIONS="$ITERATIONS" \
    MAX_DURATION="$MAX_DURATION" \
    STRICT_THRESHOLDS="$STRICT_THRESHOLDS" \
    ERROR_SAMPLE_RATE="$ERROR_SAMPLE_RATE" \
    HTTP_TIMEOUT="$HTTP_TIMEOUT" \
    DISCARD_RESPONSE_BODIES="$DISCARD_RESPONSE_BODIES" \
      k6 run --compatibility-mode=base \
      --summary-export="$SHARD_DIR/book-summary.json" \
      "$SHARD_DIR/book-ticket.js" > "$SHARD_DIR/k6.out.log" 2> "$SHARD_DIR/k6.err.log" &

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

# Settle 等待
if (( SETTLE_SECONDS > 0 )); then
  echo -e "\n${YELLOW}[3.5/7] Waiting ${SETTLE_SECONDS}s for sync settle ...${NC}"
  sleep "$SETTLE_SECONDS"
fi

echo -e "\n${YELLOW}[4/7] Querying K8s DB and Redis metrics ...${NC}"
{
  echo "run_id=${RUN_ID}"
  echo "topology=k8s-${NUM_BACKENDS}backend"
  echo "event_id=${EVENT_ID}"
  echo "ticket_type_id=${TICKET_TYPE_ID}"
  cat "$RESULT_DIR/k6-aggregate.txt"
  
  kubectl exec -i $POSTGRES_POD -- psql -q -U ts_user -d ticketing_system -t -A -F '=' -c "
SELECT 'applications_count', COUNT(*) FROM applications WHERE event_id = '${EVENT_ID}'
UNION ALL SELECT 'tickets_count', COUNT(*) FROM tickets WHERE event_id = '${EVENT_ID}'
UNION ALL SELECT 'approved_applications_count', COUNT(*) FROM applications WHERE event_id = '${EVENT_ID}' AND status = 'approved'
UNION ALL SELECT 'pending_applications_count', COUNT(*) FROM applications WHERE event_id = '${EVENT_ID}' AND status = 'pending'
UNION ALL SELECT 'ticket_type_total_quota', total_quota FROM ticket_types WHERE id = '${TICKET_TYPE_ID}'
UNION ALL SELECT 'ticket_type_remaining', remaining FROM ticket_types WHERE id = '${TICKET_TYPE_ID}'
UNION ALL SELECT 'ticket_type_consumed_by_db', total_quota - remaining FROM ticket_types WHERE id = '${TICKET_TYPE_ID}';
" | sed '/^$/d'

  REDIS_INV=$(kubectl exec -i $REDIS_POD -- redis-cli GET "inventory:${TICKET_TYPE_ID}" 2>/dev/null | tr -d '\r' || true)
  echo "redis_inventory=${REDIS_INV:-NULL}"

  STREAM_LEN_AFTER=$(kubectl exec -i $REDIS_POD -- redis-cli XLEN "${TICKET_QUEUE_STREAM}" 2>/dev/null | tr -d '\r' || true)
  STREAM_LEN_AFTER=${STREAM_LEN_AFTER:-0}
  STREAM_LEN_DELTA=$((STREAM_LEN_AFTER - STREAM_LEN_BEFORE))
  echo "redis_stream_length_before=${STREAM_LEN_BEFORE:-NULL}"
  echo "redis_stream_length_after=${STREAM_LEN_AFTER}"
  echo "redis_stream_length_delta=${STREAM_LEN_DELTA}"

  PENDING_SUMMARY=$(kubectl exec -i $REDIS_POD -- redis-cli XPENDING "${TICKET_QUEUE_STREAM}" "${TICKET_QUEUE_GROUP}" 2>/dev/null | tr -d '\r' | tr '\n' ' ' || true)
  echo "redis_stream_pending=${PENDING_SUMMARY:-NULL}"
} | tee "$RESULT_DIR/db-summary.txt"

# Logs
echo -e "\n${YELLOW}[4.5/7] Archiving pod logs ...${NC}"
BACKEND_PODS=($(kubectl get pods -l app=backend -o jsonpath='{.items[*].metadata.name}' 2>/dev/null || echo ""))
for pod in "${BACKEND_PODS[@]}"; do
  kubectl logs --tail=500 "$pod" > "$RESULT_DIR/backend-${pod}.log" 2>&1 || true
done
kubectl logs --tail=300 "$POSTGRES_POD" > "$RESULT_DIR/postgres-tail.log" 2>&1 || true
kubectl logs --tail=200 "$REDIS_POD" > "$RESULT_DIR/redis-tail.log" 2>&1 || true

# Analysis
echo -e "\n${YELLOW}[5/7] Executing results analysis ...${NC}"
TOTAL_USERS="$TOTAL_USERS" TOTAL_QUOTA="$TOTAL_QUOTA" node load-test/analyze_results.js "$RESULT_DIR"

# Cleanup
echo -e "\n${YELLOW}[6/7] Cleaning up test data ...${NC}"

if [[ "$KEEP_DATA" == "1" ]]; then
  echo -e "${YELLOW}KEEP_DATA=1，已保留測試資料${NC}"
else
  echo -e "${GREEN}正在清除本次測試資料（依外鍵順序刪除）...${NC}"
  
  cat > /tmp/cleanup.sql <<SQL
DELETE FROM checkins 
WHERE ticket_id IN (SELECT id FROM tickets WHERE event_id = '${EVENT_ID}');

DELETE FROM tickets 
WHERE event_id = '${EVENT_ID}';

DELETE FROM applications 
WHERE event_id = '${EVENT_ID}';

DELETE FROM ticket_types 
WHERE event_id = '${EVENT_ID}';

DELETE FROM events 
WHERE id = '${EVENT_ID}';

DELETE FROM users 
WHERE email LIKE 'stress_${RUN_ID}_%' 
   OR email = 'stress_mgr_${RUN_ID}@company.com';

-- 清理 Redis 相關資料
SQL

  cat /tmp/cleanup.sql | kubectl exec -i $POSTGRES_POD -- psql -q -U ts_user -d ticketing_system | tee "$RESULT_DIR/db-cleanup.log"

  kubectl exec -i $REDIS_POD -- redis-cli DEL \
    "inventory:${TICKET_TYPE_ID}" \
    "inventory_loaded:${TICKET_TYPE_ID}" \
    "lock:init_lock:${TICKET_TYPE_ID}" >/dev/null 2>&1 || true

  echo -e "${GREEN}✅ 本次測試資料清理完成${NC}"
fi

# Archive
echo -e "\n${YELLOW}[7/7] Creating archive ...${NC}"
rm -rf "$LATEST_DIR"
cp -R "$RESULT_DIR" "$LATEST_DIR"

ARCHIVE_PATH="$ZIP_PATH"
if command -v zip >/dev/null 2>&1; then
  (cd "$(dirname "$RESULT_DIR")" && zip -qr "$(basename "$ZIP_PATH")" "$(basename "$RESULT_DIR")")
elif command -v tar >/dev/null 2>&1; then
  ARCHIVE_PATH="${RESULT_DIR}.tar.gz"
  (cd "$(dirname "$RESULT_DIR")" && tar -czf "$(basename "$ARCHIVE_PATH")" "$(basename "$RESULT_DIR")")
fi

echo -e "\n${GREEN}✅ Pipeline complete.${NC}"
echo "Metrics folder: ${RESULT_DIR}"
echo "Symlink: ${LATEST_DIR}"
if [[ -n "$ARCHIVE_PATH" ]]; then
  echo "Archive: ${ARCHIVE_PATH}"
fi

# Final exit code
K6_EXIT=$(awk -F= '/shard/ {if($2!=0) exitcode=1} END{print exitcode+0}' "$RESULT_DIR/k6-exit-code.txt" 2>/dev/null || echo 0)
if (( K6_EXIT != 0 && FAIL_ON_THRESHOLD == 1 )); then
  exit "$K6_EXIT"
fi