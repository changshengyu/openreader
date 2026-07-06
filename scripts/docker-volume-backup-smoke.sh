#!/usr/bin/env sh
set -eu

IMAGE="${IMAGE:-ghcr.io/changshengyu/openreader:latest}"
PORT="${PORT:-18080}"
NAME="openreader-volume-smoke-$$"
ROOT="$(mktemp -d "${TMPDIR:-/tmp}/openreader-volume-smoke.XXXXXX")"
PASSWORD="password123"
USERNAME="smoke_$$"
BASE_URL="http://127.0.0.1:${PORT}"

cleanup() {
  docker stop "$NAME" >/dev/null 2>&1 || true
  if [ "${KEEP_OPENREADER_SMOKE:-0}" != "1" ]; then
    rm -rf "$ROOT"
  else
    echo "kept smoke directory: $ROOT"
  fi
}
trap cleanup EXIT INT TERM

need() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required command: $1" >&2
    exit 2
  }
}

need docker
need curl
need python3

mkdir -p "$ROOT/data" "$ROOT/cache" "$ROOT/library"

start_container() {
  docker run -d --rm \
    --name "$NAME" \
    -p "127.0.0.1:${PORT}:8080" \
    -e OPENREADER_ADDR=":8080" \
    -e OPENREADER_JWT_SECRET="openreader-smoke-secret-change-me" \
    -e OPENREADER_DATA_DIR="/app/data" \
    -e OPENREADER_CACHE_DIR="/app/cache" \
    -e OPENREADER_LIBRARY_DIR="/app/library" \
    -v "$ROOT/data:/app/data" \
    -v "$ROOT/cache:/app/cache" \
    -v "$ROOT/library:/app/library" \
    "$IMAGE" >/dev/null
}

wait_health() {
  i=0
  while [ "$i" -lt 60 ]; do
    if curl -fsS "${BASE_URL}/api/health" >/dev/null 2>&1; then
      return 0
    fi
    i=$((i + 1))
    sleep 1
  done
  echo "container did not become healthy" >&2
  docker logs "$NAME" >&2 || true
  exit 1
}

json_field() {
  python3 -c 'import json,sys; print(json.load(sys.stdin)[sys.argv[1]])' "$1"
}

start_container
wait_health

REGISTER_RESPONSE="$(curl -fsS -X POST "${BASE_URL}/api/auth/register" \
  -H 'Content-Type: application/json' \
  -d "{\"username\":\"${USERNAME}\",\"password\":\"${PASSWORD}\"}")"
TOKEN="$(printf '%s' "$REGISTER_RESPONSE" | json_field token)"

curl -fsS "${BASE_URL}/api/me" -H "Authorization: Bearer ${TOKEN}" >/dev/null

BACKUP_RESPONSE="$(curl -fsS -X POST "${BASE_URL}/api/backup/trigger" \
  -H "Authorization: Bearer ${TOKEN}")"
BACKUP_NAME="$(printf '%s' "$BACKUP_RESPONSE" | json_field name)"

curl -fsS "${BASE_URL}/api/backup/list" -H "Authorization: Bearer ${TOKEN}" | grep "$BACKUP_NAME" >/dev/null

docker stop "$NAME" >/dev/null
start_container
wait_health

LOGIN_RESPONSE="$(curl -fsS -X POST "${BASE_URL}/api/auth/login" \
  -H 'Content-Type: application/json' \
  -d "{\"username\":\"${USERNAME}\",\"password\":\"${PASSWORD}\"}")"
printf '%s' "$LOGIN_RESPONSE" | json_field token >/dev/null

echo "OpenReader Docker volume/backup smoke passed for ${IMAGE}"
