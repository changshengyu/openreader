# OpenReader Security Review Checklist

Use this checklist for security-sensitive changes and release reviews.

## Authentication and authorization

- [ ] `OPENREADER_JWT_SECRET` is required and not logged.
- [ ] Protected endpoints require valid JWT.
- [ ] Admin endpoints check admin role.
- [ ] User-owned rows are scoped by authenticated user ID.
- [ ] Batch operations cannot affect another user’s data.

## SSRF and remote fetches

- [ ] Source/RSS/cover/WebDAV remote URLs validate scheme.
- [ ] Redirect count is bounded.
- [ ] Request timeout is set.
- [ ] Response body size is bounded.
- [ ] Private network access is considered when server-side fetches are user-controlled.
- [ ] Headers/cookies are not logged.

## Path traversal and files

- [ ] Every user path is cleaned and joined under an allowed root.
- [ ] Final resolved path is verified to remain under the allowed root.
- [ ] Local store, uploads, cache, backups, and WebDAV all use rooted paths.
- [ ] Backup downloads only expose expected backup files.
- [ ] API errors do not leak host filesystem paths.

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
- [x] Upload size and extension allowlists are unchanged; rooted path checks reject
  traversal, query and fragment variants without leaking a filesystem path.

Evidence: `backend/api/bookinfo_asset_contract_test.go`, upload/update API tests,
`frontend/tests/overlayBookInfo.test.mjs`,
`frontend/tests/readerAppearanceAssets.test.mjs`, and the P2 BookInfo real-browser
contract (three viewports).

## P1-E2 workspace storage audit

- [x] Raw `/webdav/*` now uses the normal Bearer JWT and activity middleware before it can reach a filesystem handler.
- [x] `User.CanAccessStore` is enforced before LocalStore, WebDAV, backup and import/restore handlers inspect a path, body or file. Store-disabled users receive 403.
- [x] LocalStore, WebDAV and generated backups resolve to private descendants for regular users while the administrator retains the preserved legacy root. Cross-user access is denied without moving/deleting mounted data.
- [x] Direct and storage-backed preview/import uses user-scoped random staged tokens; confirmation consumes the staged bytes, foreign/expired tokens fail closed, and successful/expired stages are removed.
- [x] Direct local-book upload, LocalStore/WebDAV upload, preview and confirmation reads are capped by `OPENREADER_MAX_IMPORT_BYTES` (128 MiB by default) before staging or parser work. LocalStore/WebDAV writes stage beside the target and rename only after the bounded copy succeeds.
- [ ] Archive entry/expanded-size and parser-work limits still need explicit bounds; stage cleanup must also run without a later user request.

Evidence for the checked items: `backend/api/workspace_storage_access_contract_test.go`, `backend/api/workspace_import_stage_contract_test.go`, `backend/api/import_size_contract_test.go`, `frontend/tests/webdavAuthContract.test.mjs`, full Go/frontend test suites and production frontend build. This remains not a storage/backup release approval.

## P1-E3 workspace file-manager follow-up

- [x] LocalStore multi-file upload retains per-file basename validation, private-root resolution, size limits and same-directory atomic replacement; a rejected part does not truncate an existing destination or disclose its host path.
- [x] Removing visible directory/rename/download/recursive controls does not remove guarded legacy API compatibility routes or weaken the existing raw WebDAV `MKCOL`/`MOVE` path checks.
- [x] Workbench suffix gating is presentation-only: disallowed current UI formats do not bypass P1-E1 scoped preview tokens, and retained direct parser support does not expand filesystem access.
- [x] Browser regression proves LocalStore/WebDAV requests retain bearer auth and no hidden mobile control can invoke a removed operation.

Evidence: `backend/api/workspace_file_manager_p1e3_contract_test.go` covers private rooted listing fields, multi-file ordinary-file storage and a rejected later part preserving its old destination. `frontend/tests/workspaceFileManagerParity.test.mjs` keeps source-specific presentation gates separated from direct parser support. `scripts/smoke/workspace-storage-import-state-machine.mjs` validates authenticated WebDAV requests and removed actions across desktop and both required mobile sizes.

## P1-E4 TXT empty-catalogue follow-up

- [x] A valid staged TXT with an unmatched user TOC rule is no longer misclassified as a parser or transport failure. The response retains only the opaque caller-scoped stage token and returns no mounted path, parser internals, credentials or source bytes.
- [x] Confirmation consumes only that caller's staged file, archives the original safely, and creates a zero-chapter local book without fabricating a chapter or dereferencing a missing final chapter. Foreign/expired-token rejection remains covered by the existing stage-token contract.
- [x] Direct, LocalStore and WebDAV UI keep the empty catalogue distinct from an actual parser failure, so a user can retry against the immutable staged data or deliberately confirm the upstream-compatible zero-chapter state.

Evidence: `backend/services/localbook/importer_test.go`, `backend/api/api_test.go`, `backend/api/workspace_import_stage_contract_test.go`, `frontend/tests/overlayBookImport.test.mjs`, `frontend/tests/storageImportWorkflow.test.mjs`, and `scripts/smoke/local-book-import-contract.mjs` / `workspace-storage-retry-contract.mjs` at desktop and both mobile viewports.

## P2 import parser and staged-preview follow-up

- [x] Initial EPUB parsing now validates ZIP paths/symlinks/duplicates/count/per-entry/expanded-size before local import work; every archive-member read is bounded.
- [x] Initial CBZ parsing retains its existing safe checks while using the same local-import limit policy.
- [x] E4-CBZ-1 derives its first image only from the bounded/normalized archive walk and returns a short-lived CBZ capability at serialization time. It does not persist a capability, ZIP member path, raw archive path, or JWT in SQLite, archive metadata, backup/WebDAV data, sync payload storage, or logs; malformed/missing archives degrade to an empty cover without failing the bookshelf response. Evidence: `TestDirectCBZImportAndResourceCapability`, `TestParseCBZKeepsFirstArchiveImageAsCoverSeparateFromSortedCatalogue`, full backend tests and the Docker volume/backup smoke for this release.
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
- [x] Structural archive failure has no user-data mutation; valid reader-dev/Legado/OpenReader formats preserve count/event compatibility.

Evidence: `backend/api/backup_restore_contract_test.go`, existing reader-dev/Legado/OpenReader backup fixtures in `backend/api/api_test.go`, and bookmark restore fixtures. Mounted-volume Docker smoke remains mandatory before release.

## P1-E4 portable local archive backup review

- [x] Portable packages are explicit `openreader-portable-v1` archives; ordinary reader-dev/Legado/OpenReader backup ZIPs do not gain `library/` data or change their meaning.
- [x] The archive reader rejects unsafe/duplicate/case-conflicting names, directories, symlinks, unknown logical entries, invalid manifest slots, unbounded compressed/member/total sizes and bad SHA-256 before logical restore dispatch.
- [x] Original archives are streamed to a caller-private staging root, checked against the manifest, parsed under the portable per-entry budget, and never derive a destination path from an archive member or stored host path.
- [x] Trigger and restore are caller scoped. A matching `local://` identity with a different/missing destination archive is a `409` before mutation; an identical existing archive is reused, so a package cannot overwrite another user's or an unrelated same-identity book.
- [x] Type=1 local audio directories and missing/unsafe originals fail generation rather than being silently omitted. No JWT, WebDAV credential, archive member or host filesystem path appears in the API error or manifest.

Evidence: `backend/services/backup/portable_test.go`, `backend/api/portable_backup_contract_test.go`, full backend suite, and `HISTORICAL_VOLUME=1 scripts/docker-volume-backup-smoke.sh` export/upload/fresh-volume/restart coverage.

## P2 replace-rule review

- [x] Reader-global replacement rules remain user-scoped for list, create, update, batch upsert, delete, preview and content application.
- [x] New and edited regex patterns compile before persistence or preview; malformed regex returns a client-safe `400` and is never silently reinterpreted as a literal replacement.
- [x] The global Reader rule path uses Go's RE2 engine with a bounded pattern (16 KiB) and replacement (64 KiB), avoiding catastrophic-backtracking regex behavior and unbounded user-controlled rule fields.
- [x] Existing invalid stored regexes fail closed for the remaining reader pipeline; they never produce a literal replacement that could silently corrupt chapter content.
- [x] Reader-global rules use a dedicated execution path, so source-parser replacement semantics are not broadened or weakened by this UI compatibility change.
- [x] Error responses contain field/regex validation messages only; they do not expose a chapter cache path, JWT, source headers, WebDAV credentials, or database content.

Evidence: `backend/api/replace_rules_contract_test.go`, `backend/api/api_test.go` replace-rule/content cases, `frontend/tests/readerSelectedTextActions.test.mjs`, `frontend/tests/overlayReplaceRules.test.mjs`, and the full Go/frontend validation gates.

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
- [x] The stream retains existing cache request limits (`count <= 300`) and source-request timeout/redirect/body-size controls; batch limits remain unchanged.
- [x] Request cancellation propagates through context-aware chapter/pagination fetching and stops scheduling later chapters. Already written bounded cache files remain normal cache data and no terminal shelf broadcast is emitted for a cancelled stream.
- [x] Errors sent after stream opening are client-safe generic text. Authorization/validation failures happen as ordinary JSON before an event stream opens.

Evidence: `backend/api/cache_stream_contract_test.go`, `frontend/tests/bookCacheStream.test.mjs`, `frontend/tests/overlayBookManagement.test.mjs`, full backend tests and frontend build/test gate. The three-viewport BookManage SSE click smoke must be rerun after the local browser-runner authorization channel is available.

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
