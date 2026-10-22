#!/usr/bin/env sh
# Release gate: boot a fresh container from IMAGE and load its frontend bundle
# in the lockfile-pinned Playwright headless shell. openreader-smoke.mjs owns
# the checks (three viewports; page errors, console errors, /api 5xx, empty #app).
set -eu

IMAGE="${IMAGE:?IMAGE must name the image to check}"
# Local use only: serve this frontend build through the image's own binary
# instead of the bundle baked into the image. The release workflow never sets it.
SMOKE_PUBLIC_DIR="${SMOKE_PUBLIC_DIR:-}"
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
NAME="openreader-browser-smoke-$$"

cleanup() {
  docker rm -f "$NAME" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

fail() {
  echo "$1" >&2
  docker logs "$NAME" >&2 || true
  exit 1
}

# No volumes: the image's own empty /app/data, /app/cache and /app/library make
# a fresh instance. Loopback-only, Docker-assigned port, so the checks below can
# only reach this container.
set -- docker run -d --name "$NAME" -p 127.0.0.1::8080 \
  -e OPENREADER_JWT_SECRET="openreader-browser-smoke-secret"
if [ -n "$SMOKE_PUBLIC_DIR" ]; then
  set -- "$@" -v "$(cd "$SMOKE_PUBLIC_DIR" && pwd):/app/public:ro"
fi
"$@" "$IMAGE" >/dev/null

PORT="$(docker port "$NAME" 8080/tcp | head -n 1 | sed 's/.*://')"
[ -n "$PORT" ] || fail "container published no loopback port"
BASE_URL="http://127.0.0.1:${PORT}"

i=0
until curl -fsS "${BASE_URL}/api/health" >/dev/null 2>&1; do
  i=$((i + 1))
  [ "$i" -lt 60 ] || fail "container did not become healthy"
  sleep 1
done
echo "health: $(curl -fsS "${BASE_URL}/api/health")"

# Only the pinned headless shell (no CDP or custom executable), only the entry page.
env -u CDP_URL -u CHROME_PATH -u SMOKE_READER_URL TARGET_URL="$BASE_URL" \
  node "$ROOT/scripts/smoke/openreader-smoke.mjs" || fail "browser smoke failed against $IMAGE"
