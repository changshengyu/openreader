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

archive_hash() {
  shasum -a 256 "$1" | awk '{print $1}'
}

assert_archive_hash() {
  archive="$1"
  expected="$2"
  phase="$3"
  actual="$(archive_hash "$archive")"
  if [ "$actual" != "$expected" ]; then
    echo "${phase} rewrote original archive: $archive" >&2
    exit 1
  fi
}

read_historical_book() {
  book_id="$1"
  expected_format="$2"
  expected_content="$3"
  response="$(curl -fsS "${BASE_URL}/api/books/${book_id}/chapters/0/content" -H "Authorization: Bearer ${TOKEN}")"
  if [ -n "$expected_format" ]; then
    printf '%s' "$response" | grep "\"format\":\"${expected_format}\"" >/dev/null
  fi
  if [ -n "$expected_content" ]; then
    printf '%s' "$response" | grep -F "$expected_content" >/dev/null
  fi
  if printf '%s' "$response" | grep -q 'retired host'; then
    echo "historical volume read the retired-host mount" >&2
    exit 1
  fi
  printf '%s' "$response"
}

refresh_historical_book() {
  curl -fsS -X POST "${BASE_URL}/api/books/$1/refresh-local" \
    -H "Authorization: Bearer ${TOKEN}" >/dev/null
}

start_container
wait_health

if [ "$HISTORICAL_VOLUME" = "1" ]; then
  LOGIN_RESPONSE="$(curl -fsS -X POST "${BASE_URL}/api/auth/login" \
    -H 'Content-Type: application/json' \
    -d "{\"username\":\"${USERNAME}\",\"password\":\"${PASSWORD}\"}")"
  TOKEN="$(printf '%s' "$LOGIN_RESPONSE" | json_field token)"

  BOOKS_RESPONSE="$(curl -fsS "${BASE_URL}/api/books" -H "Authorization: Bearer ${TOKEN}")"
  TXT_BOOK_ID="$(printf '%s' "$BOOKS_RESPONSE" | json_book_id '旧卷 TXT 验证书')"
  EPUB_BOOK_ID="$(printf '%s' "$BOOKS_RESPONSE" | json_book_id '旧卷 EPUB 验证书')"
  UMD_BOOK_ID="$(printf '%s' "$BOOKS_RESPONSE" | json_book_id '旧卷 UMD 验证书')"
  CBZ_BOOK_ID="$(printf '%s' "$BOOKS_RESPONSE" | json_book_id '旧卷 CBZ 验证书')"

  TXT_ARCHIVE="$ROOT/library/data/${USERNAME}/old-volume-txt/legacy.txt"
  EPUB_ARCHIVE="$ROOT/library/data/${USERNAME}/old-volume-epub/legacy.epub"
  UMD_ARCHIVE="$ROOT/library/data/${USERNAME}/old-volume-umd/legacy.umd"
  CBZ_ARCHIVE="$ROOT/library/data/${USERNAME}/old-volume-cbz/legacy.cbz"
  TXT_HASH="$(archive_hash "$TXT_ARCHIVE")"
  EPUB_HASH="$(archive_hash "$EPUB_ARCHIVE")"
  UMD_HASH="$(archive_hash "$UMD_ARCHIVE")"
  CBZ_HASH="$(archive_hash "$CBZ_ARCHIVE")"

  read_historical_book "$TXT_BOOK_ID" "" '旧卷归档正文只能从 library 读取' >/dev/null
  read_historical_book "$EPUB_BOOK_ID" "epub" "" >/dev/null
  read_historical_book "$UMD_BOOK_ID" "" '第一段' >/dev/null
  CBZ_RESPONSE="$(read_historical_book "$CBZ_BOOK_ID" "cbz" "")"
  CBZ_RESOURCE_URL="$(printf '%s' "$CBZ_RESPONSE" | json_field resourceUrl)"
  curl -fsS "${BASE_URL}${CBZ_RESOURCE_URL}" | grep -F 'old-volume-first-page' >/dev/null

  for book_id in "$TXT_BOOK_ID" "$EPUB_BOOK_ID" "$UMD_BOOK_ID" "$CBZ_BOOK_ID"; do
    refresh_historical_book "$book_id"
  done
  assert_archive_hash "$TXT_ARCHIVE" "$TXT_HASH" 'historical volume refresh'
  assert_archive_hash "$EPUB_ARCHIVE" "$EPUB_HASH" 'historical volume refresh'
  assert_archive_hash "$UMD_ARCHIVE" "$UMD_HASH" 'historical volume refresh'
  assert_archive_hash "$CBZ_ARCHIVE" "$CBZ_HASH" 'historical volume refresh'

  BACKUP_RESPONSE="$(curl -fsS -X POST "${BASE_URL}/api/backup/trigger" \
    -H "Authorization: Bearer ${TOKEN}")"
  BACKUP_NAME="$(printf '%s' "$BACKUP_RESPONSE" | json_field name)"
  curl -fsS "${BASE_URL}/api/backup/list" -H "Authorization: Bearer ${TOKEN}" | grep "$BACKUP_NAME" >/dev/null
  BACKUP_PATH="$ROOT/data/webdav/users/${USERNAME}/${BACKUP_NAME}"
  curl -fsS -X POST "${BASE_URL}/api/backup/restore-legado" \
    -H "Authorization: Bearer ${TOKEN}" \
    -F "file=@${BACKUP_PATH}" >/dev/null
  assert_archive_hash "$TXT_ARCHIVE" "$TXT_HASH" 'historical backup restore'
  assert_archive_hash "$EPUB_ARCHIVE" "$EPUB_HASH" 'historical backup restore'
  assert_archive_hash "$UMD_ARCHIVE" "$UMD_HASH" 'historical backup restore'
  assert_archive_hash "$CBZ_ARCHIVE" "$CBZ_HASH" 'historical backup restore'

  docker stop "$NAME" >/dev/null
  wait_removed
  start_container
  wait_health

  LOGIN_RESPONSE="$(curl -fsS -X POST "${BASE_URL}/api/auth/login" \
    -H 'Content-Type: application/json' \
    -d "{\"username\":\"${USERNAME}\",\"password\":\"${PASSWORD}\"}")"
  TOKEN="$(printf '%s' "$LOGIN_RESPONSE" | json_field token)"
  read_historical_book "$TXT_BOOK_ID" "" '旧卷归档正文只能从 library 读取' >/dev/null
  read_historical_book "$EPUB_BOOK_ID" "epub" "" >/dev/null
  read_historical_book "$UMD_BOOK_ID" "" '第一段' >/dev/null
  CBZ_RESPONSE="$(read_historical_book "$CBZ_BOOK_ID" "cbz" "")"
  CBZ_RESOURCE_URL="$(printf '%s' "$CBZ_RESPONSE" | json_field resourceUrl)"
  curl -fsS "${BASE_URL}${CBZ_RESOURCE_URL}" | grep -F 'old-volume-first-page' >/dev/null
  echo "OpenReader historical Docker volume/backup smoke passed for ${IMAGE} (txt, epub, umd, cbz)"
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
