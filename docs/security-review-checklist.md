# OpenReader Security Review Checklist

Use this checklist for security-sensitive changes and release reviews.

## P2 trusted proxy and rate-limit identity (2026-08-25 implementation)

- [x] Direct deployments trust only the TCP peer address; caller-controlled `X-Forwarded-For` and `X-Real-IP`
  cannot select a new rate-limit bucket or forge the access-log client identity.
- [x] Reverse-proxy deployments require an explicit `OPENREADER_TRUSTED_PROXIES` IP/CIDR list. Gin's validated
  right-to-left chain algorithm remains the single identity source for both the limiter and logger.
- [x] Empty list items, malformed IPs/CIDRs and wildcard shorthand fail before directories, SQLite, scheduler,
  backup, middleware registration or listen. The default Compose deployment needs no configuration change.
- [x] Existing 429 JSON, route exemptions, query/capability redaction, CORS, JWT, WebDAV, source proxy and mounted
  data contracts remain unchanged.

Evidence: contract `30b7630`, old-implementation red tests `db89593`, implementation `f5b3869`, focused/full/
race/vet, frontend 741/741, production build, Compose and README variable checks, real-process default/trusted/invalid
probes, and GitHub Actions `32828470325` fresh/historical/portable/restart gates. The published amd64/arm64 index is
`sha256:6a2fc83bf79426e93423b1dd5756c8ea49b716d1321441d5c194efff9c03b066`. See
[`docs/compat/trusted-proxy-client-identity-rate-limit-fixed-baseline-second-audit-p2-contract.md`](compat/trusted-proxy-client-identity-rate-limit-fixed-baseline-second-audit-p2-contract.md).
Status is `aligned / regression-validated / Docker-published / awaiting-device-verification`.

## Authentication and authorization

- [ ] `OPENREADER_JWT_SECRET` is required and not logged.
- [ ] Protected endpoints require valid JWT.
- [ ] Admin endpoints check admin role.
- [ ] User-owned rows are scoped by authenticated user ID.
- [ ] Batch operations cannot affect another user’s data.

### P2 Bookmark write boundary (2026-08-12 implementation)

- [x] Single create/update bodies accept exactly one actual-read-bounded 64 KiB JSON object; batch create uses
  16 MiB/2,000 raw rows and batch delete uses 16 KiB/2,000 raw IDs after owner target resolution.
- [x] Malformed, multi-JSON, overflow, cardinality and invalid note patches fail before per-row queries, mutation,
  timestamp changes or sync events, without reflecting excerpt/note/JWT/database content.
- [x] Note edits require an explicit string and use only an owner-scoped note column update. A concurrently deleted
  target cannot be reinserted by GORM `Save` fallback, and newer immutable location/context values cannot be lost.
- [x] The frontend edit action sends only `{note}`; create/import keeps the published DTO and all-row transaction,
  chapter ownership, user/book isolation, backup and historical-row behavior.

Target contract:
[`docs/compat/bookmark-write-boundary-fixed-baseline-second-audit-p2-contract.md`](compat/bookmark-write-boundary-fixed-baseline-second-audit-p2-contract.md).
Status is `implemented / regression-validated / Docker-published / awaiting-device-verification`. Contract/red-test/
implementation ordering, focused race, full Go/vet, frontend 740/740, production build, Reader browser payload,
host/local/pullback HTTP plus SQLite-trigger probes, and sequential fresh/historical volume gates passed. The locally
built `a9a55db`/`latest` amd64/arm64 OCI index is
`sha256:944a85881170bc900c1fda0acb885bedc1dc4b17ed4e635305988163e1b635e5`.

### P2 BookSource write/import boundary (2026-08-12 implementation)

- [x] Source create/update accept exactly one actual-read-bounded 16 MiB JSON object; batch and remote URL
  controls must use a 16 KiB boundary after JWT and `CanEditSources` checks.
- [x] Local and remote source JSON stops at 5,000 raw entries before normalization, database work or response;
  remote bytes continue through the existing SSRF-safe 16 MiB fetch boundary.
- [x] Positive `sourceLimit` atomically caps future caller-active associations across concurrent create/import,
  while zero remains unlimited and historical/default/restore data is never truncated or deleted.
- [x] Every rejected/no-op request preserves source associations, COW snapshots, failure cache, parser variables,
  timestamps and sync events; errors must not expose rules, credentials, URLs with queries or SQLite details.

Target contract and current runtime counterexamples are recorded in
[`docs/compat/book-source-write-boundary-fixed-baseline-second-audit-p2-contract.md`](compat/book-source-write-boundary-fixed-baseline-second-audit-p2-contract.md).
Status is `implemented / regression-validated / Docker-published / awaiting-device-verification`. Focused race,
full Go/vet, real HTTP declared/chunked/cardinality/quota probes, fresh/historical/ownership/portable container gates
and GHCR pullback passed for `d9ddc0f`; OCI index is
`sha256:548bf0984e7fa5039411bd75f9ae8ac8496052010255bfe746bf36fa9336dc8f`.

### P2 user-setting write boundary (2026-08-12 implementation)

- [x] Auth and legal setting-key checks run before reading a setting value; legal `reader/shelf/search` PUT bodies
  accept exactly one actual-read-bounded 8 MiB JSON value for declared and chunked transport.
- [x] Overflow, malformed, second JSON and garbage fail before setting query/upsert/event, do not change the prior
  row or timestamp, and never place setting/JWT/private asset data in errors or logs.
- [x] Existing large rows remain readable/exportable/restorable without migration; normal CAS/force/concurrent upsert
  and current-user isolation stay unchanged.

Evidence: `backend/api/user_setting_write_boundary_contract_test.go`, focused/full/race/vet, frontend 740/740,
production build and isolated real declared/chunked/exact-limit HTTP. `c2bc736` also produced a local arm64 candidate;
fresh/historical volume and remote publication remain pending after the OrbStack socket approval reviewer disconnected.
[`docs/compat/user-setting-write-boundary-fixed-baseline-second-audit-p2-contract.md`](compat/user-setting-write-boundary-fixed-baseline-second-audit-p2-contract.md).

### P2 admin user write boundary (2026-08-12 implementation)

- [x] Five administrator JSON user mutations authenticate the administrator before reading credentials, then enforce
  one 16 KiB actual-read-bounded JSON value for declared and chunked bodies.
- [x] Batch user deletion/source reset accept at most 2,000 raw IDs and retain existing dedupe, protected-user,
  transaction, workspace-root and post-commit event boundaries.
- [x] Every newly written password is at least 8 UTF-16 code units and at most 72 UTF-8 bytes; rejected input is
  handled before bcrypt/SQLite and never enters errors, logs, events or backups. Existing hashes remain loginable.

Evidence: `backend/api/admin_user_write_boundary_contract_test.go`, focused/full/race/vet, frontend 740/740,
production build, real declared/chunked HTTP smoke and sequential fresh/historical volume gates. `6c1c6db` was built
locally for amd64/arm64 and published as version/latest OCI index
`sha256:55326ed147aea4370c0161d75568fe85a5095abb6dad6b487856dfeea09832a2`. See
[`docs/compat/admin-user-write-boundary-fixed-baseline-second-audit-p2-contract.md`](compat/admin-user-write-boundary-fixed-baseline-second-audit-p2-contract.md).

### P2 public auth request boundary (2026-08-12 implementation)

- [x] `POST /api/auth/login` and `/register` enforce the same 16 KiB actual-read limit for declared and chunked
  bodies before DB lookup, bcrypt or durable writes; overflow is a path-free `413` and never logs credentials.
- [x] Exactly one JSON value is accepted. Trailing whitespace is compatible; a second value or garbage is `400`
  with no user/login-time mutation.
- [x] New bcrypt passwords over 72 bytes fail as an explicit `400`, never a library-derived `500`; invalid login
  remains a generic `401`, and historical usernames are not revalidated as new accounts.

Evidence: `backend/api/auth_request_boundary_contract_test.go`,
`scripts/smoke/auth-request-boundary-contract.mjs`, focused/full/race/vet, frontend 740/740 and production build. See
[`docs/compat/auth-request-boundary-fixed-baseline-second-audit-p2-contract.md`](compat/auth-request-boundary-fixed-baseline-second-audit-p2-contract.md).
`f5c15d7` also passed sequential fresh/historical mounted-volume gates and was published locally as the matching
version/latest OCI index `sha256:db667de319ae2721cbd35990896612a738b4570a94920875ea14e2aed613503f`.

## P1 Index authenticated-session isolation (2026-07-27 candidate; browser gate pending)

- [x] Session invalidation suspends or resets Index state before token removal. Visible search/explore rows,
  pagination, loading, scroll, book objects, parser variables and local paths are removed synchronously.
- [x] A suspended scene contains only a minimal non-persistent intent. It can be restored only when the JWT
  subject proves the same user; a different or unknown account discards it and settles canonical Home before
  the authenticated shell is unblocked. Explicit logout never preserves it.
- [x] Search, Explore, sidebar source hydration, route BookInfo, local import and temporary-reader handoff freeze
  authenticated scope/token plus user/workspace generation as applicable. Old callbacks cannot write another
  account's Pinia/preferences, open overlays, show operation feedback or change routes.
- [x] Workspace request stamps include the non-persistent session generation in addition to mode/revision.
  Explore chooser recovery uses a one-shot pending flag, preventing both missed pre-mount recovery and later
  replay after returning from Reader.
- [x] Tokens remain only in short-lived operation closures and are not copied to Pinia, suspended intent, URL,
  local/session storage, events, logs or errors. Existing same-origin `safeReturnTo()` validation remains.
- [x] No API, JWT format, SQLite row, mounted path, backup/WebDAV format or server authorization rule changed.
- [ ] Real Chromium must still exercise delayed Search/Explore/BookInfo callbacks across same- and
  different-account reauthentication at 1440×900, 390×844 and 360×800. Docker release remains gated on it.

Evidence: `frontend/tests/indexWorkspaceState.test.mjs`,
`frontend/tests/authenticatedRuntimeScope.test.mjs`,
`frontend/tests/indexSessionIsolationWiring.test.mjs`,
`frontend/tests/appSidebarSearch.test.mjs`,
`frontend/tests/workspaceContinuationContract.test.mjs`, and
[`docs/compat/index-authenticated-session-p1-contract.md`](compat/index-authenticated-session-p1-contract.md).

## P1 browser-cache account isolation (2026-07-27 implementation; browser gate pending)

- [x] Browser cache statistics read value bytes only after an exact key-shape check proves ownership by the
  scope captured at operation start; other users, anonymous keys, unknown formats and substring collisions fail closed.
- [x] Source/RSS and chapter group clearing receives the captured scope explicitly and deletes only exact
  current-user keys. Book deletion and BookManage cleanup use the same frozen scoped chapter prefix.
- [x] Unscoped upstream-era chapter text is neither read, copied, counted nor removed by an authenticated account.
  It is preserved as unowned rebuildable cache; no first-login owner claim can disclose one account's text to another.
- [x] Server/browser stats, clear confirmations, result messages and busy state use scope+token+generation ownership.
  An old operation cannot commit after another refresh or authentication change.
- [x] No JWT, token or cache payload is logged or persisted by the operation guard; token material remains only in
  a short-lived closure and cache values are never surfaced in labels/errors.
- [ ] Real Chromium must still verify current-user totals, delayed-request retirement and scoped deletion at
  1440×900, 390×844 and 360×800. The focused script exists, but its first external launch request was rejected
  after the approval service disconnected; no alternate launch path was used.

Evidence: `frontend/tests/localCacheStatsScope.test.mjs`,
`frontend/tests/appCacheManagement.test.mjs`,
`frontend/tests/bookChapterCacheScope.test.mjs`, and
[`docs/compat/index-local-cache-scope-p1-contract.md`](compat/index-local-cache-scope-p1-contract.md).

## SSRF and remote fetches

- [x] Shared Source/RSS URLs validate absolute HTTP(S), host, port and userinfo before transport; the
  independently published cover fetcher has its own stricter capability/target policy. WebDAV remote-client
  behavior remains governed by its separate protocol contract.
- [x] Shared Source/RSS redirects are explicitly bounded to five and every redirect URL is revalidated.
- [x] Shared Source/RSS requests have a configurable 15-second safe default timeout and preserve earlier caller cancellation.
- [x] Shared Source/RSS response bodies are bounded to 16 MiB before charset, HTML/XML/JSON or binary processing.
- [x] Shared Source/RSS private, loopback, link-local, metadata and special-use targets are denied by default;
  DNS answers, redirects and actual dials are revalidated, and deployment exceptions require the administrator-only
  exact-host/IP/CIDR allowlist.
- [x] Shared Source/RSS public errors redact URL query, userinfo, headers/cookies, request/response body and
  proxy credentials; cross-origin redirects strip credential-bearing headers.

### P2-N1 shared Source/RSS fetch boundary (2026-08-09 published)

P2-N1 is implemented and published as `981bca7` / `latest`, OCI index
`sha256:02160e0797b3371fdfadccb550b8766d412c3e09df632ba1e36d192b26eb500d`. Focused race tests,
full Go/frontend/build, real CSS/JSONPath/XPath/RSS browser flows and fresh/historical mounted-volume gates passed.

### P2-N2 shared Source/RSS private-network boundary (2026-08-09 published)

P2-N2 is implemented and published as `d198c2e` / `latest`, OCI index
`sha256:021817e602aa589c1583ec7ccb65828172c1a2afe1e038e23651dd51c455fcc1`. The sole administrator variable
`OPENREADER_SOURCE_NETWORK_ALLOWLIST` is fail-closed; direct and explicit HTTP/SOCKS requests validate target and
proxy endpoint independently, reject mixed DNS and rebinding, pin validated IPs at dial/handshake time, and ignore
ambient process proxies. Docker public/host-gateway/loopback/exact-host/restart fixtures plus fresh/historical
mounted-volume, portable backup and restart gates passed. FlClash fake-IP ranges remain denied unless the deployment
administrator explicitly allows them; this is documented rather than silently weakening the default policy.

### P2 source-debug streaming boundary (2026-08-09 published)

- [x] `POST /api/sources/:id/debug/stream` requires Bearer auth and an active current-user source association;
  JWT is never placed in a query parameter, event, local source draft or history record.
- [x] Current source persistence remains a separate permission-checked create/update before streaming. The stream
  and all three legacy `/test*` probes are read-only and never write `source_failures`, cache, shelf, variables or sync events.
- [x] Request context cancellation stops the active remote transport and later stages, emits no fake `end`, and
  does not classify user cancellation as source failure.
- [x] Events are bounded to 128 entries/64 KiB, have strict sequence/elapsed metadata and exactly one terminal;
  errors redact URL query/userinfo, headers/cookies, response body, parser variables, JWT, WebDAV credentials and host paths.
- [x] The shared source fetcher continues to enforce HTTP(S), timeout, body, redirect, DNS/private-network and proxy
  policy. JavaScript/WebView rules remain stored but are not executed and produce a safe unsupported-code event.
- [x] Browser local source/history keys include schema and authenticated account scope; switching accounts cannot
  restore another user's drafts. Generated JSON keeps reader-dev fields plus the same lossless `rules` extension
  used by the backend exporter, without embedding auth material.

Evidence: `backend/api/source_debug_second_audit_contract_test.go`,
`backend/services/sourcedebug/service_test.go`, frontend source-debug/editor/state contracts,
`scripts/smoke/source-debug-contract.mjs`, focused race, full Go/vet, frontend 724/724, production build,
fresh/historical volume gates and locally published `f8f263d`/`latest`.

### P1 temporary remote-Reader session boundary (2026-08-09 published)

- [x] `POST /api/reader/remote-sessions` accepts one JSON value within 64 KiB; declared and chunked oversized bodies fail with a safe 413 before source lookup or transport.
- [x] Session IDs use 32 random bytes and are bound to the authenticated user. Unknown, foreign and LRU-evicted IDs are indistinguishable 404s; natural expiry remains 410 without being confused with JWT expiry.
- [x] Idle lifetime is 30 minutes and absolute lifetime is four hours. Per-session (8 MiB), per-user (8 sessions/32 MiB) and process (128 sessions/128 MiB) retention budgets use deterministic LRU eviction; bounded expiry markers preserve 410 without retaining source snapshots.
- [x] Retained-byte estimation includes the complete server-side source snapshot, book and chapters. Source rules, headers/cookies, login state, proxy credentials and resolved fetch URLs are never serialized to the client.
- [x] Content requests trust only the server catalogue index/URL. Malformed indices do not renew a lease; successful parser variables commit atomically to the book/current chapter while other chapters remain isolated.
- [x] Cancellation produces no synthetic success/error body, variable commit, source-failure row, shelf/cache/database/file write or sync event. Typed transport failures alone enter the caller-scoped short-lived failure cache, and API errors redact raw rules, credentials, response content and URL query/fragment.
- [x] The session remains memory-only and cannot enter SQLite, chapter cache, browser persistence, backup/WebDAV or shelf WebSocket events. Durable controls continue to require an explicit add-to-shelf action.

Evidence: `backend/services/remotereader/store_contract_test.go`,
`backend/api/remote_reader_second_audit_contract_test.go`, the full Go/race/vet gates,
`scripts/smoke/remote-reader-contract.mjs`,
`scripts/smoke/source-parser-workflow-contract.mjs`, and
[`docs/compat/remote-reader-session-fixed-baseline-second-audit-p1-contract.md`](compat/remote-reader-session-fixed-baseline-second-audit-p1-contract.md).
Local amd64/arm64 build, GHCR digest readback and fresh/historical mounted-volume/backup gates passed for `30dbe53`/`latest`, OCI index `sha256:9c07871ef7d3c8d99733fcecea205336576c081db651dada13eaeedafda76365`.

## P2 RSS requested-page and import review (2026-08-09 implementation)

- [x] Every source, article, import update and page cache write is scoped to the
  authenticated user. Same URLs in another account are unrelated, and source
  import commits or rolls back as one transaction before its sync event.
- [x] The visible refresh endpoint accepts only pages `1..100000`; a requested
  `sortUrl` must resolve to the owned source base/sort options and an arbitrary
  outside URL is rejected before transport. Standard feed page greater than one
  performs no network request.
- [x] Batch import is capped at 8 MiB and 5000 records, skips blank identities,
  preserves input order, and submits only the frontend's explicitly selected
  rows. Safe select-all excludes `@js:` and `webView:` records without dropping
  safe index zero.
- [x] Article cache upsert is transactional and preserves hidden read/favourite
  state; visible content is sanitized and a delayed source/page or old-account
  result cannot commit into the current dialog.
- [x] The shared `engine` source fetcher applies the P2-N1 response, redirect,
  timeout, retry, URL and redaction boundaries and the separately published P2-N2
  private-address/DNS/proxy policy. The RSS visible-workspace slice does not own those
  transport contracts, but all RSS fetches consume them.

Evidence: `backend/api/rss_requested_page_contract_test.go`,
`backend/services/rss/service_test.go`, the existing RSS parser/content tests,
frontend RSS contracts, and `scripts/smoke/rss-workspace-contract.mjs` at four
viewports. Full contract: [`docs/compat/rss-visible-workspace-fixed-baseline-second-audit-p2-contract.md`](compat/rss-visible-workspace-fixed-baseline-second-audit-p2-contract.md).

### P2 RSS write/cache concurrency boundary (2026-08-12 implemented and published)

- [x] Source create/update consumes exactly one non-null JSON object with an
  8 MiB actual-read cap; import consumes one non-empty array under its existing
  8 MiB/5,000-item flat-400 contract; article state patch consumes one non-null
  object under 16 KiB and requires at least one explicit non-null boolean.
- [x] Same-user source create/import/update/delete shares a narrow per-user
  transactional mutation boundary so concurrent same-URL requests cannot create
  duplicate active identities or resurrect an ID deleted after precheck. Other users
  remain independent.
- [x] Source/article existing-row writes use caller-scoped explicit columns,
  check zero affected and return fresh rows; no GORM `Save`/upsert fallback may
  overwrite concurrent owner columns or recreate a deleted target.
- [x] Article state owns only supplied `is_read`/`favorite`; refresh owns only
  remote/parser columns; content fetch completion owns only `content`. Source/article
  liveness is rechecked after remote work so delayed results cannot create
  orphan cache rows or revive deleted data.
- [x] Failures and tests do not expose source rules, headers/cookies, URL query,
  article content, JWT, SQLite diagnostics or filesystem paths. No schema/index,
  backup member, startup cleanup or historical-row truncation is allowed.

Contract and test-first evidence:
[`docs/compat/rss-write-boundary-fixed-baseline-second-audit-p2-contract.md`](compat/rss-write-boundary-fixed-baseline-second-audit-p2-contract.md).
Evidence: implementation `5236389`; API/service red-to-green contracts; critical race
`-count=3`, full Go/vet, frontend 740/740, production build, four-viewport RSS UI,
host HTTP + SQLite-trigger and pullback-container public-API probes. Fresh/historical
volume gates passed. Locally published `0986d8e`/`latest` resolve to OCI index
`sha256:9884d0e9b41c1a1a109f3034159ae2968fe9d889da063deaca2514e1ac371e25`.

## P2 remote book-cover proxy review (2026-07-27 implemented and published)

- [x] Public image route accepts only an opaque, purpose-separated, expiring server-issued
  capability; it never accepts a caller-supplied raw URL or login JWT in path/query.
- [x] Capability contents do not reveal the original URL/query, user/source identity or
  credentials and the complete token segment is redacted from access logs.
- [x] HTTP(S)-only URL, DNS and actual dial addresses reject private/loopback/link-local/
  multicast/unspecified/metadata ranges; every redirect is revalidated and capped at three.
- [x] Fetch has a 3-second total timeout and 8-MiB body cap, accepts only verified raster
  magic, rejects non-2xx/HTML/truncated content and never forwards Cookie/Authorization/
  source credentials.
- [x] Per-user cache paths are rooted and symlink-safe; writes are atomic/coalesced, cache
  hits are revalidated, aggregate bytes are bounded and user/global cleanup cannot cross scope.
- [x] Raw `coverUrl` remains the only persisted/exported value. `coverResourceUrl` cannot
  enter SQLite, sync persistence, Book/Chapter variables, WebDAV, exports or backups.
- [x] Public GET/HEAD errors and frontend fallback never expose URL, query, host path or
  credentials and never turn a cover failure into auth invalidation or source suppression.

Target contract and required tests:
[`docs/compat/book-cover-proxy-p2-contract.md`](compat/book-cover-proxy-p2-contract.md).
Evidence: cover service/API/middleware contracts, frontend URL/component contracts, cover-service
race/vet, real Go + Chromium three-viewport smoke, local new/historical volume and portable
backup gates. Locally published `ceb4baa`/`latest` resolve to OCI index
`sha256:c5cace40e21a9b30b4f2f7cdd9219a59ff16525b173bcf79d5994e950ff56fd2`.

## Path traversal and files

- [ ] Every user path is cleaned and joined under an allowed root.
- [ ] Final resolved path is verified to remain under the allowed root.
- [ ] Local store, uploads, cache, backups, and WebDAV all use rooted paths.

### P2 remote chapter-text cache filesystem lifecycle (2026-08-17 implemented/published)

- [x] Remote chapter cache read/stat/remove accepts only relative or current-root absolute identities that resolve to
      regular files below `cache/`; root/ancestor/entry symlinks, directories and special files fail closed.
- [x] Remote chapter cache reads use one same-file-verified handle and the configured source-response byte budget;
      mounted outside bytes, host paths and OS errors never enter chapter responses or cache API errors.
- [x] Remote cache writes use a verified parent, private stage and atomic rename; unsafe targets, cancellation and
      failures do not truncate an existing file, write outside the root, publish a DB path or leave temporary files.
- [x] Stats count only actual safe non-empty chapter files and remain current-user scoped. Clear/prune removes only
      verified regular files after commit and only when an all-user reference query proves them unreferenced.
- [x] File publication plus DB reference and reference-query plus prune share one process serialization boundary, so
      concurrent cache writes and cleanup cannot leave a newly referenced path missing.

Contract `c6f9de8`, old-implementation red tests `f8e5c04`, implementation `75cc238` and release evidence
`3cef8df` landed in order. Focused/race/full/vet, frontend/build, Reader/BookManage/sidebar browser flows,
mounted-host/container probes and fresh/historical/portable gates passed. OCI index
`sha256:8cfe72e56af0cbb191d6b31fa243153a3ce14010614c5153881b262229facf86`; status is
`aligned / regression-validated / Docker-published / awaiting-device-verification`. Full contract:
[`compat/remote-chapter-cache-filesystem-lifecycle-fixed-baseline-second-audit-p2-contract.md`](compat/remote-chapter-cache-filesystem-lifecycle-fixed-baseline-second-audit-p2-contract.md).

### P2 public upload-resource filesystem boundary (2026-08-17 inventory)

- [x] Public `GET|HEAD /uploads/*resourcePath` opens only a regular file beneath `data/uploads`; root, ancestor and
      entry symlinks plus directory/FIFO/device/socket targets fail closed without blocking or object mutation.
- [x] Metadata and response bytes derive from one `Lstat -> open -> SameFile` handle; no Gin static check/reopen
      race or fallback `c.File(path)`/`http.FileServer(http.Dir)` remains.
- [x] Existing unauthenticated legacy/current/portable-restored URLs preserve GET/HEAD, inline Content-Type,
      Content-Length, Last-Modified, conditional and Range behavior; no directory listing or redirect is introduced.
- [x] Missing/unsafe 404 responses and logs expose no upload root, symlink target, OS error, SQLite path, JWT,
      credential or file bytes. Read requests do not create, repair, delete or rewrite mounted objects.

Published `2986357` runtime inventory reproduced both an entry symlink and an ancestor symlink returning 200 with
bytes outside `data/uploads`. Contract `d0c948c`, old-implementation red tests `7181634` and implementation `277e512`
preserve that evidence while replacing Gin static reads with an explicit rooted same-file-open handler. Focused/race,
full Go/vet, frontend 741/741, build, three-viewport Reader assets, local-candidate/GHCR-pulled HTTP probes and
fresh/historical/portable mounted-volume gates pass. Locally published `277e512`/`latest` resolve to OCI index
`sha256:ca50fd59dce4f4bb13a1450ee7ee39b2a3d7b392de3902a7f3c21272e8ac9c70`; status is
`aligned / regression-validated / Docker-published / awaiting-device-verification`. Full contract:
[`compat/upload-public-read-filesystem-boundary-fixed-baseline-second-audit-p2-contract.md`](compat/upload-public-read-filesystem-boundary-fixed-baseline-second-audit-p2-contract.md).

## P2 raw WebDAV protocol review (2026-07-19 implemented; Docker gate pending)

- [x] `/webdav/*` and `/reader3/webdav/*` authenticate before reading path, Destination, Depth or body;
  Bearer JWT and bcrypt-backed Basic both resolve to the same persisted user id and permission check.
- [x] Missing/bad Basic credentials return one generic `401` plus challenge; a valid user without WebDAV
  permission returns `403`. Passwords, Authorization headers, JWTs and lock tokens never enter logs/errors.
- [x] Every existing source/target/parent/COPY descendant is checked with `Lstat`; traversal, symlink,
  cross-root, root deletion and directory-into-descendant operations fail before mutation.
- [x] PROPFIND is read-only: a missing nested directory returns `404` and is not created. DAV XML contains
  only encoded logical hrefs and bounded one-level metadata, never host paths.
- [x] PUT keeps the configured byte cap and atomic same-directory staging. COPY/MOVE validate the full plan
  before replacing a destination, and failure preserves the source and old target.
- [x] Basic is documented for HTTPS-only exposure; upstream's wildcard-origin plus credentialed CORS headers
  are not copied. Existing same-origin Bearer WebDAVBrowser requests remain compatible.

Evidence: WebDAV API/service/CORS contract tests, full Go/frontend/build gates, real Basic curl against production
middleware, and WebDAVBrowser at 1440×900, 390×844 and 360×800. Historical-volume/portable-backup and candidate
container curl remain mandatory before Docker release. Required evidence is fixed in
[`docs/compat/webdav-protocol-p2-contract.md`](compat/webdav-protocol-p2-contract.md): auth/permission,
two-prefix, PROPFIND, mutation/status, symlink, LOCK, browser regression, curl and mounted-volume tests.
- [x] Backup list/download only expose caller-root, same-file-verified regular `backup_*.zip` and
      `portable_backup_*.zip`; symlink, directory, special and prefix-only non-ZIP entries fail closed.
- [x] API errors do not leak host filesystem paths.

### P2 backup list/download filesystem boundary (2026-08-17 implemented/published)

- [x] JWT and effective WebDAV permission resolve the caller root before stat/open; regular users cannot list or
      download an administrator or another user's same-name backup.
- [x] Root/ancestor/entry symlinks, directories, FIFO/device/socket and non-ZIP names are hidden from list and
      unavailable from direct download without deleting or rewriting the mounted object.
- [x] Metadata, portable format inspection and response bytes derive from one scoped `Lstat -> open -> SameFile`
      handle; no check-then-`c.File(path)` race remains.
- [x] Missing roots keep `200 []`; invalid/missing/unsafe download responses and logs expose no root, symlink target,
      OS/ZIP detail, JWT or WebDAV credential.

Runtime inventory on published `cd3a17c` reproduced a regular-user `backup_escape.zip` symlink returning 200 with
caller-root-external bytes, a prefix-only non-ZIP returning 200, and a directory returning 301. Contract `b9deec2`
and old-implementation red tests `d7810ca` precede scoped same-file-open implementation `2986357`; focused/race,
full Go/vet, frontend 741/741, build, real HTTP and fresh/historical/portable mounted-volume gates pass. Locally built
`2986357`/`latest` resolve to OCI index
`sha256:bdb8195077000a898569e0f3f6664a5760c2b56058d67b2d6ae1d4aaf42fea5e`. Status is
`aligned / regression-validated / Docker-published / awaiting-device-verification`. Full contract:
[`compat/backup-list-download-filesystem-request-boundary-fixed-baseline-second-audit-p2-contract.md`](compat/backup-list-download-filesystem-request-boundary-fixed-baseline-second-audit-p2-contract.md).

## P2 reading-progress CAS and WebDAV mirror review (2026-07-18)

- [x] Progress GET/PUT resolves the book through `(authenticated user_id, book_id)` and resolves
  the canonical chapter through that book before any write. A supplied chapter ID cannot select a
  chapter from another book or user; negative/missing catalogue positions fail before persistence.
- [x] Existing-row progress writes use an `id + updated_at` conditional update and first writes use
  the existing `(user_id,book_id)` unique index. A losing concurrent request reloads the committed
  winner and cannot emit a second WebSocket event.
- [x] Live `bookProgress` output is attempted only after the database commit and only when the
  caller's existing WebDAV feature directory is enabled. Administrators retain the historical root;
  regular users remain under `webdav/users/<safe-username>`.
- [x] The WebDAV root and feature directory are resolved and checked; a feature-directory symlink,
  non-directory or resolved path outside the caller root fails closed. Output uses a sanitized
  filename, same-directory temporary file, bounded JSON fields and atomic rename.
- [x] A mirror failure returns only a path-free diagnostic header, never a host path, credential or
  token. It cannot roll back or falsify the already committed SQLite progress.
- [x] Real dual-client browser checks pass at 1440×900, 390×844 and 360×800 with one CAS
  winner, one conflict, both active readers converged, a clean-context restore and the WebDAV file
  matching the SQLite winner. Remote application no longer echoes an additional progress write.
- [x] Full Go tests, 474 frontend tests, production build, Reader text/mobile/continuous, shelf
  multiclient and real EPUB/CBZ browser gates pass on the implementation commit candidate.
- [x] Fresh-volume/portable restore and historical TXT/EPUB/UMD/CBZ/relative-cache/owner-isolation
  Docker gates pass; the locally built amd64/arm64 image is published as `9f19d21` and `latest`.

Targeted evidence: `backend/api/progress_p2_contract_test.go`,
`frontend/tests/readerProgressPersistence.test.mjs`, `frontend/tests/readerRouteSync.test.mjs` and
`scripts/smoke/reader-progress-multiclient-contract.mjs`. Release evidence will be appended after
the remaining gates pass.

### Reading-progress request boundary second audit (2026-08-16 implemented)

- [x] JWT rejection precedes declared/actual body admission; unauthorized oversized, malformed or multi-value input
      is not read and cannot reveal progress shape or book existence.
- [x] `PUT /api/progress` accepts one non-null UTF-8 JSON object within 16 KiB. Declared/chunked overflow is stable
      413; malformed, invalid UTF-8, wrong-shape and trailing JSON are stable 400 before service work.
- [x] `bookId` and `chapterIndex` are explicit; non-empty conflict timestamps are bounded valid RFC3339Nano.
      Persisted `mode` and event-reflected `clientId` have 20/128-byte limits without truncation or secret echo.
- [x] Every wire/field rejection leaves SQLite, shelf order, WebDAV mirror and Hub queue unchanged. Existing CAS,
      conflict 200, chapter canonicalization, mirror failure and user isolation tests remain green.
- [x] A stale/oversized session client ID is regenerated in the browser so a bad local value cannot permanently
      prevent progress synchronization; pending local position and account-generation isolation remain intact.

Evidence: `backend/api/reading_progress_request_boundary_contract_test.go`,
`frontend/tests/readerProgressRequestBoundary.test.mjs`, `scripts/smoke/reader-progress-multiclient-contract.mjs`,
focused/full/race/vet, frontend 741/741, production build and three real Chromium viewports. The local `1563bc3`
candidate build passed; the later `65199f6` release passed fresh/historical/portable mounted-volume gates and forced
arm64 revision verification, then published `65199f6`/`latest` at OCI index
`sha256:57eda43d437d98a4f2d748164d58c5816f3ff3dc199397bd9dc8f6d48334a8cb`. Full contract:
[`compat/reading-progress-request-boundary-fixed-baseline-second-audit-p2-contract.md`](compat/reading-progress-request-boundary-fixed-baseline-second-audit-p2-contract.md).

### Book control request boundary second audit (2026-08-16 implemented/published)

- [x] Six authenticated `books.go` JSON control routes enforce actual-read 16 KiB/32 KiB/1 MiB limits, one non-null UTF-8
      object, auth/target-first priority and their stable modern or reader3 error envelope.
- [x] Batch/export accept at most 200 raw unique positive owner book IDs; batch Category IDs stop at 200 before
      dedupe/query/transaction, and existing 50-book cache/100-book clear-cache limits remain.
- [x] Local refresh decodes the optional body and 16 KiB TOC rule before reading/staging its caller-owned archive;
      rejected input cannot mutate Book/Chapter, TOC/cache files, progress or events.
- [x] Remote add/source change validate bounded caller fields/categories/variables before fetch and use request context.
      Cancellation writes no failure row, book/chapter/candidate/cache state or completion event.
- [x] Batch cache/export cancellation stops later books/chapters without rolling back already durable cache work or
      truncating successful complete exports; legacy content search keeps raw whitespace, bounded controls and 200.

Evidence: contract/correction `097c862`/`669aa5b`, red tests `5cc4b18`, implementation `65199f6`, focused/full/race/vet,
frontend 741/741, production build, real-Go three-viewport browser contracts, fresh/historical/portable gates and forced
GHCR arm64 revision verification. Published OCI index:
`sha256:57eda43d437d98a4f2d748164d58c5816f3ff3dc199397bd9dc8f6d48334a8cb`. Full contract:
[`compat/book-control-request-boundary-fixed-baseline-second-audit-p2-contract.md`](compat/book-control-request-boundary-fixed-baseline-second-audit-p2-contract.md).

### ReplaceRule request boundary second audit (2026-08-16 implemented/published)

- [x] Five authenticated ReplaceRule JSON routes enforce their existing 512 KiB/16 MiB/128 KiB/4 MiB limits by
      actual read, accept exactly one non-null UTF-8 object/array and map true overflow to stable 413.
- [x] Trailing JSON/garbage, invalid UTF-8, null/wrong shape and over-cardinality fail before rule execution,
      SQLite mutation or Hub broadcast; PUT preserves auth/path/owner target-first priority.
- [x] Batch upsert/delete bind GORM and their transactions to request context; pre-commit cancellation rolls back
      every row and emits no event, while a durable commit retains the existing one convergence event.
- [x] Existing exact strings/defaults/name-upsert/order/skipped/deletedIds, RE2/match/output budgets, schema, backup and
      visible manager/Reader behavior remain unchanged.

Evidence: contract `ff6d7e3`, old-implementation red tests `c70f04e`, implementation `9f5a52b`, focused/full/race/vet,
frontend 741/741, production build, real HTTP, four-view browser and fresh/historical/portable volume gates. The locally
built amd64/arm64 `9f5a52b`/`latest` release resolves to OCI index
`sha256:7a72f2d01b26d1d28c35bb13970cb64a1f7dbf97ddebc3aa704957f58f2f56c3`. Docker CLI forced arm64 pull was
blocked by local `osxkeychain -50`; read-only GHCR Registry config inspection confirmed `architecture=arm64` and full
revision `9f5a52b3ea4da8ca557653052c5190d8023dfa61`. Status is
`aligned / regression-validated / Docker-published / awaiting-device-verification`.

## P2 backup-upload multipart request-boundary review (2026-08-25 implemented/published)

- [x] Keep JWT/activity/effective WebDAV permission before multipart parsing; unauthorized/forbidden requests create
      no temp, restore stage, row, asset or event.
- [x] Enforce the existing compressed-limit-plus-1-MiB declared/actual body envelope before restore allocation.
- [x] Accept exactly one file part named `file`, zero scalar/other file parts, and a non-empty UTF-8 `.zip` filename
      of at most 255 bytes. Reject ambiguity before stage/preflight/transaction.
- [x] Register handler-owned `multipart.Form.RemoveAll()` for every non-nil parsed form on success, invalid shape,
      wrong extension, oversized file, stage failure and restore failure; never log temp paths or submitted names.
- [x] Preserve compressed/entry/expanded ZIP budgets, path/symlink/duplicate checks, portable hashes/collisions,
      caller/source ownership, one-transaction restore and safe 400/409/413/500 envelopes.
- [x] Pass red/green shape and forced-disk cleanup tests, focused race/full/vet, real declared/chunked multipart probe,
      frontend/build, WebDAV restore three-view and fresh/historical/portable/restart gates before local publication.

The `7045827` overlay probe proved that a valid ZIP plus scalar and 34 MiB extra file still restores a Book and leaves
the parsed temp after direct handler return. Full contract:
[`compat/backup-restore-multipart-request-boundary-fixed-baseline-second-audit-p2-contract.md`](compat/backup-restore-multipart-request-boundary-fixed-baseline-second-audit-p2-contract.md).
Contract `7a2a44a`, old-implementation red tests `20ac551` and implementation `a0fb1bd` landed in order. Focused/
full/race/vet, frontend 741/741, production build, real Go HTTP shape/envelope/temp validation, WebDAV restore at
1440/390/360 and fresh/historical/portable/restart gates pass. The locally built `a0fb1bd`/`latest` release resolves
to OCI index `sha256:b25f5b05df983532bf656ec8647e553188db3ba7fb291b826cb45b65deae6f3c`.
Status: `aligned / regression-validated / Docker-published / awaiting-device-verification`.

## P2 local-book archive rooted lifecycle review (2026-08-24 implemented/published)

- [x] Resolve every imported-book owner/book root from trusted `LibraryDir`; do not accept a resolved owner root that
      has escaped through a mounted root/ancestor symlink.
- [x] Preserve a historical book-directory alias only when it resolves inside that same verified owner root and all
      source/cache/write/delete consumers remain bound to the resolved real directory identity.
- [x] Open source, chapter cache and original export as the same rooted regular file identity that passed path/type
      validation; deterministic replacement must not redirect bytes.
- [x] Keep `refresh-local` stage, metadata promotion and stale-content pruning under the verified book root; unsafe
      roots/ancestors/entries must fail before DB, event or active-generation changes.
- [x] Detach a last-reference archive identity-safely before recursive cleanup; a symlink or replacement must never
      delete its target outside `library/`, while durable Book deletion remains committed.
- [x] Preserve legal historical path rebase, archive bytes, progress/bookmarks, TXT/EPUB/UMD/CBZ, relative cache,
      portable backup and startup migration without an eager scan or destructive migration.
- [x] Pass focused/race/full/vet, real mounted owner/book/content/metadata symlink and replacement probes, frontend
      full/build, Reader/BookManage three-view smoke and fresh/historical/portable/restart volume gates before release.

The `/tmp` real HTTP probe on `OpenReader@20ba211` proved out-of-root refresh/read/write and post-delete directory
removal. Required evidence is defined in
[`compat/local-book-archive-filesystem-lifecycle-fixed-baseline-second-audit-p2-contract.md`](compat/local-book-archive-filesystem-lifecycle-fixed-baseline-second-audit-p2-contract.md).
Evidence: contract `cae9bf2`, alias correction `852df65`, red tests `92a5fa4`, implementation `125fd93`, focused/full/
race/vet, frontend 741/741, build, post-fix real HTTP, Reader/BookManage three-view and fresh/historical/portable/restart
volume gates. The locally built amd64/arm64 `125fd93`/`latest` release resolves to OCI index
`sha256:777ca720981b8a3529009211ce179b430bb354cb01e2957681f191036699f6a5`. Status:
`aligned / regression-validated / Docker-published / awaiting-device-verification`.

### Backup generation request lifecycle second audit (2026-08-17 implemented/published)

- [x] `POST /api/backup/trigger` maps every internal DB/ZIP/OS failure to fixed `500 {"error":"backup failed"}`;
      no mounted path, SQL, ZIP detail, source/archive path or credential appears in response or ordinary logs.
- [x] Logical and portable HTTP generation propagate request context through lock wait, DB reads and bounded
      archive/asset copies; a canceled waiter never starts after the active generator releases the lock.
- [x] Cancellation before final rename closes/removes the private temp and creates no list-visible package; successful
      rename is the durable boundary and is not compensation-deleted after a later disconnect.
- [x] Existing auth/WebDAV permission priority, caller roots, logical/portable formats, typed 409/413, output budgets,
      same-name collision protection and scheduled backup behavior remain unchanged.

Contract `05def84`, red tests `f9d2aff` and implementation `cd3a17c` passed focused/full/race/vet, frontend 741/741,
build, real HTTP safe-500/success/128 MiB cancel and fresh/historical/portable gates. The local amd64/arm64 release
resolves to OCI index `sha256:08e9a5ba94646e5955e9c0d4586a4be95d004d6a015b518331c02748a9e53f70`.
Status is `aligned / regression-validated / Docker-published / awaiting-device-verification`. Full contract:
[`compat/backup-generation-request-boundary-fixed-baseline-second-audit-p2-contract.md`](compat/backup-generation-request-boundary-fixed-baseline-second-audit-p2-contract.md).

## Uploads and archive formats

- [ ] File size limits are enforced before expensive parsing.
- [ ] File extension/MIME assumptions are not trusted alone.
- [ ] EPUB/ZIP entries reject absolute paths and `..` traversal.
- [ ] Decompressed size and file count are bounded.
- [ ] Temporary staged import files are per user and cleaned after success/expiry.

## Parser DoS

- [ ] TXT/PDF/UMD/Markdown parsers avoid unbounded memory growth.
- [ ] Regex rules cannot trigger catastrophic work on large content without guardrails.
- [ ] Source pagination has a stop condition.
- [ ] A bad source cannot block unrelated searches indefinitely.

## P2 invalid-source cache follow-up

- [x] Failure records are scoped by authenticated user and source ID; a global source failure never leaks to another user.
- [x] Cached error messages are bounded and client-safe: no JWT, cookie, authorization header, WebDAV credential, full URL query, response body or host path is stored or returned.
- [x] Expiry, source edit and source delete make stale rows ineligible before normal-source suppression or failed-only UI rendering.
- [x] Client cancellation and empty source results do not create a cache entry that could suppress a healthy source.

Evidence: `backend/api/source_failure_contract_test.go`, `frontend/tests/sourceFailureCacheContract.test.mjs`, and three-viewport `scripts/smoke/source-workspace-contract.mjs`.

## P2 online-source evaluator review (partial)

- [x] CSS, JSONPath, XPath and regex evaluation for search, explore, details, TOC and content continue to use the existing bounded source-request path; a parser error cannot enter the user-scoped invalid-source cache and suppress a source that may be repaired locally.
- [x] JavaScript/WebJS rules are preserved but fail explicitly with `ErrUnsupportedSourceRule` at details, TOC and content as well as search; no user-supplied script executes in the Go process, browser, filesystem or server network context.
- [x] Rule-level `##regex##replacement[##first]` transforms compile with Go RE2 after selector evaluation; invalid patterns are `ErrInvalidSourceRule`, and neither invalid nor unsupported local rules are written to `source_failures`, including source-manager test endpoints.
- [x] `@put:`/`@get:` use a bounded JSON string-map runtime with key/value/count/byte/depth limits, cloned search-result and multi-branch maps, and literal-only reads. Reader-dev-compatible Book/Chapter state persists only through the same validator, is rejected before a remote request when malformed, and never grants cookie, filesystem or server-network access. `{{ }}` remains `ErrUnsupportedSourceRule`.
- [x] The raw `//` XPath shorthand is recognized, while an ordinary relative URL is never reinterpreted as XPath.
- [x] A single next-content URL is compared to the adjacent catalog chapter after final-URL normalization; a matching URL stops the current chapter before any next-chapter request. Empty text content rules fail locally without page-cache or failure-cache writes.
- [ ] Broader rule size/chain limits remain in a later parser slice. `{{...}}` remains outside the bounded runtime and must stay disabled without an isolated JS sandbox.
- [x] Parser-facing API errors preserve existing status/top-level `error` while adding safe `code`/`stage`; remote request, rule and unavailable-content errors never disclose raw rule, variable, URL query, credential, JWT, response body or filesystem path.
- [x] Persistent parser variables use additive `books.variable`/`chapters.variable` columns, bounded JSON validation, transactional source-semantic clearing, user-scoped restore and a portable `chapterVariables.json` backup artifact. Parser errors and API error payloads never echo values.

Evidence: `backend/engine/source_rule_evaluator_test.go`, `backend/api/source_failure_contract_test.go`, `backend/api/source_error_contract_test.go`, and the full backend suite required before release.

## Release note

For each release, record which checklist sections were relevant and which tests/probes covered them.

## P2 BookInfo assets and follow-state review

- [x] `POST /uploads` derives the owner only from the authenticated JWT and writes all
  new covers/backgrounds/fonts/misc assets below `data/uploads/users/<user-id>/`.
- [x] `PUT /books/:id` accepts a new custom cover only from the same user's `covers`
  subtree; cross-user, escaped, missing and new external URLs fail closed. Existing
  legacy/database values remain readable without a bulk move or migration.
- [x] `DELETE /uploads` parses an exact user-scoped path, returns a safe non-owner
  result, retains legacy global paths, and refuses to delete a Book or reader-setting
  reference. The Reader saves the unreferenced setting before its delete request and
  restores the local font/background state if that save fails.
- [x] Upload size and extension allowlists remain compatible; Reader appearance P2-A
  additionally checks bounded image/font signatures before publishing a file. Rooted path
  checks reject traversal, query and fragment variants without leaking a filesystem path.

Evidence: `backend/api/bookinfo_asset_contract_test.go`, upload/update API tests,
`frontend/tests/overlayBookInfo.test.mjs`,
`frontend/tests/readerAppearanceAssets.test.mjs`, and the P2 BookInfo real-browser
contract (three viewports).

## P2 Reader appearance asset runtime review

第二轮 HTTP wire/multipart 生命周期复审见
[`compat/user-asset-write-boundary-fixed-baseline-second-audit-p2-contract.md`](compat/user-asset-write-boundary-fixed-baseline-second-audit-p2-contract.md)。
当前状态为 **aligned / regression-validated / Docker-published / awaiting-device-verification**：33 MiB
actual-read 包络、单一 file/type、显式 multipart 临时文件清理和 16 KiB 单 JSON 删除已关闭原先 Gin
先完整解析再做 8/32 MiB 文件检查的资源缺口。以下内容/路径/事务结论继续有效。

- [x] Declared and chunked multipart bodies share the 33 MiB actual-read envelope; JWT rejection happens before
  body parsing, while authenticated overflow returns the stable path-free 413.
- [x] Exactly one file and at most one bounded type are accepted. Ambiguous parts and oversized metadata fail
  before final write, and every successfully parsed form releases its `multipart-*` temporary storage on all exits.
- [x] Asset deletion accepts one 16 KiB JSON object plus whitespace. Overflow, a second document or trailing
  garbage cannot reach reference checks or delete the file.
- [x] Host, local candidate and GHCR-pulled HTTP probes verify upload/read/rejected-delete/final-delete bytes and
  temp-file cleanup; fresh and traced historical mounted-volume/portable gates pass before local publication.

- [x] Cover/background/font/misc uploads still enforce the existing 8 MiB/32 MiB
  admission caps before content inspection and continue to derive the destination owner
  from the authenticated JWT only.
- [x] JPEG/PNG/GIF metadata is decoded through a 1 MiB bounded header reader; WebP uses
  a bounded RIFF/chunk/dimension parser. Empty, truncated, extension-mismatched and
  HTML/random-byte payloads fail with a path-free `400` before the final user directory
  or file is created.
- [x] Raster width/height and aggregate pixel count are bounded before a directly served
  browser asset is published. TTF/OTF/WOFF/WOFF2 extensions must match their container
  signature; uploaded bytes are never executed or parsed as application code.
- [x] Reader upload success now requires the returned `reader` setting to contain the
  exact new URL. Network/CAS failure restores only the still-current local attempt and
  best-effort removes the unique unreferenced upload; it never overwrites a server-winning
  state.
- [x] Reader removal persists the reference change before physical deletion. `409`
  means another current-user setting/book still references the file and is treated as
  safe retention; legacy/global/external URLs are never auto-deleted.
- [x] Real Go + Chromium upload/reload/failed-save cleanup/delete runs pass at
  1440×900, 390×844 and 360×800; the affected BookInfo flow and Reader
  desktop/mobile/iPad matrix also pass. Frontend 558/558, full Go tests and production
  build are green.
- [x] The local `9cae206` candidate passed new and historical mounted-volume,
  restart, backup/portable restore, local-format and owner-isolation smoke. Locally
  built amd64/arm64 `9cae206` and `latest` indexes were published and independently
  resolved to `sha256:800cff1326caa8740f343cc233f7ffcd87ef38b38f744b47d1bc7712c27dc7c6`.
- [x] Logical/portable-v1 backups still contain URL strings only, while portable v2 uses
  the separately versioned, bounded, cross-user-ID asset byte packaging/remapping
  contract in `docs/compat/portable-appearance-assets-p2b-contract.md`. Its failure
  tests, runtime, real-browser restore, fresh/historical volume gates and local
  amd64/arm64 release are complete.

Targeted evidence: `backend/api/reader_appearance_assets_p2_contract_test.go`,
`backend/api/bookinfo_asset_contract_test.go`,
`frontend/tests/readerAppearanceAssets.test.mjs`, and
`scripts/smoke/reader-appearance-assets-real-api-contract.mjs`,
`docs/compat/reader-appearance-assets-p2-contract.md`.

## P1-E2 workspace storage audit

- [x] Raw `/webdav/*` now uses the normal Bearer JWT and activity middleware before it can reach a filesystem handler.
- [x] `User.CanAccessStore` is enforced before LocalStore, WebDAV, backup and import/restore handlers inspect a path, body or file. Store-disabled users receive 403.
- [x] LocalStore, WebDAV and generated backups resolve to private descendants for regular users while the administrator retains the preserved legacy root. Cross-user access is denied without moving/deleting mounted data.
- [x] Direct and storage-backed preview/import uses user-scoped random staged tokens; confirmation consumes the staged bytes, foreign/expired tokens fail closed, and successful/expired stages are removed.
- [x] Direct local-book upload, LocalStore/WebDAV upload, preview and confirmation reads are capped by `OPENREADER_MAX_IMPORT_BYTES` (128 MiB by default) before staging or parser work. LocalStore/WebDAV writes stage beside the target and rename only after the bounded copy succeeds.
- [x] Archive entry/expanded-size、UMD/PDF parser work 和跨用户 stage cleanup 已有显式上限；cleanup
  在启动时执行并每小时重复，不依赖下一次用户请求。
- [x] TXT/Markdown 已把原始输入、解码后文本、自定义规则长度和最终章节数接入统一 parser limits；
  prepared snapshot 采用通用章节上限，历史本地归档刷新/缓存重建也在完整分配前执行 1GiB legacy
  input ceiling。parser 错误不消费重试 stage、不产生书籍行或暴露宿主路径。证据见
  `docs/compat/local-text-parser-budget-p2-contract.md` 及对应 engine/importer/API/config tests。

P2 本地 parser 预算已由本机发布为 `e7f168e` / `latest`，OCI index
`sha256:8d64bbb187f65c433388bddc5385ce68d42e8b40d9b397787e4c1d354c892dac`；三视口真实导入和
fresh/historical mounted-volume/portable-backup 门均通过。

Evidence for the checked items: `backend/api/workspace_storage_access_contract_test.go`, `backend/api/workspace_import_stage_contract_test.go`, `backend/api/import_size_contract_test.go`, `frontend/tests/webdavAuthContract.test.mjs`, full Go/frontend test suites and production frontend build. This remains not a storage/backup release approval.

## P1-E3 workspace file-manager follow-up

- [x] LocalStore multi-file upload retains per-file basename validation, private-root resolution, size limits and same-directory atomic replacement; a rejected part does not truncate an existing destination or disclose its host path.
- [x] Removing visible directory/rename/download/recursive controls does not remove guarded legacy API compatibility routes or weaken the existing raw WebDAV `MKCOL`/`MOVE` path checks.
- [x] Workbench suffix gating is presentation-only: disallowed current UI formats do not bypass P1-E1 scoped preview tokens, and retained direct parser support does not expand filesystem access.
- [x] Browser regression proves LocalStore/WebDAV requests retain bearer auth and no hidden mobile control can invoke a removed operation.

第二轮 LocalStore 文件系统与 HTTP wire 复审见
[`compat/local-store-filesystem-request-boundary-fixed-baseline-second-audit-p2-contract.md`](compat/local-store-filesystem-request-boundary-fixed-baseline-second-audit-p2-contract.md)。
当前状态为 **aligned / Docker-published / awaiting-device-verification**：合同、旧实现红测、实现和真实
HTTP 探针已按 `69145fc`、`8c78775`、`bba99e1`、`930be4d` 顺序落地，并随 `65a9870` 完成卷门与发布。

- [x] Multi-file upload has one aggregate actual-read envelope, bounded part cardinality/metadata and explicit
  handler-owned multipart temporary-file cleanup; authentication and `canAccessStore` still run first.
- [x] Directory/rename/import JSON actions accept one bounded document; import request and recursive expansion
  cardinality fail before stage, database, cache or sync side effects.
- [x] Every LocalStore action rejects root/ancestor/target symlinks and special files. Open/download/import recheck
  an opened regular file, and no read/write/delete can escape the current user's resolved root.
- [x] Host/candidate HTTP, focused race, fresh/historical mounted-volume and portable restore probes cover these boundaries
  without moving or deleting pre-existing symlinks or mounted user data.

Evidence: `backend/api/local_store_filesystem_request_boundary_contract_test.go` and
`scripts/smoke/local-store-filesystem-request-boundary-contract.mjs` cover the wire/filesystem boundary; focused/full/
race/vet, frontend 740/740, build, host/candidate runtime and fresh/historical/portable mounted-volume gates pass.
`65a9870`/`latest` is published at OCI index
`sha256:255c81b43dbb7f49c707d6c609b920aa183b730401ad1c1ca32157eb0a945c71`.

## P1/P2 direct local-book multipart and workflow review (2026-08-16 implemented)

- [x] Direct preview/import/compat-alias apply the same saturating
      `maxLocalImportBytes + 1 MiB` actual-read envelope to declared and chunked multipart after JWT, before any
      `PostForm`, `FormFile`, stage, parser, database or archive work.
- [x] The stable single-object API accepts exactly one `file` or one caller-scoped `importToken`; duplicate,
      mixed, extra file/value parts, unknown fields and over-cardinality categories fail before side effects.
- [x] Filename/title/author/rule/category metadata are valid UTF-8 and byte/cardinality bounded. File bytes keep
      the independent configured import limit; token-only reparse/import cannot access a browser or mounted source.
- [x] Every successfully parsed multipart form is handler-cleaned on success and every failure branch. Cleanup cannot
      delete the immutable stage, a successful library archive or another user's derived cache.
- [x] Direct browser multi-select validates at most 64 visible TXT/EPUB/UMD/CBZ files before network, previews one at
      a time, keeps duplicate filenames as separate stable rows, and reuses the shared LocalStore/WebDAV confirmation
      state machine rather than a second business flow.
- [x] Cancellation/session invalidation aborts current requests and suppresses stale UI/shelf writes. A parser failure
      may retain only its scoped retry token; shape/wire rejection creates no token, Book, category relation, archive
      or sync event.

Formal red tests, implementation, focused/full/race/vet, frontend 737/737, build, real HTTP temporary-file probes and
three-viewport browser flow pass as `cd8f073`/`05343ec`/`3b9ae54`. Fresh/historical/portable mounted-volume gates,
candidate HTTP/browser probes and GHCR pullback revision also pass; `429444a`/`latest` is published at OCI index
`sha256:41f430a5fbf944b9a1dcf25aec6c9f6e92a11a3ff75e395d1a73120da5a6f4d5`. See [`compat/direct-local-book-import-multipart-workflow-fixed-baseline-second-audit-p1-contract.md`](compat/direct-local-book-import-multipart-workflow-fixed-baseline-second-audit-p1-contract.md).

第二轮 WebDAV mounted import/restore 复审见
[`compat/webdav-import-restore-filesystem-request-boundary-fixed-baseline-second-audit-p2-contract.md`](compat/webdav-import-restore-filesystem-request-boundary-fixed-baseline-second-audit-p2-contract.md)。
当前状态为 **aligned / Docker-published / awaiting-device-verification**；合同 `cf46e22`、旧实现红测
`1bb904a` 与实现/runtime `616a076` 已推送，并随 `65a9870` 完成卷门与发布；原生 DAV 协议和 archive
transaction 不重开。

- [x] WebDAV preview/import and WebDAV-path restore accept one actual-read-bounded JSON document after JWT and
  effective WebDAV permission; raw and expanded target cardinality fails before stage, DB, cache or event work.
- [x] Every source-backed import item is opened through the caller-rooted file service. A selected directory cannot
  introduce a nested symlink/FIFO/device/socket into parser input, and no response or log discloses a host path.
- [x] WebDAV-path restore copies one opened regular ZIP into caller-private bounded cache before archive work; mounted
  source rename/delete/replace cannot change the restore bytes and the source is never modified.
- [x] Focused/full/race/vet, declared/chunked host HTTP and three-viewport WebDAV workflow prove the request and
  mounted-read boundary before Docker publication.
- [x] Fresh/historical logical/portable mounted-volume gates prove non-destructive upgrade and restore before Docker
  publication.

## P1-E4 TXT empty-catalogue follow-up

- [x] A valid staged TXT with an unmatched user TOC rule is no longer misclassified as a parser or transport failure. The response retains only the opaque caller-scoped stage token and returns no mounted path, parser internals, credentials or source bytes.
- [x] Confirmation consumes only that caller's staged file, archives the original safely, and creates a zero-chapter local book without fabricating a chapter or dereferencing a missing final chapter. Foreign/expired-token rejection remains covered by the existing stage-token contract.
- [x] Direct, LocalStore and WebDAV UI keep the empty catalogue distinct from an actual parser failure, so a user can retry against the immutable staged data or deliberately confirm the upstream-compatible zero-chapter state.

Evidence: `backend/services/localbook/importer_test.go`, `backend/api/api_test.go`, `backend/api/workspace_import_stage_contract_test.go`, `frontend/tests/overlayBookImport.test.mjs`, `frontend/tests/storageImportWorkflow.test.mjs`, and `scripts/smoke/local-book-import-contract.mjs` / `workspace-storage-retry-contract.mjs` at desktop and both mobile viewports.

## P2 import parser and staged-preview follow-up

- [x] Initial EPUB parsing now validates ZIP paths/symlinks/duplicates/count/per-entry/expanded-size before local import work; every archive-member read is bounded.
- [x] Initial CBZ parsing retains its existing safe checks while using the same local-import limit policy.
- [x] E4-CBZ-1 derives its first image only from the bounded/normalized archive walk and returns a short-lived CBZ capability at serialization time. It does not persist a capability, ZIP member path, raw archive path, or JWT in SQLite, archive metadata, backup/WebDAV data, sync payload storage, or logs; malformed/missing archives degrade to an empty cover without failing the bookshelf response. Evidence: `TestDirectCBZImportAndResourceCapability`, `TestParseCBZKeepsFirstArchiveImageAsCoverSeparateFromSortedCatalogue`, full backend tests and the Docker volume/backup smoke for this release.
- [x] CBZ fixed-baseline runtime extracts only supported image media below a private
  `.cbz-resources/<sha256>/` generation after normalized-path, symlink, duplicate,
  file/directory-conflict, entry-count, per-entry and aggregate expansion checks. Activation is an
  atomic same-directory rename with a complete marker; no partial tree is served.
- [x] A CBZ capability remains scoped to one user/book/fingerprint and cannot select another
  generation or arbitrary host path. Source replacement invalidates old capabilities; temporary
  source absence may expose only an already complete signed generation. GET/HEAD/Range stream the
  allowlisted derived file and never log the capability or disclose a filesystem path.

Evidence: `backend/services/cbzreader/service_test.go` covers atomic activation, conflict rejection,
warm no-rehash selection, one-time recovery, source absence and source replacement invalidation;
`backend/api.TestDirectCBZImportAndResourceCapability` covers import preparation, GET/HEAD/Range,
security headers, unsupported paths and stale capabilities; full Go/frontend suites and real-Go
`scripts/smoke/reader-cbz-contract.mjs` pass at 1440×900, 390×844 and 360×800.
- [x] Standard reader-dev UMD uses a bounded `#`/`$` section reader: signature/type, section/additional lengths, segment count, offsets/titles, zlib output and total decoded text are validated before archive/database writes. Image, malformed and corrupt zlib UMD inputs fail closed; the legacy OpenReader-only prefix is isolated to its existing fallback.
- [x] Expired and orphaned preview tokens are cleaned from every user directory at startup and hourly, without touching active previews or any mounted source/library data.
- [x] Backup ZIP restore now receives a separately tested compressed/entry/expanded-size budget; it remains a distinct compatibility slice from parser/stage handling.

Evidence: `backend/engine/import_limits_contract_test.go`, `backend/engine/umd_parser_contract_test.go`, `backend/services/localbook/importer_test.go`, `backend/api/workspace_import_stage_contract_test.go`, `backend/api/umd_import_contract_test.go`, `backend/config/config_test.go`, and full `go test ./...`. Docker mounted-volume/backup validation remains required before this slice is released.

## P0 parsed local-import snapshot lifecycle (2026-07-18)

- [x] A successful local-book preview writes an optional versioned
  `<token>.parsed.json` only below the existing authenticated user's
  `cache/import-previews/<user-id>/` directory. The token remains a validated
  192-bit random hex basename; no request field can select another path or
  user's directory.
- [x] The snapshot is plain JSON data with no executable/type-polymorphic
  decoder. Its raw file size, chapter count and aggregate
  title/content/resource string bytes are bounded before save and after load.
  Limit arithmetic saturates instead of wrapping for extreme environment
  values.
- [x] The snapshot records its format version, normalized extension, exact TOC
  rule and SHA-256 of the immutable staged `.book`. A mismatched snapshot is
  never consumed; the bounded parser reconstructs it from the caller's own raw
  stage. Malformed or over-limit derived snapshots are removed.
- [x] Snapshot replacement uses a `0600` temporary file and same-directory
  atomic rename. A failed parse cannot replace the last successful snapshot.
  Expiry, successful confirmation and explicit token removal delete `.book`,
  metadata and parsed snapshot together; aged interrupted temporary files are
  confined to and cleaned from the stage directory.
- [x] Confirmation retains existing EPUB/CBZ archive limits, TXT/UMD/PDF
  parser bounds and user-scoped library path construction. It does not trust
  MIME type, expose a host path, log a token, or broaden LocalStore/WebDAV
  access. A failed database transaction compensates by removing only the newly
  allocated durable archive directory.

Evidence: `backend/api/api_test.go`,
`backend/api/workspace_import_stage_contract_test.go`,
`backend/services/localbook/importer_test.go`,
`frontend/tests/overlayBookImport.test.mjs`, and
`scripts/smoke/local-book-import-contract.mjs` at 1440x900, 390x844 and
360x800.

## EPUB catalogue/prepared-extraction performance review (2026-07-18)

- [x] Catalogue-only preview validates every central-directory path, duplicate, symlink, entry count,
  per-entry size and total expanded size before trusting OPF/NAV/NCX metadata; skipping body materialization
  must not skip archive-bomb validation.
- [x] A new prepared extraction is written only below the caller-owned newly allocated library archive, via a
  sibling temporary directory and atomic rename. Failed import compensation cannot select or remove an old book,
  mounted LocalStore/WebDAV source, or another user's directory.
- [x] The extraction marker fast path accepts only a valid SHA-256 fingerprint and exact regular-source
  size/mtime match. Any mismatch, corrupt marker, missing resource or source replacement falls back to bounded
  hashing/rebuild and invalidates capabilities for the old archive identity.
- [x] Catalogue-only and legacy full-content parsed snapshots share the existing owner/token/rule/source-hash
  checks and deserialization bounds. Empty EPUB body fields are never interpreted as authority to read a request
  path or another user's source.
- [x] One-chapter EPUB text recovery uses only normalized persisted archive paths/fragments below the verified
  extraction root, remains bounded by document/text limits, and never logs a capability, stage token, host path
  or EPUB body.
- [x] The real-browser gate must not print the WebSocket login JWT. `/ws/sync?token=...` remains a transport
  compatibility path, but access logging renders its entire query as `<redacted>` while leaving the actual
  request available to authentication middleware.

Evidence: `backend/engine/parser_test.go`, `backend/services/localbook/importer_test.go`,
`backend/services/epubreader/resource_runtime_test.go`, `backend/api/api_test.go`,
`backend/middleware/access_log_test.go`, full backend tests, both three-viewport EPUB/import browser smokes, and
the local `HISTORICAL_VOLUME=1` Docker volume/portable-backup smoke. Archive-policy failures are returned through
a client-safe parse error while host storage failures remain generic server errors.

### Fixed EPUB href catalogue correction

- [x] A TOC-only chapter is accepted only when its canonical path matches a manifest item whose media type is
  XHTML/HTML-compatible; an arbitrary NAV/NCX href cannot make a non-manifest ZIP entry readable.
- [x] Manifest and TOC paths pass the existing NUL/backslash/absolute/drive/`..` normalization. Central-directory
  duplicate, symlink, entry-count, per-entry and total-expanded limits still run before any TOC-only title read.
- [x] TOC-only title fallback uses `readEPUBZipFile` with `MaxArchiveEntryBytes`; the fixed href dedupe does not
  add network access, public archive URLs, host-path errors or unbounded body materialization.
- [x] Historical fragment capabilities remain scoped to their signed user/book/fingerprint/path. New rows leave
  fragment fields empty; resource-aware progress/bookmark reconciliation compares normalized metadata only and
  never opens a filesystem path.
- [x] The legacy pure-`toc`/no-TOC fallback runs only while recovering an existing row with missing metadata,
  reuses the bounded local EPUB parser, selects a normalized manifest/spine resource at the same numeric index,
  and neither broadens archive access nor changes new import/refresh catalogues.

Evidence: fixed EPUB engine/import/API contracts, full Go tests, 426 frontend tests, production build, both
three-viewport EPUB/import browser smokes, and the local `HISTORICAL_VOLUME=1` Docker gate covering rejected
empty-TOC replacement preservation, explicit spine refresh, owner isolation and portable backup/restore/restart.

## P2 backup restore follow-up

- [x] Multipart and WebDAV backup restore enforce one compressed input bound before an allocation or restore mutation.
- [x] ZIP member paths, duplicate canonical names, count, per-member bytes and cumulative expanded bytes are validated before restore dispatch; restore dispatch receives only the bounded preflight data.
- [x] Backend accepts only normalized `.zip` WebDAV restore targets and does not disclose a mounted path or archive member in client errors.
- [x] Structural archive failure has no user-data mutation.
- [x] Supported JSON decode or database-write failure rolls back the complete logical restore and emits no sync event.
- [x] A caller without `canEditSources` cannot mutate the global source table through backup restore; personal restore reports the skipped source artifact explicitly.
- [x] Logical backup generation propagates query/encode/ZIP/close/rename failures and never exposes a partial final ZIP.
- [x] Fixed upstream `bookmark.json` and `replaceRule.json` plus old OpenReader plural aliases are fixture-tested without duplicate execution.

Current automated evidence covers archive structure/bounds, typed-content predecode, SQLite rollback, permission isolation, atomic generation and alias fixtures. Remaining browser and release gates are specified in
`docs/compat/backup-restore-fixed-baseline-p2-contract.md`; mounted-volume Docker smoke remains mandatory.

## P1-E4 portable local archive backup review

- [x] Portable packages are explicitly versioned archives. New triggers generate v2; existing v1 remains restorable, and ordinary reader-dev/Legado/OpenReader backup ZIPs do not gain `library/`/appearance bytes or change their meaning.
- [x] The archive reader rejects unsafe/duplicate/case-conflicting names, directories, symlinks, unknown logical entries, invalid manifest slots, unbounded compressed/member/total sizes and bad SHA-256 before logical restore dispatch.
- [x] Original archives are streamed to a caller-private staging root, checked against the manifest, parsed under the portable per-entry budget, and never derive a destination path from an archive member or stored host path.
- [x] Trigger and restore are caller scoped. A matching `local://` identity with a different/missing destination archive is a `409` before mutation; an identical existing archive is reused, so a package cannot overwrite another user's or an unrelated same-identity book.
- [x] Type=1 local audio directories and missing/unsafe originals fail generation rather than being silently omitted. No JWT, WebDAV credential, archive member or host filesystem path appears in the API error or manifest.

Evidence: `backend/services/backup/portable_test.go`, `backend/api/portable_backup_contract_test.go`, full backend suite, and `HISTORICAL_VOLUME=1 scripts/docker-volume-backup-smoke.sh` export/upload/fresh-volume/restart coverage.

## P2-B portable appearance asset review

- [x] V2 exports only exact current-user managed URLs referenced by Reader settings or
  `Book.customCoverUrl`; missing, cross-owner, symlinked, oversized or magic-mismatched files
  fail without a final package or path disclosure.
- [x] Asset slots contain no source user ID/name/file name. Restore strictly validates the
  canonical manifest, unknown fields/future versions, declared entries, size/hash/magic,
  duplicate digest and placeholder closure before mutation.
- [x] Target files use target-user random paths and no-overwrite promotion. A database failure
  removes newly promoted files; a startup journal removes only crash-window files that are not
  referenced by committed rows.
- [x] V1 and ordinary logical ZIPs never interpret `openreader-asset://`; legacy asset URLs remain
  strings and are reported rather than silently presented as portable bytes.

Evidence: `backend/services/backup/portable_assets_test.go`,
`backend/api/portable_appearance_assets_p2b_contract_test.go`, full Go/frontend/build gates, and
the three-viewport real Go + Chromium portable asset smoke. Fresh and historical Docker volume
execution passed, and local amd64/arm64 tags `54a528f`/`latest` were published at OCI index
`sha256:047f9636a78604a1d5320da2972d0b16256b95d47253320b79095eaf6101a571`.

## P2 replace-rule review

- [x] Reader-global replacement rules remain user-scoped for list, create, update, batch upsert, delete, preview and content application.
- [x] New and edited regex patterns compile before persistence or preview; malformed regex returns a client-safe `400` and is never silently reinterpreted as a literal replacement.
- [x] The global Reader rule path uses Go's RE2 engine with a bounded pattern (16 KiB), replacement (64 KiB), capture count (32), match count (20,000 per rule/chapter) and output (`max(input, 64 MiB)`), avoiding catastrophic backtracking, unbounded match-index collections and `$``/`$'` output amplification.
- [x] The hidden compatibility test route caps the HTTP body at 4 MiB, decoded text at 1 MiB and result at 8 MiB. Execution overflow returns a client-safe `400`; Reader overflow preserves the complete input to that rule, keeps prior rule results and stops later rules instead of returning partial or truncated content.
- [x] Single mutation bodies are capped at 512 KiB; batch upsert is capped at 16 MiB/2,000 rows; batch delete is capped at 128 KiB/2,000 IDs; the hidden group field is capped at 800 bytes. Backup restore accepts at most 2,000 rows and prevalidates the complete non-empty set with the same field/RE2/capture limits before its first write.
- [x] Existing invalid stored regexes fail closed for the remaining reader pipeline; they never produce a literal replacement that could silently corrupt chapter content.
- [x] Reader-global rules use a dedicated execution path, so source-parser replacement semantics are not broadened or weakened by this UI compatibility change.
- [x] Error responses contain field/regex validation messages only; they do not expose a chapter cache path, JWT, source headers, WebDAV credentials, or database content.

Evidence: `backend/services/replacerules/engine_test.go`, `backend/api/replace_rules_contract_test.go`, `backend/api/api_test.go` replace-rule/content cases, `frontend/tests/readerSelectedTextActions.test.mjs`, `frontend/tests/overlayReplaceRules.test.mjs`, and the full Go/frontend validation gates.

## P2 bookmark review

- [x] Every list/create/batch/delete route scopes both the bookmark and its containing book to the authenticated user; a supplied chapter is checked against that same book before it is stored.
- [x] New reader bookmarks require a bounded non-empty paragraph context, and title/context/note fields have server-side size limits before SQLite writes.
- [x] Batch creation validates every item before opening the write transaction, preventing a malformed/foreign chapter row from leaving a partial set of bookmarks.
- [x] Restore never trusts the source database's bookmark or chapter IDs: destination-book ownership is resolved by the scoped URL/title lookup and the chapter is rebound only under that destination book.
- [x] Backup restore's modern creation-time identity prevents same-location records from collapsing while its ID is not reused across users or databases.
- [x] Client-visible validation messages expose only bookmark field/ownership errors, not database IDs beyond the requested resource, host paths, JWTs, or source credentials.

Evidence: `backend/api/bookmarks_contract_test.go`, existing bookmark ownership API tests, `frontend/tests/readerBookmarkContext.test.mjs`, `frontend/tests/readerBookmarkActions.test.mjs`, and `frontend/tests/readerSearchNavigation.test.mjs`. Archive read/expanded-size limits remain a separate open storage hardening item.

## P2 Reader book-content search review

- [x] Both modern and legacy content-search routes require JWT; the modern route verifies the requested book belongs to that authenticated user before loading chapter rows or source content.
- [x] Remote search uses the request context through source fetching, so a disconnected client stops before later chapter requests are scheduled; no cancellation state is serialized as a false successful page.
- [x] Chapter and result scanning remain bounded. The explicit 2,000-match per-chapter cap is returned as `truncated/incomplete` rather than silently advancing a cursor past omitted data.
- [x] Remote chapter/source failures are counted as client-safe `unavailableChapters`, allowing the UI to distinguish an incomplete scan from a genuine no-match result without exposing source URLs, cache paths, headers, cookies, JWTs, or filesystem details.
- [x] Existing engine timeout, redirect, response-size, scheme, and source-header protections remain the only remote-fetch path; the search handler does not introduce a new HTTP client or bypass source validation.

Evidence: `backend/api/content_search_contract_test.go`, existing legacy/modern search API tests, `frontend/tests/readerBookSearch.test.mjs`, `frontend/tests/readerGlobalDialogContract.test.mjs`, and the full Go/frontend validation gates. Three-viewport browser confirmation remains required before release.

## P1-D4 cache stream review

- [x] `POST /api/books/:id/cache/stream` remains behind the normal Bearer-token middleware and verifies the requested book belongs to that authenticated user before opening an SSE response.
- [x] The browser uses an authenticated `fetch` header; no JWT, source header, cookie, cache path, or host path is placed in an SSE query parameter or event payload.
- [x] Explicit cache windows retain `count <= 300`; authenticated BookManage whole-book requests are deliberately unbounded by chapter count but execute sequentially, remain request-cancellable, and retain source timeout/redirect/body-size controls. Batch limits remain unchanged.
- [x] Request cancellation propagates through context-aware chapter/pagination fetching and stops scheduling later chapters. Already written bounded cache files remain normal cache data and no terminal shelf broadcast is emitted for a cancelled stream.
- [x] Errors sent after stream opening are client-safe generic text. Authorization/validation failures happen as ordinary JSON before an event stream opens.

Evidence: `backend/api/cache_stream_contract_test.go`, `frontend/tests/bookCacheStream.test.mjs`, `frontend/tests/overlayBookManagement.test.mjs`, full backend tests and frontend build/test gate. The later whole-book follow-up below supersedes the old pending browser note.

### 2026-07-18 whole-book cache follow-up

- [x] Owner, local-book and missing-source checks finish before SSE headers are opened. Missing/foreign data cannot create a background job or partial cross-user cache state.
- [x] Existing cache files are counted only after rooted reads prove they exist and are non-empty. Missing/empty references are cleared under the same book before refetch; no absolute path is returned.
- [x] Canonical progress and all-failure events contain counts and fixed client text only. Raw parser/network errors, source headers, cookies, JWT, WebDAV credentials and host paths are never serialized.
- [x] Frontend job keys include the authenticated scope; logout aborts server controllers, marks browser queues cancelled and clears the in-memory registry before removing credentials. No controller, token or response body is persisted.
- [x] The whole-book browser/API smoke verifies target-only cancellation and no request on cancelled deletion confirmation at all three required viewports.
- [x] Embedded chapter images use bounded HTTP(S), per-hop DNS/redirect validation, exact-origin credential
      forwarding, cross-origin private-address rejection, image count/byte/time limits and a raster MIME allowlist.
      Blob/reference files are rooted by user/book, manifests are atomic, stale/failed refreshes preserve old valid
      references, and read-only lookups never recreate removed roots. Short-lived purpose-separated HMAC capabilities
      revalidate book/source/fingerprint/MIME/path, while cleanup and EPUB export remain offline and user-scoped.

Evidence: `backend/api/cache_stream_contract_test.go`, whole-catalogue cases in `backend/api/api_test.go`,
`backend/services/chapterimage/service_contract_test.go`, `backend/api/chapter_image_contract_test.go`,
`frontend/tests/overlayBookManagement.test.mjs`, `scripts/smoke/book-management-dialog-contract.mjs`,
`scripts/smoke/reader-image-contract.mjs`, full Go/frontend gates, and the `32dc616` historical-volume/portable-backup
Docker gate. The remaining security difference from upstream is intentional private-network/cross-origin hardening.

## EPUB iframe/resource review

Apply this section to Reader P0 EPUB work:

- [x] The iframe URL never contains the login JWT or Authorization header value.
- [x] The EPUB resource capability is signed with a purpose-separated key and is scoped to user, book, source fingerprint, read-only purpose, and expiry.
- [x] Capability comparison/signature verification is constant-time through the standard crypto library.
- [x] Invalid, expired, modified, stale-version, deleted-book, or ownership-changed capabilities fail closed.
- [x] Capability path segments are redacted from application logs and never returned in error text.
- [x] Every resource path is decoded once, normalized as a POSIX archive path, and verified below the scoped extraction root.
- [x] ZIP entries reject absolute paths, drive prefixes, NUL bytes, `..`, symlinks, duplicate/conflicting paths, and writes through existing symlinks.
- [x] Entry count, per-entry expanded size, and total expanded size are bounded before/during extraction.
- [x] Extraction uses a staging directory and only exposes an atomically completed version.
- [x] XHTML is served without EPUB-authored active scripts; the reader bridge is injected dynamically rather than written into the archived source.
- [x] A title-less, image-only first spine resource is retained as the upstream-compatible cover chapter, but it is still served only through the same per-user, signed EPUB capability and XHTML/media allowlist; import never exposes an archive path or raw ZIP member directly.
- [x] CSP blocks remote network loads and untrusted scripts while allowing scoped local CSS/images/fonts and required inline reader styles.
- [x] MIME types are allowlisted and responses set `nosniff` and `no-referrer`.
- [x] Multi-user tests prove one user's capability cannot read another user's book or resource tree.
- [x] Parent `message` handlers verify both the active iframe window and expected same-origin resource origin.
- [x] EPUB fragment values are decoded once, bounded, UTF-8/NUL-checked, and signed together with their canonical XHTML document path; a capability cannot move a slice to another resource.
- [x] Slice lookup compares DOM ids directly rather than interpolating a fragment into a CSS selector. Missing ids preserve a sanitized readable document; same-resource links to an omitted slice re-enter the parent Reader transaction instead of exposing an unrestricted resource.

Evidence for the checked EPUB items:

- Backend tests: `go test ./services/epubreader ./api ./db ./engine ./services/localbook` and full `go test ./...`; `TestDirectEPUBImageOnlyTitlepagePreviewImportAndReaderResource` proves the cover route remains capability-protected.
- Frontend tests: `npm test`.
- Browser test: `scripts/smoke/reader-epub-contract.mjs` against 1440×900, 390×844, and 360×800.
- E4-EPUB-2 additions: `backend/services/epubreader/capability_test.go`, `document_test.go`, `backend/api/api_test.go`, `backend/db/db_test.go`, `frontend/tests/readerEpubFrame.test.mjs`, and the same three-viewport browser smoke cover signed fragment bounds, migration/lazy recovery, document slicing and cross-resource navigation.

# 2026-07-13 Docker OCI fallback

- [x] The host-network OCI fallback reads registry credentials only through Docker's configured credential helper or the existing Docker config, retains the credential only in memory, and never logs an authorization header, password, or token.
- [x] OCI archive extraction rejects every path except the fixed OCI layout paths, verifies every SHA-256 descriptor before upload, and removes only its own `mkdtemp` workspace (and its opt-in temporary archive).
- [x] Uploads are limited to the explicit image/repository/tag arguments produced by the local release command; it never derives an arbitrary registry target from an archive.

## P2 user-management implementation gate

- [x] New-account validation is server-side and shared by registration and manager
  creation; existing account credentials are never revalidated or logged.
- [x] LocalStore and WebDAV/backup permissions are independently enforced before any
  request path/body/file access, while nullable legacy WebDAV permission falls back to
  the existing LocalStore value.
- [x] Batch deletion scopes every SQL row and private filesystem descendant to the
  validated target user; administrator legacy roots and another user's data are covered
  by regression tests.
- [x] Post-commit cleanup failures are client-safe and cannot cause a retry to delete
  another user or a legacy root; no password, JWT, path or credential is logged.

Evidence: `backend/api/user_management_p2_contract_test.go`,
`backend/api/workspace_storage_access_contract_test.go`,
`backend/db/db_test.go`, `frontend/tests/overlayUserManagement.test.mjs`, and
`frontend/tests/workspaceOperationRouteContract.test.mjs`.

## P0 Reader reauthentication isolation review

- [x] A 401 can invalidate the session only when the rejected Bearer token still exactly matches the current
      local token; late responses from a logged-out or superseded account are ignored.
- [x] The pending startup-auth event is consumed once without persisting or logging its token; the Reader
      invalidation event contains only a non-persistent generation.
- [x] Reader progress, pagehide/visibility/unmount keepalive, automatic reading, TTS, audio intent and chapter
      caching are suspended before credentials change.
- [x] Unauthenticated Reader/workspace routes render no previous book, chapter, catalogue, cover, bookmark,
      search result or global overlay DOM.
- [x] Account-owned overlay objects and pending selection promises are settled and cleared on session reset.
- [x] Same-account reauthentication mounts a new Reader generation; another or unknown identity returns to
      the shelf and cannot reuse the old database book ID.
- [x] Login return paths accept only same-origin absolute paths beginning with one `/`; protocol and
      scheme-relative redirects fail closed to `/`.
- [x] No API, SQLite schema, cache root, persistent setting, JWT lifetime or ownership rule changed.

Evidence: `frontend/tests/authenticatedRuntimeScope.test.mjs`,
`frontend/tests/readerPageLifecycle.test.mjs`,
`frontend/tests/readerReauthenticationWiring.test.mjs`, frontend 599/599,
the production build and full Go tests. Three-viewport browser confirmation and Docker publication remain
open because the local-server external permission request was rejected when the workspace reported no credits.

## P2 WebSocket synchronization security review (2026-08-09 extracted)

- [x] Browser handshakes reject an Origin whose host differs from request Host; the ordinary HTTP CORS reflection
      path cannot authorize a cross-site WebSocket. Same-origin and Origin-less diagnostic clients remain usable.
- [x] Missing, invalid and deleted-user JWTs return the same safe 401 before a Hub client is registered. The existing
      query-token transport remains redacted as `/ws/sync?<redacted>` and is never copied into an event/error.
- [x] The protocol is server-to-client only. A client text/binary frame is bounded and closed with policy violation;
      it is never parsed as a trusted event, relayed, persisted or used to trigger another client's store action.
- [x] Durable business events remain scoped to their owner. `users_update` reaches administrators and each affected
      user only; an unrelated ordinary account cannot learn another batch's user IDs.
- [x] Failed/rolled-back REST mutations emit nothing; Hub backpressure closes stale clients and reconnect continues
      to reconcile through authenticated REST rather than trusting a missed event stream.

Evidence: `backend/api/websocket_sync_p2_contract_test.go`, `backend/sync/hub_test.go`,
`backend/middleware/access_log_test.go`, `frontend/tests/authenticatedRuntimeScope.test.mjs`, the full Go/frontend
gates, focused race and a real two-client synchronization smoke at 1440×900, 390×844 and 360×800. See
[`compat/websocket-sync-p2-contract.md`](compat/websocket-sync-p2-contract.md).

## P1 source-manager second-audit security review (2026-08-09)

- [x] `usedBookNames` is response-only and produced by one query scoped to the authenticated user; another user's
      shelf names cannot enter the source manager projection.
- [x] Unknown reader-dev top-level fields are stored only as inert JSON under the reserved
      `__openreaderSourceExtra` rule key and restored on export; parser/runtime code ignores that key and never
      executes dormant JavaScript/WebView content.
- [x] Dangerous object keys (`__proto__`, `prototype`, `constructor`) are rejected from the preservation envelope;
      canonical source fields cannot be overridden by preserved extras.
- [x] Local source JSON imports are capped at 16 MiB and fail with 413 before JSON decoding; the multipart request
      is actual-read bounded at 17 MiB after JWT/source-edit authorization. Declared/chunked envelope overflow and
      16 MiB file overflow use distinct stable 413 errors.
- [x] The raw browser chooser rejects a known `File.size > 16 MiB` before `text()`/JSON parsing. The API accepts
      exactly one multipart file named `file`, rejects duplicate/foreign/scalar parts before decode or mutation,
      and explicitly removes every successfully parsed form on success and all failure paths.
- [x] Remote source preview continues through the shared SSRF-safe fetcher with scheme/host, redirect, timeout,
      response-size, DNS/rebinding, private-network and credential constraints.
- [x] Failure-cache categories expose only fixed safe labels and do not reveal JWTs, cookies, source headers,
      query strings, response bodies, WebDAV credentials or host filesystem paths.
- [x] The implementation changes no SQLite schema, filesystem path, backup/WebDAV format or destructive migration;
      existing source ownership, usage guard, mutation transaction and durable-only broadcast remain active.

Evidence: `backend/api/book_source_ownership_api_contract_test.go`,
`backend/api/book_source_local_import_multipart_boundary_contract_test.go`,
`backend/services/sourcecompat/export.go`, `frontend/tests/bookSourceEditor.test.mjs`,
`frontend/tests/sourceScriptTransparencyContract.test.mjs`, full Go/frontend gates, focused/full race, `go vet`,
`scripts/smoke/source-workspace-contract.mjs` at four viewports and
`scripts/smoke/booksource-local-import-multipart-contract.mjs` at three viewports. See
[`compat/booksource-local-import-multipart-fixed-baseline-second-audit-p2-contract.md`](compat/booksource-local-import-multipart-fixed-baseline-second-audit-p2-contract.md).

## P2 TXT TOC rule compatibility security review (2026-08-11)

- [x] The fixed-upstream 18-rule list changes no upload, stage-token, filesystem, SQLite, backup or WebDAV boundary.
- [x] Automatic detection still scans only enabled rules over the existing 512 KiB probe; disabled rules require an
      explicit user selection.
- [x] Ordinary rules continue through Go RE2 and the existing 16 KiB rule limit. No backtracking regex engine,
      JavaScript evaluator or remote fetch was introduced.
- [x] The two fixed-upstream cross-line lookaround rules use exact-rule dispatch and inspect at most eight adjacent
      Unicode whitespace runes. Modified/custom lookaround strings fail compilation instead of silently receiving
      broader semantics.
- [x] Input bytes, decoded text and final chapter count remain bounded by the shared local-book parser policy;
      parser failures consume no import stage and expose no source bytes or host path.

Evidence: `backend/engine/parser_test.go`, `backend/api/api_test.go`,
`frontend/tests/overlayBookImport.test.mjs`, full/focused/race Go, `go vet`, frontend 730/730, production build,
and `scripts/smoke/local-book-import-contract.mjs` at 1440×900, 390×844 and 360×800.

## P2 BookGroup / Category write-boundary review (2026-08-12 implementation)

- [x] Six authenticated BookGroup JSON mutations enforce the same 16 KiB actual-read limit for declared and
      chunked bodies; exactly 16 KiB reaches normal validation and 16 KiB + 1 returns safe flat 413.
- [x] Each mutation accepts exactly one JSON object plus whitespace. A second JSON value or trailing garbage maps
      to the endpoint's existing 400 and produces no row, timestamp or event change.
- [x] JWT remains first. Unknown built-in keys and missing/foreign owned Category/Book targets retain their existing
      pre-body 400/404 behavior and do not expose whether request data would otherwise be valid.
- [x] Invalid Category create is fully validated before `NextSortOrder`, so it cannot create built-in preference
      rows as a side effect. Mixed reorder and book-membership replacement stay atomic and caller-scoped.
- [x] `PUT /api/books/:id/category` validates the final effective ID set. A foreign `categoryId` combined with an
      empty/zero-only `categoryIds` cannot create a cross-user `books` or `book_categories` reference.
- [x] New names/colors enforce 80/24 UTF-8-byte budgets, while untouched historical oversized rows remain usable,
      backup/restorable and are never scanned or rewritten.
- [x] Errors, logs and sync events do not include JWTs, request bodies, submitted names/colors, SQLite text or host
      paths. No global middleware or unrelated endpoint limit is introduced.
- [ ] Before Docker publication, run the fresh/historical mounted-volume and backup compatibility gates against the
      implementation candidate.

Required implementation evidence: red/green API contracts for all six routes, two-user isolation and event
assertions, historical-row backup/restore coverage, focused/full/race/vet, frontend/build and isolated real HTTP.
The separate release checkbox additionally requires fresh/historical mounted-volume gates. See
[`compat/book-group-write-boundary-fixed-baseline-second-audit-p2-contract.md`](compat/book-group-write-boundary-fixed-baseline-second-audit-p2-contract.md).

Implementation evidence is complete on `6f54be3`: the red/green six-route contracts, caller/other-user persistence
and event assertions, historical oversized backup/restore, focused/full/race/vet, frontend 740/740, production build
and `scripts/smoke/book-group-write-boundary-contract.mjs` all pass. The release-specific fresh/historical
mounted-volume gate and local Docker publication still await explicit Docker socket approval.

## P2 public capability opened-file identity review (2026-08-17 implemented)

- [x] EPUB XHTML and sibling assets are read from a rooted regular handle whose identity is the object checked after
      capability, owner, generation and resource-path validation; no handler pathname reopen remains.
- [x] CBZ GET/HEAD/Range and local-audio GET/HEAD/Range pass the same verified handle and metadata to
      `http.ServeContent`; source/generation replacement cannot redirect an existing capability to another object.
- [x] Cached cover reads compare `Lstat/open/fstat` identity and never read or touch a replacement mounted object.
- [x] Root/ancestor/entry symlinks, directory/FIFO/device and deterministic validation-to-open replacement probes
      fail closed with path/token/credential-free errors while leaving every mounted object unchanged.
- [x] Capability purpose/user/book/fingerprint/expiry, EPUB CSP, MIME allowlists, byte/range behavior, private cache
      headers, old URLs and multi-user isolation remain unchanged.
- [x] Focused/race/full/vet, real EPUB/CBZ/audio/cover browser flows and fresh/historical/portable mounted-volume
      gates pass before local Docker publication.

Required evidence and scope exclusions are defined in
[`compat/public-capability-filesystem-read-lifecycle-fixed-baseline-second-audit-p2-contract.md`](compat/public-capability-filesystem-read-lifecycle-fixed-baseline-second-audit-p2-contract.md).
Implementation commit `a90f7b3` and all code-level gates are complete. `5313c49` evidence covers EPUB/CBZ/audio/
real cover at 1440/390/360, host and candidate HEAD/Range/mounted-symlink probes, plus fresh/historical/portable/
restart volumes. The final evidence commit was built locally and published as `5e63eb1`/`latest` at OCI index
`sha256:8b7bc4cd8542f79eccc54d393cf2d79041f5fe9a90b05776c473cd3f1e4c2cee`; both platform configs and a forced
remote arm64 runtime check report full revision `5e63eb1854a95dc3fd79ebafec89f4723f37f8da`. Status is
`aligned / regression-validated / Docker-published / awaiting-device-verification`.

## P2 HTTP server lifecycle review (2026-08-25 implemented/published)

- [x] Replace `router.Run` with an explicit `http.Server` using a 512 KiB maximum header and a 10-second
      `ReadHeaderTimeout`; keep global `ReadTimeout` and `WriteTimeout` disabled for valid long-lived work.
- [x] Handle `SIGINT`/`SIGTERM`, stop accepting new connections, and complete or cancel work within one bounded
      8-second shutdown budget instead of allowing Go's default signal exit to skip every defer.
- [x] Close hijacked WebSocket clients, terminate SSE by the final deadline, and invoke cleanup context,
      scheduler Stop and backup Stop exactly once without double-close panic.
- [x] Prove header, listen-error, ordinary-request drain, forced timeout, SSE, WebSocket and real container stop/restart
      behavior before publication; logs must remain credential-, query-, body-, database- and host-path-free.
- [x] The inventory introduces no code, schema, migration, persistent path, route, response, backup format or UI change.

Target contract:
[`docs/compat/http-server-lifecycle-fixed-baseline-second-audit-p2-contract.md`](compat/http-server-lifecycle-fixed-baseline-second-audit-p2-contract.md).
Contract `5b06084`, red tests `6bee4e0`, and implementation `f394c1a` landed in order. Focused/race/full/vet,
frontend 741/741, production build, real binary header/listen/SIGTERM/WebSocket, candidate Docker stop and
fresh/historical/portable/restart volume gates passed. The locally built amd64/arm64 `f394c1a`/`latest` release is
OCI index `sha256:4af0cf100434ed852fdf6727d351425cca6935c8f7f6a00eaec220de9865eafa`; both remote platform configs report
the full revision. Status is `aligned / regression-validated / Docker-published / awaiting-device-verification`.

## P2 access-log query redaction review (2026-08-25 implemented/published)

- [x] Replace every non-empty/raw query in access logs with one fixed `?<redacted>` marker before route/auth/status
      decisions can affect logging; never parse, decode, hash or partially retain query keys/values.
- [x] Preserve queryless path, method, status, latency and client IP, plus existing capability path-token redaction;
      WebSocket JWT must converge to the same global query rule.
- [x] Prove handlers still receive complete query values while 200/401/404, legacy/modern search, LocalStore,
      Explore, encoded credentials and 256 KiB query requests cannot place secrets or proportional bytes in logs.
- [x] The inventory changes no code, schema, migration, route, API result, filesystem path, backup format or UI.

Target contract:
[`docs/compat/access-log-query-redaction-fixed-baseline-second-audit-p2-contract.md`](compat/access-log-query-redaction-fixed-baseline-second-audit-p2-contract.md).
Contract `9161ce5`, red tests `cce9efd`, and implementation `f88ecec` landed in order. Focused/race/vet/full,
frontend 741/741, production build, real binary 200/401/404/256 KiB query and fresh/historical/portable/restart gates
passed. The locally built amd64/arm64 `f88ecec`/`latest` release is OCI index
`sha256:832216dbacb0650a5a6cb30b14731432714f4d48393516aed10c957a97549a29`; both remote platform configs report the
full revision. Status is `aligned / regression-validated / Docker-published / awaiting-device-verification`.

## P2 frontend public static tree review (2026-08-25 implemented/published)

- [x] Serve only trusted frontend build output through the generic public-file fallback; `api`, `ws`, `webdav`,
      `reader3`, `uploads` and `assets` retain their dedicated route and authorization behavior.
- [x] Resolve nested paths through the existing rooted file opener, reject entry/ancestor symlinks and non-regular
      files, and serve the same verified open handle so validation cannot be redirected by a pathname replacement.
- [x] Keep Vue history routes ahead of the public-file fallback, and restrict the fallback to GET/HEAD; missing files,
      directories, traversal, backslashes and unsupported methods retain safe 404/405 responses without host paths.
- [x] Do not expose or modify `data`, `cache`, `library`, uploads, database rows, environment variables, backup data
      or browser state. The change adds no schema or migration.
- [x] Prove UTF-8 and encoded paths, MIME, HEAD, Range, 304, route priority, symlink/FIFO rejection, real Reader
      asset decoding, mounted-volume compatibility and pulled-image behavior before publication.

Contract `9d32418`, red tests `525a4b6`, and implementation `5163262` landed in order. Focused/race/full/vet,
frontend 741/741, production build, real Go HTTP, three Chromium viewports and trusted Actions run `32851803480`
passed. The amd64/arm64 `5163262`/`latest` release is OCI index
`sha256:3a70be27680b32d51c11e20f56efa2be4824b12f8dff53135b45153dd2f2758d`; a fresh GHCR pull preserved the
expected health revision and theme/background bytes. Status is
`aligned / regression-validated / Docker-published / awaiting-device-verification`. Full contract:
[`compat/frontend-public-static-tree-fixed-baseline-second-audit-p2-contract.md`](compat/frontend-public-static-tree-fixed-baseline-second-audit-p2-contract.md).

## P2 authenticated-session lifecycle review (2026-08-26 implemented/published)

- [x] Protected REST, WebDAV Bearer and WebSocket all require a live User, matching auth generation and one
      non-expired persisted session after validating HS256; failure reasons remain indistinguishable 401s.
- [x] Store only SHA-256 identities for cryptographically random session tokens; never persist or log raw JWT/JTI,
      Authorization, password, claims, username or request query.
- [x] Implement seven-day sliding expiry, bounded 64-session retention, conditional renewal and current-session
      logout without allowing logout/delete races to recreate a removed session.
- [x] Password reset atomically updates hash, increments auth generation and removes all target sessions. User
      deletion and inactive cleanup must remove sessions before the User row and prevent stale-token side effects.
- [x] Adopt old JWTs only during one persisted seven-day migration window while the user remains at generation 1;
      restart cannot reopen the window and a post-reset old token cannot revive.
- [x] Exclude sessions and token identities from every backup/export; prove historical migration, rollback,
      REST/WebDAV/WS behavior, frontend best-effort logout, mounted volumes and pulled image before publication.

Target contract:
[`compat/authenticated-session-lifecycle-fixed-baseline-second-audit-p2-contract.md`](compat/authenticated-session-lifecycle-fixed-baseline-second-audit-p2-contract.md).
Contract `8db7c85`, red tests `2396537` and implementation `a0edce3` landed in order. Go full/race/vet, frontend
742/742, production build, browser/API session probes and Actions run `32914105929` fresh/historical/portable gates
passed. A pulled `a0edce3` image reported the full revision and repeated REST/WebDAV/WS revocation checks; its logs
kept WS query redacted and GORM SQL parameterized. OCI index:
`sha256:5d7fe23ba96107c5c545e9e44815514fe277e5a6f83eb25cb006859c5d515d78`. Status is
`aligned / regression-validated / Docker-published / awaiting-device-verification`.

## P2 default book-source snapshot boundary (2026-08-26 implemented/published)

- [x] Read only the fixed `data/defaultBookSources.json` as a same-file-verified non-symlink regular file with a
      16 MiB actual-read and 300-source cap; reject directories/FIFO/devices/sockets and replacement races.
- [x] Keep configured SQLite default namespace authoritative, but preserve a safe explicit legacy file (including
      `[]`) on direct pre-ownership upgrades without rewriting existing user source IDs or private namespaces.
- [x] Serialize current-user/target-user save, restore, admin reset, mirror publication and startup repair so one
      completed generation owns both SQLite and canonical JSON; cancellation/failure emits no false event.
- [x] Return stable path-free errors and logs. Never expose raw OS/SQLite text, host path, source URL/header/cookie,
      username, JWT, JSON body or temporary filename.
- [x] Keep mirror/migration state out of ordinary/portable/Legado backup and pass fresh/historical/restart volumes.

Target contract:
[`compat/default-book-source-snapshot-filesystem-transaction-fixed-baseline-second-audit-p2-contract.md`](compat/default-book-source-snapshot-filesystem-transaction-fixed-baseline-second-audit-p2-contract.md).
Contract `1c5f7b5`, red tests `6d8b8f1`, correction `07761b5` and implementation `a36b888` landed in order. Go
full/race/vet, frontend/build, path-free real HTTP, four-viewport browser, fresh/historical/portable
Actions gates and pulled-image mirror checks passed. OCI index
`sha256:63979a0e01d8942a9c594d444e6d5cdf28f0ac5c382825f71a051a52b02a21e4`; status is
`aligned / regression-validated / Docker-published / awaiting-device-verification`.

## P2 Explore request lifecycle (2026-08-26 implemented)

- [x] Resolve the caller-owned active source before query validation; foreign, detached and disabled sources retain
      one 404 and cannot be probed through page/entry error differences.
- [x] Accept only page `1..100000` and an at-most-8192-byte entry declared by that source's current explore rule;
      arbitrary relative/same-origin/cross-origin URL and request-option injection fail before remote work.
- [x] Preserve declared relative/absolute templates, URL options and existing response/UI semantics without treating
      a client override as a new remote-fetch capability.
- [x] Propagate the HTTP request context through fetch and parsing. Cancellation stops work and never writes an
      Explore business row, source failure, event or path/query/header/body-bearing response/log; shared auth
      activity/session middleware remains unchanged.
- [x] Prove owner priority, exact boundaries, zero-request rejection, cancellation and real chooser pagination before
      publication; keep existing fetcher SSRF/redirect/size/timeout policy unchanged.

Target contract:
[`compat/explore-request-lifecycle-fixed-baseline-second-audit-p2-contract.md`](compat/explore-request-lifecycle-fixed-baseline-second-audit-p2-contract.md).
Contract `2035965`, cancellation correction `9262864`, red tests `f9527c4` and implementation `938d956` landed in
order. Focused/race/full/vet, frontend 742/742, build, Compose, real HTTP, four-viewport Chromium and trusted Actions
run `32962930310` fresh/historical/portable/platform gates passed. `938d956`/`latest` OCI index is
`sha256:40cd73c3106736d88d361ae9fc81c3daf2ef1a7534b1b4db81bea46e9c6bc777`; a pulled container reported the full
revision. Status is `aligned / regression-validated / Docker-published / awaiting-device-verification`.

## P2 remote BookInfo/TOC request lifecycle (2026-08-27 inventory)

- [ ] Propagate the caller request context through temporary-session creation and explicit remote-book refresh;
      cancellation must stop BookInfo/TOC work and create no session, failure row, catalogue write or event.
- [ ] Re-read the owned Book in the refresh transaction after remote work and reject deleted or changed
      source/url/variable/updated-at snapshots before chapter mutation.
- [ ] Replace the refresh full-row `Save` with owned-field updates that cannot fallback-insert a deleted Book or
      overwrite a newer source, parser variable or metadata edit.
- [ ] Preserve normal BookInfo/TOC, progress/bookmark rebinding, cache prune, lastCheckTime, source failure and one
      durable shelf event semantics; do not reopen other remote actions or visible Reader/BookInfo flows.
- [ ] Prove cancellation and delete/change/edit races on the old implementation before coding, then pass focused/race,
      full/vet, real HTTP/browser and fresh/historical/portable publication gates.

Target contract:
[`compat/remote-book-detail-toc-request-lifecycle-fixed-baseline-second-audit-p2-contract.md`](compat/remote-book-detail-toc-request-lifecycle-fixed-baseline-second-audit-p2-contract.md).
Status is `inventory-complete / tests-and-implementation-pending`; no application or test code changed in inventory.
