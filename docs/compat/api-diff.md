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

The UserManage second audit found that the current `PUT /api/admin/users/:id` still uses a full-row GORM
`Save` after applying a partial request. This contradicts reader-dev's field-specific update action and can overwrite
a concurrent password reset or successful-login timestamp. The extracted, implementation-pending contract is
`user-management-partial-update-second-audit-p2-contract.md`.

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
