---
name: data-migration-compat
description: Data compatibility workflow for OpenReader. Use when changing GORM models, SQLite migrations, cache, library, uploads, local store, backup/restore, WebDAV storage, or persistent settings.
---

# Data Migration Compatibility

Use this skill when a change can affect persisted user data.

## Hard rules

- Do not silently redesign persistent data.
- Preserve existing `data/`, `cache/`, and `library/` mounted directories.
- Do not break old SQLite databases without a migration path.
- Do not delete cache/library files as part of schema cleanup unless explicitly requested and backed up.

## Workflow

1. Identify old data format and location.
2. Identify new data format and location.
3. Document compatibility in `docs/compat/data-migration.md`.
4. Add testdata when possible.
5. Add migration or compatibility shim.
6. Test against an existing-volume scenario.

## Required checks

Run full backend tests and, for release-sensitive changes, `scripts/docker-volume-backup-smoke.sh`.
