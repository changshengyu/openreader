# RSS visible workspace fixed-baseline second-audit P2 contract

Status: `aligned / Docker-published / awaiting-device-verification` on 2026-08-09. This audit
supersedes every earlier conclusion that treated the richer OpenReader RSS
cards, form editor, filters, refresh actions, article metadata, or eager
multi-page refresh as an allowed upstream-equivalent UI. No RSS application or
test code was changed during this inventory pass.

Fixed baseline:
`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`.

## Authority and current mapping

| Contract layer | Fixed upstream authority | Current OpenReader mapping |
| --- | --- | --- |
| Source dialog | `web/src/components/RssSourceList.vue`, root `web/src/App.vue` | `frontend/src/components/overlays/OverlayRSS.vue`, `frontend/src/components/RSSManager.vue` |
| JSON editor and import selection | `web/src/App.vue#showEditorListener`, `web/src/views/Index.vue#onSourceFileChange/#saveSourceList` | `RSSManager.vue#openEditor/#saveSource/#importRSSSources`, `frontend/src/utils/rssSourceImport.js` |
| Article-list dialog | `web/src/components/RssArticleList.vue` | `RSSManager.vue#selectSource/#handleSortChange/#loadArticles/#loadMoreArticles` |
| Article-content dialog | `web/src/components/RssArticle.vue`, root `web/src/App.vue` | `RSSManager.vue#openArticle`, nested article/image dialogs |
| Fetch/page API | `RssSourceController.kt#getRssArticles/#getRssContent`, `io/legado/app/model/rss/Rss.kt` | `frontend/src/api/rss.js`, `backend/api/rss.go`, `backend/engine/rss_parser.go` |
| Source/article storage | upstream per-user JSON namespace | authenticated REST IDs, user-scoped SQLite RSS sources/articles and browser cache |

## Visible source-scene contract

| Behavior | Fixed upstream contract | Current result | Required action |
| --- | --- | --- | --- |
| Root shell | One small centred dialog, exactly `500px` wide on desktop and fullscreen in mini/mobile mode. Its title is `RSS订阅(N)`. | The root is centred/fullscreen, but is `760px`, titled `RSS 订阅`, and contains a second bordered panel/header. | `must-fix`: use the upstream shell/title/width and remove the duplicate panel chrome. |
| Header actions | The title contains right-floated `编辑/取消`, `导入`, `新增`; their visual left-to-right order is `新增、导入、编辑/取消`. There is no refresh action. | Separate Element Plus buttons include edit/import/new plus a global refresh. | `must-fix`: restore the title actions and remove refresh. |
| Source layout | Sources sorted by `customOrder`; fixed four-column `25%` tiles at every width. Each tile contains only a `50x50` icon and source name. | Responsive cards expose fallback initials, group, enabled badge, active state and per-source tools. | `must-fix`: restore the four-column icon/name grid; remove group/badge/active/refresh metadata from this scene. |
| Edit mode | Edit mode alone overlays close at `right:6/top:8` and edit at `right:6/top:42` (`right:-5` on very small screens). Clicking the rest of the tile still opens articles. | Edit mode renders labelled refresh/edit/delete buttons under every card. | `must-fix`: use the two overlay icons and preserve tile-open behavior. |
| Empty/loading extras | Upstream has no dedicated empty-state card and no source-list loading mask. | OpenReader renders `el-empty` and loading UI. | `must-fix visible structure`: a request may still be safely guarded, but must not add an invented source-list scene. |
| Dialog ownership | Source, article list, article content and JSON editor are sibling dialogs. Opening a child does not close its parent. | The three RSS dialogs already coexist under one manager, but the editor is a nested structured form. | `partially aligned`: retain independent visibility and stale-response guards; rebuild the editor as the sibling-equivalent JSON dialog. |

## Source editor and import contract

### Manual editor

- `新增` starts a complete JSON draft with exact upstream defaults:
  `sourceName="新增RSS源"`, empty URL/icon/group, `enabled=true`,
  `singleUrl=true`, `articleStyle=0`, empty article/title/date/image/link/content
  rules, and `enableJs=true`.
- Existing-source edit serializes the whole source as four-space-indented JSON.
- The dialog title is exactly `编辑RSS源`. Desktop width follows the shared
  general dialog width (`750px` to `1000px`); mini/mobile is fullscreen.
- The body is a JSON/code editor, not a structured form or collapsed "advanced"
  section. Footer labels are `取 消` and `保 存`.
- Save parses JSON, then checks `sourceName` and `sourceUrl` in that order.
  Errors are exactly `RSS源名称不能为空`, `RSS源链接不能为空`, and
  `RSS源必须是JSON格式`. Success is `保存RSS源成功`.
- Same `sourceUrl` replaces that user's existing source. REST row IDs may remain
  an internal Go adaptation and must not change this visible identity.

The previous acceptance of an empty manual title is withdrawn. The default
must be `新增RSS源`, because it is both visible and explicitly created by the
fixed upstream component.

### File import

1. `导入` opens the file picker. A non-empty JSON array becomes a separate
   `导入RSS源` selection dialog; invalid/non-array/empty data reports
   `RSS源文件错误`.
2. The selection dialog initially selects nothing. Every row is a checkbox
   showing source name, source URL and `@Javascript`/`@WebView` risk tags.
3. The footer contains bordered `全选`, `已选择 N 个`, `取消`, `确定`.
   Cancel closes the dialog and clears the selected indices.
4. "全选" excludes records containing `@js:` or `webView:` and reports
   `部分使用了Javascript和Webview的书源未勾选`. The fixed upstream's
   `.filter(v => v)` accidentally drops safe index `0`; OpenReader must keep the
   intended first safe item selectable rather than copying that truthiness bug.
5. Confirm with no selection reports `请选择需要导入的源`. Confirm submits only
   checked records as one logical batch. Blank name/URL rows are skipped and
   same-URL rows replace existing rows. Success reports `导入RSS源成功`, reloads
   sources, closes the import dialog and clears selection.

OpenReader may implement the batch as one authenticated transactional endpoint
instead of sequential REST calls. It must not return to the current yes/no
"import all" confirmation, nor silently invent names for blank records.

## Article-list and content contract

| Behavior | Fixed upstream contract | Current result | Required action |
| --- | --- | --- | --- |
| List shell | Independent `500px` desktop/fullscreen mini dialog titled only with `sourceName`. No nested panel header. | `900px` dialog plus `文章`/count header. | `must-fix`. |
| Sorts | Only when parsed `sortUrl` has more than one non-empty newline-delimited `name::url` row and `singleUrl=false`; first row is active on open. Tab switch resets page to 1 and `hasMore=true`. | A select supports newline and added `&&`; it also coordinates filters/cache. | `must-fix visible control`: restore tabs and upstream newline parsing. Retaining `&&` only in persisted import compatibility is allowed; it must not alter fixed-baseline visible parsing. |
| Rows | Each row contains title, `pubDate`, and optional image only. Desktop padding `15px 10px`, image `120x75`; mini padding `15px 5px`, image `100x62.5`. | Rows also show author, summary, read opacity, read/favourite actions and extra chrome. | `must-fix`: remove all extra visible metadata/actions. |
| Load more | Always render `加载更多` or `没有更多啦`. Clicking while available requests exactly `page + 1`; page 1 replaces and later pages append. An empty page reports `没有数据`, leaves earlier rows intact, and sets no-more. | Cached DB pagination occurs independently of the remote refresh; source-open eagerly refreshes every remote page first. | `must-fix frontend and backend`: the requested remote page is the operation unit. |
| Open article | Row click requests content first. The article dialog opens only after successful content fetch. | Dialog opens immediately with a loading state, marks read, then fills content. | `must-fix`: fetch first; failure leaves the content dialog closed. Hidden cached read state must not alter the visible flow. |
| Content shell | Independent `500px` desktop/fullscreen mini dialog titled with article title. Body renders only `content || description`; images/videos max width `100%`; image click opens viewer. | Body duplicates title/date/author and footer actions for close/external/favourite. | `must-fix`: remove duplicate metadata/footer/actions, retain sanitized HTML and image preview. |

## Page/API translation contract

The current `POST /api/rss/sources/:id/refresh` calls
`fetchRSSRuleArticles`, which follows next-page links (up to 1000 pages) before
returning. This is not equivalent to upstream: `getRssArticles` receives one
`page` and `Rss.getArticles` performs one request for that page.

The Go/REST translation must satisfy all of the following:

1. Article-list open requests remote page `1` for one authenticated source and
   selected sort. Load more requests exactly the next page. One click may not
   crawl later pages.
2. The refresh/fetch endpoint accepts a bounded positive `page` (default `1`),
   resolves the source and sort within the authenticated user's ownership, and
   returns the rows fetched for that page plus `page`/`hasMore` metadata.
3. Rule source pagination evaluates URL/page templates for the requested page.
   When rule-next-page navigation cannot be addressed without prior URLs, the
   backend may maintain a user/source/sort cursor cache, but work must remain
   bounded to the requested transition and may not scan to page 1000.
4. Standard XML/Atom feeds remain a single remote page. Page 1 returns the feed;
   page greater than 1 returns an empty page without refetching an unbounded
   history.
5. User-scoped SQLite may upsert fetched rows for offline/cache compatibility.
   The response shown for page N is the ordered page-N fetch result, not a
   newly-sliced global database cache that can reshuffle or duplicate earlier
   pages.
6. Empty page sets `hasMore=false`. A non-empty page remains loadable according
   to the parser's next-page result. Source/sort/page request generations prevent
   a stale response from mutating a newer or closed dialog.
7. `getRssContent` remains a separate source-owned content action. Stored HTML
   sanitization, URL resolution, request timeout/size/redirect/rate/SSRF limits,
   and same-user ownership are mandatory security adaptations.

Existing `/api/rss/articles` and read/favourite state endpoints may remain as
hidden compatibility APIs. The rebuilt upstream-visible list must not expose
filters or controls that depend on them.

## Allowed differences

- JWT authentication, numeric REST IDs, Pinia and Vue 3/Element Plus mechanics.
- Per-user SQLite source/article cache, browser cache scope, WebSocket invalidation
  and atomic source deletion.
- Sanitized article HTML, safe image preview, bounded remote requests, SSRF and
  redirect protection, parser work limits, operation generations and account
  isolation.
- Hidden read/favourite storage and legacy endpoints, provided they do not add
  controls, reorder rows, dim text, auto-open dialogs, or change page semantics.
- Correctly selecting safe import index `0`, as the intended behavior of the
  upstream `全选` control and a correction of its implementation bug.

No current component shape or existing passing test is a retention reason.

## Test-first implementation gates

Before application code changes, replace tests that require the current rich UI
with failing fixed-baseline contracts:

1. Source dialog: exact title/500px/fullscreen, three title actions, four columns,
   icon/name-only tiles, edit overlays, and absence of refresh/badges/group/empty
   cards.
2. Editor/import: exact JSON defaults and validation; shared JSON editor;
   initially-unchecked selection dialog; safe select-all and selected-only batch;
   same-URL replacement.
3. Article list/content: tabs, title/date/image-only rows, exact geometry/text,
   fetch-before-open, content-only body and image preview.
4. Backend parser/API: page 1 performs one remote page request; page 2 performs
   only the next requested transition; no eager crawl; empty page and standard
   feed end semantics; stale source/sort/page work cannot commit.
5. Preserve API/data/security regression tests for name/URL identity,
   same-user replacement, cross-user isolation, transactional delete,
   sanitization, request bounds, backup/restore and session invalidation.
6. Real browser at `1440x900`, `1024x1366`, `390x844`, and `360x800`: source
   grid -> edit/new JSON -> import selection -> article page 1 -> load more ->
   content -> image -> close child/parent/reopen. Assert parent dialogs remain,
   mobile fullscreen, desktop widths, no horizontal overflow/click-through,
   no duplicate or eager requests, and no stale response commits.
7. Full frontend tests, Go tests, production build and `git diff --check`; local
   Docker/volume/backup gates before publishing a candidate.

Implementation order after this contract is committed: replace conflicting
tests, rebuild source/editor/import visible structure, rebuild article/content
visible structure, then change the page/API/parser data flow. Do not combine the
contract pass with implementation.

## 2026-08-09 implementation and validation result

The inventory contract and requested-page API contract were committed and
pushed before application code changed (`1283f03`, `dff7c86`). The implementation
then replaced the tests that had encoded the richer OpenReader scene and rebuilt
the feature in the contract order:

- The root is now the one `500px`/mobile-fullscreen `RSS订阅(N)` dialog. Its
  visible source scene contains only `新增、导入、编辑/取消` and four `25%`
  icon/name tiles with the fixed `50x50` icon geometry. The old refresh,
  group/enabled badges, active state, rich cards and invented empty scene are
  gone.
- New and edit use the independent `编辑RSS源` JSON dialog with the exact
  upstream draft and validation order. Import uses an initially unchecked
  checkbox dialog; safe select-all correctly keeps safe index zero and excludes
  `@js:`/`webView:` rows. The selected records are submitted by one authenticated
  transactional batch endpoint.
- Article list and article content are independent `500px`/mobile-fullscreen
  sibling dialogs. The list has only optional newline `name::url` tabs,
  title/date/image rows and load-more state; content is fetched before its dialog
  opens and the body renders sanitized `content || description` with image
  preview. Hidden read/favourite APIs and data remain non-visible compatibility
  capabilities.
- `POST /api/rss/sources/:id/refresh` now validates the owned source/sort and
  executes exactly the requested page. Standard feeds return no network work
  after page one; rule pages compare prepared request descriptors and do not
  eagerly crawl later pages. Page rows are upserted transactionally while
  preserving hidden read/favourite state and returned in parser order.
- Source, sort, page, workspace and authenticated-operation generations prevent
  delayed requests or writes from committing into a closed scene or another
  account. Import broadcasts only after commit.

Validation evidence:

- frontend: `706/706`; backend: `go test ./...` passed; Vite production build
  and `git diff --check` passed.
- `rss-workspace-contract.mjs` passed at `1440x900`, `1024x1366`, `390x844`
  and `360x800`, covering source/editor/import/list/content geometry, parent-child
  coexistence, safe import, requested pages `1 -> 2`, image preview, no legacy
  article-list request, no overflow and late-source isolation.
- `workspace-operation-contract.mjs` passed at desktop and both phone sizes;
  the RSS pending-write branch of
  `workspace-overlay-session-isolation-contract.mjs` passed at the same three
  viewports with no stale toast, event or reload after account replacement.

The new slice does not create a caller-selected remote fetch capability:
`sortUrl` must match the authenticated source's base/sort options, import is
bounded to 8 MiB/5000 records, and page is bounded. The inherited shared source
fetcher still lacks a universal response-byte cap, explicit redirect cap and
private-network SSRF rejection. Those pre-existing cross-source hardening items
remain open in `docs/security-review-checklist.md`; this RSS alignment result
does not claim that separate security project is complete.

## Docker publication

Implementation commit `92b70347ff1f6ced1381e63140bb49a8ab6cde3d` was
pushed to `main` before publication. OrbStack built the candidate and formal
multi-architecture release locally; no cloud build was used. Published tags:

- `ghcr.io/changshengyu/openreader:92b7034`
- `ghcr.io/changshengyu/openreader:latest`

Both tags resolve to OCI index
`sha256:095540fe28c553ab6d2cfd9ce589ec0b31809f00600cd9f59ab814b7753098ba`
with `linux/amd64` manifest
`sha256:48f5bf95817e46770e1b6c1f22cc41bb7fa65270b51da3650d3128ccde8ea619`
and `linux/arm64` manifest
`sha256:2d87fc602595ca5ae11f19ff724ba669f016b116e41e2faf9c8184adcfacab36`.

Before the formal push, the local arm64 candidate passed a fresh-volume gate
covering portable v1, portable v2 assets, cross-user isolation and restart. It
also passed the historical mounted-volume gate for TXT, EPUB, UMD, CBZ,
relative-cache and owner isolation. The module now awaits device verification;
the separate shared-fetcher security debt remains explicitly unfinished.
