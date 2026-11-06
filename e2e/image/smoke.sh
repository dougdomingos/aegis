#!/usr/bin/env bash
# Smoke test for the Orchestrator container image.
#
# Starts the image with throwaway secrets, waits for the REST API to answer a
# database-backed route, checks that the gRPC port accepts connections, and
# verifies the process is still alive afterwards.
#
# Usage: e2e/image/smoke.sh <image>
set -euo pipefail

IMAGE="${1:?usage: smoke.sh <image>}"
HTTP_PORT="${SMOKE_HTTP_PORT:-18080}"
GRPC_PORT="${SMOKE_GRPC_PORT:-19090}"
TIMEOUT_SECONDS="${SMOKE_TIMEOUT_SECONDS:-60}"

WORKDIR="$(mktemp -d)"
CONTAINER="aegis-smoke-$$"

cleanup() {
  status=$?
  if [ "$status" -ne 0 ]; then
    echo "--- container logs ---"
    docker logs "$CONTAINER" 2>&1 || true
  fi
  docker rm -f "$CONTAINER" >/dev/null 2>&1 || true
  rm -rf "$WORKDIR"
  exit "$status"
}
trap cleanup EXIT

# Throwaway secrets: the smoke test never talks to a real lab.
openssl rand -hex 32 > "$WORKDIR/lab.key"

docker run -d --name "$CONTAINER" \
  -p "127.0.0.1:${HTTP_PORT}:8080" \
  -p "127.0.0.1:${GRPC_PORT}:9090" \
  -e AEGIS_LAB_KEY=/run/secrets/lab.key \
  -e AEGIS_SESSION_SECRET="smoke-$(openssl rand -hex 16)" \
  -v "$WORKDIR/lab.key:/run/secrets/lab.key:ro" \
  "$IMAGE" >/dev/null

echo "waiting for REST API on :${HTTP_PORT} (timeout ${TIMEOUT_SECONDS}s)..."
deadline=$((SECONDS + TIMEOUT_SECONDS))
until [ "$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:${HTTP_PORT}/machines/")" = "200" ]; do
  if [ "$SECONDS" -ge "$deadline" ]; then
    echo "FAIL: GET /machines/ did not return 200 within ${TIMEOUT_SECONDS}s"
    exit 1
  fi
  if [ "$(docker inspect -f '{{.State.Running}}' "$CONTAINER")" != "true" ]; then
    echo "FAIL: container exited before becoming ready"
    exit 1
  fi
  sleep 1
done
echo "OK: GET /machines/ returned 200"

if timeout 5 bash -c "exec 3<>/dev/tcp/127.0.0.1/${GRPC_PORT}"; then
  echo "OK: gRPC port ${GRPC_PORT} accepts connections"
else
  echo "FAIL: gRPC port ${GRPC_PORT} is not reachable"
  exit 1
fi

if [ "$(docker inspect -f '{{.State.Running}}' "$CONTAINER")" != "true" ]; then
  echo "FAIL: container stopped after startup checks"
  exit 1
fi
echo "OK: container still running after checks"
echo "smoke test passed for ${IMAGE}"
