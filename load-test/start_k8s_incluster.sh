#!/usr/bin/env bash
set -uo pipefail

cd "$(dirname "$0")/.." || exit 1
REPO_ROOT="$(pwd -P)"

GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

NAMESPACE=${NAMESPACE:-default}
TARGET=${TARGET:-backend}
BASE_URL=${BASE_URL:-}
if [ -z "$BASE_URL" ]; then
  case "$TARGET" in
    backend)
      BASE_URL="http://backend:8001/v1"
      ;;
    frontend)
      BASE_URL="http://frontend-service:8080/v1"
      ;;
    *)
      echo -e "${RED}Unknown TARGET=${TARGET}. Use TARGET=backend, TARGET=frontend, or set BASE_URL explicitly.${NC}" >&2
      exit 2
      ;;
  esac
fi

TOTAL_USERS=${TOTAL_USERS:-6000}
TOTAL_QUOTA=${TOTAL_QUOTA:-$TOTAL_USERS}
MAX_TICKETS_PER_PERSON=${MAX_TICKETS_PER_PERSON:-1}
NUM_SHARDS=${NUM_SHARDS:-6}
ITERATIONS=${ITERATIONS:-1}
MAX_DURATION=${MAX_DURATION:-5m}
HTTP_TIMEOUT=${HTTP_TIMEOUT:-30s}
KEEP_DATA=${KEEP_DATA:-1}
RESET_QUEUE=${RESET_QUEUE:-1}
STRICT_THRESHOLDS=${STRICT_THRESHOLDS:-0}
FAIL_ON_THRESHOLD=${FAIL_ON_THRESHOLD:-0}
ERROR_SAMPLE_RATE=${ERROR_SAMPLE_RATE:-0.001}
DISCARD_RESPONSE_BODIES=${DISCARD_RESPONSE_BODIES:-1}
K6_SCRIPT=${K6_SCRIPT:-book-ticket.js}
K6_IMAGE=${K6_IMAGE:-grafana/k6:latest}
K6_POLL_SECONDS=${K6_POLL_SECONDS:-2}
K6_JOB_TIMEOUT_SECONDS=${K6_JOB_TIMEOUT_SECONDS:-600}
K6_CPU_REQUEST=${K6_CPU_REQUEST:-250m}
K6_CPU_LIMIT=${K6_CPU_LIMIT:-1}
K6_MEMORY_REQUEST=${K6_MEMORY_REQUEST:-256Mi}
K6_MEMORY_LIMIT=${K6_MEMORY_LIMIT:-1Gi}
TICKET_QUEUE_STREAM=${TICKET_QUEUE_STREAM:-ticket:applications}
TICKET_QUEUE_GROUP=${TICKET_QUEUE_GROUP:-ticket-workers}
POSTGRES_USER=${POSTGRES_USER:-ts_user}
POSTGRES_DB=${POSTGRES_DB:-ticketing_system}
DRAIN_TIMEOUT_SECONDS=${DRAIN_TIMEOUT_SECONDS:-900}
DRAIN_POLL_SECONDS=${DRAIN_POLL_SECONDS:-2}
CLEANUP_ON_DRAIN_TIMEOUT=${CLEANUP_ON_DRAIN_TIMEOUT:-0}
CLEANUP_K6_JOBS=${CLEANUP_K6_JOBS:-1}
RUN_LABEL=${RUN_LABEL:-booking}

RUN_ID=${RUN_ID:-$(date +%Y%m%d-%H%M%S)-${RUN_LABEL}-u${TOTAL_USERS}-${TARGET}-s${NUM_SHARDS}}
RUN_SLUG=$(printf '%s' "$RUN_ID" | tr '[:upper:]_' '[:lower:]-' | sed 's/[^a-z0-9-]/-/g; s/^-*//; s/-*$//' | cut -c1-36)
if [ -z "$RUN_SLUG" ]; then
  RUN_SLUG="run-$(date +%s)"
fi

RESULT_DIR="${REPO_ROOT}/load-test/results/${RUN_ID}"
LATEST_DIR="${REPO_ROOT}/load-test/results/latest"
ZIP_PATH="${RESULT_DIR}.zip"
SHARD_ROOT="${RESULT_DIR}/shards"

mkdir -p "$SHARD_ROOT"

POSTGRES_POD=$(kubectl -n "$NAMESPACE" get pods -l app=postgres -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")
REDIS_POD=$(kubectl -n "$NAMESPACE" get pods -l app=redis -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")
if [ -z "$POSTGRES_POD" ] || [ -z "$REDIS_POD" ]; then
  echo -e "${RED}Error: Postgres or Redis pod not found in namespace ${NAMESPACE}.${NC}" >&2
  exit 1
fi

redis_value() {
  kubectl -n "$NAMESPACE" exec "$REDIS_POD" -- redis-cli "$@" 2>/dev/null | tr -d '\r' || true
}

redis_pending_summary() {
  kubectl -n "$NAMESPACE" exec "$REDIS_POD" -- redis-cli XPENDING "$TICKET_QUEUE_STREAM" "$TICKET_QUEUE_GROUP" 2>/dev/null | tr -d '\r' | tr '\n' ' ' || true
}

redis_pending_count() {
  local raw first
  raw=$(kubectl -n "$NAMESPACE" exec "$REDIS_POD" -- redis-cli XPENDING "$TICKET_QUEUE_STREAM" "$TICKET_QUEUE_GROUP" 2>/dev/null | tr -d '\r' || true)
  first=$(printf '%s\n' "$raw" | head -n 1)
  if [[ "$first" =~ ^[0-9]+$ ]]; then
    echo "$first"
  else
    echo "0"
  fi
}

db_scalar() {
  kubectl -n "$NAMESPACE" exec "$POSTGRES_POD" -- psql -q -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -A -c "$1" 2>/dev/null | tr -d '[:space:]' || true
}

wait_for_job() {
  local job_name="$1"
  local deadline=$((SECONDS + K6_JOB_TIMEOUT_SECONDS))
  local succeeded failed
  while [ "$SECONDS" -lt "$deadline" ]; do
    succeeded=$(kubectl -n "$NAMESPACE" get job "$job_name" -o jsonpath='{.status.succeeded}' 2>/dev/null || echo "")
    failed=$(kubectl -n "$NAMESPACE" get job "$job_name" -o jsonpath='{.status.failed}' 2>/dev/null || echo "")
    succeeded=${succeeded:-0}
    failed=${failed:-0}
    if [ "$succeeded" != "0" ]; then
      return 0
    fi
    if [ "$failed" != "0" ]; then
      return 1
    fi
    sleep "$K6_POLL_SECONDS"
  done
  return 124
}

echo -e "${BLUE}=====================================================${NC}"
echo -e "${BLUE}   K8s In-Cluster k6 Queue Load Test                 ${NC}"
echo -e "${BLUE}=====================================================${NC}"
echo "RUN_ID=${RUN_ID}"
echo "NAMESPACE=${NAMESPACE}"
echo "TARGET=${TARGET}"
echo "BASE_URL=${BASE_URL}"
echo "TOTAL_USERS=${TOTAL_USERS}"
echo "NUM_SHARDS=${NUM_SHARDS}"
echo "RESULT_DIR=${RESULT_DIR}"

cat > "$RESULT_DIR/parameters.env" <<PARAMS
RUN_ID=${RUN_ID}
NAMESPACE=${NAMESPACE}
TARGET=${TARGET}
BASE_URL=${BASE_URL}
TOTAL_USERS=${TOTAL_USERS}
TOTAL_QUOTA=${TOTAL_QUOTA}
MAX_TICKETS_PER_PERSON=${MAX_TICKETS_PER_PERSON}
NUM_SHARDS=${NUM_SHARDS}
ITERATIONS=${ITERATIONS}
MAX_DURATION=${MAX_DURATION}
HTTP_TIMEOUT=${HTTP_TIMEOUT}
KEEP_DATA=${KEEP_DATA}
RESET_QUEUE=${RESET_QUEUE}
STRICT_THRESHOLDS=${STRICT_THRESHOLDS}
FAIL_ON_THRESHOLD=${FAIL_ON_THRESHOLD}
ERROR_SAMPLE_RATE=${ERROR_SAMPLE_RATE}
DISCARD_RESPONSE_BODIES=${DISCARD_RESPONSE_BODIES}
K6_SCRIPT=${K6_SCRIPT}
K6_IMAGE=${K6_IMAGE}
K6_JOB_TIMEOUT_SECONDS=${K6_JOB_TIMEOUT_SECONDS}
TICKET_QUEUE_STREAM=${TICKET_QUEUE_STREAM}
TICKET_QUEUE_GROUP=${TICKET_QUEUE_GROUP}
DRAIN_TIMEOUT_SECONDS=${DRAIN_TIMEOUT_SECONDS}
DRAIN_POLL_SECONDS=${DRAIN_POLL_SECONDS}
PARAMS

echo -e "\n${YELLOW}[1/9] Generating test data ...${NC}"
RUN_ID="$RUN_ID" TOTAL_USERS="$TOTAL_USERS" TOTAL_QUOTA="$TOTAL_QUOTA" MAX_TICKETS_PER_PERSON="$MAX_TICKETS_PER_PERSON" \
  node load-test/generate_data.js | tee "$RESULT_DIR/generate-data.log"
cp load-test/setup_db.sql "$RESULT_DIR/setup_db.sql"
cp load-test/stress_env.json "$RESULT_DIR/stress_env.json"

EVENT_ID=$(node -e "console.log(require('./load-test/stress_env.json').event_id)")
TICKET_TYPE_ID=$(node -e "console.log(require('./load-test/stress_env.json').ticket_type_id)")

echo -e "\n${YELLOW}[2/9] Inserting data into K8s PostgreSQL ...${NC}"
kubectl -n "$NAMESPACE" exec -i "$POSTGRES_POD" -- psql -q -U "$POSTGRES_USER" -d "$POSTGRES_DB" < load-test/setup_db.sql | tee "$RESULT_DIR/db-insert.log"

PRE_RESET_STREAM_LEN=$(redis_value XLEN "$TICKET_QUEUE_STREAM")
PRE_RESET_PENDING=$(redis_pending_summary)
{
  echo "pre_reset_stream_length=${PRE_RESET_STREAM_LEN:-NULL}"
  echo "pre_reset_pending=${PRE_RESET_PENDING:-NULL}"
} | tee "$RESULT_DIR/pre-reset-queue.txt"

if [ "$RESET_QUEUE" = "1" ]; then
  echo -e "\n${YELLOW}[3/9] Resetting Redis stream before this run ...${NC}"
  redis_value DEL "$TICKET_QUEUE_STREAM" >/dev/null
  sleep 1
else
  echo -e "\n${YELLOW}[3/9] RESET_QUEUE=0, leaving existing Redis stream intact ...${NC}"
fi
STREAM_LEN_BEFORE=$(redis_value XLEN "$TICKET_QUEUE_STREAM")
STREAM_LEN_BEFORE=${STREAM_LEN_BEFORE:-0}

echo -e "\n${YELLOW}[4/9] Creating k6 ConfigMaps and Jobs inside the cluster ...${NC}"
pids=()
JOB_NAMES=()
CONFIGMAP_NAMES=()
OFFSET=0

BASE_COUNT=$((TOTAL_USERS / NUM_SHARDS))
REMAINDER=$((TOTAL_USERS % NUM_SHARDS))

for ((i = 1; i <= NUM_SHARDS; i++)); do
  COUNT=$BASE_COUNT
  if [ "$i" -le "$REMAINDER" ]; then
    COUNT=$((COUNT + 1))
  fi

  SHARD_DIR="${SHARD_ROOT}/shard${i}"
  mkdir -p "$SHARD_DIR"
  cp "load-test/${K6_SCRIPT}" "$SHARD_DIR/book-ticket.js"
  echo "$BASE_URL" > "$SHARD_DIR/endpoint.txt"

  node - "$RESULT_DIR/stress_env.json" "$SHARD_DIR/stress_env.json" "$RUN_ID-shard$i" "$OFFSET" "$COUNT" <<'NODE'
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

  CM_NAME="k6-${RUN_SLUG}-s${i}"
  JOB_NAME="k6-${RUN_SLUG}-s${i}"
  CONFIGMAP_NAMES+=("$CM_NAME")
  JOB_NAMES+=("$JOB_NAME")

  kubectl -n "$NAMESPACE" delete job "$JOB_NAME" --ignore-not-found=true >/dev/null 2>&1 || true
  kubectl -n "$NAMESPACE" delete configmap "$CM_NAME" --ignore-not-found=true >/dev/null 2>&1 || true

  kubectl -n "$NAMESPACE" create configmap "$CM_NAME" \
    --from-file=book-ticket.js="$SHARD_DIR/book-ticket.js" \
    --from-file=stress_env.json="$SHARD_DIR/stress_env.json" \
    --dry-run=client -o yaml | kubectl -n "$NAMESPACE" apply -f - >/dev/null

  cat <<YAML | kubectl -n "$NAMESPACE" apply -f - >/dev/null
apiVersion: batch/v1
kind: Job
metadata:
  name: ${JOB_NAME}
  labels:
    app: k6-ticket-load-test
    run-id: ${RUN_SLUG}
    shard: "${i}"
spec:
  backoffLimit: 0
  activeDeadlineSeconds: ${K6_JOB_TIMEOUT_SECONDS}
  ttlSecondsAfterFinished: 600
  template:
    metadata:
      labels:
        app: k6-ticket-load-test
        run-id: ${RUN_SLUG}
        shard: "${i}"
    spec:
      restartPolicy: Never
      containers:
      - name: k6
        image: ${K6_IMAGE}
        imagePullPolicy: IfNotPresent
        env:
        - name: BASE_URL
          value: "${BASE_URL}"
        - name: VUS
          value: "${COUNT}"
        - name: ITERATIONS
          value: "${ITERATIONS}"
        - name: MAX_DURATION
          value: "${MAX_DURATION}"
        - name: STRICT_THRESHOLDS
          value: "${STRICT_THRESHOLDS}"
        - name: ERROR_SAMPLE_RATE
          value: "${ERROR_SAMPLE_RATE}"
        - name: HTTP_TIMEOUT
          value: "${HTTP_TIMEOUT}"
        - name: DISCARD_RESPONSE_BODIES
          value: "${DISCARD_RESPONSE_BODIES}"
        command: ["sh", "-c"]
        args:
        - |
          set +e
          cd /scripts
          k6 run --compatibility-mode=base --summary-export /tmp/book-summary.json book-ticket.js
          code=\$?
          echo "__K6_SUMMARY_BEGIN__"
          cat /tmp/book-summary.json 2>/dev/null || true
          echo "__K6_SUMMARY_END__"
          exit "\$code"
        resources:
          requests:
            cpu: "${K6_CPU_REQUEST}"
            memory: "${K6_MEMORY_REQUEST}"
          limits:
            cpu: "${K6_CPU_LIMIT}"
            memory: "${K6_MEMORY_LIMIT}"
        volumeMounts:
        - name: scripts
          mountPath: /scripts
          readOnly: true
      volumes:
      - name: scripts
        configMap:
          name: ${CM_NAME}
YAML

  OFFSET=$((OFFSET + COUNT))
done

echo -e "\n${YELLOW}[5/9] Waiting for k6 Jobs and collecting logs ...${NC}"
: > "$RESULT_DIR/k6-exit-code.txt"
for i in "${!JOB_NAMES[@]}"; do
  shard=$((i + 1))
  job="${JOB_NAMES[$i]}"
  shard_dir="${SHARD_ROOT}/shard${shard}"

  if wait_for_job "$job"; then
    code=0
  else
    code=$?
  fi

  pod=$(kubectl -n "$NAMESPACE" get pods -l job-name="$job" -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")
  if [ -n "$pod" ]; then
    kubectl -n "$NAMESPACE" logs "$pod" > "$shard_dir/k6.log" 2>&1 || true
    awk '/__K6_SUMMARY_BEGIN__/{flag=1;next}/__K6_SUMMARY_END__/{flag=0}flag' "$shard_dir/k6.log" > "$shard_dir/book-summary.json"
  else
    echo "No pod found for job ${job}" > "$shard_dir/k6.log"
  fi

  if [ ! -s "$shard_dir/book-summary.json" ]; then
    code=1
  fi
  echo "shard${shard}=${code}" | tee -a "$RESULT_DIR/k6-exit-code.txt"
done

node load-test/aggregate_k6_shards.js "$RESULT_DIR" "$NUM_SHARDS" > "$RESULT_DIR/k6-aggregate.txt"
EXPECTED_SUCCESS=$(awk -F= '/^total_success=/{print $2}' "$RESULT_DIR/k6-aggregate.txt" | tail -n 1)
EXPECTED_SUCCESS=${EXPECTED_SUCCESS:-0}

echo -e "\n${YELLOW}[6/9] Waiting for queue workers to drain ...${NC}"
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

echo -e "\n${YELLOW}[7/9] Querying final DB and Redis metrics ...${NC}"
{
  echo "run_id=${RUN_ID}"
  echo "topology=k8s-incluster-${TARGET}-${NUM_SHARDS}shards"
  echo "target=${TARGET}"
  echo "base_url=${BASE_URL}"
  echo "event_id=${EVENT_ID}"
  echo "ticket_type_id=${TICKET_TYPE_ID}"
  echo "drain_ok=${DRAIN_OK}"
  echo "drain_elapsed_seconds=${DRAIN_ELAPSED}"
  echo "drain_expected_success=${EXPECTED_SUCCESS}"
  cat "$RESULT_DIR/k6-aggregate.txt"

  kubectl -n "$NAMESPACE" exec "$POSTGRES_POD" -- psql -q -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -A -F '=' -c "
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

echo -e "\n${YELLOW}[7.5/9] Archiving pod logs ...${NC}"
BACKEND_PODS=($(kubectl -n "$NAMESPACE" get pods -l app=backend -o jsonpath='{.items[*].metadata.name}' 2>/dev/null || echo ""))
for pod in "${BACKEND_PODS[@]}"; do
  kubectl -n "$NAMESPACE" logs --tail=500 "$pod" > "$RESULT_DIR/backend-${pod}.log" 2>&1 || true
done
FRONTEND_PODS=($(kubectl -n "$NAMESPACE" get pods -l app=frontend -o jsonpath='{.items[*].metadata.name}' 2>/dev/null || echo ""))
for pod in "${FRONTEND_PODS[@]}"; do
  kubectl -n "$NAMESPACE" logs --tail=500 "$pod" > "$RESULT_DIR/frontend-${pod}.log" 2>&1 || true
done
kubectl -n "$NAMESPACE" logs --tail=300 "$POSTGRES_POD" > "$RESULT_DIR/postgres-tail.log" 2>&1 || true
kubectl -n "$NAMESPACE" logs --tail=200 "$REDIS_POD" > "$RESULT_DIR/redis-tail.log" 2>&1 || true

echo -e "\n${YELLOW}[8/9] Generating analysis report ...${NC}"
TOTAL_USERS="$TOTAL_USERS" TOTAL_QUOTA="$TOTAL_QUOTA" MAX_TICKETS_PER_PERSON="$MAX_TICKETS_PER_PERSON" \
  BASE_URL="$BASE_URL" node load-test/analyze_results.js "$RESULT_DIR"

echo -e "\n${YELLOW}[8.5/9] Generating clean English summary report ...${NC}"

TOTAL_REQS=$(awk -F= '/total_reqs=/{print $2}' "$RESULT_DIR/k6-aggregate.txt" 2>/dev/null | tail -n1 || echo "N/A")
TOTAL_SUCCESS=$(awk -F= '/total_success=/{print $2}' "$RESULT_DIR/k6-aggregate.txt" 2>/dev/null | tail -n1 || echo "N/A")
SUCCESS_RATE=$(awk -F= '/success_rate_percent=/{print $2}' "$RESULT_DIR/k6-aggregate.txt" 2>/dev/null | tail -n1 || echo "N/A")
MAX_P95=$(awk -F= '/max_shard_p95_ms=/{print $2}' "$RESULT_DIR/k6-aggregate.txt" 2>/dev/null | tail -n1 || echo "N/A")

cat > "$RESULT_DIR/run-summary.md" <<EOF
# Load Test Report

**Run ID**: ${RUN_ID}  
**Date**: $(date '+%Y-%m-%d %H:%M:%S')  
**Target**: ${TARGET} (${BASE_URL})  
**Total Users**: ${TOTAL_USERS}  
**Shards**: ${NUM_SHARDS}

## Summary

| Item                        | Value                  |
|-----------------------------|------------------------|
| Total Requests              | ${TOTAL_REQS}          |
| Successful Tickets          | ${applications_count:-N/A} |
| Success Rate                | ${SUCCESS_RATE}%       |
| Max p95 Latency             | ${MAX_P95} ms          |
| Tickets Issued              | ${tickets_count:-N/A}  |
| Remaining Tickets           | ${ticket_type_remaining:-N/A} |
| Redis Inventory             | ${redis_inventory:-N/A} |
| Drain Status                | $([ "${DRAIN_OK:-0}" = "1" ] && echo "✅ Completed (${DRAIN_ELAPSED}s)" || echo "⚠️ Not Fully Drained") |

## Conclusion
- **${applications_count:-0}** out of **${TOTAL_USERS}** tickets were successfully processed.
- Data consistency: $([ "${DRAIN_OK:-0}" = "1" ] && echo "Good" || echo "Needs attention")

---
Generated by K8s In-Cluster Load Test Framework
EOF

echo -e "${GREEN}✅ English summary report generated: ${RESULT_DIR}/run-summary.md${NC}"