# Reader-dev vs OpenReader Gap Analysis

Baseline: `changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`.

Local upstream checkout used for this pass: `/private/tmp/changshengyu-reader-dev`.

Status labels:

- `must-fix`: required to match upstream/user-visible behavior or avoid known bug.
- `acceptable-change`: allowed technical adaptation or explicit user-requested improvement.
- `intentional-redesign`: deliberate OpenReader runtime/product difference that remains compatible.
- `unknown`: needs deeper extraction before implementation.

## Summary

The current risk is not framework selection. The risk is implementing from an abstract “match upstream” prompt instead of a verifiable upstream behavior contract. Future module work must extract upstream behavior first, then implement OpenReader changes against that contract.

## Module matrix

| Module | reader-dev original behavior/files | OpenReader current behavior/files | Difference/risk | Status | Recommended tests |
|---|---|---|---|---|---|
| Frontend scene structure | `web/src/views/Index.vue`, `web/src/views/Reader.vue`; router has `/` and `/reader`. | `frontend/src/router/index.js` splits `/`, `/search`, `/discover`, `/local-store`, `/sources`, `/settings`, `/books/:id`, `/books/:id/read`. | Current route/page split fragments upstream workspace flows. Old URLs may stay as redirects, but product structure should converge to Index + Reader. | `must-fix` for P1 | Router redirect tests; browser flow search → BookInfo → read. |
| Reader mobile toolbar state | `web/src/views/Reader.vue`: `showToolBar: true`; center tap toggles; panel open branches return without hiding toolbar. | `frontend/src/views/Reader.vue` now uses `mobileChromeVisible = ref(true)`, and `useReaderTools` / `useReaderPanels` no longer hide chrome for panel actions. | Default-visible toolbar and panel coexistence are implemented; remaining work is full upstream interaction audit for all special branches. | `aligned` for base P0; `partial` for full Reader P0 | Keep unit contracts and `scripts/smoke/reader-mobile-contract.mjs`; extend branch coverage as missing upstream cases are extracted. |
| Reader mobile panel structure | Upstream uses Element popovers with `popper-class="popper-component"`; mini interface popper is full-width and coexists with toolbar. | Current mobile shelf/toc/source/settings/search/cache/bookmark panels use `ReaderMobileWorkspacePanel`, a full-width toolbar-coexisting workspace; visible mobile `el-drawer` is no longer used for these panels. | Full-width coexistence and no visible drawer are implemented as a Vue 3/Element Plus adaptation. Exact upstream popover internals still need per-panel layout review. | `technical-stack-equivalent` + `partial` | Browser tests for settings/catalog/source/shelf panel + toolbar visible; per-panel layout review before final P0. |
| Reader mobile content geometry | Upstream mini `.chapter` uses `width: 100vw`, `padding: 0 16px`, `box-sizing: border-box`, `text-align: justify`; slide mode also uses 16px content margins. | Current mobile `.reader-page` uses `width: 100vw`, `padding: 0 16px`, `box-sizing: border-box`, and justified reader body/paragraphs. | Base geometry is implemented; acceptance requires actual rendered paragraph left/right gap checks, not only CSS value checks. | `aligned` for base P0 | DOM geometry probe for page/body/paragraph left/right gaps within 1px across 390×844 and 360×800; ensure toolbar show/hide does not shift content. |
| Reader scrolling vs click paging | Upstream has page/scroll modes with discrete click navigation. | User requested continuous native finger/wheel scrolling while click paging remains segmented. | Intentional UX improvement if it does not change mode selection semantics. | `acceptable-change` | Browser scroll continuity probe; click paging regression tests. |
| Reader settings controls | Upstream uses controls that are easier to distinguish visually; user requested minus/value/plus controls instead of current easy-to-mis-tap slider behavior. | Current setting stepper exists but must be rechecked against upstream layout/state. | Allowed UX adaptation, but values/defaults/state must match upstream. | `acceptable-change` | Unit tests for value bounds; browser setting interaction test. |
| Reader content formats | Upstream `Content.vue` handles text, images/comic-like content, EPUB iframe documents, audio-related branches, and cross-chapter behavior. | Current `ReaderChapterContent.vue` handles text/images/volume blocks, CBZ image resources, EPUB iframe resources, and a dedicated audio branch for `type === 1` chapters; continuous chapter retention, extension, anchor, error, and explicit-jump behavior follows the extracted fixed-baseline contract. | EPUB, image/CBZ rendering/import/resource serving, continuous cross-chapter behavior, remote/local audio playback, and audio resource capabilities are implemented and browser/backend-validated. TTS parity remains pending. | `aligned` for implemented formats; `unknown` for TTS | Keep EPUB/image/CBZ/continuous/audio browser contracts; add TTS fixtures. |
| BookInfo | Upstream has one `web/src/components/BookInfo.vue` used from workspace and reader flows. | Current has shared `BookInfoDialog.vue` / `BookInfoPanel.vue` / `OverlayBookInfo.vue`; the old `/books/:id` URL redirects to the Index workspace and opens the shared dialog. | The independent `BookDetail.vue` route structure has been removed from the product path; search/discover/route actions are centralized; Reader opens plain BookInfo without injecting toolbar shortcut actions. Remaining P1 work is Index-scene placement and search/discover/source flow convergence. | `partial` for P1 | Single BookInfo action contract; search/shelf/reader reuse tests. |
| Bookshelf/BookManage/BookGroup | Upstream: `BookShelf.vue`, `BookManage.vue`, `BookGroup.vue` under Index workspace. | Current: `Home.vue`, overlay management components, categories/store utilities. | Some enhancements may be valid, but workflow and mobile sidebar behavior need upstream comparison. | `unknown` | Workspace browser flows; category/order tests. |
| Mobile Index sidebar | Upstream sidebar width/drag/fixed bottom buttons must be extracted from `Index.vue` and related CSS. | Current `AppLayout.vue` and mobile navigation had reported drag/fixed-button mismatch. | User-visible mismatch: GitHub/day-night buttons should not slide with drawer content. | `must-fix` for P1 | Mobile drag smoke; fixed-bottom button geometry probe. |
| Search/explore/source flow | Upstream Index integrates search/explore/source and BookInfo transitions. | Current has separate `Search.vue`, `Discover.vue`, `Sources.vue` pages. | Flow fragmentation can change API order, panel state, and back behavior. | `must-fix` for P1 | Search → result group → BookInfo → add/read browser test. |
| Online source parsing | Upstream reader3-compatible source semantics live across web components and Java backend. | Current Go parser in `backend/engine/source_*` has tests and compatibility shims. | Must continue fixture-based extraction; do not infer equivalence from passing current tests alone. | `unknown` | HTML fixture/golden tests for search/info/toc/content. |
| Local import catalog parsing | Upstream `BookController.kt` imports local files through `Book.initLocalBook(...)` and `LocalBook.getChapterList(...)`; TXT parsing uses `TextFile.kt` with `DefaultData.txtTocRules`, enabled-rule reverse scoring, Java regex constructs such as `(?<=...)` and `(?!...)`, deterministic local file reads, and `TocEmptyException` for empty catalogs. | Current staged upload token flow is a valid OpenReader enhancement, but Go TXT detection uses a reduced rule set and only partially normalizes Java regex lookbehind; negative lookahead rules can fail to compile or over-match. | Staged tokens remain acceptable; TXT catalog rule compatibility is a user-visible parser bug and must be fixed before claiming local import parity. | `must-fix` for parser slice; staged upload is `acceptable-change` | Golden TXT fixtures for upstream enabled rules, Java regex normalization, negative-lookahead false-positive prevention, deterministic preview/import/reparse without upload; keep EPUB/PDF/UMD/CBZ regression fixtures. |
| Replace rules/content cleanup | Upstream `ReplaceRule.vue` and backend semantics need extraction. | Current Go endpoints and overlays exist. | Rule ordering, scope, and test output may differ. | `unknown` | Golden rule application tests; UI batch action tests. |
| RSS | Upstream `RssSourceList.vue`, `RssArticleList.vue`, `RssArticle.vue`. | Current `RSSManager.vue`, overlays, Go RSS parser. | UI and parser semantics need mapping. | `unknown` | RSS fixture parser tests; source/article browser smoke. |
| WebDAV/local store | Upstream `WebDAV.vue`, `LocalStore.vue` and server storage behavior. | Current Go WebDAV/local-store endpoints and browser component exist. | Path safety and workflow compatibility both need explicit contract. | `unknown` | Path traversal tests; upload/list/import browser smoke; Docker volume smoke. |
| Backup/restore | Upstream backup flows and reader-dev formats require extraction. | Current OpenReader backup service and Legado restore exist. | Must preserve OpenReader data and document reader-dev/Legado import semantics. | `unknown` | Restore testdata; backup list/download/restore tests. |
| Auth/user management | Upstream user management components include `AddUser.vue`, `UserManage.vue`; OpenReader adds JWT. | Current JWT/multi-user/admin endpoints are intentional runtime adaptation. | Auth model differs intentionally, but UI/workspace placement and permission behavior need checks. | `intentional-redesign` + `unknown` | Auth dialog and admin browser smoke; user-scope API tests. |
| Docker/runtime | Upstream ships Java/Gradle/Docker variants. | Current single Go binary + frontend dist in Alpine, env-driven volumes. | Intentional deployment redesign. Must preserve local Docker build and volume compatibility. | `intentional-redesign` | `PUSH=0 ./scripts/docker-build-push.sh`; `scripts/docker-volume-backup-smoke.sh`. |

## Immediate parser contract: TXT local import catalog rules

Status: extracted and implemented for the TXT rule-compatibility slice on 2026-07-07.

Upstream files:

- `/private/tmp/changshengyu-reader-dev/src/main/java/com/htmake/reader/api/controller/BookController.kt`
- `/private/tmp/changshengyu-reader-dev/src/main/java/io/legado/app/model/localBook/LocalBook.kt`
- `/private/tmp/changshengyu-reader-dev/src/main/java/io/legado/app/model/localBook/TextFile.kt`
- `/private/tmp/changshengyu-reader-dev/src/main/resources/defaultData/txtTocRule.json`

| Concern | Upstream behavior | Required OpenReader behavior | Status |
|---|---|---|---|
| Local read source | Uploaded/imported local files are copied into local storage, then parsed from the local file by `LocalBook.getBookInputStream`; catalog extraction is not network-dependent after the file exists locally. | OpenReader keeps staged upload/import-token and local-store import flows, but preview/reparse/import must parse the staged/local bytes deterministically without depending on client upload speed after staging. | `acceptable-change` for staged tokens |
| TXT rule source | `DefaultData.txtTocRules` loads the fixed JSON rule list; only enabled rules participate in automatic detection. | `DefaultTXTTocRules()` should expose the upstream enabled rule set, including rule `-1 目录(去空白)`, not a reduced/simplified subset. | `aligned` for enabled TXT rules in this slice |
| Rule scoring | `TextFile.getTocRule()` iterates enabled rules in reverse order and chooses a rule only when its match count is at least 2; ties update to the later iteration, effectively preserving upstream preference. | Automatic TXT detection must use the same enabled-rule reverse scoring rather than relying only on the old broad `ChapterTitlePattern`. | `aligned` for line-based Go parser adaptation |
| Java regex compatibility | Upstream rules use Java regex lookbehind/lookahead, especially `(?<=...)`, `正文(?!完|结)`, `节(?!课)`, `集(?![合和])`, `部(?![分赛游])`, and `篇(?!张)`. | Go parser must normalize supported upstream Java regex forms without compile failure and must preserve the false-positive prevention semantics for common negative lookaheads. | `aligned` for known enabled-rule constructs |
| Empty catalog | Upstream throws `TocEmptyException` when no chapters are found. | OpenReader keeps its existing preview/import error mapping for no readable chapters; this is an API adaptation, not a parser behavior change. | `technical-stack-equivalent` |
| Non-TXT formats | EPUB/UMD/CBZ delegate to format-specific parsers; PDF is an OpenReader-added format already in the current pipeline. | This slice does not change EPUB/PDF/UMD/CBZ behavior. | `unchanged` |

Validation evidence:

1. `backend/engine.TestDefaultTXTTocRulesIncludeUpstreamEnabledRules` verifies the exposed rule list includes upstream rule `-1` and every enabled rule compiles after normalization.
2. `backend/engine.TestParseTXTWithRuleAcceptsUpstreamNegativeLookahead` verifies upstream Java negative-lookahead rules do not fail compilation and do not split on false-positive text like `第一节课` or `正文完结`.
3. Existing parser/localbook/API tests cover import archiving, custom TXT rules, local-store import preview/import, and chapter refresh.

## Immediate P1 contract: Index mobile sidebar and workspace shell

Status: extracted on 2026-07-07 before implementation.

This contract is tied to `changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`, primarily `web/src/views/Index.vue`.

### Upstream behavior contract

| Concern | Upstream evidence | Required OpenReader behavior |
|---|---|---|
| Scene boundary | The upstream router is effectively `Index.vue` plus `Reader.vue`; `Index.vue` owns the shelf, search, explore, source management, import, local store, WebDAV, user, backup, cache, RSS, replace-rule, and book-info flows. | OpenReader may keep old URLs as compatibility shims, but the visible workspace shell must behave like one Index scene. Search/source/settings page splits must not create different mobile sidebar state or duplicate business flows. |
| Sidebar width | `.navigation-wrapper` has `width: 260px` and `min-width: 260px`. | The visible sidebar width is 260px on desktop and mobile. CSS variables/tests must assert 260px, not infer it from current layout. |
| Mobile collapsed state | `showNavigation` starts `false`; when `collapseMenu` is true the computed `navigationClass` becomes `navigation-hidden`, which sets `margin-left: -260px`. | On mobile/mini workspace the sidebar is hidden by default and exposed by menu button or horizontal drag. Desktop keeps the sidebar visible. |
| Mobile drag boundary | `handleTouchMove()` accepts right-drag opening when `!showNavigation && moveX > 0 && moveX <= 270`, and left-drag closing when `showNavigation && moveX < 0 && moveX >= -270`. During drag it writes `navigationStyle.marginLeft = moveX - 270 + "px"` for opening and `moveX + "px"` for closing. | Drag should follow the finger using the upstream 270px gesture window while final static sidebar width remains 260px. Tests should cover the 270px boundary separately from the 260px CSS width. |
| Gesture start guard | Upstream ignores touches within 20px of any viewport edge and cancels horizontal dragging when vertical movement dominates. | Keep the 20px edge guard and vertical-scroll passthrough. Horizontal drags must call `preventDefault`/`stopPropagation`; vertical scrolls must not. |
| Gesture end transition | Any positive horizontal move opens; any negative horizontal move closes. `navigationStyle` is reset after touch end. | Keep the upstream sign-based open/close decision and reset inline drag style at the end/cancel of the gesture. |
| Shelf click close | `.shelf-wrapper` has `@click="showNavigation = false"`. The menu icon uses `@click.stop="toggleMenu"`. | Clicking/tapping the workspace closes an open mobile sidebar. Sidebar/menu clicks and bottom icon clicks must not pass through and close it. |
| Bottom icons placement | The GitHub and night/day buttons live in `.bottom-icons`, a direct child of `.navigation-wrapper`, not inside `.navigation-inner-wrapper`. CSS: `position: absolute; bottom: 30px; width: 188px; left: 36px; display:flex; justify-content:space-between; pointer-events:none`, while child controls set `pointer-events: all`. | GitHub and day/night controls must be fixed to the sidebar bottom area, not part of the scrollable sidebar content. Scrolling sidebar content must not move them. OpenReader additionally keeps them visually stable during mobile drag because the user explicitly reported that these buttons should not slide with the drawer gesture. |
| Scrollable navigation content | `.navigation-inner-wrapper` has `height: 100%`, `overflow-y: auto`, and bottom padding `66px`; bottom icons are outside this scrollable wrapper. | The navigation content can scroll independently, leaving a bottom clearance so it does not hide behind fixed bottom buttons. |
| Mobile shelf geometry | At `max-width: 750px`, `.shelf-wrapper` uses `padding: 0`, safe-area top padding, title padding `20px 24px 0`, group margins `24px`, and list rows `padding: 10px 20px`. | Mobile shelf should keep upstream-like compact title/group/list spacing while sidebar behavior is being restored. Current Home mobile layout may retain OpenReader-specific book-row details only when spacing and menu visibility remain compatible. |
| Night toggle | Upstream `toogleNight` toggles day/night and changes icon color/image. | OpenReader may map this to the existing theme store, but it must be reachable from the fixed bottom control and not depend on navigating to Settings. |
| GitHub target | Upstream links to the original project; OpenReader links to `changshengyu/openreader`. | This repository target is an intentional OpenReader adaptation. Placement/interaction must still match upstream. |

### Upstream mobile shelf geometry contract

| Concern | Upstream evidence | Required OpenReader behavior |
|---|---|---|
| Mobile breakpoint | `@media screen and (max-width: 750px)` wraps the mini Index layout. | OpenReader mobile shelf geometry should switch at the same 750px breakpoint through `shouldUseMiniInterface`/CSS-compatible rules. |
| Shelf shell | Mobile `.shelf-wrapper` sets `padding: 0`, safe-area top padding, and hides horizontal overflow through the parent `.index-wrapper`. | The mobile shelf page should not keep desktop card padding, border radius, or horizontal overflow. |
| Title row | Mobile `.shelf-title` uses `padding: 20px 24px 0 24px`; base title font remains the upstream 20px shelf-title style. The menu icon appears only when `collapseMenu` is true and uses `@click.stop`. | Mobile Home title should use upstream spacing and a compact title scale. The menu button must remain click-isolated and placed in the title row. |
| Header actions | Upstream title actions are right-floated `.title-btn` text buttons inside the same title row. | OpenReader can keep flex actions for Vue 3 responsiveness, but action text must remain compact and horizontally scrollable if needed; it should not force a 30px title or squeeze the title to one side. |
| Shelf search | Upstream only shows shelf search when edit mode is active, directly after the title and before groups. | Keep current behavior but align horizontal inset to the upstream title/group geometry. |
| Book groups | Mobile `.book-group-wrapper` uses `margin-left: 24px` and `margin-right: 24px`. | Group tabs/chips must respect 24px side insets on mobile instead of spanning flush edge-to-edge. |
| Book list container | Mobile `.books-wrapper .wrapper` switches from desktop grid to vertical flex column. | OpenReader list mode is acceptable, but mobile book rows should be full-width vertical list rows, not desktop cards. |
| Book row spacing | Mobile `.book` sets `box-sizing: border-box`, `width: 100%`, `margin-bottom: 0`, and `padding: 10px 20px`. | OpenReader mobile `.book-row` must use upstream row padding and no card radius/shadow/border chrome. |
| Cover and info | Upstream keeps desktop cover/info dimensions: cover `84px × 112px`, info height `112px`, info left margin `20px`, title 16px/2-line clamp, metadata 12–13px. | OpenReader may use CSS grid, but mobile rows should resolve to the same visible cover width/height and info spacing, not viewport-scaled cover sizes. |

### Current mobile shelf evidence and classification

| Layer | Current evidence | Difference | Classification |
|---|---|---|---|
| Title spacing/scale | `Home.vue` mobile CSS uses `padding: 22px 16px 10px`, and narrower breakpoints override to `18px 14px 0`; title font is 30px/28px. | Too large and too narrow compared with upstream 20px title and 24px side inset. | `must-fix` |
| Group wrapper | Mobile `.book-group-wrapper` has `margin-left: 0`, `margin-right: 0`, and later `padding: 5px 0`. | Groups span edge-to-edge instead of upstream 24px margins. | `must-fix` |
| Book rows | Mobile `.book-row` uses viewport-clamped cover columns, 14/16px or 12/clamped padding, and no fixed 84×112 cover. | Visible row geometry differs from upstream and can drift across 360/390px screens. | `must-fix` |
| Layout model | OpenReader uses CSS grid/list rows and chip buttons instead of Element tabs/desktop `.book` flex. | Acceptable only if visible geometry and operations remain upstream-compatible. | `technical-stack-equivalent` |
| Empty/loading rows | OpenReader adds skeleton/empty states. | Acceptable enhancement; must not alter normal loaded shelf geometry. | `acceptable-change` |

Required implementation gates for this shelf-geometry slice:

1. Change mobile Home CSS to upstream insets and dimensions: title 24px side inset, compact 20px title, group 24px side margins, rows `10px 20px`, covers `84px × 112px`, info margin/gap equivalent to 20px.
2. Add source-level CSS tests for these constants so future refactors do not drift back.
3. Extend the Index mobile browser smoke to verify at 390×844 and 360×800:
   - title left/right insets are approximately 24px;
   - group wrapper side insets are approximately 24px;
   - first book row left/right padding is approximately 20px;
   - cover box is approximately 84×112;
   - no horizontal overflow.
4. Keep the larger Index scene convergence and BookInfo consolidation as separate P1 slices.

### Current OpenReader evidence and classification

| Layer | Current evidence | Difference | Classification |
|---|---|---|---|
| Sidebar frame | `frontend/src/layouts/AppLayout.vue` uses `.app-sidebar` fixed left, width `var(--app-sidebar-width)`, and `.app-sidebar-scroll` for the scrollable content. | Structurally capable of matching upstream. Need assert width source and mobile transitions. | `technical-stack-equivalent` |
| Bottom icons | `sidebar-bottom-icons` is outside `.app-sidebar-scroll`, so scroll does not move it. | This is already aligned with the upstream fixed-bottom structure, but tests should lock it so future edits do not regress. | `aligned` |
| Bottom icon drag behavior | Mobile CSS applies a counter-transform using `--mobile-nav-drag-offset`. | Upstream moves the whole navigation frame during drag, but the user explicitly requested GitHub/day-night controls not to slide with side-panel dragging. Keep this as a documented OpenReader UX difference. | `acceptable-change` |
| Gesture width | `useAppMobileNavigation.js` uses `navigationWidth = 260` for both CSS width and drag clamp. | Upstream drag window is 270px while the sidebar width is 260px. | `must-fix` |
| Drag style | Current opening drag style uses `marginLeft: moveX - width`, producing `-180px` for an 80px drag from hidden state. | Upstream uses `moveX - 270`; after changing only the drag bound, an 80px drag should be `-190px`. | `must-fix` |
| Touch guards | Current composable keeps the 20px edge guard and vertical-dominance passthrough. | Aligned and should be retained. | `aligned` |
| Route/action close | `runNavAction()` and sidebar search navigation close mobile sidebar after every route/action. | Upstream Index does not navigate between separate pages for these workspace panels, but shelf click does close the sidebar. This is part of the larger P1 scene-convergence work; for this slice, do not add new closures beyond the existing workspace click behavior. | `partial` |
| Workspace click close | `.app-workspace @click="closeMobileNavigation"` mirrors upstream shelf click close. | Keep, but make sure sidebar controls/bottom buttons do not pass the click into workspace. | `aligned` |
| Tests | `frontend/tests/appMobileNavigation.test.mjs` currently asserts 260px drag clamp/style. | Tests encode the wrong drag contract and must be updated to upstream 270px gesture semantics while preserving 260px visual width. | `must-fix` |

### Required implementation gates

1. Split the mobile sidebar visual width (260px) from the upstream gesture window (270px).
2. Update `useAppMobileNavigation` drag style and clamp tests:
   - static `navigationStyle` keeps `--mobile-nav-width: 260px`;
   - hidden + 80px right-drag yields `marginLeft: -190px`;
   - hidden + 270px right-drag is accepted;
   - hidden + 271px right-drag is ignored/clamped according to the upstream window;
   - open + 270px left-drag is accepted.
3. Add a source/DOM-level test locking `.sidebar-bottom-icons` outside `.app-sidebar-scroll`, with fixed/absolute positioning and child pointer events.
4. Add or update a real-browser mobile smoke that:
   - opens the sidebar by menu and by drag at 390×844;
   - verifies content scrolling does not move GitHub/day-night buttons relative to the sidebar frame;
   - verifies workspace tap closes the sidebar;
   - verifies bottom icon click does not close the sidebar by propagation.
5. Keep this as an incremental P1 shell-alignment slice. Larger Index convergence remains pending: merging Search/Discover/Sources/Settings into the upstream single workspace scene and consolidating BookInfo.

## Immediate P1 contract: shared BookInfo and old detail URL compatibility

Status: extracted on 2026-07-07 before implementation.

Upstream authority: `web/src/components/BookInfo.vue` used from the single `Index.vue` workspace.

### Upstream behavior contract

| Concern | Upstream evidence | Required OpenReader behavior |
|---|---|---|
| Single BookInfo scene | Upstream has one `BookInfo.vue` dialog with `title="书籍信息"`, bound to `$store.state.showBookInfo`. It is a child of the Index workspace, not an independent route scene. | OpenReader should converge to one shared `BookInfoPanel/Dialog` flow opened from shelf, search, discover, reader, and old URLs. |
| Dialog shell | Upstream uses `el-dialog`, width `dialogSmallWidth`, fullscreen on mini interface, and closes through the dialog close hook. | OpenReader's Vue 3 `BookInfoDialog` fullscreen/mobile behavior is a technical equivalent if every entry point uses it. |
| In-shelf state | Upstream computes `isInShelf` by comparing `bookUrl` against shelf books. In-shelf books show source, latest chapter, follow switch, group name/action, local refresh, cover upload, and intro. | OpenReader may use `id/sourceId/categoryIds`, but the same visible actions must be available from the shared dialog for shelf books. |
| Search/explore result state | When not in shelf, upstream shows a centered `加入书架` operation zone instead of a separate detail page. | Search/discover previews should not route to `/books/:id`; they should use the shared dialog actions to add/read. |
| Old detail route | Upstream does not expose a separate `/books/:id` detail page. | OpenReader may preserve `/books/:id` as an old-link compatibility entry, but it should redirect into the Index workspace and open the shared BookInfo dialog. It must not keep an independent full-page BookDetail product structure as the canonical flow. |
| Reader entry | Reader can open book info while preserving reader toolbar/panel state rules. | Reader book-info action should open the shared dialog/actions; it should not depend on the independent BookDetail page. |
| Allowed additions | Current OpenReader has browser-cache counts, local cache actions, multi-user scopes, and Go API ids. | These are acceptable additions only inside the shared BookInfo/workspace flow. They are not a reason to keep duplicate page-level business logic. |

### Current evidence and classification

| Layer | Current evidence | Difference | Classification |
|---|---|---|---|
| Shared panel | `BookInfoPanel.vue` is shared by `BookInfoDialog.vue` and `OverlayBookInfo.vue`; the old `BookDetail.vue` file has been removed after `/books/:id` became a compatibility redirect. | Good base for convergence. | `aligned` |
| Global overlay | `OverlayBookInfo.vue` owns shared source name, group name, cover upload, local refresh, follow switch, and cache count actions. | This is the closest current equivalent of upstream `BookInfo.vue`. | `aligned` |
| Full detail route | `frontend/src/router/index.js` keeps the `book-detail` route name only as an old-link redirect to `/` with `bookInfo=<id>`. | Aligned with old-link compatibility while removing the independent page structure. | `aligned` |
| Search/discover actions | Search/discover previews no longer create `完整详情` actions that route to `book-detail`. | Aligned with shared BookInfo flow. | `aligned` |
| Reader action | `useReaderPanels.openBookInfo()` no longer adds a `完整详情` route action. | Aligned with shared BookInfo flow. | `aligned` |
| Shelf edit action | `Home.vue.goEditBook()` opens the shared edit dialog. | Aligned with workspace dialog responsibility. | `aligned` |
| Action ownership | Upstream `BookInfo.vue` owns only BookInfo-native actions: add to shelf for non-shelf books, cover upload, local-book update, follow/update switch, and group setting. Current OpenReader centralizes allowed search/discover and old-link compatibility actions in `bookInfoOverlayActions.js`; Reader no longer injects toolbar shortcut actions. | Contextual action policy is now centralized. Broader Index-scene placement remains pending. | `aligned` for this slice |
| Search/discover existing-book preview | Upstream uses the same BookInfo dialog fed through the Index workspace event bus; it does not route to a separate detail scene. Current Search/Discover correctly open the shared overlay and use shared action policy for `查看详情`/`继续阅读`/add actions. | Old-link and “read after add” compatibility are generated by shared action policy rather than local label construction. | `aligned` for this slice |
| Route compatibility action | Upstream has no `/books/:id`; OpenReader keeps it only as old-link compatibility. `AppLayout.vue` uses the shared read-action builder while opening the shared overlay. | Acceptable compatibility action is centralized. | `aligned` |
| Reader BookInfo actions | Upstream Reader opens the same `BookInfo.vue` for the current reading book by emitting `showBookInfoDialog` with the merged shelf book; reader toolbar buttons such as catalog/bookmarks/settings remain separate controls. | OpenReader Reader BookInfo must open the shared overlay with the current book/progress/status only. It must not inject catalog/bookmark/search/source/cache/settings shortcut buttons into BookInfo. | `must-fix` |

Required implementation gates for this slice:

1. Change `/books/:id` to redirect to the Index workspace with `?bookInfo=<id>` while preserving login guard behavior.
2. Add AppLayout-level old-link handler that loads `/api/books/:id`, merges shelf/progress context when available, and opens `overlay.openBookInfo()` with shared actions such as continue reading.
3. Remove `book-detail` navigation from shelf/search/discover/reader BookInfo actions; use shared overlay/edit/read flows instead.
4. Add unit/source tests for route redirect and action removal.
5. Add or extend smoke coverage proving `/books/1` lands on `/` with BookInfo dialog visible, without rendering the full `BookDetail.vue` page.

### 2026-07-07 implementation note

- `/books/:id` now redirects to `/` with `?bookInfo=<id>`.
- `AppLayout.vue` hydrates that query by loading the book and opening the shared `OverlayBookInfo` dialog.
- `BookDetail.vue` has been removed from the frontend source tree.
- Search, Discover, Reader, and mobile shelf edit no longer route users into the removed independent detail page.
- `frontend/tests/bookInfoRouteContract.test.mjs` and `scripts/smoke/index-mobile-sidebar-contract.mjs` lock the compatibility redirect and shared-dialog behavior.

### Next BookInfo action-convergence gate

Authoritative upstream files:

- `web/src/components/BookInfo.vue`: visible BookInfo dialog and native actions.
- `web/src/App.vue`: single `BookInfo` instance, opened by `showBookInfoDialog` event.
- `web/src/views/Index.vue`, `web/src/views/Reader.vue`, `web/src/components/BookManage.vue`: entry points emit the same BookInfo dialog rather than navigating to independent detail pages.

Current files to converge:

- `frontend/src/components/overlays/OverlayBookInfo.vue`: single visible overlay host.
- `frontend/src/views/Search.vue` and `frontend/src/views/Discover.vue`: search/explore preview and add-to-shelf actions.
- `frontend/src/layouts/AppLayout.vue`: `/books/:id` old-link compatibility action.
- `frontend/src/composables/useReaderPanels.js`: reader BookInfo contextual action list.

Required implementation before claiming P1 BookInfo parity:

1. Introduce one shared BookInfo overlay action-policy module/composable.
2. Move route/search/discover/reader contextual actions through that module.
3. Preserve allowed OpenReader compatibility actions only with explicit policy names:
   - old-link `/books/:id` may expose `继续阅读`;
   - search/discover may expose `加入并阅读` as an OpenReader convenience only if the normal `加入书架` path remains identical;
   - reader BookInfo actions must not close or hide reader tool layers unless the invoked target panel itself requires it.
4. Add source tests proving the action labels and handlers are not hand-written in individual pages.
5. Re-run the `/books/:id` real-browser smoke after implementation.

### 2026-07-07 action-policy implementation note

- `frontend/src/utils/bookInfoOverlayActions.js` centralizes contextual search/discover and old-link BookInfo action labels.
- Search/Discover no longer hand-write `查看详情` / `继续阅读` / `加入书架` / `加入并阅读` / `开始阅读` labels.
- `/books/:id` old-link compatibility uses the shared read-action builder.
- Reader-specific toolbar shortcuts are not upstream BookInfo actions and have been removed from `useReaderPanels.openBookInfo()`; Reader now opens a plain shared BookInfo overlay like upstream.

## Immediate P0 contract: Reader mobile

| Behavior | Upstream evidence | Current evidence | Required action |
|---|---|---|---|
| Tool layer default | `Reader.vue` data contains `showToolBar: true`. | `Reader.vue` contains `mobileChromeVisible = ref(false)`. | Set mobile toolbar default to visible and test it. |
| Panel open | Upstream click handler returns when popovers/settings are visible; opening a panel does not hide `showToolBar`. | `useReaderTools.openMobileTool` and `useReaderPanels` set `mobileChromeVisible.value = false`. | Remove panel/action coupling to toolbar visibility. |
| Mobile panel container | Upstream mobile popper is full-width via `.popper-component`. | Current mobile uses bottom drawers for shelf/toc/source/settings/search/cache/bookmark. | Rebuild mobile panels as toolbar-coexisting popovers/workspaces. |
| Horizontal layout | Upstream mini chapter padding is 16px and justified. | Current mobile content padding is 22px with altered vertical padding. | Restore upstream 16px geometry and paragraph semantics. |
| Tests | Upstream behavior is source contract. | Existing tests assert toolbar hides after panel actions. | Delete/rewrite conflicting tests. |

### 2026-07-06 implementation note

Implemented in commit work following this contract:

- Mobile reader tool layer now defaults to visible and initial chapter loading no longer hides it.
- Opening reader tools/panels no longer forces the mobile tool layer closed.
- Mobile shelf, catalog, bookmark, search, source, cache, and settings panels now use a toolbar-coexisting full-width workspace instead of visible bottom `el-drawer`.
- Settings panel coexistence, no visible mobile drawer, center tap behavior, and 16px mobile reader geometry are covered by `scripts/smoke/reader-mobile-contract.mjs`.
- Ordinary text rendering now preserves safe upstream inline HTML semantics via sanitized `v-html`, while keeping plain text for search/progress.
- Image markup now marks chapters with upstream comic semantics, and CBZ-like book URLs hide the chapter title to match upstream `Content.vue`.
- Unit tests that previously encoded “panel open hides toolbar” were rewritten.
- EPUB document reading now uses a dedicated iframe/resource branch that preserves XHTML, relative CSS/images/fonts, hash links, cross-document links, and upstream bridge events.
- EPUB resources are served through scoped, expiring capabilities instead of exposing the login JWT to iframe/resource requests.
- The EPUB browser contract is covered by `scripts/smoke/reader-epub-contract.mjs` at 1440×900, 390×844, and 360×800.

Still pending in Reader P0:

- Complete separate `Content.vue` parity review for TTS/read-aloud controls.
- The final Reader P0 acceptance image remains pending. Intermediate validation image `ca43409` has been published and is not the final Reader P0 release.

## Immediate P0 contract: continuous cross-chapter reading

Status: implemented and validated on 2026-07-06.

This contract is tied to `changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`. It replaces earlier audit notes and tests that treated a fixed previous-1/next-2 window as upstream behavior.

### Upstream state and transition contract

| Concern | Fixed upstream evidence | Required behavior |
|---|---|---|
| Mode eligibility | `Reader.vue.isScrollRead` is true only for `上下滚动` and `上下滚动2`, and false for EPUB, audio, and slide/page branches. | Only `scroll` and `scroll2` render a multi-chapter text/image list. EPUB/audio/page/flip keep their dedicated branches. |
| Initial state | `data()` initializes `scrollStartChapterIndex = 0`, `showNextChapterSize = 1`, and `showPrevChapterSize = 0`. Entering a scroll mode sets `scrollStartChapterIndex = chapterIndex`. | The first continuous window starts at the current chapter and contains at most the current chapter plus one following chapter. It must not eagerly prepend a previous chapter or load two following chapters. |
| `上下滚动` retention | `computeShowChapterList()` starts from `scrollStartChapterIndex`; that value remains the explicitly entered/jumped chapter while natural scrolling updates `readingBook.index`. | Ordinary `scroll` accumulates already-rendered chapters from the explicit start chapter while appending forward. Natural scrolling must not silently discard earlier chapters. |
| `上下滚动2` retention | `computeShowChapterList()` overrides its start with `chapterIndex - showPrevChapterSize`; the fixed baseline keeps `showPrevChapterSize = 0`. The settings text says this mode automatically hides read chapters. | `scroll2` starts at the current visible chapter after chapter progress advances, so already-read chapters are removed. It does not retain a synthetic previous chapter. |
| Reverse extension | The top-of-document previous-chapter preload block in `scrollHandler()` is commented out in the fixed baseline. Previous chapter navigation remains available through explicit previous-page/chapter actions, which set a new `scrollStartChapterIndex` and rebuild at the top. | Natural upward scrolling at the top must not prepend a previous chapter. Remove the current `scroll2` top extension and its height compensation. Explicit previous navigation still loads the previous chapter. |
| Forward extension threshold | `scrollHandler()` starts loading when `scrollTop > scrollHeight - 4 * windowHeight`, described as reaching the fourth-to-last page. | Begin extending when the viewport is within approximately four viewport heights of the document bottom. The current later threshold (`scrollTop + viewport > scrollHeight - 2 * viewport`) is not equivalent. |
| Forward extension size | The next index is the last displayed index plus one. `showNextChapterSize` is recalculated as `nextIndex - chapterIndex`, then exactly that adjacent chapter is loaded before recomputing the list. | Add one adjacent chapter per extension transaction, without duplicate requests. In `scroll2`, recomputation may simultaneously remove chapters before the current visible chapter. |
| Extension lock | `preCaching` guards the transaction and progress saving; it is cleared on both success and failure. | Only one boundary extension runs at a time. The lock must release after success or failure, and progress must not be persisted from a transient replacement state. |
| Chapter switch by scrolling | `saveReadingPosition()` chooses the current visible `h3/p`, finds its `.chapter-content[data-index]`, updates `readingBook.index/title`, and persists the chapter index. | Visible-chapter detection updates chapter identity, title, local progress, and server progress from one consistent snapshot. |
| Scroll anchor | Before replacing a changed list, `computeShowChapterList()` captures the current paragraph's chapter index, `data-pos`, and viewport offset, then restores that offset after DOM replacement. | Removing read chapters or appending content must not move the paragraph being read. Clamp the restored scroll position to the actual scroll range. |
| Explicit chapter navigation | `toNextChapter()`/`toLastChapter()` set `scrollStartChapterIndex` to the target and call `computeShowChapterList(true)`; reset mode scrolls to the top. Search/bookmark jumps similarly set the start chapter, rebuild, then locate the target paragraph. | Catalog, search, bookmark, previous, and next actions rebuild around the requested chapter and position. They are distinct from natural boundary extension. |
| Adjacent load failure | `loadShowChapter()` stores a chapter block containing `获取章节内容失败！` and marks it as an error; a later load may replace an error cache entry. | A failed adjacent chapter must not be silently dropped into a blank gap or leave endless invisible retry churn. Render an in-list error block with a retry path, while retaining the currently readable chapter. |
| Book bounds | A missing catalog index rejects as `章节不存在`; extension catches the failure and releases `preCaching`. Explicit next/previous actions show first/last chapter messages. | Never request outside `[0, catalog.length - 1]`. Reaching the book end remains stable and does not keep scheduling extension attempts. |

### Route, persistence, and API mapping

| Layer | Upstream contract | OpenReader adaptation | Classification |
|---|---|---|---|
| Route/jump inputs | Upstream reader events carry `chapterIndex` plus a paragraph/bookmark position. | Keep `/books/:id/read?chapter=&offset=&percent=` and `resume=1`; these are old-link and Go/Vue adaptations. A route jump must enter the same explicit-rebuild transition above. | `acceptable-change` |
| Local position | Upstream stores `bookChapterProgress@{name}_{author}` and the current user's `@readingRecent`; the paragraph offset is relative to the visible chapter. | Keep user-scoped `openreader_chapter_progress@{user}@{bookId}` with chapter index/id, text offset, chapter percent, mode, and timestamp. The visible chapter snapshot must remain internally consistent. | `acceptable-change` |
| Server progress | Upstream posts `/saveBookProgress` with book URL and chapter index while local cache keeps the paragraph offset. | Keep authenticated `PUT /progress` with chapter id/index, offset, chapter/full-book percent, mode, and conflict metadata. This is the Go multi-user data model, not a reason to change the visible transition order. | `intentional-redesign` |
| Chapter content | Upstream calls `getBookContent(chapterIndex, ..., cache=true)` and caches up to three books in memory. | Keep current chapter API and scoped browser memory cache/preload implementation, but boundary extension must expose failures and follow the upstream window policy. | `acceptable-change` |

### OpenReader implementation mapping

| Previous incorrect evidence | Implemented replacement | Classification | Contract test |
|---|---|---|---|
| Initial `scroll2` window was previous 1/current/next 2. | `readerChapterWindowIndexes()` now begins at the current/explicit start chapter and initially shows only current plus one following chapter. | `aligned` | Book-start, middle, and book-end window tests. |
| `scroll2` retained a synthetic previous chapter and pruned on every scroll event. | Visible chapter identity updates without DOM replacement; the controlled forward-extension transaction removes every read chapter only in `scroll2`, while `scroll` retains its explicit start. | `aligned` | Mode-specific retention and anchored transaction tests. |
| Top proximity automatically prepended the previous chapter. | Natural top scrolling no longer extends backward. Explicit previous/catalog/search/bookmark navigation rebuilds the selected chapter and requested position. | `aligned` | Extension-zone and loaded explicit-navigation tests. |
| Forward extension began within three viewport heights. | `readerChapterWindowExtension()` now uses the fixed upstream `scrollTop > scrollHeight - 4 * clientHeight` boundary. | `aligned` | Exact threshold tests immediately below/above the boundary. |
| Adjacent failures were silently dropped. | A failed adjacent chapter renders an in-list error block with stable chapter position and retry; current content remains readable and the extension lock always releases. | `aligned` plus explicit retry affordance | Failure/retry and serialized-extension tests plus browser failure fixture. |
| Preload and window construction could issue duplicate requests for the same chapter. | `useReaderChapterContent()` deduplicates in-flight requests by book, chapter, and refresh intent. | `acceptable-change` | Concurrent load test and browser request counters. |
| Mode changes held the already-readable current chapter behind a global loading state until adjacent requests finished. | The cached current block renders immediately; only missing adjacent blocks load, and adjacent latency/failure cannot blank the current chapter. | `aligned` | Mode controller and delayed/failing adjacent chapter browser tests. |

### Required implementation gates

1. Replace existing chapter-window tests that assert previous-1/next-2 retention or natural reverse extension.
2. Add state tests for initial load, forward extension, visible chapter advance, `scroll` retention, `scroll2` read-chapter removal, explicit previous navigation, book bounds, and extension lock release.
3. Add a delayed/failing adjacent chapter fixture proving the current chapter remains readable and the error is visible/retryable.
4. Add a real-browser long-book smoke at 1440×900, 390×844, and 360×800. Verify native finger/wheel continuity, no horizontal shift, no anchor jump after `scroll2` cleanup, and no duplicate chapter requests.
5. Preserve the user-requested difference: native touch/wheel scrolling remains continuous, while click zones and keyboard paging keep discrete viewport-sized movement.

Validation evidence:

- Backend `go test ./...`: passed.
- Frontend `npm test`: 261 passed.
- Frontend `npm run build`: passed.
- `scripts/smoke/reader-continuous-contract.mjs`: passed at 1440×900, 390×844, and 360×800, plus a 390×844 adjacent-failure/retry pass.
- The browser contract verifies initial `[current, next]` rendering, native 137px wheel movement, symmetric readable geometry, `scroll2` read-chapter removal, paragraph anchor drift within 2px, no duplicate adjacent requests, current-content survival on failure, and successful retry.
- Existing `scripts/smoke/reader-mobile-contract.mjs` and `scripts/smoke/reader-image-contract.mjs` both passed after this change.

## Immediate P0 contract: Content image/comic/CBZ reading

Status: implemented and validated on 2026-07-06.

### Upstream behavior contract

| Layer | Upstream evidence | Required visible/state behavior |
|---|---|---|
| Image detection | `web/src/components/Content.vue` treats any content line containing `<img` as cartoon/comic content. | OpenReader must detect image markup before stripping/sanitizing text, and mixed text+image lines must preserve their text positions around the image. |
| Image source rewriting | Upstream replaces `src=` with `data-src=` and replaces `__API_ROOT__` with `apiRoot`, then relies on `v-lazy-container`. | OpenReader may use Vue/Element lazy images instead of `v-lazy-container`, but must resolve `src`, `data-src`, `data-original`, `data-url`, and `__API_ROOT__` to the same effective image URL. |
| Comic image layout | Upstream global `.content-body img` forces images to `width: 100%`, `max-width: 100vw`, and `display: block`. | Comic/CBZ images should visually fill the readable content width instead of being capped by a generic prose-image max width. Text illustrations may keep caption/preview behavior only when it does not shrink comic pages. |
| CBZ title handling | Upstream `isCbz` is `readingBook.bookUrl.toLowerCase().endsWith(".cbz")`; CBZ chapters render no chapter title. | OpenReader must treat `.cbz` source/original/local URLs as CBZ and hide the chapter heading for CBZ chapters, including query/hash suffixes. |
| Continuous chapter list | Upstream `renderScrollChapterList()` applies the same image/CBZ rules to every visible chapter in scroll modes. | OpenReader must apply image, comic, and CBZ layout consistently in single chapter, scroll, and scroll2 multi-chapter windows. |
| Image-load pagination | Upstream Reader subscribes to `$Lazyload.$on("loaded", lazyloadHandler)` and `lazyloadHandler()` calls `computePages()` unless audio is active. | When an image finishes loading in OpenReader, flip/page pagination and chapter progress must be recomputed. A late-loading image must not leave stale page counts or wrong scroll restoration. |
| Position/search semantics | Upstream assigns `data-pos` based on the original line offset before converting image tags to lazy markup. Search/TTS paragraph traversal still mostly targets `h3,p`, while image wrappers carry `data-pos`. | OpenReader should preserve source offsets for text/image blocks. Searchable text must not include raw image markup, and image-only CBZ pages must still expose block positions for progress/bookmark calculations. |
| Error/unsafe HTML handling | Upstream directly injects HTML for image lines. | OpenReader must keep its stricter HTML sanitizer and URL allowlist for security; this is an allowed security hardening if the visible safe image behavior remains aligned. |

### Current OpenReader mapping

| Concern | Current evidence | Classification | Required action |
|---|---|---|---|
| Image parsing | `frontend/src/utils/readerContent.js` parses `<img>` into `type: "image"` blocks and reads `src`, `data-src`, `data-original`, `data-url`, including a non-browser parser path for deterministic tests. | `aligned` | Covered by parser contract tests for `__API_ROOT__`, mixed text+image lines, unsafe image URLs, and source positions. |
| CBZ detection | `useReaderChapterPresentation.js` checks `url`, `bookUrl`, `libraryPath`, and `originalFile`, ignoring query/hash. | `acceptable-change` | Keep broader detection because Go/Vue data shape differs from upstream `bookUrl`; document as compatibility adaptation. |
| Image layout | `ReaderChapterContent.vue` keeps the generic illustration cap but overrides comic/CBZ image boxes and image elements to fill the readable column width. | `aligned` | Browser geometry checks cover desktop page mode, mobile continuous scroll, and mobile flip mode. |
| CBZ heading | `hideTitle` is set for CBZ and `h1` is skipped. | `aligned` | Tests cover query/hash `.CBZ`, `originalFile`, and `libraryPath` shapes. |
| Image-load relayout | `ReaderChapterContent.vue` emits `image-load`; Reader calls `updateFlipLayout()` and refreshes progress state after every successful image load. | `aligned` | Static wiring test plus delayed-image browser checks cover page/flip recalculation. |
| Preview | Element Plus image preview is used for ordinary image blocks; the image block stops click propagation before the Reader center-tap handler. | `acceptable-change` | Browser checks prove preview opens without hiding the default-visible mobile toolbar. |
| Security | Current parser strips unsafe inline HTML and rejects non-http(s) image URLs. | `acceptable-change` | Keep the stricter allowlist; tests should prove `javascript:` and script attributes do not survive. |

### Validation evidence

Frontend unit tests:

1. `parseReaderContentBlocks()` resolves `__API_ROOT__`, `src`, `data-src`, `data-original`, and `data-url`, preserving source `pos/endPos`.
2. Mixed text+image+text lines produce text blocks around an image block with stable offsets.
3. Unsafe image URLs and unsafe inline HTML are rejected while safe inline text remains searchable.
4. CBZ detection hides titles for `.cbz`, `.CBZ?x#y`, `originalFile`, and `libraryPath` shapes.
5. Image load events from the content component bubble to Reader layout recomputation without firing for non-image text blocks.

Real-browser gate:

1. Open a fixture reader chapter at 1440×900, 390×844, and 360×800 with delayed-loading images.
2. Confirm comic/CBZ images fill the readable width, CBZ titles are hidden, and ordinary text remains justified with symmetric mobile padding.
3. Confirm image load changes page count/layout rather than leaving stale pagination.
4. Confirm image preview does not hide the mobile toolbar or pass through as a center tap.
5. Confirm actual rendered mobile page/body/paragraph left/right gaps remain symmetric within 1px and do not shift when toolbar visibility changes.

Implemented coverage:

- `frontend/tests/readerContent.test.mjs`
- `frontend/tests/readerChapterPresentation.test.mjs`
- `frontend/tests/readerImageWiring.test.mjs`
- `scripts/smoke/reader-image-contract.mjs`

The browser contract covers 1440×900 desktop page mode, 390×844 and 360×800 mobile continuous scroll, plus a separate 390×844 mobile flip-pagination pass.

Repository gate:

- Backend `go test ./...`: passed.
- Frontend `npm test`: 257 passed.
- Frontend `npm run build`: passed.
- Existing `scripts/smoke/reader-mobile-contract.mjs`: passed after the image click-through fix.

## Immediate P0 contract: EPUB document reading

### Upstream behavior contract

| Layer | Upstream evidence | Required visible/state behavior |
|---|---|---|
| Detection | `Content.vue` and `Reader.vue` identify a local EPUB from the `.epub` book URL. | OpenReader may use its current `originalFile`/`libraryPath` model, but only local EPUB books enter this branch. |
| Chapter API | `BookController.getBookContent` extracts the EPUB and returns the XHTML chapter URL instead of flattened text. | Loading an EPUB chapter must preserve its XHTML document, CSS, images, fonts, anchors, and relative resource paths. A missing archive/chapter remains an explicit load error, never a blank reader. |
| Resource serving | `YueduApi.kt` exposes extracted EPUB files under `/epub/*`; HTML responses receive the reader bridge from `BookConfig.epubInjectJavascript`. | The iframe receives a same-origin XHTML resource URL whose relative assets resolve without separate frontend rewriting. |
| Rendering | `Content.vue.renderEpub` renders the chapter in `.epub-iframe`. | EPUB uses a dedicated iframe branch; it must not pass through the ordinary paragraph/image block renderer. |
| Bridge lifecycle | Child emits `inited`, `load`, and `setHeight`; parent sends `setStyle` and `execute`. | On initialization, apply current reader typography and request height. On load, recompute pages and restore position. Height is at least 80% of the viewport and content changes trigger page recalculation. |
| Reader style | Parent injects font family, font size, weight, color, line height, paragraph spacing, hidden scrollbars, and responsive image rules. | Theme and typography changes update the already-open EPUB without reimporting or flattening it. Images remain block-level, within viewport width, and preserve aspect ratio. |
| Input forwarding | Child forwards ordinary clicks and keydown; image clicks emit the full image list/index; in-document hash clicks emit the target rectangle. | Center click follows the Reader toolbar state machine, keyboard navigation remains available, image preview opens at the selected image, and hash links scroll to the target without losing reader chrome state. |
| Linked chapter navigation | Child `load` includes its URL; `Reader.vue.epubLocationChangeHandler` maps a navigated XHTML path back to the catalog index. | Internal links to another spine document update the active chapter/index/title and progress state instead of leaving Reader state stale. |
| Position | `Reader.vue.showPosition` restores document scroll immediately and again after `iframeLoad`; EPUB progress is document `scrollTop`. | Returning to a chapter restores the same vertical position after the iframe has its final height. |
| Content transforms | `Reader.vue.filterContent` and `formatChinese` return EPUB content unchanged. | Do not apply text replacement or simplified/traditional conversion to the XHTML document during rendering. Plain-text search/index data may continue to use the parsed text copy. |
| Parent touch handling | Parent click/touch handlers return early for EPUB because the bridge owns iframe input. | Iframe events must not double-trigger parent touch/click handlers or penetrate open panels. |

### OpenReader API and security adaptation

The Java upstream serves extracted files directly from its data tree. OpenReader adds JWT multi-user isolation, so copying that route literally would expose another user's book or make the iframe return `401` because an iframe navigation cannot attach the Axios Bearer header.

The compatible Go/Vue contract is:

| Contract | Required OpenReader behavior | Classification |
|---|---|---|
| Existing chapter endpoint | `GET /api/books/:id/chapters/:index/content` remains Bearer-authenticated and keeps the existing `chapter` and plain-text `content` fields. It adds `format` (`text` or `epub`) and, for EPUB only, `resourceUrl` plus its expiry metadata. Existing text clients continue to work. | `acceptable-change` API adaptation |
| Iframe resource URL | `resourceUrl` uses a signed, opaque path capability scoped to one user, one book, one extracted archive version, read-only access, and a bounded expiry. It must not contain the login JWT. | `acceptable-change` security hardening |
| Resource route | A route outside Bearer middleware validates the capability before every XHTML/CSS/image/font request. Invalid, expired, wrong-book, wrong-version, or malformed capabilities return an error and never fall back to another path. | `must-fix` for multi-user isolation |
| Relative resources | The capability is a path segment shared by the chapter and its resource root, so ordinary relative EPUB URLs resolve under the same authorization scope. | `must-fix` |
| XHTML handling | XHTML is served dynamically with the OpenReader bridge and restrictive response headers. Original archived files remain unmodified. EPUB-authored executable scripts and remote network loads are disabled; internal document links, CSS, images, and fonts remain usable. | `acceptable-change` security hardening |
| Errors | Archive missing/corrupt, unsafe path, extraction limit, missing resource, and capability failure have distinguishable HTTP status/error responses. The frontend shows the chapter load error and retry action instead of an empty iframe. | `must-fix` |

The concrete additive response shape is:

```json
{
  "chapter": {},
  "content": "plain-text fallback used by existing search/cache flows",
  "format": "epub",
  "resourceUrl": "/api/epub-resource/<signed-capability>/<chapter-path>",
  "resourceExpiresAt": "RFC3339 timestamp"
}
```

For non-EPUB chapters, `format` is `text`, and `resourceUrl`/`resourceExpiresAt` are omitted.

### Data and parser contract

| Concern | Current OpenReader evidence | Required action |
|---|---|---|
| Persistent source | Import already archives `OriginalFile` under `library/<LibraryPath>` and stores `LibraryPath`, `TOCFile`, and chapter rows. | Preserve all existing rows/files; no destructive migration and no re-upload requirement. |
| Current flattening | `ParseEPUBWithRule` reads spine XHTML and stores only extracted text in per-chapter caches. | Keep the text copy for search/fallback, but retain each chapter's canonical EPUB resource path in the imported chapter contract so the original document can be served. Existing imports must recover this mapping from the archived EPUB when first opened. |
| Derived extraction | No extracted EPUB resource tree currently exists. | Extract to a derived versioned directory under the book's existing library directory. It is disposable/rebuildable and must be included in volume-path safety checks, not treated as a new persistent database source of truth. |
| Archive version | The source EPUB may be replaced while its library path stays stable. | Bind extracted data and resource capabilities to a deterministic source fingerprint; stale extraction/capabilities cannot read the replacement archive. |
| Archive safety | Existing parser reads selected ZIP entries but does not provide a full resource-serving extraction boundary. | Reject absolute paths, `..` traversal, drive prefixes, NUL names, symlinks, duplicate/conflicting paths, excessive entry counts, oversized entries, and excessive total uncompressed size. Never write outside the derived extraction root. |
| Media handling | Current reader has no EPUB resource MIME contract. | Serve XHTML/HTML, CSS, common images, SVG, and fonts with correct MIME types, `nosniff`, no-referrer, and a restrictive CSP. Unknown executable/media types are denied rather than served as active content. |

### EPUB implementation validation evidence

Backend/API tests:

1. `backend/api.TestDirectEPUBImportAndRefreshUseTocRule` imports a fixture EPUB containing XHTML, relative CSS, image, same-document hash, cross-chapter link, and active-script attempts; it verifies searchable text plus canonical `resourcePath`.
2. The same API test verifies existing imported EPUB rows without `resourcePath` recover metadata from `OriginalFile` without schema/data loss.
3. The chapter endpoint returns the additive EPUB response while ordinary text responses remain backward-compatible.
4. A valid capability serves chapter XHTML and relative resources with MIME/security headers and rebuilds missing derived extraction data.
5. Modified capability, stale archive version, ownership-changed book, missing resource, and unsupported active content are rejected.
6. `backend/services/epubreader` tests reject ZIP-slip, symlink, duplicate path, entry-count, per-entry-size, total-expanded-size, and symlink-ancestor extraction paths.
7. `backend/db.TestAutoMigrateAddsEPUBResourcePathWithoutLosingChapters` verifies additive migration of old chapter rows.

Frontend unit/contract tests:

1. `frontend/tests/readerChapterLoader.test.mjs` verifies `format: epub` renders through the dedicated iframe branch and never ordinary paragraph blocks.
2. `frontend/tests/readerEpubFrame.test.mjs` verifies bridge origin/source validation and `inited`, `load`, `setHeight`, `click`, `clickHash`, `keydown`, and `previewImageList` forwarding.
3. `frontend/tests/readerMode.test.mjs` verifies EPUB forces the upstream vertical-page branch even when the stored reader mode is flip/scroll.
4. `frontend/tests/readerViewportProgress.test.mjs` verifies EPUB progress is stored as document scroll pixels.

Real-browser gate:

1. `scripts/smoke/reader-epub-contract.mjs` opens the fixture at 1440×900, 390×844, and 360×800 against a real local server.
2. It confirms XHTML typography, upstream reader style override, preserved non-typography CSS, relative CSS/image/font loads, image preview, internal hash, cross-chapter link, keyboard forwarding, center-click toolbar behavior, panel coexistence, and saved-position restoration.
3. It confirms no resource `401`, no blank page, no authored script execution, no ordinary paragraph blocks, and no duplicate center-click toggle.

Deferred from this EPUB slice:

- TTS/read-aloud and auto-reading behavior: upstream hides some reader controls for EPUB while `Content.vue` contains generic autoplay methods. This requires a separate evidence pass before claiming parity.
- Search result navigation into exact text inside iframe content.
- Remaining CBZ archive/import and lazy-loading edge cases.

## Immediate P0 contract: CBZ/comic image and audio chapter reading

### Upstream evidence

| Feature | Upstream authority | Contract |
|---|---|---|
| CBZ detection | `web/src/components/Content.vue.isCbz`, `web/src/views/Reader.vue.isCbz` | A book whose `bookUrl` ends with `.cbz` enters the comic/CBZ branch. CBZ chapters hide the chapter title and render the chapter content as image content rather than normal prose. |
| CBZ import/catalog | `src/main/java/io/legado/app/model/localBook/CbzFile.kt`, `BookController.extractCbz`, `BookController.getLocalChapterList` | CBZ files are accepted as local books. The archive is extracted to a derived `index` directory; non-directory, non-XML entries are sorted lexicographically and become one chapter per image/file path. `ComicInfo.xml` can update title/author and the first image can become cover. |
| CBZ chapter content | `BookController.getBookContent` CBZ branch | A CBZ chapter resolves to the extracted file. Image extensions `jpg/jpeg/gif/png/bmp/webp/svg` return `<img src='__API_ROOT__...'>`; non-image files return the raw file URL. Missing extraction or missing chapter file returns an explicit error. |
| Comic image rendering | `Content.vue.render` and `renderScrollChapterList` | Any content containing `<img` enters a comic/image rendering path. Image `src` is rewritten to lazy `data-src`; `__API_ROOT__` is expanded; image click opens preview. `isCarToon` disables auto-reading/TTS controls. |
| Audio detection | `Content.vue.isAudio`, `Reader.vue.isAudio` | A book with `readingBook.type === 1` enters an audio branch. Audio disables scroll/slide page interactions, keyboard paging, progress slider display in mini interface, auto-reading, and speech/TTS controls. |
| Audio player | `Content.vue.renderAudio` | Audio content is rendered by an `<audio preload>` element whose `src` is the chapter content URL. The UI includes cover, elapsed/total time, seek slider, -15s/+15s, previous/next chapter, play/pause, volume mute and volume slider. |
| Audio state transitions | `Content.vue.play/computeDuration/onTimeupdate/onEnd/prevChapter/nextChapter` | Initial mount calls `play(true)` and computes duration. If `autoPlay` is true the element starts playback. `timeupdate` emits progress update; `ended` resets current time/duration, enables autoplay, and advances to the next chapter. Previous/next chapter buttons set autoplay and emit chapter navigation. |
| Reader input guard | `Reader.vue.handleTouchStart/eventHandler/keydownHandler` | Parent touch and keyboard handlers return early for audio. Center tap only toggles the toolbar when read bar is not shown; it must not page text. |

### Current OpenReader evidence and classification

| Layer | Current OpenReader evidence | Difference | Classification |
|---|---|---|---|
| Local import allow-list | `backend/api/imports.go`, `backend/api/localstore.go`, and local import services accept `.cbz` in the same local-book pipeline. | OpenReader keeps the single Go import path rather than upstream Java extraction classes. | `technical-stack-equivalent` |
| CBZ parsing/extraction | `backend/services/cbzreader` and local-book import tests preserve the archive, derive sorted image chapters, and protect archive/resource paths. | Uses scoped capability URLs instead of exposing library paths. | `acceptable-change` security hardening |
| CBZ content response | `backend/api/books.go.chapterContent` returns `format: "cbz"`, `resourceUrl`, `resourceExpiresAt`, and `<img src="...">` content for CBZ chapters. | Existing JSON envelope is preserved; resource serving is capability-protected. | `aligned` |
| Image rendering | `frontend/src/components/reader/ReaderChapterContent.vue`, `useReaderChapterPresentation.js`, `parseReaderContentBlocks` convert `<img>` to image blocks, hide CBZ titles, collect preview image lists, and recompute layout on image load. | Vue 3/Element Plus `el-image lazy` replaces upstream `v-lazy-container`. | `technical-stack-equivalent` |
| Lazy-loading model | Upstream uses `v-lazy-container` and `data-src`; OpenReader uses Element Plus `el-image lazy` with preview. | Visible behavior is acceptable if images load lazily, trigger layout recomputation, and preview does not toggle toolbar. | `acceptable-change` |
| Audio detection/API | `backend/api/books.go.chapterContent` returns `format: "audio"` for `book.Type == 1`, validates direct HTTP(S) audio URLs, keeps `content`, and adds `resourceUrl/resourceExpiresAt`. | Remote/direct audio and same-origin signed local/private `/api/audio-resource` are implemented. | `aligned` |
| Audio UI | `frontend/src/components/reader/ReaderAudioContent.vue` renders an audio branch with cover, hidden media element, elapsed/total time, seek slider, `-15s/+15s`, previous/next, play/pause, mute/unmute, volume slider, progress events, ended-to-next behavior, and restore-by-offset. | Uses Vue/native range inputs instead of upstream Element `el-slider`, preserving visible behavior. | `technical-stack-equivalent` |
| Reader controls | `Reader.vue`, `useReaderPointer`, `useReaderKeyboard`, and `useReaderMode` now keep audio out of text paging/scrolling, hide auto-reading/TTS, and let center taps toggle the mobile toolbar without side paging. | Escape still closes panels/returns home as an OpenReader compatibility behavior. | `aligned` |

### OpenReader adaptation contract

| Concern | Required behavior | Classification |
|---|---|---|
| CBZ storage | Accept `.cbz` in upload/import/local-store paths. Preserve the original archive under the existing `library/` model and derive extracted pages under a rebuildable, user-scoped directory inside that book's library root. | `must-fix` |
| CBZ archive safety | Reject ZIP-slip paths, absolute paths, drive prefixes, NUL names, symlinks, duplicate/conflicting paths, excessive entry counts, excessive per-entry size, and excessive total expanded size. Never write outside the derived extraction root. | `acceptable-change` security hardening |
| CBZ catalog | Build chapter rows by lexicographically sorted non-directory image entries. `ComicInfo.xml` is metadata only and must not become a readable chapter. Unsupported non-image entries should not break import; if retained for upstream parity, serving them must use a safe static resource route and explicit MIME handling. | `must-fix` |
| CBZ cover/info | If present, parse `ComicInfo.xml` for title/author and use the first valid image as cover without requiring network access. | `must-fix` |
| CBZ chapter endpoint | `GET /api/books/:id/chapters/:index/content` may keep the existing JSON envelope but must mark CBZ explicitly, for example `format: "cbz"` plus `content: "<img src='...'>"` or equivalent image metadata. Existing text clients must not break. | `acceptable-change` API adaptation |
| CBZ resource serving | Page image URLs must be same-origin, user/book scoped, and safe for iframe/image loading without exposing another user's library path. Current JWT-bearing API routes may serve JSON; direct image resources need either cookie-compatible auth or signed scoped capabilities. | `acceptable-change` security hardening |
| CBZ frontend | Reader must hide CBZ chapter titles, render image pages full readable width, keep mobile padding symmetric, recompute pagination/progress after image load, and keep image preview clicks from toggling toolbar. | `must-fix` |
| Audio chapter API | Introduce an explicit audio chapter contract instead of overloading text content. The chapter response should identify audio, provide a same-origin/signed resource URL, title, cover, and enough progress metadata to save current playback seconds. | `must-fix` |
| Audio frontend | Add a dedicated audio content branch/component with upstream controls: cover, elapsed/total duration, seek, -15s/+15s, previous/next chapter, play/pause, mute and volume. Mount should load metadata and respect autoplay after manual previous/next or ended advancement. | `must-fix` |
| Audio progress | Save audio progress on `timeupdate` as chapter position/time without disturbing text scroll position semantics. On chapter reopen, restore the saved playback second before autoplay. | `must-fix` |
| Audio input model | Parent touch/click/keyboard paging handlers must return early for audio. Center tap toggles toolbar only; audio must not trigger page navigation, auto-reading, or TTS. | `must-fix` |

### 2026-07-07 implementation note

Implemented in commit work following this contract:

- `GET /api/books/:id/chapters/:index/content` now emits `format: "audio"` for `type == 1` books and rejects unsafe direct audio URLs such as non-HTTP(S) schemes or embedded credentials.
- Audio chapter URLs skip text replacement rules so global cleanup regexes cannot corrupt media URLs.
- Reader loader keeps audio chapters out of the ordinary paragraph renderer and stores `resourceUrl/resourceExpiresAt` in an audio resource branch.
- `ReaderAudioContent` renders the audio media branch, restores the saved playback second, emits time progress, supports `-15s/+15s`, previous/next chapter, and advances on ended.
- Audio progress is persisted as playback seconds in the chapter offset while preserving existing text/EPUB/CBZ progress semantics.
- Audio chapters hide auto-reading and TTS controls, force the non-text page branch, and suppress text paging from click zones, side taps, keyboard arrows, page keys, Home/End, Space, and wheel-driven vertical reading.
- `scripts/smoke/reader-audio-contract.mjs` validates the audio reader in Chrome at 390×844 and 1440×900.

Follow-up status:

- Same-origin signed local/private `/api/audio-resource/:capability/*resourcePath` with byte-range support and MIME allow-list is covered in the local/private audio resources slice below.
- Online audio source parsing fixtures for empty content rules and common media selector rules are covered in the online audio source parsing slice below.

### 2026-07-07 follow-up contract: local/private audio resources

Additional OpenReader adaptation required by the audio contract:

- Upstream audio rendering treats chapter content as the playable media URL. OpenReader may keep safe remote HTTP(S) URLs direct, but local/private library media must not expose raw filesystem paths or require the login JWT inside the media URL.
- Local/private audio chapter media can be identified by chapter content, `chapter.url`, or `chapter.resourcePath`; the chosen path is valid only if it resolves below the scoped book library root after cleaning and symlink-aware absolute-path checks.
- The signed capability must be scoped to one user, one book, one source/library fingerprint, a single purpose (`audio-resource`), and a bounded expiry. It must not be interchangeable with login, EPUB, or CBZ capabilities.
- The resource route must support `GET`, `HEAD`, and byte `Range` requests with browser-friendly audio MIME headers, `nosniff`, `no-referrer`, `same-origin`, and private short cache headers.
- Error bodies and access logs must not leak host filesystem paths, signed capabilities, login JWTs, source credentials, or WebDAV secrets.

Required tests for this slice:

| Layer | Test requirement |
|---|---|
| API response | Remote safe HTTP(S) audio remains direct; local/private audio returns `/api/audio-resource/<capability>/<resourcePath>` with `format: "audio"` and RFC3339 expiry. |
| Resource serving | Valid signed local audio serves `GET`, `HEAD`, and `Range: bytes=...` with an allow-listed audio MIME type. |
| Authorization | Tampered/expired/wrong-purpose capabilities and ownership-changed books are rejected. |
| Path safety | Traversal, absolute paths outside the book library root, missing files, and unsupported media extensions are rejected with client-safe JSON errors. |
| Logging | Access logs redact the audio capability segment. |

Implementation status:

- Completed in this slice: `GET /api/books/:id/chapters/:index/content` now keeps safe remote HTTP(S) audio direct and returns same-origin signed `/api/audio-resource/<capability>/<path>` URLs for local/private audio under the scoped book library root.
- Completed in this slice: `/api/audio-resource/:capability/*resourcePath` supports `GET`, `HEAD`, browser byte ranges, allow-listed audio MIME types, private cache headers, and EPUB/CBZ-style capability redaction.
- Completed in this slice: capability validation is purpose-separated and binds user ID, book ID, resource path, file fingerprint, and expiry.
- Completed in this slice: API tests cover safe remote behavior, local signed resource serving, `HEAD`, `Range`, tampered capability, ownership changes, traversal, unsupported media, and log redaction.
- Follow-up online audio source parsing fixtures for empty content rules and common media selector rules are covered in the next slice.

### 2026-07-07 follow-up contract: online audio source parsing

Upstream evidence:

| Feature | Upstream authority | Contract |
|---|---|---|
| Source type | `BookSource.bookSourceType`, `BookType.audio`, `WebBook.getBookInfo/getChapterList` | A source whose `bookSourceType` is `1` creates/searches books with `book.type = 1`, which drives the Reader audio branch. |
| Empty content rule | `WebBook.getBookContent` | If `bookSource.getContentRule().content` is empty, upstream returns `bookChapter.url` directly. For audio sources this means a chapter URL can itself be the playable media URL. |
| Content rule result | `BookContent.analyzeContent` | If a content rule exists, upstream evaluates `ruleContent.content`, formats the result, follows `nextContentUrl` pages, applies `replaceRegex`, and returns the final string. For audio sources the returned string is consumed by the audio player as the media `src`. |
| URL base | `BookChapter.getAbsoluteURL`, `AnalyzeUrl`, and `BookContent.analyzeContent` | Relative chapter URLs are resolved from the TOC/book base before fetching; relative media URLs extracted from content rules must resolve against the redirected content page URL. |

Current OpenReader evidence and classification:

| Layer | Current evidence | Difference | Classification |
|---|---|---|---|
| Source type propagation | `engine.SearchBooks`, `FetchBookInfoAndTOC`, `createRemoteBook`, `changeBookSource` carry `BookSource.SourceType` to result/book `Type`. | Matches upstream audio type propagation. | `aligned` |
| Empty content rule | `engine.FetchChapterContent` falls back to `body|text` when `ContentRule` is empty. | Upstream returns the chapter URL directly; current behavior can fetch an MP3 URL as HTML/text or return page text, producing an unplayable audio chapter. | `must-fix` |
| Extracted media URL | `extractChapterContent` joins `Extract(...)` results but only resolves image URLs in the `html` branch. | For audio sources, `ruleContent.content` such as `audio|attr:src`, `source|attr:src`, or `a|attr:href` must produce absolute playable URLs, including relative paths. | `must-fix` |
| Pagination/replace | Existing content pagination and replace logic applies to all sources. | Keep for audio sources, but only after media URL extraction has produced URL strings. | `technical-stack-equivalent` |
| Safety | `chapterContent` and `audioreader` validate direct HTTP(S), credentials, and local/private resource paths. | Parser may return a candidate URL; API layer remains responsible for final safety enforcement. | `acceptable-change` security hardening |

Required tests for this slice:

| Layer | Test requirement |
|---|---|
| Engine parser | Audio source with empty `ContentRule` returns the resolved chapter URL without fetching/parsing a content page. |
| Engine parser | Audio source with `ContentRule: "audio|attr:src"` resolves relative media URLs against the redirected content page URL. |
| Engine parser | Audio source with `ContentRule: "source|attr:src"` and `a|attr:href` also resolves to absolute media URLs. |
| API integration | A remote audio book created from `bookSourceType: 1` can load chapter content as `format: "audio"` with `resourceUrl` equal to the resolved playable URL. |
| Regression | Existing text content, image HTML content, pagination, `ContentURLRule`, and replace rules remain unchanged for non-audio sources. |

Implementation status:

- Completed in this slice: `engine.FetchChapterContent` now matches upstream `WebBook.getBookContent` for audio sources by returning the resolved chapter URL directly when `ruleContent.content` is empty.
- Completed in this slice: audio source `ruleContent.content` extraction resolves `audio|attr:src`, `source|attr:src`, and `a|attr:href` media candidates against the content page URL, including relative and protocol-relative URLs.
- Completed in this slice: API integration test verifies a `bookSourceType: 1` remote book returns `format: "audio"` and a playable resolved `resourceUrl`.
- Still pending: broader real-world online audio source corpus fixtures if imported source sets contain additional non-selector/audio-specific rules.

### 2026-07-07 follow-up contract: audio custom controls

Additional upstream evidence from `web/src/components/Content.vue.renderAudio` and methods:

- The underlying `<audio>` element is present but not rendered as the primary browser-native control surface.
- Visible audio UI consists of cover, elapsed time, seek slider, total duration, `-15s`, previous chapter, play/pause, next chapter, `+15s`, mute/unmute icon, and volume slider.
- Component state includes `currentTime`, `audioDuration`, `playing`, `currentSpeed`, `audioVolume`, `startTime`, and `autoPlay`.
- `seekTime(val)` writes `audio.currentTime`; `setAudioVolume(val)` writes `audio.volume = val / 100`.
- `toggle()` pauses if currently playing, otherwise calls `play()`.
- `onPlay/onPause/onTimeupdate/onEnd` update playing/current time and emit progress or next chapter transitions.

OpenReader follow-up requirements:

| Concern | Required behavior | Status before implementation |
|---|---|---|
| Hidden media element | Keep the real `<audio preload="metadata">` for browser playback, but remove native `controls` from the primary UI. | `must-fix` |
| Play state | Add explicit play/pause button tied to audio `play/pause/ended/error` events. Browser autoplay failures must not break the page. | `must-fix` |
| Seek slider | Add visible range slider bound to current playback second and duration. Drag/change must call `audio.currentTime` and emit progress. | `must-fix` |
| Volume | Add mute/unmute and 0-100 volume slider, writing `audio.volume`. | `must-fix` |
| Cover | Display book cover when available, with a stable fallback. | `must-fix` |
| Tests | Extend unit/source tests and `reader-audio-contract.mjs` to verify no native controls, custom buttons/sliders, play/pause, seek, volume, and mute behavior. | `must-fix` |

Implementation status:

- Completed in the follow-up audio-control slice: OpenReader now hides native audio controls, renders cover/fallback, custom seek slider, play/pause, previous/next, `-15s/+15s`, mute/unmute, and volume slider.
- `frontend/tests/readerAudioContent.test.mjs` verifies the component wiring against the extracted upstream control contract.
- `scripts/smoke/reader-audio-contract.mjs` verifies custom controls in Chrome, including play/pause, seek, volume, mute, and mobile toolbar center-tap behavior.

### Recommended tests before implementation

Backend/API:

1. Import a fixture CBZ containing `ComicInfo.xml`, nested image paths, mixed image extensions, and one unsupported file; verify title/author/cover and sorted chapter rows.
2. Chapter content for a CBZ image returns explicit CBZ/image format and a safe same-origin image URL; missing extraction rebuilds from the preserved archive.
3. CBZ security tests reject traversal, absolute paths, duplicate normalized paths, symlink-like entries, excessive entry count, per-entry size, and total expanded size.
4. Existing TXT/EPUB/PDF/UMD imports and old local-store rows remain unchanged.
5. Audio fixture/API test verifies an audio chapter response shape, saved playback progress, previous/next autoplay state, and access isolation for the audio resource.

Frontend unit/contract:

1. `ReaderChapterContent` keeps CBZ titles hidden and treats image-only blocks as comic blocks.
2. Image load emits pagination/progress recomputation and preview click does not pass through to the reader click zones.
3. `ReaderAudioContent` renders upstream-equivalent controls and normalizes duration/time/volume.
4. Reader mode/input tests verify audio disables scroll/slide paging, keyboard paging, auto-reading, and TTS controls.
5. Progress persistence tests verify audio seconds do not overwrite text paragraph offsets incorrectly.

Real-browser gate:

1. Open a CBZ fixture at 1440×900, 390×844, and 360×800; confirm sorted pages, hidden titles, full-width images, symmetric mobile padding, preview, and no stale pagination after image load.
2. Open an audio fixture at the same viewports; confirm metadata load, seek, -15s/+15s, previous/next chapter, volume/mute, ended-to-next behavior, and restored playback second.
3. Confirm toolbar/panel coexistence still follows the Reader mobile state contract and that audio/image interactions do not produce blank pages or unintended center-click toggles.

Deferred from this slice:

- Broader real-world online audio book-source corpus fixtures, if imported source sets expose audio through additional non-selector/audio-specific rules.
- Browser autoplay restrictions: OpenReader may require an explicit user gesture before first audio playback, but previous/next and ended autoplay should match upstream after the user has interacted.

### 2026-07-07 follow-up contract: Reader TTS/read-aloud controls

Upstream evidence from `reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`:

| Feature | Upstream authority | Contract |
|---|---|---|
| Availability | `web/src/views/Reader.vue` floating button `v-if="speechAvalable && !isEpub && !isCarToon && !isAudio"` | System speech is available only when `window.speechSynthesis.getVoices` exists and the current chapter is not EPUB, comic/image, or audio. |
| Read bar visibility | `showReadBar`, `readBarTheme`, floating TTS button | Clicking the read-aloud button toggles the read bar independently from active speaking; the bar can be visible before playback starts. |
| Paging/slide mode coupling | `isSlideRead()` | Showing the read bar disables slide-read behavior in the same way as auto-reading, EPUB, comic, and audio modes. |
| Bottom spacing | `chapterTheme()` | When the read bar is visible, content adds bottom padding of `280px` when config is expanded and `80px` when collapsed. |
| Controls | `read-bar` template and `speechPrev/toggleSpeech/speechNext/exitRead` | Visible controls are close, previous paragraph, play/pause, next paragraph, and collapse/expand config. Close stops speech and hides the bar. |
| Voice list | `fetchVoiceList()` | Voices are sorted with `zh-*` voices first, then by `lang`; config presents the voice list as selectable buttons. |
| Persisted config | `plugins/config.js` and `plugins/vuex.js` `speechVoiceConfig` | Defaults are `voiceName: ""`, `speechRate: 1`, `speechPitch: 1`; changes persist in cache. |
| Rate range | `speechRate` slider | Rate is `0.5` to `2`, step `0.1`, reset `1`. Changing while speaking restarts the current paragraph. |
| Pitch range | `speechPitch` slider | Pitch is `0` to `2`, step `0.1`, reset `1`. Changing while speaking restarts the current paragraph. |
| Sleep timer | `speechMinutes` slider and `speechEndTime` | Sleep is `0` to `180` minutes. When the deadline passes, reading stops and shows `定时关闭朗读`. |
| Paragraph source | `getCurrentParagraph/getPrevParagraph/getNextParagraph` | Speech reads visible `h3,p` DOM paragraphs, highlights the active paragraph with class `reading`, previous/next move across chapter boundaries. |
| Error handling | `startSpeech()` `utterance.onerror` | Speech synthesis errors show `朗读错误: ...` and update speaking state without blanking the page. |

Current OpenReader evidence and classification:

| Layer | Current evidence | Difference | Classification |
|---|---|---|---|
| Availability | `Reader.vue` `ttsSupportedForChapter` checks speech support, non-EPUB, and non-audio; image/comic uses chapter format checks elsewhere. | Equivalent for audio/EPUB; image/comic eligibility still needs explicit verification against `chapterFormat` values. | `unknown` |
| Read bar visibility | `Reader.vue` now keeps `ttsBarRequested` separate from `tts.state.playing`, and `readerTTSBarVisible()` gates only requested/support/chapter eligibility. | Matches upstream requirement that opening the read bar does not start speech; unlike upstream, OpenReader keeps the mobile tool layer policy governed by the existing Reader chrome contract. | `technical-stack-equivalent` |
| Read bar structure | `ReaderTTSBar.vue` now exposes close, previous paragraph, play/pause, next paragraph, collapse/expand config, voice list, rate, pitch, and sleep controls. | Equivalent control surface, with Vue 3/Element Plus styling and current pause/resume support retained. | `technical-stack-equivalent` |
| Rate range | `useTTS`, `readerStore`, `ReaderTTSBar`, `ReaderSettingsPanel`, `Settings.vue` use `0.5–2`. | Matches upstream. | `aligned` |
| Pitch range | `useTTS`, `readerStore`, `ReaderTTSBar`, `ReaderSettingsPanel`, `Settings.vue` use `0–2`. | Matches upstream. | `aligned` |
| Voice ordering | `useTTS.loadVoices()` uses `sortTTSVoices()`, sorting `zh-*` first then by language without filtering non-English/non-Chinese voices. | Matches upstream ordering while keeping `voiceURI` persistence. | `aligned` |
| Config persistence | Pinia reader store persists `ttsRate`, `ttsPitch`, `ttsVoiceURI`. | Uses `voiceURI` instead of upstream `voiceName`; this is a Vue 3/browser-stability adaptation as long as display labels remain human-readable. | `acceptable-change` |
| Restart on config change | `useTTS.setRate/setPitch/setVoice` call `restartCurrent()`. | Matches upstream restart-on-change behavior. | `aligned` |
| Sleep timer | `useReaderTTS` uses 0–180 minutes and emits `定时关闭朗读`. | Matches upstream timer range and message. | `aligned` |
| Paragraph traversal | `useReaderTTS` now derives speech paragraphs from rendered `h1,h2,h3,p`, starts from an active/visible paragraph, marks `.reading/.tts-active`, and routes previous/next across chapter boundaries. | Upstream reads `h3,p`; OpenReader includes `h1/h2` because current Vue renderer uses headings for chapter titles. | `technical-stack-equivalent` |
| Error handling | `useTTS` now forwards utterance errors to `useReaderTTS`, which displays `朗读错误: ...`. | Matches upstream user-visible error semantics. | `aligned` |

Required tests for this TTS slice:

| Layer | Test requirement |
|---|---|
| Utility/store | Rate clamps to `0.5–2`; pitch clamps to `0–2`; sleep remains `0–180`. |
| Voice ordering | Browser voices are sorted with `zh-*` first, then by `lang`, without dropping non-Chinese/non-English voices when available. |
| UI contract | Reader TTS bar and reader/global settings expose the upstream rate and pitch ranges. |
| Runtime behavior | Changing rate, pitch, or voice while speaking restarts the current paragraph. |
| Follow-up UI | TTS button toggles bar visibility independently from speaking, and the bar exposes voice list/config in the reader surface. |
| Follow-up navigation | Previous/next paragraph starts from visible `h3,p` and crosses chapter boundaries. |
| Follow-up errors | Speech synthesis errors surface `朗读错误: ...` without breaking the Reader. |

Implementation status:

- Completed in this slice: TTS rate is normalized to upstream `0.5–2` everywhere it is stored or edited.
- Completed in this slice: TTS pitch is normalized to upstream `0–2` everywhere it is stored or edited.
- Completed in this slice: browser voices are sorted with upstream `zh-*` first, then by language, without dropping non-Chinese/non-English voices.
- Completed in this slice: unit tests cover TTS rate/pitch normalization, sleep timer range, progress label, deadline expiration, and voice ordering.
- Completed in this slice: Reader TTS button opens/closes the read bar without starting speech; the play button starts speech; closing the read bar stops active speech.
- Completed in this slice: `ReaderTTSBar` now contains voice list/config controls in the Reader surface and uses an upstream-like high layer so mobile floating tools do not intercept its controls.
- Completed in this slice: `scripts/smoke/reader-tts-contract.mjs` verifies the real-browser TTS bar contract with a mocked `speechSynthesis`.
- Completed in this slice: TTS paragraph source now comes from rendered DOM headings/paragraphs rather than only splitting plain text, and starts from the active or first visible paragraph.
- Completed in this slice: TTS previous/next now restarts the target DOM paragraph and can cross chapter boundaries.
- Completed in this slice: `speechSynthesis` utterance errors are surfaced as `朗读错误: ...`.
- Validation note: unit tests and production build passed after this slice. The enhanced real-browser TTS smoke was updated to check next-paragraph DOM highlighting and error toast, but final rerun was blocked by the workspace approval/spend cap after an initial timing assertion exposed and fixed an insufficient wait condition.

### 2026-07-08 follow-up contract: Reader settings panel labels and first-screen structure

Upstream evidence from `reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`:

| Feature | Upstream authority | Contract |
|---|---|---|
| Component | `web/src/components/ReadSettings.vue` | Reader settings is a scrollable `settings-wrapper` with title `设置` and action `重置为默认配置`. |
| First-screen order | `ReadSettings.vue` template | The main sections appear in order: `特殊模式`, `配置方案`, `方案类型`, `阅读主题`, custom theme block, `正文字体`, `简繁转换`, `字体大小`, `字体粗细`, `段落行高`, `段落间距`, `字体颜色`, `页面模式`, `页面宽度` on desktop, `翻页方式`, `动画时长`, `自动翻页`, `滚动像素`, `翻页速度`, `全屏点击`, `选择文字`, operations. |
| Selection controls | `.selection-zone .span-item` | Choices are button-like discrete items, not hard-to-hit raw sliders. Numeric settings use a minus/value/plus control. |
| Selection color | `ReadSettings.vue` scoped style | `.span-item.selected`, custom background `.selected`, theme/font `.selected`, upload active icons, and hover states use `#ed4259` for selected border/text. |
| Settings density | `ReadSettings.vue` scoped style | `.settings-wrapper` uses 24px padding, title margin-bottom 28px, list rows separated by 20px, left labels are inline-block `56px` plus `16px` right margin, and right-side controls start on the same row. |
| Mobile gating | `v-if="!$store.state.miniInterface"` and `v-show` on read methods | `页面宽度` is hidden on mobile; `左右滑动` appears only on mobile. |
| User-requested adaptation | Existing OpenReader issue/request | OpenReader may keep the safer minus/value/plus `ReaderSettingStepper` and larger mobile controls instead of upstream small inputs/sliders. |

Current OpenReader evidence and classification:

| Layer | Current evidence | Difference | Classification |
|---|---|---|---|
| Shell/title | `ReaderMobileWorkspacePanel` wraps `ReaderSettingsPanel`; `ReaderSettingsPanel` title row shows `设置` and `重置为默认配置`. | Upstream `Reader.vue` opens `ReadSettings` directly inside `el-popover popper-class="popper-component"`; there is no second mobile wrapper title. The current mobile wrapper adds a duplicate `设置` header. | `must-fix` |
| Order | `ReaderSettingsPanel.vue` broadly follows upstream order and adds brightness plus TTS controls. | `亮度` and TTS controls are OpenReader enhancements/user-requested additions; placement does not block upstream core flow. | `acceptable-change` |
| Labels | Current labels shorten several upstream names: `主题`, `字体`, `字号`, `字重`, `行高`. | These should match upstream visible text: `阅读主题`, `正文字体`, `字体大小`, `字体粗细`, `段落行高`. | `must-fix` |
| Mobile row density | `ReaderSettingsPanel.vue` mobile CSS uses stacked `.setting-row` labels for general rows and only makes typography/stepper rows two-column. | Upstream keeps labels and controls on the same row throughout the settings list, which makes the first screen denser and reduces misalignment between settings sections. | `must-fix` |
| Selected control color | `ReaderSettingsPanel.vue` still uses `#409eff`, `#0f5451`, and `#2f6f6d` for active theme dots, uploaded labels, font options, hover states, and font-size presets. | Upstream selected state is consistently `#ed4259`; blue/teal selected states make the settings UI look like the wrong component system. | `must-fix` |
| Discrete option controls | `ReaderSettingsPanel.vue` uses `el-radio-group` / `el-radio-button` for `特殊模式`, `简繁转换`, `页面模式`, `翻页方式`, `自动阅读`, `全屏点击`, and `选择文字`. | Upstream renders these as `.span-item` button-like options in `.selection-zone`, not Element segmented radios. | `must-fix` |
| Numeric controls | `ReaderSettingStepper` is used for size/weight/line-height/paragraph/animation/auto-read/TTS. | Preserves user-requested minus/value/plus controls; do not revert to mis-tap sliders. | `intentional-redesign` |
| Mobile gating | `页面宽度` is hidden when `miniInterface`; `左右滑动` is shown only when `miniInterface`. | Matches upstream gating. | `aligned` |

Required tests for this settings-label slice:

| Layer | Test requirement |
|---|---|
| Unit/static | `ReaderSettingsPanel` must expose the upstream canonical labels for theme/font/typography sections. Mobile settings must suppress the generic workspace header so the panel has only one `设置` title row. |
| Unit/static | Mobile settings CSS must keep `.setting-row` in a two-column `72px + content` layout, matching upstream `56px + 16px` label geometry while preserving larger touch controls. |
| Unit/static | Reader settings active/selected CSS must use upstream `#ed4259` and reject the previous blue/teal active colors. |
| Unit/static | Reader settings must not use `el-radio-group` / `el-radio-button`; upstream-style discrete options must use local `.selection-zone` and `.selection-button` controls. |
| Regression | Existing reader settings stepper tests must continue passing. |
| Build | Production build must compile after label changes. |

Implementation status:

- Completed in this slice: `ReaderSettingsPanel` visible labels now use upstream canonical text for `阅读主题`, `正文字体`, `字体大小`, `字体粗细`, and `段落行高`.
- Completed in this slice: `frontend/tests/readerSettingsPanelContract.test.mjs` locks those labels and rejects the previous shortened forms.
- Completed in this slice: mobile settings now suppresses the generic `ReaderMobileWorkspacePanel` header, keeps the upstream-like `ReadSettings` title row, and closes via the still-visible mobile settings tool toggle.
- Completed in this slice: mobile settings rows now use upstream-like two-column label/control geometry for the base row layout.
- Completed in this slice: settings active theme dots, background selections, uploaded labels, font options, hover states, and font-size presets now use the upstream `#ed4259` selected color instead of blue/teal.
- Completed in this slice: first-batch discrete options now use upstream-like `.selection-zone` / `.selection-button` controls instead of Element radio groups.
- Pending follow-up: detailed per-control visual pass for mobile `ReadSettings` first-screen density after the base row structure is aligned.

## Required workflow for each future module

1. Use `readerdev-compat-inventory`.
2. Update this file or a focused `docs/compat/*.md` contract.
3. Add/update tests for `must-fix` behavior.
4. Implement OpenReader changes.
5. Run module gate and record allowed differences.
6. Publish Git commits promptly. Publish Docker after any coherent, fully verified slice suitable for user validation; a complete module boundary remains preferred.
