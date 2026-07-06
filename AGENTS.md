# OpenReader Codex Instructions

This repository is a refactor/rewrite of `changshengyu/reader-dev`. Treat upstream behavior as the product contract, not the current rewritten component shape.

## Current stack

- Backend: Go 1.24, Gin, GORM, SQLite WAL, Gorilla WebSocket, goquery.
- Frontend: Vue 3.5, Vite, Pinia, Vue Router, Element Plus.
- Deployment: one Go binary serves the API and the built frontend in a single Docker container.
- Persistent data: `data/`, `cache/`, and `library/` must remain upgrade-compatible.

## Required project skills

Use the repo-level skills under `.agents/skills/` when they match the change:

- `readerdev-compat-inventory`: before implementing an upstream-backed feature, extract and record the reader-dev vs OpenReader compatibility matrix; do not code during that pass.
- `api-contract-compat`: API route, request/response, auth, status code, and error compatibility.
- `data-migration-compat`: SQLite schema, cache/library/data directories, backup/restore, WebDAV storage, and migration safety.
- `booksource-parser-compat`: online source, selector, XPath-like rule, RSS, chapter, and import parser compatibility.
- `frontend-ux-compat`: Vue route, local state, reader/workspace UX, touch/keyboard behavior, and Playwright smoke compatibility.
- `openreader-backend`: Go API, data model, service, repository, SQLite, WebSocket, backup, WebDAV.
- `openreader-frontend`: Vue, reader UI, Index workspace, Pinia state, Element Plus, mobile behavior.
- `openreader-parser`: book sources, selectors/XPath, RSS, TXT/EPUB/PDF/Markdown/UMD parsing and import.
- `openreader-security`: SSRF, path traversal, upload, archive extraction, JWT, WebDAV credentials, parser DoS.
- `openreader-regression`: tests, builds, smoke checks, browser verification, compatibility gates.
- `openreader-docker-release`: local Docker build and GHCR publish workflow.

## Refactor baseline

- Fixed upstream baseline: `changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`.
- Local upstream checkout, when available: `/private/tmp/changshengyu-reader-dev`.
- Do not use current rewritten files as proof that a behavior is correct.
- Before rebuilding a module, map upstream files, API/data contracts, state transitions, defaults, and visible behavior.
- For upstream-backed features, first update `docs/compat/reader-dev-openreader-gap-analysis.md` or a more specific compatibility contract. Code comes after the contract.
- Keep user data and old URLs compatible. Wrong UI structures may be deleted or rebuilt.
- Allowed differences must be explicit: Vue 3/Pinia/Go runtime adaptation, multi-user isolation, cache hardening, security fixes, or user-requested UX improvements.

## Architecture rules

### Backend

- Gin handlers bind input, authorize, choose status codes, and serialize responses.
- Business logic belongs in services.
- Database reads/writes belong in repositories or narrowly scoped persistence functions.
- Write operations touching multiple tables/files must use transactions or documented compensation.
- All user-controlled paths must be normalized, rooted, and checked before access.
- Remote fetches must have timeout, response size limit, redirect limit, and scheme/host validation where applicable.
- Do not introduce destructive migrations. Preserve existing SQLite data.

### Frontend

- Upstream-visible reader and Index behavior is authoritative.
- Do not keep duplicate business flows. BookInfo, source management, import, and reader actions should converge to shared components/stores.
- Mobile and desktop interaction rules must be verified separately.
- Reader tests that encode behavior conflicting with upstream must be deleted or rewritten.
- User-requested improvements currently allowed:
  - native continuous finger/wheel scrolling while click paging remains paged;
  - numeric minus/value/plus setting controls instead of easy-to-mis-tap sliders.

### Release

- Do not use cloud Docker builds for releases.
- Build Docker images locally and push to GHCR only after a complete module passes its gate.
- Every Docker release report must include completed items, allowed differences, unfinished items, image tags, digest, and verification summary.

## Minimum validation

For normal implementation changes:

```bash
cd backend && go test ./...
cd frontend && npm test
cd frontend && npm run build
```

For frontend interaction changes, add a real-browser smoke check. For release candidates, also run a local Docker build and volume/backup compatibility check.
