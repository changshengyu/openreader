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

## EPUB reader compatibility migration

Status: implemented for the Reader P0 EPUB slice; remaining Reader P0 work is outside this EPUB resource migration.

### Existing representation

- The original imported EPUB is already archived below `library/<Book.LibraryPath>` and referenced by `Book.OriginalFile`.
- `Book.LibraryPath`, `Book.OriginalFile`, `Book.TOCFile`, and `Book.SourceFile` are persistent source-of-truth fields.
- `Chapter.CachePath` points to the flattened plain-text chapter copy used by existing reader/search/cache flows.
- Existing EPUB chapter rows do not retain the canonical XHTML resource path from the archive.

### Additive representation

- Add nullable/empty `Chapter.ResourcePath` (`resourcePath` in JSON) through GORM auto-migration. It stores a normalized POSIX EPUB path such as `OEBPS/Text/chapter-1.xhtml`; it is never an absolute host path.
- Add optional `resourcePath` to archived `chapters.json` entries. Old archives without it remain valid.
- EPUB import writes both:
  - the existing plain-text `CachePath`;
  - the canonical XHTML `ResourcePath`.
- Existing imported EPUBs are lazily backfilled from the archived original file and current TOC rule when first opened/refreshed. Backfill updates only matching chapter rows and the optional portable `chapters.json` metadata.

No table or column is removed. Text, PDF, UMD, Markdown, remote, and existing EPUB rows remain readable when `ResourcePath` is empty.

### Derived extracted resources

- Extraction lives below the existing book root:

```text
library/<Book.LibraryPath>/.epub-resources/<source-fingerprint>/
```

- The source fingerprint is deterministic from the archived EPUB bytes. A replacement file receives a new directory/version and invalidates old resource capabilities.
- The original EPUB remains the source of truth. `.epub-resources/` is derived and may be recreated when absent or corrupt.
- Extraction is staged in a sibling temporary directory and atomically renamed only after every entry passes validation. Failed extraction must not leave a partially active version.
- Cleanup may remove stale derived fingerprint directories for the same book after the current version is durable, but must never delete `OriginalFile`, `content/`, `chapters.json`, or `bookSource.json`.

### Compatibility and recovery

- Old databases: GORM adds the empty `resource_path` column; no full-table destructive migration.
- Old `chapters.json`: missing `resourcePath` is treated as unknown and recovered from the source EPUB.
- Missing derived directory: rebuild transparently from `OriginalFile`.
- Missing/corrupt source EPUB: preserve all database rows and plain-text caches; return a reader error instead of deleting/reimporting the book.
- Backup/restore and WebDAV: the existing original EPUB and metadata remain sufficient. Derived `.epub-resources/` need not be present in a backup to recover the book.
- Docker volumes: all new files remain under the existing `library/` mount. No new volume is introduced.

### Required migration evidence

- Auto-migrate an existing database containing chapters without `resource_path`; old rows and caches remain readable: covered by `backend/db.TestAutoMigrateAddsEPUBResourcePathWithoutLosingChapters`.
- Open an old imported EPUB and verify lazy path backfill without re-upload: covered by `backend/api.TestDirectEPUBImportAndRefreshUseTocRule`.
- Remove `.epub-resources/` and verify deterministic rebuild: covered by the same API test through a repeated resource request after deleting the derived directory.
- Replace the source archive and verify old capability/version rejection: covered by the same API test.
- Run full backend tests and `scripts/docker-volume-backup-smoke.sh` before an EPUB release image.
