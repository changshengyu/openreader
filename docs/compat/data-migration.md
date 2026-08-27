# OpenReader Data Migration and Storage Contract

Status: working compatibility ledger; implemented migrations and remaining action-level audits are recorded below.

## User-facing migration contract (2026-08-11)

The English and Chinese READMEs now expose three distinct, supported migration paths. These are documentation
of existing behavior and do not authorize a new schema or filesystem migration:

| Source | Supported path | Explicit boundary |
|---|---|---|
| reader-dev/Legado | Upload the original logical `backup*.zip` to the authenticated user's WebDAV root and restore it through the single WebDAV file manager. | Never reuse the upstream SQLite database, IDs, accounts, passwords, JWT state or host paths. Upstream logical backups do not contain local-book originals. |
| OpenReader account/host | Use portable v2 for an account-level move. | Portable packages are caller-scoped, fail closed when a required archive/asset is unavailable, and exclude local audio directories. |
| Complete OpenReader instance | Stop the service and copy `data/`, `cache/` and `library/` together with deployment configuration. | A partial root copy is not described as a complete migration. Preserve `OPENREADER_JWT_SECRET` only when existing sessions should remain valid. |

An in-place OpenReader upgrade retains the same three mounts and relies only on already-reviewed additive startup
migrations. The documented rollback is a complete pre-upgrade volume snapshot plus the prior image; it never merges
an old database into directories already written by a newer container. The README also distinguishes ordinary
logical `backup_*.zip`, OpenReader `portable_backup_*.zip`, and a cold three-root system snapshot so users do not
mistake logical reader-dev compatibility for full filesystem portability.

## P2 reading-progress CAS, WebDAV mirror and request boundary

The implemented 2026-07-18 fixed-baseline contract in
[`reading-progress-p2-contract.md`](reading-progress-p2-contract.md) and the pending second-audit request contract in
[`reading-progress-request-boundary-fixed-baseline-second-audit-p2-contract.md`](reading-progress-request-boundary-fixed-baseline-second-audit-p2-contract.md)
do not authorize a schema migration. `reading_progresses` retains its existing `(user_id,book_id)` unique index and
precise chapter/offset/percent fields. Atomicity uses conditional writes against the existing row ID and `updated_at`,
not a replacement table or destructive migration.

The implemented upstream-compatible live progress mirror is additive filesystem output only. It may
write a safe `bookProgress/<book>_<author>.json` or `legado/bookProgress/...` file when that
directory already exists in the caller's WebDAV root. It must not move/delete historical files,
create the feature directory implicitly, cross the administrator/regular-user root boundary, or
change `readingProgress.json` backup/restore behavior.

## Persistent roots

| Root | Purpose | Compatibility rule |
|---|---|---|
| `data/` | SQLite database, uploads, WebDAV backup directory. | Must survive container upgrade. |
| `cache/` | Chapter/content cache. | May be regenerated, but broad deletion requires explicit user action or migration note. |
| `library/` | Imported original files and local store. | Must not be moved or deleted without migration. |

## P2 raw WebDAV protocol compatibility (audit pending implementation)

[`webdav-protocol-p2-contract.md`](webdav-protocol-p2-contract.md) adds protocol routes and authentication,
not a storage migration. `/webdav/*` and the upstream-compatible `/reader3/webdav/*` must resolve to the
same existing caller-scoped directory: administrators keep the historical `data/webdav/` root and regular
users keep `data/webdav/users/<safe-username>/`. No startup scan, move, copy, rename or cleanup is allowed.

Basic authentication validates the existing bcrypt `users.password_hash`; it adds no credential column,
token file or setting. Stateless LOCK tokens are response-only and never enter SQLite, mounted volumes,
backups or logs. COPY/MOVE/MKCOL/PUT may change only paths explicitly requested under the authenticated
caller's existing root, after traversal and symlink rejection. The Docker gate must mount a historical
administrator root and two regular-user roots, exercise both URL prefixes, restart, and prove that backup
and import files remain in place and isolated.

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

## P2 BookInfo custom assets (implemented without a migration)

`docs/compat/bookinfo-shelf-mutations-p2-contract.md` defines the migration-free
move from global new upload writes to user-rooted new asset paths. It does **not**
authorize moving or deleting existing `data/uploads/<kind>/...` files: legacy
Book/setting/backup URL strings must remain readable from mounted `data/` after
an upgrade. New `/uploads/users/<user-id>/<kind>/...` values use the existing
string fields and require no SQLite schema change; ownership is derived from the
authenticated user and rooted filesystem path. Go API tests verify legacy/static
readability, user-rooted writes, cross-user rejection and referenced-resource
deletion refusal; the BookInfo real-browser contract verifies the visible
cover/follow/group/local-refresh flow at the three release viewports. Docker
volume/backup verification must cover both legacy and new paths before release.

### 2026-07-23 custom asset backup correction

The preceding statement proves mounted-`data/` volume survival and URL-string
compatibility only. Ordinary logical backups and `openreader-portable-v1`
currently serialize Reader/Book URL fields but do not package
`data/uploads/users/<user-id>/` bytes or rewrite user IDs on restore. They must
not be described as cross-instance custom-asset restore. Runtime upload
consistency is P2-A; P2-B is the separately versioned, bounded and transactional
asset format in `docs/compat/portable-appearance-assets-p2b-contract.md`.
P2-B runtime is now implemented: new packages are v2, existing v1 remains
restorable without interpreting placeholders, and ordinary logical ZIPs remain
URL-only. Restoration allocates new target-user files and URLs; it does not move,
rename or delete existing uploads, SQLite rows, backup ZIPs or mounted files.
Docker new/old volume proof remains a release gate.

## Priority unresolved areas

- Reader-dev backup format import/export mapping.
- Source rule format and default source persistence.
- Reader progress and bookmark migration semantics.
- Local store/WebDAV path normalization and permissions.
- Cache invalidation rules for local and remote books.

## P2-S1 book-source ownership association migration

Status: **implemented, migration/browser/Docker validated and published in `0db752e`**. The
2026-07-28 evidence audit reopened the old fixture, then added a true global-source database and
closed the dedicated migration/COW/backup/restart gate before publication.

- Existing `book_sources` rows remain in place. The additive `user_book_sources` table records active
  or detached visibility, while `book_source_namespaces` distinguishes uninitialized users from an
  explicitly empty source list.
- Upgrade from the global model creates active associations for every existing user without changing
  source, book, or source-failure IDs. If legacy sources exist, user ID zero receives the initial
  default-template associations.
- `schema_migrations` stores `book-source-ownership-v1` in the same transaction as namespace and
  association rows. A failed association write rolls back the marker and can be retried on restart.
- A pre-existing user with zero global sources still gets a namespace marker; the default namespace
  stays absent until a default template exists.
- Contract evidence:
  `backend/db/book_source_ownership_migration_contract_test.go`. Source CRUD/search/reader/backup
  now use these associations through P2-S2…S4.
- The release fixture must represent the actual old format, not a database that has already recorded
  `book-source-ownership-v1`: it contains `users`, global `book_sources`, caller-owned books and
  `source_failures` referencing those source IDs, while `user_book_sources`,
  `book_source_namespaces` and the migration marker do not exist. Opening that mounted volume must
  create associations for every existing user and user zero without rewriting any old source/book/
  failure ID.
- The fixture remains additive to the established TXT/EPUB/UMD/CBZ/relative-cache old-volume data.
  It must not delete or simplify those records to make the ownership test easier.
- Required Docker evidence is API-visible COW plus stopped-volume SQLite inspection: one user's edit
  remaps only that user's association/book/failure; the other user and default template retain the old
  snapshot. Logical and portable archives from administrator legacy root and regular-user private root
  must contain only caller-active sources, survive restore and remain isolated after restart.
- Final evidence: `scripts/docker-source-ownership-smoke.sh` and both modes of
  `scripts/docker-volume-backup-smoke.sh` pass against the locally built `0db752e` image. The remote
  amd64/arm64 index is
  `sha256:83f53fe3aa523fc1196454d4c5f1d413648eb72ad1e87c83c838e7200859207e`.

## P2 invalid-source runtime cache

Status: implemented and tested. This is a derived, caller-scoped runtime cache and is not part of a reader backup format.

- Existing `data/`, `cache/`, `library/`, `book_sources`, shelf, category, chapter, progress and backup records remain byte/schema compatible. A GORM migration may add only an additive `source_failures` SQLite table with a unique current-user/source key and expiry index.
- No existing source row is marked disabled, mutated or removed merely because one user saw a request failure. The 600-second failure status belongs only to that JWT user. The current global source row remains unchanged until the owner migration; after
  [`book-source-ownership-p2-contract.md`](book-source-ownership-p2-contract.md), each failure row
  is authorized through that user's private association. Only a later copy-on-write edit may remap that
  user's failure row; it cannot suppress or mutate another user's association.
- The table is intentionally excluded from backup/export/restore: it is a short-lived replacement for reader-dev's `storage/cache/invalidBookSourceCache/<userNameSpace>` files, not user-authored configuration.
- Read/write paths prune expired records and ignore a record whose retained source URL no longer matches the source's current URL after editing. Deleting a source may delete its derived records, but no old source, book, cache or mount file may be touched.
- Required evidence: upgrade an existing SQLite volume; verify no existing row changes; verify cross-user/expiry/edit/delete isolation; run full Go tests and Docker mounted-volume backup smoke.

Implementation evidence: `db.AutoMigrate` only adds `source_failures`; it never alters existing source/user/book rows. Records are created and expired under the JWT user/source unique key and are neither exported nor restored. `backend/api/source_failure_contract_test.go` verifies isolation, expiry and source-edit invalidation; release Docker volume smoke remains required before publishing this slice.

## P0 Reader source-candidate derived cache

Status: implemented, migration-tested and Docker-published as `a2ecc17` on 2026-08-11.

- Startup may add only a `book_source_candidates` table and its indexes. Existing users, books, sources,
  chapters, progress, bookmarks, `data/`, `cache/`, and `library/` content must not be rewritten.
- Each row is scoped by `user_id` and `book_id`, with a unique current-user/book/`book_url` identity. It stores
  the bounded source/book projection needed by Reader source switching plus stable order and timestamps.
- Remote-book creation seeds the current row in the same SQLite transaction. Old books are not bulk rewritten;
  their first `available` read seeds the current snapshot idempotently.
- Book deletion removes that book's rows in the same transaction. User deletion removes all rows for that user.
  Source copy-on-write may remap only the affected user's candidate source ids; source deletion never deletes
  another user's snapshots.
- A successful source change upserts the selected/current snapshot in the same transaction as book and chapter
  replacement. `books.source_id + books.url` remains authoritative; no persisted `current` flag is introduced.
- Candidate rows are derived cache and are deliberately excluded from reader-dev logical backup, OpenReader
  portable backup, WebDAV backup and restore payloads. Restored books seed on first `available` access.
- Field lengths and rows per book are bounded. Stable oldest non-current rows are pruned at the cap; the current
  source snapshot is never pruned.

Required evidence: fresh and historical `AutoMigrate`, idempotent historical seeding, two-user isolation,
book/user deletion cleanup, backup member stability, restore-then-seed, full Go tests and mounted-volume smoke.

Implementation evidence: startup `AutoMigrate` adds only `book_source_candidates`; the historical migration
test verifies existing book identity is unchanged and no candidate rows are eagerly created. API/service contracts
verify first-available seeding, remote-book and source-change transactional seeding, user/book deletion, source COW,
200-row pruning with current retention, two-user isolation, and backup exclusion. Full Go and focused race tests pass.
The local `a2ecc17` image passed fresh portable-v1/v2-assets/cross-user/restart and the successful sequential
historical TXT/EPUB/UMD/CBZ/relative-cache/owner-isolation mounted-volume gate before publication.

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

- The mapping is determined from the authenticated persisted user after the relevant LocalStore or WebDAV authorization. It does not rewrite old paths.
- Scheduled backup runs once per persisted user and filters all user-owned export rows. The authenticated administrator
  trigger keeps the legacy root path but must pass the administrator `userID` into logical export; a root location is
  not authority to include other users. The ownerless `RunNow()` helper must not be used by an authenticated API path.
- LocalStore/WebDAV uploads are atomically staged in their destination directory and accept at most `OPENREADER_MAX_IMPORT_BYTES` (default 128 MiB), so a rejected replacement does not truncate an existing file. A preview copies at most that same amount into `cache/import-previews/<user-id>/<random-token>.book` plus metadata; direct upload and every confirmation reread use the same bound. The cache location is user-private, token entropy is 192 bits, metadata expires after 24 hours, and success/expired-token access removes both files. It is safe to clear this derived cache; it is never part of the source file or library archive.
- The 2026-07-18 parsed-result lifecycle correction adds an optional versioned `<random-token>.parsed.json` beside the existing `.book`/`.json` pair. It contains only bounded parser output, the exact normalized extension/rule and a SHA-256 binding to the staged raw bytes. It is derived caller-scoped cache, not a database or backup format. Existing two-file stages remain valid and create the third file lazily after their next successful preview. A failed reparse cannot replace the last successful snapshot; expiry, explicit cache clearing and successful token consumption remove all three files. No existing `data/`, `library/`, SQLite row, archived source or chapter cache is rewritten.
- P1-E3 changes only the workbench's visible file-manager operations and makes LocalStore upload accept multiple already-supported multipart file parts. It does not move a LocalStore/WebDAV root, rename any existing source file, alter a book/library archive, or add a SQLite table/column. Each accepted part is independently staged and atomically renamed in its existing caller-scoped directory; a failure leaves other successfully written selected files and every pre-existing destination intact, matching the upstream multi-upload's per-file side effects.
- The 2026-08-12 LocalStore boundary implementation adds only future-request admission and rooted filesystem checks: no SQLite schema/row, mounted root, source file, library archive, import token, backup member, or browser key is rewritten. Existing symlinks and special files are left in place but hidden/rejected by the API; immutable staged-token confirmation remains independent of later source removal. Fresh/historical/portable mounted-volume verification is still required before publishing its Docker image.
- The implemented 2026-08-16 WebDAV import/restore boundary authorizes no migration. It adds only future-request admission plus caller-private derived snapshots below existing cache roots. Existing `data/webdav/` files, backups, symlinks and special files remain untouched; source-backed reads reject or skip unsafe entries, token-only import remains source-independent, and a restore snapshot is deleted after the request rather than written back to the mounted ZIP. Contract/red-test/implementation evidence is `cf46e22`/`1bb904a`/`616a076`; `65a9870` passed fresh/historical `data/cache/library`, logical/portable v1/v2, cross-user and restart gates before local amd64/arm64 publication.
- The implemented direct browser multi-select/multipart second audit authorizes no migration. It changes only future direct-request admission and frontend orchestration: the existing single-object API, two-/three-file stages, 24-hour token cleanup, SQLite rows, library archives, hidden legacy parser formats, backup members and mounted roots remain unchanged. Selected files are previewed one at a time and confirmed only by their caller-scoped token; cancelled tokens remain derived cache and expire normally. Contract/red-test/implementation/runtime evidence is `279f688`/`cd8f073`/`05343ec`/`3b9ae54`; `429444a` passed fresh/historical `data/cache/library`, logical/portable v1/v2, cross-user and restart gates before local amd64/arm64 publication.
- Extra parser formats already stored in `library/`, LocalStore, WebDAV, SQLite book rows and old direct API clients remain readable. P1-E3 only stops advertising `.text/.md/.pdf` and WebDAV `.cbz` as new workbench import actions, so no data migration, cleanup, archive rewrite or background deletion is permitted.
- A LocalStore/WebDAV preview retry that supplies an existing `importToken` reads that staged file directly, including when the mounted source was renamed, deleted, or changed after the first preview. A no-match custom TXT rule leaves the token in place, so a later retry/import uses the same bytes. An old client that does not send an `importToken` retains the path-based import fallback.
- Reader-dev-compatible TXT detection and fallback chunking apply only when a TXT is newly imported or explicitly refreshed/reparsed. Existing imported books, their SQLite rows, `chapters.json`, original archives, and chapter cache files are not rewritten during application startup or a Docker upgrade. This is intentionally a no-migration behavior change for future parsing operations; a user can choose an explicit local refresh/reparse for an old book.
- Remaining migration/release work: Docker mounted-volume/backup verification for the completed bounded parser and restore paths.

## P2 import-parser limits and staged-preview cleanup

Status: parser and staged-preview cleanup implementation complete. Backup ZIP restore is documented and implemented in the following section; no SQLite or mounted-root migration is authorized.

- New parser-limit environment values must be additive with documented defaults. An unset deployment keeps the default bounded policy; existing `OPENREADER_MAX_IMPORT_BYTES` remains the byte limit before staging.
- The follow-up TXT/Markdown budget is additive and migration-free. `OPENREADER_MAX_PARSED_CHAPTERS`
  limits final parser output for new/explicit parse work; when unset it follows an explicitly configured legacy
  `OPENREADER_MAX_UMD_CHAPTERS` value, otherwise defaults to 100000. Existing rows, archives and caches are not
  scanned on startup. Historical lazy recovery uses the wider bounded compatibility policy and must reject an
  over-ceiling source before reading the complete file into memory.
- Implementation keeps this no-migration contract: no model, `AutoMigrate`, backup schema, stage filename or mounted
  root changed. The new generic chapter field exists only in process configuration/parser limits, while historical
  refresh and lazy reconstruction retain the existing source path and use a bounded reader before allocation.
- Release `e7f168e` passed both fresh portable-v1/v2/cross-user/restart and historical
  TXT/EPUB/UMD/CBZ/relative-cache/owner-isolation mounted-volume gates.
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

Status: old-SQLite/path/cache implementation, legal relative-cache migration, full-format and
cross-user Docker fixtures complete. The additive portable archive backup runtime is also complete;
its separate extension contract is
[`portable-local-archive-backup-p1e4-contract.md`](portable-local-archive-backup-p1e4-contract.md).
The mounted-volume contract remains [`local-book-old-volume-p1e4-contract.md`](local-book-old-volume-p1e4-contract.md).

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
  mounted local book but cannot be described as standalone local-book recovery. The implemented
  `openreader-portable-backup` v1 ZIP is an explicit, separately named OpenReader extension;
  it does not alter the legacy ZIP's entry set, restore path or reader-dev/Legado compatibility.

Required evidence: start a real old SQLite file and mounted TXT/EPUB/UMD/CBZ archives; remove
derived content; prove reading, scoped lazy recovery, refresh atomicity, unchanged original
hashes, user isolation, safe handling of stale absolute paths, and Docker stop/restart. The
release image must also run a backup trigger/list and safe restore that leaves the mounted
local archive and chapter rows unchanged.

Implementation evidence: `backend/api/old_volume_contract_test.go` creates an old SQLite
file without the current EPUB resource/fragment/variable columns, closes it, then performs the
production migration order before serving a deleted-cache local chapter. It locks old
progress/bookmark preservation, archive-root-only rebasing of a stale absolute source/cache path
and cross-user 404 behavior. `backend/db.TestMigrateLocalBookCacheSkipsUnsafeHistoricalCachePath`
locks that startup never copies or deletes `cache/../...` host data. The local
`HISTORICAL_VOLUME=1` Docker smoke creates the same kind of old SQLite fixture with relative-path
TXT plus stale-absolute EPUB, standard reader-dev UMD and CBZ archives, then adds a distinct book
whose legal `cache/legacy-cache/chapter.txt` must become private relative
`content/legacy-cache/chapter.txt`. It mounts readable `/retired-host` decoys, proves every archive
can recover, refresh, survive backup/restore without mutation and remain readable after restart,
and verifies the relative cache bytes, source removal and persisted SQLite field before and after
restart; the ordinary fresh-volume smoke also passes. Full Go tests, frontend tests and production
build pass. The fixture also contains an already-existing second user with an independent archive;
real JWT requests verify mutual list/read/refresh isolation, and the owner backup restore/restart
leaves the other user's archive and chapter cache path unchanged. This completes the all-format,
relative-cache and cross-user Docker portions. Portable archive recovery is additionally proven by
the second fresh-volume leg of that same smoke: it transfers the owner-only TXT/EPUB/UMD/CBZ
archives plus logical metadata to new `data/cache/library` mounts, validates hash/read/refresh and
restarts the destination without importing stale cache or a second user's data. The validated old-volume slice was published from
Git `c7d5abb` as `ghcr.io/changshengyu/openreader:c7d5abb` and `:latest`, both pointing to
multi-architecture index `sha256:d7000822b4a135c3ee9ab12c4cbef5c5343cfc87c125cc3e5f05f52098d46fa7`.
`TestHistoricalMountedVolumeRebuildsEPUBUMDAndCBZArchives` additionally covers stale absolute
archive paths and missing derived content for all remaining E4 formats at the API boundary; the
container fixture now exercises the equivalent four-format volume.

VOLUME-CACHE-3 now moves one legal relative cache into the owning archive's `content/` directory
while persisting a relative `content/...` cache path. Existing absolute, traversal and symlink
values remain fail-closed; copy-before-database-update/delete prevents a failed SQLite write from
losing the only readable cache. This is an OpenReader mounted-volume compatibility/security
requirement, not an upstream reader-dev storage behavior.

VOLUME-OWNER-5 now places two already-existing users and independent private local archives in the
old SQLite fixture. Real HTTP/JWT tests and Docker smoke prove list/read/refresh are mutually 404
across users, while one user's backup restore and restart leave the other user's archive, chapter
cache path and readability unchanged.

## P2 backup ZIP restore compatibility and bounds

Status: archive structure/bounds plus ordinary backup aliases, stable generation, transactional content restore and source permission are implemented under the 2026-07-22
[`backup-restore-fixed-baseline-p2-contract.md`](backup-restore-fixed-baseline-p2-contract.md); browser/Docker release validation is pending.

- Old OpenReader plural aliases, reader-dev singular `bookmark.json`/`replaceRule.json`, `bookshelf.json`, Legado `myBookShelf.json` and nested progress remain readable. The restore planner prefers richer OpenReader aliases when both are present and executes each logical bookmark/rule artifact once.
- A new bounded archive reader is runtime-only: it reads from the uploaded or scoped WebDAV ZIP, validates archive structure before dispatch, and never writes a new backup format, table, column, cache tree or library file.
- Structural archive failures (compressed cap, entry/path/count/size/total budget, duplicate canonical name, unreadable member) happen before mutation. Existing user rows and mounted files are left intact.
- Legacy nested progress filenames remain accepted only beneath a normalized `bookProgress/` path. Unsupported names are ignored after a valid archive plan is accepted; no user-controlled ZIP path is extracted to the host filesystem.
- Existing administrator legacy roots and regular-user private WebDAV roots remain unchanged. The backend `.zip` requirement affects only invalid direct restore requests, not existing valid backup files.

Required evidence: valid reader-dev/Legado/OpenReader fixtures, invalid archive fixtures with no database mutation, multipart and stored-WebDAV size rejection, restore broadcast/count regression, and Docker mounted-volume backup smoke.

Implementation evidence: uploaded and WebDAV recovery read a bounded compressed payload, then `backupRestoreArchive` validates and reads every member before planning. `backend/api/backup_restore_contract_test.go` and `backup_fixed_baseline_contract_test.go` cover unsafe/over-budget structures, no-write structural and typed-content failures, database rollback, upload bounds, non-ZIP WebDAV targets, atomic generation failure, source-edit permission, singular aliases, alias deduplication and newest-progress merge. Browser and mounted-volume Docker evidence remain mandatory before release.

## P2 replace-rule persistence compatibility

Status: fixed-baseline persistence contract implemented and regression-tested on 2026-07-28. The
additive schema is retained; ordering and restore identity now follow the fixed contract. See
[`replace-rule-fixed-baseline-p2-contract.md`](replace-rule-fixed-baseline-p2-contract.md).

- Existing `replace_rules` rows stay in the same SQLite table with the same `id`, `user_id`, `name`, `pattern`, `replacement`, `scope`, `is_regex`, `enabled`, and timestamps. AutoMigrate only adds `group_name` and `sort_order DEFAULT 0`; no row is deleted, deduplicated, rewritten, or moved during startup.
- Reader-visible list, execution and backup order is `id ASC`, the SQLite equivalent of the upstream
  JSON array insertion order. `sort_order` remains stored/exported for round-trip compatibility but
  must not reorder the fixed Web Reader pipeline.
- Old rows whose nullable `is_regex` value is absent are interpreted as upstream's plain-text default (`false`) at read/execution time. This corrects a prior OpenReader default without changing the stored nullable value.
- Old rows with an empty scope remain global only as a read-compatibility shim for already-deployed OpenReader data. The new editor/API requires an explicit scope; the next successful edit/import writes `*` (or a book-specific scope) instead of another empty value.
- Backup restore accepts both `enabled` and legacy `isEnabled`. It processes archive rows in order and
  upserts by the caller's exact rule name, not pattern. Missing/empty names or patterns are skipped,
  never synthesized; missing `isRegex` restores as plain text and an external empty scope becomes
  explicit `*`. Restore accepts at most 2,000 rows and prevalidates every non-empty row before its
  first write, so invalid field/RE2/capture data participates in the existing whole-backup rollback.
  No new table/column and no `data/`, `cache/`, or `library/` path is introduced.
- New/updated inputs are bounded (name 120 bytes, hidden group 800 bytes, scope 800 bytes,
  pattern 16 KiB, replacement 64 KiB, regex captures 32) and regular expressions are compiled before
  a write. A rejected API or restore write leaves existing rows and mounted volumes untouched.

Required evidence: rewritten tests must cover exact whitespace preservation, ID order despite nonzero
`sort_order`, exact scope second-segment behavior, name-based ordered restore, invalid-row skipping,
JavaScript replacement tokens and old duplicate-name preservation. Full backend/frontend/browser and
Docker mounted-volume/backup gates remain required before publishing this slice.

## P1-D4 book deletion, cache and refresh lifecycle

Status: extracted 2026-07-10; implementation must not add a destructive schema migration.

### Existing representation

- `books`, `chapters`, `book_categories`, `bookmarks`, and `reading_progress` are SQLite rows. Book/category/progress/bookmark rows are user scoped; chapter rows are owned by their book.
- Remote `Chapter.CachePath` is a relative path under `cache/`, calculated from the book/chapter URLs. A physical cache path can be referenced by more than one chapter row and must therefore be reference-checked before removal.
- Direct, LocalStore, and WebDAV imports are copied by `ArchiveImportedBook` into a private `library/data/<safe-user>/<unique-book>/` archive. `OriginalFile`, `chapters.json`, `bookSource.json`, `content/`, and derived EPUB/CBZ resources live under that archive.
- E4-CBZ-1 keeps this persisted layout and SQLite schema unchanged. A CBZ's upstream-compatible first-image cover is selected during one bounded extraction into `library/<Book.LibraryPath>/.cbz-resources/<sha256>/`; the resulting signed resource URL and derived path are never written back to `books`, `chapters.json`, `bookSource.json`, backups or WebDAV metadata. Old archives remain lazy-readable without migration, while malformed/missing archives with no complete generation retain the existing empty-cover response.
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
- E4-EPUB-2 additionally added nullable `Chapter.ResourceFragment` and `Chapter.ResourceEndFragment` (`resourceFragment` and `resourceEndFragment` in JSON and `chapters.json`). They hold bounded decoded DOM ids only; they never form filesystem paths. The 2026-07-18 fixed-baseline correction keeps these columns solely for already published rows/staged snapshots; newly parsed local EPUB catalogues leave both empty and use whole-XHTML behavior.
- Newly parsed EPUB import writes both the existing plain-text `CachePath` and canonical XHTML
  `ResourcePath`, with empty fragment fields. An upgrade-time prepared snapshot created by the prior version
  remains confirmable and may still write its historical fragment metadata; rejecting it as `invalid token`
  is forbidden.
- Existing imported EPUBs are not collapsed on startup/read. Missing canonical paths may still be lazily backfilled without changing chapter count. Recovery first uses the persisted TOC rule; for a legacy row whose path is missing, pure `toc` produced no chapters, and the archive has a readable spine, runtime recovery alone may locate the same index through `spin`. This does not change new pure-`toc` preview/import/refresh semantics. Only an explicit `refresh-local` rebuilds the catalogue by canonical href; before replacement, old progress/bookmarks with a known EPUB resource path are mapped to the matching new row, while unknown references retain their existing index/offset and clear only an invalid row id.

No table or column is removed. Text, PDF, UMD, Markdown, remote, and existing EPUB rows remain readable when `ResourcePath` is empty.

Migration evidence: `TestAutoMigrateAddsEPUBResourcePathWithoutLosingChapters` drops the three EPUB
resource columns from a populated SQLite fixture and proves auto-migration restores them without changing
the existing chapter. EPUB-FIXED-2/3 replace the former incorrect “new fragment import” assertion: they must
prove new imports write href-deduped rows, historical fragment rows/staged snapshots remain readable, and
explicit refresh performs resource-aware reference reconciliation. Docker mounted-volume/backup smoke
remains required before publishing the release image.

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
- Old pure-`toc` books without NAV/NCX: if a persisted row also lacks `resourcePath`, only that runtime
  recovery may use the spine at the same index and backfill the normalized path. New pure-`toc` catalogues stay empty.
  A default explicit refresh therefore rejects the empty replacement without mutating the old rows; an explicit
  `spin` refresh may then rebuild and persist the spine catalogue.
- Missing derived directory: rebuild transparently from `OriginalFile`.
- Missing/corrupt source EPUB: preserve all database rows and plain-text caches; return a reader error instead of deleting/reimporting the book.
- Backup/restore and WebDAV: the existing original EPUB and metadata remain sufficient. Derived `.epub-resources/` need not be present in a backup to recover the book.

### EPUB catalogue-only preview and prepared extraction compatibility (2026-07-18)

- The staged `<token>.parsed.json` remains versioned plain JSON at the same user-scoped cache path. A new
  EPUB snapshot omits chapter `content`, stores one entry per canonical href and leaves fragment metadata empty.
  Older full-content and catalogue-only snapshots—including snapshots containing the prior multi-fragment
  catalogue—remain valid input to confirmation; absence of body content is not an invalid token and does not
  require a browser re-upload.
- New EPUB confirmation may create `.epub-resources/<sha256>/` under the newly allocated book archive before
  committing its SQLite rows. This is derived data at the existing location, not a new mounted root or backup
  requirement. If extraction or transaction work fails, the existing new-archive compensation removes that
  whole allocation; no old archive or mounted LocalStore/WebDAV source is modified.
- A complete extraction marker continues to record the SHA-256 fingerprint plus archived source size and
  modification time. Matching size/mtime can select this process-created immutable extraction without hashing
  again; a changed source identity, malformed marker, missing resource, or old marker falls back to the existing
  bounded SHA-256 and atomic rebuild path.
- Old books with no extraction, an old plain-fingerprint marker, missing chapter cache, or missing fragment
  columns remain lazy-readable. EPUB chapter cache recovery must derive only the requested chapter from the
  verified resource tree; it must not rewrite unrelated rows or require a destructive migration.

Evidence completed 2026-07-18: old/new parsed snapshot confirmation, failed-confirm compensation, old marker and
no-marker fixtures, source-replacement capability invalidation, full backend tests, and the local
`HISTORICAL_VOLUME=1` mounted-volume/portable-backup smoke. The source EPUB remains authoritative; derived
extraction may always be deleted and rebuilt.
- Docker volumes: all new files remain under the existing `library/` mount. No new volume is introduced.

### CBZ immutable image generation compatibility (2026-07-18)

- CBZ extraction uses the existing private imported-book root and adds only derived data:

```text
library/<Book.LibraryPath>/.cbz-resources/<source-sha256>/
```

- The original CBZ, SQLite schema, `chapters.json`, `bookSource.json`, LocalStore/WebDAV source,
  portable backup format and mounted roots do not change. Only supported normalized image members
  are extracted; ComicInfo remains parser metadata and arbitrary ZIP members are not exposed.
- A sibling staging directory is renamed into place only after archive entry count, path, symlink,
  duplicate, per-entry and aggregate expansion limits pass and a complete marker has been written.
  Interrupted/failed generations are not active.
- New confirmation may prepare the generation before the existing transaction. Its existing
  caller-owned archive compensation removes the whole new allocation if preparation or database
  work fails. It never removes an old book or mounted source.
- Existing books require no migration: missing generations are rebuilt lazily from `OriginalFile`.
  A signed capability may continue reading an existing complete generation while the source is
  temporarily absent. If source size/mtime changes, SHA-256 is recomputed before reuse; a different
  fingerprint invalidates the old capability and creates a new generation.
- Portable backup intentionally continues to carry the original CBZ and metadata only. Restore to
  an empty volume must rebuild `.cbz-resources` deterministically. Deleting the derived directory is
  always recoverable while the original archive remains valid.

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

## P2 user-workspace permission and deletion migration

Status: implemented and regression-tested on 2026-07-17. The additive column leaves existing
rows nullable and does not rewrite mounted-volume data.

- Add only a nullable `users.can_access_webdav` column. `NULL` means an existing
  row continues to inherit the historical `can_access_store` value. New accounts write
  an explicit value; an administrator changing WebDAV permission makes that row
  explicit. No existing `can_access_store` value, user name, password hash, storage
  path, library archive, cache file, upload or backup is rewritten during migration.
- LocalStore keeps `can_access_store`; WebDAV, WebDAV-backed backup and WebDAV import
  use effective `can_access_webdav`. Direct local upload stays independent of those two
  workspace-entry permissions, matching the upstream entry controls.
- A user deletion first prepares only rooted private paths, then commits its SQLite
  deletion transaction. After commit it may prune `data/webdav/users/<safe-username>/`,
  `library/localStore/users/<safe-username>/`, `library/data/<safe-username>/` and
  `data/uploads/users/<user-id>/`. It must never derive a deletion target from a raw
  request value or delete the administrator's legacy roots.
- An old mounted volume with no new column must auto-migrate without changing its
  effective permissions. Required evidence is a populated SQLite fixture, a two-user
  storage fixture, full backend tests and the Docker volume/backup smoke.

## P2 bookshelf browser-cache freshness compatibility (2026-07-18)

- No SQLite, mounted-volume, backup, WebDAV, book/category/progress row or API response migration is introduced.
- Existing IndexedDB/localStorage keys named
  `localCache@bookshelf@getBookshelf:<request-key>:<user-scope>` remain readable and are not cleared on upgrade.
- Their role changes only in runtime ordering: a cold load requests the authenticated `/api/books` snapshot first;
  the scoped persistent list is committed only when that network request fails and no newer request/local/WebSocket
  mutation has invalidated its revision.
- A successful network result continues to replace the same cache key. A fallback cache result must not be copied
  across user scopes or persisted as a new server-fresh revision.
- Hub backpressure handling and foreground reconciliation carry no persisted state. Closing a slow WebSocket client
  is a transport recovery action; reconnect reuses the existing authenticated full-shelf API and does not rewrite data.

Required evidence before release: old scoped browser cache remains a usable offline fallback; stale cache never
precedes a successful delayed server response; two same-user clients converge after import and reconnect; current
Docker volume/backup smoke remains byte/data compatible.

Implementation evidence: network-pending/fallback/revision unit contracts and the real two-context Go/SQLite/
WebSocket Chrome smoke pass at 1440×900, 390×844 and 360×800. The browser retains the same scoped cache key and
the server uses the unchanged shelf/settings rows. The locally built `ff4cd9d` candidate passed the historical
mounted-volume restart, backup/portable restore, TXT/EPUB/UMD/CBZ, relative-cache and owner-isolation smoke before
the same commit was published for amd64/arm64. No persistence migration was required.

The same multi-client gate makes first-time settings writes atomic through the already existing unique index on
`user_settings(user_id,key)`. It adds no table, column, index, default row, mounted file, or backup field. Existing
rows keep their IDs/values/timestamps; only two concurrent attempts to create the same missing row stop surfacing a
UNIQUE error. SQLite connection count, WAL/busy-timeout configuration and existing `-wal`/`-shm` files do not change.

## P1 Index browser-cache scope compatibility (2026-07-27)

- No SQLite table/column/index, API shape, mounted root, backup/WebDAV member, cache/library file or Docker volume
  changes. `data/`, `cache/` and `library/` are byte-compatible with the previous image.
- Existing current-user keys remain unchanged and readable except for the P2-S4 source-list key:
  `rssSources@user:<id>`, `reader@user:<id>@...`,
  `user:<id>@...@chapterContent-...` and `bookshelf@getBookshelf:...:user:<id>`.
- P2-S4 reads source lists only from `bookSourceList@source-owner-v1@user:<id>`. The prior
  `bookSourceList@user:<id>` value can contain pre-ownership global IDs, so it is never used as a
  source-list fallback. It remains recognizable and clearable as current-user derived cache;
  `sources_update` removes both current-user variants and never another user's key. IndexedDB
  schema version and all business data remain unchanged.
- Upstream-era browser chapter keys without a user scope are not rewritten or deleted during upgrade. Because no
  field can prove their owner, authenticated reads/statistics/clear/delete no longer claim them for whichever
  account happens to sign in next. They remain rebuildable derived cache rather than book, progress or bookmark data.
- The frontend scope+token generation guard and cache statistics are runtime-only. No migration marker, owner claim,
  background rewrite or new persistent setting is introduced.
- Required release evidence remains full frontend/backend/build checks, the three-viewport browser-cache contract,
  and current plus historical Docker volume/backup smoke. The first three automated code gates pass; browser and
  Docker gates remain pending.

## P0 Reader inline chapter browser-cache compatibility (2026-08-11)

- No SQLite table/column/index, API shape, mounted root, backup/WebDAV member, cache/library file or Docker volume
  changes. Existing `data/`, `cache/` and `library/` content remains byte-compatible.
- Existing `localCache@<user-scope>@...@chapterContent-<index>` keys remain unchanged and readable. The Reader now
  schedules the complete requested range; cache-first hits still advance progress without issuing another API
  request, so no cache migration, rewrite or owner claim is needed.
- Each task freezes the existing authenticated user scope before workers start. A book or account change cancels
  pending work and suppresses late UI updates; at most two already authorized in-flight requests can finish in the
  frozen old scope. No result can be re-keyed into the new user scope.
- Shelved TXT/EPUB/UMD/CBZ books use the same existing chapter-content endpoint and scoped browser keys. Temporary
  Reader sessions retain `Cache-Control: no-store` and do not gain a persistent cache writer.
- Queue progress, cancellation tokens and cached-index projections are runtime-only. Existing browser cache and
  mounted server chapter cache remain usable after upgrade and rollback.
- The locally built `4da98fa` candidate passed fresh portable-v1/v2-assets/cross-user/restart and historical
  TXT/EPUB/UMD/CBZ/relative-cache/owner-isolation mounted-volume and portable-restore gates before the same commit
  was published for amd64/arm64. No migration was required.

## P2 whole-book chapter text cache compatibility (2026-07-18)

- No SQLite table/column/index, mounted root, backup member, WebDAV file, browser cache key or chapter-cache
  filename format changes. Existing `data/`, `cache/` and `library/` volumes remain the only persistent roots.
- Existing non-empty rooted chapter files and their `chapters.cache_path` values remain readable and are skipped
  when `refresh=false`. `refresh=true` overwrites through the existing chapter-cache writer; it does not first
  delete the previous usable file or migrate unrelated chapter rows.
- A `cache_path` is cleared only when the rooted read proves the file is missing or empty. A failed refetch then
  leaves that row accurately uncached instead of letting shelf statistics count a dead reference. Permission or
  unexpected filesystem errors are not treated as authorization to delete a path.
- Whole-book selection, canonical progress counters, frontend task maps and cancellation state are runtime-only.
  They create no durable job table and are absent from backup/restore. Old clients may continue using positive
  max-300 windows and legacy response aliases.
- Embedded chapter-image persistence is not introduced by this slice. Therefore there is no new image directory,
  capability, backup rule or cleanup migration to preserve; those contracts must be designed before that feature.

Required release evidence: current and historical mounted-volume restart plus portable backup/restore,
TXT/EPUB/UMD/CBZ and relative-cache/owner-isolation smoke using the final locally built image.

## P2 embedded chapter-image derived cache compatibility (implementation in progress)

- No SQLite table, column, index, model field, backup member, WebDAV file, local-book archive, browser key, or existing chapter-text filename changes.
- New files are rebuildable derived data only, rooted under
  `cache/chapter-images/user-<user-id>/book-<book-id>/`. `blobs/<sha256-normalized-url>` stores allow-listed raster bytes; `refs/chapter-<chapter-id>.json` stores only key/MIME/fingerprint/size. Neither location stores the source URL, source header, JWT, capability, host path, or user-controlled filename.
- Existing mounted volumes require no migration. A missing image root means an empty image cache; existing text cache remains readable. `cache/` continues to be the only Docker mount involved, so restart can reuse verified files and portable logical backup/restore intentionally omits them.
- Image capabilities are generated only while serializing a response and are never durable data. The frontend consumes the optional mapping after parsing the original text so offsets, bookmarks, progress, and `data-pos` remain compatible with pre-feature clients.
- Cache cleanup is compensating post-commit file work: database changes commit first, then the exact rooted user/book image tree or obsolete chapter references are removed. A failed database transaction must never delete image files; malformed reference metadata makes blob pruning fail closed.
- Source/catalogue changes may delete only the affected book's derived image root after the replacement rows are durable. Shared blobs are deduplicated only inside one user/book root, never across owners or books.

Required release evidence: service/API ownership and malformed-cache tests, old-volume restart, portable backup/restore, TXT/EPUB/UMD/CBZ and relative-cache/owner-isolation smoke using the final locally built image.

## P2 BookGroup built-in preference and backup compatibility (2026-07-19 extracted)

- Existing `categories`, `book_categories`, `books.category_id`, user settings, `data/`, `cache/`, and `library/`
  remain unchanged and readable. Custom group membership stays many-to-many; no deployed Category ID is converted
  to a reader-dev bit mask.
- Add only `book_group_preferences`, uniquely keyed by `(user_id,key)`, for the four built-in semantic keys. Rows
  are lazily completed with names `全部/本地/音频/未分组`, show=true and orders `-10/-9/-8/-7`; this is an
  additive AutoMigrate operation with no rewrite of existing rows or files.
- Built-in and Category sort values share one logical order. A mixed reorder updates both tables atomically. A
  failed validation/write leaves the previous full order intact. Deleting a user removes preference rows in the
  same transaction as all other user-owned SQLite rows.
- New backups retain every existing member and add `bookGroup.json`. Fixed negative IDs represent built-ins;
  custom rows receive deterministic positive power-of-two portable IDs plus OpenReader `categoryId`/stable-key
  extensions. `bookshelf.json.group` uses the same temporary map, while existing `categoryNames` remains the
  lossless OpenReader round-trip field.
- Restore accepts three generations without destructive conversion: old OpenReader archives with only
  categories/categoryNames, reader-dev archives with bookGroup/group masks, and new archives containing both.
  It restores groups before shelf memberships, prefers categoryNames when present, and otherwise translates the
  reader-dev mask through `bookGroup.json`. Missing built-ins are completed with defaults.
- `bookGroup.json` is added to the portable logical allowlist but stays under the existing ZIP entry/count/size/
  expansion bounds. Restore broadcasts only the destination user's categories, book-group projection and shelf.

Required evidence before release: populated old SQLite migration, two-user projection/isolation, transaction
rollback, user deletion, old OpenReader restore, reader-dev group-mask restore, new round-trip, full backend tests,
and the Docker mounted-volume/backup compatibility smoke.

## P1 bookshelf latest-chapter timestamp compatibility (2026-07-22 extracted)

- Add only `books.last_check_time` as an integer millisecond timestamp matching reader-dev's
  `lastCheckTime`. Existing `created_at`, `updated_at`, reading-progress rows and `shelfOrderAt` projection remain
  unchanged.
- After additive AutoMigrate, rows with zero `last_check_time` are backfilled once from `created_at`; use
  `updated_at` only when creation time is unavailable. Reading progress must never seed this field.
- New shelf rows receive a non-zero insertion timestamp. `shelfOrderAt` uses reading progress or creation time,
  never generic `updated_at`. Remote refresh/scheduler advance `last_check_time` only when chapter
  count grows; metadata edits, reading, failed refresh and no-growth refresh do not.
- Legacy reader-dev restore accepts a positive `lastCheckTime`. Missing, zero, negative or malformed values use
  the destination insertion time. Embedded OpenReader `Book` backup JSON then round-trips it automatically.
- This migration does not touch archives, derived content, chapter/cache paths, category membership, bookmarks,
  progress, `data/`, `cache/` or `library/`.

Required evidence: old-table migration plus idempotent second run, progress/update separation, growing/no-growth
refresh, reader-dev restore and re-export, frontend source-field contract, and mounted-volume/backup smoke before
Docker release. See `bookshelf-last-check-time-p1-contract.md`.

## P1 book-deletion browser-state convergence compatibility (2026-07-22)

- No SQLite table/column/index, mounted directory, backup member, browser cache
  key or local-progress key changes. Existing scoped shelf snapshots,
  `openreader_chapter_progress@<scope>@<book-id>` entries and chapter-content
  keys retain their deployed formats.
- Successful server deletion now removes matching browser state as derived/user
  state cleanup; failed or cancelled deletion removes nothing. Legacy unscoped
  chapter/progress keys remain readable and are removed only for the confirmed
  deleted book as before.
- Async cleanup captures the authenticated scope that received/initiated the
  deletion. Shelf snapshot lookup/removal and scoped chapter-prefix generation
  use that frozen scope even if another account becomes current before
  IndexedDB/localStorage work completes. It cannot migrate or delete another
  user's scoped keys.
- Reader suspension, overlay reconciliation, BookManage selection pruning and
  the deletion event carry no durable state and require no migration.

Required release evidence: scoped cache/progress unit contracts, real
two-client WebSocket deletion at desktop and both mobile viewports, full
frontend/backend/build gates, and the unchanged mounted-volume/backup smoke.

## P2 UserManage last-login compatibility (2026-07-28)

- No table, column, index, file, mounted directory or backup member is added or rewritten.
  The deployed `users.last_active_at` column is retained as the compatible storage location;
  successful login now gives that value its reader-dev `lastLoginAt` meaning.
- Existing non-zero values remain readable and are not backfilled. A zero value remains a valid
  legacy state and renders blank until the account successfully logs in.
- `lastLoginAt` is the canonical new API field. `lastActiveAt` remains an equal response alias so
  old clients do not break; no persisted JSON or SQLite field is renamed.
- Failed credentials do not change the value. Successful registration initializes it like
  reader-dev's `User` default, and later successful logins advance it.
- User deletion, inactive-cleanup compatibility, backup/restore, WebDAV roots and every
  `data/`, `cache/`, and `library/` path remain unchanged.

Required release evidence: focused login/list API contract, full Go and frontend tests, production
build, one-table UserManage browser smoke at 1440×900, 1024×1366, 390×844 and 360×800, followed by
the unchanged Docker volume/backup compatibility gate.

Release evidence completed with `f44447f`: both ordinary and historical volume scripts passed,
including restart, portable v1/v2 assets, cross-user isolation, TXT/EPUB/UMD/CBZ and relative-cache
fixtures. No migration or mounted data rewrite was observed.

## P2 UserManage partial-update compatibility (2026-08-09)

- No table, column, index, file, mounted directory, backup member or WebDAV path changes.
- Permission writes now use an explicit SQL column map. Existing user rows and every omitted account field remain
  untouched; in particular, a concurrent `password_hash` or `last_active_at` update cannot be replaced by a stale
  permission-screen snapshot.
- Existing `can_access_webdav=NULL` rows remain NULL in SQLite. API responses expose the effective inherited value
  on a response copy only; this is not a backfill or migration.
- Existing `false` permission values and zero unlimited book/source limits remain readable and writable. Negative
  limits and empty patches are rejected without updating `updated_at` or emitting synchronization events.

Focused trigger-based concurrency tests, race tests and full API regressions pass. `77a60d8` then passed fresh
mounted volume portable v1/v2 assets, cross-user and restart, plus historical TXT/EPUB/UMD/CBZ, relative-cache,
owner-isolation and portable restore. No mounted data or archive rewrite was observed.

## P0 ReaderSettings scheme/snapshot compatibility (2026-08-02 extracted)

- No SQLite table, column, index, mounted directory, API path, backup member or WebDAV file changes. Reader settings
  remain JSON in the existing per-user `user_settings(key=reader)` row plus the already scoped Pinia cache.
- Existing semantic theme keys, custom schemes, custom font/background URLs and legacy `mono` values remain readable.
  Migration may reclassify old scheme fields but must not delete a scheme, uploaded file or global asset reference.
- New custom-scheme serialization uses an explicit allowlist equivalent to reader-dev's `syncConfigFiled`, plus the
  user-requested brightness value. Old scheme copies may contain `pageType`, `autoTheme`, TTS values and full asset
  inventories; those fields are ignored when applying a scheme so they can no longer overwrite global state.
- `customFontsMap` and `customBgImageList` remain global per-user asset inventories. `ttsRate`, `ttsPitch` and
  `ttsVoiceURI` remain the independent TTS configuration. Reset and scheme switching never delete or empty them.
- Normal and Kindle modes gain two per-user recent-config snapshots inside the existing reader JSON. Missing snapshots
  use fixed-upstream defaults; old `normalModeSnapshot` is accepted as a one-way compatibility input. Snapshots never
  use an unscoped localStorage key and cannot cross authenticated users.
- If `settingsVersion` advances, normalization is lazy and non-destructive. Values above the old arbitrary font/
  auto-read UI maxima remain finite and readable instead of being truncated; NaN/Infinity still fall back safely.
- Built-in theme textures and 14 built-in reading backgrounds are immutable frontend assets. They are not upload files,
  are not included in portable asset manifests and do not change `data/`, `cache/` or `library/`.

Required evidence before release: old/current reader JSON fixtures, cross-user reset and delayed settings load,
normal↔Kindle reload, custom scheme/asset preservation, portable v1/v2 assets, historical mounted-volume restart and
owner-isolation smoke. See `reader-settings-fixed-baseline-second-audit-p0-contract.md`.

## P2 WebSocket synchronization protocol compatibility (2026-08-09 extracted)

- `/ws/sync` remains an ephemeral transport. It adds no SQLite row, browser-persisted event queue, mounted file,
  backup member, WebDAV object or migration.
- Existing server event names and payloads remain readable by old clients. The protocol becomes explicitly
  server-to-client only; removing the unused frontend `send()` and rejecting inbound application messages cannot
  delete or translate persisted state.
- Tightening `users_update` recipients changes no user, role or source row. Administrators receive the complete
  changed-ID set; an affected ordinary user receives only its own ID so profile refresh/deletion logout still works;
  unrelated users receive nothing.
- A reconnect never replays an event log. Existing REST/SQLite authority, foreground reconciliation, operation
  generation and cache fallback remain unchanged, including historical browser caches.
- Because there is no data-format change, no new old-volume fixture is required. A release still runs the unchanged
  fresh/historical volume and portable-backup gates to prove that the transport hardening did not accidentally touch
  `data/`, `cache/` or `library/`.

See [`websocket-sync-p2-contract.md`](websocket-sync-p2-contract.md).

## P2 public auth request-boundary compatibility (2026-08-12)

- No SQLite table, column, index, user row, JWT claim, browser key, mounted directory, backup member or WebDAV
  object changes. Existing `data/`, `cache/` and `library/` layouts remain byte-for-byte compatible.
- The 16 KiB limit applies only to the wire body of public login/registration requests. It does not rewrite stored
  usernames, password hashes or login timestamps and does not add a migration/backfill.
- The bcrypt 72-byte registration boundary constrains only newly submitted public passwords. Existing password
  hashes remain readable; login keeps existing username trimming and does not apply new-account username rules.
- Rejected oversized, multi-value or overlong-password requests are validated before durable work. Contract tests
  directly prove no user row or `last_active_at` mutation; runtime smoke proves no rejected registration appears in
  the public server's authoritative user list.

Focused/full/race/vet, frontend 740/740, production build and isolated production-shape HTTP smoke pass. `f5c15d7`
then passed the unchanged fresh portable-v1/v2-assets/cross-user/restart and historical
TXT/EPUB/UMD/CBZ/relative-cache/owner-isolation gates before local dual-architecture publication. No migration or
mounted data rewrite was observed. See `auth-request-boundary-fixed-baseline-second-audit-p2-contract.md`.

## P2 administrator user-write boundary compatibility (2026-08-12)

- No SQLite table, column, index, JWT claim, mounted path, backup member, WebDAV object or browser key changed.
  Existing password hashes and legacy usernames remain loginable; only future public/admin password writes use the
  corrected 8 UTF-16-code-unit minimum and existing bcrypt 72 UTF-8-byte maximum.
- The 16 KiB single-JSON boundary applies only to five administrator user mutations, after authentication. The 2,000
  raw-ID limit applies only to source reset and batch deletion before dedupe/query/transaction. Rejected requests do
  not create users, update limits/hashes, alter source namespaces, plan workspace cleanup or broadcast events.
- `6c1c6db` passed focused/full/race/vet, frontend 740/740, production build, real HTTP declared/chunked and exact-limit
  checks, then sequential fresh portable-v1/v2-assets/cross-user/restart and historical
  TXT/EPUB/UMD/CBZ/relative-cache/owner-isolation gates. No mounted data or archive rewrite was observed before local
  dual-architecture publication. See `admin-user-write-boundary-fixed-baseline-second-audit-p2-contract.md`.

## P2 user-setting write-boundary compatibility (2026-08-12 implementation)

- The 8 MiB single-JSON limit applies only to future authenticated `PUT /api/settings/:key` wire bodies
  after legal-key validation. It adds no table, column, index, row, browser key, backup member or mounted file.
- Existing `user_settings` values, including rows larger than the new PUT limit, remain readable and exportable.
  Startup and restore do not scan, truncate, delete or rewrite them. Logical/portable/WebDAV restore keeps its own
  archive and transaction limits rather than reusing the interactive HTTP PUT limit.
- Rejected oversized/multi-value writes leave the prior value and `updated_at` unchanged and emit no sync event.
  Normal CAS/force, `(user_id,key)` upsert, reader local-field cleanup and three-key backup mapping remain authoritative.
- `c2bc736` passed focused/full/race/vet, frontend 740/740, production build and isolated real HTTP. A directly seeded
  setting larger than 8 MiB also passed GET, logical backup and restore without truncation. The local arm64 candidate
  built with the correct revision; fresh/historical volume and remote multi-architecture publication remain pending
  because the OrbStack socket approval request was rejected after its automatic reviewer disconnected. See
  `user-setting-write-boundary-fixed-baseline-second-audit-p2-contract.md`.

## P1 manual shelf refresh compatibility (2026-08-09)

- No table, column, index, mounted directory, backup member, WebDAV object or browser key format changes.
  Existing `Book`, `Chapter`, `ReadingProgress` and `Bookmark` rows remain the durable authority.
- An exact-prefix remote catalogue update appends only new chapter rows and preserves every existing chapter ID,
  cache path, progress reference and bookmark reference. An equal catalogue performs no chapter rewrite.
- A successful non-prefix replacement reuses the existing catalogue replacement transaction. Recoverable progress
  and bookmark offsets/indexes remain; obsolete chapter IDs are rebound by canonical resource or index, and only
  unreferenced derived chapter caches are removed after commit. Original imports and mounted library files are not
  cleanup targets.
- A remote, parser, stale-snapshot or transaction failure leaves that book's old catalogue, summary, variable,
  progress, bookmarks and cache readable. Another book may still commit independently in the same refresh round.
- Existing old volumes need no migration or backfill. `43635a1` passed the unchanged fresh/historical Docker volume
  and portable-backup gates before publication; see
  `bookshelf-manual-refresh-fixed-baseline-second-audit-p1-contract.md`.

## P2 backup restore transaction-worker compatibility (2026-08-09 extracted)

- The logical restore already promises one SQLite transaction for selected settings, groups, shelf, progress,
  bookmarks, RSS, rules and permitted sources. Its transaction worker must therefore contain only the transaction
  DB and services explicitly rebuilt on that DB.
- Copying the full `Server` also copies its mutex and unrelated long-lived runtime/service pointers. It is forbidden
  even if today's helpers happen not to dereference those fields; a later helper could silently escape the rollback
  boundary through an original-DB service.
- Replacing the shallow copy with a minimal transaction-bound worker changes no table, row, archive, mounted file,
  API field or WebSocket event. Existing old/current backup fixtures remain authoritative.
- `go vet ./...` is an explicit red/green gate in addition to the existing rollback and fresh/historical volume
  tests. See `backup-restore-fixed-baseline-p2-contract.md`.

## P2 BookGroup / Category write-boundary compatibility (2026-08-12 implementation)

- The implemented 16 KiB single-JSON boundary applies only to six authenticated HTTP mutations in the existing
  BookGroup state machine. It adds no table, column, index, row, backup member, browser key or mounted file.
- The existing `categories`, `book_group_preferences`, `book_categories` and `books` rows remain authoritative.
  No startup scan, truncation, backfill, ID rewrite or relationship conversion is permitted.
- Future explicitly submitted names and colors will enforce the already documented model budgets of 80/24 UTF-8
  bytes. Historical oversized values remain readable, exportable and restorable, and an update that does not touch
  the oversized field must remain possible.
- Logical, portable, WebDAV, reader-dev and categories-only restore retain their own archive/transaction contracts;
  they do not reuse the interactive HTTP body or field limit.
- Rejected create/update/reorder/set requests must leave rows and timestamps unchanged and emit no sync event.
  In particular, an invalid Category create may not lazily seed four built-in preference rows before returning 400.
- A book-category mutation must validate the final effective ID set before writing either `books.category_id` or
  `book_categories`; an empty secondary array cannot let a foreign fallback ID create a cross-user relationship.
  Existing contaminated rows are not silently scanned or rewritten by this HTTP-boundary slice.
- No frontend geometry or BookGroup backup format changes. See
  `book-group-write-boundary-fixed-baseline-second-audit-p2-contract.md`; status is
  `implementation-complete / regression-validated / mounted-volume-and-Docker-pending`. `6f54be3` changes no
  schema or persistent format; API backup/restore coverage proves historical oversized rows remain lossless. The
  release-specific fresh/historical mounted-volume gate still awaits explicit Docker socket approval.

## P2 Book control request-boundary compatibility (2026-08-16 extracted)

- The proposed 16 KiB/32 KiB/1 MiB single-JSON boundaries affect only six future authenticated HTTP requests. They add no
  table, column, index, startup scan, data rewrite, browser key, mounted path or backup member.
- Existing Book, Chapter, Category, Progress, Bookmark and source-candidate rows remain authoritative. Historical
  oversized text/URL rows are not scanned or truncated; the new Book byte limits apply only to fields explicitly
  submitted by future remote-add/source-change requests.
- `refresh-local` continues to reuse the existing `library` archive and atomically stage/promote derived TOC data.
  Moving request admission before archive reads must leave every rejected request with identical Book/Chapter rows,
  source bytes, TOC metadata, cache paths and progress.
- Batch cache cancellation may retain chapters already durably cached before cancellation, exactly like the published
  stream cache contract, but must not start later books or delete prior cache. Export cancellation writes no database
  or mounted data. Remote add/change cancellation creates no row/candidate and records no source failure.
- Logical/portable/WebDAV backups and restores keep their own archive/cardinality contracts; they do not reuse these
  interactive request limits. Fresh/historical/portable volume gates remain required before release. See
  `book-control-request-boundary-fixed-baseline-second-audit-p2-contract.md`.
- Implemented in `65199f6` with zero model/migration/path/backup-format diff. The local candidate passed fresh plus
  historical/portable mounted-volume gates for TXT/EPUB/UMD/CBZ, relative cache, owner isolation, restart and portable
  v1/v2 before the locally built amd64/arm64 image was published. `65199f6`/`latest` resolve to OCI index
  `sha256:57eda43d437d98a4f2d748164d58c5816f3ff3dc199397bd9dc8f6d48334a8cb`.

## P2 ReplaceRule request-boundary compatibility (2026-08-16 implemented/published)

- `9f5a52b` changes only authenticated request decoding/admission and request-context propagation. It adds no table,
  column, index, migration, startup scan, path, mounted file, archive member, browser key or backup format.
- Existing `replace_rules` rows remain byte-for-byte authoritative, including stable IDs/order, exact whitespace,
  duplicate names, legacy empty scope and historical mode values. No existing row is normalized, truncated or
  rewritten by startup, list, backup or restore.
- Logical/portable/reader-dev/Legado/WebDAV backup and restore retain their independent size/cardinality/transaction
  contracts. Rejected new HTTP requests mutate neither SQLite nor mounted `data/`, `cache/` or `library/` state.
- The local candidate passed fresh portable-v1/v2-assets, cross-user and restart plus historical TXT/EPUB/UMD/CBZ,
  relative-cache, owner-isolation and restore gates. The locally built amd64/arm64 `9f5a52b`/`latest` release resolves
  to OCI index `sha256:7a72f2d01b26d1d28c35bb13970cb64a1f7dbf97ddebc3aa704957f58f2f56c3`.

## P2 backup-generation request lifecycle compatibility (2026-08-17 implemented/published)

- `cd3a17c` changes only future ordinary/portable generation request cancellation, internal-error projection and
  path-free logging. It adds no table, column, index, migration, startup scan, caller root, archive member, manifest
  version, filename prefix, browser key or persisted setting.
- Existing logical/portable ZIPs, SQLite rows and mounted `data/`, `cache/` and `library/` files are neither scanned
  nor rewritten. A canceled request removes only its current private pre-rename temp; an already renamed durable ZIP
  is retained. Scheduled and existing in-process callers keep background-context compatibility entry points.
- The local candidate passed fresh portable-v1/v2-assets, cross-user and restart plus historical TXT/EPUB/UMD/CBZ,
  relative-cache, owner-isolation and restore gates. Real HTTP also proved fixed safe 500, ordinary list/download and
  cleanup after disconnecting a 128 MiB portable copy with a visible private temp.
- The locally built amd64/arm64 `cd3a17c`/`latest` release resolves to OCI index
  `sha256:08e9a5ba94646e5955e9c0d4586a4be95d004d6a015b518331c02748a9e53f70`; both remote platform configs carry
  full revision `cd3a17c63f9768130a33b4a199a3228cb94d8261`.

## P2 backup list/download filesystem-boundary compatibility (2026-08-17 implemented/published)

- `2986357` changes only future backup list filtering, safe file opening, portable format inspection and download
  response handling. It adds no table, column, index, migration, startup scan, caller root, archive member, manifest
  version, filename prefix, generation path, browser key or persisted setting.
- Existing regular `backup_*.zip` and `portable_backup_*.zip` remain readable without rewriting. Symlinks,
  directories, special files and prefix-only non-ZIP objects remain untouched in their mounted location but are no
  longer projected or downloadable through the backup API. Raw WebDAV management retains its independent contract.
- The local candidate passed fresh portable-v1/v2-assets, cross-user and restart plus historical TXT/EPUB/UMD/CBZ,
  relative-cache, owner-isolation and restore gates. Real HTTP also proved valid logical/portable bytes, portable-invalid
  projection, same-name caller isolation and symlink/non-ZIP/directory/FIFO/ancestor fail-closed behavior.
- The locally built amd64/arm64 `2986357`/`latest` release resolves to OCI index
  `sha256:bdb8195077000a898569e0f3f6664a5760c2b56058d67b2d6ae1d4aaf42fea5e`; remote amd64 manifest is
  `sha256:e782f3219c910e2c70580fc74b5d0bc9fd7014fa370e638e16b07eccb5e99628`, arm64 manifest is
  `sha256:85e0a43109b2d98e77e3150b4a1600c08fb638dbd7298eb3800631128245fa8f`, and both configs carry full revision
  `298635792caaa9a8dfb6de09fd2879f837c84f22`.

## P2 public upload-resource filesystem-boundary compatibility (2026-08-17 implemented/published)

- `277e512` changes only future public GET/HEAD opening below the existing `data/uploads` root. It adds no table,
  column, index, migration, startup scan, directory, URL, asset namespace, backup member, browser key or setting field.
- Existing legacy `/uploads/<kind>/<name>` and current `/uploads/users/<id>/<kind>/<name>` regular files remain
  byte-for-byte readable without moving, renaming or rewriting. Existing root/ancestor/entry symlinks, directories and
  special files remain untouched on disk but are no longer projected through the public resource capability.
- The local candidate and GHCR-pulled image passed regular GET/HEAD/Range/304, unsafe empty-404, unchanged-object and
  path-free-log probes. Fresh portable-v1/v2-assets/cross-user/restart and historical TXT/EPUB/UMD/CBZ,
  relative-cache, owner-isolation and restore gates passed against unchanged `data/cache/library` mounts.
- The locally built amd64/arm64 `277e512`/`latest` release resolves to OCI index
  `sha256:ca50fd59dce4f4bb13a1450ee7ee39b2a3d7b392de3902a7f3c21272e8ac9c70`; remote amd64 manifest is
  `sha256:f936ed8b9dbf3bf61a5d0f621bd7c06f4861f0c1a963df2fe85cc3890cc3ed81`, arm64 manifest is
  `sha256:111f01ed3ddf7c404bdc6dc3a40c97180ce4821dc0bff852d8b4abf3e1213c91`, and both configs carry full revision
  `277e512fa1a0135cff4089298d4644ee72ddf518`.

## P2 remote chapter-text cache filesystem-lifecycle compatibility (2026-08-17 extracted)

- The pending second audit changes no SQLite table/column/index, mounted root, cache hash name, backup member,
  WebDAV object, browser key or environment variable. Existing `chapters.cache_path` remains authoritative input.
- Relative paths and historical absolute paths below the current `cache/` root remain lazy-readable. Retired-host or
  unsafe paths are not rewritten at startup; they remain until a successful recache publishes the existing relative
  hash identity or the owning user explicitly clears/deletes that remote book.
- Explicit clear may reset the caller's unsafe/missing remote DB references after commit, but must leave every unsafe,
  special or outside-root mounted object untouched. Local imported-book rows and `library/` content are excluded.
- Reference-aware pruning remains post-commit and compensating. It may remove a verified regular derived file only
  after an all-user query proves no remote chapter still references its canonical cache identity; query failure and
  concurrent publication fail closed.
- Required release evidence is focused/race/full/vet, real Reader/BookManage/sidebar cache flows, mounted symlink and
  shared-reference runtime probes, plus unchanged fresh/historical/portable `data/cache/library` gates before local
  amd64/arm64 publication. Full contract:
  [`remote-chapter-cache-filesystem-lifecycle-fixed-baseline-second-audit-p2-contract.md`](remote-chapter-cache-filesystem-lifecycle-fixed-baseline-second-audit-p2-contract.md).
- `75cc238` implements this boundary without a migration or startup scan. Host/candidate mounted probes, Docker
  named-volume restart and fresh/historical/portable gates passed. The locally built amd64/arm64 `3cef8df`/`latest`
  release resolves to OCI index `sha256:8cfe72e56af0cbb191d6b31fa243153a3ce14010614c5153881b262229facf86`;
  both pulled platform configs report full revision `3cef8dfdccd45970596b3d8916a2cb6fab1480dc`.

## P2 public capability filesystem-read lifecycle compatibility (2026-08-17 extracted)

- The pending EPUB/CBZ/local-audio/cached-cover opened-file change adds no table, column, index, migration, startup
  scan, directory, capability field, URL, environment variable, backup member or browser key.
- Existing regular files under `library/` and `cache/cover-images`, original archives, complete EPUB/CBZ generation
  markers and historical relative paths remain byte-for-byte authoritative. No startup rewrite or eager rebuild is
  allowed.
- Unsafe symlinks, directories and special files remain untouched on mounted storage but cannot be projected by a
  public capability. A failed identity check must not delete an archive, generation, audio source or cover object.
- Required release evidence includes fresh/historical/portable mounted volumes and restart, proving TXT/EPUB/UMD/
  CBZ/audio, relative cache, owner isolation and backup/restore remain compatible. Full contract:
  [`public-capability-filesystem-read-lifecycle-fixed-baseline-second-audit-p2-contract.md`](public-capability-filesystem-read-lifecycle-fixed-baseline-second-audit-p2-contract.md).
- `a90f7b3` implements opened-file identity without schema, migration, startup scan or layout changes. Focused/full/
  race/vet, host and `5313c49` candidate mounted-symlink probes pass. Fresh volumes preserve portable v1/v2 assets,
  cross-user isolation and restart; historical volumes preserve TXT/EPUB/UMD/CBZ, relative cache, owner isolation,
  portable restore and archive hashes. The local candidate reports full revision
  `5313c49f6a18b3cce769ea03e4f8cdf8fddafebe`.
- The final locally built amd64/arm64 `5e63eb1`/`latest` release resolves to OCI index
  `sha256:8b7bc4cd8542f79eccc54d393cf2d79041f5fe9a90b05776c473cd3f1e4c2cee`; remote amd64 manifest is
  `sha256:f5ac952ead7e410c3debe7c2892f84cc5d6e0212a3982c3cd18209e084186c3d`, arm64 manifest is
  `sha256:a7a243fe0bc4083ebdb6f4f1df5206315a6d3cbac05544525544e44244663d76`, and both configs carry full revision
  `5e63eb1854a95dc3fd79ebafec89f4723f37f8da`. A forced remote arm64 pull reported the same index and healthy
  runtime revision. No mounted path, database row or backup member changed in the release.

## P2 local-book archive filesystem lifecycle compatibility (2026-08-24 implemented/published)

- The `125fd93` change adds no table, column, index, migration, startup rewrite, archive layout, backup member, URL,
  browser key or environment variable. It only constrains future local-book reads, explicit refresh and post-delete
  cleanup to the existing trusted `library/` mount.
- Existing regular current-relative and historical-absolute `LibraryPath`/`OriginalFile` representations remain
  readable through the established basename/suffix rebase. The retired host path itself never becomes readable,
  and no valid original archive is rewritten by refresh.
- Root, `data`, owner, outside-book, content/metadata ancestor and entry symlinks or special files remain untouched on
  mounted storage but cannot be read, written, promoted, pruned or recursively deleted through the API. A historical
  book-directory alias resolving inside the same verified owner root remains compatible; a durable Book delete is not
  rolled back when best-effort unsafe cleanup is refused.
- Refresh continues to stage a new generation and commit chapter/progress/bookmark metadata atomically. Unsafe archive
  identity must fail before DB/file/event work; valid old rows, archive hashes and active cache remain unchanged.
- Focused/full/race/vet, frontend 741/741, production build, post-fix real HTTP, Reader and BookManage three-view
  smoke passed.
  The local `125fd93` candidate then passed fresh portable-v1/v2-assets/cross-user/restart and historical
  TXT/EPUB/UMD/CBZ/relative-cache/owner-isolation/archive-hash/portable-restore gates. The locally built amd64/arm64
  release resolves to OCI index `sha256:777ca720981b8a3529009211ce179b430bb354cb01e2957681f191036699f6a5`.
  Full contract:
  [`local-book-archive-filesystem-lifecycle-fixed-baseline-second-audit-p2-contract.md`](local-book-archive-filesystem-lifecycle-fixed-baseline-second-audit-p2-contract.md).

## P2 backup-upload multipart request compatibility (2026-08-25 implemented/published)

- The `a0fb1bd` change adds no schema, migration, startup scan, mounted path, backup member, manifest, archive budget,
  environment variable, route, response field, browser key or visible restore workflow.
- Existing logical reader-dev/Legado/OpenReader ZIPs and portable v1/v2 packages remain byte-for-byte inputs to the
  same preflight and transaction. Current/historical `data/cache/library` volumes are not scanned or rewritten.
- Only future ambiguous HTTP uploads with scalar, duplicate or differently named extra file parts are rejected before
  stage/restore. A valid singular `file=*.zip` retains current counts, permission, collision and rollback semantics.
- Handler-owned `multipart.Form.RemoveAll()` shortens request temp lifetime only; caller-private restore stage and its
  existing cleanup remain separate. Fresh portable-v1/v2-assets/cross-user/restart and historical TXT/EPUB/UMD/CBZ/
  relative-cache/owner-isolation/archive-hash/portable-restore gates passed against unchanged mounts. The locally built
  amd64/arm64 `a0fb1bd`/`latest` release resolves to OCI index
  `sha256:b25f5b05df983532bf656ec8647e553188db3ba7fb291b826cb45b65deae6f3c`. Full contract:
  [`backup-restore-multipart-request-boundary-fixed-baseline-second-audit-p2-contract.md`](backup-restore-multipart-request-boundary-fixed-baseline-second-audit-p2-contract.md).

## P2 HTTP server lifecycle compatibility (2026-08-25 implemented/published)

- `f394c1a` adds no schema, migration, startup scan, persistent path, environment variable, route, response field,
  browser key, backup member or manifest. Existing `data/cache/library` bytes are not scanned, moved or rewritten.
- Graceful stop only changes process admission/lifetime: 512 KiB/10 second headers, an 8 second drain and explicit
  cleanup notification. SQLite WAL/transaction, atomic stage/file publication and existing route body budgets remain
  authoritative; global read/write timeouts stay disabled for large and streaming work.
- Fresh portable-v1/v2-assets/cross-user/restart and historical TXT/EPUB/UMD/CBZ/relative-cache/owner-isolation gates
  passed against the local candidate. Candidate `docker stop -t 10` completed in 0.50 seconds with exit 0 and no OOM.
- The locally built amd64/arm64 release is `f394c1a`/`latest`, OCI index
  `sha256:4af0cf100434ed852fdf6727d351425cca6935c8f7f6a00eaec220de9865eafa`. Full contract:
  [`http-server-lifecycle-fixed-baseline-second-audit-p2-contract.md`](http-server-lifecycle-fixed-baseline-second-audit-p2-contract.md).

## P2 access-log query redaction compatibility (2026-08-25 implemented/published)

- No schema, migration, startup scan, persistent path, environment variable, route, payload, backup member, manifest,
  browser key or UI is introduced. Existing `data/cache/library` bytes are not scanned or rewritten.
- Only future access-log lines replace raw query with fixed `?<redacted>`; request parsing, SQLite/files/events and
  backup/restore remain byte-for-byte outside this slice. Existing log files are not scanned or rewritten.
- Fresh portable-v1/v2-assets/cross-user/restart and historical TXT/EPUB/UMD/CBZ/relative-cache/owner-isolation gates
  passed against the local candidate. The locally built amd64/arm64 release is `f88ecec`/`latest`, OCI index
  `sha256:832216dbacb0650a5a6cb30b14731432714f4d48393516aed10c957a97549a29`.
- Full contract:
  [`access-log-query-redaction-fixed-baseline-second-audit-p2-contract.md`](access-log-query-redaction-fixed-baseline-second-audit-p2-contract.md).
  Status is **aligned / regression-validated / Docker-published / awaiting-device-verification**.

## P2 trusted-proxy client identity compatibility (2026-08-25 implemented/published)

- `f5b3869` adds one optional process-only environment variable and no schema, migration, startup scan, persistent
  path, route, payload, backup member, manifest member, browser key or UI state. Existing `data/cache/library` bytes
  are not scanned, moved or rewritten.
- Direct deployments need no change and now ignore caller-controlled forwarded client-IP headers. Reverse proxies
  can explicitly list their own IP/CIDR through `OPENREADER_TRUSTED_PROXIES`; this affects limiter/log identity only.
- GitHub Actions `32828470325` passed fresh portable-v1/v2-assets/cross-user/restart and historical
  TXT/EPUB/UMD/CBZ/relative-cache/owner-isolation gates against unchanged volumes.
- The published amd64/arm64 `f5b3869`/`latest` OCI index is
  `sha256:6a2fc83bf79426e93423b1dd5756c8ea49b716d1321441d5c194efff9c03b066`. Full contract:
  [`trusted-proxy-client-identity-rate-limit-fixed-baseline-second-audit-p2-contract.md`](trusted-proxy-client-identity-rate-limit-fixed-baseline-second-audit-p2-contract.md).

## P2 authenticated-session lifecycle compatibility (2026-08-26 implemented/published)

- The migration is additive only: existing users gain nonzero `auth_version=1`, a new `user_sessions`
  runtime table stores only hashed random identities and timestamps, and one idempotent schema-migration marker fixes
  the legacy-JWT transition start. Existing user/profile/shelf/source/content rows and mounted files are not scanned
  or rewritten.
- `user_sessions` is disposable authentication state and must never enter ordinary, portable or Legado backup,
  WebDAV backup, source export or user configuration. Restoring data does not restore sessions.
- Existing signed JWTs are not invalidated immediately: for at most seven days after the persistent marker they may
  be adopted when the user still exists with `auth_version=1`. Restart and repeat migration cannot reopen the
  window; password reset increments the version so an old token cannot be adopted afterward.
- Historical/current `data/cache/library`, migration idempotence, backup omission, password-reset rollback and
  user-deletion cleanup passed focused, race, historical-volume and portable-backup tests before publication.

Target contract:
[`authenticated-session-lifecycle-fixed-baseline-second-audit-p2-contract.md`](authenticated-session-lifecycle-fixed-baseline-second-audit-p2-contract.md).
Status is **aligned / regression-validated / Docker-published / awaiting-device-verification**. Contract `8db7c85`,
red tests `2396537` and implementation `a0edce3` landed in order. Actions run `32914105929` passed fresh and historical
volume gates; the published OCI index is
`sha256:5d7fe23ba96107c5c545e9e44815514fe277e5a6f83eb25cb006859c5d515d78`.

## P2 default book-source compatibility mirror migration (2026-08-26 implemented/published)

- A direct pre-ownership upgrade with a safe valid `data/defaultBookSources.json` must preserve that explicit default,
  including `[]`, while existing users retain all legacy active-source associations and stable source IDs.
- If ownership-v1 is already applied, SQLite remains authoritative; a potentially stale compatibility JSON cannot
  overwrite it. Startup canonicalizes the mirror from the committed namespace before serving source actions.
- Legacy input is one rooted, same-file-verified regular file, at most 16 MiB and 300 source objects. Missing means no
  legacy default; symlink/special/oversized/malformed input creates no namespace and exposes no path.
- Migration/recovery markers are additive and idempotent. They never enter ordinary/portable/Legado backup and never
  rewrite user namespaces, books, failures, cache or mounted library files.

Full contract:
[`default-book-source-snapshot-filesystem-transaction-fixed-baseline-second-audit-p2-contract.md`](default-book-source-snapshot-filesystem-transaction-fixed-baseline-second-audit-p2-contract.md).
Contract `1c5f7b5`, red tests `6d8b8f1`, test correction `07761b5` and implementation `a36b888` landed in order.
Direct pre-ownership custom/nonempty and explicit-empty upgrades, already-migrated stale-mirror restart, stable user
source IDs, database rollback, backup omission, fresh/historical/portable volumes and pulled-image 0600 mirror all
passed. Actions run `32919553203` published OCI index
`sha256:63979a0e01d8942a9c594d444e6d5cdf28f0ac5c382825f71a051a52b02a21e4`. Status is
**aligned / regression-validated / Docker-published / awaiting-device-verification**.

## P2 local-book refresh request lifecycle compatibility (2026-08-27 inventory)

- The target adds no schema, migration, startup scan, persistent path, environment variable, archive generation,
  backup member, route, payload, browser key or UI state.
- Existing regular current/historical `LibraryPath`, `OriginalFile`, `TOCFile`, `SourceFile`, local archive bytes and
  TXT/EPUB/UMD/CBZ/PDF/Markdown parser outputs remain authoritative. Rooted archive and opened-file rules are not
  reopened.
- A cancelled or stale refresh must discard only its uncommitted result and request-owned inactive stage. It must not
  rewrite the current Book/Chapter/Progress/Bookmark rows, promote metadata/cache, prune active data or resurrect a
  concurrently deleted Book.
- A successful refresh continues to replace the catalogue and rebind references, but updates only local-catalogue
  fields. Existing title/author/cover/intro/category/can-update and mounted path fields are not owned by a late
  refresh result.
- Rollback to an older image sees the same SQLite and filesystem formats; it only reintroduces stale `Save` and
  cancellation risks.

Target contract:
[`local-book-refresh-request-lifecycle-fixed-baseline-second-audit-p2-contract.md`](local-book-refresh-request-lifecycle-fixed-baseline-second-audit-p2-contract.md).
Status is **inventory-complete / tests-and-implementation-pending**; no data or application code changed in inventory.
