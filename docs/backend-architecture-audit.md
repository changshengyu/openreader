# Backend Architecture Audit

Status: active engineering baseline.

## Scope

This audit covers the current Go backend shape and the target constraints for continued OpenReader refactoring.

## Current structure

| Area | Current location | Assessment | Required direction |
|---|---|---|---|
| HTTP routing | `backend/api/server.go` | Centralized route registration is workable. | Keep stable routes; document compatibility shims. |
| Handlers | `backend/api/*.go` | Many handlers still contain business/data logic. | New and touched code should move business logic into services. |
| Services | `backend/services/backup`, `backend/services/localbook`, `backend/services/scheduler` | Service layer exists but is incomplete. | Expand service layer around import, source parsing, backup, cache, WebDAV, and book management. |
| Persistence | Direct GORM usage across API/services | Works but mixes transaction semantics into handlers. | Introduce repository-style helpers when refactoring high-risk modules. |
| Models | `backend/models/models.go` | Central model file is easy to inspect but high-churn. | Avoid destructive schema changes; document new fields. |
| Parser/fetcher | `backend/engine/*` | Good test coverage exists for source parsing and imports. | Keep fixture-based parser tests; add limits and security checks when touched. |
| Auth/middleware | `backend/middleware/*`, `backend/api/auth.go` | JWT and activity tracking exist. | Preserve multi-user isolation and admin boundaries. |
| Sync | `backend/sync/hub.go` | WebSocket hub exists. | Emit sync only after durable writes. |
| Static frontend | `backend/main.go` | Single binary route is aligned with Docker goal. | Preserve SPA fallback and `/api` separation. |

## Target backend rules

- Handler: bind, validate, authorize, call service, respond.
- Service: own workflow and transaction boundaries.
- Repository/helper: own repeated GORM queries and user scoping.
- Parser/fetcher: own remote/file parsing with bounded resources.
- Config: keep paths and secrets centralized in `backend/config`.

## Priority refactor candidates

1. `backend/api/books.go`: book lifecycle, refresh, source change, cache, search, progress interactions.
2. `backend/api/imports.go` + `backend/services/localbook`: local import preview/import should stay deterministic and token-backed.
3. `backend/api/sources.go` + `backend/engine/source_*`: source import/test/search should preserve reader3 semantics while adding safety bounds.
4. `backend/api/backup.go` + `backend/services/backup`: backup/restore needs explicit transaction and compatibility notes.
5. `backend/api/localstore.go` + `backend/api/webdav.go`: path normalization and multi-user/store permission boundaries.

## Backend validation

Run:

```bash
cd backend && go test ./...
```

For touched import/source/security modules, add targeted tests in `backend/engine`, `backend/services`, or `backend/api` before relying on full-suite success.
