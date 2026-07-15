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
- [x] Standard reader-dev UMD uses a bounded `#`/`$` section reader: signature/type, section/additional lengths, segment count, offsets/titles, zlib output and total decoded text are validated before archive/database writes. Image, malformed and corrupt zlib UMD inputs fail closed; the legacy OpenReader-only prefix is isolated to its existing fallback.
- [x] Expired and orphaned preview tokens are cleaned from every user directory at startup and hourly, without touching active previews or any mounted source/library data.
- [x] Backup ZIP restore now receives a separately tested compressed/entry/expanded-size budget; it remains a distinct compatibility slice from parser/stage handling.

Evidence: `backend/engine/import_limits_contract_test.go`, `backend/engine/umd_parser_contract_test.go`, `backend/services/localbook/importer_test.go`, `backend/api/workspace_import_stage_contract_test.go`, `backend/api/umd_import_contract_test.go`, `backend/config/config_test.go`, and full `go test ./...`. Docker mounted-volume/backup validation remains required before this slice is released.

## P2 backup restore follow-up

- [x] Multipart and WebDAV backup restore enforce one compressed input bound before an allocation or restore mutation.
- [x] ZIP member paths, duplicate canonical names, count, per-member bytes and cumulative expanded bytes are validated before restore dispatch; restore dispatch receives only the bounded preflight data.
- [x] Backend accepts only normalized `.zip` WebDAV restore targets and does not disclose a mounted path or archive member in client errors.
- [x] Structural archive failure has no user-data mutation; valid reader-dev/Legado/OpenReader formats preserve count/event compatibility.

Evidence: `backend/api/backup_restore_contract_test.go`, existing reader-dev/Legado/OpenReader backup fixtures in `backend/api/api_test.go`, and bookmark restore fixtures. Mounted-volume Docker smoke remains mandatory before release.

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
- [x] CSP blocks remote network loads and untrusted scripts while allowing scoped local CSS/images/fonts and required inline reader styles.
- [x] MIME types are allowlisted and responses set `nosniff` and `no-referrer`.
- [x] Multi-user tests prove one user's capability cannot read another user's book or resource tree.
- [x] Parent `message` handlers verify both the active iframe window and expected same-origin resource origin.

Evidence for the checked EPUB items:

- Backend tests: `go test ./services/epubreader ./api ./db ./engine ./services/localbook` and full `go test ./...`.
- Frontend tests: `npm test`.
- Browser test: `scripts/smoke/reader-epub-contract.mjs` against 1440×900, 390×844, and 360×800.

# 2026-07-13 Docker OCI fallback

- [x] The host-network OCI fallback reads registry credentials only through Docker's configured credential helper or the existing Docker config, retains the credential only in memory, and never logs an authorization header, password, or token.
- [x] OCI archive extraction rejects every path except the fixed OCI layout paths, verifies every SHA-256 descriptor before upload, and removes only its own `mkdtemp` workspace (and its opt-in temporary archive).
- [x] Uploads are limited to the explicit image/repository/tag arguments produced by the local release command; it never derives an arbitrary registry target from an archive.
