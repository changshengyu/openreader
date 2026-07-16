#!/usr/bin/env sh
set -eu

IMAGE="${IMAGE:-ghcr.io/changshengyu/openreader:latest}"
PORT="${PORT:-18080}"
HISTORICAL_VOLUME="${HISTORICAL_VOLUME:-0}"
ROOT="$(mktemp -d "${TMPDIR:-/tmp}/openreader-volume-smoke.XXXXXX")"
NAME="${NAME:-openreader-volume-smoke-$(basename "$ROOT")}"
PASSWORD="password123"
USERNAME="smoke_$$"
BASE_URL="http://127.0.0.1:${PORT}"

case "$HISTORICAL_VOLUME" in
  0|1) ;;
  *)
    echo "HISTORICAL_VOLUME must be 0 or 1" >&2
    exit 2
    ;;
esac

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

mkdir -p "$ROOT/data" "$ROOT/cache" "$ROOT/library" "$ROOT/retired-host"

if [ "$HISTORICAL_VOLUME" = "1" ]; then
  need go
  need shasum
  (
    cd backend
    GOCACHE="${GOCACHE:-$PWD/.gocache}" go run ./cmd/create-old-volume-fixture -root "$ROOT"
  )
  USERNAME="legacy_owner"
  PASSWORD="legacy-volume-secret"
fi

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
    -v "$ROOT/retired-host:/retired-host:ro" \
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

wait_removed() {
  i=0
  while docker inspect "$NAME" >/dev/null 2>&1; do
    if [ "$i" -ge 30 ]; then
      echo "container was stopped but not removed: $NAME" >&2
      exit 1
    fi
    i=$((i + 1))
    sleep 1
  done
}

json_field() {
  python3 -c 'import json,sys; print(json.load(sys.stdin)[sys.argv[1]])' "$1"
}

json_book_id() {
  python3 -c '
import json, sys
title = sys.argv[1]
for item in json.load(sys.stdin):
    if item.get("title") == title:
        print(item["id"])
        break
else:
    raise SystemExit("historical fixture book was not listed")
' "$1"
}

start_container
wait_health

if [ "$HISTORICAL_VOLUME" = "1" ]; then
  LOGIN_RESPONSE="$(curl -fsS -X POST "${BASE_URL}/api/auth/login" \
    -H 'Content-Type: application/json' \
    -d "{\"username\":\"${USERNAME}\",\"password\":\"${PASSWORD}\"}")"
  TOKEN="$(printf '%s' "$LOGIN_RESPONSE" | json_field token)"

  BOOKS_RESPONSE="$(curl -fsS "${BASE_URL}/api/books" -H "Authorization: Bearer ${TOKEN}")"
  BOOK_ID="$(printf '%s' "$BOOKS_RESPONSE" | json_book_id '旧卷 TXT 验证书')"
  ORIGINAL_ARCHIVE="$ROOT/library/data/${USERNAME}/old-volume-txt/legacy.txt"
  ORIGINAL_HASH="$(shasum -a 256 "$ORIGINAL_ARCHIVE" | awk '{print $1}')"

  CONTENT_RESPONSE="$(curl -fsS "${BASE_URL}/api/books/${BOOK_ID}/chapters/0/content" -H "Authorization: Bearer ${TOKEN}")"
  printf '%s' "$CONTENT_RESPONSE" | grep '旧卷归档正文只能从 library 读取' >/dev/null
  if printf '%s' "$CONTENT_RESPONSE" | grep -q 'retired host'; then
    echo "historical volume read the retired-host mount" >&2
    exit 1
  fi

  curl -fsS -X POST "${BASE_URL}/api/books/${BOOK_ID}/refresh-local" \
    -H "Authorization: Bearer ${TOKEN}" >/dev/null
  if [ "$(shasum -a 256 "$ORIGINAL_ARCHIVE" | awk '{print $1}')" != "$ORIGINAL_HASH" ]; then
    echo "historical volume refresh rewrote the original archive" >&2
    exit 1
  fi

  BACKUP_RESPONSE="$(curl -fsS -X POST "${BASE_URL}/api/backup/trigger" \
    -H "Authorization: Bearer ${TOKEN}")"
  BACKUP_NAME="$(printf '%s' "$BACKUP_RESPONSE" | json_field name)"
  curl -fsS "${BASE_URL}/api/backup/list" -H "Authorization: Bearer ${TOKEN}" | grep "$BACKUP_NAME" >/dev/null
  BACKUP_PATH="$ROOT/data/webdav/users/${USERNAME}/${BACKUP_NAME}"
  curl -fsS -X POST "${BASE_URL}/api/backup/restore-legado" \
    -H "Authorization: Bearer ${TOKEN}" \
    -F "file=@${BACKUP_PATH}" >/dev/null
  if [ "$(shasum -a 256 "$ORIGINAL_ARCHIVE" | awk '{print $1}')" != "$ORIGINAL_HASH" ]; then
    echo "historical backup restore rewrote the original archive" >&2
    exit 1
  fi

  docker stop "$NAME" >/dev/null
  wait_removed
  start_container
  wait_health

  LOGIN_RESPONSE="$(curl -fsS -X POST "${BASE_URL}/api/auth/login" \
    -H 'Content-Type: application/json' \
    -d "{\"username\":\"${USERNAME}\",\"password\":\"${PASSWORD}\"}")"
  TOKEN="$(printf '%s' "$LOGIN_RESPONSE" | json_field token)"
  CONTENT_RESPONSE="$(curl -fsS "${BASE_URL}/api/books/${BOOK_ID}/chapters/0/content" -H "Authorization: Bearer ${TOKEN}")"
  printf '%s' "$CONTENT_RESPONSE" | grep '旧卷归档正文只能从 library 读取' >/dev/null
  echo "OpenReader historical Docker volume/backup smoke passed for ${IMAGE}"
  exit 0
fi

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
wait_removed
start_container
wait_health

LOGIN_RESPONSE="$(curl -fsS -X POST "${BASE_URL}/api/auth/login" \
  -H 'Content-Type: application/json' \
  -d "{\"username\":\"${USERNAME}\",\"password\":\"${PASSWORD}\"}")"
printf '%s' "$LOGIN_RESPONSE" | json_field token >/dev/null

echo "OpenReader Docker volume/backup smoke passed for ${IMAGE}"
