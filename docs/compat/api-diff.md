# Reader-dev vs OpenReader API Diff

Status: initial scaffold.

Reader-dev is a Java/Spring + Vue 2 application. OpenReader is a Go/Gin + Vue 3 rewrite with JWT multi-user auth and a single-container runtime. Exact endpoint-by-endpoint extraction is still required per module before backend refactors.

## Known intentional OpenReader additions

| Area | OpenReader behavior | Rationale |
|---|---|---|
| Auth | JWT login/register and `/api/me`. | Multi-user self-hosted deployment. |
| Health | `/api/health` exposes runtime/build metadata. | Docker and support diagnostics. |
| Volumes | `data/`, `cache/`, `library/`. | Stable single-container persistence. |
| Legacy shim | `/api/reader3/searchBookContent`. | Preserve reader3-compatible search behavior for migrated UI/API clients. |

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
