# Reader-dev vs OpenReader API Diff

Status: specialist contracts cover the implemented priority modules; remaining routes are audited action by action.

Reader-dev is a Java/Spring + Vue 2 application. OpenReader is a Go/Gin + Vue 3 rewrite with JWT multi-user auth and a single-container runtime. Reader, sources, import, book management, backup/WebDAV/local store, settings/admin/RSS/replace/resource routes now have specialist contracts. Any route not named by one of those contracts still requires action-level extraction before backend refactors.

## Known intentional OpenReader additions

| Area | OpenReader behavior | Rationale |
|---|---|---|
| Auth | JWT login/register and `/api/me`. | Multi-user self-hosted deployment. |
| Health | `/api/health` exposes runtime/build metadata. | Docker and support diagnostics. |
| Volumes | `data/`, `cache/`, `library/`. | Stable single-container persistence. |
| Legacy shim | `/api/reader3/searchBookContent`. | Preserve reader3-compatible search behavior for migrated UI/API clients. |

## Book-source ownership correction

The deployed OpenReader REST paths remain stable, but their previous global-table semantics are a `must-fix`, not
an intentional JWT adaptation. Reader-dev resolves `bookSource.json` below each user namespace; therefore
`/api/sources*`, debug, search/explore, Reader, scheduler, backup and admin counts must all use authenticated-user
associations. Shared rows created by the additive migration are storage deduplication only and must use
copy-on-write. Full route/status/error compatibility is recorded in
`book-source-ownership-p2-contract.md` and `api-contract.md`.

Implementation status on 2026-08-09: source management/debug, search, explore, remote-book, Reader
content/cache, scheduler, backup/WebDAV restore and administrator count/default/reset/delete consumers are
association-scoped and released. The `/ws/sync` protocol audit and implementation are recorded in
`websocket-sync-p2-contract.md`.

The source-debug second audit is now implemented and browser-validated. The wrong three-tab Dialog was removed;
the standalone editor saves before one Bearer POST SSE stream and restores exact search/explore/direct-entry →
BookInfo → TOC → first-content sequencing, runtime-variable boundaries, adjacent-chapter protection and ordered
bounded logs. Existing `/api/sources/:id/test*` paths remain response-compatible shims but no debug path writes
`source_failures`. Full status is in `source-debug-fixed-baseline-second-audit-p2-contract.md`; the locally built
`f8f263d`/`latest` image has passed fresh/historical volume gates and is published.

The UserManage second audit found and removed a full-row GORM `Save` from `PUT /api/admin/users/:id`.
The endpoint now updates only explicitly supplied columns, reloads a fresh row, projects legacy nullable WebDAV
access without backfilling it, and rejects empty or negative-limit patches. Frontend switches own one field each,
with field-scoped pending and rollback. The Docker-published contract is
`user-management-partial-update-second-audit-p2-contract.md`; `77a60d8` passed fresh/historical volume gates.

## WebSocket synchronization direction and scope

Reader-dev has no WebSocket write path. OpenReader retains `GET /ws/sync?token=<jwt>` as a multi-client runtime
adaptation, but the protocol is server-to-client only: only committed REST mutations may produce events. The current
arbitrary client-event relay, unconditional Origin acceptance and global `users_update` recipient set were
`must-fix` and are now removed. Exact handshake statuses, event envelopes, account scopes, log redaction and tests
are recorded in `websocket-sync-p2-contract.md`.

## Required extraction before backend changes

For each module, record:

- reader-dev method/path/query/body/status/response;
- OpenReader method/path/query/body/status/response;
- whether the difference is `must-fix`, `acceptable-change`, `intentional-redesign`, or `unknown`;
- test file covering the contract.

## Priority modules

1. Reader content/progress/bookmarks.
2. Source search/catalog/chapter content.
3. Local import preview/import.
4. Book management/category/batch operations.
5. Backup/WebDAV/local store.

## Manual shelf refresh second audit

The fixed reader-dev Index and Reader-internal shelf both map a visible “刷新” click to
`getBookshelf?refresh=1`; the backend checks current-namespace remote updateable books with concurrency 16,
isolates one book's fetch failure, refreshes its TOC, and returns the complete shelf. OpenReader now translates
both visible actions through one deduplicated Pinia transaction: `POST /api/books/check-updates` performs bounded
TOC-only checks with persisted variables and per-book atomic catalogue reconciliation, then the client performs
one authoritative `GET /api/books` after invalidating only replaced browser chapter caches.

This was a `must-fix`, not an allowed REST redesign. The stable OpenReader POST path remains the translation layer;
bounded fetch, authoritative catalogue reconciliation, stale-result protection, per-book atomic writes, safe
partial-failure counts and one durable shelf event are implemented behind it. Exact API/data behavior and the
Docker-published evidence are recorded in `bookshelf-manual-refresh-fixed-baseline-second-audit-p1-contract.md`.
`43635a1`/`latest` points to OCI index
`sha256:0f75a0434d209af901cde81f86127f8e62fa78d6cb3610d6c10ef2e0863053c0`.

## Search/Explore temporary Reader second audit

Reader-dev keeps an unshelved search result in the browser `readingBook` state and reconstructs BookInfo/TOC/content without calling `saveBook`; durable progress is rejected until the user explicitly adds the book. OpenReader's authenticated opaque `/api/reader/remote-sessions*` API is a technical-stack/security translation of that lifecycle, not a new persistent book type.

The fixed-baseline second audit verifies the existing user binding, no-store response, server-authoritative chapter URL, zero shelf writes and Book/Chapter variable precedence. It also found and corrected three `must-fix` differences: the create body was unbounded, the in-memory session map had no per-session/user/process retention budget, and malformed chapter indices renewed the idle lease before validation. Exact limits, LRU/expiry-marker semantics, cancellation/error-redaction requirements and regression evidence are recorded in [`remote-reader-session-fixed-baseline-second-audit-p1-contract.md`](remote-reader-session-fixed-baseline-second-audit-p1-contract.md). Status: `aligned / Docker-published / awaiting-device-verification`; `30dbe53`/`latest` OCI index is `sha256:9c07871ef7d3c8d99733fcecea205336576c081db651dada13eaeedafda76365`.
