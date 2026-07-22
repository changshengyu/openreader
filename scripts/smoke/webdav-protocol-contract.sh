#!/usr/bin/env sh
set -eu

TARGET_URL="${TARGET_URL:-http://127.0.0.1:18089}"
TARGET_URL="${TARGET_URL%/}"
USERNAME="${WEBDAV_USERNAME:-davsmoke}"
PASSWORD="${WEBDAV_PASSWORD:-davsmoke123}"
REGISTER="${REGISTER:-1}"
ROOT_NAME="${WEBDAV_SMOKE_ROOT:-protocol-smoke-$$}"
TMP_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/openreader-webdav-smoke.XXXXXX")"
HEADERS="$TMP_ROOT/headers"
BODY="$TMP_ROOT/body"

cleanup() {
  curl -sS --user "$USERNAME:$PASSWORD" -X DELETE \
    "$TARGET_URL/reader3/webdav/$ROOT_NAME" >/dev/null 2>&1 || true
  rm -rf "$TMP_ROOT"
}
trap cleanup EXIT INT TERM

request() {
  method="$1"
  url="$2"
  shift 2
  curl -sS -D "$HEADERS" -o "$BODY" -w '%{http_code}' -X "$method" "$url" "$@"
}

assert_status() {
  actual="$1"
  expected="$2"
  action="$3"
  if [ "$actual" != "$expected" ]; then
    echo "$action returned $actual, expected $expected" >&2
    sed -n '1,40p' "$BODY" >&2
    exit 1
  fi
}

assert_header() {
  name="$1"
  expected="$2"
  if ! awk -v wanted="$name" -v expected="$expected" '
    BEGIN { IGNORECASE = 1; found = 0 }
    {
      line = $0
      sub(/\r$/, "", line)
      split(line, parts, ":")
      if (tolower(parts[1]) == tolower(wanted)) {
        value = substr(line, length(parts[1]) + 2)
        sub(/^[[:space:]]+/, "", value)
        if (value == expected) found = 1
      }
    }
    END { exit found ? 0 : 1 }
  ' "$HEADERS"; then
    echo "missing header $name: $expected" >&2
    sed -n '1,40p' "$HEADERS" >&2
    exit 1
  fi
}

if [ "$REGISTER" = "1" ]; then
  status="$(request POST "$TARGET_URL/api/auth/register" \
    -H 'Content-Type: application/json' \
    --data "{\"username\":\"$USERNAME\",\"password\":\"$PASSWORD\"}")"
  assert_status "$status" 200 "register smoke user"
fi

status="$(request OPTIONS "$TARGET_URL/reader3/webdav/")"
assert_status "$status" 200 "anonymous OPTIONS"
assert_header DAV "1,2"
assert_header MS-Author-Via DAV
assert_header Allow "OPTIONS, DELETE, GET, PUT, PROPFIND, MKCOL, MOVE, COPY, LOCK, UNLOCK"

status="$(request PROPFIND "$TARGET_URL/reader3/webdav/")"
assert_status "$status" 401 "anonymous PROPFIND"
assert_header WWW-Authenticate 'Basic realm="OpenReader WebDAV"'

status="$(request PROPFIND "$TARGET_URL/reader3/webdav/" --user "$USERNAME:wrong-password")"
assert_status "$status" 401 "invalid Basic PROPFIND"

status="$(request PROPFIND "$TARGET_URL/reader3/webdav/" --user "$USERNAME:$PASSWORD" -H 'Depth: 0')"
assert_status "$status" 207 "authenticated root PROPFIND"
grep -F 'DAV:' "$BODY" >/dev/null

status="$(request PUT "$TARGET_URL/reader3/webdav/$ROOT_NAME/missing.txt" \
  --user "$USERNAME:$PASSWORD" --data-binary 'missing parent')"
assert_status "$status" 409 "PUT without parent"

status="$(request MKCOL "$TARGET_URL/reader3/webdav/$ROOT_NAME" --user "$USERNAME:$PASSWORD")"
assert_status "$status" 201 "MKCOL smoke directory"

status="$(request PUT "$TARGET_URL/reader3/webdav/$ROOT_NAME/source.txt" \
  --user "$USERNAME:$PASSWORD" --data-binary 'webdav protocol smoke')"
assert_status "$status" 201 "PUT source"

status="$(request PROPFIND "$TARGET_URL/reader3/webdav/$ROOT_NAME" \
  --user "$USERNAME:$PASSWORD" -H 'Depth: 1')"
assert_status "$status" 207 "directory PROPFIND"
grep -F 'source.txt' "$BODY" >/dev/null

status="$(request COPY "$TARGET_URL/reader3/webdav/$ROOT_NAME/source.txt" \
  --user "$USERNAME:$PASSWORD" \
  -H "Destination: $TARGET_URL/webdav/$ROOT_NAME/copied.txt")"
assert_status "$status" 201 "cross-prefix COPY"

status="$(request GET "$TARGET_URL/webdav/$ROOT_NAME/copied.txt" --user "$USERNAME:$PASSWORD")"
assert_status "$status" 200 "current-prefix GET"
grep -Fx 'webdav protocol smoke' "$BODY" >/dev/null

status="$(request LOCK "$TARGET_URL/reader3/webdav/$ROOT_NAME/copied.txt" --user "$USERNAME:$PASSWORD")"
assert_status "$status" 200 "LOCK"
lock_token="$(awk 'BEGIN { IGNORECASE = 1 } tolower($1) == "lock-token:" { sub(/\r$/, "", $2); print $2; exit }' "$HEADERS")"
case "$lock_token" in
  urn:uuid:*) ;;
  *) echo "LOCK did not return a UUID token" >&2; exit 1 ;;
esac

status="$(request UNLOCK "$TARGET_URL/reader3/webdav/$ROOT_NAME/copied.txt" \
  --user "$USERNAME:$PASSWORD" -H "Lock-Token: $lock_token")"
assert_status "$status" 204 "UNLOCK"

status="$(request DELETE "$TARGET_URL/reader3/webdav/$ROOT_NAME/copied.txt" --user "$USERNAME:$PASSWORD")"
assert_status "$status" 200 "upstream-prefix DELETE"

status="$(request DELETE "$TARGET_URL/reader3/webdav/$ROOT_NAME" --user "$USERNAME:$PASSWORD")"
assert_status "$status" 200 "recursive cleanup DELETE"

echo "OpenReader live WebDAV protocol smoke passed for $TARGET_URL"
