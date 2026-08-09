#!/usr/bin/env sh
set -eu

IMAGE="${IMAGE:-ghcr.io/changshengyu/openreader:latest}"
PORT="${PORT:-18280}"
FIXTURE_PORT="${FIXTURE_PORT:-$((PORT + 100))}"
PUBLIC_SOURCE_URL="${PUBLIC_SOURCE_URL:-https://raw.githubusercontent.com/changshengyu/openreader/main/README.md}"
ROOT="$(mktemp -d "${TMPDIR:-/tmp}/openreader-source-network.XXXXXX")"
NAME="${NAME:-openreader-source-network-$(basename "$ROOT")}"
INVALID_NAME="${INVALID_NAME:-${NAME}-invalid}"
BASE_URL="http://127.0.0.1:${PORT}"
FIXTURE_URL="http://host.docker.internal:${FIXTURE_PORT}/sources.json"
USERNAME="networkadmin$$"
PASSWORD="password123"
FIXTURE_PID=""

cleanup() {
  docker stop "$NAME" >/dev/null 2>&1 || true
  docker rm -f "$INVALID_NAME" >/dev/null 2>&1 || true
  if [ -n "$FIXTURE_PID" ]; then
    kill "$FIXTURE_PID" >/dev/null 2>&1 || true
  fi
  if [ "${KEEP_OPENREADER_NETWORK_SMOKE:-0}" = "1" ]; then
    echo "kept source-network smoke root: $ROOT"
  else
    rm -rf "$ROOT"
  fi
}
trap cleanup EXIT INT TERM

need() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required command: $1" >&2
    exit 2
  }
}

need curl
need docker
need python3

mkdir -p "$ROOT/data" "$ROOT/cache" "$ROOT/library" "$ROOT/invalid-data"
printf '0' >"$ROOT/fixture-count"

python3 -c '
import http.server, pathlib, sys
port = int(sys.argv[1])
counter = pathlib.Path(sys.argv[2])
class Handler(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        value = int(counter.read_text()) + 1
        counter.write_text(str(value))
        body = b"[]"
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)
    def log_message(self, *_):
        pass
http.server.ThreadingHTTPServer(("0.0.0.0", port), Handler).serve_forever()
' "$FIXTURE_PORT" "$ROOT/fixture-count" &
FIXTURE_PID="$!"

attempt=0
while [ "$attempt" -lt 30 ]; do
  if curl -fsS "http://127.0.0.1:${FIXTURE_PORT}/health" >/dev/null 2>&1; then
    break
  fi
  attempt=$((attempt + 1))
  sleep 1
done
if [ "$attempt" -ge 30 ]; then
  echo "source-network host fixture did not start" >&2
  exit 1
fi

fixture_count() {
  tr -d '[:space:]' <"$ROOT/fixture-count"
}

json_field() {
  python3 -c 'import json,sys; print(json.load(sys.stdin)[sys.argv[1]])' "$1"
}

assert_error() {
  path="$1"
  expected="$2"
  python3 -c '
import json, sys
actual = json.load(open(sys.argv[1])).get("error")
if actual != sys.argv[2]:
    raise SystemExit(f"error={actual!r}, want={sys.argv[2]!r}")
' "$path" "$expected"
}

assert_source_persisted() {
  token="$1"
  curl -fsS "${BASE_URL}/api/sources" -H "Authorization: Bearer ${token}" |
    python3 -c '
import json, sys
rows = json.load(sys.stdin)
if not any(row.get("name") == "LAN retained source" for row in rows):
    raise SystemExit("LAN retained source disappeared across policy restart")
'
}

start_container() {
  allowlist="$1"
  if [ -n "$allowlist" ]; then
    docker run -d --rm \
      --name "$NAME" \
      -p "127.0.0.1:${PORT}:8080" \
      -e OPENREADER_ADDR=":8080" \
      -e OPENREADER_JWT_SECRET="openreader-source-network-smoke-secret" \
      -e OPENREADER_SOURCE_NETWORK_ALLOWLIST="$allowlist" \
      -v "$ROOT/data:/app/data" \
      -v "$ROOT/cache:/app/cache" \
      -v "$ROOT/library:/app/library" \
      "$IMAGE" >/dev/null
  else
    docker run -d --rm \
      --name "$NAME" \
      -p "127.0.0.1:${PORT}:8080" \
      -e OPENREADER_ADDR=":8080" \
      -e OPENREADER_JWT_SECRET="openreader-source-network-smoke-secret" \
      -v "$ROOT/data:/app/data" \
      -v "$ROOT/cache:/app/cache" \
      -v "$ROOT/library:/app/library" \
      "$IMAGE" >/dev/null
  fi
}

wait_health() {
  attempt=0
  while [ "$attempt" -lt 60 ]; do
    if curl -fsS "${BASE_URL}/api/health" >/dev/null 2>&1; then
      return 0
    fi
    attempt=$((attempt + 1))
    sleep 1
  done
  echo "source-network OpenReader container did not become healthy" >&2
  docker logs "$NAME" >&2 || true
  exit 1
}

stop_container() {
  docker stop "$NAME" >/dev/null
  attempt=0
  while docker inspect "$NAME" >/dev/null 2>&1; do
    if [ "$attempt" -ge 30 ]; then
      echo "source-network container was not removed" >&2
      exit 1
    fi
    attempt=$((attempt + 1))
    sleep 1
  done
}

login() {
  curl -fsS -X POST "${BASE_URL}/api/auth/login" \
    -H 'Content-Type: application/json' \
    -d "{\"username\":\"${USERNAME}\",\"password\":\"${PASSWORD}\"}" |
    json_field token
}

preview_status() {
  token="$1"
  url="$2"
  output="$3"
  curl -sS -o "$output" -w '%{http_code}' -X POST "${BASE_URL}/api/sources/remote-preview" \
    -H "Authorization: Bearer ${token}" \
    -H 'Content-Type: application/json' \
    -d "{\"url\":\"${url}\"}"
}

# Invalid deployment policy must fail before opening SQLite and must not echo
# the private configuration value.
docker run -d \
  --name "$INVALID_NAME" \
  -e OPENREADER_SOURCE_NETWORK_ALLOWLIST="http://private-secret.internal" \
  -v "$ROOT/invalid-data:/app/data" \
  "$IMAGE" >/dev/null
sleep 1
if [ "$(docker inspect -f '{{.State.Running}}' "$INVALID_NAME")" = "true" ]; then
  echo "invalid source network allowlist started the server" >&2
  exit 1
fi
INVALID_LOGS="$(docker logs "$INVALID_NAME" 2>&1 || true)"
case "$INVALID_LOGS" in
  *private-secret*)
    echo "invalid allowlist value leaked into startup logs" >&2
    exit 1
    ;;
  *"invalid source network allowlist entry 1"*) ;;
  *)
    echo "invalid allowlist did not produce the stable startup error" >&2
    printf '%s\n' "$INVALID_LOGS" >&2
    exit 1
    ;;
esac
if [ -e "$ROOT/invalid-data/openreader.db" ]; then
  echo "invalid allowlist opened SQLite before failing startup" >&2
  exit 1
fi
docker rm "$INVALID_NAME" >/dev/null

# Strict mode: public destination is reachable, host gateway and loopback are
# rejected before the host fixture sees a request.
start_container ""
wait_health
REGISTER_RESPONSE="$(curl -fsS -X POST "${BASE_URL}/api/auth/register" \
  -H 'Content-Type: application/json' \
  -d "{\"username\":\"${USERNAME}\",\"password\":\"${PASSWORD}\"}")"
TOKEN="$(printf '%s' "$REGISTER_RESPONSE" | json_field token)"
curl -fsS -X POST "${BASE_URL}/api/sources" \
  -H "Authorization: Bearer ${TOKEN}" \
  -H 'Content-Type: application/json' \
  -d "{\"bookSourceName\":\"LAN retained source\",\"bookSourceUrl\":\"${FIXTURE_URL}\",\"enabled\":true}" >/dev/null
BASELINE_COUNT="$(fixture_count)"

PUBLIC_STATUS="$(preview_status "$TOKEN" "$PUBLIC_SOURCE_URL" "$ROOT/public.json")"
if [ "$PUBLIC_STATUS" != "400" ]; then
  echo "public source probe status=$PUBLIC_STATUS" >&2
  cat "$ROOT/public.json" >&2
  exit 1
fi
assert_error "$ROOT/public.json" "invalid remote JSON format"

STRICT_STATUS="$(preview_status "$TOKEN" "$FIXTURE_URL" "$ROOT/strict.json")"
if [ "$STRICT_STATUS" != "400" ]; then
  echo "strict host-gateway probe status=$STRICT_STATUS" >&2
  cat "$ROOT/strict.json" >&2
  exit 1
fi
assert_error "$ROOT/strict.json" "failed to fetch remote source URL"
if [ "$(fixture_count)" != "$BASELINE_COUNT" ]; then
  echo "strict policy reached the host-gateway fixture" >&2
  exit 1
fi
stop_container

# Exact administrator hostname allowlist restores the LAN fixture without
# granting unrelated loopback addresses, and the existing source row survives.
start_container "host.docker.internal"
wait_health
TOKEN="$(login)"
assert_source_persisted "$TOKEN"
ALLOW_STATUS="$(preview_status "$TOKEN" "$FIXTURE_URL" "$ROOT/allowed.json")"
if [ "$ALLOW_STATUS" != "200" ]; then
  echo "allowlisted host-gateway probe status=$ALLOW_STATUS" >&2
  cat "$ROOT/allowed.json" >&2
  exit 1
fi
python3 -c '
import json, sys
payload = json.load(open(sys.argv[1]))
if payload.get("count") != 0 or payload.get("sources") != []:
    raise SystemExit(f"unexpected allowlisted preview payload: {payload}")
' "$ROOT/allowed.json"
if [ "$(fixture_count)" != "$((BASELINE_COUNT + 1))" ]; then
  echo "allowlisted fixture request count did not advance exactly once" >&2
  exit 1
fi
LOOPBACK_STATUS="$(preview_status "$TOKEN" "http://127.0.0.1:${FIXTURE_PORT}/sources.json" "$ROOT/loopback.json")"
if [ "$LOOPBACK_STATUS" != "400" ]; then
  echo "unrelated loopback probe status=$LOOPBACK_STATUS" >&2
  cat "$ROOT/loopback.json" >&2
  exit 1
fi
assert_error "$ROOT/loopback.json" "failed to fetch remote source URL"
stop_container

# Removing the deployment allowlist and restarting restores strict mode while
# preserving the historical source data and leaving the fixture untouched.
start_container ""
wait_health
TOKEN="$(login)"
assert_source_persisted "$TOKEN"
FINAL_COUNT="$(fixture_count)"
FINAL_STATUS="$(preview_status "$TOKEN" "$FIXTURE_URL" "$ROOT/final.json")"
if [ "$FINAL_STATUS" != "400" ]; then
  echo "post-allowlist strict probe status=$FINAL_STATUS" >&2
  cat "$ROOT/final.json" >&2
  exit 1
fi
assert_error "$ROOT/final.json" "failed to fetch remote source URL"
if [ "$(fixture_count)" != "$FINAL_COUNT" ]; then
  echo "removed allowlist still reached the host fixture" >&2
  exit 1
fi

echo "source network policy Docker smoke passed"
