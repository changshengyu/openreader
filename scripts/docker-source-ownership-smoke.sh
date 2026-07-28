#!/usr/bin/env sh
set -eu

IMAGE="${IMAGE:-ghcr.io/changshengyu/openreader:latest}"
PORT="${PORT:-18120}"
LEGACY_PORT="${LEGACY_PORT:-$((PORT + 1))}"
PASSWORD="password123"
FRESH_ADMIN="sourceadmin$$"
FRESH_USER="sourceuser$$"
FRESH_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/openreader-source-fresh.XXXXXX")"
LEGACY_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/openreader-source-legacy.XXXXXX")"
FRESH_NAME="openreader-source-fresh-$(basename "$FRESH_ROOT")"
LEGACY_NAME="openreader-source-legacy-$(basename "$LEGACY_ROOT")"
FRESH_URL="http://127.0.0.1:${PORT}"
LEGACY_URL="http://127.0.0.1:${LEGACY_PORT}"

cleanup() {
  docker stop "$FRESH_NAME" >/dev/null 2>&1 || true
  docker stop "$LEGACY_NAME" >/dev/null 2>&1 || true
  if [ "${KEEP_OPENREADER_SOURCE_SMOKE:-0}" = "1" ]; then
    echo "kept fresh source root: $FRESH_ROOT"
    echo "kept legacy source root: $LEGACY_ROOT"
  else
    rm -rf "$FRESH_ROOT" "$LEGACY_ROOT"
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
need go
need python3

json_field() {
  python3 -c 'import json,sys; print(json.load(sys.stdin)[sys.argv[1]])' "$1"
}

login() {
  base_url="$1"
  username="$2"
  password="$3"
  curl -fsS -X POST "${base_url}/api/auth/login" \
    -H 'Content-Type: application/json' \
    -d "{\"username\":\"${username}\",\"password\":\"${password}\"}" |
    json_field token
}

register() {
  base_url="$1"
  username="$2"
  curl -fsS -X POST "${base_url}/api/auth/register" \
    -H 'Content-Type: application/json' \
    -d "{\"username\":\"${username}\",\"password\":\"${PASSWORD}\"}"
}

start_container() {
  name="$1"
  port="$2"
  root="$3"
  docker run -d --rm \
    --name "$name" \
    -p "127.0.0.1:${port}:8080" \
    -e OPENREADER_ADDR=":8080" \
    -e OPENREADER_JWT_SECRET="openreader-source-smoke-secret" \
    -e OPENREADER_DATA_DIR="/app/data" \
    -e OPENREADER_CACHE_DIR="/app/cache" \
    -e OPENREADER_LIBRARY_DIR="/app/library" \
    -v "$root/data:/app/data" \
    -v "$root/cache:/app/cache" \
    -v "$root/library:/app/library" \
    -v "$root/retired-host:/retired-host:ro" \
    "$IMAGE" >/dev/null
}

wait_health() {
  name="$1"
  base_url="$2"
  attempt=0
  while [ "$attempt" -lt 60 ]; do
    if curl -fsS "${base_url}/api/health" >/dev/null 2>&1; then
      return 0
    fi
    attempt=$((attempt + 1))
    sleep 1
  done
  echo "container did not become healthy: $name" >&2
  docker logs "$name" >&2 || true
  exit 1
}

stop_container() {
  name="$1"
  docker stop "$name" >/dev/null
  attempt=0
  while docker inspect "$name" >/dev/null 2>&1; do
    if [ "$attempt" -ge 30 ]; then
      echo "container was stopped but not removed: $name" >&2
      exit 1
    fi
    attempt=$((attempt + 1))
    sleep 1
  done
}

source_list() {
  base_url="$1"
  token="$2"
  curl -fsS "${base_url}/api/sources" -H "Authorization: Bearer ${token}"
}

assert_source_list() {
  expected="$1"
  forbidden="${2:-}"
  python3 -c '
import json, sys
rows = json.load(sys.stdin)
actual = sorted(str(row.get("name", "")) for row in rows)
expected = sorted(item for item in sys.argv[1].split("|") if item)
forbidden = [item for item in sys.argv[2].split("|") if item]
if actual != expected:
    raise SystemExit(f"source names={actual}, want={expected}")
for name in forbidden:
    if name in actual:
        raise SystemExit(f"forbidden source leaked: {name}")
' "$expected" "$forbidden"
}

source_id() {
  name="$1"
  python3 -c '
import json, sys
name = sys.argv[1]
for row in json.load(sys.stdin):
    if row.get("name") == name:
        print(row["id"])
        break
else:
    raise SystemExit(f"source not found: {name}")
' "$name"
}

renamed_source_payload() {
  old_name="$1"
  new_name="$2"
  python3 -c '
import json, sys
old_name, new_name = sys.argv[1], sys.argv[2]
for row in json.load(sys.stdin):
    if row.get("name") == old_name:
        row["name"] = new_name
        print(json.dumps(row, ensure_ascii=False, separators=(",", ":")))
        break
else:
    raise SystemExit(f"source not found: {old_name}")
' "$old_name" "$new_name"
}

update_source() {
  base_url="$1"
  token="$2"
  source_id="$3"
  payload="$4"
  curl -fsS -X PUT "${base_url}/api/sources/${source_id}" \
    -H "Authorization: Bearer ${token}" \
    -H 'Content-Type: application/json' \
    -d "$payload"
}

create_source() {
  base_url="$1"
  token="$2"
  name="$3"
  source_url="$4"
  curl -fsS -X POST "${base_url}/api/sources" \
    -H "Authorization: Bearer ${token}" \
    -H 'Content-Type: application/json' \
    -d "{\"bookSourceName\":\"${name}\",\"bookSourceUrl\":\"${source_url}\",\"enabled\":true}"
}

book_source_id() {
  title="$1"
  python3 -c '
import json, sys
title = sys.argv[1]
for row in json.load(sys.stdin):
    if row.get("title") == title:
        print(row.get("sourceId", 0))
        break
else:
    raise SystemExit(f"book not found: {title}")
' "$title"
}

trigger_backup() {
  base_url="$1"
  token="$2"
  curl -fsS -X POST "${base_url}/api/backup/trigger" \
    -H "Authorization: Bearer ${token}" |
    json_field name
}

trigger_portable() {
  base_url="$1"
  token="$2"
  curl -fsS -X POST "${base_url}/api/backup/portable/trigger" \
    -H "Authorization: Bearer ${token}" |
    json_field name
}

restore_backup() {
  base_url="$1"
  token="$2"
  path="$3"
  curl -fsS -X POST "${base_url}/api/backup/restore-legado" \
    -H "Authorization: Bearer ${token}" \
    -F "file=@${path}" >/dev/null
}

assert_zip_sources() {
  path="$1"
  expected="$2"
  forbidden="${3:-}"
  python3 -c '
import json, sys, zipfile
path, expected_raw, forbidden_raw = sys.argv[1:4]
with zipfile.ZipFile(path) as archive:
    try:
        rows = json.loads(archive.read("bookSource.json"))
    except KeyError:
        raise SystemExit(f"{path}: bookSource.json is missing")
actual = sorted(str(row.get("bookSourceName") or row.get("name") or "") for row in rows)
expected = sorted(item for item in expected_raw.split("|") if item)
forbidden = [item for item in forbidden_raw.split("|") if item]
if actual != expected:
    raise SystemExit(f"{path}: source names={actual}, want={expected}")
for name in forbidden:
    if name in actual:
        raise SystemExit(f"{path}: forbidden source leaked: {name}")
' "$path" "$expected" "$forbidden"
}

assert_legacy_pre_migration() {
  python3 -c '
import sqlite3, sys
db = sqlite3.connect(sys.argv[1])
tables = {row[0] for row in db.execute("select name from sqlite_master where type = ?", ("table",))}
errors = []
if db.execute("select count(*) from book_sources").fetchone()[0] != 2:
    errors.append("legacy global source count is not 2")
if db.execute("select count(*) from books where source_id > 0").fetchone()[0] != 2:
    errors.append("legacy remote-book reference count is not 2")
if db.execute("select count(*) from source_failures").fetchone()[0] != 2:
    errors.append("legacy source-failure count is not 2")
for forbidden in ("user_book_sources", "book_source_namespaces", "schema_migrations"):
    if forbidden in tables:
        errors.append(f"legacy fixture unexpectedly contains {forbidden}")
if errors:
    raise SystemExit("; ".join(errors))
' "$1"
}

assert_legacy_migration() {
  python3 -c '
import sqlite3, sys
db = sqlite3.connect(sys.argv[1])
users = {name: uid for uid, name in db.execute("select id, username from users")}
sources = {name: sid for sid, name in db.execute("select id, name from book_sources")}
source_ids = sorted(sources.values())
for username in ("legacy_owner", "legacy_other"):
    actual = [row[0] for row in db.execute(
        "select source_id from user_book_sources where user_id = ? and detached = 0 order by source_id",
        (users[username],),
    )]
    if actual != source_ids:
        raise SystemExit(f"{username} migrated associations={actual}, want={source_ids}")
default_ids = [row[0] for row in db.execute(
    "select source_id from user_book_sources where user_id = 0 and detached = 0 order by source_id"
)]
if default_ids != source_ids:
    raise SystemExit(f"default migrated associations={default_ids}, want={source_ids}")
for username in ("legacy_owner", "legacy_other"):
    count = db.execute("select count(*) from book_source_namespaces where user_id = ?", (users[username],)).fetchone()[0]
    if count != 1:
        raise SystemExit(f"{username} namespace count={count}, want=1")
if db.execute("select count(*) from book_source_namespaces where user_id = 0").fetchone()[0] != 1:
    raise SystemExit("default namespace missing after migration")
if db.execute(
    "select count(*) from schema_migrations where key = ?",
    ("book-source-ownership-v1",),
).fetchone()[0] != 1:
    raise SystemExit("ownership migration marker missing")
source_a = sources["旧全局书源 A"]
source_b = sources["旧全局书源 B"]
for title in ("旧卷 用户A远程书", "旧卷 用户B远程书"):
    value = db.execute("select source_id from books where title = ?", (title,)).fetchone()[0]
    if value != source_a:
        raise SystemExit(f"{title} source_id={value}, want={source_a}")
for username in ("legacy_owner", "legacy_other"):
    value = db.execute("select source_id from source_failures where user_id = ?", (users[username],)).fetchone()[0]
    if value != source_b:
        raise SystemExit(f"{username} failure source_id={value}, want={source_b}")
' "$1"
}

assert_fresh_database() {
  database="$1"
  admin_name="$2"
  user_name="$3"
  python3 -c '
import sqlite3, sys
db = sqlite3.connect(sys.argv[1])
users = {name: uid for uid, name in db.execute("select id, username from users")}
def active_names(user_id):
    return sorted(row[0] for row in db.execute(
        "select s.name from user_book_sources u join book_sources s on s.id = u.source_id "
        "where u.user_id = ? and u.detached = 0",
        (user_id,),
    ))
expected = {
    users[sys.argv[2]]: ["Docker 管理员源"],
    users[sys.argv[3]]: ["Docker 普通用户源"],
    0: ["Docker 管理员源"],
}
for user_id, names in expected.items():
    actual = active_names(user_id)
    if actual != names:
        raise SystemExit(f"fresh user {user_id} sources={actual}, want={names}")
' "$database" "$admin_name" "$user_name"
}

assert_legacy_final_database() {
  python3 -c '
import sqlite3, sys
db = sqlite3.connect(sys.argv[1])
users = {name: uid for uid, name in db.execute("select id, username from users")}
sources = {name: sid for sid, name in db.execute("select id, name from book_sources")}
expected_names = {
    users["legacy_owner"]: ["旧全局书源 B", "旧卷 用户A重启专属源"],
    users["legacy_other"]: ["旧全局书源 A", "旧全局书源 B"],
    0: ["旧全局书源 A", "旧全局书源 B"],
}
for user_id, expected in expected_names.items():
    actual = sorted(row[0] for row in db.execute(
        "select s.name from user_book_sources u join book_sources s on s.id = u.source_id "
        "where u.user_id = ? and u.detached = 0",
        (user_id,),
    ))
    if actual != sorted(expected):
        raise SystemExit(f"legacy user {user_id} sources={actual}, want={sorted(expected)}")
owner_book_source = db.execute(
    "select source_id from books where title = ?", ("旧卷 用户A远程书",)
).fetchone()[0]
other_book_source = db.execute(
    "select source_id from books where title = ?", ("旧卷 用户B远程书",)
).fetchone()[0]
if owner_book_source != sources["旧卷 用户A重启专属源"]:
    raise SystemExit("owner remote book did not retain the COW source")
if other_book_source != sources["旧全局书源 A"]:
    raise SystemExit("other remote book was remapped by owner COW")
owner_failure_count = db.execute(
    "select count(*) from source_failures where user_id = ?",
    (users["legacy_owner"],),
).fetchone()[0]
other_source_b = db.execute(
    "select s.id from user_book_sources u join book_sources s on s.id = u.source_id "
    "where u.user_id = ? and u.detached = 0 and s.name = ?",
    (users["legacy_other"], "旧全局书源 B"),
).fetchone()[0]
other_failure_ids = [row[0] for row in db.execute(
    "select source_id from source_failures where user_id = ?",
    (users["legacy_other"],),
)]
if owner_failure_count != 0:
    raise SystemExit(f"restored owner retained {owner_failure_count} derived source failures")
if other_failure_ids != [other_source_b]:
    raise SystemExit(f"owner restore changed the other user failures: {other_failure_ids}")
if db.execute(
    "select count(*) from schema_migrations where key = ?",
    ("book-source-ownership-v1",),
).fetchone()[0] != 1:
    raise SystemExit("ownership migration marker is not durable")
' "$1"
}

mkdir -p \
  "$FRESH_ROOT/data" "$FRESH_ROOT/cache" "$FRESH_ROOT/library" "$FRESH_ROOT/retired-host" \
  "$LEGACY_ROOT/data" "$LEGACY_ROOT/cache" "$LEGACY_ROOT/library" "$LEGACY_ROOT/retired-host"

start_container "$FRESH_NAME" "$PORT" "$FRESH_ROOT"
wait_health "$FRESH_NAME" "$FRESH_URL"

FRESH_ADMIN_RESPONSE="$(register "$FRESH_URL" "$FRESH_ADMIN")"
FRESH_ADMIN_TOKEN="$(printf '%s' "$FRESH_ADMIN_RESPONSE" | json_field token)"
FRESH_USER_RESPONSE="$(register "$FRESH_URL" "$FRESH_USER")"
FRESH_USER_TOKEN="$(printf '%s' "$FRESH_USER_RESPONSE" | json_field token)"

create_source "$FRESH_URL" "$FRESH_ADMIN_TOKEN" \
  "Docker 管理员源" "https://docker-admin-source.invalid" >/dev/null
create_source "$FRESH_URL" "$FRESH_USER_TOKEN" \
  "Docker 普通用户源" "https://docker-user-source.invalid" >/dev/null
curl -fsS -X POST "${FRESH_URL}/api/sources/default/save" \
  -H "Authorization: Bearer ${FRESH_ADMIN_TOKEN}" >/dev/null

printf '%s' "$(source_list "$FRESH_URL" "$FRESH_ADMIN_TOKEN")" |
  assert_source_list "Docker 管理员源" "Docker 普通用户源"
printf '%s' "$(source_list "$FRESH_URL" "$FRESH_USER_TOKEN")" |
  assert_source_list "Docker 普通用户源" "Docker 管理员源"

FRESH_ADMIN_BACKUP="$(trigger_backup "$FRESH_URL" "$FRESH_ADMIN_TOKEN")"
FRESH_USER_BACKUP="$(trigger_backup "$FRESH_URL" "$FRESH_USER_TOKEN")"
FRESH_ADMIN_BACKUP_PATH="$FRESH_ROOT/data/webdav/$FRESH_ADMIN_BACKUP"
FRESH_USER_BACKUP_PATH="$FRESH_ROOT/data/webdav/users/$FRESH_USER/$FRESH_USER_BACKUP"
[ -f "$FRESH_ADMIN_BACKUP_PATH" ] || {
  echo "administrator logical backup was not written to the legacy root" >&2
  exit 1
}
[ -f "$FRESH_USER_BACKUP_PATH" ] || {
  echo "regular-user logical backup was not written to the private root" >&2
  exit 1
}
assert_zip_sources "$FRESH_ADMIN_BACKUP_PATH" "Docker 管理员源" "Docker 普通用户源"
assert_zip_sources "$FRESH_USER_BACKUP_PATH" "Docker 普通用户源" "Docker 管理员源"

FRESH_ADMIN_PORTABLE="$(trigger_portable "$FRESH_URL" "$FRESH_ADMIN_TOKEN")"
FRESH_USER_PORTABLE="$(trigger_portable "$FRESH_URL" "$FRESH_USER_TOKEN")"
assert_zip_sources "$FRESH_ROOT/data/webdav/$FRESH_ADMIN_PORTABLE" \
  "Docker 管理员源" "Docker 普通用户源"
assert_zip_sources "$FRESH_ROOT/data/webdav/users/$FRESH_USER/$FRESH_USER_PORTABLE" \
  "Docker 普通用户源" "Docker 管理员源"

FRESH_ADMIN_SOURCES="$(source_list "$FRESH_URL" "$FRESH_ADMIN_TOKEN")"
FRESH_ADMIN_SOURCE_ID="$(printf '%s' "$FRESH_ADMIN_SOURCES" | source_id "Docker 管理员源")"
FRESH_ADMIN_RENAME_PAYLOAD="$(printf '%s' "$FRESH_ADMIN_SOURCES" |
  renamed_source_payload "Docker 管理员源" "Docker 管理员临时改名")"
update_source "$FRESH_URL" "$FRESH_ADMIN_TOKEN" "$FRESH_ADMIN_SOURCE_ID" \
  "$FRESH_ADMIN_RENAME_PAYLOAD" >/dev/null
restore_backup "$FRESH_URL" "$FRESH_ADMIN_TOKEN" "$FRESH_ADMIN_BACKUP_PATH"
printf '%s' "$(source_list "$FRESH_URL" "$FRESH_ADMIN_TOKEN")" |
  assert_source_list "Docker 管理员源" "Docker 管理员临时改名|Docker 普通用户源"
printf '%s' "$(source_list "$FRESH_URL" "$FRESH_USER_TOKEN")" |
  assert_source_list "Docker 普通用户源" "Docker 管理员源|Docker 管理员临时改名"

stop_container "$FRESH_NAME"
start_container "$FRESH_NAME" "$PORT" "$FRESH_ROOT"
wait_health "$FRESH_NAME" "$FRESH_URL"
FRESH_ADMIN_TOKEN="$(login "$FRESH_URL" "$FRESH_ADMIN" "$PASSWORD")"
FRESH_USER_TOKEN="$(login "$FRESH_URL" "$FRESH_USER" "$PASSWORD")"
printf '%s' "$(source_list "$FRESH_URL" "$FRESH_ADMIN_TOKEN")" |
  assert_source_list "Docker 管理员源" "Docker 普通用户源"
printf '%s' "$(source_list "$FRESH_URL" "$FRESH_USER_TOKEN")" |
  assert_source_list "Docker 普通用户源" "Docker 管理员源"
stop_container "$FRESH_NAME"
assert_fresh_database "$FRESH_ROOT/data/openreader.db" "$FRESH_ADMIN" "$FRESH_USER"

(
  cd backend
  GOCACHE="${GOCACHE:-$PWD/.gocache}" \
    go run ./cmd/create-old-volume-fixture -root "$LEGACY_ROOT"
)
assert_legacy_pre_migration "$LEGACY_ROOT/data/openreader.db"

start_container "$LEGACY_NAME" "$LEGACY_PORT" "$LEGACY_ROOT"
wait_health "$LEGACY_NAME" "$LEGACY_URL"
stop_container "$LEGACY_NAME"
assert_legacy_migration "$LEGACY_ROOT/data/openreader.db"

start_container "$LEGACY_NAME" "$LEGACY_PORT" "$LEGACY_ROOT"
wait_health "$LEGACY_NAME" "$LEGACY_URL"
LEGACY_OWNER_TOKEN="$(login "$LEGACY_URL" "legacy_owner" "legacy-volume-secret")"
LEGACY_OTHER_TOKEN="$(login "$LEGACY_URL" "legacy_other" "legacy-other-volume-secret")"

LEGACY_OWNER_SOURCES="$(source_list "$LEGACY_URL" "$LEGACY_OWNER_TOKEN")"
LEGACY_OTHER_SOURCES="$(source_list "$LEGACY_URL" "$LEGACY_OTHER_TOKEN")"
printf '%s' "$LEGACY_OWNER_SOURCES" |
  assert_source_list "旧全局书源 A|旧全局书源 B"
printf '%s' "$LEGACY_OTHER_SOURCES" |
  assert_source_list "旧全局书源 A|旧全局书源 B"
LEGACY_SHARED_SOURCE_ID="$(printf '%s' "$LEGACY_OWNER_SOURCES" | source_id "旧全局书源 A")"

LEGACY_PRE_BACKUP="$(trigger_backup "$LEGACY_URL" "$LEGACY_OWNER_TOKEN")"
LEGACY_PRE_BACKUP_PATH="$LEGACY_ROOT/data/webdav/users/legacy_owner/$LEGACY_PRE_BACKUP"
assert_zip_sources "$LEGACY_PRE_BACKUP_PATH" "旧全局书源 A|旧全局书源 B"

LEGACY_COW_PAYLOAD="$(printf '%s' "$LEGACY_OWNER_SOURCES" |
  renamed_source_payload "旧全局书源 A" "旧卷 用户A专属源")"
LEGACY_COW_RESPONSE="$(update_source "$LEGACY_URL" "$LEGACY_OWNER_TOKEN" \
  "$LEGACY_SHARED_SOURCE_ID" "$LEGACY_COW_PAYLOAD")"
LEGACY_COW_SOURCE_ID="$(printf '%s' "$LEGACY_COW_RESPONSE" | json_field id)"
if [ "$LEGACY_COW_SOURCE_ID" = "$LEGACY_SHARED_SOURCE_ID" ]; then
  echo "legacy shared source update did not use copy-on-write" >&2
  exit 1
fi
printf '%s' "$(source_list "$LEGACY_URL" "$LEGACY_OWNER_TOKEN")" |
  assert_source_list "旧卷 用户A专属源|旧全局书源 B" "旧全局书源 A"
printf '%s' "$(source_list "$LEGACY_URL" "$LEGACY_OTHER_TOKEN")" |
  assert_source_list "旧全局书源 A|旧全局书源 B" "旧卷 用户A专属源"

OWNER_REMOTE_SOURCE_ID="$(curl -fsS "${LEGACY_URL}/api/books" \
  -H "Authorization: Bearer ${LEGACY_OWNER_TOKEN}" |
  book_source_id "旧卷 用户A远程书")"
OTHER_REMOTE_SOURCE_ID="$(curl -fsS "${LEGACY_URL}/api/books" \
  -H "Authorization: Bearer ${LEGACY_OTHER_TOKEN}" |
  book_source_id "旧卷 用户B远程书")"
if [ "$OWNER_REMOTE_SOURCE_ID" != "$LEGACY_COW_SOURCE_ID" ] ||
   [ "$OTHER_REMOTE_SOURCE_ID" != "$LEGACY_SHARED_SOURCE_ID" ]; then
  echo "copy-on-write did not isolate legacy remote-book source IDs" >&2
  exit 1
fi

LEGACY_OWNER_BACKUP="$(trigger_backup "$LEGACY_URL" "$LEGACY_OWNER_TOKEN")"
LEGACY_OTHER_BACKUP="$(trigger_backup "$LEGACY_URL" "$LEGACY_OTHER_TOKEN")"
LEGACY_OWNER_BACKUP_PATH="$LEGACY_ROOT/data/webdav/users/legacy_owner/$LEGACY_OWNER_BACKUP"
LEGACY_OTHER_BACKUP_PATH="$LEGACY_ROOT/data/webdav/users/legacy_other/$LEGACY_OTHER_BACKUP"
assert_zip_sources "$LEGACY_OWNER_BACKUP_PATH" \
  "旧卷 用户A专属源|旧全局书源 B" "旧全局书源 A"
assert_zip_sources "$LEGACY_OTHER_BACKUP_PATH" \
  "旧全局书源 A|旧全局书源 B" "旧卷 用户A专属源"

LEGACY_OWNER_PORTABLE="$(trigger_portable "$LEGACY_URL" "$LEGACY_OWNER_TOKEN")"
LEGACY_OTHER_PORTABLE="$(trigger_portable "$LEGACY_URL" "$LEGACY_OTHER_TOKEN")"
assert_zip_sources "$LEGACY_ROOT/data/webdav/users/legacy_owner/$LEGACY_OWNER_PORTABLE" \
  "旧卷 用户A专属源|旧全局书源 B" "旧全局书源 A"
assert_zip_sources "$LEGACY_ROOT/data/webdav/users/legacy_other/$LEGACY_OTHER_PORTABLE" \
  "旧全局书源 A|旧全局书源 B" "旧卷 用户A专属源"

restore_backup "$LEGACY_URL" "$LEGACY_OWNER_TOKEN" "$LEGACY_PRE_BACKUP_PATH"
printf '%s' "$(source_list "$LEGACY_URL" "$LEGACY_OWNER_TOKEN")" |
  assert_source_list "旧全局书源 A|旧全局书源 B" "旧卷 用户A专属源"
printf '%s' "$(source_list "$LEGACY_URL" "$LEGACY_OTHER_TOKEN")" |
  assert_source_list "旧全局书源 A|旧全局书源 B" "旧卷 用户A专属源"

LEGACY_OWNER_SOURCES="$(source_list "$LEGACY_URL" "$LEGACY_OWNER_TOKEN")"
LEGACY_RESTORED_SOURCE_ID="$(printf '%s' "$LEGACY_OWNER_SOURCES" | source_id "旧全局书源 A")"
LEGACY_RESTART_PAYLOAD="$(printf '%s' "$LEGACY_OWNER_SOURCES" |
  renamed_source_payload "旧全局书源 A" "旧卷 用户A重启专属源")"
LEGACY_RESTART_RESPONSE="$(update_source "$LEGACY_URL" "$LEGACY_OWNER_TOKEN" \
  "$LEGACY_RESTORED_SOURCE_ID" "$LEGACY_RESTART_PAYLOAD")"
LEGACY_RESTART_SOURCE_ID="$(printf '%s' "$LEGACY_RESTART_RESPONSE" | json_field id)"

stop_container "$LEGACY_NAME"
start_container "$LEGACY_NAME" "$LEGACY_PORT" "$LEGACY_ROOT"
wait_health "$LEGACY_NAME" "$LEGACY_URL"
LEGACY_OWNER_TOKEN="$(login "$LEGACY_URL" "legacy_owner" "legacy-volume-secret")"
LEGACY_OTHER_TOKEN="$(login "$LEGACY_URL" "legacy_other" "legacy-other-volume-secret")"
printf '%s' "$(source_list "$LEGACY_URL" "$LEGACY_OWNER_TOKEN")" |
  assert_source_list "旧卷 用户A重启专属源|旧全局书源 B" "旧全局书源 A"
printf '%s' "$(source_list "$LEGACY_URL" "$LEGACY_OTHER_TOKEN")" |
  assert_source_list "旧全局书源 A|旧全局书源 B" "旧卷 用户A重启专属源"

OWNER_REMOTE_SOURCE_ID="$(curl -fsS "${LEGACY_URL}/api/books" \
  -H "Authorization: Bearer ${LEGACY_OWNER_TOKEN}" |
  book_source_id "旧卷 用户A远程书")"
OTHER_REMOTE_SOURCE_ID="$(curl -fsS "${LEGACY_URL}/api/books" \
  -H "Authorization: Bearer ${LEGACY_OTHER_TOKEN}" |
  book_source_id "旧卷 用户B远程书")"
if [ "$OWNER_REMOTE_SOURCE_ID" != "$LEGACY_RESTART_SOURCE_ID" ] ||
   [ "$OTHER_REMOTE_SOURCE_ID" = "$LEGACY_RESTART_SOURCE_ID" ]; then
  echo "legacy source ownership did not survive restart" >&2
  exit 1
fi
stop_container "$LEGACY_NAME"
assert_legacy_final_database "$LEGACY_ROOT/data/openreader.db"

echo "OpenReader Docker book-source ownership smoke passed for ${IMAGE} (legacy migration, COW, admin/private roots, logical/portable, restore, restart)"
