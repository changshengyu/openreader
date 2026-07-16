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

## P2 invalid-source runtime cache

Status: implemented and tested. This is a derived, caller-scoped runtime cache and is not part of a reader backup format.

- Existing `data/`, `cache/`, `library/`, `book_sources`, shelf, category, chapter, progress and backup records remain byte/schema compatible. A GORM migration may add only an additive `source_failures` SQLite table with a unique current-user/source key and expiry index.
- No existing source row is marked disabled, mutated or removed merely because one user saw a request failure. The 600-second failure status belongs only to that JWT user; another user can still use the global source.
- The table is intentionally excluded from backup/export/restore: it is a short-lived replacement for reader-dev's `storage/cache/invalidBookSourceCache/<userNameSpace>` files, not user-authored configuration.
- Read/write paths prune expired records and ignore a record whose retained source URL no longer matches the source's current URL after editing. Deleting a source may delete its derived records, but no old source, book, cache or mount file may be touched.
- Required evidence: upgrade an existing SQLite volume; verify no existing row changes; verify cross-user/expiry/edit/delete isolation; run full Go tests and Docker mounted-volume backup smoke.

Implementation evidence: `db.AutoMigrate` only adds `source_failures`; it never alters existing source/user/book rows. Records are created and expired under the JWT user/source unique key and are neither exported nor restored. `backend/api/source_failure_contract_test.go` verifies isolation, expiry and source-edit invalidation; release Docker volume smoke remains required before publishing this slice.

## P2-Parser-1G persistent source-rule variables

Status: implemented and migration-tested on 2026-07-13. The format is additive: old SQLite databases and backups remain valid, while new backups can preserve reader-dev-compatible remote parser state.

### Upstream and current representations

- Fixed `reader-dev@fa22f271849d45f93349ae1636223e27b16a4691` uses nullable JSON strings named `variable` on `SearchBook`, `Book`, and `BookChapter`. `RuleData` remains in-memory only, while `Book` and `BookChapter` serialize their maps after each write.
- `models.Book.Variable` and `models.Chapter.Variable` are additive SQLite text columns. Empty/NULL is an empty map; invalid values fail closed at the parser boundary before any remote fetch.
- `bookshelf.json` keeps its name and gains optional `sourceName`/`variable` fields. `chapterVariables.json` exports only remote chapter state with portable book/chapter identities.

### Additive format and restore rules

- `AutoMigrate` adds nullable `books.variable` and `chapters.variable` text columns only. The stored JSON is the bounded 1F string-to-string map: at most 32 entries, 128-byte keys, 4096-byte values and 16 KiB total; nested data is rejected.
- `bookshelf.json` retains its existing name and optional `sourceName`/`variable` properties. Optional `chapterVariables.json` contains the source name, book URL/title and chapter URL/index/title identities plus `variable`; cache paths, source headers, cookies and database IDs are never exported.
- Restore accepts old archives with neither field. It validates every new map before dispatch, resolves only the destination user's book/chapter, and never treats source/book/chapter IDs from the archive as reusable identity. Chapter values restore only after their shelf row exists, inside one transaction.
- A source change, source URL change, source rule-set change, source import update, clear or default restore clears variables for affected remote books/chapters in the same transaction. Local books export and restore empty variables.
- No mounted root or filesystem artifact is introduced. Cache keys do not include variables; invalid persistent input is a local source-rule error rather than a cache or source-failure mutation.

### Migration evidence

1. `db.AutoMigrate` is additive and retains existing rows; the normal full backend suite initializes fresh databases through the same migration path.
2. `backend/api/persistent_source_variables_contract_test.go` proves remote create/content persistence and atomic source-semantic clearing; all reads remain scoped by the owning user book.
3. `backend/api/backup_restore_contract_test.go` proves a generated backup restores Book and Chapter state into a different database/user while resolving the source by name; legacy fixtures without these fields remain in the full suite.
4. Full backend/frontend tests, production build and `scripts/docker-volume-backup-smoke.sh` are mandatory release gates for this format.

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
- P1-E3 changes only the workbench's visible file-manager operations and makes LocalStore upload accept multiple already-supported multipart file parts. It does not move a LocalStore/WebDAV root, rename any existing source file, alter a book/library archive, or add a SQLite table/column. Each accepted part is independently staged and atomically renamed in its existing caller-scoped directory; a failure leaves other successfully written selected files and every pre-existing destination intact, matching the upstream multi-upload's per-file side effects.
- Extra parser formats already stored in `library/`, LocalStore, WebDAV, SQLite book rows and old direct API clients remain readable. P1-E3 only stops advertising `.text/.md/.pdf` and WebDAV `.cbz` as new workbench import actions, so no data migration, cleanup, archive rewrite or background deletion is permitted.
- A LocalStore/WebDAV preview retry that supplies an existing `importToken` reads that staged file directly, including when the mounted source was renamed, deleted, or changed after the first preview. A no-match custom TXT rule leaves the token in place, so a later retry/import uses the same bytes. An old client that does not send an `importToken` retains the path-based import fallback.
- Reader-dev-compatible TXT detection and fallback chunking apply only when a TXT is newly imported or explicitly refreshed/reparsed. Existing imported books, their SQLite rows, `chapters.json`, original archives, and chapter cache files are not rewritten during application startup or a Docker upgrade. This is intentionally a no-migration behavior change for future parsing operations; a user can choose an explicit local refresh/reparse for an old book.
- Remaining migration/release work: Docker mounted-volume/backup verification for the completed bounded parser and restore paths.

## P2 import-parser limits and staged-preview cleanup

Status: parser and staged-preview cleanup implementation complete. Backup ZIP restore is documented and implemented in the following section; no SQLite or mounted-root migration is authorized.

- New parser-limit environment values must be additive with documented defaults. An unset deployment keeps the default bounded policy; existing `OPENREADER_MAX_IMPORT_BYTES` remains the byte limit before staging.
- Configured import limits apply while previewing, importing or explicitly reparsing new bytes. Existing `books`, `chapters`, `chapters.json`, archived originals, cached chapter content and reader progress are never scanned, rewritten or deleted by startup cleanup; lazy recovery of a pre-existing local archive uses a documented wider but still bounded compatibility ceiling instead of retroactively applying the new-upload policy.
- The preview cleanup worker operates only below `cache/import-previews/<user-id>/`. It may remove an expired token's `.book` and `.json` pair or a stale orphan created by an interrupted stage write. It must not remove a fresh valid pair, any LocalStore/WebDAV source, any `library/` archive, SQLite row or backup file.
- Parser rejection happens before `ArchiveImportedBook`, category mutation, chapter-row creation, sync broadcast or staged-token consumption. A rejected input leaves the mounted source and existing shelf untouched.
- Backup ZIP hardening is intentionally separate because it must preserve reader-dev/Legado/OpenReader JSON restore compatibility. This parser/cleanup slice adds no backup-format or restore behavior change.

Required evidence: malformed EPUB/UMD/PDF fixtures; valid-format regression fixtures; expired/fresh/orphan staged-file fixtures across user directories; configuration-default test; full backend tests and mounted-volume smoke after implementation.

Implementation evidence: parser-limit environment values are additive and defaulted; only newly previewed/imported/reparsed bytes use them. The cleanup worker removes only expired/invalid preview pairs and aged orphaned stage files under the existing derived cache root. `backend/engine/import_limits_contract_test.go`, `backend/services/localbook/importer_test.go`, `backend/api/workspace_import_stage_contract_test.go`, `backend/config/config_test.go`, and full `go test ./...` pass. The backup ZIP reader below retains every JSON format and shares the pending Docker mounted-volume smoke.

### UMD reader-dev binary parser follow-up (audited 2026-07-13)

The current UMD parser recognizes an early OpenReader-only `#TEXTNOV` layout, whereas the fixed reader-dev upstream writes and reads the segmented `0xde9a9b89` UMD format with UTF-16LE metadata/content and zlib body chunks. This is an import/parser compatibility correction, not a data migration.

- No startup scan, SQLite schema change, cache rewrite, library move or backup-format change is authorized. Existing imported books continue reading their archived chapter/cache files unchanged.
- Newly staged or explicitly refreshed `.umd` inputs must first use the bounded reader-dev parser. A narrowly isolated legacy fallback may remain only for an actual `#TEXTNOV` input, so historical OpenReader pseudo-UMD files are not made unreadable by the correction.
- The standard parser must cap segment count, declared chapter count, section lengths and total decompressed bytes before materializing title/offset/content arrays. Any failure occurs before `ArchiveImportedBook`, chapter rows, category writes, sync broadcasts or staged-token consumption.
- Direct upload, LocalStore and WebDAV retries continue to reuse the same immutable, caller-scoped staged bytes. Parsing never consults the original mounted path after staging, so observed catalogue failures cannot depend on network speed or later source-file changes.
- Required migration evidence is an existing-volume regression containing cached local books plus an unconsumed staged `.umd` preview: upgrade leaves the cached books intact, the staged standard UMD can be previewed/imported, and a failed reparse retains only its caller's retry token.

Implementation evidence: the runtime now recognizes the standard segmented reader-dev UMD stream first, parses its bounded UTF-16LE/zlib sections and retains the previous OpenReader-only prefix only as an isolated fallback. No model, schema, mounted-root, archive-path or backup-format write changed. `backend/engine/umd_parser_contract_test.go` verifies actual upstream writer framing (`F1` separators and final `81` table included); `backend/api/umd_import_contract_test.go` verifies direct staged upload plus LocalStore/WebDAV preview→confirm after the original mounted UMD has been deleted, then removes the derived chapter cache and proves reader recovery from the archived UMD. The same API contract verifies standard UMD `refresh-local` preserves its original archive while rebuilding ordered chapters, and a pre-existing `#TEXTNOV` archive row recovers lazily without import or migration. Corrupted compressed input retains only its scoped retry stage and returns no host path. Full Go tests pass for this evidence; the still-pending E4-VOLUME-1 Docker/backup smoke must cover a real historical SQLite volume and the remaining formats.

## P1-E4 old mounted local-book volume recovery

Status: compatibility inventory complete; fixture-first implementation pending. The focused
contract is [`local-book-old-volume-p1e4-contract.md`](local-book-old-volume-p1e4-contract.md).

- A recoverable installation is the mounted tuple `data/`, `cache/`, and `library/`, not a
  new database plus an application-level backup ZIP. `data/openreader.db` may retain old
  columns and WAL companions; `library/` retains each local book's original archive and
  metadata; `cache/` can contain only derived/staged data.
- Startup runs additive GORM migration followed by the historical local-cache migration.
  The latter may move a valid in-root local cache into the book's `library/.../content/`
  area, but must not rewrite original archive bytes, metadata, progress, bookmarks or an
  unrelated user's files.
- `OriginalFile`, `LibraryPath`, `TOCFile`, `SourceFile`, and `CachePath` in an old SQLite
  file are persisted input rather than authority to read arbitrary host paths. Historical
  absolute values require a private archive-root rebase only; direct absolute candidates,
  traversal and cross-user roots must fail closed without leaking a host path.
- The existing logical backup ZIP deliberately contains no `library/` archives or local
  chapter catalogue. A trigger/list/restore operation therefore must preserve an already
  mounted local book but cannot be described as standalone local-book recovery. Export or
  portable archive backup remains a separate compatibility item.

Required evidence: start a real old SQLite file and mounted TXT/EPUB/UMD/CBZ archives; remove
derived content; prove reading, scoped lazy recovery, refresh atomicity, unchanged original
hashes, user isolation, safe handling of stale absolute paths, and Docker stop/restart. The
release image must also run a backup trigger/list and safe restore that leaves the mounted
local archive and chapter rows unchanged.

## P2 backup ZIP restore compatibility and bounds

Status: implemented; release validation pending. Existing backup formats, SQLite rows and mounted roots remain readable.

- Valid OpenReader, reader-dev and Legado backup ZIPs continue to restore the existing sources, RSS sources, user settings, categories, shelf rows, progress, bookmarks and replace rules through their current scoped/upsert compatibility paths. The response count object and sync events remain compatible.
- A new bounded archive reader is runtime-only: it reads from the uploaded or scoped WebDAV ZIP, validates archive structure before dispatch, and never writes a new backup format, table, column, cache tree or library file.
- Structural archive failures (compressed cap, entry/path/count/size/total budget, duplicate canonical name, unreadable member) happen before mutation. Existing user rows and mounted files are left intact.
- Legacy nested progress filenames remain accepted only beneath a normalized `bookProgress/` path. Unsupported names are ignored after a valid archive plan is accepted; no user-controlled ZIP path is extracted to the host filesystem.
- Existing administrator legacy roots and regular-user private WebDAV roots remain unchanged. The backend `.zip` requirement affects only invalid direct restore requests, not existing valid backup files.

Required evidence: valid reader-dev/Legado/OpenReader fixtures, invalid archive fixtures with no database mutation, multipart and stored-WebDAV size rejection, restore broadcast/count regression, and Docker mounted-volume backup smoke.

Implementation evidence: uploaded and WebDAV recovery read a bounded compressed payload, then `backupRestoreArchive` validates and reads every member before any restore helper receives data. The restore dispatcher consumes that validated byte map rather than reopening ZIP members. `backend/api/backup_restore_contract_test.go` covers unsafe/over-budget structures, no-write structural failure, upload bounds and non-ZIP WebDAV targets; existing `api_test.go` and bookmark backup fixtures preserve reader-dev/Legado/OpenReader restore counts and records. Docker mounted-volume/backup smoke remains the release gate.

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
- E4-CBZ-1 keeps this persisted layout and SQLite schema unchanged. A CBZ's upstream-compatible first-image cover is derived from the bounded private archive walk only while serializing an import, shelf or detail response; the resulting signed resource URL and ZIP member path are never written back to `books`, `chapters.json`, `bookSource.json`, backups or WebDAV metadata. Old archives therefore remain readable without migration, while malformed/missing archives retain the existing empty-cover response.
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

## P1-B search preference compatibility

Status: extracted on 2026-07-13; implementation is pending. This is a JSON-setting read/write compatibility shim only. It does not add a table or modify mounted `data/`, `cache/`, or `library/` files.

### Existing representation

- The per-user SQLite `user_settings` row with `key = "search"` stores JSON unchanged; browser Pinia persistence and legacy `openreader_sidebar_search` also contain the same logical fields.
- Existing OpenReader payloads use `{searchType:"all"|"group"|"single", group, sourceId, concurrent}` and may contain `8`, `16`, `32`, or `60`.
- Existing backup restore deliberately preserves a `concurrent:32` payload; startup and restore must not rewrite that row.

### Read and write compatibility

- Missing, malformed, zero or negative concurrency reads as the upstream new-user default `24`.
- Canonical upstream values `12/18/24/30/36/42/48/54/60` are retained unchanged.
- Historical OpenReader values `8/16/32` remain readable and selectable as explicitly labelled legacy values. They are not silently reset; the first user-selected canonical value replaces them through the normal setting-write and conflict mechanism.
- `all`, `group`, and `single` remain stored because they are the deployed source-ID adapter for upstream multi/group/single selection. No migration changes `sourceId` or attempts to resolve a historical source URL at startup.
- No background update, Docker upgrade, backup restore, scope switch, or login is allowed to write a new search preference merely because it was read. Only the existing explicit user preference save path writes normalized JSON.

### Required evidence

- Frontend tests must cover defaults, canonical values, legacy 8/16/32 preservation, server preference loading and explicit replacement.
- Existing backup restore tests retain `concurrent:32`; an added test verifies loading that restored setting does not reset it.
- The release gate runs the existing Docker volume/backup smoke to confirm the unchanged `user_settings` and mounted volume survive restart.

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
- Older installed EPUB chapter rows may not retain canonical XHTML paths or fragment boundaries from the archive.

### Additive representation

- Add nullable/empty `Chapter.ResourcePath` (`resourcePath` in JSON) through GORM auto-migration. It stores a normalized POSIX EPUB path such as `OEBPS/Text/chapter-1.xhtml`; it is never an absolute host path.
- Add optional `resourcePath` to archived `chapters.json` entries. Old archives without it remain valid.
- E4-EPUB-2 additionally adds nullable `Chapter.ResourceFragment` and `Chapter.ResourceEndFragment` (`resourceFragment` and `resourceEndFragment` in JSON and `chapters.json`). They hold bounded decoded DOM ids only; they never form filesystem paths. A missing value preserves the current whole-XHTML behavior for old rows/backups.
- EPUB import writes both:
  - the existing plain-text `CachePath`;
  - the canonical XHTML `ResourcePath` and, when a TOC/NCX entry targets it, nullable fragment bounds.
- Existing imported EPUBs are lazily backfilled from the archived original file and current TOC rule when first opened/refreshed. Backfill updates only matching chapter rows and the optional portable `chapters.json` metadata.

No table or column is removed. Text, PDF, UMD, Markdown, remote, and existing EPUB rows remain readable when `ResourcePath` is empty.

Migration evidence: `TestAutoMigrateAddsEPUBResourcePathWithoutLosingChapters` drops the three EPUB
resource columns from a populated SQLite fixture and proves auto-migration restores them without changing
the existing chapter. `TestDirectEPUBTOCFragmentsImportAsBoundedReaderChapters` proves a legacy empty
fragment row and its `chapters.json` companion are lazily restored from the archived EPUB. Docker mounted
volume/backup smoke remains required before publishing the release image.

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
