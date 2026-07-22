# Bookshelf latest-chapter time P1 compatibility contract

Baseline: `changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`.

## User-visible contract

The time shown before a shelf card's latest chapter is the time at which that shelf book most
recently discovered new chapters. It is not the user's last reading time and is not the generic
row update time.

Upstream keeps these concerns separate:

- `Book.lastCheckTime`: initialized when the book enters the shelf and updated when a refreshed
  catalogue contains more chapters than `totalChapterNum`;
- `Book.durChapterTime`: updated when reading progress is saved;
- `Index.vue`: renders `dateFormat(book.lastCheckTime)` before `latestChapterTitle`;
- the Vuex `shelfBooks` getter sorts by `durChapterTime`, so reading still moves a book to the front
  without changing the latest-chapter timestamp shown on its card.

Authoritative upstream evidence:

- `src/main/java/io/legado/app/data/entities/Book.kt` defines independent `lastCheckTime` and
  `durChapterTime` fields;
- `BookController.getBookShelfBooks()` and `saveShelfBookLatestChapter()` only advance
  `lastCheckTime` when the catalogue gains chapters;
- `web/src/views/Index.vue` displays `lastCheckTime` on the latest-chapter line;
- `web/src/plugins/vuex.js` sorts shelf books by `durChapterTime` and updates it from reading state.

## Current OpenReader difference

| Layer | Current behavior | Decision |
|---|---|---|
| Data | `models.Book` has only GORM `CreatedAt`/`UpdatedAt`; there is no persisted upstream-equivalent `lastCheckTime`. | `must-fix` with a non-destructive nullable/default-compatible column and legacy backfill. |
| API | `/api/books` exposes `shelfOrderAt`, which intentionally merges progress and row times for ordering, but exposes no dedicated latest-chapter update timestamp. | `must-fix`; add `lastCheckTime` while retaining `shelfOrderAt` for ordering compatibility. |
| UI | `Home.latestChapterLabel()` falls back through `lastCheckTime || shelfOrderAt || updatedAt`. Because the first field is absent, reading progress or metadata edits can become the displayed latest-chapter time. | `must-fix`; the visible label must never consume `shelfOrderAt` or progress time. |
| Ordering | Backend and frontend sort by the newest of progress and shelf insertion/update timestamps, so a metadata edit/refresh can also move a book. | `must-fix`; match upstream `durChapterTime`: progress time when read, otherwise shelf insertion time. Generic metadata updates must not reorder the shelf. |
| Refresh | Remote refresh and scheduler save `Book`, changing generic `updated_at`; neither records whether new chapters were actually discovered. | `must-fix`; advance `lastCheckTime` only when chapter count increases. Failed/no-growth refresh must not advance it. |
| Source change | Upstream replaces the shelf metadata with a newly resolved `Book` whose `lastCheckTime` is initialized at source selection, while retaining reading progress. | `must-fix`; successful source change advances `lastCheckTime`, failed source change does not, and reading order remains progress-driven. |
| Import/restore | New books naturally receive GORM create time; reader-dev backups can contain `lastCheckTime`, but the current restore DTO discards it. | `must-fix`; initialize new rows from insertion time, import valid upstream milliseconds, and preserve it in OpenReader backup/restore. |

## Data migration contract

1. Add `books.last_check_time` as an integer millisecond timestamp matching reader-dev JSON.
2. Existing rows with zero/missing values are backfilled once from `created_at`, falling back to
   `updated_at` only if the create time is unavailable. Reading progress must not participate.
3. Migration is additive and idempotent; it must not rewrite titles, chapters, progress, archives,
   cache paths, category membership or generic timestamps.
4. New books receive a non-zero value at creation/import. Legacy reader-dev restore accepts a
   positive `lastCheckTime`; malformed/negative values fall back to the destination insertion time.
5. OpenReader and portable backups retain the field through the embedded `Book` JSON.

## Tests required before implementation

1. Frontend contract: a book with a recent `shelfOrderAt`/progress but an older `lastCheckTime`
   renders the older update label; no fallback to reading time or generic `updatedAt` is allowed.
2. API projection: list/single/mutation responses expose `lastCheckTime` independently of
   `shelfOrderAt`; progress updates may change order but not this value.
3. Refresh: adding chapters advances `lastCheckTime`; same-size, smaller, failed, and metadata-only
   operations do not. Scheduler follows the same rule.
4. Migration: an older `books` table gains the column and backfills from creation time without
   changing any existing data; a second migration is a no-op.
5. Backup/restore: reader-dev `lastCheckTime` survives restore and re-export; old backups without it
   receive a safe destination-time value.
6. Real browser: desktop `1440×900` and mobile `390×844` show update time on the latest-chapter line,
   while reading the book still moves it to the front without changing that label.

## Allowed differences

- OpenReader keeps ISO `shelfOrderAt` as an API projection for Go/Pinia multi-client ordering;
  reader-dev stores `durChapterTime` directly on the book. This difference is allowed only because
  both produce last-reading-first shelf order.
- Relative Chinese labels may remain in place of upstream's formatter, provided the timestamp source
  is exclusively `lastCheckTime` and the value updates under the same chapter-growth rule.

## Implementation evidence (2026-07-22)

- `models.Book.LastCheckTime` is an additive millisecond field. GORM initializes new rows; startup migration
  backfills only null/non-positive legacy rows from `created_at` (or `updated_at`/current time solely when the
  earlier source is unavailable) with `UpdateColumn`, so generic `updated_at` is not touched.
- Remote catalogue refresh and the scheduler compare the new catalogue to the persisted chapter count and advance
  the field only for real growth. Scheduler repair of a missing row below the already-known count does not advance
  it. Successful source change advances it with the new source snapshot.
- Legacy backup restore retains positive reader-dev `lastCheckTime`; missing/invalid values use the normal
  destination create initialization, while an existing destination row is not overwritten by a missing value.
  Standard `bookshelf.json` export now includes the field through the embedded Book model.
- Backend and frontend `shelfOrderAt` now use progress time or shelf insertion time. Generic metadata `updatedAt`
  is only a fallback for malformed legacy rows without creation time and cannot normally reorder a book.
- `Home.vue` renders only `book.lastCheckTime` before the latest chapter. A stale browser cache without the additive
  field shows `最新` until the network snapshot arrives; it never substitutes a reading timestamp.
- Fail-first contracts cover migration/idempotence, create initialization, growing/no-growth/repair refresh,
  source change, progress/order separation, reader-dev restore and backup export. Full frontend `512/512`, full Go
  tests and the Vite production build pass.
- Real Chromium passes the focused shelf contract at `1440×900`, `390×844` and `360×800`: a two-hour-old
  `lastCheckTime` is shown even when `shelfOrderAt` and `updatedAt` are current. The broader Index smoke continues
  past this proof and currently stops at its pre-existing unrelated “confirmed search BookInfo group property”
  assertion; that failure is not used as evidence for this slice.
- Commit `a10ad14` was pushed before release. The candidate passed fresh volume/backup and historical
  TXT/EPUB/UMD/CBZ/relative-cache/owner-isolation/portable-restore gates. The historical script's first run hit its
  known post-restart transient `404`; a clean immediate rerun completed every assertion.
- The image was built locally for linux/amd64 and linux/arm64 and published as `a10ad14` and `latest`; both point
  to OCI index `sha256:76c9851fb5a10d44722fff35986cd78d7aa43176faf8b1658d4d3f134a740a0d`.
  Platform manifests are amd64 `sha256:01377a52853ede692f50c940b53dc586e44886d704eeaed76f7fd9500b3123cb`
  and arm64 `sha256:b78c0689d8483e51177c79c2f9e262aa5ab08c92f9432d86454c5fbdea59e2da`.
