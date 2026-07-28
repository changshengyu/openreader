# RSS source lifecycle P2 compatibility contract

Status: audited and implemented on 2026-07-27; reopened, fixed,
regression-validated and Docker-published on 2026-07-28 for the manual-create
editor transition described below. This contract was extracted before changing
the corresponding RSS application behavior.

Fixed baseline:
`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`.

## Authority map

| Contract area | Fixed upstream authority | OpenReader authority |
| --- | --- | --- |
| Source list and editor | `web/src/components/RssSourceList.vue` | `frontend/src/components/RSSManager.vue`, `frontend/src/components/overlays/OverlayRSS.vue` |
| Import and same-URL replacement | `web/src/views/Index.vue#onSourceFileChange`, `#saveSourceList`; `RssSourceController.kt#saveRssSources` | `RSSManager.vue#normalizeRSSSourceImport`, `frontend/src/utils/rssSourceImport.js`, Go RSS source endpoints |
| Source defaults and aliases | `io/legado/app/data/entities/RssSource.kt` | `backend/api/rss.go#rssSourceRequest`, `backend/models/models.go#RSSSource` |
| Article-list lifecycle | `web/src/components/RssArticleList.vue` | `RSSManager.vue#selectSource`, `#handleSortChange`, `#loadArticles`, `#loadMoreArticles` |
| Source deletion | `RssSourceController.kt#deleteRssSource` | `backend/api/rss.go#deleteRSSSource` |

## Extracted contract and audit result

| Behavior | Fixed upstream contract | Current OpenReader result | Classification / required action |
| --- | --- | --- | --- |
| Three-level scene ownership | Source list, article list and article content are independent dialogs. Closing a child returns to its parent; closing the root resets the complete RSS scene. | Root, list and content are already independent dialogs, including compact fullscreen behavior and image preview. | `aligned`; retain existing three-dialog browser contract. |
| Manual source defaults | A newly opened editor starts with `sourceName="新增RSS源"`, `singleUrl=true`, `articleStyle=0`, `enabled=true`, `enableJs=true`. | Boolean/rule defaults match. The title starts empty rather than using the upstream placeholder. | `minor visible difference`; using an empty title is acceptable only if save rejects it exactly as the upstream contract requires. |
| Required source identity | Both source name and source URL are required. The editor refuses to submit either blank value, and the upstream controller rejects either blank value. | Frontend checks only URL. Go create/update silently replaces an empty title with the URL. | `must-fix`; frontend must block blank name and URL, and backend must return `400` without mutating data. |
| Same-URL save | A single-source save finds an existing source by `sourceUrl` and replaces it instead of appending a duplicate. Batch import uses the same URL identity and skips records without a name or URL. | JSON import plans same-URL records as updates, but manually creating a source with an existing URL inserts another row. | `must-fix`; preserve REST IDs internally while making user-visible create-by-URL idempotent per user. Do not affect another user's source. |
| `singleUrl` import default | `RssSource.singleUrl` defaults to `false`; JSON conversion also uses `false` when the field is absent. The manually-created editor is the special case that explicitly starts at `true`. | Manual editor/backend create default to `true`; imported records without the field normalize to `false`. | `aligned`; do not change the import default to `true`. |
| Source order and fields | Source list is ordered by `customOrder`; imported Legado names and transport/parser fields are retained. | API orders by `custom_order`, supports current/upstream aliases, and persists the extracted fields. | `aligned Go adaptation`; keep existing API/data tests. |
| Article-list request ownership | Opening a source establishes one source/sort/page-one state. Switching sort discards the previous list state. Closing the article-list scene means its pending work can no longer change visible state. | A completed request checks only root `props.visible`. A request for source A, an old sort/filter, or a closed article-list dialog can overwrite or append to the current list. Old `finally` blocks can also clear the loading state owned by a newer request. | `must-fix`; use request generations plus a source/sort/filter snapshot. Invalidate list and load-more work on source/sort/filter changes and child/root close. Commit results and loading flags only for the current generation. |
| Article-content request ownership | Article content belongs to the currently-open article dialog. | `articleOpenRequest` already invalidates late content responses on reset and article change. | `aligned`; retain and regression-test. |
| Source deletion atomicity | The visible operation is one source deletion. The upstream JSON storage performs one source-list replacement; it has no separately persisted article cache to leave half-deleted. | SQLite deletes cached articles first, then the source. A source-delete failure or a not-found source can leave article rows deleted. | `must-fix Go adaptation`; delete source and user-scoped article rows in one transaction. Roll back articles on source failure or zero affected rows; broadcast only after commit. |
| OpenReader enhancements | Upstream does not persist read/favorite filters or sanitize stored article HTML. | Per-user article cache, unread/favorite state, pagination, bounded remote fetches and sanitized HTML are present. | `allowed enhancements`; preserve user isolation, fetch limits, stored state and sanitization. |

## API contract

### `POST /api/rss/sources`

- Accept current `title`/`url` and upstream `sourceName`/`sourceUrl` aliases.
- Trim both identity fields.
- Blank title: `400 {"error":"title is required"}` and no insert/update.
- Blank URL: `400 {"error":"url is required"}` and no insert/update.
- If the authenticated user already owns a source with the same normalized URL,
  update that row and return it instead of creating a second row.
- Never match or mutate a same-URL source owned by another user.
- Preserve manual-create defaults (`singleUrl=true`, `enabled=true`,
  `articleStyle=0`, `enableJs=true`, `loadWithBaseUrl=true`) when fields are
  omitted.

### `PUT /api/rss/sources/:id`

- The row must belong to the authenticated user.
- Blank title or URL returns the same `400` contract and leaves the existing row
  unchanged.
- ID remains the internal update authority. Changing a URL to one already owned
  by a different row must not silently create another duplicate; return a stable
  `409 {"error":"RSS source URL already exists"}`.

### `DELETE /api/rss/sources/:id`

- The source lookup/deletion and deletion of its user-scoped cached articles are
  one SQLite transaction.
- Missing or cross-user source: `404 {"error":"RSS source not found"}` with no
  article mutation.
- Any database failure: rollback all mutations and return `500`.
- Emit `rss_update/source-delete` only after commit.

## Frontend state-transition contract

The article query identity is:

`rootVisible + articleListVisible + sourceId + selected sort + filter + page`.

1. Source selection resets article/content/image state, chooses the first valid
   sort, opens the article-list dialog, then loads and refreshes that source.
2. A source, sort or filter transition invalidates every older list and
   load-more request before starting the replacement request.
3. Closing the article-list dialog or root RSS dialog invalidates all pending
   article-list and load-more requests.
4. A response may replace/append rows, page state, `hasMore`, or loading state
   only when both its generation and captured query identity are still current.
5. Source/import/delete/sync refreshes may reload articles only while the
   selected article-list scene remains open.
6. Article content keeps its existing independent request generation.

## Required regression evidence

1. Frontend source contract:
   - blank title and URL are rejected before API submission;
   - absent imported `singleUrl` remains `false`;
   - manual new-source default remains `true`.
2. Frontend async contract:
   - delayed source-A response cannot overwrite source B;
   - delayed old-sort/filter response cannot overwrite the current query;
   - delayed load-more cannot append after close/reset;
   - an old request cannot clear a newer loading indicator.
3. Backend API/data contract:
   - create/update reject blank title and preserve existing data;
   - same-user same-URL create updates without adding a row;
   - cross-user same URL remains isolated;
   - update URL collision returns `409`;
   - a forced source-delete failure rolls article deletion back;
   - missing source deletion does not remove orphaned/user-scoped article rows.
4. Existing Go tests, frontend tests and production build.
5. Real Chromium at `1440x900`, `390x844`, and `360x800`: source → article list
   → content → close/reopen, plus delayed source-switch response ordering.

Docker is eligible only after the implementation and all five gates pass.

## 2026-07-28 manual-create editor transition audit

The authenticated-overlay browser gate exposed a separate regression in the
already-audited manual source flow.

| Behavior | Fixed upstream contract | Current OpenReader evidence before implementation | Classification / required action |
| --- | --- | --- | --- |
| Click `新增` | `RssSourceList.vue#editRssSource(false)` normalizes the falsy value to a complete new-source object, then immediately opens the shared editor. No network request is required to create the draft. | `RSSManager.vue#openEditor()` defaults its argument to `null`, then passes that value to `pickRSSAdvancedFields()`. JavaScript default parameters do not replace an explicit `null`, so the helper reaches `Object.prototype.hasOwnProperty.call(null, field)` and throws before `editorVisible=true`. Real Chromium records `TypeError: Cannot convert undefined or null to object`; the editor never appears. | `must-fix`; normalize null/falsy manual-create input before every field read. |
| New-source defaults | Upstream starts with `sourceName="新增RSS源"`, empty URL/icon/group, `enabled=true`, `singleUrl=true`, `articleStyle=0`, and `enableJs=true`. | OpenReader intentionally uses an empty visible title, but otherwise owns the same manual defaults. This difference was already accepted because both clients reject a blank title at save time. | `aligned with documented minor visible difference`; retain the current empty title and all existing defaults. |
| Editor lifecycle observability | Upstream owns the editor as a root event-bus dialog. Closing the RSS root removes the scene. | The Vue 3 nested editor had no stable component class even though the authenticated-session browser contract already listed `.rss-source-editor-dialog` as a private overlay. | `must-fix testability adaptation`; add the stable root class without changing layout or interaction. |

Required regression evidence before reclosing this contract:

1. A frontend contract test proves that the manual-create path normalizes
   `null` before advanced-field extraction and retains `singleUrl=true`.
2. Real Chromium can click `新增`, submit a deliberately pending create
   request, invalidate account A, authenticate account B, and prove the editor
   closes, the delayed A result emits no toast/event/reload, and manual reopen
   contains B data only.
3. The complete Overlay session-isolation matrix passes at `1440x900`,
   `390x844`, and `360x800`.
4. Frontend full tests, production build, Go full tests and `git diff --check`
   pass before the implementation is committed.

Implementation result:

- `openEditor()` now normalizes the upstream falsy new-source sentinel before
  reading identity or advanced fields. Existing-source editing is unchanged;
  manual creation still starts with an empty OpenReader title and the upstream
  `singleUrl=true/articleStyle=0/enabled=true/enableJs=true` defaults.
- The nested editor now exposes `.rss-source-editor-dialog`, allowing session
  invalidation checks to prove that it is removed together with the private RSS
  scene.
- The focused contract test first failed on the unsafe `null` transition and
  passes after implementation. Frontend full tests pass `645/645`; production
  build, Go full tests and `git diff --check` pass.
- Real Chromium passed RSS pending-create isolation and the complete Overlay
  matrix at `1440x900`, `390x844`, and `360x800`. The delayed account-A write
  emitted no toast, business event or write-after-login reload, and manual
  reopen contained only account-B RSS data.
- The locally built `342d736` candidate passed ordinary and historical mounted
  volume/backup gates. `ghcr.io/changshengyu/openreader:342d736` and `latest`
  were then published locally as amd64/arm64 index
  `sha256:1643625269f5a04f867c56da9e3bee04c1318d807e73ca6fc0913ab408645921`.

## Implementation record

- The RSS editor now rejects a blank source name before submission. Go
  create/update endpoints enforce the same trimmed name and URL contract without
  falling back from an empty name to the URL.
- Creating a source with a URL already owned by the authenticated user now
  updates that row in place. An ID-based update that collides with another
  same-user URL returns `409`; another user's matching URL is never considered.
- Source deletion now performs the ownership lookup, article-cache deletion and
  source deletion in one GORM/SQLite transaction. Forced source-delete failures
  and missing-source requests leave article rows untouched.
- Article list and load-more requests now carry independent generations and the
  full root/list/source/sort/filter/page identity. Child/root close and every
  query transition invalidate old work; only the current request controls rows,
  pagination, errors and loading indicators.
- The source-selection and sort-selection continuations also retain their
  captured query identity, so a late old list response cannot trigger a refresh
  for either the old or newly-selected source.
- The upstream import distinction remains unchanged: manually-created sources
  default `singleUrl=true`, while imported JSON without the field defaults to
  `false`.

Validation:

- Frontend: `npm test` passed **579/579** tests.
- Backend: `go test ./...` passed, including forced transaction rollback,
  not-found preservation, same-URL replacement, cross-user isolation and
  collision tests.
- Production frontend build passed.
- Real Chromium passed the complete RSS source → list → content → image →
  close/reopen sequence at `1440x900`, `390x844`, and `360x800`.
- The same browser gate delayed source-one list data, closed that child dialog,
  opened source two, and verified that source-one neither overwrote rows nor
  caused a second refresh.

Docker release:

- Git/image version: `0f636f1`
- Tags: `ghcr.io/changshengyu/openreader:0f636f1` and
  `ghcr.io/changshengyu/openreader:latest`
- Multi-architecture index:
  `sha256:25398a86aa809b5ce1157f64dee1fd0d85673089480223a7375eae9bb86c6f5c`
- `linux/amd64`:
  `sha256:54a4f9eb1bb0725dca07c839ecdd3fc2b3482d92c918762cedc58e1b2d10d575`
- `linux/arm64`:
  `sha256:d487ee131e37dc3afa6d2518b5686e931d7dca872210b374cb6f8357c57b3c38`
- The ordinary mounted-volume smoke passed portable v1, portable v2 assets,
  cross-user isolation and restart recovery.
- The historical-volume smoke passed TXT, EPUB, UMD, CBZ, relative-cache and
  owner-isolation fixtures.
