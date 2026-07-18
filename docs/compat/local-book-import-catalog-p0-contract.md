# Local-book import and catalogue P0 contract

Baseline: `changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`.

Audit date: 2026-07-18. This contract supersedes the earlier broad claim that the
local-import flow was fully aligned merely because a staged upload could be
reused. The TXT matcher itself remains aligned for the previously extracted
slice; the upload/preview/confirm state machine and parsed-result lifecycle do
not in the pre-implementation state recorded below.

Implementation status: **completed and validated on 2026-07-18** for the
preview/confirm lifecycle. The frontend now applies latest-request-only state
with transport cancellation. Direct, LocalStore and WebDAV successful previews
atomically store a versioned parsed snapshot; confirmation consumes the exact
matching rule/hash snapshot and old two-file stages rebuild it lazily. Failed
parses retain the last successful snapshot, failed durable imports compensate
their newly allocated archive, and all three stage files share owner-scoped
expiry/removal. The wider non-TXT parser-semantic audit remains open.

## Authoritative upstream evidence

- `web/src/views/Index.vue`
  - `onBookFileChange` uploads the selected bytes once to
    `/importBookPreview`.
  - the returned local `Book` and chapter list become the editable import
    dialog state.
  - `getChapterListByRule` reparses the server-side local asset; it does not
    upload the browser file again.
  - `saveBook(importBookInfo, true)` confirms the already prepared local book.
  - closing the dialog deletes the temporary local asset.
- `src/main/java/com/htmake/reader/api/controller/BookController.kt`
  - `importBookPreview` copies the upload into the caller's local asset area,
    builds `Book.initLocalBook(...)`, and returns `LocalBook.getChapterList`.
  - `getChapterListByRule` reads that same local asset and returns a new book /
    chapter pair.
  - `saveBook` moves/copies the prepared local asset into the durable book
    directory. It does not parse the whole source again before confirmation.
- `src/main/java/io/legado/app/model/localBook/TextFile.kt`
  - charset/rule detection uses the first 512,000 source bytes;
  - parsing after upload is local and therefore independent of network speed;
  - an explicit unmatched rule is an empty catalogue, while no selected rule
    uses deterministic 10-KiB pseudo chapters.

## Current OpenReader evidence

- `frontend/src/composables/useOverlayBookImport.js` uploads once, stores the
  returned `importToken`, and uses the token for rule refresh and confirmation.
- `frontend/src/api/books.js` allows ten minutes for preview; the old 12-second
  upload timeout is no longer present.
- `backend/api/local_import_stage.go` stores immutable caller-scoped raw bytes
  below `cache/import-previews/<user-id>/` for 24 hours.
- `backend/services/localbook/importer.go` calls
  `parseUploadedBookWithLimits(...)` in both `Preview` and `Import`.
- `backend/engine/epub_parser.go` reads and normalizes every readable spine
  resource during preview, including chapter content. Confirmation repeats
  that work, then writes one cache file per chapter.
- `useOverlayBookImport.preview()` has no request generation check or abort
  signal. A response from a superseded file/rule request can overwrite the
  latest dialog state, and a response can repopulate state after close/reset.

## Compatibility matrix

| Concern | Upstream contract | Current behavior | Classification | Required action |
|---|---|---|---|---|
| Browser upload count | Selected bytes are uploaded once. All later parsing uses a server-side local copy. | Direct upload creates a stage once; rule retries and confirmation submit only the token. | `technical-stack-equivalent` | Preserve the token flow and ten-minute transport timeout. Do not reintroduce repeated browser uploads. |
| Network independence after stage | Rule parsing and confirmation read only the local asset. | Token requests read immutable staged bytes. LocalStore/WebDAV token retries also ignore later mounted-source changes. | `acceptable-change` | Preserve caller scoping, 24-hour cleanup, and immutable snapshot semantics. |
| Preview response ordering | The upstream dialog has one active upload and explicit refresh actions; returned state corresponds to the current local `Book`. | Multiple asynchronous calls can overlap. Older success/failure responses can replace a newer file/rule preview or repopulate a closed dialog. | `must-fix` | Give each preview a generation, abort the superseded request, ignore stale completion, and make intentional cancellation silent. Closing/resetting the dialog must invalidate in-flight work. |
| Loading state ordering | The visible loading state represents the active operation. | Any older request's `finally` can set `previewing=false` while a newer request is still running. | `must-fix` | Only the current generation may mutate loading/error/data/token state. |
| Parsed-result lifecycle | Preview creates the server-side prepared local book and chapter catalogue; confirmation saves that prepared asset without reparsing the entire source. | Preview parses the entire source, discards `ParsedBook`, and confirmation parses it again. EPUB preview reads all spine content twice across preview + confirm. | `must-fix` | Store a bounded, caller-scoped parsed snapshot for the active token/rule and consume that exact snapshot on confirmation. A changed rule must atomically replace the snapshot only after successful parsing. |
| Final durability | Confirmation makes the prepared book durable and refreshes the shelf. | Original file, chapter caches, archive JSON and SQLite rows are assembled during confirmation. Filesystem failures can leave a partial archive because compensation is incomplete. | `must-fix` | Treat archive preparation as a compensating transaction: no shelf broadcast/token consumption before DB + required files succeed; remove a newly created archive on failure. Existing data must never be touched. |
| TXT automatic rule | First 512,000 bytes, enabled rules in reverse, one-match threshold, upstream tie overwrite. | Same extracted behavior. | `aligned` | Keep existing engine fixtures. |
| TXT explicit no-match | Empty catalogue remains visible and confirmable. | `200` with zero chapters and reusable token; confirmation can create a zero-chapter local book. | `aligned` | Preserve this state; do not translate it into a transport failure. |
| Java regex compatibility | Java multiline patterns are applied directly. | Known upstream lookbehind/lookahead forms are normalized for Go RE2; arbitrary unsupported Java constructs fail explicitly. | `acceptable-change` | Keep explicit client-safe errors and the staged token. Do not silently use a different rule. |
| EPUB TOC rules | The six `spin`/`toc` combinations are editable and refresh the local catalogue. | The six choices exist and parsing preserves spine/Toc title semantics plus fragment boundaries. | `partial` | Keep current semantic fixtures; add performance/lifecycle evidence proving one full parse for the confirmed rule. |
| Empty/corrupt/oversized input | Unsupported or unreadable files fail without adding a shelf item. | Parser/size limits run before book creation; token is retained after parser failure. | `aligned` for rejection order | Keep 413, parser limits, owner isolation and retry-stage tests. |
| Visible formats | Upstream direct picker exposes TXT/EPUB/UMD/CBZ. | Current picker exposes the same four formats; older API support for PDF/Markdown remains hidden. | `aligned` / compatibility extension | Keep hidden legacy API support and existing imported data readable. |

## Required tests before implementation

1. Frontend deferred-promise contract:
   - select file A, then file B; file A resolves last and cannot overwrite B;
   - start rule A, then rule B; rule A failure/success cannot alter B;
   - close/reset during preview; late completion cannot restore data/token or
     show an error;
   - only the latest request controls `previewing`.
2. API/service parse-count contract:
   - preview a staged TXT and EPUB under rule R;
   - confirm with the same token and R;
   - the full parser executes once, and the imported titles/content equal the
     preview snapshot.
3. Reparse replacement contract:
   - preview rule A, reparse rule B, confirm B;
   - confirmation consumes only B's snapshot;
   - failed/cancelled rule C leaves B confirmable.
4. Compensation contract:
   - inject a DB or chapter-file failure after archive creation;
   - no book/chapter/category row, broadcast, consumed token or orphan durable
     library directory remains.
5. Real-browser contract at 1440x900, 390x844 and 360x800:
   - delayed request ordering with a deterministic test fixture;
   - a representative multi-chapter TXT and EPUB complete preview/reparse/
     confirm and immediately appear on the shelf;
   - no page error, console error, duplicate upload, or 5xx response.
6. Mounted-data regression:
   - upgrade with existing TXT/EPUB/UMD/CBZ books and an old two-file stage;
   - old books remain readable; an old stage without a parsed snapshot remains
  retryable and is upgraded lazily on its next successful preview.

Implemented evidence:

- 426 frontend unit/contract tests;
- full `go test ./...`, including snapshot consumption, old two-file stage,
  failed-reparse retention, bounds/overflow, cleanup and archive compensation;
- production Vite build;
- `scripts/smoke/local-book-import-contract.mjs` against the real Go API at
  1440x900, 390x844 and 360x800, covering delayed stale response isolation,
  TXT reparse/confirm, EPUB preview/confirm and immediate shelf visibility.

## Data and release constraints

- No SQLite schema change is required for the preview snapshot.
- Snapshot files are derived cache only and must remain below the existing
  caller-scoped stage directory. They may be removed by the same expiry/clear-
  cache policy as the raw `.book`/metadata pair.
- Existing `.book` + `.json` stages must continue to load. Missing parsed data
  means “parse once and create it”, not “invalid token”.
- Snapshot serialization must be versioned and bounded by the existing import,
  archive-entry and extracted-text limits. It must never deserialize arbitrary
  executable types.
- A release is allowed after the race/lifecycle slice passes frontend, backend,
  browser and mounted-volume gates, even if the wider non-TXT parser audit is
  still open.
