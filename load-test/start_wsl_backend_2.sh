#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

SOURCE_CONTAINER="${SOURCE_CONTAINER:-ts_backend}"
BACKEND_2_NAME="${BACKEND_2_NAME:-ts_backend_wsl_2}"
NETWORK="${NETWORK:-corporate-event-ticketing-system-team13_default}"
HEALTH_URL="${HEALTH_URL:-http://${BACKEND_2_NAME}:8001/health}"
ENV_FILE="$(mktemp)"

cleanup() {
  rm -f "$ENV_FILE"
}
trap cleanup EXIT

IMAGE="$(docker inspect "$SOURCE_CONTAINER" --format '{{.Config.Image}}')"
docker inspect "$SOURCE_CONTAINER" --format '{{range .Config.Env}}{{println .}}{{end}}' |
  grep -v '^PATH=' > "$ENV_FILE"

docker stop "$BACKEND_2_NAME" >/dev/null 2>&1 || true

docker run --rm -d \
  --name "$BACKEND_2_NAME" \
  --network "$NETWORK" \
  --env-file "$ENV_FILE" \
  "$IMAGE" >/dev/null

sleep 2

docker ps --filter "name=${BACKEND_2_NAME}" --format '{{.Names}} {{.Status}} {{.Networks}}'
docker run --rm --network "$NETWORK" curlimages/curl:latest -fsS "$HEALTH_URL"
echo
