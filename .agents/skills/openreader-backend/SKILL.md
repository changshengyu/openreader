---
name: openreader-backend
description: Backend architecture guardrails for OpenReader Go/Gin/GORM/SQLite changes. Use when modifying handlers, services, models, migrations, WebSocket sync, WebDAV, backup, imports, cache, or any backend API behavior.
---

# OpenReader Backend

Use this skill for backend work in `backend/`.

## Non-negotiable structure

- Keep Gin handlers thin: bind/validate input, call services, map errors to HTTP status, serialize output.
- Put business logic in `backend/services/...`.
- Put reusable persistence rules in repository-style helpers or clearly scoped model/service functions.
- Keep SQLite WAL compatibility and existing data intact. Avoid destructive migrations.
- Use `db.Transaction` for multi-row or row+file mutations.
- Emit WebSocket/sync events only after durable writes succeed.

## API behavior rules

- Preserve existing endpoint paths unless a compatibility redirect/shim is added.
- Return stable JSON field names matching current frontend and upstream semantic expectations.
- Keep legacy compatibility endpoints only when they protect existing clients, e.g. reader3-compatible routes.
- Prefer precise status codes:
  - `400` invalid request or unsupported file/rule.
  - `401` unauthenticated/invalid JWT.
  - `403` authenticated but not allowed.
  - `404` object not found for the current user.
  - `409` conflict or duplicate.
  - `500` unexpected server failure.

## File and network safety

- Normalize user paths with `filepath.Clean`.
- Join only under configured roots: `DataDir`, `CacheDir`, `LibraryDir`, local-store roots, upload roots.
- Reject path traversal after resolving the final path.
- Never trust uploaded filenames, archive entry names, WebDAV paths, or book source URLs.
- Remote fetches must have timeout, redirect limit, body size limit, and safe scheme handling.

## Required checks before handoff

Run from `backend/`:

```bash
go test ./...
```

If the change touches backup, WebDAV, local import, cache, or Docker-mounted paths, also run the Docker/volume compatibility smoke script documented in `docs/docker-volume-backup-compatibility.md`.
