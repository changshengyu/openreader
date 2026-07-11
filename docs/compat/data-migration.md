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

## P1-E2 workspace storage scope and staged import compatibility

Status: implemented in-progress; no database migration is required.

### Mounted-root mapping

| Caller | LocalStore root | WebDAV and backup root | Compatibility rule |
|---|---|---|---|
| Administrator | Existing `library/localStore/` | Existing `data/webdav/` | The legacy tree stays where it is. Existing files and backups remain readable without a move. |
| Regular user | `library/localStore/users/<safe-username>/` | `data/webdav/users/<safe-username>/` | New writes, browse/import/download/delete operations and generated backups are private descendants of the same mounts. |

- The mapping is determined from the authenticated persisted user after `canAccessStore` authorization. It introduces no new SQLite column and does not rewrite old rows or paths.
- Scheduled backup runs once per persisted user and filters all user-owned export rows. Administrator `RunNow()` remains the legacy-root compatibility path.
- LocalStore/WebDAV uploads are atomically staged in their destination directory and accept at most `OPENREADER_MAX_IMPORT_BYTES` (default 128 MiB), so a rejected replacement does not truncate an existing file. A preview copies at most that same amount into `cache/import-previews/<user-id>/<random-token>.book` plus metadata; direct upload and every confirmation reread use the same bound. The cache location is user-private, token entropy is 192 bits, metadata expires after 24 hours, and success/expired-token access removes both files. It is safe to clear this derived cache; it is never part of the source file or library archive.
- A LocalStore/WebDAV preview retry that supplies an existing `importToken` reads that staged file directly, including when the mounted source was renamed, deleted, or changed after the first preview. A no-match custom TXT rule leaves the token in place, so a later retry/import uses the same bytes. An old client that does not send an `importToken` retains the path-based import fallback.
- Reader-dev-compatible TXT detection and fallback chunking apply only when a TXT is newly imported or explicitly refreshed/reparsed. Existing imported books, their SQLite rows, `chapters.json`, original archives, and chapter cache files are not rewritten during application startup or a Docker upgrade. This is intentionally a no-migration behavior change for future parsing operations; a user can choose an explicit local refresh/reparse for an old book.
- Remaining migration/release work: archive expanded-size/entry-count and parser-work limits, expiry cleanup independent of a future request, plus Docker mounted-volume verification.

## P2 replace-rule persistence compatibility

Status: implemented without a schema or mounted-volume migration.

- Existing `replace_rules` rows stay in the same SQLite table with the same `id`, `user_id`, `name`, `pattern`, `replacement`, `scope`, `is_regex`, `enabled`, and timestamps. No row is deleted, deduplicated, rewritten, or moved during startup.
- Reader-visible execution order is now the durable insertion order (`id ASC`) rather than the previous accidental `updated_at DESC` API order. Editing an existing row does not change its ID or its pipeline position. Backup writes the same `user_id, id` order, so restore into an empty database recreates the reader pipeline in the same sequence.
- Old rows whose nullable `is_regex` value is absent are interpreted as upstream's plain-text default (`false`) at read/execution time. This corrects a prior OpenReader default without changing the stored nullable value.
- Old rows with an empty scope remain global only as a read-compatibility shim for already-deployed OpenReader data. The new editor/API requires an explicit scope; the next successful edit/import writes `*` (or a book-specific scope) instead of another empty value.
- Backup restore accepts both `enabled` and legacy `isEnabled`. Missing `isRegex` restores as plain text; empty legacy scope stays readable through the shim. No new table/column and no `data/`, `cache/`, or `library/` path is introduced.
- New/updated inputs are bounded (name 120 bytes, scope 800 bytes, pattern 16 KiB, replacement 64 KiB) and regular expressions are compiled before a write. A rejected write leaves existing rows and mounted volumes untouched.

Required evidence: `backend/api/replace_rules_contract_test.go` covers defaults, ordering, scope compatibility and invalid regex rejection; full backend tests cover backup/restore. A Docker volume/backup smoke remains required before publishing this slice.

## P1-D4 book deletion, cache and refresh lifecycle

Status: extracted 2026-07-10; implementation must not add a destructive schema migration.

### Existing representation

- `books`, `chapters`, `book_categories`, `bookmarks`, and `reading_progress` are SQLite rows. Book/category/progress/bookmark rows are user scoped; chapter rows are owned by their book.
- Remote `Chapter.CachePath` is a relative path under `cache/`, calculated from the book/chapter URLs. A physical cache path can be referenced by more than one chapter row and must therefore be reference-checked before removal.
- Direct, LocalStore, and WebDAV imports are copied by `ArchiveImportedBook` into a private `library/data/<safe-user>/<unique-book>/` archive. `OriginalFile`, `chapters.json`, `bookSource.json`, `content/`, and derived EPUB/CBZ resources live under that archive.
- Browser chapter cache keys are user-scoped in current clients but are not database rows; they must be explicitly removed by the shelf/browser store when a book-delete sync event arrives.

### Required lifecycle and compatibility shim

1. Delete database relationships in one SQLite transaction. Do not remove physical files before commit.
2. After a successful commit, prune captured remote cache paths only when no remaining `chapters.cache_path` references them. Cleanup is idempotent and may be retried; it must never walk outside `cache/`.
3. Delete an imported archive only when `Book.LibraryPath` is a normalized descendant of that owner's `library/data/<safe-user>/` root. Do not delete a LocalStore/WebDAV source path merely because it was used to create an import copy. Preserve unrelated user archives and all mounted roots.
4. Refresh/source-change captures old derived cache references, commits the replacement chapter rows first, then prunes unreferenced stale entries. For local refresh retain `OriginalFile`, `chapters.json`, and `bookSource.json`; only replace/prune derived chapter content as required.
5. User-wide cache statistics and clear operations query only remote books belonging to the authenticated user. A clear operation resets only that user's chapter cache references in a transaction, then does reference-aware post-commit filesystem cleanup. It never returns an absolute cache-directory path.
6. Existing deployments may contain absolute/legacy cache paths. Treat them as read-compatibility candidates only; cleanup may remove them solely after resolving them safely below the appropriate `cache/` or owned book-library root. Unsafe/unresolved paths are left untouched and their rows are cleared only when the caller explicitly clears/deletes the book.

### P1-D4-B1 current implementation boundary (2026-07-11)

- Remote refresh and source change now replace `chapters` rows transactionally and reconcile the existing user-owned `reading_progress.chapter_id` and `bookmarks.chapter_id` fields in that transaction. The schema and JSON backup shape are unchanged; a removed catalogue index is represented by its existing position row with `chapter_id = 0`, so old backups and clients retain a recoverable index/offset.
- The former remote cache paths are captured before row replacement and are removed only after commit, only when they still resolve below `cache/`, and only when no remote chapter row references the same file. Existing cache volumes therefore remain readable after a failed fetch or SQLite transaction.
- Local refresh now writes a new content generation and its archive metadata into an inactive `.refresh-*` directory. The chapter/book/reference transaction commits before same-filesystem renames promote that content and metadata; previous unreferenced private `content/` files are then pruned. A forced staging failure removes only the inactive directory and leaves the previous rows, content, metadata and `OriginalFile` usable. Legacy local rows without a verified private archive use a new scoped cache generation and never delete an external LocalStore/WebDAV source.

### Backup and Docker impact

- No table, column, or mounted root changes. Existing `data/`, `cache/`, and `library/` volumes remain compatible.
- Backup/restore remains sufficient because original local imports and SQLite rows retain their existing paths/fields. Derived remote cache files need not be backed up.
- The release gate requires API fixtures for cross-user cache/delete isolation and `scripts/docker-volume-backup-smoke.sh` to prove a mounted volume survives restart after the cleanup changes.

## Reader `themeType` persisted-setting compatibility

Status: implemented and validated in the Reader custom-theme semantic mode slice.

### Existing representation

- Existing OpenReader Pinia and server-synchronized reader settings persist `theme`, custom colors, custom backgrounds, and `customConfigList`.
- Existing payloads and saved custom configurations do not contain `themeType`.
- Reader and shared shell night-state rendering currently infer night mode from `theme === "dark" || theme === "black"`.
- The value is stored inside the existing JSON reader setting and browser-persisted Pinia state. No SQLite column or filesystem path is dedicated to it.

### Additive representation

- Add `themeType: "day" | "night"` to:
  - default reader state;
  - server-synchronized reader setting payloads;
  - custom configuration snapshots and built-in configurations;
  - sanitized settings restored from Pinia/server JSON.
- Preset theme selection derives the value: `dark` and `black` become `night`; all other non-custom presets become `day`.
- Selecting `custom` preserves the current explicit `themeType`, matching reader-dev.
- The custom theme settings block lets the user explicitly choose `day` or `night`.

### Compatibility shim

- Old settings or custom configs with a missing/invalid `themeType` infer `night` when their saved theme is `dark` or `black`; all other themes infer `day`.
- Explicit valid `day`/`night` values are preserved for `custom` themes. Non-custom presets are always normalized from their preset identity, matching reader-dev `setConfig`.
- Sanitization applies the same rule to top-level settings and every custom configuration, so old built-in and user-created schemes remain readable.
- This is an additive JSON setting change only. It introduces no destructive SQLite migration, no new volume, and no changes under `data/`, `cache/`, or `library/`.

### Required migration evidence

- `frontend/tests/readerThemeType.test.mjs` proves old payload inference, explicit custom-value preservation, preset recalculation, custom preservation, payload/custom-config wiring, and semantic night rendering.
- Frontend full tests and production build pass with settings version `12`.
- `scripts/smoke/reader-mobile-contract.mjs` verifies custom `白天` / `黑夜` switching at desktop `1440×900`, mobile `390×844`, and mobile `360×800`; desktop settings and the mobile reader tool layer remain visible.
- A Docker volume/backup smoke remains required before publishing a release image even though this slice does not alter SQLite or filesystem data.

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

## P2 bookmark ordering and backup compatibility

Status: implemented in the bookmark API/Reader slice; browser interaction and Docker volume gates remain pending.

- No table, column, mounted root, or JSON filename changes. Existing `bookmarks` rows and `bookmarks.json` remain readable.
- Bookmarks now present/export in `id ASC` creation order. A note edit changes `updated_at` only and must never reorder the manager or a later backup.
- Modern OpenReader exports keep their original `createdAt`. Restore maps the exported book URL/title to the destination user's book, then uses the location, saved context, and `createdAt` as an idempotency identity. This preserves multiple independent bookmarks that share a chapter and offset while a repeat restore does not duplicate them.
- Older reader-dev/OpenReader rows without a creation timestamp use a narrower location/content identity for compatibility. Their original source IDs are not reused, so a destination database never suffers cross-user primary-key collisions.
- Restore safely drops a stale source `chapterId` and rebinds the destination book's chapter at the saved chapter index when it exists. Missing catalogue entries retain index/offset recovery with `chapterId: 0`.
- The change introduces no destructive migration and does not access `data/`, `cache/`, or `library/` outside the existing backup archive workflow.

Required evidence: `backend/api/bookmarks_contract_test.go` covers stable export order, independent same-location restore, repeat-restore idempotency, and destination chapter rebinding; full backend tests remain required. A Docker volume/backup smoke is still required before release.
