# Book deletion consumer convergence P1 contract

Status: extracted on 2026-07-22 from the fixed upstream baseline
`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`.
This inventory pass changes no application code. It narrows the next P1-D
implementation slice to the state that survives after a successful single or
batch book deletion.

## Upstream authority

- `web/src/views/Index.vue#deleteBook`
- `web/src/components/BookManage.vue#deleteBookList`
- `web/src/views/Reader.vue#toShelf`, `deactivated`, `beforeDestroy`
- `web/src/App.vue` root-owned BookManage and BookInfo dialogs
- `web/src/router/index.js`

The upstream single-delete action confirms that book information and reading
progress will be removed, posts one `/deleteBook` request, then reloads the
bookshelf. Batch deletion clears the current selection, reloads BookManage and
reloads the root shelf. Upstream Reader is a separate `/reader` scene and exits
to `/`; a normal Index delete action therefore cannot leave that same mounted
client reading the deleted book.

## Current OpenReader evidence

- `backend/api/books.go#deleteBook`, `batchBooks`, and `deleteBookRecords`
- `backend/api/book_cleanup.go`
- `frontend/src/stores/bookshelf.js#removeBook`, `removeBookLocal`,
  `batchDeleteBooks`
- `frontend/src/composables/useSync.js`
- `frontend/src/stores/overlay.js`
- `frontend/src/components/GlobalOverlayHost.vue`
- `frontend/src/views/Reader.vue`
- `frontend/src/composables/useReaderPageLifecycle.js`
- `frontend/src/composables/useReaderProgressPersistence.js`

OpenReader intentionally adds authenticated WebSocket synchronization and can
therefore receive deletion while another tab/device has BookInfo, bookmarks,
content search, edit/group dialogs or Reader open. That state has no direct
upstream equivalent, but it must converge to the same post-delete product state
instead of keeping an operable stale book.

## Compatibility matrix

| Contract | Fixed upstream behavior | Current OpenReader behavior | Decision |
|---|---|---|---|
| Single delete API | After confirmation the selected shelf item, its progress and its private book data are removed; the shelf is reloaded. | `DELETE /api/books/:id` verifies current-user ownership, deletes `BookCategory`, `Chapter`, `Bookmark`, `ReadingProgress` and `Book` rows transactionally, then prunes only unreferenced remote cache/image artifacts or the owner's direct-import directory and broadcasts `bookshelf_delete`. | `aligned + hardened runtime adaptation`; retain `204`, ownership isolation and post-commit cleanup/broadcast. |
| Batch delete API | Selected rows are removed, selection is cleared, and both management list and shelf reload. | `POST /api/books/batch` validates all ids as caller-owned, performs each row cleanup in one transaction, returns `{ affected, deletedIds }`, prunes artifacts after commit and sends one ids broadcast. | `technical-stack-equivalent`; preserve returned-id authority and reject mixed/foreign ownership without partial deletion. |
| Shelf/cache consumer | Reloaded upstream lists no longer expose the deleted item. | Direct and sync paths remove shelf memory/cache rows and browser chapter cache. They do not clear `reader.progressByBook` or scoped/legacy local progress. | `must-fix`: every confirmed deleted id clears shelf entries, chapter cache and local reading progress before a future sort/reopen can consume them. |
| Root book overlays | Upstream root reload makes a deleted shelf record unavailable; Index and Reader cannot remain mounted concurrently. | BookInfo, BookEdit, BookGroup(set), bookmarks, bookmark form and content search retain independent book objects after `removeBookLocal`. Operations can then issue requests for a deleted id. | `must-fix`: close and clear only per-book scenes targeting a deleted id. Resolve an outstanding bookmark-form promise as unsaved/deleted. Keep unrelated books and shelf-wide BookManage/BookGroup(manage) open. |
| BookManage selection | Upstream batch success clears selection and reloads its table. | The table derives rows from Pinia, but a remote deletion can leave ids in controller selection until another interaction/close. | `must-fix`: prune deleted/nonexistent ids from active selection while keeping the manager usable. Local batch success still clears all selected ids as upstream does. |
| Active Reader on another client | Not reachable in the upstream single-client route topology; returning to shelf saves the existing book's progress while it still exists. | A `bookshelf_delete` event only removes the shelf row. Reader remains on `/books/:id/read`; route leave/unmount still force a keepalive progress write, which can target an already deleted book. | `must-fix runtime adaptation`: for the active id, atomically mark deletion, cancel pending saves/automatic reading, suppress all later progress writes, close per-book overlays, and `router.replace({ name: 'home' })`. Do not push a stale Reader entry into history and do not recreate progress. |
| Ordering and duplicate events | Upstream performs one successful mutation then reloads. | The initiating client can both reconcile its direct API response and receive its own WebSocket broadcast; cache clearing is asynchronous. | `must-fix`: deletion convergence is idempotent. Duplicate/out-of-order events for the same id must not throw, navigate twice, affect another book or reintroduce a removed shelf/cache/progress row. |
| Failure semantics | A failed/cancelled delete leaves the shelf and progress intact and reports failure/cancel. | Direct store mutation happens after API success, while sync events represent committed server state. | `aligned`; do not perform consumer cleanup before API success. Best-effort browser-cache failure must not roll back a committed server deletion. |

## Required state transition

For a normalized, unique set of positive `deletedIds`:

1. The backend database transaction commits before artifact cleanup and before
   the sync event. Existing API status/response schemas remain unchanged.
2. Each client removes only those ids from shelf memory and persisted shelf
   snapshots, clears browser chapter cache, and clears scoped plus legacy local
   progress. Repeating the transition is a no-op.
3. The root overlay owner closes only scenes whose target id is deleted:
   BookInfo, BookEdit, BookGroup in `set` mode, bookmarks, bookmark form and
   content search. Shelf-wide managers and scenes for other books remain open.
4. An active Reader for one deleted id suppresses progress persistence before
   any route transition, cancels queued work, and replaces the route with Home.
   Its route-leave and unmount hooks must observe the suppression flag.
5. A Reader for another id is untouched. A temporary remote Reader without a
   matching persisted shelf id is untouched.

## Tests required before implementation

1. Backend API contracts retain single/batch cascade, ownership isolation,
   shared-cache reference safety, direct-import ownership and exact
   `bookshelf_delete` payloads. Add payload assertions only if current tests do
   not already observe the Hub boundary; do not change the API merely for UI.
2. Pinia/unit contract: direct, batch and sync deletion clear shelf cache,
   browser chapters and local progress; duplicate ids are harmless; failed API
   deletion changes nothing.
3. Overlay contract: matching per-book scenes close/clear, bookmark-form promise
   resolves without save, BookManage and BookGroup(manage) remain, and another
   book's scenes remain untouched.
4. Reader contract: matching deletion cancels progress/auto-read and performs
   one Home `replace` with no subsequent `PUT /api/progress`; nonmatching and
   temporary-reader cases remain mounted.
5. Real Go + two-browser-context smoke at `1440x900`, `390x844`, and `360x800`:
   client A opens BookInfo, bookmarks/search and Reader in separate phases;
   client B deletes the book through the real API/UI. Client A must converge
   without 404/500 loops, stale overlays, console errors or horizontal
   overflow, while an unrelated book remains usable.

## Allowed differences and non-goals

- WebSocket multi-client convergence, current-user scoping, transactional row
  cleanup and reference-safe artifact cleanup are required OpenReader runtime
  adaptations.
- Vue 3/Pinia event/store composition may differ from upstream; the visible
  post-delete state and user data result may not.
- No schema migration, route removal, shared LocalStore/WebDAV source deletion,
  another user's cleanup, or redesign of delete confirmation copy is authorized
  in this slice.
- This contract does not reopen already validated cache/source refresh or book
  metadata editing behavior.
