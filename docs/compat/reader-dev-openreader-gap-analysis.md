# Reader-dev vs OpenReader Gap Analysis

Baseline: `changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`.

Local upstream checkout used for this pass: `/private/tmp/reader-dev-upstream-audit`.

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
| Reader mobile panel structure | Primary shelf/source/catalog/settings use Element popovers; bookmarks/search-content are App-level dialogs; cache is an inline read-bar zone. | OpenReader now aligns the four primary panels with `ReaderMobileWorkspacePanel.primary`, but Reader still has local workspace panels for bookmark/search/cache while global drawer implementations coexist. | Primary popovers are aligned; bookmarks/search/cache are a separate `must-fix` P0 dialog/cache slice, not valid variants of a popover. | `partial` for Reader P0 | Keep primary-panel browser contract; add App-dialog and inline-cache browser contracts before replacing the remaining local workspaces. |
| Reader mobile content geometry | Upstream mini `.chapter` uses `width: 100vw`, `padding: 0 16px`, `box-sizing: border-box`, `text-align: justify`; slide mode also uses 16px content margins. | Current mobile `.reader-page` uses `width: 100vw`, `padding: 0 16px`, `box-sizing: border-box`, and justified reader body/paragraphs. | Base geometry is implemented; acceptance requires actual rendered paragraph left/right gap checks, not only CSS value checks. | `aligned` for base P0 | DOM geometry probe for page/body/paragraph left/right gaps within 1px across 390×844 and 360×800; ensure toolbar show/hide does not shift content. |
| Reader scrolling vs click paging | Upstream has page/scroll modes with discrete click navigation. | User requested continuous native finger/wheel scrolling while click paging remains segmented. | Intentional UX improvement if it does not change mode selection semantics. | `acceptable-change` | Browser scroll continuity probe; click paging regression tests. |
| Reader settings controls | Upstream uses controls that are easier to distinguish visually; user requested minus/value/plus controls instead of current easy-to-mis-tap slider behavior. | Current setting stepper exists but must be rechecked against upstream layout/state. | Allowed UX adaptation, but values/defaults/state must match upstream. | `acceptable-change` | Unit tests for value bounds; browser setting interaction test. |
| Reader content formats | Upstream `Content.vue` handles text, images/comic-like content, EPUB iframe documents, audio-related branches, and cross-chapter behavior. | Current `ReaderChapterContent.vue` handles text/images/volume blocks, CBZ image resources, EPUB iframe resources, and a dedicated audio branch for `type === 1` chapters; continuous chapter retention, extension, anchor, error, and explicit-jump behavior follows the extracted fixed-baseline contract. | EPUB, image/CBZ rendering/import/resource serving, continuous cross-chapter behavior, remote/local audio playback, and audio resource capabilities are implemented and browser/backend-validated. TTS parity remains pending. | `aligned` for implemented formats; `unknown` for TTS | Keep EPUB/image/CBZ/continuous/audio browser contracts; add TTS fixtures. |
| BookInfo | Upstream has one `web/src/components/BookInfo.vue` used from workspace and reader flows. | Current has shared `BookInfoDialog.vue` / `BookInfoPanel.vue` / `OverlayBookInfo.vue`; the old `/books/:id` URL redirects to the Index workspace and opens the shared dialog. | The independent `BookDetail.vue` route structure has been removed from the product path; search/discover/route actions are centralized; Reader opens plain BookInfo without injecting toolbar shortcut actions. Remaining P1 work is Index-scene placement and search/discover/source flow convergence. | `partial` for P1 | Single BookInfo action contract; search/shelf/reader reuse tests. |
| Bookshelf/BookManage/BookGroup | Upstream: `BookShelf.vue`, `BookManage.vue`, `BookGroup.vue` under Index workspace. | Current: `Home.vue`, overlay management components, categories/store utilities. | Some enhancements may be valid, but workflow and mobile sidebar behavior need upstream comparison. | `unknown` | Workspace browser flows; category/order tests. |
| Mobile Index sidebar | Upstream sidebar width/drag/fixed bottom buttons must be extracted from `Index.vue` and related CSS. | Current `AppLayout.vue` and mobile navigation had reported drag/fixed-button mismatch. | User-visible mismatch: GitHub/day-night buttons should not slide with drawer content. | `must-fix` for P1 | Mobile drag smoke; fixed-bottom button geometry probe. |
| Search/explore/source flow | Upstream Index integrates search/explore/source and BookInfo transitions. | Current has separate `Search.vue`, `Discover.vue`, `Sources.vue` pages. | Flow fragmentation can change API order, panel state, and back behavior. | `must-fix` for P1 | Search → result group → BookInfo → add/read browser test. |
| Online source parsing | Upstream reader3-compatible source semantics live across web components and Java backend. | Current Go parser in `backend/engine/source_*` has tests and compatibility shims. | Must continue fixture-based extraction; do not infer equivalence from passing current tests alone. | `unknown` | HTML fixture/golden tests for search/info/toc/content. |
| Local import catalog parsing | Upstream `BookController.kt` imports local files through `Book.initLocalBook(...)` and `LocalBook.getChapterList(...)`; TXT parsing uses `TextFile.kt` with `DefaultData.txtTocRules`, enabled-rule reverse scoring, Java regex constructs such as `(?<=...)` and `(?!...)`, deterministic local file reads, and `TocEmptyException` for empty catalogs. | Go exposes the enabled upstream TXT rules, reverse scoring and supported lookbehind/lookahead normalization. Direct upload and LocalStore/WebDAV previews all issue user-scoped staged-input tokens, so a confirmation parses the same bytes even after the source file changes. | Rule and immutable-input behavior are aligned for the covered upstream constructs. Explicit upload/parser byte bounds and background cleanup remain a security/reliability gap. | `partial` | Golden TXT fixtures for upstream enabled rules, Java regex normalization, negative-lookahead false-positive prevention, deterministic preview/import/reparse without upload; keep EPUB/PDF/UMD/CBZ regression fixtures. |
| Replace rules/content cleanup | Upstream `ReplaceRule.vue`, `ReplaceRuleForm.vue`, `Reader.vue`, `ReplaceRuleController.kt`. | Current Go endpoints and overlays exist. | Audit found default-mode, list/application order, regex flags/failure handling, form validation, and selected-text editor-flow differences. | `must-fix` P2 | Rule-semantics API tests; selected-text editor contract; browser manager/editor smoke. |
| Bookmarks | Upstream `Bookmark.vue`, `BookmarkForm.vue`, `Reader.vue`, `BookmarkController.kt`. | Current ID-backed bookmark APIs and root overlays exist. | Form and manager ownership are close, but selected-text paragraph context, stale-content jump fallback, list ordering and direct API validation differ. | `must-fix` P2 | Bookmark context/jump/API contracts; three-viewport dialog smoke. |
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
| Tool layer default | `Reader.vue` data contains `showToolBar: true`. | `Reader.vue` now contains `mobileChromeVisible = ref(true)`. | Base behavior is aligned; retain direct state and browser coverage. |
| Panel open | Upstream click handler returns when primary popovers/settings are visible; opening a panel does not hide `showToolBar`. | `useReaderTools` and `useReaderPanels` do not hide `mobileChromeVisible` for panel actions. | Base behavior is aligned; primary-panel toggle/exclusivity remains a separate must-fix. |
| Mobile panel container | Upstream mini `.popper-component` is `top: 0`, `left: 0`, `width: 100vw`, without border/shadow; each child owns its own `24px + safe-area` padding. | `ReaderMobileWorkspacePanel` is full viewport but centrally reserves `58px` above and `96px` below for every panel. | Rebuild generic panel geometry and let panel content own the upstream-like insets. |
| Horizontal layout | Upstream mini chapter padding is 16px and justified. | Current mobile reader page uses `width: 100vw`, `padding: 0 16px`, `box-sizing: border-box`, and justified body. | Base geometry is aligned; real rendered symmetry remains required. |
| Tests | Upstream behavior is source contract. | Existing toolbar-hide tests were replaced, but not every primary tool has a same-button toggle/exclusive-panel contract. | Add state and browser coverage before implementation. |

### 2026-07-06 implementation note

Implemented in commit work following this contract:

- Mobile reader tool layer now defaults to visible and initial chapter loading no longer hides it.
- Opening reader tools/panels no longer forces the mobile tool layer closed.
- Mobile shelf, catalog, source, and settings now use the toolbar-coexisting full-width workspace/popup adaptation. Bookmark, search, and cache were initially moved there too, but the 2026-07-10 dialog/cache audit classifies that structure as incorrect and requires replacement.
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

### 2026-07-10 focused audit: mobile primary popovers and reader chrome

Authoritative upstream files:

- `web/src/views/Reader.vue`: the four primary `el-popover` controls, `showToolBar: true`, central-content state machine, keyboard branches, `popperWidth`, and toolbar/read-bar z-index ordering.
- `web/src/App.vue`: mini `.popper-component` is pinned to `(0, 0)`, forced to `100vw`, has no border/shadow, and stays visually beneath the reader top toolbar.
- `web/src/components/BookShelf.vue`, `BookSource.vue`, and `PopCatalog.vue`: each popover body uses its own negative Element-Popover margin and `24px + safe-area` content padding; `ReadSettings.vue` owns its own settings title/layout.

| Contract | Upstream behavior | Current OpenReader evidence | Classification | Required test before code |
|---|---|---|---|---|
| Initial chrome | `showToolBar` starts `true`; loading a chapter does not change it. | `Reader.vue` starts `mobileChromeVisible` at `true`; `useReaderChapterLoader` changes it only when a caller explicitly asks for `hideChrome`. | `aligned` | Initial 390×844 and 360×800 assertions. |
| Primary panel action | Shelf, source, catalog, and settings are click-triggered `el-popover` references. A second click on the same reference closes its popover; opening a different reference must not leave a second visible primary workspace. Neither operation mutates `showToolBar`. | `useReaderPrimaryPanels` owns the four refs. `toggle()` closes all primary refs before opening another and closes a same-tool second click; it never receives or mutates `mobileChromeVisible`. Reader routes only mobile primary actions through it. | `aligned` for state transition | `readerPrimaryPanels.test.mjs` plus browser test asserting same-tool close, A→B switching, exactly one workspace, and visible chrome at 390×844/360×800. |
| Popover root geometry | In mini mode, `.popper-component` is `top:0`, `left:0`, `width:100vw`, `box-sizing:border-box`, no border/shadow. Its component decides body padding. | `ReaderMobileWorkspacePanel.primary` is a `0px`-padded, direct `100vw × 100dvh` popup root with no glass blur. Its `Reader.vue` child body owns `24px + safe-area` insets; shelf/toc/source lists fill the remaining scrollable row and settings owns its own scroll. | `aligned` for the four primary popovers | Static contract plus 390×844/360×800 DOM assertion for root `(0,0,100vw)`, `0px` root padding, primary body, and no visible drawer. |
| Primary panel header ownership | Shelf/source/catalog provide their own title/actions; settings provides its own `设置 / 重置为默认配置` row. There is no generic popover close header. | Shelf and catalog now render their own upstream-like title/action rows, source retains its own `来源(n)` row, and settings retains `ReaderSettingsPanel`'s title row. All four use `primary` with `show-header=false`; App-level bookmark/search dialogs and the inline cache zone are documented in the focused audit below. | `aligned` for shelf/source/catalog/settings. | Static ownership contract and mobile DOM assertion that primary panels have no generic header. |
| Chrome layering | Top toolbar is `z-index:2001`; popover is below it. The content click handler first returns when one of the four primary popovers is open, so no tool toggle/page action leaks through. | Reader mobile chrome uses `z-index:8`, workspace uses `z-index:7`, `isOverlayOpen` blocks reader click/touch/wheel actions, and workspace events stop propagation. | `technical-stack-equivalent` for current primary panels | Panel click + center tap + side paging test, including each primary panel. |
| Floating dialogs | Bookmark, search-content, and BookInfo are App-level dialogs raised from Reader events, not the four primary popovers. | Bookmark/search now use App-level Element Plus dialogs; BookInfo remains the existing global overlay. Their modal interactions are covered by the focused dialog contract below. | `aligned` for bookmark/search shell ownership. | Real-browser dialog and click-through contract. |
| Paging/keyboard | With no primary popover open, center toggles toolbar; side click/page and arrow keys hide toolbar before moving. With a primary popover open, keyboard does nothing. `Escape` returns to shelf. | `useReaderKeyboard` takes the computed `useReaderPrimaryPanels.isOpen()` state and returns before page, arrow, home/end, space, or Escape handling. The pre-existing pointer overlay guard continues to prevent panel clicks from reaching reader interactions. | `aligned` for primary popovers | `readerKeyboard.test.mjs` locks the keyboard guard; browser panel center-tap check covers pointer leakage. Bookmark/search/cache remain deferred to their dialog audit. |

Allowed differences retained for this slice:

- Vue 3/Element Plus workspace implementation may replace Vue 2 `el-popover`, provided the state machine, visible layering, root geometry, and per-panel layout match the contract.
- Native continuous finger/wheel scrolling with click paging remains the explicit user-requested improvement.
- Numeric minus/value/plus setting controls remain the explicit user-requested improvement.

Implementation order after this audit gate:

1. Completed: `useReaderPrimaryPanels` is the single controller shared by shelf/source/catalog/settings, without coupling it to `mobileChromeVisible`.
2. Completed: root geometry moved out of `ReaderMobileWorkspacePanel.primary`; shelf/source/catalog own upstream-like content headers/insets and settings retains its existing owner.
3. Completed: keyboard guards use the active primary-panel contract; pointer guards already use the shared reader-overlay guard.
4. Run desktop 1440×900 plus mobile 390×844/360×800 browser contracts; publish a Docker image only after the cohesive slice passes.

### 2026-07-10 focused audit: Reader App dialogs and inline cache zone

Authoritative upstream files:

- `web/src/views/Reader.vue`: Reader emits `showBookmarkDialog` and `showSearchBookContentDialog`, consumes `showBookmark` and `showSearchContent` results, and renders the cache controls directly inside `.read-bar` as `showCacheContentZone`.
- `web/src/App.vue`: owns one root `Bookmark` and one root `SearchBookContent` instance; event listeners set their visible state and target book.
- `web/src/components/Bookmark.vue`: book-scoped bookmark-management dialog; mini interface uses `el-dialog :fullscreen="true"` and jump emits an event back to Reader before closing.
- `web/src/components/SearchBookContent.vue`: root fullscreen-on-mini dialog with a header search input, tabular results, load-more, and saved-scroll restoration; choosing a result emits back to Reader before closing.

| Contract | Upstream behavior | Current OpenReader evidence | Classification | Required test before code |
|---|---|---|---|---|
| Bookmark ownership | Reader merges the reading/shelf book and asks the root App `Bookmark` dialog to open. The root dialog filters the global bookmark collection to that book. Reader itself owns the content positioning after receiving the selected bookmark. | `Reader.vue` now routes the current merged reading book through `useOverlayStore.openBookmark()`. It retains only position-scoped bookmark creation/note mutation with `trackItems:false`; `OverlayBookmarks` is the unique list/edit/import/delete UI/data owner. | `aligned` for UI ownership; position-scoped creation is a required reader responsibility. | Reader action opens exactly one global bookmark dialog with the current book; no Reader-local bookmark workspace remains; selected bookmark closes dialog and preserves reader route semantics. |
| Bookmark dialog shell | Upstream uses App-level `el-dialog`, `dialogWidth` on desktop and `fullscreen` on mini interface; its title is `<book> 书签管理` with import, table selection, batch delete, jump, and edit. | `OverlayBookmarks` now uses one App-level `el-dialog`: 880px desktop width, `fullscreen` mini mode, title/import action, table selection, batch delete, jump, and edit. The old Reader workspace/drawer and card panel have been removed. | `aligned` | 1440×900 dialog width/title/action contract; 390×844/360×800 fullscreen dialog rect; verify no bottom drawer and no duplicate dialog. |
| Bookmark data adaptation | Upstream stores bookmarks globally and filters by name/author; save/delete/import refresh the shared collection. | OpenReader keeps authenticated per-book APIs and user-scoped data, with `useBookBookmarks` update events. This is required for Go/multi-user isolation. | `acceptable-change` | Same-book and cross-book jump/import/delete tests; ensure one active data owner and refresh event path. |
| Search ownership | Reader asks the root App `SearchBookContent` dialog to open with the current book. A chosen result emits `showSearchContent`; Reader loads/rebuilds the target chapter then finds/highlights the requested occurrence. | Reader now opens `useOverlayStore.openSearchBookContent()` and no longer owns a search panel or `useBookContentSearch` instance. The global dialog owns the sole search state and routes the existing `chapter`, `line`, `match`, `percent`, and `q` query fields back to Reader. | `aligned` | Reader action opens only global search dialog; result closes it, preserves route compatibility, then uses existing route-sync highlighting. |
| Search dialog shell | Upstream uses root `el-dialog`, fullscreen on mini interface. The header is the search input; results are a table; footer provides load-more, restore-last-position when relevant, and cancel. | `OverlayBookContentSearch` now uses one App-level `el-dialog`: header input, tabular results, load-more, existing full-scan enhancement, saved-scroll restoration, cancel, and mini fullscreen. The old narrow drawer and Reader workspace/card UI have been removed. | `aligned` | Desktop/mobile dialog geometry and no-drawer assertion; search input/header, result selection, load-more, cancellation, and previous-result scroll restoration tests. |
| Search pagination/API | Upstream posts a book URL, keyword, and `lastIndex`; its result rows carry chapter/result text and a next cursor. | OpenReader uses a per-book authenticated route plus richer `chapter/line/match/percent/q` navigation metadata, remote/local paging guards, and scoped result cache. | `acceptable-change` | Preserve current API/data fields and route compatibility while replacing only UI ownership/shell. No backend route change is required in this slice. |
| Cache ownership | `showCacheContent()` toggles an inline `.cache-content-zone` within Reader's bottom `.read-bar`. It shows `后面50章`, `后面100章`, `后面全部`; while active it replaces actions with status and cancel. On mini interface this zone is part of the bottom reader bar, not a dialog. | `showCacheContentZone` is now the Reader state. `ReaderDesktopProgress` positions `ReaderCachePanel` beside the desktop progress control, and `ReaderMobileChrome` places it inside the bottom bar. It preserves 50/100/all/status/cancel and toggles without closing the tool layer. | `aligned` | Cache action toggles a single inline zone, does not open workspace/drawer or hide chrome, preserves 50/100/all/cancel and status, and remains reachable at desktop/mobile target viewports. |
| Dialog/chrome layering | Upstream Reader's top tool bar is above ordinary Reader content; App dialogs own their modal input surface. Exact Element UI z-index interaction must be tested instead of inferred from current custom workspace z-indexes. | Global dialogs now own the full-screen mobile input surface while Reader chrome state remains `visible`; a center click inside each dialog is caught by the dialog and does not toggle or page Reader. Primary toolbar/workspace state remains independent. | `aligned` for state isolation and click blocking. | Runtime mobile dialog click-through and geometry assertion at both target viewports. |

Allowed differences retained for this slice:

- Go per-book bookmark/search APIs, user-scoped storage, explicit route query fields, and browser-cache paging are multi-user/runtime adaptations; their data semantics remain intact.
- The result presentation may use Vue 3/Element Plus components rather than Vue 2 tables only when the dialog ownership, fullscreen/mobile behavior, controls, and jump transition remain equivalent.

Implementation order after this audit gate:

1. Completed: Reader bookmark/search actions route through `useOverlayStore`; local list/search drawers, card panels, and duplicate list/search owners are removed.
2. Completed: `OverlayBookmarks` and `OverlayBookContentSearch` are App-level dialogs with upstream-like desktop/fullscreen mobile shells, while keeping the existing Go API and route semantics.
3. Completed: `showCacheDrawer` is replaced by `showCacheContentZone` in the desktop/mobile read bars; the cache engine and cancellation behavior are unchanged.
4. Completed: `readerGlobalDialogContract.test.mjs` plus `scripts/smoke/reader-mobile-contract.mjs` cover no-drawer ownership, desktop 1440×900 dialog/cache geometry, 390×844 and 360×800 fullscreen geometry, inline cache controls, Dialog click blocking, and tool-layer state.

### 2026-07-10 implementation and validation note

- Validation passed: frontend 298 tests, production build, backend `go test ./...`, and the real-browser Reader contract at 1440×900, 390×844, and 360×800.
- The user-requested native continuous finger/wheel scrolling and numeric reader setting steppers remain intact.
- The former Reader-bound note dialog has been replaced by the root `OverlayBookmarkForm` protocol documented below.

### 2026-07-11 focused audit: SearchBookContent result completeness and cancellation

Authoritative upstream files:

- `web/src/components/SearchBookContent.vue`: Enter starts a new search at `lastIndex = -1`; `加载更多` continues from the server cursor; result-row selection emits the complete result and closes the root dialog.
- `src/main/java/com/htmake/reader/api/controller/BookController.kt#searchBookContent/searchChapter`: starts at `lastIndex + 1`, scans chapters in order, appends every match from a scanned chapter, then stops only after that complete chapter makes the requested result size reachable. It returns the last scanned chapter index.
- The upstream handler sets a connection close handler so no later chapter work is scheduled after the browser has disconnected.

| Contract | Upstream behavior | Current OpenReader evidence | Classification | Required tests before code |
|---|---|---|---|---|
| Dialog/state ownership | One root dialog owns keyword, result list, cursor, saved result-list scroll, load-more and result selection. It is fullscreen on mini interface; Reader only consumes a chosen result. | `OverlayBookContentSearch` + `useBookContentSearch` own one root Element Plus dialog and Reader consumes route result metadata. | `aligned` | Keep root/fullscreen/no-drawer and saved-scroll contracts. |
| Cursor/result completeness | A page may exceed the requested size when its final scanned chapter has more matches: all of that chapter's matches are returned before `lastIndex` advances. No same-chapter matches are silently skipped. | `collectContentMatches` caps `perChapterLimit`/`matchLimit` while setting `lastIndex` to that chapter, so the next request begins at the following chapter and permanently loses the remaining matches in a dense chapter. | `must-fix` | Dense single-chapter fixture must return all matches up to an explicit visible safety cap, mark any safety truncation, and never claim the cursor can recover skipped matches. |
| Cancellation | Browser disconnect stops subsequent upstream chapter work. A partial response is not represented as a successful completed page. | Modern `/books/:id/search` calls `loadChapterText` with `context.Background()`, so a closed dialog/request can still keep fetching remote chapters. | `must-fix` | Cancelled request fixture proves no later remote chapter is requested and no successful stale result is delivered. |
| Search matching/result fields | Upstream searches raw chapter text in source order, returns chapter title/index, match ordinal, excerpt/result text, and position data which Reader uses to locate the result after its target chapter has loaded. | OpenReader adds case-insensitive/punctuation-normalized matching and per-book route metadata (`chapter`, `line`, `match`, `percent`, `q`). These are acceptable only if result order/ordinal and post-load locating remain stable. | `acceptable-change` | Exact, normalized, multi-match and cross-chapter fixtures; result selection loads the target chapter then highlights the requested occurrence. |
| Errors and unavailable chapters | Missing book/keyword/end are visible user messages; a chapter that cannot provide content does not corrupt the cursor for following chapters. | Modern API has normal REST status codes, but unavailable remote chapters currently collapse into an indistinguishable empty result. | `must-fix` | A partially unavailable scan reports a client-safe incomplete-search state; an all-unavailable scan must not show a false “没有匹配内容”. |
| Legacy compatibility | `/reader3/searchBookContent` accepts GET/POST URL/bookUrl aliases and returns `isSuccess/data.list/lastIndex`. | OpenReader preserves both methods as an adapter. | `aligned` adapter | Keep existing legacy route tests while modern-route tests cover current UI. |

Implementation constraints:

1. Preserve the authenticated per-book modern route and the legacy URL adapter; do not move the dialog back into Reader or a mobile drawer.
2. Carry the request context through remote chapter reads and stop scheduling after cancellation. Do not turn a client disconnect into a `500` response.
3. A resource protection cap is allowed, but it must be explicit in the response/UI (`truncated`/unavailable status) rather than silently advancing the cursor over omitted matches.
4. Keep current `chapter`, `line`, `match`, `percent`, and `q` navigation compatibility while verifying a Reader result jump does not toggle mobile tools or click through the dialog.

Implementation record (2026-07-11):

- Modern `/api/books/:id/search` now scans under `c.Request.Context()` and carries cancellation through remote chapter fetches. A cancelled request returns without scheduling a following chapter or fabricating a successful result page.
- The search page threshold is evaluated only after all safe matches of the final scanned chapter are collected, matching reader-dev's cursor semantics. The old request-only `perChapterLimit` is retained as a compatibility input but no longer silently advances past omitted results.
- A 2,000-match per-chapter safety cap is explicit: the API returns `truncated: true` and `incomplete: true`; remote fetch/source failures increment `unavailableChapters` and also set `incomplete`. The root dialog renders these states as a warning instead of “没有匹配内容”.
- The legacy Reader3 URL route keeps its original response shape and gains the same status fields additively inside `data`, so existing clients continue to read `list/lastIndex/hasMore/total` unchanged.
- Required browser follow-up remains: at 1440×900, 390×844 and 360×800, verify result selection loads/highlights the intended occurrence, the warning is visible for an unavailable chapter, and dialog clicks leave the Reader chrome unchanged.

### 2026-07-10 focused audit: Reader App-level BookmarkForm protocol

Authoritative upstream files:

- `web/src/App.vue`: renders one root `BookmarkForm`, receives `showBookmarkForm(bookmark, isAdd, callback)`, and invokes the callback exactly once when the Dialog closes.
- `web/src/components/BookmarkForm.vue`: one `el-dialog`, desktop `dialogWidth`, mini-interface fullscreen; book, author, chapter, and content are readonly while the note is editable. Save validates book identity/content then persists and closes.
- `web/src/views/Reader.vue`: selection action opens the operation confirmation; its bookmark branch builds the reading-position payload and emits `showBookmarkForm(..., true, callback)` rather than writing directly. The callback clears the in-flight selection-create guard.
- `web/src/components/Bookmark.vue`: edit starts the same root `BookmarkForm` protocol with `isAdd=false`; the list manager does not own a nested editor dialog.

| Contract | Upstream behavior | Current OpenReader evidence | Classification | Required test before code |
|---|---|---|---|---|
| Root ownership | `App.vue` owns one `BookmarkForm` instance, separate from the bookmark-management dialog and Reader. | `GlobalOverlayHost` mounts the single `OverlayBookmarkForm`; Reader and `OverlayBookmarks` only invoke `useOverlayStore.openBookmarkForm()`. The former local form and nested editor are removed. | `aligned` | Exactly one `OverlayBookmarkForm` is mounted by `GlobalOverlayHost`; Reader and list actions only open it through the overlay store. |
| Open/close callback | `showBookmarkForm` receives a callback; the root watcher invokes it once after either save or cancel/close, releasing Reader's `showAddBookmarking` lock. | `useOverlayStore.openBookmarkForm()` returns one promise and stores one resolver; `finishBookmarkForm()` clears it before resolving, so save/cancel/close cannot resolve it twice. `useReaderSelection` awaits the form promise while its operation guard is active. | `aligned` | A pending Reader selection creation remains guarded until the global form finishes; confirm, cancel, Escape, and modal close each resolve exactly once without click-through. |
| Create from selected text | Upstream `showTextOperate()` uses the cancel branch for “添加书签”; `showAddBookmark()` resolves the selected paragraph context and opens `BookmarkForm` with the generated book/chapter/content payload. No write happens until the user confirms the form. | The Reader selection branch now calls `useReaderBookmarkActions.createFromSelectedText()`, which opens the root form with the current book, chapter, offset/percent, and trimmed excerpt. No Reader-side API create call remains. | `aligned` for state transition | Selection cancel opens the global form with the current book/chapter/trimmed excerpt; saving performs one create, cancellation performs none. |
| Current-position note | Reader derives current chapter/position/content, then the root form displays the immutable reading context plus editable note. | The Reader note action now opens the same root form. `OverlayBookmarkForm` presents readonly book/author/chapter/excerpt and an editable note; mini mode uses fullscreen. | `aligned` | Reader note action opens the same global form at 1440×900 and 390×844/360×800; readonly context remains visible and mobile is fullscreen. |
| Edit flow | Bookmark list emits the selected bookmark into the root form with `isAdd=false`. The root form edits note content and persists through the same close lifecycle. | `OverlayBookmarks.openEditor()` opens the root form in `edit` mode. The root form preserves title/excerpt as readonly context and uses the existing update API for the note; `useOverlayBookmarkActions` no longer has draft/editor state. | `aligned` | Bookmark-list edit opens the root form; no nested editor remains. Preserve existing title/excerpt data but treat it as readonly context unless a later user requirement explicitly expands upstream editing. |
| Data/API adaptation | Upstream uses global `bookName/bookAuthor/chapterName/bookText/content` fields and a single `/saveBookmark` request. | OpenReader uses authenticated per-book `POST /books/:id/bookmarks` and `PUT /bookmarks/:id` with `chapterId/chapterIndex/offset/percent/title/excerpt/note`. | `acceptable-change` | The global form maps its readonly fields to existing rows and uses the current per-book create/update routes; no backend/data migration. |
| Reader state and route | Upstream Reader takes responsibility for extracting current reading context, while the form does not alter Reader route/progress. | Reader already has `currentOffset`, `currentChapterPercent`, chapter title and visible excerpt helpers. | `technical-stack-equivalent` | Opening/cancelling/saving a form does not change chapter, progress, tool visibility, or Reader route. |
| Modal shell | Upstream uses a root Element dialog at desktop width and `fullscreen` mini mode. Form clicks are modal input, not Reader clicks. | `OverlayBookmarkForm` is a root 640px desktop Dialog and fullscreen mini Dialog. It participates in Reader overlay guards and browser tests verify modal click isolation. | `aligned` | Browser: one desktop dialog and one fullscreen mobile dialog; clicking the form does not page/hide Reader chrome. |

Allowed differences retained for this slice:

- The Go per-book create/update routes, authenticated user isolation, stable numeric `bookId`, and precise reading-progress fields replace upstream’s global bookmark storage.
- Existing stored `title` and `excerpt` fields remain preserved in SQLite and backups, but the upstream-compatible form presents them as readonly context.
- The existing selection confirmation wording is a Vue 3/Element Plus adaptation; its “add bookmark” branch must still open the form rather than write directly.

Implementation order after this audit gate:

1. Completed: root form state and resolve-once close protocol live in `useOverlayStore`; `GlobalOverlayHost` mounts `OverlayBookmarkForm`.
2. Completed: the form uses upstream readonly book/chapter/content context and editable note fields, mapped to existing `createBookmark`/`updateBookmark` API helpers.
3. Completed: Reader note and selected-text bookmark actions open the root form; `ReaderBookmarkFormDialog` and direct Reader create writes are removed.
4. Completed: bookmark-list edit opens the root form; nested editor state is removed from `OverlayBookmarks` and `useOverlayBookmarkActions`.
5. Completed: unit/static contracts plus the real-browser Reader test cover root ownership, resolve-once close, edit save, desktop/mobile geometry, readonly context, no drawer, and click isolation.

### 2026-07-10 BookmarkForm implementation and validation note

- Browser verification now opens a bookmark from the App-level manager, edits its note through the root form, and saves through `PUT /bookmarks/:id` at 1440×900, 390×844, and 360×800.
- The form click remains modal and does not toggle Reader chrome or page content. Opening/cancelling/saving does not alter the Reader route or progress.
- The selected-text create path is covered by unit contracts for “open form, do not direct-write”; the API/form-close branches are the next target if this flow gains a dedicated browser fixture.

## P1 full audit: Index workspace scene convergence

Status: audit completed on 2026-07-10. No application implementation is authorized by this section until its test contracts have been added for the selected implementation batch.

Fixed upstream authority: `web/src/views/Index.vue` and the root dialogs in `web/src/App.vue` at `changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`.

### Upstream scene contract

`Index.vue` is one long-lived product scene, not a group of page routes. Its state owns `isSearchResult`, `isExploreResult`, result rows, result pagination/scroll position, `showNavigation`, source/import/manage Dialog flags, local-store/WebDAV Dialog flags, and the shelf list. Search and Explore replace the shelf list in-place; returning to the shelf clears only result state. Shared BookInfo, Bookmark, SearchBookContent, and BookmarkForm live once at root `App.vue` and are opened by Index/Reader events.

| Concern | Upstream evidence | Required OpenReader behavior |
|---|---|---|
| Scene boundary | `Index.vue` renders the navigation and shelf together; it has no separate Search/Discover/Sources/Settings/LocalStore product routes. | The canonical visible workspace must be one Index-equivalent scene. Existing URLs may remain only as redirects/query compatibility shims. |
| Shelf / search / explore state | `isSearchResult` and `isExploreResult` switch `bookList` from shelf rows to search/explore results. `backToShelf()` clears only result state. `showSearchList()` accepts Explore results in the same shelf area. | Sidebar search and Explore must transition within the workspace, keep the same sidebar/mobile navigation state, and use the shared BookInfo/add/read flow. |
| Search continuation | `searchPage`, `searchLastIndex`, `loadingMore`, and `lastScrollTop` keep continuation in the same list. | Remote/local pagination and query metadata may use Go APIs, but load-more/back-to-shelf must stay in the same workspace list and retain result scroll position. |
| Navigation/sidebar | `.navigation-wrapper` and `.shelf-wrapper` coexist in one flex scene. Desktop width is 260px. Mobile starts hidden, uses 270px drag range, ignores 20px edges/vertical gestures, and shelf clicks close it. | Keep the existing 260px visual width, 270px gesture window, 20px guard, fixed bottom controls, and user-requested bottom-icon drag stabilization. Do not make a navigation action implicitly destroy the current workspace scene. |
| Mobile shelf geometry | At ≤750px the shelf uses 24px title/group insets, 20px title, 10×20px book rows, 84×112 covers. | Retain the previously aligned OpenReader mobile Home geometry when it becomes the Index workspace body. |
| Source operations | Index sidebar opens source management/import/remote/failure/debug Dialogs; Explore is an in-scene popover. | Source management remains a workspace overlay responsibility. Full-page source settings may only survive as compatibility entry redirects. |
| Shelf operations | Index opens book manage, group manage, import, local-store and cache actions in the same scene. | Existing global overlays are valid Vue 3 equivalents; avoid a second full-page operation flow. |
| User/WebDAV/RSS/replace operations | Index sidebar opens user-space, WebDAV, backup/cache, RSS and root dialogs without leaving the workspace. | Preserve OpenReader’s secure multi-user and Go backup/WebDAV APIs, but surface them as workspace overlays rather than canonical pages. |
| Shared BookInfo | Cover click emits one root BookInfo Dialog; search/explore and shelf reuse it. | Retain the existing shared `OverlayBookInfo` implementation and old `/books/:id` redirect; remove remaining page-specific preview/action ownership as each flow converges. |

### OpenReader mapping and classification

| Layer | Current evidence | Difference from upstream | Classification |
|---|---|---|---|
| Workspace shell | `AppLayout.vue` already owns the persistent sidebar, recent reading, search settings, bottom GitHub/theme controls, cache/user/WebDAV actions, and `GlobalOverlayHost`. | It is structurally capable of being Index, but renders a route slot rather than owning one workspace scene. | `partial` |
| Canonical routes | `router/index.js` exposes `/`, `/search`, `/discover`, `/sources`, `/settings`, and `/local-store` as separate business pages. | This is the central information-architecture split absent from upstream. | `must-fix` |
| Sidebar search | `useAppSidebarSearch` stores search preferences, but `goSearch()` navigates to `/search`; `AppLayout.runNavAction()` closes mobile sidebar after every route/action. | Search is no longer an in-place shelf transition and mobile sidebar/workspace state is reset by navigation. | `must-fix` |
| Explore | `Home.vue` routes `书海` to `Discover.vue`; sidebar Explore also routes to `/discover`. | Upstream Explore is an Index popover/result state sharing the shelf body. | `must-fix` |
| Search / BookInfo / read | `Search.vue` and `Discover.vue` already reuse `overlay.openBookInfo()` plus shared add/read actions. | Shared BookInfo is good, but result orchestration/data state is duplicated across separate pages. | `partial` |
| Source manager | `Sources.vue` is a full page; `AppLayout` links sidebar source actions to its route/query panels. | Upstream source manager/import/failure/debug live as Index-owned dialogs. | `must-fix` |
| Book manage / groups / import | `GlobalOverlayHost` already hosts BookManagement, BookGroups, BookImport, LocalStore, WebDAV, backup, user, RSS, replace-rule and BookInfo overlays. | Overlay ownership is close to upstream; duplicate full-page LocalStore and Settings routes remain. | `partial` |
| Local store / WebDAV | `OverlayLocalStore`/`OverlayWebDAV` exist, while `/local-store` and part of `Settings.vue` provide additional full-page flows. | Two visible entry structures can drift and do not match one Index scene. | `must-fix` for canonical ownership; `acceptable-change` for Go filesystem/security APIs |
| Settings / user space | `Settings.vue` is a standalone page; sidebar account action navigates to it. Reader-specific settings are already separate as required by upstream Reader. | Index-level user/config/backup management must become workspace overlays; Reader settings are not part of this P1 merge. | `must-fix` for workspace settings; `out-of-scope` for Reader settings |
| Mobile sidebar interaction | `useAppMobileNavigation.js` uses a 260px visual width and 270px drag limit. `AppLayout.vue` keeps bottom controls outside the scroll container and counter-transforms them during drag. | Matches upstream mechanics, with the user-requested fixed bottom-icon behavior as an explicit difference. Route-driven close behavior still conflicts with one-scene ownership. | `aligned` for geometry/gesture; `must-fix` for scene transition semantics |
| Mobile shelf body | `Home.vue` now carries upstream 24px/20px/10×20px/84×112 geometry and a mobile menu trigger. | The visual shelf body is already suitable for reuse as Index content. | `aligned` |
| Data/API | OpenReader uses Pinia, authenticated Go APIs, numeric ids, browser cache, multi-user progress and URL query contracts. | Different from Vuex/event bus/global JSON storage but required by the current runtime. | `acceptable-change` |

### Canonical ownership target

| Capability | Target owner | Legacy compatibility |
|---|---|---|
| Shelf, search results, Explore results | One `IndexWorkspace` body rendered from `/` under `AppLayout` | `/search` and `/discover` redirect to `/` with explicit query/mode; preserve keyword/source/search-type query fields. |
| Source list/manage/import/remote/failure/debug | `GlobalOverlayHost` source overlays opened by `AppLayout`/Index actions | `/sources` redirects to `/` with a source overlay intent query. |
| Local store, WebDAV, backup, account/user/RSS/replace | Existing global overlays | `/local-store` and `/settings` redirect to `/` with an overlay intent query; preserve only documented panel parameters. |
| BookInfo/add/read | Existing shared `OverlayBookInfo` and reader route | `/books/:id` remains the existing `?bookInfo=<id>` compatibility shim. |
| Reader | `Reader.vue` remains its own scene | `/books/:id/read` and current reading query semantics remain unchanged. |

### Implementation batches and gates

1. **P1-A — Workspace state contract, no visual migration yet.** Extract a shared Index workspace store/composable for `mode = shelf|search|explore`, keyword, result rows, continuation cursor, and list scroll restoration. Existing Search/Discover pages must mirror their resolved result state into it. No route is removed in this batch.
2. **P1-B — Shelf/search/explore body convergence.** Move Search/Discover result rendering into the canonical `/` workspace body while reusing their current API clients and shared BookInfo actions. Convert `/search` and `/discover` into compatibility redirects. Real-browser gates: sidebar search → result → BookInfo → add/read → back to shelf at 1440×900, 390×844, 360×800.
3. **P1-C — Source workspace convergence.** Extract Sources page actions into overlays, route sidebar actions directly to those overlays, and turn `/sources` into an intent redirect. Verify import/remote/failure/debug preserve current source API fields and no mobile sidebar click leaks through.
4. **P1-D — Operations/settings convergence.** Canonicalize LocalStore/WebDAV/backup/account/user/RSS/replace overlays; convert full-page legacy routes into intent redirects. Preserve multi-user permissions and data directories; no database migration.
5. **P1-E — Final workspace regression.** Verify one BookInfo, one import flow, one source flow, one local-store/WebDAV flow, mobile drag/toolbar/bottom controls, and full old-link compatibility before each Docker release.

### Required pre-implementation tests for P1-A

- Unit contract for workspace mode transition: `shelf → search → explore → shelf`, preserving the workspace search configuration and query-compatible intent fields.
- Unit contract that result pagination/scroll state remains in the same workspace mode rather than a new route component.
- Static contract that canonical `/` owns the workspace body while legacy `/search` and `/discover` are only redirects after P1-B; do not add this assertion until the migration batch begins.
- Existing mobile sidebar tests must continue to prove 260px visual width, 270px gesture window, 20px edge guard, fixed bottom controls, and workspace click close.
- Browser fixture design for P1-B must mock shelf/search/explore/add/read APIs and check no horizontal overflow at 390×844 and 360×800.

### P1-A implementation record (2026-07-10)

- Added `frontend/src/stores/indexWorkspace.js`: a route-independent Pinia representation of upstream `isSearchResult`, `isExploreResult`, shared result rows, page/`lastIndex` continuation, loading state, result scroll position, and return-to-shelf reset semantics.
- The legacy `Search.vue` and `Discover.vue` pages now mirror their completed request state into this store; `Home.vue` applies the upstream result-reset when the shelf scene is entered. Their existing API calls, BookInfo actions, layouts, and URLs remain unchanged in this batch.
- Tests lock the `shelf → search → explore → shelf` transitions, continuation/scroll semantics, route-independent store boundary, and legacy-page adapter ownership. Full frontend regression and production build pass.
- Deliberately unfinished: sidebar search and Explore still navigate to legacy pages. Replacing those navigation transitions with the canonical `/` scene, rendering the shared results there, and converting `/search`/`/discover` to redirects are P1-B work; this record does not claim those behaviors are aligned yet.

### P1-B implementation record (2026-07-10, browser gate passed)

- Canonical `/` now owns all three Index body modes. `Home.vue` renders the shelf, embedded Search, or embedded Explore from the shared `indexWorkspace` state without changing the `AppLayout` sidebar or remounting a separate route scene.
- `/search` and `/discover` are compatibility redirects to `/?workspace=search` and `/?workspace=explore`; all existing query parameters are retained. Returning to the shelf, branding, and the sidebar shelf action clear only the workspace compatibility parameters so refresh cannot reopen an obsolete result scene.
- Sidebar searches call a workspace callback rather than `router.push`; the sidebar remains in its current mobile state for search. Explore intentionally carries `closeMobile: true`, matching upstream's Explore-trigger close behavior. The shelf is still the only ordinary workspace click that closes the mobile sidebar.
- Search and Explore retain their existing Go API clients, shared BookInfo dialog, add/read actions, result deduplication, and source selection logic. A request revision makes a second sidebar search while already on the Search body refresh the existing component instead of requiring a remount.
- Added `scripts/smoke/index-workspace-contract.mjs` for `/search` and `/discover` compatibility redirects, sidebar second-search, BookInfo → add-and-read, Explore, shelf return, and 1440×900 / 390×844 / 360×800 overflow checks. Its syntax is validated.
- Non-browser validation passed: frontend 307 tests, frontend production build, backend `go test ./...`, `git diff --check`, and smoke script syntax validation.
- Real-browser execution passed: `index-workspace-contract.mjs` covered old-link redirects, a second sidebar search in the same scene, BookInfo → add-and-read, Explore, shelf return, and horizontal-overflow checks at 1440×900, 390×844, and 360×800. The existing `index-mobile-sidebar-contract.mjs` also passed at 390×844 and 360×800, confirming the 260px width, 270px drag range, fixed bottom controls, and shelf geometry remain intact.
- P1-B is therefore suitable for a local Docker release and user verification. P1-C/P1-D remain separate work: canonical source, local-store, WebDAV, user-space, and settings overlays have not yet been converged.

## P1-C full audit: source-management workspace convergence

Status: audit completed on 2026-07-10. This section is an implementation gate: no source-management application code changes are allowed until the listed state, route, and browser contracts are added.

Fixed upstream authority: `web/src/views/Index.vue` methods `uploadBookSource`, `onSourceFileChange`, `loadRemoteBookSource`, `saveSourceList`, `showFailureBookSource`, `debugBookSource`, `editBookSource`, and the source import/manage Dialogs at `changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`.

### Upstream source workspace contract

| Concern | Upstream evidence | Required OpenReader behavior |
|---|---|---|
| Ownership | The `Index.vue` sidebar opens `showBookSourceManageDialog`; it does not navigate to a Sources route. | Canonical source management must be a workspace overlay opened from `AppLayout`/Index, while `/sources` survives only as an intent redirect. |
| Main manager | A single dialog owns grouped source filtering, source table, paging, source-to-shelf usage protection, add/edit, export, clear, restore-default, and batch selection. | Preserve one controller and one visible manager. Existing current-page table/mobile cards may be retained as a Vue 3 equivalent only after they are rehomed into the overlay. |
| Local import | File input parses JSON; valid entries open a selection preview. User confirms selected rows. Invalid input shows a source-file error. | Preserve file selection/preview/confirmation, JSON shapes, source tags, cancellation and reload broadcast. Do not directly import a file without the preview selection. |
| Remote import | Prompt remembers the last remote URL, fetches remote JSON, then opens the same selection preview as local import. | Keep one remote URL dialog and the same preview/confirm transaction. Persisting the last URL per authenticated user is a compatible current-runtime adaptation. |
| Failure detection | Upstream obtains invalid-source errors, opens the same manager in failure view, and supports disabling/removing failed rows. | Keep current bounded batch test and health summary as a security/runtime improvement, but surface it inside the source overlay rather than a route page. |
| Debug | Upstream opens a separate source-debug page. | The current in-overlay three-step debug dialog (search → TOC → content) is an allowed improvement if it keeps source rules and request results unmodified and opens without leaving Index. |
| Mobile | Upstream source dialogs are fullscreen under collapsed/mobile UI; side actions do not create a second product page. | Source overlay is fullscreen/appropriate mobile popover and must consume clicks. Source manager opening does not implicitly close the Index sidebar unless the specific upstream action does. |
| Events | Successful save/import/default/restore causes source list refresh. | Keep `sources_update` WebSocket event and `openreader:sources-update` browser event with debounced reload; no stale list after an overlay transaction. |

### Current OpenReader mapping and classification

| Layer | Current evidence | Difference from upstream | Classification |
|---|---|---|---|
| Canonical UI | `frontend/src/views/Sources.vue` is a full application page and `AppLayout.vue` routes sidebar actions to `/sources` with `panel`/`action` query fields. | Separates source management from the Index workspace. | `must-fix` |
| Controller/UI capability | `Sources.vue` already owns add/edit, grouped search, paging, selection, usage guard, batch actions, defaults, health state, file/remote preview and three-step debug. | The capability is richer but entangled with page route lifecycle. | `partial`; extract/rehome, do not duplicate |
| Transfer transaction | `useSourceTransfer.js` provides one local/remote preview and selected-source save path. | Functionally close to upstream and already avoids direct blind imports. | `aligned` |
| Source editor | Current editor exposes compatible rule fields and text replacements in a responsive drawer. | Upstream JSON editor is less structured. Vue form/drawer is a user-safety improvement. | `acceptable-change` |
| Health/failure | Current `/sources/batch-test` performs bounded concurrent tests and returns explicit rows; `failedOnly`/disable-failed are page controls. | Upstream failure state is less structured. | `acceptable-change` in API/runtime; `must-fix` for overlay ownership |
| Debug | Current `/sources/:id/test`, `/test-chapter`, `/test-content` dialog is more guided than the upstream external debug page. | Same parser semantics need preserved; navigation is intentionally improved. | `acceptable-change` |
| Backend persistence | `models.BookSource`, `backend/api/sources.go`, backup restore and source-update broadcast retain legacy `bookSource*`, rule, header, group, default and import shapes. | Go/SQLite/multi-user authorization differs from upstream JavaScript/local storage. | `acceptable-change` and must retain |
| Legacy URL | `/sources`, `/sources?action=import`, `/sources?panel=remote`, `/sources?action=health`, `/sources?action=debug` are live routes. | They need to become root-workspace intents without discarding query compatibility. | `must-fix` |

### API and data contract to retain

| Operation | OpenReader method/path | Required side effect/error semantics |
|---|---|---|
| List/create/update/delete | `GET/POST /sources`, `GET/PUT/DELETE /sources/:id` | Source-edit permission for writes; list includes `usedBookCount`; deleting a used source returns a conflict/guard rather than destroying referenced books. |
| Defaults | `GET /sources/default`, `POST /sources/default/save`, `POST /sources/default/restore` | Save writes the current compatible source snapshot; restore is transactional replacement and broadcasts `restore-default`. |
| Batch | `POST /sources/batch` | Supports `enable`, `disable`, `delete`, `group`; caps ids, reports `affected`/`skippedUsed`, and broadcasts. |
| File import/export | `POST /sources/import`, `GET /sources/export?sourceIds=` | Accept legacy array, `bookSources`, `sources`, or single-source JSON; export retains legacy field names/rules and source ordering. |
| Remote import | `POST /sources/remote-preview`, `POST /sources/remote` | Preview is read-only; only confirmed import mutates data and broadcasts `remote-import`. Remote fetching remains bounded by engine policy. |
| Health/debug | `POST /sources/batch-test`, `POST /sources/:id/test`, `/test-chapter`, `/test-content` | Health bounds timeout and concurrency; debug must return parser data/error without changing the source. |
| Sync | `sources_update` WebSocket → `openreader:sources-update` | Overlay and sidebar source caches invalidate/reload without a route reload. |

### P1-C canonical target and migration batches

1. **P1-C1 — Overlay state and controller extraction.** Move the reusable manager body and its controller from `Sources.vue` into a shared source-manager component/composable. Add a single `overlay.sourceManageVisible` plus `overlay.sourceManageIntent = manage|import|remote|health|debug`; no API or parser changes.
2. **P1-C2 — Index entry convergence.** Change all source sidebar items to overlay actions. The root workspace remains mounted, source operations do not route away, and `/sources` query variants redirect to `/?overlay=sources&sourceAction=…`.
3. **P1-C3 — Lifecycle/mobile regression.** Retire the full Sources page structure, preserve the source editor/import/remote/debug nested interactions, and verify mobile fullscreen/pointer isolation, source-update reload and legacy URLs.

### Required pre-implementation contracts

- Unit state contract for source-manager intent replacement, close/reset behavior, and route-intent normalization (`manage`, `import`, `remote`, `health`, `debug`).
- Static route contract: canonical `/` owns source overlay intent after P1-C2; old `/sources` fields map one-for-one and preserve unrelated query parameters.
- Reuse/update source transfer tests so local and remote imports always preview before mutation and keep selected-source counts/tags.
- Preserve backend API tests for legacy import/export, default snapshot restore, used-source deletion guard, batch bounds, remote preview/import, health timeout/concurrency, and three-step debug. No backend API rewrite is authorized in P1-C unless a documented contract gap is found.
- Real-browser smoke: desktop 1440×900 and mobile 390×844/360×800 sidebar source-management action → overlay → import/remote/health/debug intent → close → same root route; assert no pointer leakage, no horizontal overflow, and source-update refresh.

### P1-C implementation record

Status: implemented and validated on 2026-07-10.

- **C1 — shared controller ownership.** The former route-owned `Sources.vue` has been moved to `frontend/src/components/workspace/SourceManager.vue`. `OverlaySources.vue` is the only host and owns the single Pinia state pair `sourceManageVisible` / `sourceManageIntent`. Reopening with another intent replaces the intent; closing resets it to `manage`.
- **C2 — Index convergence and old URLs.** Every AppLayout source action opens that shared overlay. `/sources`, `/sources?action=import|health|debug`, and `/sources?panel=remote` now redirect to the root workspace with `overlay=sources` and a normalized `sourceAction`, retaining all unrelated query keys. Closing the overlay removes only these intent keys.
- **C3 — lifecycle and mobile behavior.** The manager is full-screen on compact screens, remains in the same root-workspace scene, receives `openreader:sources-update` reload events, and does not close or receive clicks through to the mobile sidebar. Local and remote import continue to use a selection preview before any mutation; health and three-step debug remain nested manager dialogs.
- **Allowed differences.** The current Vue 3/Pinia dialog, structured editor, bounded health test, guided in-overlay debug, Go/SQLite authorization, and user-scoped persisted remote URL are retained runtime/safety improvements. No book-source API, parser, database, or backup contract changed in P1-C.
- **Evidence.** Unit contracts cover resettable intents, one shared host, old-route normalization, and all Index actions. `scripts/smoke/source-workspace-contract.mjs` passed at 1440×900, 390×844, and 360×800: legacy remote/import/health/debug intents, manager close/query cleanup, mobile sidebar persistence and no click-through, import preview/confirmation, source-update reload, and no horizontal overflow. Full frontend tests (311), production build, and backend tests also pass.

## P1-D full audit: BookInfo and shelf-operation convergence

Status: audit completed on 2026-07-10. This is a compatibility gate: implementation begins only after the listed controller, API, and browser contracts are added or updated. The authority is `web/src/views/Index.vue` (`toDetail`, `addBookToShelf`, `saveBook`, `deleteBook`, `showBookManage`, `showManageBookGroup`), `web/src/components/BookInfo.vue`, `BookManage.vue`, and `BookGroup.vue` in `changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`.

### Upstream BookInfo and shelf-operation contract

| Concern | Upstream behavior | Required OpenReader behavior |
|---|---|---|
| Ownership | `Index` and reader invoke one global BookInfo dialog through `showBookInfoDialog`; it is not a product page. | All shelf, search, explore and reader entry points must retain one global BookInfo overlay and one current-book state, with old `/books/:id` links only acting as compatibility intents. |
| BookInfo content | Cover, title, tags/kind, author, origin, latest chapter, follow switch, group summary, intro and local-book refresh are in one dialog. Cover editing, follow and group controls appear only for a shelf book. | Preserve the same visibility gates and order. Extra word count, progress and browser-cache rows are acceptable only when they do not replace or hide the upstream fields. Plain-text intro rendering is a required XSS-safe adaptation of upstream HTML rendering. |
| Add to shelf | A searched/explored book opens BookInfo; “加入书架” asks for groups, lets the user cancel, then saves the selected group mask. “加入并阅读” must preserve the same choice before routing to Reader. | Remote add actions must use one reusable category-selection transaction before mutation. A preselected workspace category may seed the choice, but must not silently bypass confirmation or cancellation. |
| Read and edit | Reading routes from the selected shelf record and saved progress. JSON editing requires title, URL and origin, and a non-shelf book must be added before it can be edited. | Keep the existing structured Vue editor as an allowed safer UI, but it must share the same shelf-record/update transaction and never create a second BookInfo flow. |
| Delete | Single and batch deletion require confirmation, delete book metadata plus progress, then reload the shelf. | Keep confirmation and preserve current multi-user transactional cleanup of progress, bookmarks, categories, chapters/cache files and browser-cache invalidation. All live shelf/reader/BookInfo consumers must receive the removal event. |
| Book management | One `书架管理` dialog owns search, selection, per-book information/edit/group/cache/export operations, and batch delete/add-group/remove-group. Desktop remains a table; compact UI becomes fullscreen rather than a separate product scene. | Rebuild the current management Drawer into a root-workspace dialog/fullscreen-mobile overlay while retaining the one controller, current responsive cards and allowed batch safety enhancements. |
| Cache/export | Upstream exposes server/browser cache actions and TXT/EPUB export; server cache uses cancellable SSE. | Retain the current REST/Go bounded cache pipeline only after its visible whole-book/selection/cancel semantics are mapped explicitly. JSON export is an allowed current backup/interoperability extension; TXT/EPUB must remain available. |
| Group management | One BookGroup dialog has a setting mode for a book and a management mode for add/rename/show/delete/drag-sort. A non-empty group cannot be deleted. | Use one category controller and preserve set/manage modes, confirmation/empty guard, visibility and ordering. Current user-scoped many-to-many categories are an allowed data-model adaptation of upstream bit masks, but UI must remain one dialog/fullscreen-mobile overlay. |

### Current mapping and classification

| Layer | Current evidence | Classification and required outcome |
|---|---|---|
| BookInfo host | `OverlayBookInfo.vue` + `BookInfoDialog.vue` + `BookInfoPanel.vue` are the single host used by `Home`, `Search`, `Discover`, `Reader`, and compatibility routes. | `aligned` structurally. Recheck every context action and overlay-close route transition during implementation. |
| Shelf-only fields | Current code limits cover replacement, remote follow, local refresh and group setting by actual shelf membership. Source/category rows refresh from live events. | `aligned`; retain. Plain-text intro and extra stats are `acceptable-change` security/usability improvements. |
| Search/explore add | `Search.vue`/`Discover.vue` inject contextual action closures and call `createRemoteBook` with currently selected category ids. | `must-fix`: add and add-and-read must open a shared group-selection confirmation, including cancel, as upstream does. Remove duplicated category/route closure logic after convergence. |
| Edit ownership | `BookEditDialog` is shared through `OverlayBookInfo`; Home and BookManage open it directly. | `partial`: structured edit is `acceptable-change`, but its preconditions, post-save shelf/reader/BookInfo synchronization and non-shelf prohibition need contract tests. |
| Single/batch deletion | `bookshelf.removeBook` and `batchDeleteBooks` update local shelf/cache state after REST actions. Backend scopes all rows by user. | `partial`: retain the hardened backend cleanup, then add API and browser tests proving progress/bookmarks/categories/chapters/cache cleanup plus active overlay/reader handling. |
| Book management shell | `OverlayBookManagement.vue` is a Drawer with desktop table and mobile card list. | `must-fix`: upstream ownership is a root workbench dialog (fullscreen on compact UI), not a side/bottom Drawer. Rebuild shell only; preserve the current shared controller and safe card/table rendering. |
| Batch cache/export | Current controller adds batch cache/clear/JSON export and uses bounded REST operations instead of upstream SSE cancellation. | `unknown` pending exact cache-state audit: preserve Go resource limits, but either restore an explicit cancel/progress contract or record the bounded behavior as an approved security difference. |
| Group shell and data | `OverlayBookGroups.vue` is a Drawer; `useOverlayBookGroups` has correct set/manage modes, empty guard, visibility, sort and live BookInfo update. Categories are user-scoped rows/many-to-many relations. | `must-fix` for dialog/fullscreen-mobile shell; `aligned`/`acceptable-change` for controller and data model. |
| Backend/API | Go routes map book/category operations to authenticated REST endpoints and broadcasts. | `acceptable-change` architecture, subject to action-by-action response/error/side-effect tests; no schema migration or endpoint rewrite is authorized solely for UI convergence. |

### API/data semantics that P1-D must preserve

| Upstream action | Current API mapping | Required result |
|---|---|---|
| shelf list / refresh | `GET /books`, `GET /categories` | Per-user ordering/progress/category/cache counts; refresh must not merge another user's rows or regress newer local progress. |
| add/update/delete | `POST /books`, `POST /books/remote`, `PUT/DELETE /books/:id`, `POST /books/batch` | Validate shelf ownership and category ids, atomically update book/category rows, broadcast shelf changes, and clean dependent rows/files when deleting. |
| local refresh / follow | `POST /books/:id/refresh-local`, `PUT /books/:id` | Keep local import file and TOC-rule semantics; refresh invalidates reader/browser chapter caches only for that book. Follow toggling changes only `canUpdate`. |
| category set/manage | `PUT /books/:id/category`, `GET/POST/PUT/DELETE /categories`, `PUT /categories/reorder` | Many-to-many user-scoped categories retain empty/delete guards, ordering and visible state; legacy primary `categoryId` remains compatible. |
| source change | `GET /books/:id/source-candidates`, `POST /books/:id/change-source` | Preserve reader’s current-book source switching separately from P1-D shell work; candidate selection must replace catalog atomically and retain title/author rename rules. |
| cache/export | `POST /books/:id/cache`, `POST /books/export`, `POST /books/batch` | Preserve ownership, cache path safety, local/remote distinction, bounded work, and TXT/EPUB compatibility. |

### P1-D implementation batches and mandatory tests

1. **P1-D1 — BookInfo action contract.** Add a shared add-to-shelf category-selection controller and tests for shelf/search/explore/reader ownership, follow/local-refresh/cover gates, add cancellation, add-and-read route, edit synchronization and old detail URL close behavior.
2. **P1-D2 — BookManage shell.** Rehost the existing management controller in an upstream-style dialog/fullscreen-mobile overlay. Test desktop table, mobile fullscreen layout, search/selection, single and batch delete, group add/remove, cache/clear/export and no root-workspace/sidebar disappearance.
3. **P1-D3 — BookGroup shell.** Rehost set/manage modes in one dialog/fullscreen-mobile overlay. Test preselection, save/cancel, non-empty delete guard, visibility, drag order and live BookInfo/shelf synchronization.
4. **P1-D4 — API/data regression.** Expand Go/API contract coverage for category validation, user isolation, transactional delete cleanup, local refresh cache invalidation, update/follow field preservation, batch cache bounds and export formats. Do not alter persistent schemas unless a demonstrated compatibility gap requires a non-destructive migration.
5. **Release gate.** Run front/back full tests, production build, and real-browser checks at 1440×900, 390×844 and 360×800. The browser probe must open BookInfo from shelf/search/explore/reader, exercise add/cancel/add-and-read, both management overlays, and assert no duplicate UI/route transition, no pointer leakage and no horizontal overflow.

### P1-D1 implementation record: BookInfo add-to-shelf transaction

Status: implemented and validated on 2026-07-10.

- Added one global `OverlayBookAddToShelf` dialog and a cancellable Pinia transaction (`selectBookAddCategories` / `finishBookAddCategories`). It is fullscreen on compact UI, seeds the current search/explore category selection, accepts an intentionally empty selection, resolves cancellation without mutation, and closes/replaces an earlier pending transaction safely.
- Added `useBookInfoAddToShelf`, the only remote BookInfo creation transaction. Search and Explore now use it to select categories before `POST /books/remote`; it normalizes ids, prevents cancellation from creating a book, updates the shared shelf only after success, and always clears the per-book action loading key.
- Kept the current workspace category select as a convenience default rather than a silent bypass. Structured Vue controls and the current multi-category relation are allowed Vue 3/data-model adaptations of the upstream group-mask prompt.
- Evidence: five new unit/static contracts cover cancellation, id normalization, failure cleanup, transaction replacement and Search/Explore ownership. The Index browser smoke passed at 1440×900, 390×844 and 360×800: legacy search redirect → BookInfo → add-and-read → category cancel (zero creates) → category confirm (one create) → Reader, followed by sidebar search/explore and overflow checks. P1-D2/P1-D3/D4 remain pending.

### P1-D2/D3 implementation record: BookManage and BookGroup dialog shells

Status: implemented and validated on 2026-07-10.

- **D2 — BookManage.** `OverlayBookManagement.vue` now uses one root-workspace `el-dialog`, with a bounded desktop width and `fullscreen` at the compact breakpoint. The old side/bottom Drawer shell and direction/size coupling have been removed. `useOverlayBookManagement`, the desktop table, mobile cards, search, selection, cache/export operations and batch footer are retained as the one existing management controller rather than duplicated into a route or second scene.
- **D3 — BookGroup.** `OverlayBookGroups.vue` now hosts both `set` and `manage` modes in one root-workspace `el-dialog`, again fullscreen on compact UI. The shared category controller, preselected groups, confirmation/cancel, non-empty delete guard, visibility toggling, drag-sort lifecycle and BookInfo update event remain intact; the previous narrow Drawer shell is removed.
- **Overlay ownership.** `GlobalOverlayHost.vue` supplies the shared compact-mode decision to both dialogs. Opening either workspace tool does not navigate away from `/`, and closing it does not manufacture a second workspace route.
- **Allowed differences.** Vue 3/Element Plus dialogs replace the upstream Vue 2 shell; OpenReader retains responsive mobile cards for book management and user-scoped many-to-many category rows instead of the upstream category bit mask. These are implementation/data adaptations, not separate user flows.
- **Evidence.** `frontend/tests/bookManagementDialogContract.test.mjs` and `frontend/tests/bookGroupDialogContract.test.mjs` lock the single-dialog/fullscreen host contract. Existing `overlayBookManagement.test.mjs` covers selection, category batch changes, cache/clear, delete and export; `overlayBookGroups.test.mjs` covers set/save/cancel, deletion guard, visibility, sort persistence and lifecycle. `scripts/smoke/book-management-dialog-contract.mjs` passed at 1440×900, 390×844 and 360×800: both dialogs open/close in the root workspace without horizontal overflow, both are fullscreen on compact screens, panel clicks do not close the mobile sidebar, and BookInfo opens above BookManage then closes without closing it.
- **Remaining P1-D work.** D4 still must extend Go/API/data coverage for category validation, multi-user isolation, delete cleanup, local-refresh cache invalidation, follow/update field preservation, cache bounds and export formats before the entire shelf-operation module can claim parity.

### P1-D4 audit: shelf-operation API, cache and data lifecycle

Status: extracted on 2026-07-10; no P1-D4 application code is changed by this audit. Authority is `BookController.kt` methods `saveBookGroupId`, `addBookGroupMulti`, `removeBookGroupMulti`, `deleteBook`, `deleteBooks`, `cacheBookSSE`, `deleteBookCache`, `getShelfBookWithCacheInfo` and `exportBook`, plus upstream `BookManage.vue` / `BookGroup.vue`.

| Concern | Upstream contract | Current OpenReader evidence | Classification / required result |
|---|---|---|---|
| Set a book's groups | `saveBookGroupId` rejects a non-positive group mask; `BookGroup.setBookGroup()` rejects an empty table selection with `请选择书籍分组`. | `useOverlayBookGroups.saveBookGroupSetting()` sends an empty `categoryIds` array and `PUT /books/:id/category` accepts it, clearing every group. | `must-fix`: the BookGroup **set** mode must require at least one selected category. This does not change the deliberately separate add-to-shelf category chooser, which may create an ungrouped book. |
| Category ownership / batch mutations | Upstream operates only inside the current user namespace. | Category ids are validated against the authenticated user and category/write operations use transactions; foreign book ids are filtered by `user_id`. | `aligned` for ownership; add explicit contract coverage so a foreign category or book id cannot mutate current-user rows. |
| Single/batch book deletion | Upstream removes the shelf item and deletes that book's user data directory, which contains local source and chapter cache material. | `deleteBookRecords` atomically removes book/category/chapter/bookmark/progress rows, but neither `deleteBook` nor batch deletion removes remote cache files or the user-private imported `library/data/<user>/<book>` archive. Browser chapter entries also survive because the shelf store only drops list/cache metadata. | `must-fix`: after a successful database deletion, remove unreferenced remote cache files, remove only the deleted user's private imported-book directory, and remove local browser chapter cache keys for every deleted id. Never remove shared local-store/WebDAV source files or another user's data. |
| Remote-cache accounting and clear-all | Upstream cached chapters live in the authenticated namespace; deleting cache affects that namespace only. | `GET /cache/stats` and `DELETE /cache` join/reset every remote book in SQLite, return the server cache-root path, and can clear another user's chapter cache state. | `must-fix` multi-user/security regression: statistics and clear-all must be scoped to the authenticated user's remote books, must not return a host path, and must broadcast only the affected user's refreshed shelf state. Physical cache-file removal must preserve a path still referenced by another chapter row. |
| Per-book/batch cache mutation | Upstream server caching is SSE with visible incremental count and client-disconnect cancellation; local browser caching has a cancellable request handle. | OpenReader uses bounded REST (`20` single-book UI chapters, API max `300`; batch max `50` books × `10` chapters) and browser caching is bounded. Cache writes/clears do not broadcast an updated shelf item to other tabs; clear currently writes DB rows/file state incrementally. | `must-fix` for durable per-user event/state ordering; `acceptable-change` only for the bounded REST transport if explicit progress/cancel parity is reintroduced or the final bounded no-cancel behavior is separately approved. First preserve limits and make cache clear transactional before evaluating a cancel job/SSE adapter. |
| Refresh and source change | Rebuilding a catalogue replaces the active chapter/cache view; deleted book data must not become stale user-visible cache. | `refreshBook`, `changeBookSource`, and `refreshLocalBook` delete old chapter rows then write new ones, but leave obsolete remote cache files and superseded local derived content files. Local refresh keeps original source/archive metadata, which is correct. | `must-fix`: capture old chapter cache references, commit the new catalogue first, then safely prune only superseded derived caches. Preserve `OriginalFile`, `chapters.json`, `bookSource.json`, local-store/WebDAV sources, and recoverable bookmark/progress rows. |
| Export | Upstream `exportBook` sends the original local file untouched; remote books export TXT or EPUB. | OpenReader exposes JSON as an allowed backup/interoperability extension and can ZIP multiple outputs, but reserializes a local book through TXT/EPUB rather than returning its original archive/file. | `must-fix` for the single-local-book upstream flow: return the archived original file with a safe attachment name. JSON/multi-book ZIP may remain only as documented OpenReader export extensions. |

Required tests before implementation:

1. Controller/UI: BookGroup set mode with no selected rows must show `请选择书籍分组`, make no request, and leave the dialog/book unchanged; selected multi-category save remains valid.
2. API: category/book id validation rejects foreign ids without changing any row; category batch updates remain atomic and emit the scoped shelf event after commit.
3. API/files: single and batch delete remove the caller's progress/bookmarks/category rows, remote cache file and direct-import `library/data/<user>/<book>` archive, while preserving another user's rows/cache/archive and local-store/WebDAV originals.
4. Browser/store: direct and sync-driven book deletion remove scoped/legacy browser chapter cache entries, not merely the shelf-list cache.
5. API: two users with remote cached chapters receive isolated `/cache/stats`; one user's `DELETE /cache` leaves the other user's cache path/database state intact and does not expose a filesystem path.
6. API: per-book/batch cache clear is transactional from the caller's view and broadcasts only after durable state; concurrent/shared cache-path cleanup never removes a file still referenced by another chapter.
7. API/files: remote refresh, source change and local refresh prune superseded derived cache entries after commit while retaining original imports and recovering progress/bookmark chapter ids where indices remain valid.
8. API/export: remote single TXT/EPUB exports retain current attachment semantics; one local book returns its original stored file; JSON and multi-book ZIP extensions remain bounded and user scoped.

P1-D4 implementation order: (1) write the failing API/browser contracts, (2) create rooted reference-aware cleanup helpers and scoped cache queries, (3) apply post-commit cleanup/broadcasts, (4) restore the local-export and non-empty group-set contracts, then (5) rerun the Docker volume/backup gate.

### P1-D4-A implementation record: ownership, deletion and cache lifecycle

Status: implemented and validated on 2026-07-11. This is the first P1-D4 slice; refresh/source-change cache replacement and server-cache progress/cancellation remain separate required work.

- **Owned request boundary.** Batch and export endpoints now require every supplied book id to belong to the authenticated user. A foreign/missing id produces a current-user `404` before any batch mutation or export occurs; foreign category ids remain a `400` validation error.
- **Post-commit deletion cleanup.** Single and batch deletion capture remote cache references and a direct-import archive while rows still exist, commit SQLite deletion first, then prune only remote files with no remaining chapter reference and only the owner's `library/data/<user>/<book>` archive. This preserves another user's cache/archive and never treats LocalStore/WebDAV sources as delete targets.
- **Scoped server cache.** `/api/cache/stats` and `DELETE /api/cache` now operate on the current user's remote books, omit the host cache directory, clear chapter rows transactionally, remove only unreferenced physical files, and broadcast a current-user shelf refresh after durable state changes. Per-book/batch cache writes and clears now broadcast refreshed shelf items as well.
- **BookGroup and browser cache.** The BookGroup **set** flow rejects empty selections with `请选择书籍分组`, retaining the dialog/book state. Direct, batch, and sync-driven shelf removal clear known scoped/legacy browser chapter-cache keys after server deletion or sync receipt.
- **Local export.** A single archived local book now returns its original source file for TXT/EPUB commands, matching upstream. Legacy/local rows with no safe archived original retain the existing derived TXT/EPUB fallback; JSON and multi-book ZIP stay documented OpenReader interoperability extensions.
- **Evidence.** New Go lifecycle contracts verify cross-user cache isolation, reference-safe cleanup, private archive deletion, original-file export, and foreign-id rejection. Frontend contracts verify group selection and browser-cache invalidation. Full backend tests pass; frontend tests pass (322), production build passes, and `book-management-dialog-contract.mjs` passes at 1440×900, 390×844 and 360×800, including preselection, empty-group rejection, dialog/sidebar coexistence and overflow checks.
- **Remaining P1-D4-B.** The upstream SSE cache progress/disconnect-cancel interaction has not been restored; the current bounded REST behavior remains under review and must not be treated as a completed parity decision.

### P1-D4-B pre-implementation contract: catalogue replacement and cache jobs

Status: extracted on 2026-07-11; no P1-D4-B application code is changed by this contract. Upstream authority remains `BookController.kt#getLocalChapterList(..., refresh = true)`, `setBookSource`, `cacheBookSSE`, `deleteBookCache`, and `BookManage.vue` cache controls.

| Concern | Upstream behavior | Current OpenReader evidence | Required outcome |
|---|---|---|---|
| Remote refresh catalogue | A forced upstream catalogue load replaces the stored list used by future content/cache reads. The cache represents that refreshed catalogue, not a mixture of old/new index records. | `refreshBook` updates/creates by index but never deletes a chapter no longer returned. It also retains `CachePath` while changing a chapter URL, so `loadChapterText()` can serve old cached content for a changed chapter. | `must-fix`: stage the new full catalogue, replace old chapter rows in one SQLite transaction, then post-commit prune every superseded remote cache reference. A failed fetch/transaction leaves the original catalogue and cache readable. |
| Source change | Upstream persists the selected source and rebuilds the catalogue against it; previous source content must not masquerade as content from the replacement source. | `changeBookSource` deletes/creates rows transactionally but does not capture/prune old remote `CachePath` files. | `must-fix`: capture old references before deletion, commit the source/catalogue replacement, clear/prune old remote caches after commit, then publish one refreshed shelf item. |
| Local refresh | Upstream reparses the archived original source on refresh and rewrites its catalogue metadata while retaining the original local import. | `refreshLocalBook` deletes/creates rows in a transaction but writes `content/`, `chapters.json`, and `bookSource.json` during that transaction; old derived content can remain and a transaction failure can leave new files without matching rows. | `must-fix`: stage regenerated derived files below the owned archive, commit chapter/book metadata atomically, atomically promote the staged metadata/content after success, and prune only obsolete derived content. Never delete `OriginalFile`, LocalStore/WebDAV source files, or another user's archive. |
| Progress and bookmarks | Upstream book progress remains a book-level position across catalogue refresh; a changed TOC cannot point at a deleted database chapter object. | OpenReader updates chapter ids for matching local-refresh indices, but leaves a progress/bookmark `chapterId` pointing at a deleted row when its index no longer exists; remote/source replacement does not reconcile them. | `must-fix`: preserve offsets/indices for recoverability, rebind chapter ids only for surviving indices, and clear obsolete chapter-id foreign references without deleting the user’s progress/bookmark record. Reader remains responsible for clamping an out-of-range index. |
| Server cache progress/cancel | `cacheBookSSE` emits incremental `message` counts, terminal `end`/`error`, and stops work when the EventSource disconnects; BookManage uses the active state as a second click to cancel. | OpenReader REST calls are bounded (`20` single UI chapters, API max `300`; batch 50×10), but return only on completion and have no per-book cancel/progress state. | `must-fix`: add an owner-scoped `POST /books/:id/cache/stream` SSE-compatible response that keeps the existing bounds, emits per-chapter progress and terminal result/error, and uses request abort as cancellation. The Vue client uses authenticated `fetch` streaming (not a JWT query parameter), while the active BookManage control becomes a stop action. Batch limits remain an explicit OpenReader extension. |

Required tests before implementation:

1. Remote refresh fixture: old cached chapters `[0, 1, 2]` versus a new TOC whose changed URL at `0` and removed `2` prove the response has exactly the new rows and neither old cache can be returned or remain referenced.
2. Source-change fixture: capture old remote cache files, replace the source/catalogue, then assert post-commit cleanup and a single scoped shelf update; fetch failure leaves old rows/cache untouched.
3. Local-refresh fixture: force a shorter reparsed catalogue, verify original archive survives, old derived content is not active, committed rows/`chapters.json` agree, and unmatched progress/bookmark rows retain their position but have no stale chapter id.
4. Failure fixture: force a derived-file/catalogue write failure and assert the previous local catalogue/content/archive remain usable; no half-promoted state becomes active.
5. Cache-job contract: start, progress, completed, error, and cancel transitions are owner scoped; double-click cancels only that book; a disconnect/cancel stops scheduling new fetches; batch bounds and server resource limits remain enforced.

Implementation order: introduce a reusable staged catalogue/derived-artifact transaction plan, test remote refresh and source-change first, add local staged promotion/recovery, then expose the cache-job state through the BookManage controller and real-browser lifecycle smoke.

### P1-D4-B1 implementation record: remote catalogue replacement and chapter-reference recovery

Status: implemented and backend-validated on 2026-07-11. This is the completed first implementation slice of P1-D4-B; local derived-file staging and upstream cache-job progress/cancellation remain open.

- **Remote refresh and source change.** `POST /books/:id/refresh` and `POST /books/:id/change-source` now stage their fetched remote chapter list in memory, replace all persisted chapter rows in one SQLite transaction, then prune only post-commit cache files no remaining remote chapter references. A changed URL can no longer reuse its old `CachePath`, and a chapter absent from the replacement catalogue cannot remain visible.
- **Progress/bookmark recovery.** Replacement uses a shared in-transaction reconciliation step for `ReadingProgress` and `Bookmark`: matching `chapterIndex` values receive the new chapter id; missing indices retain their index, offset, percent, title/note and other position data but receive `chapterId: 0`. No reading position or bookmark row is deleted merely because a source TOC changed.
- **Local-reference safety.** Local refresh now performs the same reference reconciliation within its database transaction instead of best-effort updates after commit. This fixes stale references even while its derived-cache promotion is still being rebuilt below.
- **Evidence.** `TestRemoteRefreshReplacesCatalogueAndClearsSupersededCaches`, `TestChangeSourceReplacesCatalogueAndPrunesOldRemoteCache`, and `TestLocalRefreshClearsStaleChapterReferencesWithoutDeletingOriginal` start from stale chapter/cache fixtures and verify full replacement, safe cache pruning, original-file preservation and reference recovery. `cd backend && go test ./...` passes.
- **Still required.** `refreshLocalBook` currently writes regenerated `content/`, `chapters.json`, and `bookSource.json` during its database transaction. It must be changed to staged file generation plus rollback-safe atomic promotion, with a forced-write-failure fixture. The current bounded REST cache path also still lacks upstream per-book progress/disconnect cancellation.

### P1-D4-B2 implementation record: staged local catalogue promotion

Status: implemented and parser/API-validated on 2026-07-11.

- **Inactive staging.** Local refresh now writes parsed chapter content to a unique `.refresh-*` staging directory below the authenticated user's private archive (or under the cache root for a legacy local row). The future `CachePath` contains a fresh content generation, so a replacement catalogue can never fall through to the former chapter URL/index cache.
- **Durable boundary and promotion.** The transaction replaces the chapter rows, reconciles progress/bookmarks, saves the book fields, and writes `chapters.json` / `bookSource.json` only inside the inactive stage. After commit, the staged content generation and metadata files are promoted with same-filesystem renames; only then is the shelf item broadcast. Previous private `content/` files no longer referenced by the replacement rows are pruned. `OriginalFile`, LocalStore/WebDAV sources and other users' archives are never promotion or cleanup targets.
- **Failure recovery.** A staged-artifact write failure occurs before the SQLite transaction, removes the stage, and leaves the original archive, live content, `chapters.json`, `bookSource.json`, book row and chapter row untouched. A failed remote fetch/source change likewise still leaves the existing catalogue/cache readable because fetches complete before their replacement transaction starts.
- **Evidence.** `TestLocalRefreshPromotesNewGenerationAndPrunesOldDerivedContent` verifies new cache generation, old-derived-content pruning, metadata/database agreement, and original-file preservation. `TestLocalRefreshStageFailurePreservesActiveCatalogueAndArchive` verifies the forced-failure boundary. Remote/source failure contracts verify no mutation on fetch errors. `go test ./engine ./services/localbook ./api` passes.

### P1-D4-B3 implementation record: streaming cache progress and cancellation

Status: implemented and contract-tested on 2026-07-11; the three-viewport browser rerun with the new SSE click path is pending because the local browser-runner authorization disconnected during launch.

- **Authenticated stream contract.** `POST /books/:id/cache/stream` validates the same owner/bounded request as the legacy REST cache endpoint before opening `text/event-stream`. It emits a per-chapter `message`, terminal `end`, or client-safe terminal `error`. The legacy `/cache` endpoint remains for deployed clients and bounded batch cache operations remain an explicit OpenReader extension.
- **Cancellation boundary.** The stream's request context is propagated into source content fetch and pagination. Browser `AbortController` cancellation or a client disconnect stops before scheduling another chapter fetch, retains only already completed cache files, and deliberately skips a final shelf-update broadcast for the incomplete operation.
- **BookManage interaction.** The current remote book's cache button now becomes `停止 n/total`; activating it a second time aborts only that book's stream. Vue uses authenticated `fetch` SSE parsing rather than `EventSource`, so the JWT is never placed in a URL. A terminal stream error is surfaced through the existing BookManage error path; successful completion merges the returned shelf item.
- **Evidence.** Go contracts cover success/progress/end, owner rejection before stream opening, total source failure/error and cancellation without next-chapter scheduling. Frontend contracts cover SSE framing/error handling plus active-book progress and stop behavior. Full backend, frontend unit and production build gates passed before the browser-runner transport interruption.

## P1-E pre-implementation audit: remaining Index operation routes

Status: implemented and unit/build-validated on 2026-07-11. The three-viewport browser smoke is committed as `scripts/smoke/workspace-operation-contract.mjs`; execution remains pending because the local browser-runner authorization transport was interrupted. This uses the fixed `Index.vue` scene contract already recorded above.

### Current duplicate ownership

| Capability | Upstream Index responsibility | Current OpenReader evidence | Required target |
|---|---|---|---|
| Local book store | Index opens the local-store dialog while the shelf/side navigation remains the same scene. | `OverlayLocalStore.vue` already embeds `LocalStore.vue`, but `/local-store` still renders that same component as an independent page. | `must-fix`: retain one embedded LocalStore body under the global overlay; turn `/local-store` into a root `overlay=local-store` compatibility intent without losing unrelated query keys. No LocalStore path or import API changes. |
| WebDAV and backup | Index opens WebDAV and backup dialogs. | `OverlayWebDAV.vue` and `OverlayBackups.vue` are canonical overlay hosts; `Settings.vue` duplicates their visible backup/WebDAV responsibilities. | `must-fix`: route sidebar and legacy `settings?panel=backup|webdav` to the existing overlays. Preserve mount paths, authenticated APIs, restore effects and the current backup/WebDAV security checks. |
| Account and cache | Index keeps user-space/cache operations in the long-lived workspace rather than replacing it with a settings route. | `Settings.vue` contains account/sync summary, health, remote/browser cache operations and logout. AppLayout already owns cache commands; user management is a separate admin-only overlay. | `must-fix`: extract the account/cache summary into one workspace-settings overlay body. Do not map a normal user's account intent to admin UserManage; retain logout, sync/health and current-user cache scope. |
| Reading preferences | Reader settings are part of the Reader control surface; Index-level navigation must not discard existing preferences merely because legacy Settings was removed. | `Settings.vue` contains a global reader preference editor, while `ReaderSettingsPanel.vue` is the authoritative in-reader control but is not reusable as an Index route replacement. | `must-fix`: extract/reuse the global reader-preference editor inside the workspace-settings overlay, keep its persisted setting keys/defaults/custom assets, and keep Reader's own settings panel independent. `/settings?panel=reader` must be a compatibility intent, not a silent drop. |
| Replace rules, RSS, admin | Index opens root dialogs for replace rules, RSS and user space. | `OverlayReplaceRules`, `OverlayRSS`, and `OverlayUserManagement` already exist; `Settings.vue` only forwards actions to them. | `aligned ownership`: legacy panels must normalize to their existing overlays; preserve admin authorization and no route-page duplicate. |

### Required intent mapping and state transitions

The canonical route remains `/`. Legacy settings/local-store links must preserve unrelated query values and normalize only their operation keys:

| Legacy URL | Root intent | Open/close behavior |
|---|---|---|
| `/local-store` | `overlay=local-store` | Open LocalStore overlay; closing removes only `overlay`. |
| `/settings?panel=backup` | `overlay=backup` | Open Backup overlay; closing removes only `overlay`. |
| `/settings?panel=webdav` | `overlay=webdav` | Open WebDAV overlay; closing removes only `overlay`. |
| `/settings?panel=replace` | `overlay=replace-rules` | Open shared ReplaceRules overlay; Reader may still open the same overlay. |
| `/settings?panel=rss` | `overlay=rss` | Open shared RSS overlay. |
| `/settings?panel=admin` | `overlay=user-manage` | Open shared UserManage overlay; normal users keep the existing backend authorization error/empty state. |
| `/settings`, `?panel=account`, `?panel=cache`, `?panel=reader` | `overlay=workspace-settings&settingsPanel=account|cache|reader` | Open exactly one extracted workspace-settings body. Invalid/missing panel defaults to account. Closing removes only `overlay` and `settingsPanel`. |

AppLayout must hydrate these intents only outside Reader and clear them only after the corresponding overlay closes. Opening an operation from the sidebar must not manufacture or hide a separate workspace route. A mobile overlay interaction must not pass through to close the sidebar unless that action explicitly calls the existing close rule.

### Implementation and validation gates

1. Extract the Settings page sections into reusable bodies without changing persisted reader/preference/cache/backup APIs; retain the page only temporarily as an adapter while tests are introduced.
2. Add `workspaceSettingsVisible` / `workspaceSettingsPanel` and route-intent normalization/cleanup alongside the existing overlay store. Reuse existing LocalStore/WebDAV/Backup/RSS/Replace/User overlay states instead of creating a second business controller.
3. Convert `/local-store` and `/settings` to compatibility redirects only after all eight panel intents are handled. Delete the full-page structures only after every body has a canonical overlay owner.
4. Required tests before claiming P1-E: static router + intent mapping contract; close/reset/preserved-query contract; account/cache/reader preference persistence contract; LocalStore/WebDAV/backup/replace/RSS/admin overlay lifecycle tests; real browser desktop 1440×900 and mobile 390×844/360×800 route intent → overlay → close → root scene with no horizontal overflow or sidebar click-through.

Allowed differences: Vue 3/Pinia dialog bodies, JWT/multi-user authorization, and the user-requested fixed sidebar bottom controls. No SQLite schema, cache root, library/local-store/WebDAV path, backup format, or reader-setting key changes are authorized by this route-convergence slice.

### Implementation record

- **Canonical operation scene.** `/local-store` and every `/settings?panel=…` legacy link now redirect to `/` with a single root overlay intent, preserving unrelated query keys. Account, cache and reader preference panels share one workspace-settings drawer; backup, WebDAV, replace rules, RSS and user management reuse their pre-existing canonical overlays.
- **Duplicate removal.** The previous full-page `Settings.vue` implementation no longer owns backup, WebDAV, RSS, replace-rule or user-management widgets. It now contains only the workspace settings body, while the old URLs remain working compatibility adapters.
- **Close behavior.** `AppLayout` hydrates overlay query intents outside Reader and removes only `overlay` / `settingsPanel` after the corresponding overlay closes. Source overlay ownership remains independent.
- **Manager visibility.** The sidebar now exposes the user-management overlay only when `profile.role === "admin"`, matching upstream manager-mode visibility. The legacy admin intent is retained so a pasted old link still receives the backend’s authoritative 403 for a normal user.
- **Evidence.** Static contracts cover all eight legacy panels, one shared workspace settings controller and absence of a second operation implementation. Frontend unit tests (333), frontend production build and backend Go test suites passed. The pending browser script checks all mapped links and close behavior at 1440×900, 390×844 and 360×800, including mobile sidebar click-through and horizontal overflow.

## P1-E2 pre-implementation audit: workspace storage, backup, RSS and user-space semantics

Status: audited on 2026-07-11 from the fixed upstream baseline `changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`; implementation is now in progress against the contracts below. The table records the pre-implementation evidence and remains the authority for the remaining RSS, parser-bound and browser gates.

### Authority and scene contract

| Upstream authority | Required behavior | Current OpenReader evidence | Difference / priority |
|---|---|---|---|
| `web/src/views/Index.vue` (`showLocalStoreManageDialog`, `showWebDAVManageDialog`, `backupToWebdav`, user-space/cache/RSS entries) | All operations remain in the Index scene. Opening storage or an operation dialog does not replace the shelf scene; backup asks for confirmation; user-management entries are only visible in manager mode. | P1-E now owns the root overlay route intents, but the normal-user sidebar still exposes “加载用户空间”, then lets the protected admin API fail. | `must-fix P1`: gate manager-only entries from `profile.role`; retain backend 403 as the authority. |
| `web/src/components/LocalStore.vue` | Open resets to the root folder; list only the current directory, navigates directories, caps a large result until “加载更多”, supports search, selected delete/import, upload and then opens the shared book-import preview. Importable upstream extensions are TXT/EPUB/UMD/CBZ. | `LocalStore.vue` is an embedded root overlay with breadcrumbs, current-directory default, preview-before-write and a 100 item display cap. Direct unsupported files return a predictable item error; every successful preview stores an immutable user-scoped input token for confirmation. | P1 current-directory and deterministic P2 preview are implemented. `.text/.md/.pdf` remain documented user-requested runtime extensions. Upload/parser byte limits and automatic expiry cleanup are still required. |
| `web/src/components/WebDAV.vue` | Same Index-owned dialog pattern as LocalStore: root on open, current-directory list, directory navigation, selected delete/import/upload, ZIP restore confirmation, and preview-before-add-to-shelf. | `WebDAVBrowser.vue` preserves the browser workflow, directory breadcrumbs, ZIP restore, preview-before-write and richer metadata/mobile layout. Raw `/webdav/*` requires JWT plus `CanAccessStore`; regular users resolve only inside their private WebDAV root and previews use immutable staged input tokens. | P0 security and P2 private-data/preview semantics are implemented. WebDAV upload byte limits and operation browser smoke remain required. |
| `Index.vue#backupToWebdav`, `WebDAV.vue#restoreFromWebdav` | Backup warns before overwrite; recovery asks for confirmation and refreshes the Index data afterwards. | `/api/backup/*` is authenticated, restore is user-scoped at the database layer and `applyRestoreResult` refreshes store/settings/events. Generated files/list/download are now private for regular users; scheduled generation runs per user while the administrator keeps the legacy backup root. | `aligned` for scoped backup storage, with a pending Docker volume/backup gate. |
| `RssSourceList.vue`, `RssArticleList.vue`, `RssArticle.vue` | Source list is ordered by `customOrder`; edit mode gates destructive source actions; opening a source opens an article list; sort tabs follow `singleUrl`/`sortUrl`; article open, image preview and external link are separate modal states. | `RSSManager.vue` preserves source import/edit/delete, rule fields, `singleUrl`/sort semantics, paging, content, image preview and user-scoped rows. It intentionally combines the upstream modal chain into one root drawer with article state. | `technology-equivalent + allowed enhancement`: retain unread/favorite and combined drawer only if source order, article reset, external-link safety and parser request limits stay covered. `P2`: add a browser contract for source → articles → article → close/reset and source order. |
| `Index.vue` user-space actions and upstream `UserManage` | Manager-only create/reset/delete and normal-user data isolation. | Go admin endpoints enforce role, manager navigation is hidden for normal users, and `CanAccessStore` now blocks storage handlers before path/body/file work. | `aligned` for this capability boundary; retain backend enforcement for pasted legacy intents. |
| `BookManage`/import preview flow | Preview parsing supplies book metadata/catalogue before a durable shelf mutation and permits correction/cancellation. | Direct upload, LocalStore and WebDAV use `LocalBookImportPreviewDialog`. Preview tokens now select user-scoped immutable bytes for confirm/reparse and are consumed only after a successful durable import. | The reported source-mutation/network-timing class is fixed. Large upload/archive/parser bounds remain a `must-fix P2` security/reliability item. |

### Data and security finding

The current implementation has two incompatible storage models: database book/setting/RSS rows are scoped by authenticated user, while `LocalStoreDir`, the raw `/webdav/*` tree and generated backup files are global. The latter is not an allowed multi-user adaptation.

The replacement must meet these non-destructive rules:

1. Add an explicit `requireStoreAccess` authorization boundary that loads the authenticated user and returns `403` before any LocalStore/WebDAV/backup file operation when `CanAccessStore` is false. Raw WebDAV must be protected by the same authentication boundary; clients must send the existing bearer token rather than embedding it in a URL.
2. New storage resolves below a user-private descendant of the existing mounted roots, for example `library/localStore/users/<safe-user>/` and `data/webdav/users/<safe-user>/`. Existing `library/localStore/` and `data/webdav/` files are never moved or removed automatically.
3. Preserve a documented legacy-read path for the administrator until explicit migration, and never make an existing shared backup silently visible to a non-admin user. Generated backups and backup list/download/restore must be scoped to their owner.
4. Preserve `data/`, `cache/` and `library/` mounts; no destructive SQLite migration. Any new scope marker must be additive and have a fallback for pre-existing database records/volume paths.
5. Bound multipart upload sizes, archive entry count/expanded size, parser input sizes and per-preview work. Preview records must expire/clean up safely and cannot be accessed by another user.

### Required contracts before implementation

1. **API/security tests:** unauthenticated raw WebDAV is rejected; a store-disabled user gets 403 for every LocalStore/WebDAV/backup endpoint; user A cannot list/download/delete/import/restore user B’s scoped files or backups; traversal/absolute paths and unsafe WebDAV `Destination` still fail closed.
2. **Data migration tests:** existing global LocalStore/WebDAV/backup fixtures remain present after upgrade; an admin can explicitly see/migrate the legacy content; a new user receives an empty private root; Docker mounted volumes survive restart.
3. **Parser/import tests:** preview does not create books; confirm consumes only the exact staged input; same upload retries without a second upload; concurrent/expired/foreign token handling is safe; unsupported direct-file paths produce a deterministic validation error; TXT/EPUB/PDF/Markdown/UMD/CBZ fixtures cover success and parse failure.
4. **Frontend contracts:** default LocalStore is non-recursive with opt-in recursive search; manager-only items are hidden for normal users; LocalStore/WebDAV preview result merges shelf items only after commit; RSS source order, source/article close reset and mobile overlay click-through are covered.
5. **Release gate:** full backend/frontend tests, 1440×900/390×844/360×800 operation smoke, and local Docker volume/backup smoke before publishing an image.

Allowed differences: Vue 3/Pinia component composition; authenticated bearer transport in place of the upstream token-in-download-URL pattern; user-private storage and parser hardening; user-requested extra local formats and import preview controls. No global data deletion, automatic legacy file move, backup format break, or loss of existing LocalStore/WebDAV imports is authorized.

### P0 authorization implementation record

- **Storage access boundary.** `requireStoreAccess` now checks the authenticated user’s existing `canAccessStore` capability before every LocalStore, WebDAV import/restore and backup handler touches paths, files, multipart input or parser work. The raw WebDAV protocol group now uses the normal JWT and activity middleware before that same capability check.
- **Browser transport.** Raw WebDAV browser requests use a root-scoped Axios client that shares the existing Bearer-token/401 handling with `/api`; no token is placed in WebDAV URLs.
- **Evidence.** `workspace_storage_access_contract_test.go` covers unauthenticated raw WebDAV rejection, a permitted user’s normal listing, and 403-before-operation across LocalStore, raw WebDAV, import and backup paths for a store-disabled user. Full Go tests, frontend tests (333) and the production build pass. The workspace browser smoke now also rejects a missing WebDAV Authorization header in its mock.
- **P0 boundary.** This first authorization slice intentionally did not change roots. It is superseded by the scoped-root implementation record below; upload/parser bounds and the real-browser/Docker-volume gates remain required before any storage/backup Docker release.

### P1-E2 implementation record: scoped mounted storage and deterministic preview inputs

- **Non-destructive storage scope.** `storeRoot()` preserves the pre-existing LocalStore and WebDAV roots for administrators, including all already-mounted legacy files. A regular user now resolves only below `users/<safe-username>/` inside those same roots. No SQLite row, volume mount or legacy file is moved or deleted; an administrator can explicitly inspect the preserved legacy root while a regular user cannot list or fetch it.
- **Backup scope.** Manual backup list, download and generation use the same private root. The scheduled backup service now generates a separate user-scoped backup for every persisted user and filters all personal settings, RSS sources, categories, shelf rows, bookmarks, progress and replace rules by that user's id. Global book-source export remains intentionally shared because book-source management is global in the current Go runtime.
- **Stable import snapshots.** LocalStore and WebDAV preview now copy each selected file into the existing user-scoped `cache/import-previews/<user-id>/` stage and return its random token. Confirmation forwards that token through `LocalBookImportPreviewDialog`, reloads only the staged bytes, permits a rule/title/author reparse against those same bytes, and consumes the stage only after success. A deleted or changed source file therefore cannot turn a successful preview into a different import. Tokens are user-scoped, expire after 24 hours, and are removed on success or expired-token access.
- **Predictable direct-file validation.** Unsupported direct file paths return an explicit per-item `unsupported file type` result without calling a parser. Directory imports continue to skip non-importable descendants, preserving the upstream selected-directory workflow and OpenReader's additional `.text/.md/.pdf` support.
- **Evidence.** `workspace_storage_access_contract_test.go` covers permission denial before I/O, legacy administrator roots, normal-user private roots, private backup contents and unsupported direct files. `workspace_import_stage_contract_test.go` covers LocalStore/WebDAV preview snapshots after the original source is deleted plus foreign/expired token rejection. `localBookImportStagingContract.test.mjs` proves the frontend forwards the token.
- **Still required before a storage release.** Archive expanded-size/entry-count and parser-work limits, staged-token cleanup without a subsequent request, RSS interaction smoke, and the 1440×900/390×844/360×800 workspace plus Docker volume/backup gates remain open. Direct/local-book preview/import and LocalStore/WebDAV upload input is capped at `OPENREADER_MAX_IMPORT_BYTES` (128 MiB by default); storage uploads use atomic replacement so rejected files do not truncate an existing source.

## P2 RSS workspace contract

Status: extracted on 2026-07-11 from `web/src/components/RssSourceList.vue`, `RssArticleList.vue`, `RssArticle.vue`, and the RSS entry in upstream `Index.vue`. No RSS application code is changed by this audit section.

| Concern | Upstream state transition | Current OpenReader evidence | Required result |
|---|---|---|---|
| Root source scene | Index opens `RssSourceList` as a workspace dialog. Sources are ordered by `customOrder`; edit mode alone reveals edit/delete affordances; add/import remain root actions. | `OverlayRSS` owns the canonical root drawer. `RSSManager` orders from the API and gates edit/delete behind `rssEditMode`, while preserving a richer editor and source import reconciliation. | `technology-equivalent`: retain the root overlay and richer editor/import flow, but keep one source owner and `customOrder` order. |
| Select a source | Clicking a source opens `RssArticleList`; its `show=true` transition parses `singleUrl`/`sortUrl` then immediately requests page 1 from the selected source. | `selectSource()` only changes `selectedSourceId` and lists already persisted database rows. A source with no prior manual refresh can therefore open as an empty article list, unlike upstream. | `must-fix`: source selection must start a refresh for the selected/default sort and then show the refreshed list; a fetch failure may show an existing cached list as a resilience enhancement. |
| Sort selection | Upstream parses `sortUrl` entries as `name::url`, selects the first entry on open, resets `page=1`/`hasMore=true`, then fetches that sort. `singleUrl=true` always uses the base feed. | `rssSortOptions()` supports both upstream line syntax and an allowed `&&` shorthand; sort change refreshes, but the root/source transition does not uniformly reset article/reader state. | Preserve the upstream first-sort/reset/fetch rule; retain `&&` as an explicit OpenReader import enhancement. |
| Article pagination | Upstream loads page 1, appends later pages, and marks no-more when a page is empty. Closing `RssArticleList` clears sort options, articles, page, and has-more state. | OpenReader has bounded DB pagination (`limit=50`, server max 100), deduped append, unread/favorite filters, and cached server rows. Drawer close does not reset article selection/page/filter; manager stays mounted behind `el-drawer`. | `must-fix`: reset selected source's transient sort/list/page/article/image state on root close and source switch. Unread/favorite filters and server-persisted article cache are allowed enhancements. |
| Article open / content | Upstream fetches content before opening `RssArticle`; image clicks create a gallery from content images. | OpenReader opens one article dialog, fetches/sanitizes content, persists read/favorite state, opens an image viewer, and offers a safe `noopener,noreferrer` external-link action. | `technology-equivalent + allowed enhancement`: dialog may remain inside the root drawer, but a source switch/root close must close it and clear `selectedArticle`; content/image clicks must not leak through to the manager. |
| API/data and errors | Upstream API is source-URL driven and client-cached. | Authenticated Go APIs are user-scoped by source/article IDs, preserve RSS/Atom parsing, `singleUrl`/sort semantics, request policies, HTML sanitization, refresh de-duplication, and sync broadcasts. | `acceptable Go adaptation`: retain multi-user IDs/cache/filters, but frontend requests must reproduce upstream source-open and reset ordering. |

Required pre-implementation tests:

1. Static/controller contract: selecting a source clears the stale article/reader state, resets the first sort/page, invokes a selected-source refresh, then loads the resulting rows; refresh failure still requests the scoped cached rows.
2. Root close contract: article dialog, image preview, selected article, filters, pagination/sort and article rows reset; reopening reloads sources before selecting a fresh default.
3. Source/sort contract: `singleUrl` wins over `sortUrl`; first upstream `name::url` option starts at page 1; `&&` remains documented as the only accepted extension.
4. Browser gate: 1440×900, 390×844, 360×800 — open RSS → source → fresh article → content → image → close → reopen; assert no stale dialog/list state and no workspace/mobile sidebar click-through.

Allowed differences: Vue 3 combined root drawer, persisted per-user article cache, unread/favorite filters, safe external-link button, and `&&` sort shorthand. The source-open refresh, order, close/reset and content/image state contracts are not optional.

### P2 RSS implementation record

- **Root lifecycle.** `OverlayRSS` now supplies its visibility to `RSSManager`, so the manager does not fetch sources while the global drawer is closed. Closing clears the source list, selected source/sort, article rows/page/filter, edit state, article dialog, selected article and image viewer. Reopening reloads the sources before selecting a fresh default.
- **Source and sort transitions.** Selecting a source clears stale article/reader state, resets to the first permitted sort, displays the selected scoped cache and automatically calls the selected-source refresh. Changing sort performs the same reset/cache/refresh sequence. This restores the upstream source-open and sort-first-page behavior while retaining OpenReader's persistent article cache as a fallback.
- **Late response containment.** An article-content request is generation-guarded, and source/article list responses ignore state mutation when the root drawer is no longer visible. A close/source switch can no longer resurrect an old article dialog or its rows.
- **Evidence.** `frontend/tests/rssWorkspaceContract.test.mjs` asserts the root visibility lifecycle, source refresh, sort reset, article/image closure and late-response guards. Frontend tests (333) and the production build pass; the three-viewport RSS browser sequence remains the release gate for this UI slice.

## P2 replace-rule compatibility contract

Status: extracted on 2026-07-11 before implementation. Authority is fixed upstream `web/src/components/ReplaceRule.vue`, `ReplaceRuleForm.vue`, `App.vue`, `views/Reader.vue`, `plugins/config.js`, `src/main/java/com/htmake/reader/api/controller/ReplaceRuleController.kt`, and `src/main/java/io/legado/app/data/entities/ReplaceRule.kt`.

| Concern | Upstream behavior | Current OpenReader evidence | Required result |
|---|---|---|---|
| Root manager and editor | `App.vue` owns one manager dialog and one independent `ReplaceRuleForm`. The manager supports import, enable toggle, edit and selected-row deletion; the editor can also open directly from Reader without opening the manager. | `OverlayReplaceRules` combines a drawer manager and local editor state. Reader opens a short replacement prompt and saves immediately instead of entering the same editor. | `must-fix`: keep the Vue 3 drawer/table as a technical equivalent, but make the full editor a root-addressable, shared scene. Selected text opens that editor directly and must not force the manager open. |
| Form fields and defaults | `defaultReplaceRule` is `{ name: '', pattern: '', replacement: '', scope: '', isRegex: false, isEnabled: true }`. The form requires non-empty name, pattern and scope; add mode rejects a duplicate name. | The current draft defaults scope to `*`; list normalization and a missing `isRegex` both mean regex; save accepts empty name/scope and substitutes the pattern for an empty name. | `must-fix`: new/imported missing-mode rules default to plain text (`isRegex: false`) and enabled; the visible add/edit flow requires name, pattern and scope. Existing persisted rows are not deleted or rewritten merely for this UI correction. |
| Identity and persistence order | `saveReplaceRule` and `saveReplaceRules` replace an existing rule by matching `name`, otherwise append; the stored JSON array order is therefore stable and an edit does not move a rule. | `POST` can create duplicate names; list is `updated_at desc` while content application has no explicit order. Updating a rule can visibly change the replacement pipeline. | `must-fix`: name is the per-user upsert key for add/import; list and content application use one stable insertion order (OpenReader database `id ASC` is the allowed equivalent). No destructive unique-index migration may discard an existing duplicate row. |
| Scope matching | Reader splits `scope` at `;`: `*` or exact book name matches; a second part must equal the exact `bookUrl`. The UI prevents a blank scope. | Go treats empty scope as `*`, and the UI writes `*` by default. | `must-fix` for newly edited/created rules: explicit `*`, book title, or `book title;book URL` scope is required. Empty persisted scope stays a legacy global-scope compatibility case so existing OpenReader data does not silently stop working; it is normalized to explicit `*` on the next successful edit/import. |
| Text replacement semantics | For text chapters only (not EPUB or audio), enabled matching rules run in saved order. Plain text calls JavaScript `String.replace` once. Regex calls `new RegExp(pattern, 'ig')`, replacing all matches case-insensitively. A malformed regex is not treated as a literal replacement. | Go uses the shared parser helper: omitted `isRegex` becomes true; regex is case-sensitive; malformed regex falls back to literal replacement. | `must-fix`: global reader rules need a dedicated upstream-compatible application path: plain first-match replacement; regex global case-insensitive replacement; invalid regex must never fall back to a literal replacement. Go source-rule cleanup remains on its existing parser-specific path. EPUB/audio bypass remains aligned. |
| Test/editor feedback | Upstream has no standalone API preview; the editor is the confirmation point and manager reloads after any successful mutation. | `/api/replace-rules/test` is an OpenReader enhancement, but it currently follows the mismatched parser helper. WebSocket and browser events refresh the Reader chapter after a mutation. | `acceptable adaptation`: retain preview, sync events and explicit REST errors, but preview must use the same plain/regex semantics as the Reader. Reject invalid regex with a client-safe `400` rather than silently creating a different literal rule. |
| Reader selected text | With `selectionAction: '操作弹窗'` (the upstream default), selection opens an operation chooser. Choosing filtering creates a timestamped `文本替换 YYYY-MM-DD HH:mm:ss` plain-text, enabled rule scoped to `book.name;book.bookUrl`, then opens the complete form. `忽略` opens nothing. | Selection mode defaults and chooser/ignore handling exist, but selected text is whitespace-collapsed/capped and filtering opens a replacement prompt that persists the rule immediately. | `must-fix`: preserve the operation chooser and ignored branch, then hand the original selected text and upstream timestamp/scope defaults to the shared editor. Do not persist or broadcast until the editor is saved. |
| Import/delete | Upstream imports a JSON array, saves/upserts each valid named/pattern rule in input order, and deletes selected rules by their identity. | Current normalized import accepts an array or `{ rules }`, plus legacy aliases; batch save/delete is transactional and user-scoped. | `technical-equivalent + allowed enhancement`: retain safer JSON validation, aliases, transaction, IDs and per-user isolation; preserve input ordering, name-upsert semantics and selection-only deletion. |

### API/data contract and implementation gates

The public REST paths remain `/api/replace-rules*` for deployed OpenReader clients. The reader3-compatible upstream actions remain semantics references, not paths to revive. The API translation must be:

| OpenReader route | Required semantics |
|---|---|
| `GET /api/replace-rules` | JWT/current user only; stable `id ASC` pipeline order; return both `enabled` and legacy-readable `isEnabled` during the compatibility window. |
| `POST /api/replace-rules` | Validate name/pattern/scope and plain/regex mode. Upsert by current-user name: `201` for append, `200` when an existing name is replaced in place. |
| `PUT /api/replace-rules/:id` | JWT/current-user ID only; validate the same fields; never reorder the rule. |
| `POST /api/replace-rules/batch` | All-or-nothing input validation, then name-upsert in request order. Report created/updated/skipped without crossing a user boundary. |
| `POST /api/replace-rules/test` | Use the same reader-rule replacement engine. Invalid regex is `400`; output is not saved. |
| delete routes | Preserve the existing ID-based OpenReader paths, current-user isolation, stable deleted-ID reporting and post-commit sync event. |

Required tests before application code:

1. Go contract: default missing `isRegex` is plain; plain replacement changes only the first match; regex changes all case variants; invalid regex does not become a literal match; EPUB/audio bypass; exact title/URL scope; stable ordering survives update and batch upsert.
2. Go API/data contract: add/edit validation rejects missing name/pattern/scope and invalid regex; same-name POST replaces in place; batch is atomic; old user data with blank scope remains readable/global until edited; backup/restore preserves order and fields.
3. Frontend controller contract: empty draft matches upstream defaults; selected-text filtering opens a shared editor draft with timestamp/name/scope but performs no API write before save; cancel performs no mutation; save emits the existing Reader refresh event.
4. Browser gate at 1440×900, 390×844 and 360×800: workspace manager → add/edit/toggle/import/delete; Reader selection → operation chooser → full form → save/cancel; verify manager/editor clicks do not pass through to Reader tools or page turn.

Allowed differences: Vue 3 root overlay instead of Vue 2 App dialogs; ID-backed SQLite rows and JWT/WebSocket synchronization; JSON alias import support; atomic batch writes; explicit REST validation and error responses. The user-visible defaults, scope, execution order, replacement semantics and selected-text editor flow are not optional.

### P2 replace-rule implementation record

- **Reader-compatible pipeline.** Reader-global rules now list and execute in stable insertion (`id ASC`) order. A plain-text rule changes its first matching occurrence; a regex rule changes all case-insensitive matches. EPUB and audio keep the upstream bypass. The unrelated book-source cleanup helper remains unchanged.
- **Safe validation.** New/edit/batch rules require a name, pattern and explicit scope. Missing `isRegex` now means upstream plain text. Regex compilation happens before persistence and preview; malformed expressions return `400` instead of silently becoming literal replacements. New current-user names upsert in place, while an ID edit cannot overwrite a different existing name.
- **Data preservation.** No ReplaceRule schema/volume migration was added. Existing empty-scope rows remain global until edited; nullable old mode rows read as plain text. Backup ordering is now `user_id, id`, so a restored empty database receives the same rule pipeline rather than update-time order.
- **Shared selected-text form.** Reader selection retains the upstream chooser/ignore setting. Choosing filtering now opens the canonical full ReplaceRule editor with a timestamped, enabled plain-text, book-scoped draft; no API write or Reader refresh occurs until that editor is saved. The manager drawer stays closed for this direct-editor flow.
- **Evidence.** `backend/api/replace_rules_contract_test.go` covers defaults, same-name upsert, validation, ordering, plain/regex semantics, legacy scope and backup ordering. Existing API tests cover scoped content and restore compatibility. `frontend/tests/readerSelectedTextActions.test.mjs`, `overlayReplaceRules.test.mjs`, and `readerSelection.test.mjs` cover the editor handoff/defaults. `go test ./...`, frontend `npm test` (335), and `npm run build` pass.
- **Still required before release.** Real-browser manager and selected-text-editor verification at 1440×900, 390×844 and 360×800, followed by local Docker build and mounted volume/backup smoke. Those checks are not claimed complete while the browser/Docker execution authorization remains unavailable.

## P2 bookmark compatibility contract

Status: extracted on 2026-07-11 before implementation. Authority is fixed upstream `web/src/components/Bookmark.vue`, `BookmarkForm.vue`, `App.vue`, `views/Reader.vue`, `src/main/java/com/htmake/reader/api/controller/BookmarkController.kt`, and `src/main/java/io/legado/app/data/entities/Bookmark.kt`.

| Concern | Upstream behavior | Current OpenReader evidence | Required result |
|---|---|---|---|
| Dialog ownership and scope | `App.vue` owns one Bookmark manager and one independent BookmarkForm. Reader opens the manager for its current merged shelf book; the manager filters stored rows by `bookName`/`bookAuthor`, supports import, selection delete, edit and jump. | `GlobalOverlayHost` owns `OverlayBookmarks` plus `OverlayBookmarkForm`; Reader passes the current book and the manager supports the same actions. | `technical-equivalent`: retain the root Vue 3 dialogs and current-book entry. Close/reset and panel click isolation must remain verified. |
| Form fields | Upstream form keeps book name, author, chapter and captured content read-only; only the note is editable. It rejects absent book identity or captured content. | Current global form renders the same read-only context and note editor, but backend direct create can accept an empty excerpt and an arbitrary `chapterId`. | `must-fix`: form stays as-is structurally; API must reject empty context, clamp location values, and verify any supplied chapter belongs to the owned book before persistence. |
| Multiple bookmarks and identity | Upstream JSON controller accidentally replaces a row by matching only `bookName`/`bookAuthor`, despite the manager exposing multiple selection/delete rows. | SQLite uses stable IDs and allows multiple current-user bookmarks per book. | `intentional technical adaptation`: preserve multiple ID-backed bookmarks and never collapse/de-duplicate deployed data. List/import/delete semantics must still present the upstream manager flow. |
| List order | Upstream shows its persisted array order; saving an existing row replaces in place without moving that slot. | Current API orders by `chapter_index, offset, created_at`, which changes visible sequence from insertion order. | `must-fix`: manager/API order uses stable bookmark creation order (`id ASC`/creation order). An edit must not reorder the list. |
| Reader selected-text creation | Upstream trims only for matching, requires selected text to locate one or two full paragraphs, then saves the matched paragraph plus following paragraphs (maximum five paragraphs / roughly 150 characters). If no paragraph can be located, it reports `选择1-2段整段文字才能定位段落` and opens no form. The form receives that context plus chapter/page position. | Current takes any selected string, trims/caps it at 500 characters, and opens the form with that raw selection. | `must-fix`: derive a bookmark context from rendered reader paragraphs using the upstream punctuation-insensitive approximate matching/threshold fallback; preserve paragraph boundaries and the 5-paragraph/150-character capture limit; no form or write when matching fails. |
| Jump to bookmark | Upstream closes the manager, loads the target chapter if needed, then finds the stored paragraph context with decreasing similarity thresholds and scrolls to it. A stale offset alone is not authoritative after content/font changes. | Current closes and routes with chapter/offset/percent only. This is fast but can land on the wrong text after a catalog/content rewrite. | `must-fix`: retain offset/percent as a fast restore adaptation, but carry bookmark context through the reader jump and fall back to the upstream paragraph matcher after the chapter is rendered. Failure shows the upstream-style locate error rather than silently claiming success. |
| Import and persistence | Upstream imports a JSON array with `bookName`, `bookAuthor`, `chapterIndex`, `chapterPos`, `chapterName`, `bookText`, and `content`; controller storage is namespace-scoped. | Current maps legacy fields to an explicitly selected OpenReader book and creates transactional ID rows, then broadcasts a book-specific update. | `acceptable multi-user adaptation`: retain explicit current-book import, aliases, IDs, batching and sync events. Import must preserve legacy chapter/text/note fields and validate every accepted record without crossing users/books. |
| Formats and special readers | Upstream paragraph context/jump is a text-reader algorithm. | OpenReader has EPUB, CBZ and audio additions. | `acceptable adaptation`: text chapters must follow the contract. EPUB/CBZ/audio may retain their dedicated position/resource behavior, but must never fabricate a text paragraph match or let a dialog click page the Reader. |

### API/data contract and required tests

OpenReader retains ID REST routes rather than reviving reader3 paths. The mapping must preserve upstream-visible operations while adding unavoidable user/book isolation:

| OpenReader route | Required semantics |
|---|---|
| `GET /api/books/:id/bookmarks` | Caller-owned book only; stable creation order; returns location plus paragraph context/note without host paths or another user's metadata. |
| `POST /api/books/:id/bookmarks` | Caller-owned book and optional chapter ownership are verified; excerpt/context is required for text bookmarks; chapter index/offset/percent are normalized. Emits only after durable creation. |
| `POST /api/books/:id/bookmarks/batch` | Legacy import aliases are normalized before validation; accepted rows are atomic/current-book scoped and preserve request order. Invalid context/chapter must not leave a partial batch. |
| `PUT /api/bookmarks/:id` | Current-user row only; mirrors the form by allowing note/content edits while keeping reader location/context immutable unless an explicit future migration is documented. |
| delete routes | ID and batch deletion remain current-user/current-book scoped, return stable deleted IDs and broadcast after commit. |

Required tests before application code:

1. Paragraph context utility: punctuation/whitespace-insensitive one/two-paragraph selection finds the original rendered paragraph; captures at most five following paragraphs and 150 characters; unmatched selection returns the upstream error state without a form draft.
2. Reader action contract: selected-text bookmark uses the paragraph context rather than the raw capped selection; direct note/current bookmark uses the current visible paragraph context; EPUB/CBZ/audio keep their dedicated branches.
3. Jump contract: target chapter loads first, context matcher is attempted after render when a saved offset no longer identifies the context, and a failed match emits `无法定位内容所在段落` without toggling Reader tools.
4. Go API/data contract: empty excerpt, foreign chapter ID, negative/out-of-range fields and a mixed-validity batch fail safely; multiple bookmarks remain independent; `id ASC` list/backup/restore order survives edits; no user can list/update/delete another user's bookmark.
5. Browser gate at 1440×900, 390×844 and 360×800: Reader selected text → bookmark form → save; manager → edit/note/import/batch delete/jump; dialog/mobile tool layers coexist and clicks do not pass through to page turning.

Allowed differences: Vue 3 root dialogs, SQLite IDs/multiple-bookmark support, JWT scopes, transactional batches, explicit current-book legacy import, and offset/percent fast restore. The paragraph-context capture/fallback, visible ordering, form validation and text-reader interaction flow are not optional.

### P2 bookmark implementation record (2026-07-11)

- `GET /api/books/:id/bookmarks` now returns a caller-owned book's records in immutable creation/ID order; note edits no longer move a row.
- New and batch bookmark writes require paragraph context, normalize numeric location fields, verify an optional chapter belongs to the caller's book, and validate an entire batch before its one SQLite transaction starts. The editor only changes `note`, leaving a saved reader context immutable.
- Reader text selection now uses the upstream-style punctuation/whitespace-insensitive paragraph matcher. It only opens the bookmark form for a selection matching one or two full rendered paragraphs, and saves a five-paragraph/roughly-150-character context rather than the raw selected string. Text-reader jumps retain offset/percent as a fast path and carry that context for post-render recovery.
- `bookmarks.json` exports in stable creation order. Modern exports restore with their original `createdAt` plus context as the idempotency identity, so independently created bookmarks at one chapter/offset remain separate; legacy rows without timestamps retain a narrower location/content fallback. Restore rebinds a matching current-book chapter ID by its saved index.
- Remaining gate: the Reader dialog/tool-layer browser scenario at desktop, 390×844, and 360×800 must still be run before declaring the bookmark UI slice release-ready.

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
| Theme controls | `ReadSettings.vue` template and scoped style | `阅读主题` uses `.selection-zone`; preset themes are `.theme-item` circles at `34px × 34px`, `margin-right: 16px`, `border-radius: 100%`, selected state reveals a check icon and uses the same `#ed4259` selected color. The custom theme entry is a normal `.span-item` labeled `自定义`, so it keeps the 78×34 button geometry instead of another circle. |
| Custom theme block | `ReadSettings.vue` custom theme block | When custom theme is active, the settings row has left label `自定义`; the right side is `.custom-theme`, and each color/background item is an inline `.custom-theme-title` with `margin-right: 28px` and `margin-bottom: 5px`. |
| Custom background previews | `ReadSettings.vue` custom theme block | Built-in/custom background images are compact `.content-bg-preview` thumbnails at `36px × 36px`; delete icons sit at `top: -6px; right: -6px`; upload is an inline red `上传` text action. |
| Font controls | `ReadSettings.vue` template and scoped style | `正文字体` is rendered in `.selection-zone` with one `.span-item` per font. Each font item is `78px × 34px`, uses `font: 14px / 34px ...`, has `border-radius: 2px`, selected state `border/color #ed4259`, and an upload icon absolutely positioned at `top: -10px; right: -10px`; uploaded fonts mark that icon active in `#ed4259`. |
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
| Theme controls | `ReaderSettingsPanel.vue` uses `.theme-grid`, `.theme-dot`, `28px` desktop dots, `34px` mobile dots, a selected `box-shadow`, and renders the custom theme entry as another circular `+` dot. | Upstream uses `.selection-zone`, `.theme-item` circles, check/moon glyphs, and a rectangular `.span-item` `自定义` button. | `must-fix` |
| Custom theme block | `ReaderSettingsPanel.vue` renders custom theme controls as several separate `.setting-row` entries with labels `页面背景颜色`, `浮窗背景颜色`, `阅读背景颜色`, and `阅读背景图片`; it also adds per-color `恢复默认` buttons. | Upstream groups these under one left label `自定义` and inline `.custom-theme-title` controls. The global `重置为默认配置` remains the upstream reset path. | `must-fix` |
| Custom theme mode | Upstream `config.js` persists `themeType: "day"` in the day defaults and `"night"` in the night defaults. `ReadSettings.vue` exposes `主题模式` with `白天` / `黑夜` only while `theme === "custom"`. `vuex.js#setConfig` forces non-custom, non-night themes to `day`, forces the default night theme to `night`, preserves the explicit value for custom themes, and `getters.isNight` reads `themeType` rather than deriving night state from colors. Current OpenReader has no `themeType` field and derives night state from `theme === "dark" || theme === "black"`. | Add a persisted `themeType: "day" | "night"` reader setting. Preset theme selection must derive the matching type; custom theme selection must preserve its explicit type. Reader/night-shell rendering must read `themeType`. Existing settings and custom configs without the field must infer `night` for `dark`/`black`, otherwise `day`. | `must-fix` |
| Custom background previews | `ReaderSettingsPanel.vue` uses card-like `.bg-image-grid` tiles with 4:3 aspect ratio, overlay text, and large circular delete buttons. | Upstream uses compact inline 36×36 thumbnails with a small top-right delete icon and inline red `上传`. | `must-fix` |
| Font controls | `ReaderSettingsPanel.vue` still uses `.font-family-grid` as a 2/5-column card grid; `.font-family-option` is at least 40/42px tall, padded, rounded 6px, uses card background, and shows `已上传` text plus inline action buttons. | Upstream font choices are compact `.span-item` controls in the same visual system as other settings choices. OpenReader can keep upload/clear capabilities, but the visible option geometry, active color, and upload icon placement should match upstream. | `must-fix` |
| Numeric controls | `ReaderSettingStepper` is used for size/weight/line-height/paragraph/animation/auto-read/TTS. | Preserves user-requested minus/value/plus controls; do not revert to mis-tap sliders. | `intentional-redesign` |
| Mobile gating | `页面宽度` is hidden when `miniInterface`; `左右滑动` is shown only when `miniInterface`. | Matches upstream gating. | `aligned` |

Required tests for this settings-label slice:

| Layer | Test requirement |
|---|---|
| Unit/static | `ReaderSettingsPanel` must expose the upstream canonical labels for theme/font/typography sections. Mobile settings must suppress the generic workspace header so the panel has only one `设置` title row. |
| Unit/static | Mobile settings CSS must keep `.setting-row` in a two-column `72px + content` layout, matching upstream `56px + 16px` label geometry while preserving larger touch controls. |
| Unit/static | Reader settings active/selected CSS must use upstream `#ed4259` and reject the previous blue/teal active colors. |
| Unit/static | Reader settings must not use `el-radio-group` / `el-radio-button`; upstream-style discrete options must use local `.selection-zone` and `.selection-button` controls. |
| Unit/static | Theme presets must use upstream-like `.theme-item` geometry (`34px × 34px`, circular, no selected `box-shadow`), expose a check/moon glyph, and render `自定义` as a rectangular selection button instead of a circular plus dot. |
| Unit/static | Custom theme controls must render as one upstream-like `自定义` setting row with `.custom-theme` and inline `.custom-theme-title` items; separate rows and per-color `恢复默认` buttons should not remain in the reader panel. |
| Unit/static | The custom theme block must expose `主题模式` with `白天` / `黑夜` selection buttons bound to a dedicated `themeType` model. The controls must only render while `theme === "custom"`. |
| Store/data | Reader defaults, synchronized payloads, custom-config snapshots, and sanitized custom-config lists must preserve `themeType`. Missing or invalid old values must infer `night` for `dark`/`black`, otherwise `day`. Selecting a non-custom preset must recalculate the type; selecting `custom` must preserve it. |
| Rendering | Reader and shared shell night-state decisions must use normalized `themeType`, so a custom dark presentation can activate night styling without pretending to be the built-in dark preset. |
| Unit/static | Custom background previews must use upstream-like `.content-bg-preview` geometry (`36px × 36px` inline thumbnails), small top-right delete icons, and an inline red `上传` action; they must not keep 4:3 card overlays. |
| Unit/static | Font options must use upstream-like `.font-family-option` geometry: `78px × 34px`, `border-radius: 2px`, selected `#ed4259`, and upload/clear actions positioned like upstream upload icons at the item top-right. |
| Real browser | Mobile settings smoke must verify theme item, background thumbnail, and font option width/height so later responsive CSS cannot drift back to card-style controls. |
| Regression | Existing reader settings stepper tests must continue passing. |
| Build | Production build must compile after label changes. |

Implementation status:

- Completed in this slice: `ReaderSettingsPanel` visible labels now use upstream canonical text for `阅读主题`, `正文字体`, `字体大小`, `字体粗细`, and `段落行高`.
- Completed in this slice: `frontend/tests/readerSettingsPanelContract.test.mjs` locks those labels and rejects the previous shortened forms.
- Completed in this slice: mobile settings now suppresses the generic `ReaderMobileWorkspacePanel` header, keeps the upstream-like `ReadSettings` title row, and closes via the still-visible mobile settings tool toggle.
- Completed in this slice: mobile settings rows now use upstream-like two-column label/control geometry for the base row layout.
- Completed in this slice: settings active theme dots, background selections, uploaded labels, font options, hover states, and font-size presets now use the upstream `#ed4259` selected color instead of blue/teal.
- Completed in this slice: first-batch discrete options now use upstream-like `.selection-zone` / `.selection-button` controls instead of Element radio groups.
- Completed in this slice: `正文字体` font choices now use upstream-like compact `.selection-zone` geometry with `78px × 34px` options, `#ed4259` selected/upload active states, and top-right upload/clear action placement.
- Completed in this slice: `阅读主题` now uses upstream-like `.selection-zone` / `.theme-item` controls, `34px × 34px` circular preset themes, check/moon glyphs, and a rectangular `自定义` selection button instead of a circular plus dot.
- Completed in this slice: custom background image previews now use upstream-like compact `36px × 36px` `.content-bg-preview` thumbnails, small top-right red delete icons, and an inline red `上传` action instead of card overlays.
- Completed in this slice: custom theme color/background controls now render under one upstream-like `自定义` row with inline `.custom-theme-title` items instead of several separate setting rows.
- Completed in this slice: upstream `主题模式` now maps to persisted `themeType`; custom themes preserve the explicit day/night value, preset themes derive it, and Reader/shared-shell night-state rendering reads it.
- Completed in this slice: settings payloads, built-in and user custom-config snapshots, old-data sanitization, and Kindle temporary state preserve normalized `themeType`; the reader settings version is now `12`.
- Completed in this slice: desktop and mobile custom-theme controls expose `白天` / `黑夜`, and real-browser smoke verifies that switching semantic night mode does not close the active settings/tool layer.
- Pending follow-up: detailed per-control visual pass for mobile `ReadSettings` first-screen density after the base row structure is aligned.

## Required workflow for each future module

1. Use `readerdev-compat-inventory`.
2. Update this file or a focused `docs/compat/*.md` contract.
3. Add/update tests for `must-fix` behavior.
4. Implement OpenReader changes.
5. Run module gate and record allowed differences.
6. Publish Git commits promptly. Publish Docker after any coherent, fully verified slice suitable for user validation; a complete module boundary remains preferred.
