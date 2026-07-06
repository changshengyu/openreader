# OpenReader Data Migration and Storage Contract

Status: initial scaffold.

## Persistent roots

| Root | Purpose | Compatibility rule |
|---|---|---|
| `data/` | SQLite database, uploads, WebDAV backup directory. | Must survive container upgrade. |
| `cache/` | Chapter/content cache. | May be regenerated, but broad deletion requires explicit user action or migration note. |
| `library/` | Imported original files and local store. | Must not be moved or deleted without migration. |

## SQLite rules

- Use non-destructive migrations.
- Keep existing rows readable.
- Add columns/tables with defaults where possible.
- Backfill in transactions when data consistency matters.
- Add tests for schema/data migration when a model changes.

## Compatibility inventory required

Before changing storage for a module, document:

- reader-dev source/config/data representation when applicable;
- current OpenReader table/path representation;
- migration or compatibility shim;
- backup/restore impact;
- Docker volume impact.

## Priority unresolved areas

- Reader-dev backup format import/export mapping.
- Source rule format and default source persistence.
- Reader progress and bookmark migration semantics.
- Local store/WebDAV path normalization and permissions.
- Cache invalidation rules for local and remote books.
