#!/usr/bin/env bash
set -uo pipefail

cd "$(dirname "$0")/.." || exit 1

# Color definitions
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

# Default environment variables
HTTP_TIMEOUT=${HTTP_TIMEOUT:-30s}
SETTLE_SECONDS=${SETTLE_SECONDS:-0}
TOTAL_USERS=${TOTAL_USERS:-2000}
TOTAL_QUOTA=${TOTAL_QUOTA:-50000}
MAX_TICKETS_PER_PERSON=${MAX_TICKETS_PER_PERSON:-1}
VUS=${VUS:-1000}
ITERATIONS=${ITERATIONS:-1}
MAX_DURATION=${MAX_DURATION:-3m}
BASE_URL=${BASE_URL:-http://localhost:8001/v1}
KEEP_DATA=${KEEP_DATA:-1}
STRICT_THRESHOLDS=${STRICT_THRESHOLDS:-0}
FAIL_ON_THRESHOLD=${FAIL_ON_THRESHOLD:-0}
ERROR_SAMPLE_RATE=${ERROR_SAMPLE_RATE:-0.02}
DISCARD_RESPONSE_BODIES=${DISCARD_RESPONSE_BODIES:-0}
K6_SCRIPT=${K6_SCRIPT:-book-ticket.js}
RATE=${RATE:-100}
TIME_UNIT=${TIME_UNIT:-1s}
DURATION=${DURATION:-1m}
PRE_ALLOCATED_VUS=${PRE_ALLOCATED_VUS:-100}
MAX_VUS=${MAX_VUS:-200}
READ_DETAIL_RATIO=${READ_DETAIL_RATIO:-0.3}
MIXED_BOOKING_RATIO=${MIXED_BOOKING_RATIO:-0.1}
TICKET_QUEUE_STREAM=${TICKET_QUEUE_STREAM:-ticket:applications}
TICKET_QUEUE_GROUP=${TICKET_QUEUE_GROUP:-ticket-workers}
RUN_LABEL=${RUN_LABEL:-booking}

# RUN_ID generation isolates concurrent test runs
RUN_ID=${RUN_ID:-$(date +%Y%m%d-%H%M%S)-${RUN_LABEL}-u${TOTAL_USERS}-q${TOTAL_QUOTA}-v${VUS}-i${ITERATIONS}}
RESULT_DIR="load-test/results/${RUN_ID}"
LATEST_DIR="load-test/results/latest"
ZIP_PATH="${RESULT_DIR}.zip"

# Locate target K8s Postgres and Redis pods dynamically
POSTGRES_POD=$(kubectl get pods -l app=postgres -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")
REDIS_POD=$(kubectl get pods -l app=redis -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")

if [ -z "$POSTGRES_POD" ] || [ -z "$REDIS_POD" ]; then
  echo -e "${RED}Error: Postgres or Redis pod not found in cluster.${NC}"
  exit 1
fi

mkdir -p "$RESULT_DIR"

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
VUS=${VUS}
ITERATIONS=${ITERATIONS}
MAX_DURATION=${MAX_DURATION}
BASE_URL=${BASE_URL}
KEEP_DATA=${KEEP_DATA}
STRICT_THRESHOLDS=${STRICT_THRESHOLDS}
FAIL_ON_THRESHOLD=${FAIL_ON_THRESHOLD}
ERROR_SAMPLE_RATE=${ERROR_SAMPLE_RATE}
DISCARD_RESPONSE_BODIES=${DISCARD_RESPONSE_BODIES}
K6_SCRIPT=${K6_SCRIPT}
RATE=${RATE}
TIME_UNIT=${TIME_UNIT}
DURATION=${DURATION}
PRE_ALLOCATED_VUS=${PRE_ALLOCATED_VUS}
MAX_VUS=${MAX_VUS}
READ_DETAIL_RATIO=${READ_DETAIL_RATIO}
MIXED_BOOKING_RATIO=${MIXED_BOOKING_RATIO}
TICKET_QUEUE_STREAM=${TICKET_QUEUE_STREAM}
TICKET_QUEUE_GROUP=${TICKET_QUEUE_GROUP}
HTTP_TIMEOUT=${HTTP_TIMEOUT}
SETTLE_SECONDS=${SETTLE_SECONDS}
PARAMS

echo -e "${BLUE}=====================================================${NC}"
echo -e "${BLUE}   Corporate Event Ticketing System Load Test (K8s)  ${NC}"
echo -e "${BLUE}=====================================================${NC}"
echo "RUN_ID=${RUN_ID}"
echo "RESULT_DIR=${RESULT_DIR}"

# Generate dynamic datasets scoped to this specific RUN_ID
echo -e "\n${YELLOW}[1/7] Generating testing data ...${NC}"
RUN_ID="$RUN_ID" TOTAL_USERS="$TOTAL_USERS" TOTAL_QUOTA="$TOTAL_QUOTA" MAX_TICKETS_PER_PERSON="$MAX_TICKETS_PER_PERSON" node load-test/generate_data.js | tee "$RESULT_DIR/generate-data.log"
cp load-test/setup_db.sql "$RESULT_DIR/setup_db.sql"
cp load-test/stress_env.json "$RESULT_DIR/stress_env.json"
EVENT_ID=$(node -e "console.log(require('./load-test/stress_env.json').event_id)")
TICKET_TYPE_ID=$(node -e "console.log(require('./load-test/stress_env.json').ticket_type_id)")

# Stream local setup SQL directly into the K8s PostgreSQL Pod via stdin
echo -e "\n${YELLOW}[2/7] Inserting data into K8s PostgreSQL ...${NC}"
cat load-test/setup_db.sql | kubectl exec -i $POSTGRES_POD -- psql -q -U ts_user -d ticketing_system | tee "$RESULT_DIR/db-insert.log"

sleep 2

STREAM_LEN_BEFORE=$(kubectl exec -i $REDIS_POD -- redis-cli XLEN "${TICKET_QUEUE_STREAM}" 2>/dev/null | tr -d '\r' || true)
STREAM_LEN_BEFORE=${STREAM_LEN_BEFORE:-0}

# Run k6 scenario against K8s base URL
echo -e "\n${YELLOW}[3/7] Running k6 script ${K6_SCRIPT} ...${NC}"
pushd load-test >/dev/null || exit 1
set +e
BASE_URL="$BASE_URL" \
VUS="$VUS" \
ITERATIONS="$ITERATIONS" \
MAX_DURATION="$MAX_DURATION" \
STRICT_THRESHOLDS="$STRICT_THRESHOLDS" \
ERROR_SAMPLE_RATE="$ERROR_SAMPLE_RATE" \
HTTP_TIMEOUT="$HTTP_TIMEOUT" \
DISCARD_RESPONSE_BODIES="$DISCARD_RESPONSE_BODIES" \
RATE="$RATE" \
TIME_UNIT="$TIME_UNIT" \
DURATION="$DURATION" \
PRE_ALLOCATED_VUS="$PRE_ALLOCATED_VUS" \
MAX_VUS="$MAX_VUS" \
READ_DETAIL_RATIO="$READ_DETAIL_RATIO" \
MIXED_BOOKING_RATIO="$MIXED_BOOKING_RATIO" \
  k6 run --summary-export="../${RESULT_DIR}/book-summary.json" "$K6_SCRIPT" 2>&1 | tee "../${RESULT_DIR}/k6-console.log"
K6_EXIT=${PIPESTATUS[0]}
set -e
popd >/dev/null || exit 1

echo "K6_EXIT=${K6_EXIT}" > "$RESULT_DIR/k6-exit-code.txt"
if (( K6_EXIT != 0 )); then
  echo -e "${YELLOW}k6 threshold failed, continuing metric extraction...${NC}"
fi

if (( SETTLE_SECONDS > 0 )); then
  echo -e "\n${YELLOW}[3.5/7] Waiting ${SETTLE_SECONDS}s for sync settle ...${NC}"
  sleep "$SETTLE_SECONDS"
fi

# Extract target verification metrics using precise scopes
echo -e "\n${YELLOW}[4/7] Querying K8s DB and Redis metrics ...${NC}"
{
  echo "event_id=${EVENT_ID}"
  echo "ticket_type_id=${TICKET_TYPE_ID}"
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
  if [[ "$STREAM_LEN_BEFORE" =~ ^[0-9]+$ && "$STREAM_LEN_AFTER" =~ ^[0-9]+$ ]]; then
    STREAM_LEN_DELTA=$((STREAM_LEN_AFTER - STREAM_LEN_BEFORE))
  else
    STREAM_LEN_DELTA="NULL"
  fi
  echo "redis_stream_length_before=${STREAM_LEN_BEFORE:-NULL}"
  echo "redis_stream_length_after=${STREAM_LEN_AFTER:-NULL}"
  echo "redis_stream_length_delta=${STREAM_LEN_DELTA}"
  PENDING_SUMMARY=$(kubectl exec -i $REDIS_POD -- redis-cli XPENDING "${TICKET_QUEUE_STREAM}" "${TICKET_QUEUE_GROUP}" 2>/dev/null | tr -d '\r' | tr '\n' ' ' || true)
  echo "redis_stream_pending=${PENDING_SUMMARY:-NULL}"
} | tee "$RESULT_DIR/db-summary.txt"

# Fetch logs from the first available backend replica instance
echo -e "\n${YELLOW}[4.5/7] Archiving pod log tails ...${NC}"
BACKEND_POD=$(kubectl get pods -l app=backend -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")
if [ -n "$BACKEND_POD" ]; then
  kubectl logs --tail=300 "$BACKEND_POD" > "$RESULT_DIR/backend-tail.log" 2>&1 || true
fi
kubectl logs --tail=300 "$POSTGRES_POD" > "$RESULT_DIR/postgres-tail.log" 2>&1 || true
kubectl logs --tail=200 "$REDIS_POD" > "$RESULT_DIR/redis-tail.log" 2>&1 || true

# Render metric breakdowns
echo -e "\n${YELLOW}[5/7] Executing results analysis module ...${NC}"
TOTAL_USERS="$TOTAL_USERS" TOTAL_QUOTA="$TOTAL_QUOTA" MAX_TICKETS_PER_PERSON="$MAX_TICKETS_PER_PERSON" VUS="$VUS" ITERATIONS="$ITERATIONS" MAX_DURATION="$MAX_DURATION" BASE_URL="$BASE_URL" node load-test/analyze_results.js "$RESULT_DIR"

# Safe target data teardown
if [[ "$KEEP_DATA" != "1" ]]; then
  echo -e "\n${YELLOW}[6/7] Scoping down cleanup to transient data generated by RUN_ID ...${NC}"
  echo "
DELETE FROM tickets WHERE event_id = '${EVENT_ID}';
DELETE FROM applications WHERE event_id = '${EVENT_ID}';
DELETE FROM ticket_types WHERE event_id = '${EVENT_ID}';
DELETE FROM events WHERE id = '${EVENT_ID}';
DELETE FROM users WHERE email LIKE 'stress_${RUN_ID}_%' OR email = 'stress_mgr_${RUN_ID}@company.com';
" | kubectl exec -i $POSTGRES_POD -- psql -q -U ts_user -d ticketing_system | tee "$RESULT_DIR/db-cleanup.log"
  
  kubectl exec -i $REDIS_POD -- redis-cli DEL "inventory:${TICKET_TYPE_ID}" "inventory_loaded:${TICKET_TYPE_ID}" "lock:init_lock:${TICKET_TYPE_ID}" >/dev/null 2>&1 || true
else
  echo -e "\n${YELLOW}[6/7] KEEP_DATA=1, sandbox variables preserved for tracking.${NC}"
fi

# Step 7: Compress and generate artifact reports
echo -e "\n${YELLOW}[7/7] Executing archive compression paths ...${NC}"
rm -rf "$LATEST_DIR"
mkdir -p "$(dirname "$LATEST_DIR")"
cp -R "$RESULT_DIR" "$LATEST_DIR"

ARCHIVE_PATH="$ZIP_PATH"
if command -v zip >/dev/null 2>&1; then
  (cd "$(dirname "$RESULT_DIR")" && zip -qr "$(basename "$ZIP_PATH")" "$(basename "$RESULT_DIR")")
elif command -v python3 >/dev/null 2>&1; then
  python3 - "$RESULT_DIR" "$ZIP_PATH" <<'PYZIP'
import sys, zipfile
from pathlib import Path
result_dir = Path(sys.argv[1])
zip_path = Path(sys.argv[2])
with zipfile.ZipFile(zip_path, 'w', zipfile.ZIP_DEFLATED) as zf:
    for item in result_dir.rglob('*'):
        if item.is_file():
            zf.write(item, item.relative_to(result_dir.parent))
PYZIP
elif command -v tar >/dev/null 2>&1; then
  ARCHIVE_PATH="${RESULT_DIR}.tar.gz"
  (cd "$(dirname "$RESULT_DIR")" && tar -czf "$(basename "$ARCHIVE_PATH")" "$(basename "$RESULT_DIR")")
else
  ARCHIVE_PATH=""
  echo -e "${YELLOW}Warning: Archiving libraries unavailable.${NC}"
fi

echo -e "\n${GREEN}Pipeline complete.${NC}"
echo "Metrics folder: ${RESULT_DIR}"
if [[ -n "$ARCHIVE_PATH" ]]; then
  echo "Compressed package: ${ARCHIVE_PATH}"
fi
echo "Symlink destination: ${LATEST_DIR}"

if (( K6_EXIT != 0 && FAIL_ON_THRESHOLD == 1 )); then
  exit "$K6_EXIT"
fi