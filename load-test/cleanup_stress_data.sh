#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.." || exit 1

DB_USER=${DB_USER:-ts_user}
DB_NAME=${DB_NAME:-ticketing_system}
TICKET_QUEUE_STREAM=${TICKET_QUEUE_STREAM:-ticket:applications}
TICKET_QUEUE_GROUP=${TICKET_QUEUE_GROUP:-ticket-workers}
DRY_RUN=${DRY_RUN:-1}
RESET_QUEUE=${RESET_QUEUE:-0}

if [[ "$DRY_RUN" != "0" ]]; then
  echo "DRY_RUN=1: showing what would be deleted. Re-run with DRY_RUN=0 to delete."
fi

echo "[1/4] Finding stress test events and ticket types..."
EVENT_IDS=$(docker compose exec -T postgres psql -q -U "$DB_USER" -d "$DB_NAME" -t -A -c "SELECT id FROM events WHERE title LIKE '壓測活動-%';" | tr -d '\r' || true)
TICKET_TYPE_IDS=$(docker compose exec -T postgres psql -q -U "$DB_USER" -d "$DB_NAME" -t -A -c "SELECT tt.id FROM ticket_types tt JOIN events e ON tt.event_id = e.id WHERE e.title LIKE '壓測活動-%';" | tr -d '\r' || true)
EVENT_COUNT=$(printf '%s\n' "$EVENT_IDS" | sed '/^$/d' | wc -l | tr -d ' ')
TYPE_COUNT=$(printf '%s\n' "$TICKET_TYPE_IDS" | sed '/^$/d' | wc -l | tr -d ' ')
echo "Stress events: ${EVENT_COUNT}"
echo "Stress ticket types: ${TYPE_COUNT}"

if [[ "$DRY_RUN" != "0" ]]; then
  echo "[DRY_RUN] Would delete DB rows for events with title LIKE '壓測活動-%' and users with stress emails / employee IDs."
  echo "[DRY_RUN] Would delete Redis inventory keys for the stress ticket_type IDs found above."
  if [[ "$RESET_QUEUE" == "1" ]]; then
    echo "[DRY_RUN] Would also delete Redis stream ${TICKET_QUEUE_STREAM} and queue status/reservation keys."
  fi
  exit 0
fi

echo "[2/4] Deleting Redis inventory keys for stress ticket types..."
while IFS= read -r id; do
  [[ -z "$id" ]] && continue
  docker compose exec -T redis redis-cli DEL "inventory:${id}" "inventory_loaded:${id}" "lock:init_lock:${id}" >/dev/null || true
done <<< "$TICKET_TYPE_IDS"

if [[ "$RESET_QUEUE" == "1" ]]; then
  echo "[3/4] Resetting Redis queue/stream keys..."
  docker compose exec -T redis redis-cli DEL "$TICKET_QUEUE_STREAM" >/dev/null || true
  # Best-effort cleanup of queue status / reservation hashes created by load tests.
  docker compose exec -T redis sh -lc "redis-cli --scan --pattern 'queue:status:*' | xargs -r redis-cli DEL >/dev/null" || true
  docker compose exec -T redis sh -lc "redis-cli --scan --pattern 'queue:reservation:*' | xargs -r redis-cli DEL >/dev/null" || true
else
  echo "[3/4] RESET_QUEUE=0, keeping Redis Stream history."
fi

echo "[4/4] Deleting stress DB rows..."
docker compose exec -T postgres psql -q -U "$DB_USER" -d "$DB_NAME" <<'SQL'
BEGIN;
DELETE FROM tickets WHERE event_id IN (SELECT id FROM events WHERE title LIKE '壓測活動-%');
DELETE FROM applications WHERE event_id IN (SELECT id FROM events WHERE title LIKE '壓測活動-%');
DELETE FROM ticket_types WHERE event_id IN (SELECT id FROM events WHERE title LIKE '壓測活動-%');
DELETE FROM events WHERE title LIKE '壓測活動-%';
DELETE FROM users WHERE email LIKE 'stress_%@company.com' OR email LIKE 'stress_mgr_%@company.com' OR employee_id LIKE 'STRESS_%' OR employee_id LIKE 'STRESS_MGR_%';
COMMIT;
SQL

echo "✅ Stress load-test data cleanup completed."
