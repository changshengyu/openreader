# OpenReader Compatibility API Contract

Status: working contract. Keep this file updated when endpoint semantics change.

## Global rules

- Public API root: `/api`.
- Auth: `Authorization: Bearer <jwt>` for protected `/api` endpoints.
- WebDAV roots: `/webdav` and upstream-compatible `/reader3/webdav`.
- The upstream WebDAV compatibility root is implemented and shares the same caller-scoped storage;
  see [`webdav-protocol-p2-contract.md`](webdav-protocol-p2-contract.md)).
- Sync WebSocket: `/ws/sync`.

The sync route is specified by [`websocket-sync-p2-contract.md`](websocket-sync-p2-contract.md):
`GET /ws/sync?token=<jwt>` is a same-origin, active-user-authenticated, server-to-client-only transport. Client
application messages are not a second write API and must be closed without relay. Owner events remain user-scoped;
`users_update` is limited to administrators plus each affected user with a self-only ID projection. The route,
server event names and existing business payloads remain stable.
- Expected error shape for handled failures: JSON object with `error`.
- User-owned resources must be scoped to the authenticated user unless documented as admin/global.
- Concurrent first writes to the same authenticated `settings/:key` use the existing `(user_id,key)` unique key as
  an atomic upsert. They retain validation, stale-base conflict behavior, response shape and scoped sync events;
  a normal same-user startup race must never expose a UNIQUE error as `500`.
- Concurrent authenticated startup reads such as `GET /api/sources` and `GET /api/explore/sources` may both
  initialize the caller's book-source namespace. Every pooled SQLite connection must therefore inherit WAL and
  a 5000 ms busy timeout, while all in-process book-source service instances serialize the rare uninitialized
  namespace transaction. A bounded retry handles SQLite's immediate `BUSY/LOCKED` result when that read transaction
  must upgrade behind an unrelated writer; ordinary initialization contention must retain the existing successful
  response schemas instead of exposing `database is locked` as `500`.
- `PUT /api/settings/:key` may receive additive `force:true` only for the confirmed “备份用户配置” action.
  It bypasses stale-base comparison for that authenticated user's legal `reader/shelf/search` row only; ordinary
  background writes keep CAS. Explicit restore never creates a missing row as a side effect of reading it.

## Public endpoints

| Method | Path | Purpose | Compatibility notes |
|---|---|---|---|
| `GET` | `/api/health` | Health and build metadata. | OpenReader runtime addition; keep stable for Docker/probes. |
| `GET`, `HEAD` | `/api/cover/:capability` | Serve a server-projected remote book cover through a same-origin, short-lived resource capability. | Implemented and published in `ceb4baa` on 2026-07-27. The path never accepts a raw URL or login JWT. Successful responses are bounded, type-verified and privately cacheable; malformed/tampered capabilities are `403`, unavailable/unsafe remote images are `404`. See [`book-cover-proxy-p2-contract.md`](book-cover-proxy-p2-contract.md). |
| `POST` | `/api/auth/register` | Create user; first user becomes admin. | OpenReader multi-user addition. One JSON body is limited to 16 KiB; overflow is `413`. New passwords are 8 characters and at most 72 bytes for bcrypt; oversize is `400`. |
| `POST` | `/api/auth/login` | Return JWT and user object. | OpenReader auth addition; one JSON body is limited to 16 KiB and overflow is `413`. Invalid credentials remain generic `401`, and legacy usernames are not revalidated as new registrations. |

The complete auth wire/error/no-side-effect contract is
[`auth-request-boundary-fixed-baseline-second-audit-p2-contract.md`](auth-request-boundary-fixed-baseline-second-audit-p2-contract.md).
It is `aligned / regression-validated / Docker-pending`: focused contract/race tests, the full Go/frontend gates and
a real declared/chunked HTTP smoke prove the current handlers enforce it. Mounted-volume gates and publication remain.

## Protected endpoint groups

| Group | Representative paths | Contract notes |
|---|---|---|
| User/settings/admin | `/api/me`, `/api/settings/:key`, `/api/admin/users` | Settings are per user. Admin endpoints require admin role. |
| Sources | `/api/sources`, `/api/sources/import`, `/api/sources/:id/test*`, `/api/sources/:id/debug/stream` | Preserve reader3-compatible source fields and parser semantics. The three deployed test endpoints keep their authenticated `200` response fields, while the canonical debugger uses one cancellable stream for search/explore → BookInfo → TOC → first content and carries the BookInfo/TOC runtime into content. Debugging has no `source_failures` side effect; only the separate explicit health-check action may record diagnostic failures. |
| Bookshelf | `/api/books`, `/api/books/:id`, `/api/books/batch`, `/api/books/export` | Book operations must not cross user boundaries. `GET /api/books` is the current user's authoritative mutable shelf snapshot: the frontend requests it network-first and may use its scoped persistent copy only after a network failure. |
| Reader content | `/api/books/:id/chapters`, `/api/books/:id/chapters/:index/content` | Content fetch uses a valid cache first and returns stable chapter data. On a remote cache miss, one single `nextContentUrl` that resolves to the adjacent catalog chapter is a chapter boundary, not continuation content. A blank text `contentRule` remains the existing client-safe `502` response but must not cache page HTML or create a `source_failures` row; audio sources retain their approved blank-rule media-URL behavior. |
| Reader legacy search | `/api/reader3/searchBookContent` | Compatibility endpoint; keep until old clients/routes no longer need it. |
| Progress | `/api/progress/:bookID`, `/api/progress` | Progress writes must be conflict-safe and user scoped. |
| Bookmarks | `/api/books/:id/bookmarks`, `/api/bookmarks/:id` | Bookmark CRUD and batch operations remain user/book scoped. |
| Local store | `/api/local-store*` | All paths must stay rooted under configured local store/library paths. |
| Import | `/api/imports/books/preview`, `/api/imports/books`, `/api/imports/txt` | Preview may return `importToken`; import must be able to reuse staged content. |
| Uploads | `/api/uploads` | Uploaded assets are content-validated before final write, rooted under data uploads and user-scoped for new writes/deletes; legacy global upload URLs remain readable. BookInfo ownership is in [`bookinfo-shelf-mutations-p2-contract.md`](bookinfo-shelf-mutations-p2-contract.md); implemented Reader upload/save/delete ordering, signature/dimension admission and the separate pending P2-B backup boundary are in [`reader-appearance-assets-p2-contract.md`](reader-appearance-assets-p2-contract.md). |
| Cache | `/api/cache/stats`, `/api/cache`, `/api/books/:id/cache` | Cache operations must not delete unrelated user data. |
| Replace rules | `/api/replace-rules*` | See the P2 replace-rule contract below: stable name-upsert order and upstream-visible plain/regex/scope semantics. |
| RSS | `/api/rss/sources`, `/api/rss/sources/import`, `/api/rss/sources/:id/refresh`, `/api/rss/articles` | Source writes are current-user scoped. The visible article flow fetches exactly one requested remote page; remote fetch limits and parser safety apply. See the P2 RSS page contract below. |
| Explore | `/api/explore/sources`, `/api/explore/:sourceId` | Browse source catalogs with bounded pagination/fetch behavior. |
| Backup/WebDAV import | `/api/backup/*`, `/api/webdav/import-*` | Backup/restore must preserve existing data and report clear compatibility failures. |

UserManage 权限部分更新的第二轮固定基准见
[`user-management-partial-update-second-audit-p2-contract.md`](user-management-partial-update-second-audit-p2-contract.md)。
`PUT /api/admin/users/:id` 必须只更新请求中显式存在的权限/限额列；禁止把读取到的完整 User 快照
`Save` 回数据库，从而覆盖并发登录时间或密码重置。前端每个 switch 同样只能拥有自己的单字段 payload。
该合同已于 2026-08-09 实施并通过 focused/race/full Go、frontend 707/707、build 和四视口浏览器验证；
本机发布的 `77a60d8` 还通过 fresh/historical volume 门，OCI index 为
`sha256:a1a37b223e10a3c43febd23250dd7790394c200d69e7c9548255cf1fdba3b017`。

## P2 RSS source import and requested-page contract

Status: implemented, regression-validated and Docker-published on 2026-08-09.
The complete visible, state and parser contract is
[`rss-visible-workspace-fixed-baseline-second-audit-p2-contract.md`](rss-visible-workspace-fixed-baseline-second-audit-p2-contract.md).

All routes below require `Authorization: Bearer <jwt>` and scope every source,
article, cache write and sync event to the authenticated user.

| Method / path | Request | Success / side effects | Errors |
|---|---|---|---|
| `POST /api/rss/sources/import` | JSON array containing only the records selected in the import dialog. Current `title/url` and upstream `sourceName/sourceUrl` aliases are accepted. Missing `singleUrl` uses the upstream import default `false`. | `200 {"created":N,"updated":N,"skipped":N}`. In one SQLite transaction, trim only identity fields, skip blank name/URL, replace same-user same-URL rows in place, create new rows in input order, and emit one source sync event only after commit. Another user's same URL is unrelated. | `400` malformed/non-array/over-limit payload; `500` rollback with no partial source changes. |
| `POST /api/rss/sources/:id/refresh?page=N&sortName=...&sortUrl=...` | `page` is a positive bounded integer, default `1`. `sortUrl` must resolve through the owned source's allowed sort/base semantics; it is not an arbitrary fetch capability. | `200 {"items":[...],"page":N,"hasMore":bool,"imported":N,"total":N,"sortUrl":"..."}`. Fetch exactly the requested remote page/transition, preserve parser order in `items`, user-scope and upsert those rows, then emit one post-commit article sync event. It must not prefetch later pages. | `400` invalid page, unsupported rule or bounded remote/parser failure; `404` missing/cross-user source; client-safe `{ "error": "..." }`. |
| `GET /api/rss/articles` | Existing optional `sourceId`, `sort`, `unread`, `favorite`, `page`, `limit`. | Existing user-scoped cached-list response remains for deployed/hidden clients. It is not the data source for the fixed-baseline visible RSS page list. | Existing stable `400/500` behavior; never exposes another user's rows. |
| `GET /api/rss/articles/:id/content` | Existing owned article ID. | `200` with sanitized content/link fields after the source-owned content action. No read/favourite mutation is implied by opening content. | `404` missing/cross-user article/source; `400` bounded fetch/parser failure. |

Standard RSS/Atom has one remote page: page 1 returns its items and
`hasMore=false`; page greater than 1 returns an empty successful page without
refetching the feed. Rule sources may return `hasMore=true` only when their
parsed next-page state supports the next requested transition. Timeout, response
size, redirect, scheme/host, SSRF, request-rate and parser-work limits apply to
every page. Response/error data must not reveal credentials, request headers,
private host paths or raw security diagnostics.

## P2 remote book-cover projection contract (implemented and published)

Reader-dev projects remote book covers through a same-origin cached `/reader3/cover?path=...`
resource and falls back to its bundled no-cover image. OpenReader keeps deployed REST paths and
does not copy the upstream endpoint's public arbitrary-URL SSRF surface.

Authenticated book/search/explore/source-candidate/temporary-reader responses retain the raw
`coverUrl` and may add `coverResourceUrl`. A missing field means legacy/non-remote fallback, a
non-empty value is a capability, and a present-empty value means the new server rejected the
remote URL and the browser must not fall back to it. The new field is response-only: SQLite, parser state,
source-change requests, sync persistence, exports, WebDAV and every backup format continue to
contain the original URL. The frontend displays
`customCoverUrl → coverResourceUrl → coverUrl → placeholder`.

`GET|HEAD /api/cover/:capability` is intentionally public for browser image loading, but accepts
only an opaque, purpose-separated, expiring server-issued capability. It applies HTTP(S)-only
SSRF/DNS/dial/redirect validation, a 3-second timeout, an 8-MiB body limit, image magic checking,
atomic per-user bounded cache writes and capability redaction in access logs. No source cookie,
Authorization header, raw URL/query or host path is returned or logged. Complete endpoint,
projection, cache, cleanup and failure semantics are fixed in
[`book-cover-proxy-p2-contract.md`](book-cover-proxy-p2-contract.md).

## P2 raw WebDAV protocol contract

Status: implemented and non-Docker validation complete on 2026-07-19. The complete compatibility, authentication,
filesystem and migration contract is
[`webdav-protocol-p2-contract.md`](webdav-protocol-p2-contract.md).

| Method / path | Request | Success / side effects | Auth and errors |
|---|---|---|---|
| `OPTIONS /webdav/*`, `/reader3/webdav/*` | No body. An unauthenticated capability probe is allowed. | `200`; advertises `DAV: 1,2`, `Allow` and `MS-Author-Via`. | A supplied invalid Authorization returns `401` with Basic challenge. No filesystem access. |
| `PROPFIND /webdav/*`, `/reader3/webdav/*` | Bearer JWT or Basic credentials; `Depth: 0/1` (infinity safely bounded to 1). | `207` DAV-namespace multistatus for the target and at most one child level; no physical path. | `401` missing/bad identity, `403` valid identity without WebDAV permission, `404` missing target. Read never creates a directory. |
| `GET/PUT/MKCOL/DELETE` on both roots | Bearer or Basic; PUT remains bounded by `OPENREADER_MAX_IMPORT_BYTES`. | Current user root only. Existing `/webdav` directory GET remains a deployed frontend listing adapter; upstream alias uses PROPFIND for listing. | Root/path/symlink checks precede I/O. Parent/type/missing errors use the focused contract statuses. |
| `MOVE/COPY` on both roots | Safe Destination within the same caller root; `Overwrite: T` is required to replace. | `201` after a complete validated rename/copy; recursive COPY does not follow symlinks. | `400` missing/invalid Destination, `403` unsafe/root operation, `409` missing parent, `412` missing source or forbidden overwrite. Failure preserves source and old target. |
| `LOCK/UNLOCK` on both roots | LOCK may include Timeout; UNLOCK requires Lock-Token. | Upstream-compatible stateless lock response: LOCK `200` plus random `urn:uuid:` token; UNLOCK `204`. | No lock table/file and no token logging; missing unlock token `400`. |

Both roots retain current caller-scoped storage: the administrator sees the historical WebDAV root and a
regular user sees only `data/webdav/users/<safe-username>/`. Basic validates the existing bcrypt password;
it is not a new stored credential and should only be exposed through HTTPS.

## P2 reading-progress API contract

Status: audited, implemented, full-regression validated and Docker-published as `9f19d21`
on 2026-07-18. The complete route, concurrency, chapter-identity, WebSocket and
existing-directory WebDAV mirror contract is
[`reading-progress-p2-contract.md`](reading-progress-p2-contract.md).

The deployed `GET /api/progress/:bookID` and `PUT /api/progress` paths remain stable. The
implementation uses a database CAS, derives chapter ID/title from the caller-owned book catalogue,
preserves the existing `200` plus
`X-OpenReader-Progress-Conflict: 1` compatibility response, and broadcasts only after one winner
commits. Existing `bookProgress/` or `legado/bookProgress/` directories regain upstream-compatible
per-book JSON mirrors without weakening OpenReader's private WebDAV roots.

## P1-B workspace search API contract

Status: implemented for the P1-B search-default/error slice on 2026-07-13 from fixed reader-dev `Index.vue`, `config.js`, and `BookController.kt`. OpenReader keeps its authenticated REST path and source-ID representation, but restores the upstream search defaults and error semantics.

| Method / path | Request | Success / side effects | Errors / compatibility adapter |
|---|---|---|---|
| `POST /api/search` | Authenticated JSON `{keyword, sourceIds?, concurrentCount?, page?, lastIndex?, searchSize?}`. `sourceIds` is the current user-scoped adaptation of upstream all/group/single source selection. | A single selected source uses `page`; multiple selected sources use `lastIndex` as a cursor over the original ordered `sourceIds` sequence. Response stays `{list,page,lastIndex,hasMore}`. Multi-source results are deduplicated; individual failed sources are recorded/skipped under the existing source-failure contract. Failure-suppressed sources are skipped without renumbering `lastIndex`; if every otherwise configured source is currently suppressed, this remains a successful empty result. | Blank keyword is `400 {error:"keyword is required"}`. No enabled/selected source must return a handled error whose frontend semantics are **“未配置书源”**, rather than a successful empty list. Existing direct clients retain the top-level `{error}` shape. |

The omitted/zero `concurrentCount` default is **24**. This is the upstream workspace default; a positive caller-provided count remains bounded by the selected source count. OpenReader does not add a `/reader3/searchBookMulti` product dependency: the frontend maps upstream `multi`/`bookSourceGroup` to ordered `sourceIds` and keeps the deployed REST response shape.

## P2-Parser-3A source-script error contract

Status: implemented and verified on 2026-07-13 against fixed reader-dev `BaseSource.kt`, `AnalyzeUrl.kt` and `WebBook.kt`. This is a Go/JWT security adaptation, not a route redesign.

| Affected existing route family | Trigger | Required response and side effect |
| --- | --- | --- |
| Search and explore (`/api/search`, `/api/explore/*`) | Source `header` starts with `@js:`/`<js>` or `loginCheckJs` is non-blank. | Keep the route's current status and top-level `error`; append `code: "source_rule_unsupported"` and `stage: "search"` or `"explore"`. Reject before any remote request and do not create a source-failure cache row. |
| Remote add/refresh/change-source and reader catalogue | The same source-script trigger. | Keep existing route status and `error`; append `code: "source_rule_unsupported"`, `stage: "book_info"`. No remote request, cache mutation or source-failure row. |
| Reader chapter content | The same source-script trigger. | Keep the existing `502` response and `error`; append `code: "source_rule_unsupported"`, `stage: "content"`. No remote request, chapter-cache write or source-failure row. |
| Source debug (`/api/sources/:id/test*`) | The same source-script trigger. | Retain authenticated `200` debug envelopes with their existing result field plus redacted `error`, `code: "source_rule_unsupported"` and the relevant stage. No remote request and no source-failure row. |

The response must never contain the script, source header, cookie, URL query, remote response body or a host path. Static JSON headers remain supported. `preUpdateJs`, `content.webJs`, option `webJs`, and `sourceRegex` are preserved but are not included in this trigger because the fixed upstream call graph does not consume them; a later implementation needs a fresh contract.

## P2 invalid-source cache API contract

Status: implemented and tested on 2026-07-12 from fixed reader-dev `BookController.kt`, `Index.vue` and `vuex.js`. See [source-failure-cache.md](source-failure-cache.md) for data and state details.

| Method / path | Request | Success / side effects | Auth and errors |
|---|---|---|---|
| `GET /api/sources/invalid` | None. | Returns `[]` or current-user, unexpired source records merged with `{errorMessage,failedAt,expiresAt}`. It never starts a source request. Expired/deleted/edited-source rows are pruned/ignored. | JWT required. `401` invalid/missing session; `500` only before a database read can complete. No source credentials, query string, remote response body, host path or internal error appears in `errorMessage`. |
| `POST /api/reader3/getInvalidBookSources` | No body. | Compatibility adapter for the same caller-scoped 600-second failures; no new frontend flow may depend on this legacy path. | JWT required and same isolation/error rules as the canonical route. |

Source-facing routes retain their current response schemas. Only a real remote source-request failure may create/update exactly one current-user failure cache row after its request has failed; a blank result, a rule syntax/unsupported-rule/configuration error, and a client-cancelled context do not. During its 600-second TTL the same caller's normal multi-source search/candidate flow skips that source, while an explicit health check may still probe it and may record a configuration failure for its health result.

## P2 backup restore archive contract

Status: **structure/budget preflight, logical content/transaction/permission compatibility, P2-S4
source ownership implementation, automated tests, dual-account browser and dedicated
ownership-upgrade Docker gates are complete and published in `0db752e`.** The archive bounds below remain authoritative. The upstream filename/field bridge,
atomic generation/restore and source-edit capability contract are defined by
[`backup-restore-fixed-baseline-p2-contract.md`](backup-restore-fixed-baseline-p2-contract.md).

| Method / path | Request | Success / side effects | Errors / safety contract |
|---|---|---|---|
| `POST /api/backup/trigger` | Authenticated, no body; effective `canAccessWebdav` required. | Existing `{message,path,name}` remains. The file stays in the administrator legacy root or regular-user private root, but every logical artifact—including `bookSource.json`—contains only the authenticated user. An uninitialized source namespace omits that member; initialized empty writes `[]`. | Existing safe `500` remains; no archive is visible before atomic completion and no other-user data or host path is exposed. |
| `POST /api/backup/restore-legado` | Authenticated multipart field `file`; filename must end in `.zip` (case-insensitive). | Existing counts remain; restore accepts `myBookShelf.json`/`bookshelf.json`, nested `bookProgress/`, upstream `bookmark.json`/`replaceRule.json` and old OpenReader plural aliases without double-processing. Allowed source restore replaces/reconciles only the authenticated user's active namespace; additive `sourcesSkipped` reports denied current-user source mutation. | Existing structural `400/413` rules remain. Supported content is fully planned before one SQLite transaction; decode/DB error returns client-safe `400/500`, rolls back all logical writes and emits no sync event. |
| `POST /api/backup/restore-webdav` | Authenticated JSON `{path}`; the caller-scoped WebDAV path must reference a `.zip` file. | Same planner/transaction/count/owner/permission semantics as uploaded restore; the sole WebDAV manager owns the confirmation. Bookshelf source name/URL resolves only in the caller's active associations. | `400` if file/path is missing, directory, non-ZIP, or archive validation fails; `413` for an oversized file. The response never exposes server paths, another user's source existence, or ZIP parser details. |

Configuration defaults: `OPENREADER_MAX_BACKUP_RESTORE_BYTES=134217728`, `OPENREADER_MAX_BACKUP_ARCHIVE_ENTRIES=5000`, `OPENREADER_MAX_BACKUP_ARCHIVE_ENTRY_BYTES=16777216`, and `OPENREADER_MAX_BACKUP_ARCHIVE_EXPANDED_BYTES=134217728`. These are an allowed OpenReader security improvement; they do not change the exported data schema or user-visible restore sequence.

P2-S4 keeps `sources` as imported/updated/reactivated count and may add
`sourceDetached`/`sourceRemoved` when replace-style reconciliation only removes or detaches old active
sources. Those additive fields also drive a target-user `sources_update`; old clients may ignore them.

### P1-E4 portable local archive extension

Status: implemented. Reader-dev has no local-file backup format, so this is a deliberately named
OpenReader extension rather than a change to either row above. The full format, scoped-root,
collision, staged-restore and limit contract is
[`portable-local-archive-backup-p1e4-contract.md`](portable-local-archive-backup-p1e4-contract.md).

| Method / path | Request | Success / side effects | Errors / compatibility |
|---|---|---|---|
| `POST /api/backup/portable/trigger` | Authenticated, no body. | Writes caller-scoped `portable_backup_*.zip`; returns `{message,path,name,format:"openreader-portable-v1",localBooks}`. The package retains the ordinary logical JSON entries and adds the v1 manifest plus verified private original archives. | `409` for missing/unsafe archive or Type=1 local audio, `413` for a configured output/archive limit, no host path in errors. `POST /api/backup/trigger` remains logical only. |
| `GET /api/backup/list`, `GET /api/backup/download/:name` | Existing authentication/name path. | List adds the optional `format` (`logical` or `openreader-portable-v1`); download accepts the existing `backup_*.zip` and additive `portable_backup_*.zip` only in the caller root. | Existing clients may ignore `format`; traversal, other prefixes and cross-user files remain rejected. |
| `POST /api/backup/restore-legado`, `POST /api/backup/restore-webdav` | Existing multipart or scoped WebDAV request. | Manifest detection dispatches v1 packages to full logical + local archive recovery and returns additive `localBooks`. Legacy reader-dev/Legado/OpenReader ZIPs retain their original restore path. | Unknown/invalid v1, collision, hash, ZIP/path or parse failure returns a client-safe error before package-controlled data mutation; a portable identity collision is `409`. |

Configuration defaults are additive: `OPENREADER_MAX_PORTABLE_BACKUP_BYTES=536870912`,
`OPENREADER_MAX_PORTABLE_ARCHIVE_ENTRIES=10000`,
`OPENREADER_MAX_PORTABLE_ARCHIVE_ENTRY_BYTES=268435456`, and
`OPENREADER_MAX_PORTABLE_ARCHIVE_EXPANDED_BYTES=536870912`. Portable archive parsing uses this
independent per-file bound rather than the smaller interactive-upload cap; normal imports retain
their own `OPENREADER_MAX_IMPORT_BYTES` policy.

### P2-B portable appearance asset v2

Status: runtime, contract tests, full suites, three-viewport real-browser restore, fresh/historical
Docker volume gates and the local amd64/arm64 GHCR release are complete as of 2026-07-27. The exact
format and transaction contract is
[`portable-appearance-assets-p2b-contract.md`](portable-appearance-assets-p2b-contract.md).

| Method / path | Implemented additive contract | Compatibility |
|---|---|---|
| `POST /api/backup/portable/trigger` | New packages return `format:"openreader-portable-v2"` plus `localBooks`, `assets`, and `legacyAssets`; v2 contains caller-referenced private cover/background/font bytes and uses opaque package placeholders. | Route/auth remain unchanged. Ordinary backups remain logical-only; existing v1 files remain restorable. Invalid/missing/cross-owner referenced assets are `409`, limits are `413`, and failure creates no final package. |
| `GET /api/backup/list` | Detect v1/v2 from the bounded root manifest instead of inferring v1 from the filename prefix. | Existing `name/size/time` fields and download basename/root checks remain. Damaged or future portable versions must not be reported or restored as v1/logical. |
| Both restore routes | v2 rewrites declared placeholders to newly allocated target-user URLs and returns `assets`/`legacyAssets`. Asset finalization and all rewritten setting/book rows use one SQLite transaction plus tested file compensation. | v1 follows its existing path. Multiple/unknown manifests, invalid assets or placeholders fail before mutation and emit no sync event. |

The v2 asset subset also retains the current upload caps (8 MiB image, 32 MiB font) inside the
existing portable global entry/expanded budgets and reruns extension/magic/image-dimension checks.

## P2 UserManage API contract

Status: account/permission/deletion behavior was implemented on 2026-07-17 from fixed reader-dev
`UserManage.vue`, `AddUser.vue`, and `UserController.kt`; the P2-S1 through P2-S3 ownership pass on
2026-07-27 subsequently replaced the global-source assumption with per-user active/detached
associations and restored the upstream administrator source actions. The 2026-07-28 re-audit restored
the persisted `lastLoginAt` contract and kept `lastActiveAt` as a response-only compatibility alias.

| Method / path | Request | Success / side effects | Auth and errors |
|---|---|---|---|
| `POST /api/auth/login` | Existing `{username,password}` body. | After credentials succeed, advances the compatible persisted `last_active_at` value, then returns both canonical `user.lastLoginAt` and deprecated `user.lastActiveAt` with the same instant. A failed credential check changes no time. | Existing login error contract remains; persistence failure is `500` and no token response is emitted. Existing usernames and password hashes remain valid. |
| `GET /api/admin/users` | None. | Returns manager-visible rows with stable current fields: `id`, `username`, `role`, limits/capabilities/counts, canonical `lastLoginAt`, deprecated alias `lastActiveAt`, and `createdAt`. Both time fields are the same persisted value. `sourceCount` is the target user's active-source count; an uninitialized namespace projects the current default count without creating rows. | JWT administrator only. A non-admin gets `403` `{"error":{"code":"FORBIDDEN","message":"admin access required"}}`; no user rows leak. |
| `POST /api/admin/users` | `{username,password,canEditSources?,canAccessStore?,canAccessWebdav?,bookLimit?,sourceLimit?}`. `role` may be absent or `user`; `admin` is rejected. | `201` creates exactly one ordinary user and broadcasts one `users_update` after commit. New LocalStore/WebDAV permissions default to `true` unless explicitly set; results expose effective `canAccessStore` and `canAccessWebdav`. Existing administrator rows are never changed/migrated. | Administrator only. Username must be at least 5 ASCII letters/digits and not `default`; password must be at least 8 characters. Invalid input/role assignment `400`; duplicate `409`. |
| `PUT /api/admin/users/:id` | Any explicit subset of ordinary-user capability/limit fields, including independent `canAccessStore` and `canAccessWebdav`. | Updates only supplied fields, returns effective permissions, then broadcasts one post-commit update. Updating either workspace permission never changes the other. | Administrator only. `403` when `:id` is an administrator, `404` missing id, `400` malformed body. |
| `PUT /api/admin/users/:id/password` | `{password}` with at least eight characters. | Changes one ordinary user's password and broadcasts once. | Administrator only. `403` for administrator target, `404` missing id, `400` invalid password/body. |
| `POST /api/admin/users/:id/sources/default` | No body. | Copies the target user's already-initialized active source list, including an explicit empty list, to the default template without modifying any user's namespace. Returns `{count}`. | Administrator only. `404` missing target; `409` target namespace not initialized; no lazy initialization or caller-source fallback. |
| `POST /api/admin/users/sources/reset` | `{ids:number[]}`. | Validates all deduplicated targets, then reconciles each target to the current default in one transaction. Returns `{reset,imported,updated,skipped}`, emits one target-scoped `sources_update` per user and one administrator `users_update` after commit. | Administrator only. `400` empty IDs; `404` missing target or missing default. Any validation/write failure leaves the full batch unchanged. |
| `POST /api/admin/users/batch-delete` | `{ids:number[]}`. | Deletes only selected ordinary users and every user-owned SQLite row, source namespace and association in one transaction. Only after commit it removes the validated private WebDAV, LocalStore, imported-library, and upload descendants; response includes safe numeric `cleanupFailures` without a host path. Unreferenced source snapshots may then be reclaimed. Emits one update after commit. | Administrator only. `400` empty/protected-only input; the current user and every administrator are excluded. A post-commit cleanup failure never rolls back the completed row deletion. |
| `POST /api/admin/cleanup-inactive` | None. | Compatibility-only background action: finds inactive ordinary users and calls the same complete deletion plan; it is not exposed in the upstream-aligned UI. | Administrator only; administrators are never deleted. |

The physical `book_sources` rows are shareable immutable snapshots, not a global user-visible list.
Visibility and mutation authority come from the target user's source namespace and associations.
P2-S4 separately governs owner-scoped source artifacts in backup/restore and the logical browser
cache-key version. All handled errors retain the deployed `{error}` shape.

## P0 local TXT preview and staged-reparse contract

Status: TXT matching and raw-byte token reuse were implemented on 2026-07-11. The parsed-snapshot and latest-request-only lifecycle correction identified on 2026-07-18 is now implemented and validated without changing the response schema; see [`local-book-import-catalog-p0-contract.md`](local-book-import-catalog-p0-contract.md).

| Method / path | Request | Success and side effects | Errors / retry contract |
|---|---|---|---|
| `POST /api/imports/books/preview` | Multipart `file`, optional `title`, `author`, `tocRule`, or an existing `importToken` instead of `file`. JWT required. | A new upload creates a caller-scoped immutable stage before parsing. Successful response is `200` with `{title,author,chapterCount,chapters,importToken}`. The server atomically records the bounded parsed result for that token and exact rule without changing the public response. An empty `tocRule` uses the automatic first-512,000-byte probe and may return pseudo chapters for text without a TOC. An explicit TXT `tocRule` with no match is likewise a normal `200` empty catalogue, retaining the token for a rule refresh or empty-catalog confirmation. EPUB preview records catalogue metadata/resource boundaries only; it does not materialize every XHTML body merely to return titles. For newly parsed EPUB TOC rules, repeated `href#fragment` entries collapse to one canonical href item; the first href fixes order, the final TOC title write fixes title, and no fragment field is exposed or persisted for that new item. | Unsupported/invalid input remains `400` `{error,importToken}`. A failed reparse must not replace the last successful parsed snapshot. No durable book rows/library archives are created during preview. |
| `POST /api/imports/books` | Same multipart/token fields plus existing category fields. JWT required. | `201` creates the book only from the staged bytes or submitted upload. When token/rule/hash match a successful preview, confirmation consumes that parsed catalogue instead of executing the catalogue parser again. Old full-content and catalogue-only snapshots—including an upgrade-time snapshot containing historical fragment rows—and stages without a snapshot remain accepted/rebuilt lazily. New preview snapshots persist one chapter per canonical EPUB href with empty fragment metadata. EPUB may prepare its caller-owned bounded derived resource tree below the new archive before commit so the first Reader request does not rehash/re-extract it. A consumed staged token and all derived snapshot files are deleted only after a successful import. A local TXT book with an explicit unmatched rule is allowed to persist with zero chapter rows, matching upstream local-book confirmation semantics. | `400` for unsupported/parser/archive-policy extraction errors; the client-safe extraction message cannot expose a host path. Host storage/database failures remain generic `500`. Failed confirmation leaves an existing stage and its last successful snapshot reusable until normal expiry. No book row, broadcast, consumed token or orphan new library directory may survive a failed confirmation. |
| `POST /api/local-store/import-preview`, `POST /api/webdav/import-preview` | JSON `{paths}` or `{items:[{path,title,author,tocRule}]}`. JWT/store permission required. | `200 {items}`. Each readable item is copied once to a caller-scoped immutable stage; success item contains `{path,book,importToken}`. | A parser failure remains an item-level `{path,error,importToken}` in the `200` envelope. The token remains valid for a later `{items:[{path,importToken,tocRule}]}` preview/import; mounted-file mutation/removal cannot affect that retry. |
| `POST /api/local-store/import`, `POST /api/webdav/import` | Existing paths/items/category body. | Successful staged item uses the original preview bytes and deletes its token after durable import. | Item-level parser failures retain the staged token and do not create book/cache rows. A container response does not expose paths outside the caller's scoped store. |

## P1-E3 workspace file-manager API compatibility

Status: implemented and regression-tested on 2026-07-13. The fixed reader-dev contract is represented by the P1-E3 workspace UI; existing OpenReader paths remain available for deployed clients even when their corresponding operation is removed from the new workbench.

| Method / path | Workbench request / response contract | Compatibility and security rule |
|---|---|---|
| `GET /api/local-store?path=<relative>` | The rebuilt LocalStore sends only the current relative directory and expects `{path,items}`. Every item includes `name`, `path`, `size`, `lastModified`, `isDir`, and an internal parser-capability `importable` flag. | The normal UI never sends `recursive`; hidden dot-name entries are omitted. A legacy client may still send `recursive=1` until separately retired. All paths remain caller-rooted. |
| `POST /api/local-store/upload` | Authenticated multipart body has `path` plus one or more `file` parts. A successful multi-file write returns `201 {paths:[...]}` in submitted order; one-file callers also receive the stable `path` first item field during the compatibility window. | Upload is file management, not parser admission: any basename-safe regular file may be stored. Each file remains bounded and atomically staged beside its target; invalid/oversized data returns a client-safe `400`/`413` and must not truncate an existing destination. |
| `POST /api/local-store/directory`, `PUT /api/local-store/rename`, `GET /api/local-store/download` | Existing authenticated request/response shapes stay unchanged. | They are legacy/API compatibility routes only; P1-E3 LocalStore UI must not create these calls. |
| `PUT /webdav/<path>` | The rebuilt WebDAV picker performs one authenticated, bounded PUT per selected file and refreshes the current directory after all successful writes. | Multiple visible selections are an UI-level sequence over the existing raw WebDAV contract. Every PUT retains normal bearer auth, caller-rooting and atomic staging. |
| `MKCOL /webdav/<path>`, `MOVE /webdav/<path>` | Existing raw WebDAV compatibility methods remain unchanged. | P1-E3 WebDAV UI must not surface new-directory or rename controls. |
| `POST /api/local-store/import-preview`, `POST /api/webdav/import-preview` | The P1-E2 controller may only be launched by reader-dev-visible suffixes: LocalStore `txt/epub/umd/cbz`, WebDAV `txt/epub/umd`. | Go can retain additional direct-parser formats for existing books/clients, but they cannot reappear as a workbench button without a later documented product decision. |

This API shape is an allowed Go/JWT adaptation: reader-dev returns an empty chapter list in its controller response for a local `TocEmptyException`; OpenReader preserves deployed REST paths and represents the same explicit no-match case as a successful empty preview. Actual malformed/unsupported parser failures remain `400` while preserving retryable staged context in direct and storage-backed flows.

## P2 replace-rule API contract

Status: re-extracted and implemented 2026-07-28 from fixed `reader-dev` `ReplaceRuleController.kt`,
`ReplaceRule.vue`, `ReplaceRuleForm.vue`, `ReplaceRule.kt`, and `Reader.vue`. The focused current
contract is [`replace-rule-fixed-baseline-p2-contract.md`](replace-rule-fixed-baseline-p2-contract.md).
OpenReader keeps REST/SQLite/JWT routes but must preserve exact field bytes and the user-visible rule
pipeline.

The implementation now uses one `services/replacerules` engine for `/test` and Reader content,
preserves exact accepted string bytes, lists/applies by `id ASC`, and emits no batch update event
when every input row was skipped and no durable write occurred. Regex execution is bounded to
32 capture groups, 20,000 matches per rule/chapter and `max(input, 64 MiB)` output; overflow preserves
the complete input to that rule and stops the Reader pipeline. Focused API/engine tests plus the
full Go suite pass; Docker volume publication evidence is recorded in the focused contract.

| Method / path | Request and validation | Success / side effects | Auth and errors |
|---|---|---|---|
| `GET /api/replace-rules` | None. | Returns only the caller's rules in stable insertion order (`id ASC`), never `sort_order` or update-time order. Compatibility output retains `enabled` plus legacy-readable `isEnabled`. | JWT required; `500` only for a read failure. |
| `POST /api/replace-rules` | `{name, pattern, replacement, scope, isRegex, enabled|isEnabled}`. Exact empty name, pattern and scope are rejected; accepted strings are not trimmed. A missing `isRegex` means plain text; regex must compile under the bounded RE2 subset. Request body ≤ 512 KiB; hidden group ≤ 800 bytes. | Current-user exact-name upsert. Appending returns `201`; replacing the earliest existing same-name row in place returns `200`, without moving pipeline order. Emits `replace_rules_update` after commit. | JWT required; `400` for missing/oversized fields, invalid or unsupported regex; no cross-user lookup. |
| `PUT /api/replace-rules/:id` | Same validated fields. | Updates only the owned ID and does not change its stable position. Emits one post-commit update event. | JWT required; `400` invalid body/regex, `404` missing/foreign ID, `409` when renaming to another existing current-user name. |
| `POST /api/replace-rules/batch` | JSON array ≤ 16 MiB/2,000 rows. Exact empty name/pattern rows retain the upstream-compatible `skipped` result; whitespace is data, not blank normalization. Every accepted rule must have an explicit scope and valid plain/regex mode before any accepted row is written. | Transactional current-user exact-name upsert in input order, returning `{rules,created,updated,skipped}`. A malformed regex rejects the batch without a partial accepted-row write. | JWT required; `400` malformed/oversized array, regex or scope, `500` before a failed transaction can mutate state. |
| `POST /api/replace-rules/test` | `{pattern,replacement,isRegex,text}` using the same compiler/mode as real Reader content; request body ≤ 4 MiB, decoded text ≤ 1 MiB and output ≤ 8 MiB. | Returns `{input,output,changed}` only; no persistence or sync event. | JWT required; `400` invalid regex, missing pattern/text, field limit or execution overflow. |
| `DELETE /api/replace-rules/:id`, `POST /api/replace-rules/batch-delete` | Existing ID paths/payload; batch body ≤ 128 KiB/2,000 IDs. | Delete only owned rows, retain ordered `deletedIds`, and emit after durable deletion. | JWT required; single missing/foreign ID is `404`; invalid/oversized batch is `400`. |

Reader content applies enabled matching rules only to text chapters, in the same listed order: plain
text changes the first occurrence; regex changes every case-insensitive occurrence. Scope comparison
is exact; an explicit second segment, including an empty one, must equal the exact book URL. For the
accepted RE2 pattern subset, replacement-string expansion must match JavaScript `String.replace`.
An execution overflow keeps every earlier rule result, keeps the overflowing rule's input intact and
stops later rules; no truncated result is returned. EPUB and audio content bypass the pipeline. A
legacy persisted empty scope remains global only to avoid breaking existing OpenReader data; any
successful edit/import writes an explicit non-empty scope.

## P1-D4 shelf-operation API contract

Status: extracted 2026-07-10. These routes retain their OpenReader paths while matching the fixed reader-dev shelf-operation behavior through a JWT/user-scoped adaptation.

| Method / path | Request | Success / side effects | Auth and errors |
|---|---|---|---|
| `PUT /api/books/:id/category` | `{ "categoryId": number }` or `{ "categoryIds": number[] }` | Replaces the shelf book's categories atomically, updates legacy primary `categoryId`, and emits one `bookshelf_update` after commit. The BookGroup set UI must not call this with an empty selection; direct API empty-array compatibility remains explicitly documented only if an ungrouped-book workflow needs it. | Owner only. `400` for malformed/foreign category, `404` for foreign/missing book, `500` only before an unsuccessful transaction can alter rows. |
| `POST /api/books/batch` | `{ "action": "delete"\|"category"\|"category-add"\|"category-remove"\|"cache"\|"clear-cache", "bookIds": number[], ... }` | Category and delete actions are transactional. Delete removes category links, chapters, bookmarks, progress, scoped browser-cache references and post-commit derived files; category actions emit one scoped `bookshelf_update`. Cache actions keep bounded request limits and emit affected shelf items only after durable cache state. | Owner only. Invalid/foreign category ids fail without mutation. Foreign book ids never expose or mutate another user's record; the response must distinguish an empty owned selection from a successful cross-user mutation. |
| `DELETE /api/books/:id` | None | Removes the caller's book rows in one transaction, broadcasts `bookshelf_delete` after commit, then prunes only that book's unreferenced remote cache files and private imported archive directory. | Owner only; `404` for another user's id. Failure before commit leaves all rows/files unchanged. Post-commit derived-cache cleanup is idempotent and must not delete another user's/shared path. |
| `POST /api/books/:id/cache` | `{ "chapterIndex"?: number, "all"?: boolean, "count"?: number, "refresh"?: boolean }` | `all=true,count<=0` means the whole remaining catalogue (chapter 0 when the index is omitted); an explicit positive count remains a max-300 compatibility window. `refresh=false` skips valid existing cache and `refresh=true` refetches it. Returns canonical `cachedCount/successCount/failedCount` plus legacy `cached/requested/failed` aliases and the refreshed book. | Owner only; malformed payload `400`, missing/foreign book `404`. Local books retain the no-server-cache result. See `book-management-cache-p2-contract.md`. |
| `POST /api/books/:id/cache/stream` | Same body as `/cache`; authenticated `fetch` request with a readable response body. | Each `message` and terminal `end` carries canonical `{ bookId, chapterIndex, processed, total, cachedCount, successCount, failedCount }` plus legacy aliases. Terminal success includes `book`. Aborting stops further fetches and skips a completed shelf broadcast while retaining already-written entries. The client must not put a JWT in a query string. | Owner only. Validation failures are JSON `400`/`404` before the stream opens. Missing/empty files are not valid existing cache. Client-safe terminal errors cannot expose source credentials, host paths, or internal stacks. |
| `GET /api/cache/stats` | None | Returns only the authenticated user's remote cache counts/size. The response never includes an absolute host cache path. | JWT required; it must not reveal another user's chapter count, filename, or root. |
| `DELETE /api/cache` | None | Clears only the authenticated user's remote chapter-cache references in a transaction, then removes only cache files left unreferenced by all chapter rows; emits a current-user shelf refresh after commit. | JWT required; no other user's database cache state or still-referenced file may be removed. |
| `POST /api/books/export` | `{ "bookIds": number[], "format": "txt"\|"epub"\|"json" }` | A single local book returns its archived original file. Remote books retain TXT/EPUB export. JSON and multi-book ZIP are explicit OpenReader extensions and remain user-scoped/bounded. | JWT required. Empty/foreign-only selections are client errors; safe `Content-Disposition` names must not expose host paths. |
| `POST /api/books/:id/refresh`, `POST /api/books/:id/refresh-local`, `POST /api/books/:id/change-source` | Existing route bodies | Replace chapter rows atomically. For EPUB `refresh-local`, the newly parsed catalogue uses one row per canonical href and empty fragment metadata; an old fragment progress/bookmark is rebound by canonical resource path before the generic index fallback. Only after commit, prune superseded derived caches while preserving `OriginalFile`, `chapters.json`, `bookSource.json`, local-store/WebDAV source files, and valid progress/bookmark recovery. Broadcast the merged shelf item after durable writes. | Owner only. Parse/fetch errors and an explicitly selected rule with no readable chapters return `400` and leave the current catalogue/cache metadata usable without deleting source files. Thus an old pure-`toc`/no-TOC EPUB remains readable after a rejected default refresh and can be explicitly refreshed with `{"tocRule":"spin"}`. Ordinary startup/read/backup never silently collapses historical fragment rows; only a successful explicit refresh applies the new catalogue. |

### P0 Reader source-candidate API contract (2026-08-11 implemented)

Status: implemented, regression-validated and Docker-published as `a2ecc17`. The stable OpenReader route remains
`GET /api/books/:id/source-candidates`; the additive `mode` query separates the fixed-upstream
available/refresh/search state transitions.

| Mode | Request and response | Authorization, side effects and errors |
| --- | --- | --- |
| omitted or `available` | No remote request. Returns the current user's saved candidate array in stable order. A historical book with no rows is transactionally seeded from its current shelf snapshot. Each row projects `current` from the authoritative book source id and URL. | JWT and current-user book ownership required; foreign/missing book is `404`. No candidate, source or source-failure data from another user may be observed. |
| `refresh` | Revalidates only source ids already represented in the saved candidate set. Remote results are retained only when both title and author exactly match the shelf book. The response is the replacement saved array; the current shelf snapshot is retained when its source fails. | Partial source failure records only the caller-scoped failure cache and does not roll back successful candidates or remove the current candidate. No active but uncached source is contacted. |
| `search` | Accepts `group`, `offset`, `limit`, and compatibility `paged=1`. `offset` is the next active-source index in that group. Returns `{list,offset,nextOffset,hasMore,total,searched,matched,failed,empty}`; `list` contains only this scan batch. | Uses caller-active enabled sources, existing failed-source filtering, bounded concurrency, per-source timeout and request cancellation. Only exact title+author matches are merged by `bookUrl` into the derived cache. |

Unknown `mode` is `400` with the existing `{error}` envelope. The server derives title and author from the
owned book and never accepts client identity fields for candidate discovery. `POST /api/books/:id/change-source`
keeps its existing body and success object, but commits the selected candidate snapshot with the source/catalogue
transaction; the frontend updates the current projection locally and must not issue a follow-up candidate search.

The upstream uses namespace-specific JSON storage and SSE cache progress. OpenReader's REST/SQLite adaptation is allowed only where it preserves the visible action semantics, current-user isolation, durable event ordering, and bounded resource use described above.

### P1 manual shelf refresh API contract

Status: implemented and regression-validated 2026-08-09; Docker pending. Full state, transaction and test requirements are in
[`bookshelf-manual-refresh-fixed-baseline-second-audit-p1-contract.md`](bookshelf-manual-refresh-fixed-baseline-second-audit-p1-contract.md).

| Method / path | Request | Success / side effects | Auth and errors |
|---|---|---|---|
| `POST /api/books/check-updates` | Empty body or `{}`; deployed-client extra JSON fields remain ignored. | Checks only the caller's remote `canUpdate` shelf books with fetch concurrency ≤16. Returns backward-compatible `{newChapters,books}` plus `{checked,updated,failed,replacedBookIds}`. Every successful book is committed atomically; one book's remote/parse/transaction/stale-snapshot failure does not roll back another book. A single post-commit `bookshelf_update` contains changed shelf items. | JWT required. Candidate-list or final-projection failure is `500 {"error":"检查书籍更新失败"}`; per-book failures remain `200` and are represented only by a safe count. No error may expose rules, credentials, remote bodies or host paths. |

The successful remote TOC is authoritative for rename, URL change, reorder and shrink as well as append growth.
Exact-prefix growth preserves existing chapter IDs/cache; non-prefix replacement rebinds progress/bookmarks and
prunes only superseded derived cache after commit. `lastCheckTime` advances only for positive growth relative to the
persisted book summary. Initial/focus/WebSocket shelf reloads do not invoke this route.

## EPUB reader resource contract

### Authenticated chapter response

`GET /api/books/:id/chapters/:index/content`

| Field | Contract |
|---|---|
| Method/path | Existing `GET /api/books/:id/chapters/:index/content`; no path change. |
| Auth | Existing `Authorization: Bearer <jwt>` requirement. The book lookup remains scoped to the authenticated user. |
| Request | Existing numeric book ID and zero-based chapter index. |
| Text response | `200` JSON keeps `chapter` and `content`; adds `"format": "text"`. |
| EPUB response | `200` JSON keeps `chapter` and searchable plain-text `content`; adds `"format": "epub"`, `resourceUrl`, and RFC3339 `resourceExpiresAt`. Plain text is generated/cached only from the requested resource; Reader iframe preparation does not wait for unrelated chapters. Newly parsed catalogues have one row per canonical href and empty fragment fields. A historical fragment chapter keeps canonical `chapter.resourcePath` plus nullable `resourceFragment`/`resourceEndFragment`; its `resourceUrl` includes an encoded `#resourceFragment` for iframe location. |
| Side effects | For EPUB, may safely reuse/extract/rebuild a derived resource tree, lazily create only the requested chapter's text cache, and backfill canonical `resourcePath` plus nullable fragment metadata. If an old row has no path and its persisted pure `toc` rule yields no chapters, runtime recovery may use the spine at the same index solely to keep that historical row readable; new catalogue generation does not use this fallback. A valid complete marker whose source size/mtime still match may avoid a redundant SHA-256 pass; any identity change or invalid marker must rehash/rebuild and invalidate the old capability. It must not alter the archived source EPUB. |
| `400` | Invalid book/chapter parameter. |
| `404` | Book/chapter/source archive is not available to the current user. |
| `422` | EPUB exists but is corrupt, unsafe, unsupported, or exceeds extraction limits. |
| `500` | Unexpected persistence or filesystem failure. |
| Error body | `{ "error": "<stable client-safe message>" }`; never include a host filesystem path or token. |

The EPUB additions are backward-compatible JSON fields. Existing clients that only consume `chapter` and `content` continue to work.

Example:

```json
{
  "chapter": {
    "id": 7,
    "bookId": 3,
    "index": 0,
    "title": "第一章"
  },
  "content": "第一章\n正文……",
  "format": "epub",
  "resourceUrl": "/api/epub-resource/<capability>/OEBPS/chapter-1.xhtml",
  "resourceExpiresAt": "2026-07-06T12:00:00Z"
}
```

### Capability-protected EPUB resources

`GET /api/epub-resource/:capability/*resourcePath`

| Field | Contract |
|---|---|
| Auth | Does not accept or require the login Bearer token. Authorization is the signed path capability returned by the protected chapter endpoint. |
| Capability scope | One user ID, one book ID, one source fingerprint/extracted version, read-only access, and a bounded expiration. For a fragment chapter it additionally signs the canonical XHTML path and nullable start/end fragment ids used to slice that one document; it is never interchangeable with a login JWT. |
| Path | `resourcePath` is URL-decoded once, normalized as an EPUB POSIX path, and resolved strictly below that book/version's derived extraction root. |
| Success | `200` with a supported XHTML/HTML, CSS, image, SVG, or font MIME type. `HEAD` may return the same headers without a body. |
| XHTML | Dynamically receives the OpenReader iframe bridge and restrictive security headers. When the capability is for a fragment chapter, the served document contains only the upstream-equivalent start/end DOM range; sibling CSS/image/font resources retain the same capability root and are not sliced. The archived/extracted source file is not modified in place. |
| Relative assets | The capability remains a stable path segment so relative chapter CSS/image/font/link URLs stay within the same authorized root. |
| `400` | Malformed capability or unsafe/malformed resource path. |
| `403` | Invalid signature, expired capability, wrong purpose, wrong archive version, or book ownership no longer matches. |
| `404` | Scoped book/resource no longer exists. |
| `415` | Resource media type is not on the EPUB reader allowlist. |
| Error body | JSON for API-style failures. Iframe failures remain non-blank because the parent detects the resource load failure and displays the reader retry state. |

Security headers include at minimum:

- `X-Content-Type-Options: nosniff`;
- `Referrer-Policy: no-referrer`;
- a CSP that permits only the injected bridge script and same-capability local styles/images/fonts/data resources;
- no permissive cross-origin credential policy.

The route must not log the capability value. Application access logs should redact the capability path segment.

## CBZ reader resource contract

### Authenticated chapter response

`GET /api/books/:id/chapters/:index/content`

| Field | Contract |
|---|---|
| Method/path | Existing `GET /api/books/:id/chapters/:index/content`; no path change. |
| Auth | Existing `Authorization: Bearer <jwt>` requirement. The book lookup remains scoped to the authenticated user. |
| Request | Existing numeric book ID and zero-based chapter index. |
| CBZ response | `200` JSON keeps `chapter` and `content`; adds `"format": "cbz"`, `resourceUrl`, and RFC3339 `resourceExpiresAt`. `content` remains compatible with the upstream image chapter shape and contains an `<img>` tag pointing at `resourceUrl`. |
| Side effects | May verify/recover the chapter `resourcePath` and create/reuse a bounded immutable `.cbz-resources/<source-fingerprint>/` generation below the private book root. A complete generation is selected by its marker and matching source identity without reopening or rehashing the CBZ on every chapter. It must not modify the original CBZ archive. |
| `400` | Invalid book/chapter parameter or unsafe archive path. |
| `404` | Book/chapter/source archive/page is not available to the current user. |
| `415` | The selected CBZ entry is not a supported image media type. |
| `422` | CBZ exists but is corrupt, unsafe, unsupported, or exceeds parser/resource limits. |
| `500` | Unexpected persistence or filesystem failure. |
| Error body | `{ "error": "<stable client-safe message>" }`; never include a host filesystem path or token. |

The CBZ additions are backward-compatible JSON fields. Existing clients that only consume `content` will see upstream-style image HTML.

### CBZ bookshelf and book-detail cover projection

`POST /api/imports/books`, `POST /api/local-store/import`, `POST /api/webdav/import`,
`GET /api/books`, `GET /api/books/:id`, and any existing `bookshelf_update` payload retain their
current book/book-list JSON shapes. For a local CBZ with no `customCoverUrl`, the existing
`coverUrl` field is projected at response time to
`/api/cbz-resource/<capability>/<first-safe-archive-image>`.

The source image is the first safe image encountered in CBZ archive order, matching
reader-dev `CbzFile.parseBookInfo`; it is intentionally independent of the lexicographically
sorted chapter catalogue. A new import prepares that same bounded immutable image generation
before committing the book, so projecting a cover or opening the first page does not rescan the
archive. The response capability is bound to the current user, book and archive fingerprint,
expires normally, and remains readable without appending the login JWT.
`coverUrl` capability values and archive member paths are **not** written to `books`,
`chapters.json`, `bookSource.json`, backups, WebDAV metadata, or logs. A user-supplied
`customCoverUrl` remains the frontend's first-choice cover and is never overwritten.

If both the archived CBZ and its last complete derived generation are unavailable, malformed,
unsafe, over budget, or have no supported image,
the stable book endpoint stays successful with its stored/empty `coverUrl`; it must not turn a
normal bookshelf response into an archive or host-path error.

Example:

```json
{
  "chapter": {
    "id": 9,
    "bookId": 4,
    "index": 0,
    "title": "001.jpg",
    "resourcePath": "pages/001.jpg"
  },
  "content": "<img src=\"/api/cbz-resource/<capability>/pages/001.jpg\" />",
  "format": "cbz",
  "resourceUrl": "/api/cbz-resource/<capability>/pages/001.jpg",
  "resourceExpiresAt": "2026-07-06T12:00:00Z"
}
```

### Capability-protected CBZ image resources

`GET|HEAD /api/cbz-resource/:capability/*resourcePath`

| Field | Contract |
|---|---|
| Auth | Does not accept or require the login Bearer token. Authorization is the signed path capability returned by the protected chapter endpoint. |
| Capability scope | One user ID, one book ID, one source fingerprint, read-only access, and a bounded expiration. It is signed with a purpose-separated key derived from `OPENREADER_JWT_SECRET`; it is never interchangeable with a login JWT or EPUB capability. |
| Path | `resourcePath` is URL-decoded once, normalized as a ZIP/POSIX path, and resolved strictly below that book/fingerprint's complete immutable derived generation. Only supported image media are extracted or served. |
| Source identity | A complete marker records fingerprint, source size and modification time. Matching identity selects the generation without hashing; changed identity is hashed once and invalidates a mismatched old capability. If the archived source is temporarily absent, an already complete generation for the signed fingerprint remains readable. |
| Recovery | If the signed generation/resource is absent and the current archived source is available, the service may run the bounded atomic extraction once. It serves the rebuilt file only when its fingerprint still matches the signed capability. |
| Success | `200` with a supported image MIME type, `Content-Length`, `Last-Modified`, and byte-range support. A valid single range returns `206` plus `Content-Range`; an unsatisfiable range returns `416`. `HEAD` returns the corresponding headers and no body. Responses stream the derived file and do not load the whole image or reopen the CBZ. |
| `400` | Malformed capability or unsafe/malformed resource path. |
| `403` | Invalid signature, expired capability, wrong purpose, wrong archive fingerprint, or book ownership no longer matches. |
| `404` | Scoped book/resource no longer exists. |
| `415` | Resource media type is not on the CBZ image allowlist. |
| Error body | JSON for handled failures. Reader displays a retryable chapter error rather than a blank page when the image cannot be resolved. |

Security headers include at minimum:

- `X-Content-Type-Options: nosniff`;
- `Referrer-Policy: no-referrer`;
- `Cross-Origin-Resource-Policy: same-origin`;
- private short-lived cache headers.

The route must not log the capability value. Application access logs should redact the capability path segment.

## Audio reader resource contract

### Authenticated chapter response

`GET /api/books/:id/chapters/:index/content`

| Field | Contract |
|---|---|
| Method/path | Existing `GET /api/books/:id/chapters/:index/content`; no path change. |
| Auth | Existing `Authorization: Bearer <jwt>` requirement. The book lookup remains scoped to the authenticated user. |
| Detection | Audio reading applies to books whose `type` is `1`, matching upstream `readingBook.type === 1`. |
| Audio response | `200` JSON keeps `chapter` and `content`; adds `"format": "audio"`, `resourceUrl`, and RFC3339 `resourceExpiresAt`. `content` remains the same audio URL string for clients that already read it directly. |
| Resource source | For remote audio chapters, the audio URL may remain a source-provided HTTP(S) URL if it is already safe for direct browser playback. For local/private library audio, return a same-origin signed resource URL. |
| Side effects | No text cache rewrite is required. Progress writes store the current playback second as the chapter offset, matching upstream `durChapterPos` behavior. |
| `400` | Invalid book/chapter parameter, unsafe audio URL, or malformed local resource path. |
| `404` | Book/chapter/source audio is not available to the current user. |
| `415` | Resource media type is not on the audio allowlist. |
| `500` | Unexpected persistence, source, or filesystem failure. |
| Error body | `{ "error": "<stable client-safe message>" }`; never include host filesystem paths, signed tokens, cookies, or source credentials. |

Example:

```json
{
  "chapter": {
    "id": 12,
    "bookId": 5,
    "index": 0,
    "title": "第一集"
  },
  "content": "/api/audio-resource/<capability>/tracks/001.mp3",
  "format": "audio",
  "resourceUrl": "/api/audio-resource/<capability>/tracks/001.mp3",
  "resourceExpiresAt": "2026-07-07T12:00:00Z"
}
```

### Capability-protected local audio resources

`GET /api/audio-resource/:capability/*resourcePath`

| Field | Contract |
|---|---|
| Auth | Does not accept or require the login Bearer token. Authorization is the signed path capability returned by the protected chapter endpoint. |
| Capability scope | One user ID, one book ID, one source fingerprint, read-only access, and a bounded expiration. It is signed with a purpose-separated key derived from `OPENREADER_JWT_SECRET`; it is not interchangeable with login, EPUB, or CBZ capabilities. |
| Path | `resourcePath` is URL-decoded once, normalized, and resolved strictly below that book's local library root or approved archive-derived resource root. |
| Local chapter resolution | A local/private audio chapter may identify its media file through chapter content, `chapter.url`, or `chapter.resourcePath`. Absolute filesystem paths and relative paths are accepted only after they resolve under the authenticated book's library root. Remote HTTP(S) URLs continue to use the safe direct remote contract above. |
| Success | `200` with a supported audio MIME type. `HEAD` returns the same client-relevant headers without a body. Byte-range requests return `206` with `Content-Range` when serving local files so browsers can seek efficiently. |
| `400` | Malformed capability or unsafe/malformed resource path. |
| `403` | Invalid signature, expired capability, wrong purpose, wrong source fingerprint, or book ownership no longer matches. |
| `404` | Scoped book/resource no longer exists. |
| `415` | Resource media type is not on the audio allowlist. |

Security headers include at minimum:

- `X-Content-Type-Options: nosniff`;
- `Referrer-Policy: no-referrer`;
- `Cross-Origin-Resource-Policy: same-origin`;
- private short-lived cache headers.

Remote audio URLs must be validated before they are returned to the browser: only HTTP(S), no embedded credentials, no JavaScript/data/file schemes, and no server-side credential leakage.

Implementation tests must cover:

- remote HTTP(S) audio remains unchanged and does not leak the login JWT;
- local/private audio chapter responses return `/api/audio-resource/<capability>/<path>`;
- `GET`, `HEAD`, and `Range` requests serve only allow-listed audio media under the scoped book library root;
- modified, expired, wrong-purpose, wrong-user/book, traversal, missing-file, and unsupported-media requests fail with client-safe errors;
- access logs redact `/api/audio-resource/<capability>/...` the same way EPUB/CBZ resource capabilities are redacted.

## Legacy WebDAV summary

| Method | Path | Purpose |
|---|---|---|
| `OPTIONS/PROPFIND` | `/webdav/*path`, `/reader3/webdav/*path` | Discover and list with standard DAV metadata. |
| `GET` | both roots | Download; `/webdav` additionally retains the deployed browser directory-list adapter. |
| `PUT/MKCOL/MOVE/COPY/DELETE` | both roots | Caller-scoped file mutations with fixed protocol statuses. |
| `LOCK/UNLOCK` | both roots | Upstream-compatible stateless lock handshake. |

WebDAV paths are normalized, caller-rooted, and protected from traversal and symlinks. Protected methods accept
`Authorization: Bearer <JWT>` or HTTPS-only Basic credentials; missing/invalid credentials return `401` before
filesystem access, while an authenticated user whose effective `canAccessWebdav` permission is false receives
`403` before path parsing or file mutation. Anonymous OPTIONS remains available for discovery. The browser uses
header-based authenticated requests and never appends a JWT to a download URL. The focused table above and
[`webdav-protocol-p2-contract.md`](webdav-protocol-p2-contract.md) supersede this summary.

## Workspace storage access contract

`/api/local-store*` requires authenticated `canAccessStore`; raw `/webdav/*`, `/api/webdav/import*`, and `/api/backup/*` require authenticated effective `canAccessWebdav`. A missing/invalid token returns `401`; the relevant disabled capability returns `403`; handlers must perform that check before validating a supplied path, parsing a multipart body, reading an archive, or creating a backup. Direct `/api/imports/books*` remains an authenticated bookshelf action and is not made dependent on either workspace-entry permission.

Storage resolves without destructive migration: administrators continue to use the existing LocalStore/WebDAV roots so mounted legacy files remain readable; regular users resolve below `users/<safe-username>/` within the same mounts. Generated backup list/download/restore follows that same scope, and scheduled backups are generated per user. Direct LocalStore/WebDAV imports may carry a user-scoped `importToken` returned by preview; on confirmation it authoritatively selects the immutable staged bytes rather than rereading the mutable storage path.

`POST /api/auth/register` uses the same new-account input rule as manager-created
users. This does not invalidate existing data: already-persisted accounts with a short
password or legacy username remain able to use `POST /api/auth/login`; validation runs
only when creating a new account. The protected `admin` first-account behavior remains
an allowed OpenReader runtime adaptation.

## Bookmark contract

| Method / path | Request | Success / side effects | Auth and errors |
|---|---|---|---|
| `GET /api/books/:id/bookmarks` | None | Lists the caller-owned book's independent bookmarks in stable `id ASC` creation order. | JWT and book ownership required; foreign/missing book is `404`. |
| `POST /api/books/:id/bookmarks` | Reader location plus non-empty `excerpt`/paragraph context and optional `chapterId`. | Creates one immutable location/context record and broadcasts after the durable write. Numeric position fields are normalized. | JWT/current-book required; empty context, oversize data, and a chapter belonging to another book are `400`. |
| `POST /api/books/:id/bookmarks/batch` | Array of the same payload shape. | Validates every row before one transaction; request order becomes creation order and a bad row leaves no valid prefix behind. | JWT/current-book required; empty/malformed batches or any invalid row return `400`. |
| `PUT /api/bookmarks/:id` | `{ "note": string }` | Edits the note only; the original book, chapter, offset, title, and paragraph context remain unchanged. | JWT/current-user required; absent row `404`, oversize note `400`. |
| Backup / restore `bookmarks.json` | Existing JSON shape, including `bookTitle` and `bookUrl`. | Exports ID/creation order; restores modern timestamped rows idempotently without merging independent same-location bookmarks, and remaps a matching destination chapter by index. | Per-user restore scope applies. Legacy rows without timestamps remain readable through the narrow fallback identity. |

## Reader book-content search contract

Fixed-baseline correction audited on 2026-07-27: both routes search the original chapter text
(`reader-dev` explicitly uses `useReplace=false`), use case-sensitive exact matching, and advance the
next lookup from the previous match position plus one so overlapping matches are retained. Existing
tests that require case-insensitive or punctuation/whitespace-normalized result creation do not prove
upstream compatibility and must be replaced. OpenReader may retain bounded scans, cancellation and
explicit partial-result metadata as documented safety adaptations.

Second-audit correction on 2026-08-02: “exact” also applies to the query value itself. The fixed
upstream forwards `keyword` unchanged and rejects only a zero-length string; it does not trim leading,
trailing, or all-space queries. The modern Vue/Go path and Reader selection intent must therefore
preserve the same raw value end to end. See
[`book-content-search-fixed-baseline-second-audit-p2-contract.md`](book-content-search-fixed-baseline-second-audit-p2-contract.md).

| Method / path | Request | Success / side effects | Auth and errors |
|---|---|---|---|
| `GET /api/books/:id/search` | Raw `q` (or legacy `keyword`), optional `paged`, `lastIndex`, `chapterLimit`, `matchLimit`, `scanUntilMatch`, and local/remote work bounds. Alias selection must not trim the selected query. | Lists caller-owned book matches in source chapter order from raw, non-replaced chapter text. The unchanged query participates in exact, case-sensitive, position-ascending and overlapping matching and is returned unchanged in every row. A cursor always represents the last fully scanned chapter: all matches from that final chapter are returned before a later request can start at its successor. Response preserves `{ list, lastIndex, hasMore, total }` and additionally reports explicit `incomplete`, `unavailableChapters`, and `truncated` states. Offset/line/percent may remain additive Vue/Go navigation fields. | JWT/current-book required; a truly zero-length query is `400`, while leading/trailing/all-space values are valid exact queries. Foreign/missing book is `404`. A remote book whose configured source no longer exists fails before chapter scanning with `400 {"error":"未配置书源"}`. Request cancellation stops remote fetch scheduling without writing a false successful result. A returned incomplete page is `200` with a client-safe state, not a host/source error. |
| `GET` / `POST /api/reader3/searchBookContent` | Existing `url`/`bookUrl`, raw `keyword`, `lastIndex`, `size` aliases. URL normalization remains allowed, but `keyword` is never trimmed. | Keeps legacy `{ isSuccess, data: { list, lastIndex, hasMore, total } }` response and upstream URL lookup behavior. Every list row includes `resultCountWithinChapter`, `resultText`, `chapterTitle`, the unchanged `query`, `chapterIndex`, `queryIndexInResult`, and `queryIndexInChapter`; indexes and the at-most-20-unit excerpt radius use Java/Kotlin UTF-16 code-unit semantics. Additive incomplete fields remain allowed. | JWT required; zero-length keyword, missing shelf book and missing configured source remain HTTP 200 `isSuccess:false` with the upstream Chinese `errorMsg`; whitespace-only keyword is valid. Like the modern route, request cancellation must flow into raw chapter loading, stop later remote fetches, and return without fabricating a successful page. |

Search result selection is a frontend event, not a new API request: the App-level Dialog emits one
monotonic Reader intent for every row click. Re-selecting the same row must execute again. Live selection
must not use `router.push` or add browser history; existing `chapter/line/match/q` URLs remain accepted for
cold-start/deep-link compatibility and may be mirrored only with history-neutral replacement. The Dialog,
Pinia intent, Reader matcher and cold-start route must not trim or otherwise normalize the query.

OpenReader retains bounded remote/local scanning as a runtime/security adaptation; matching itself is the fixed-upstream exact, case-sensitive, overlapping algorithm and is not normalized. A bound may never silently advance `lastIndex` past omitted same-chapter matches: it must set `truncated: true`, and the UI must say that results are incomplete. Unavailable remote content is likewise surfaced by `incomplete/unavailableChapters` rather than as a false “没有匹配内容”. The frontend must pass an `AbortSignal`; closing the search dialog, replacing its keyword/book, or resetting its state aborts the active transport request without treating the intentional abort as a visible search error. Closing and reopening the same book preserves completed keyword/results/scroll state, matching the upstream root-dialog lifecycle.

## BookInfo and result-card remote add contract

The 2026-08-02 fixed-baseline BookInfo audit corrected the ownership of two distinct upstream actions.
Search/Explore result cards own the category-confirmed add action; an unshelved BookInfo owns a direct
add action and must not open the category chooser. Both actions may share the same authenticated
mutation utility and stable OpenReader endpoint, but they do not share the same visible state machine.

| Method / path | Request | Success / side effects | Auth and errors |
|---|---|---|---|
| `POST /api/books/remote` | `{title,bookUrl,sourceId,author?,coverUrl?,intro?,kind?,wordCount?,sourceName?,variable?,categoryId?,categoryIds?}`. Result-card confirmation supplies its selected positive category IDs. BookInfo direct add supplies no positive category ID; an omitted or empty category list means an ungrouped new book. | A new URL is parsed and created as `201` with the authoritative shelf projection, chapters and exactly the requested positive category memberships. An existing caller-owned URL returns `200` without duplicating the book. If the request has no positive category selection, an existing book's memberships are preserved; category clearing remains the explicit category endpoint's responsibility. Every successful durable mutation emits one caller-scoped shelf update. | JWT and a caller-visible enabled source are required. Required-field/category/variable validation and source parsing failures retain the existing safe `400 {error,code?,stage?}` forms; internal persistence failures are `500`. No other user's same URL or category is visible or mutated. |

Frontend ownership rules:

- Search/Explore result-card “加入书架” stops card navigation, opens `设置分组`, starts no write on
  `暂不加入`, and calls the endpoint only after `确定`.
- Unshelved BookInfo “加入书架” calls the endpoint directly, never opens the chooser, remains on the
  current Index or temporary Reader route, and upserts only the returned server projection.
- Opening a result cover only opens the shared BookInfo; opening result body may create a temporary
  Reader session. Neither preview action persists a shelf book.

## P1-B remote temporary-reader contract (second audit Docker-published)

Reader-dev permits a search/explore result to enter Reader before it has been
added to the shelf. Its Vuex `readingBook` is an in-memory reading context, not
a saved shelf row. OpenReader adds a server-owned expiring session so Vue 3 can
preserve that behavior without treating `POST /api/books/remote` as a read
operation. The main slice is implemented and covered by an API contract test
plus the desktop and two mobile browser flows on 2026-07-13. The fixed-baseline
second audit on 2026-08-09 found and corrected missing request/session memory
budgets and an invalid chapter-index lease extension. The authoritative
lifecycle, variable, cancellation, redaction and retention contract is now
[`remote-reader-session-fixed-baseline-second-audit-p1-contract.md`](remote-reader-session-fixed-baseline-second-audit-p1-contract.md);
implemented, regression-validated and Docker-published as `30dbe53`.

| Method / path | Request | Success / side effects | Auth and errors |
|---|---|---|---|
| `POST /api/reader/remote-sessions` | `{ sourceId, bookUrl, title, author?, coverUrl?, intro?, kind?, wordCount?, variable? }`. `sourceName` is display-only and ignored for authorization. The decoded body must be limited to 64 KiB. | Validates the caller-visible source and normalized bounded variable map; resolves BookInfo + TOC once with the current source snapshot; returns `201 { id, expiresAt, book, chapters }`. `book.id` is always `0`; its opaque `variable` remains available only for a later explicit add-to-shelf. The server stores only a user-bound, opaque, expiring and budgeted runtime session; it creates **no** Book, Chapter, Progress, Bookmark, cache file, backup record, or websocket bookshelf event. | JWT required. Missing `sourceId`/`bookUrl`/`title` or invalid variables: `400`; body/session too large: `413`; unavailable source: `404`; parser/request failure: safe `502 { error, code?, stage: "book_info" }`; no raw rule/header/cookie/URL-query detail is exposed. Source-request failures may enter the caller's existing short-lived source-failure cache. |
| `GET /api/reader/remote-sessions/:id` | Opaque session id. | Returns the original normalized `{ id, expiresAt, book, chapters }` without reparsing or persisting it. `Cache-Control: no-store`. | JWT required. Unknown or another user's id: `404`; expired id: `410 { error: "remote reader session expired" }`. |
| `GET /api/reader/remote-sessions/:id/chapters/:index/content` | Opaque session id and non-negative chapter ordinal. Index validation occurs before session lookup/lease renewal. | Uses only the server-stored source snapshot, book variables and chapter variables to fetch/parse that TOC row. Returns the normal Reader `{ chapter, content, format }` shape (and the existing safe remote-audio fields when applicable). It must never accept a client-supplied chapter URL or source rule. Refreshes the bounded idle expiry only for a valid request and never writes a shelf cache/chapter/progress row. | JWT/session binding required; malformed index `400`; unknown/foreign/evicted session `404`; expired `410`; missing chapter `404`; parser/request failure `502 { error, code?, stage: "content" }`. Cancellation stops further source work and returns no synthetic success. |

### Runtime and frontend boundary

- Session state is held server-side behind a high-entropy opaque ID carried only by the authenticated OpenReader route/API; it is never a JWT, never appears in a source URL, never enters backup/WebDAV/export data, and uses `Cache-Control: no-store`. The implemented idle TTL is 30 minutes and the absolute lifetime is four hours. Expiration returns `410`, never a silent account/session logout.
- The complete source snapshot, request credentials and resolved fetch URLs stay server-side. Bounded opaque Book/Chapter variables may be returned to the same authenticated user because they are upstream entity state and the Book variable is needed for a later explicit add-to-shelf; the frontend never interprets them as request URLs. Temporary sessions deliberately do **not** save browser-local or server progress, and must not call `/progress/:bookId`, bookmark, cache, category, source-change, refresh, or any other shelf-ID endpoint with a fabricated ID.
- Retention is bounded by the specialist contract: 8 MiB per session, eight sessions/32 MiB per user, and 128 sessions/128 MiB per process, with deterministic least-recently-used eviction after expired-session purge. Eviction is memory-only and appears as 404.
- Search and Explore must call this same session creation endpoint and use the same reader route form, e.g. `/reader/remote/:sessionId`; neither preview nor result-body reading may call `POST /books/remote`. Persistence starts only from an explicit result-card or BookInfo “加入书架” action. The result card confirms categories first; BookInfo adds directly. Both may forward the returned opaque `variable` field.
- Reader controls that require a durable shelf record (bookmark creation, group editing, cache/clear cache, durable progress, source change/refresh) must be either temporarily unavailable with an explicit “加入书架后可用” state or receive a separately documented temporary-session contract. They must never fail as a hidden `404` caused by a synthetic book ID.

Regression evidence now includes `backend/api/remote_reader_contract_test.go`, `backend/api/remote_reader_second_audit_contract_test.go`, `backend/services/remotereader/store_contract_test.go`, focused race, full Go/vet, frontend 713/713/build, three-viewport `remote-reader-contract.mjs`, and real Go CSS/JSONPath/XPath `source-parser-workflow-contract.mjs`. Body/retention budgets, expiry/LRU, invalid-index lease behavior, safe parser error redaction, cancellation, source-failure cache and cross-chapter variable propagation are covered. Local dual-architecture build, GHCR digest readback and fresh/historical mounted-volume/backup gates passed for `30dbe53`.

## Compatibility rule

If a refactor changes frontend routes, API paths should stay stable unless an old path is kept as a redirect/shim. Document removals before deleting compatibility behavior.

## P2 owner-scoped book-source API contract (2026-07-27 extracted)

Reader-dev stores active sources below each user's namespace and copies defaults only when that private source file
does not yet exist. OpenReader keeps its deployed JWT `/api/sources*` routes and relational source IDs, but every
lookup must first resolve the authenticated user's active association. Knowing another account's numeric source ID
does not authorize access.

| Method / path | Stable request and success response | User scope, side effects and errors |
|---|---|---|
| `GET /api/sources` | `200` ordered `BookSource[]`; existing JSON fields remain unchanged. `usedBookCount` counts only caller-owned books and the additive response-only `usedBookNames:string[]` contains those books' titles in stable book-ID order. An unused source returns an empty array. | Lazily initializes the caller from the current default exactly once. The usage projection must be computed in bounded grouped queries, never per-source N+1 reads, and must not expose another user's title. It does not add a SQLite column or enter source export/backup/WebDAV data. An initialized empty namespace returns `[]`; persistence failure is `500 {"error":"failed to list sources"}`. |
| `GET /api/sources/:id` | Existing `200 BookSource`. | Only a caller-active association is addressable. Missing, detached, or foreign ID is the same `404 {"error":"source not found"}`; no ownership detail is disclosed. |
| `POST /api/sources` | Existing payload/default normalization and `201 BookSource`. | Requires JWT plus `CanEditSources`; creates only a caller association. Validation remains `400`, disabled editing `403`, persistence failure `500`. After commit, `sources_update` is sent only to the caller's clients. |
| `PUT /api/sources/:id` | Existing full source payload and `200 BookSource`. The returned ID may change when an upgraded shared snapshot is copied. | Requires JWT plus `CanEditSources`; foreign/detached ID is `404`. A shared snapshot is copy-on-write and only caller books/failure rows are remapped. Semantic rule changes clear only caller variables. Persistence failure remains `500`. |
| `DELETE /api/sources/:id` | Existing `204`; a caller-owned source used by caller books remains `409 {"error":"source is used by bookshelf books","usedBookCount":N}`. | Requires JWT plus `CanEditSources`. Foreign/detached ID is `404`. Deletion removes only caller association/failures; the shared snapshot is garbage-collected only when globally unreferenced. |
| `GET /api/sources/export?sourceIds=...` | Existing JSON download and sourceIds validation. With no IDs, exports all caller-active sources; supplied IDs retain caller order/filter behavior. | Foreign/detached IDs are omitted and cannot be exported. A selection containing no caller source returns `200 []`; malformed input remains `400`. |
| `POST /api/sources/:id/test`, `/test-chapter`, `/test-content`; `POST /api/sources/:id/debug/stream`; `POST /api/sources/batch-test`; `GET /api/sources/invalid` | Existing test payloads and optional structured parser error fields remain; the additive canonical stream is defined in `source-debug-fixed-baseline-second-audit-p2-contract.md`. | Single-item foreign/detached ID is `404`; batch selection silently excludes non-caller IDs. Debug/test requests never write `source_failures`; only batch health-check reads/writes are caller-scoped. |
| `DELETE /api/sources`; `POST /api/sources/batch`; import/remote-import | Existing request validation and response envelopes remain stable. | Mutation affects only caller-active associations. Batch foreign IDs are not counted as affected. Import identity is normalized `bookSourceUrl/baseUrl` within the caller namespace; no same-name or ID match may mutate another user. |
| `GET /api/sources/default`; `POST /api/sources/default/restore` | Existing status/restore paths remain compatible. Restore reconciles only the caller against current default; initialized empty and restore-default remain distinct states. | Restore requires `CanEditSources`; used unmatched sources become detached instead of leaving dangling book references. |
| `POST /api/sources/default/save` | Existing path remains as an admin compatibility shim and returns `{count}`; `count` may be zero for an explicitly configured empty default. | Requires administrator role in addition to `CanEditSources`; copies the caller's active list into the default namespace. It does not rewrite initialized users or broadcast a false private-source update to every account. |
| `GET /api/admin/users` | Existing user summary array; additive `sourceCount` remains. | `sourceCount` is each target's active association count, excluding detached rows. An uninitialized target projects the current default count without creating its namespace. Admin JWT required. |
| `POST /api/admin/users/:id/sources/default` | No body; `200 {"count":N}` where `N` may be zero. | Admin JWT required. Missing target is `404 {"error":"user not found"}`; existing but uninitialized target is `409 {"error":"user sources are not initialized"}`. Copies that target's active snapshot to default and never falls back to caller sources. |
| `POST /api/admin/users/sources/reset` | `{ids:[positive user ids]}`; success is `200 {reset,imported,updated,skipped}`. | Admin JWT required. IDs are deduplicated; empty effective selection is `400`. Every target must exist and the default namespace must be configured; missing target/default is `404` and the whole batch remains unchanged. All target reconciles commit in one transaction, then each target alone receives `sources_update`; `users_update` refreshes admin summaries. |

Search, explore, remote-book, change-source, Reader content/cache and scheduler consumers now resolve the same
association service. New/read-by-selection operations require caller-active enabled sources, while an existing
caller-owned book may continue resolving its caller-detached snapshot; a foreign source id is treated as missing
before any remote request. Administrator source-count/default/reset/delete and owner-scoped
logical/portable/WebDAV backup/restore consumers are implemented. No API method, body, success schema,
status or error envelope is changed by the remaining Docker work: the dedicated smoke invokes the
stable routes above and compares actual source IDs/lists plus ZIP members before and after restart.
Until the old-global-source fixture, COW, administrator-root/regular-root and restore-isolation checks
pass together, the implemented management/runtime API slices are not sufficient release evidence.

The additive `usedBookNames` projection above is required by the fixed upstream source-manager
`书架书籍` column and is governed by
[`source-manager-fixed-baseline-second-audit-p1-contract.md`](source-manager-fixed-baseline-second-audit-p1-contract.md).
It is deliberately limited to `GET /api/sources`: single-source persistence responses and source
archives remain source configuration records, not bookshelf-data envelopes.

## P2 local-text parser budget

- Existing import paths, request fields and success schemas are unchanged. TXT/`.text`/Markdown direct preview,
  LocalStore and WebDAV preview now consume the same configured decoded-text and final-chapter budgets as confirm.
- A parser-budget failure remains `400` with the existing safe parser-limit message and caller-owned retry
  `importToken`; raw transport overflow remains the existing `413`. Storage batch envelopes keep per-item errors.
- `POST /api/books/:id/refresh-local` returns `400` for the bounded historical input ceiling before any mutation.
  Lazy cache reconstruction treats the same bounded-read failure as an unavailable derived chapter and discloses no
  host path. No route, auth rule, response field, SQLite schema, backup field or WebSocket event was added.

## P1 bookshelf latest-chapter timestamp contract (2026-07-22 extracted)

Existing methods, paths, auth, status codes and error envelopes remain unchanged. Shelf book response objects gain
one additive `lastCheckTime` integer containing Unix milliseconds.

| Path family | Response / side effect | Compatibility rule |
|---|---|---|
| `GET /api/books`, `GET /api/books/:id`, and existing book mutation/import responses | Each shelf-book projection includes non-zero `lastCheckTime`; existing `shelfOrderAt`, `progress`, `createdAt` and `updatedAt` remain present. | `shelfOrderAt` is progress time when read, otherwise insertion time. Generic metadata `updatedAt` neither reorders the shelf nor changes `lastCheckTime`. |
| `POST /api/books/:id/refresh`, scheduled/manual update check | If the fetched catalogue contains more chapters than the persisted pre-refresh count, commit the new chapter data and current `lastCheckTime` together. | Same-size/smaller result, metadata-only edit and failed refresh do not advance `lastCheckTime`; current statuses/errors remain stable. |
| `POST /api/books/:id/change-source` | A successful source replacement commits the newly resolved latest chapter and current `lastCheckTime` while retaining independent reading progress. | Failed source selection does not change either timestamp; existing request/status/error behavior remains stable. |
| Book create/import and legacy/portable restore | New rows initialize the field. A positive reader-dev `lastCheckTime` is retained on restore and later export. | Old payloads may omit it. Invalid/non-positive values fall back to destination insertion time rather than producing an error. |

The frontend latest-chapter label consumes only `lastCheckTime`. It must not infer this display value from
`shelfOrderAt`, progress or generic `updatedAt`. Full rationale and migration rules are in
`bookshelf-last-check-time-p1-contract.md`.

## P2 BookGroup unified projection contract (2026-07-19 extracted)

The fixed reader-dev baseline persists four editable built-in groups together with custom groups. OpenReader keeps
its deployed `/categories` and many-to-many membership APIs, and adds one user-scoped projection instead of
changing existing category response shapes. Full behavior and backup mapping are defined in
`book-group-p2-contract.md`.

| Method / path | Request | Success / side effects | Auth and errors |
|---|---|---|---|
| `GET /api/book-groups` | None. | `200` ordered array containing the four lazily seeded built-ins and every current custom Category. Stable keys are `builtin:all`, `builtin:local`, `builtin:audio`, `builtin:ungrouped`, and `category:<id>`. | JWT/current user only. No cross-user Category may enter the projection. Unexpected persistence failure is `500 {error}`. |
| `PUT /api/book-groups/:key` | Built-in semantic key `all`, `local`, `audio`, or `ungrouped`; body contains at least one of `{name,show}`. | `200` updated built-in projection row. Commit precedes `book_groups_update`. | Unknown key, empty body, or blank normalized name is `400`; no row from another user is addressable. |
| `PUT /api/book-groups/reorder` | `{ "keys": ["builtin:...", "category:<id>", ...] }`, containing the current projection exactly once. | One SQLite transaction assigns the complete mixed order to built-in preferences and Categories, then returns the full ordered projection. After commit it emits `book_groups_update` and the existing `categories_update` compatibility event. | Missing, duplicate, extra, malformed, or foreign token is `400` and rolls back every order write; persistence failure is `500`. |

Existing category create/update/delete/reorder paths retain their current request and response contracts. Their
post-commit sync side effects additionally invalidate or broadcast the unified projection. New Categories append
after the maximum order across both data sources; the old custom-only reorder endpoint remains compatible for old
clients but is not used by the rebuilt mixed manager.

## P2 embedded chapter-image cache contract (implementation in progress)

Reader-dev downloads embedded chapter images while caching text and reuses those files during EPUB export. OpenReader keeps the existing authenticated chapter path and adds an optional browser-safe mapping rather than changing persisted text or text-position semantics.

| Method / path | Request | Success / side effects | Auth and errors |
|---|---|---|---|
| `GET /api/books/:id/chapters/:index/content` | Existing path and Bearer JWT. | Existing `chapter`, `content`, and `format` remain unchanged. For a remote text chapter with verified cached images, optional `cachedImages` maps normalized original HTTP(S) URLs to `/api/chapter-image/<capability>` and optional `cachedImagesExpiresAt` is RFC3339. Text cache writes retain original URLs. A failed image fetch never fails readable chapter text. | Existing ownership and chapter errors remain unchanged. Image-cache absence/corruption omits the optional mapping; it does not become a chapter `401`, `404`, or `502`. |
| `GET|HEAD /api/chapter-image/:capability` | One opaque path capability; no JWT header, query token, source URL, or filesystem path. | Serves one verified raster blob with its detected MIME, exact `Content-Length`, `X-Content-Type-Options: nosniff`, same-origin/no-referrer policy, and private short-lived cache headers. `HEAD` returns identical relevant headers and no body. | Malformed, invalid, expired, wrong-purpose, or unsafe capability: `403`; deleted book/owner/source or missing blob: `404`; unexpected storage/DB failure: `500`. Errors are stable JSON and never expose token/path/source credentials. |
| Existing cache/clear/delete/refresh/source-change routes | Existing request and response fields. | Successful text cache work also attempts bounded image caching. Image failures do not alter chapter success/failure counters. Cache clearing, book/user deletion, catalogue replacement, refresh, and source change remove that book's derived image references after the database transaction commits. | Existing auth/status behavior remains stable. A cleanup filesystem failure is never permission to roll back or corrupt committed user rows, and must not delete another user's root. |
| Existing EPUB export | Existing authenticated export request. | Includes only already-cached verified blobs below `OEBPS/Images/`, adds OPF image manifest entries, and rewrites matching chapter image elements. It performs no export-time network fetch. Missing images do not fail export and never place capabilities or host paths in the archive. | Existing export error envelope remains unchanged. |

Capabilities are HMAC purpose-separated from login/EPUB/CBZ/audio tokens and bind user ID, book ID, source ID, blob key, fingerprint, and expiry. The resource handler rechecks the current database owner/source and file fingerprint on every open. Access logs redact the entire capability segment.

## P2 parser structured-error contract (P2-Parser-2A implemented)

Status: implemented and API-tested on 2026-07-13. Reader-dev has no equivalent REST envelope, so OpenReader preserves deployed transport semantics while making parser failures machine-readable and safe. This remains independent from the separately implemented P2-Parser-1G persistent-variable migration.

| Path family | Existing stable behavior | Additive contract |
|---|---|---|
| `GET /api/books/:id/chapters/:index/content` | Remote failure remains `502 {"error":"failed to load chapter content"}`. | Implemented optional `code` (`source_rule_invalid`, `source_rule_unsupported`, `source_request_failed`, `content_unavailable`) and `stage: "content"`. |
| `POST /api/search` (single paged source), `GET /api/explore/:sourceId`, `POST /api/books/remote`, source change/refresh | Existing status and top-level `error` remain stable. | Implemented stable `error` text plus optional `code`/`stage` (`search`, `explore`, `book_info`); raw Go/source-request detail is never serialized. |
| `/api/sources/:id/test*`, the additive canonical debug stream, and batch test | Existing authenticated test `200` shapes include result payload plus `error`/`message`; the canonical stream uses ordered stage/log/end/error events. | Optional `code`/`stage` remain. Debug messages never include variable values, rule source, request URL query, response body, cookie, authorization header, JWT, WebDAV secret or filesystem path. Debug/test requests never write the invalid-source cache; batch health remains the explicit diagnostic writer. |

### Shared source/RSS remote-request failure boundary (P2-N1)

The routes and successful response bodies remain unchanged. Search, Explore, remote BookInfo/TOC/content,
source test, RSS source page/content and remote source JSON import all pass through the shared request boundary
defined in [`shared-source-fetcher-p2-contract.md`](shared-source-fetcher-p2-contract.md): HTTP(S)-only absolute
URLs without userinfo, 15-second default total request timeout, 16-MiB response cap, five redirects and at most
three retries. Same-origin redirects retain source headers; cross-origin redirects retain only safe negotiation
headers and never Cookie, Authorization, Proxy-Authorization or custom source credentials.

| Endpoint family | Existing status / shape retained | Bounded failure behavior |
|---|---|---|
| `POST /api/search`, `GET /api/explore/:sourceId`, remote BookInfo/TOC/content and Reader chapter content | Existing status, top-level `error`, optional `code`/`stage` and source-failure classification remain authoritative. | Unsafe URL, response limit, redirect limit and timeout are source-request failures, but public payloads contain only stable path/query/header/body/proxy-credential-free messages. Caller cancellation remains cancellation and is not cached as a failed source. |
| `POST /api/sources/:id/test*`, `POST /api/sources/batch-test` | Existing HTTP 200 diagnostic envelope remains. | The safe `error`/`message` and optional `code`/`stage` identify request failure without echoing the raw URL or source credentials. |
| `POST /api/sources/remote-preview`, `POST /api/sources/remote` | Existing JWT/edit permission, 200 success shape and 400 failure shape remain. | Malformed/unsafe/oversized/redirect-limited fetches return the existing generic `{"error":"failed to fetch remote source URL"}`; remote response bytes never reach JSON decoding after the cap. |
| `POST /api/rss/sources/:id/refresh`, `GET /api/rss/articles/:id/content` | Existing authenticated source/article ownership, requested-page semantics, success payload and current 400 failure status remain. | Fetch diagnostics are redacted before concatenation; no URL query, userinfo, header/cookie, body or proxy credential enters the `error` field or persisted RSS article/source state. |

P2-N1 does not change SQLite, backup, WebSocket or frontend request schemas. The separate P2-N2 contract is now
implemented, regression-validated and Docker-published. It adds only the deployment variable
`OPENREADER_SOURCE_NETWORK_ALLOWLIST` (comma-separated exact hostname, bare IP or CIDR; empty means public-only),
fails startup on invalid non-empty entries, and keeps all business API paths/status/success/error schemas unchanged.
Unsafe private/DNS/proxy targets become the existing `source_request_failed` family without exposing the target,
DNS answer, allowlist or proxy credentials. Engine/config/API focused tests, the full Go suite, Engine race,
frontend 706/706, production build, Docker public/LAN/loopback/restart and fresh/historical volume gates pass without
changing a business response schema. P2-N2 was published locally to GHCR as `d198c2e` / `latest`; the verified OCI
index is `sha256:021817e602aa589c1583ec7ccb65828172c1a2afe1e038e23651dd51c455fcc1`.
P2-N1 was implemented in `981bca7` and published locally to GHCR as `981bca7` / `latest`; the verified OCI
index is `sha256:02160e0797b3371fdfadccb550b8766d412c3e09df632ba1e36d192b26eb500d`.

`code` and `stage` are optional additive fields. Legacy frontend paths continue to use `error`; no parser error becomes an authentication failure. Normal source operations may classify `engine.IsSourceRequestError` for their existing failure policy, but source debug/test requests are now explicitly read-only and never enter `source_failures`; the separate batch health action owns diagnostic failure writes. The completed second-audit implementation and release evidence are in [`source-debug-fixed-baseline-second-audit-p2-contract.md`](source-debug-fixed-baseline-second-audit-p2-contract.md).

## P2 source-debug second-audit API contract (2026-08-09 implemented)

The fixed baseline saves the current editor source and starts one SSE debugger which automatically runs search or
direct-entry dispatch, BookInfo, TOC and first-chapter content while carrying the BookInfo/TOC parser runtime and
the adjacent chapter boundary. Its `Debugger.infoDebug` deliberately re-enters by `bookUrl`, so a search/explore
result's own variable is not carried into BookInfo; the contract preserves that exact call graph. The current three
unrelated REST probes are not an upstream-equivalent state machine.

OpenReader retains `POST /api/sources/:id/test`, `/test-chapter` and `/test-content` as deployed-client shims with
their existing bodies, authenticated `200` debug envelopes, optional `code/stage`, validation and owner-scoped
`404`. It adds `POST /api/sources/:id/debug/stream` as the canonical Bearer-JWT streaming translation. The stream
defaults a blank keyword to “我的”, dispatches absolute URL / `::` / `++` / `--` exactly as the fixed baseline,
uses ordered bounded `log`/`stage` events and exactly one `end` or `error`, and stops on request cancellation.

Neither the canonical stream nor any legacy `/test*` probe may mutate the invalid-source cache. Source persistence
occurs before the stream through the existing create/update APIs and therefore keeps `CanEditSources`, user
association, copy-on-write and post-commit sync behavior. Full request/response/status/side-effect/error fields and
planned tests are fixed in
[`source-debug-fixed-baseline-second-audit-p2-contract.md`](source-debug-fixed-baseline-second-audit-p2-contract.md).

Implementation status: the canonical stream, cancellation, bounded/redacted events, exact dispatch/runtime
boundaries and zero failure-cache side effects are implemented. Focused race, full Go/vet, frontend 724/724,
production build, four-viewport real-browser validation and fresh/historical volume gates pass. The locally built
amd64/arm64 image is published as `f8f263d`/`latest`, OCI index
`sha256:9c83821de9e5f4df223b6e69a6d67eff512fa55d4a271f544718ccad8ae58ba1`.

## P2 parser persistent-variable contract (P2-Parser-1G implemented)

| Path / payload | Additive behavior | Compatibility and safety |
| --- | --- | --- |
| Search / explore result | Result objects may include opaque `variable` JSON. | Existing consumers may ignore it. The frontend forwards it only through the existing add-remote-book payload; it is never rendered as HTML or interpreted by JavaScript. |
| `POST /api/books/remote` | Optional `variable` accepts the bounded JSON string map and seeds BookInfo/TOC parsing. Returned book keeps normal shape with optional `variable`. | Omitted values remain empty. Malformed/non-string/oversized maps return existing-style `400` with safe `error`/`code`/`stage`, before a remote request. |
| Remote refresh/change-source and chapter content | The server reads/writes optional Book/Chapter variables around existing parser calls. Chapter content stores the returned Book/Chapter map atomically with its cache path. | Existing paths and successful response bodies do not change. A source semantics change clears obsolete state rather than translating or exposing it. |
| Backup restore | `bookshelf.json.variable` and optional `chapterVariables.json` are accepted. | Old archives need neither field. New maps are fully validated before restore mutation and target only the authenticated destination user's source-name-resolved book/chapters; source/book/chapter database IDs are never variable identity. |
