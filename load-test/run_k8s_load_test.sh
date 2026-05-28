#!/usr/bin/env bash
set -uo pipefail

cd "$(dirname "$0")/.." || exit 1

GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

# =====================================================================
# 參數初始化
# =====================================================================
TOTAL_USERS=${TOTAL_USERS:-2000}
TOTAL_QUOTA=${TOTAL_QUOTA:-50000}
VUS=${VUS:-1000}
ITERATIONS=${ITERATIONS:-1}
PARALLELISM=${PARALLELISM:-4}
K8S_NAMESPACE=${K8S_NAMESPACE:-default}
BASE_URL=${BASE_URL:-http://backend:8001/v1}

# [1/7] 產生測試資料
echo -e "\n${YELLOW}[1/7] Generating testing data...${NC}"
export TOTAL_USERS TOTAL_QUOTA
node load-test/generate_data.js || { echo -e "${RED}[ERROR] Failed to generate data. Exiting...${NC}"; exit 1; }

# 獲取 RUN_ID 並確保其為當前唯一
RUN_ID=$(node -e "console.log(require('./load-test/stress_env.json').run_id)")
RESULT_DIR="load-test/results/${RUN_ID}"
mkdir -p "$RESULT_DIR"
cp load-test/stress_env.json "$RESULT_DIR/stress_env.json"
CONFIGMAP_NAME="k6-spec-${RUN_ID}"

echo -e "${BLUE}>>> Running Test: ${RUN_ID} (Parallelism: ${PARALLELISM})${NC}"

# [2/7] 注入與預熱
POSTGRES_POD=$(kubectl get pods -n "$K8S_NAMESPACE" -l app=postgres -o name | head -n1)
REDIS_POD=$(kubectl get pods -n "$K8S_NAMESPACE" -l app=redis -o name | head -n1)

if [ -n "$POSTGRES_POD" ]; then
    kubectl exec -i $POSTGRES_POD -- psql -q -U postgres -d ticket_db -c "TRUNCATE TABLE users, tickets, applications, ticket_types, events CASCADE;" >/dev/null
    kubectl exec -i $POSTGRES_POD -- psql -q -U postgres -d ticket_db < load-test/setup_db.sql >/dev/null
fi

if [ -n "$REDIS_POD" ]; then
    # ==== 新增下面這行，每次壓測前徹底清空 Redis 的殘留 Queue ====
    kubectl exec $REDIS_POD -- redis-cli FLUSHALL >/dev/null
    
    # 重新注入這次的庫存
    kubectl exec $REDIS_POD -- redis-cli SET "inventory:$(node -e "console.log(require('./load-test/stress_env.json').ticket_type_id)")" "${TOTAL_QUOTA}" >/dev/null
fi

# [3/7] 壓測執行
kubectl create configmap "$CONFIGMAP_NAME" -n "$K8S_NAMESPACE" --from-file=book-ticket.js=load-test/book-ticket.js --from-file=stress_env.json=load-test/stress_env.json

cat <<EOF > "$RESULT_DIR/k6-k8s-execution.yaml"
apiVersion: k6.io/v1alpha1
kind: TestRun
metadata:
  name: k6-dist-booking
  namespace: ${K8S_NAMESPACE}
spec:
  parallelism: ${PARALLELISM}
  paused: "false"
  script:
    configMap:
      name: ${CONFIGMAP_NAME}
      file: book-ticket.js
  runner:
    env:
      - name: BASE_URL
        value: "${BASE_URL}"
      - name: VUS
        value: "${VUS}"
      - name: ITERATIONS
        value: "${ITERATIONS}"
EOF

kubectl apply -f "$RESULT_DIR/k6-k8s-execution.yaml"

echo "[INFO] Waiting for TestRun to complete (checking status.stage)..."
for i in {1..60}; do
    STAGE=$(kubectl get testrun k6-dist-booking -n "$K8S_NAMESPACE" -o jsonpath='{.status.stage}' 2>/dev/null)
    if [ "$STAGE" == "finished" ] || [ "$STAGE" == "error" ]; then
        echo "[INFO] TestRun reached stage: $STAGE"
        break
    fi
    sleep 5
done
# [4/7] 抓取日誌 (優化：精準只抓 Runner Pods，排除 Starter 報錯)
echo "[INFO] Retrieving logs from k6 runners..."
> "$RESULT_DIR/k6-console.log" # 確保日誌檔是乾淨的

# 找出所有的 Runner Pods (名稱格式如 k6-dist-booking-1-xxxxx)，強制過濾掉 starter Pod
RUNNER_PODS=$(kubectl get pods -n "$K8S_NAMESPACE" -l k6_cr=k6-dist-booking --no-headers | grep -E "k6-dist-booking-[0-9]+" | awk '{print $1}')

if [ -z "$RUNNER_PODS" ]; then
    echo -e "${RED}[WARNING] No runner pods found. The test might have failed to start.${NC}"
else
    for pod in $RUNNER_PODS; do
        echo "Fetching logs from $pod..."
        # 依序抓取每個 Runner 的 log 並附加進檔案
        kubectl logs "$pod" -n "$K8S_NAMESPACE" >> "$RESULT_DIR/k6-console.log" 2>&1 || true
    done
fi

# [5/7] 產出報告與強制更名
echo -e "\n${YELLOW}[5/7] Writing and renaming report...${NC}"
node load-test/analyze_results.js "$RESULT_DIR"

# 確保檔名符合要求
NEW_FILENAME="${RUN_ID}-booking-u${TOTAL_USERS}-q${TOTAL_QUOTA}-v${VUS}-i${ITERATIONS}.md"
find "$RESULT_DIR" -maxdepth 1 -name "*.md" ! -name "$NEW_FILENAME" -exec mv {} "$RESULT_DIR/$NEW_FILENAME" \;

# [6/7] 清理 (執行完此步後 Pod 就會消失，這是正常的)
echo "[INFO] Cleaning up test resources..."
kubectl delete -f "$RESULT_DIR/k6-k8s-execution.yaml" --ignore-not-found=true
kubectl delete configmap "$CONFIGMAP_NAME" -n "$K8S_NAMESPACE" --ignore-not-found=true

echo -e "\n${GREEN}[SUCCESS] Test finished. Final report: ${NEW_FILENAME}${NC}"