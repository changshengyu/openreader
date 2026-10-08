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

for prefix in reader3/webdav webdav; do
  for attempt in 1 2; do
    status="$(request MKCOL "$TARGET_URL/$prefix/$ROOT_NAME/mkdir/nested/leaf" --user "$USERNAME:$PASSWORD")"
    assert_status "$status" 201 "recursive/idempotent MKCOL ($prefix/$attempt)"
    [ ! -s "$BODY" ]
  done
done
status="$(request PROPFIND "$TARGET_URL/webdav/$ROOT_NAME/mkdir/nested/leaf" --user "$USERNAME:$PASSWORD" -H 'Depth: 0')"
assert_status "$status" 207 "MKCOL recursive leaf exists"

status="$(request PUT "$TARGET_URL/reader3/webdav/$ROOT_NAME/source.txt" \
  --user "$USERNAME:$PASSWORD" --data-binary 'webdav protocol smoke')"
assert_status "$status" 201 "PUT source"

status="$(request MKCOL "$TARGET_URL/webdav/$ROOT_NAME/source.txt" --user "$USERNAME:$PASSWORD")"
assert_status "$status" 409 "MKCOL preserves existing regular file"
[ ! -s "$BODY" ]

status="$(request PUT "$TARGET_URL/webdav/$ROOT_NAME/source.txt" \
  --user "$USERNAME:$PASSWORD" --data-binary 'overwritten content')"
assert_status "$status" 201 "cross-prefix PUT overwrite"
[ ! -s "$BODY" ]
status="$(request GET "$TARGET_URL/reader3/webdav/$ROOT_NAME/source.txt" --user "$USERNAME:$PASSWORD")"
assert_status "$status" 200 "GET overwritten source"
grep -Fx 'overwritten content' "$BODY" >/dev/null

status="$(request PUT "$TARGET_URL/webdav/$ROOT_NAME/source.txt" \
  --user "$USERNAME:$PASSWORD" --data-binary '')"
assert_status "$status" 201 "PUT empty overwrite"
status="$(request GET "$TARGET_URL/reader3/webdav/$ROOT_NAME/source.txt" --user "$USERNAME:$PASSWORD")"
assert_status "$status" 200 "GET empty source"
[ ! -s "$BODY" ]

status="$(request PUT "$TARGET_URL/reader3/webdav/$ROOT_NAME" \
  --user "$USERNAME:$PASSWORD" --data-binary 'cannot replace directory')"
assert_status "$status" 405 "PUT directory target"

status="$(request PUT "$TARGET_URL/reader3/webdav/$ROOT_NAME/source.txt" \
  --user "$USERNAME:$PASSWORD" --data-binary 'webdav protocol smoke')"
assert_status "$status" 201 "restore smoke source"

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

status="$(request COPY "$TARGET_URL/webdav/$ROOT_NAME/source.txt" \
  --user "$USERNAME:$PASSWORD" -H "Destination: $TARGET_URL/reader3/webdav/$ROOT_NAME/copied.txt")"
assert_status "$status" 412 "COPY existing target without overwrite"

status="$(request PUT "$TARGET_URL/webdav/$ROOT_NAME/source.txt" \
  --user "$USERNAME:$PASSWORD" --data-binary 'copy overwrite bytes')"
assert_status "$status" 201 "prepare COPY overwrite"
status="$(request COPY "$TARGET_URL/webdav/$ROOT_NAME/source.txt" \
  --user "$USERNAME:$PASSWORD" -H "Destination: $TARGET_URL/reader3/webdav/$ROOT_NAME/copied.txt" -H 'Overwrite: T')"
assert_status "$status" 201 "COPY overwrite existing file"
[ ! -s "$BODY" ]
status="$(request GET "$TARGET_URL/webdav/$ROOT_NAME/copied.txt" --user "$USERNAME:$PASSWORD")"
assert_status "$status" 200 "GET COPY overwrite"
grep -Fx 'copy overwrite bytes' "$BODY" >/dev/null

for directory in tree tree/nested tree/empty; do
  status="$(request MKCOL "$TARGET_URL/webdav/$ROOT_NAME/$directory" --user "$USERNAME:$PASSWORD")"
  assert_status "$status" 201 "prepare COPY tree $directory"
done
status="$(request PUT "$TARGET_URL/webdav/$ROOT_NAME/tree/nested/file.txt" \
  --user "$USERNAME:$PASSWORD" --data-binary 'nested copy bytes')"
assert_status "$status" 201 "prepare nested COPY file"
status="$(request COPY "$TARGET_URL/reader3/webdav/$ROOT_NAME/tree" \
  --user "$USERNAME:$PASSWORD" -H "Destination: $TARGET_URL/webdav/$ROOT_NAME/tree-copy")"
assert_status "$status" 201 "COPY recursive tree"
status="$(request GET "$TARGET_URL/webdav/$ROOT_NAME/tree-copy/nested/file.txt" --user "$USERNAME:$PASSWORD")"
assert_status "$status" 200 "GET recursive COPY file"
grep -Fx 'nested copy bytes' "$BODY" >/dev/null
status="$(request PROPFIND "$TARGET_URL/webdav/$ROOT_NAME/tree-copy/empty" --user "$USERNAME:$PASSWORD" -H 'Depth: 0')"
assert_status "$status" 207 "COPY preserves empty directory"

status="$(request COPY "$TARGET_URL/webdav/$ROOT_NAME/source.txt" \
  --user "$USERNAME:$PASSWORD" -H "Destination: $TARGET_URL/reader3/webdav/$ROOT_NAME/tree-copy" -H 'Overwrite: T')"
assert_status "$status" 201 "COPY file over old directory"
status="$(request GET "$TARGET_URL/webdav/$ROOT_NAME/tree-copy" --user "$USERNAME:$PASSWORD")"
assert_status "$status" 200 "GET file over copied directory"
grep -Fx 'copy overwrite bytes' "$BODY" >/dev/null

status="$(request PUT "$TARGET_URL/webdav/$ROOT_NAME/move-source.txt" \
  --user "$USERNAME:$PASSWORD" --data-binary 'move source bytes')"
assert_status "$status" 201 "prepare MOVE source"
status="$(request MOVE "$TARGET_URL/reader3/webdav/$ROOT_NAME/move-source.txt" \
  --user "$USERNAME:$PASSWORD" -H "Destination: $TARGET_URL/webdav/$ROOT_NAME/copied.txt")"
assert_status "$status" 412 "MOVE existing target without overwrite"
status="$(request PUT "$TARGET_URL/webdav/$ROOT_NAME/move-target.txt" \
  --user "$USERNAME:$PASSWORD" --data-binary 'old move target')"
assert_status "$status" 201 "prepare MOVE target"
status="$(request MOVE "$TARGET_URL/reader3/webdav/$ROOT_NAME/move-source.txt" \
  --user "$USERNAME:$PASSWORD" -H "Destination: $TARGET_URL/webdav/$ROOT_NAME/move-target.txt" -H 'Overwrite: T')"
assert_status "$status" 201 "cross-prefix MOVE file overwrite"
[ ! -s "$BODY" ]
status="$(request GET "$TARGET_URL/webdav/$ROOT_NAME/move-source.txt" --user "$USERNAME:$PASSWORD")"
assert_status "$status" 404 "MOVE removes original source name"
status="$(request GET "$TARGET_URL/webdav/$ROOT_NAME/move-target.txt" --user "$USERNAME:$PASSWORD")"
assert_status "$status" 200 "GET moved file"
grep -Fx 'move source bytes' "$BODY" >/dev/null
status="$(request MOVE "$TARGET_URL/webdav/$ROOT_NAME/tree" \
  --user "$USERNAME:$PASSWORD" -H "Destination: $TARGET_URL/reader3/webdav/$ROOT_NAME/move-target.txt" -H 'Overwrite: T')"
assert_status "$status" 201 "MOVE directory over old file"
status="$(request GET "$TARGET_URL/webdav/$ROOT_NAME/move-target.txt/nested/file.txt" --user "$USERNAME:$PASSWORD")"
assert_status "$status" 200 "GET moved directory member"
grep -Fx 'nested copy bytes' "$BODY" >/dev/null
status="$(request PROPFIND "$TARGET_URL/reader3/webdav/$ROOT_NAME/move-target.txt/empty" --user "$USERNAME:$PASSWORD" -H 'Depth: 0')"
assert_status "$status" 207 "MOVE preserves empty directory"
status="$(request PUT "$TARGET_URL/webdav/$ROOT_NAME/move-source.txt" \
  --user "$USERNAME:$PASSWORD" --data-binary 'final move bytes')"
assert_status "$status" 201 "prepare MOVE file over directory"
status="$(request MOVE "$TARGET_URL/webdav/$ROOT_NAME/move-source.txt" \
  --user "$USERNAME:$PASSWORD" -H "Destination: $TARGET_URL/reader3/webdav/$ROOT_NAME/move-target.txt" -H 'Overwrite: T')"
assert_status "$status" 201 "MOVE file over old directory"
status="$(request MKCOL "$TARGET_URL/webdav/$ROOT_NAME/move-destination" --user "$USERNAME:$PASSWORD")"
assert_status "$status" 201 "prepare cross-parent MOVE"
status="$(request MOVE "$TARGET_URL/reader3/webdav/$ROOT_NAME/move-target.txt" \
  --user "$USERNAME:$PASSWORD" -H "Destination: $TARGET_URL/webdav/$ROOT_NAME/move-destination/final.txt")"
assert_status "$status" 201 "MOVE across different opened parents"
status="$(request GET "$TARGET_URL/webdav/$ROOT_NAME/move-destination/final.txt" --user "$USERNAME:$PASSWORD")"
assert_status "$status" 200 "GET cross-parent moved file"
grep -Fx 'final move bytes' "$BODY" >/dev/null

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
