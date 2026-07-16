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
| Frontend scene structure | `web/src/views/Index.vue`, `web/src/views/Reader.vue`; router has `/` and `/reader`. | Canonical Index work stays at `/`; historical search/discover/source/settings/storage/detail URLs now preserve their intent through root-workspace redirects/overlays, while Reader remains a separate scene. | Vue Router has more compatibility routes than upstream, but they no longer create independent product pages. | `aligned` for extracted P1 scene convergence | Router redirect tests; browser flow search → BookInfo → read. |
| Reader mobile toolbar state | `web/src/views/Reader.vue`: `showToolBar: true`; center tap toggles; panel open branches return without hiding toolbar, with a read-aloud-bar exception. | `frontend/src/views/Reader.vue` uses `mobileChromeVisible = ref(true)`; primary panels/global dialogs retain it, while TTS opening hides it and TTS center taps do not retoggle it. | Default-visible toolbar, panel/dialog coexistence and the extracted TTS exception are implemented. | `aligned` for Reader P0 tool-layer states | Unit contracts plus `reader-mobile-contract.mjs` and `reader-tts-contract.mjs` at desktop/390/360. |
| Reader mobile panel structure | Primary shelf/source/catalog/settings use Element popovers; bookmarks/search-content are App-level dialogs; cache is an inline read-bar zone. | OpenReader uses `ReaderMobileWorkspacePanel.primary` for the four primary panels, shared root dialogs for bookmark/search/BookInfo, and an inline cache zone. | The Vue/Element structure differs but ownership, geometry, coexistence and click protection are verified. | `technical-stack-equivalent` | `reader-mobile-contract.mjs` covers all branches at desktop/390/360. |
| Reader mobile content geometry | Upstream mini `.chapter` uses `width: 100vw`, `padding: 0 16px`, `box-sizing: border-box`, `text-align: justify`; slide mode also uses 16px content margins. | Current mobile `.reader-page` uses `width: 100vw`, `padding: 0 16px`, `box-sizing: border-box`, and justified reader body/paragraphs. | Base geometry is implemented; acceptance requires actual rendered paragraph left/right gap checks, not only CSS value checks. | `aligned` for base P0 | DOM geometry probe for page/body/paragraph left/right gaps within 1px across 390×844 and 360×800; ensure toolbar show/hide does not shift content. |
| Reader scrolling vs click paging | Upstream has page/scroll modes with discrete click navigation. | User requested continuous native finger/wheel scrolling while click paging remains segmented. | Intentional UX improvement if it does not change mode selection semantics. | `acceptable-change` | Browser scroll continuity probe; click paging regression tests. |
| Reader settings controls | Upstream uses controls that are easier to distinguish visually; user requested minus/value/plus controls instead of current easy-to-mis-tap slider behavior. | Current setting stepper exists but must be rechecked against upstream layout/state. | Allowed UX adaptation, but values/defaults/state must match upstream. | `acceptable-change` | Unit tests for value bounds; browser setting interaction test. |
| Reader content formats | Upstream `Content.vue` handles text, images/comic-like content, EPUB iframe documents, audio-related branches, read-aloud, and cross-chapter behavior. `EpubFile.kt` additionally treats `BookChapter.startFragmentId`/`endFragmentId` and the next chapter URL as EPUB content boundaries. | Current `ReaderChapterContent.vue` handles text/images/volume blocks, CBZ image resources, EPUB iframe resources, a dedicated audio branch for `type === 1` chapters, and the extracted TTS/read-bar state machine. | E4 keeps the image-only first EPUB spine cover and now preserves NAV/NCX fragment directory entries, signed XHTML slices, exact `(resourcePath, resourceFragment)` matching, same-resource slice navigation and cross-XHTML chapter transitions. CSP/capability protection remains the allowed Go/Vue security adaptation. | `aligned` for E4-EPUB-2 | [`epub-fragment-p1e4-contract.md`](epub-fragment-p1e4-contract.md), parser/API/migration/security tests, and `reader-epub-contract.mjs` at 1440/390/360. |
| BookInfo | Upstream has one `web/src/components/BookInfo.vue` used from workspace and reader flows. | Current has shared `BookInfoDialog.vue` / `BookInfoPanel.vue` / `OverlayBookInfo.vue`; the old `/books/:id` URL redirects to the Index workspace and opens the shared dialog. | The independent `BookDetail.vue` route structure has been removed from the product path; search/discover/route actions are centralized; Reader opens plain BookInfo without injecting toolbar shortcut actions. Remaining P1 work is Index-scene placement and search/discover/source flow convergence. | `partial` for P1 | Single BookInfo action contract; search/shelf/reader reuse tests. |
| Bookshelf/BookManage/BookGroup | Upstream: `BookShelf.vue`, `BookManage.vue`, `BookGroup.vue` under Index workspace. | Current: `Home.vue`, overlay management components, categories/store utilities. | Some enhancements may be valid, but workflow and mobile sidebar behavior need upstream comparison. | `unknown` | Workspace browser flows; category/order tests. |
| Mobile Index sidebar | Upstream sidebar width/drag/fixed bottom buttons are defined by `Index.vue` and related CSS. | `AppLayout.vue` and `useAppMobileNavigation.js` now separate 260px visual width from the 270px gesture window, with bottom controls outside the scroll container. | The user-requested stable bottom controls during drag are an explicit OpenReader UX adaptation; the extracted upstream interaction contract is browser-validated. | `aligned` for extracted P1 sidebar slice | Mobile drag/fixed-bottom/shelf-geometry smoke at 390×844 and 360×800. |
| Search/explore/source flow | Upstream Index integrates search/explore/source and BookInfo transitions. | Root workspace owns Search/Explore bodies and source overlays; historical URLs are compatibility intents, and shared BookInfo owns the handoff. | API clients and OpenReader multi-user extensions remain, but no separate page flow remains. | `aligned` for extracted P1 scene convergence | Search → result group → BookInfo → add/read browser test. |
| Online source parsing | Upstream reader3-compatible source semantics live across `AnalyzeRule` plus `BookList/BookInfo/BookChapterList/BookContent`. | Current Go parser executes the extracted CSS/JSONPath/XPath/regex/composite/replace/pagination subsets, bounded persisted `@put`/`@get` variables, and redacted parser errors. Dynamic headers and `loginCheckJs` now fail before any request rather than being silently ignored. | `{{...}}`/arbitrary JavaScript remain explicit security-gated unsupported behavior; this is not a silent parsing gap. | `aligned` for extracted P2 parser + explicit security difference | Parser/request-isolation and source-debug/error-redaction contracts; browser source flow. |
| Local import catalog parsing | Upstream `BookController.kt` imports local files through `Book.initLocalBook(...)` and `LocalBook.getChapterList(...)`; TXT parsing uses `TextFile.kt` with a 512-KiB detection probe, enabled-rule reverse scoring with a one-match threshold, direct Java multiline matching, `前言`, and deterministic 10-KiB no-TOC pseudo chapters. | Go now probes the first 512 KiB, applies the enabled-rule reverse scoring/one-match semantics, preserves matching custom titles and `前言`, creates upstream-style no-TOC pseudo chapters, and makes upload/LocalStore/WebDAV rule retries reuse immutable user-scoped staged bytes. | Materialized per-chapter cache remains an allowed Go/multi-user adaptation; TXT parsing behavior is aligned for the extracted slice. | `aligned` for TXT P0; `partial` for non-TXT parser audit | Engine/import/API fixtures, frontend retry-state contract, full backend/frontend tests, and mounted-volume smoke before release. |
| Replace rules/content cleanup | Upstream `ReplaceRule.vue`, `ReplaceRuleForm.vue`, `Reader.vue`, `ReplaceRuleController.kt`. | Current Go endpoints and overlays exist. | Default-mode, list/application order, regex flags/failure handling, form validation, manager shell and selected-text editor flow have been rebuilt and verified for the extracted P2 slice. | `aligned` for extracted P2 | Rule-semantics API tests; selected-text editor contract; browser manager/editor smoke. |
| Bookmarks | Upstream `Bookmark.vue`, `BookmarkForm.vue`, `Reader.vue`, `BookmarkController.kt`. | Current ID-backed bookmark APIs and root overlays exist. | Form/manager ownership, paragraph context, stale-offset fallback, creation order and request validation have been rebuilt and verified for the extracted P2 slice. | `aligned` for extracted P2 | Bookmark context/jump/API contracts; three-viewport dialog smoke. |
| RSS | Upstream `RssSourceList.vue`, `RssArticleList.vue`, `RssArticle.vue`. | Current root source dialog, independent article-list/content dialogs, `RSSManager.vue`, overlays and Go RSS parser. | The three-dialog transition, reset/refresh ordering and compact fullscreen behavior have been rebuilt; persistent per-user cache/filtering and sanitization remain allowed adaptations. | `aligned` for extracted P2 RSS | RSS fixture/parser tests; source/article browser smoke. |
| WebDAV/local store | Upstream `WebDAV.vue`, `LocalStore.vue` and server storage behavior. | Current Go endpoints, private mounted-root adaptation and workspace dialogs exist. | P2 storage UI/import audit found CBZ reachability and LocalStore result-gate differences despite the prior path/security alignment. | `partial` | Storage UI/import contract, path traversal tests, upload/list/import browser smoke, Docker volume smoke. |
| Backup/restore | Upstream backup flows and reader-dev formats require extraction. | Current OpenReader backup service and Legado restore exist. | Must preserve OpenReader data and document reader-dev/Legado import semantics. | `unknown` | Restore testdata; backup list/download/restore tests. |
| Auth/user management | Upstream user management components include `AddUser.vue`, `UserManage.vue`; OpenReader adds JWT. | Current JWT/multi-user/admin endpoints are intentional runtime adaptation. | Root dialog, ordinary-user creation, protected-account controls, time metadata and direct legacy-intent behavior have been rebuilt and verified for the extracted P2 slice. | `aligned` for extracted P2 + intentional runtime redesign | Auth dialog and admin/non-admin browser smoke; protected-account API tests. |
| Docker/runtime | Upstream ships Java/Gradle/Docker variants. | Current single Go binary + frontend dist in Alpine, env-driven volumes. Official Node/Go/Alpine base digests are pinned; CA roots are copied from the Go builder and the Go binary embeds IANA time-zone data, so the final stage has no mutable registry/APK package step. | Intentional deployment redesign. The digest pinning and embedded runtime assets are an allowed reproducibility/security adaptation; mounted-volume behavior remains unchanged. | `intentional-redesign` | `PUSH=0 ./scripts/docker-build-push.sh`; `scripts/docker-volume-backup-smoke.sh`. |

## Immediate parser contract: TXT local import catalog rules

Status: **implemented and validated on 2026-07-11.** The previous 2026-07-07 implementation claim was superseded after its scoring, fallback, sampling, preface and matcher omissions were identified. The implementation below is now protected by engine, importer, API and frontend retry-state tests; non-TXT parser audit remains outside this slice.

Authoritative upstream files (fixed baseline `fa22f271849d45f93349ae1636223e27b16a4691`, checked out at `/private/tmp/reader-dev-upstream-audit`):

- `src/main/java/com/htmake/reader/api/controller/BookController.kt`
- `src/main/java/io/legado/app/model/localBook/LocalBook.kt`
- `src/main/java/io/legado/app/model/localBook/TextFile.kt`
- `src/main/resources/defaultData/txtTocRule.json`

| Concern | Upstream behavior | Current OpenReader behavior | Classification and required action |
|---|---|---|---|
| Local read source | Upload preview first copies bytes into the user's local asset path; LocalStore/WebDAV preview opens an already-local path. `LocalBook.getBookInputStream` subsequently reads only the local file. No catalog parsing request depends on network after the bytes are available. | Direct upload and LocalStore/WebDAV flows stage immutable, user-scoped bytes. Token-based preview retries and confirmations read only that staged file, even after a mounted source changes or is removed. | `acceptable-change`: the staged snapshot is a stricter, multi-user-safe equivalent; regression proves preview → failed rule → valid rule → import without the original mounted file. |
| Detection sample | `TextFile.getChapterList()` determines charset and automatic TOC rule from the **first 512,000 source bytes** only, then parses the entire local file with the selected rule. A heading that appears only after this probe does not retroactively enable a TOC rule. | `decodeTXTForCatalog` detects charset and rule from the first 512,000-byte probe, then decodes/parses the whole staged document with that charset. | `aligned` for the extracted TXT behavior; parsing remains over OpenReader's decoded/materialized representation. |
| Enabled rule selection | `getTocRule()` filters enabled default rules, iterates them in reverse, starts `maxCs = 1`, and replaces the selection on `cs >= maxCs`. Thus one matching heading can select a rule, and equal counts follow the upstream overwrite order. It has no generic chapter-title fallback. | `detectTXTTitlePattern` iterates enabled `DefaultTXTTocRules` in reverse with the same one-match/overwrite behavior and returns no generic contender. | `aligned`. |
| Regex matcher acceptance | Once a TOC rule is selected (or supplied by the user), upstream applies that Java multiline regex directly and accepts its matched text as the title. It does not apply an additional 72-rune cap or terminal-punctuation rejection. | Selected/custom TXT matchers retain matching long and punctuation-ended titles; supported Java lookbehind/lookahead normalization and false-positive exclusions remain active. | `aligned` for the enabled upstream constructs; arbitrary unsupported Java regex syntax still returns an explicit parse error. |
| Explicit rule with no matches | `TextFile.analyze(nonEmptyPattern)` returns an empty TOC; `LocalBook.getChapterList` converts this to `TocEmptyException`, and preview reports no chapters rather than silently applying no-TOC splitting. `BookController.saveBook` does not require a local book to have a nonempty TOC. | A nonempty unmatched `tocRule` now returns a normal staged preview with `chapterCount: 0`, `chapters: []` and the original token. Direct, LocalStore and WebDAV flows show a recoverable empty-catalogue notice and still allow confirmation as a zero-chapter local book. | `aligned`: the Go stage token is a multi-user-safe transport adaptation, while the visible and confirmable empty-catalogue state follows the upstream controller. |
| Preface before first detected heading | With a selected TOC rule, any nonblank bytes before the first heading produce a chapter titled `前言`; no length/line-count threshold applies. | `parseTXTText` now emits `前言` for any nonblank preface before the first matching title. | `aligned`. |
| No-TOC fallback | With no selected rule, upstream produces deterministic pseudo chapters from local bytes. It starts at `10 * 1024` bytes and uses the first following newline as the split point; titles use `第{blockPos}章({chapterPos})`. The final tail over 100 bytes is a chapter, while a shorter tail is appended to the preceding chapter (except an otherwise empty TOC still receives one pseudo chapter). | `parseTXTWithoutToc` applies the same 512-KiB block / 10-KiB newline-after split / 100-byte-tail semantics over decoded staged text, preserving contiguous cached chapter content. | `technical-stack-equivalent`: source-encoding byte offsets are intentionally replaced by decoded cached-content offsets. |
| Offsets and content storage | Java stores source-encoding byte offsets and reads each chapter from the original local file on demand. | Go materializes chapter content into per-chapter cache files and records UTF-8 decoded-string offsets in the archive. | `technical-stack-equivalent`: retain materialized cache and immutable archive for multi-user/runtime safety. Tests must require contiguous reconstructed content; raw-offset identity is not required because no OpenReader read path seeks into the original TXT by these values. |
| Empty/short files | `LocalBook` throws `TocEmptyException` only when the format parser returns an empty list; `TextFile.analyze()` normally emits a pseudo chapter even when no TOC is detected. The controller returns an empty chapter list only for a genuine `TocEmptyException`. | Automatic title-less TXT parsing produces the corresponding pseudo chapter; only a nonempty unmatched explicit rule maps to no-readable-chapters. | `aligned` for the extracted fallback/error distinction. |
| Non-TXT formats | EPUB/UMD/CBZ delegate to format parsers; PDF is not a reader-dev local-import format. | EPUB/UMD/CBZ remain format-specific; PDF, Markdown and `.text` remain only historical OpenReader API/archive compatibility formats, not visible workspace imports. | `aligned UI + explicit data compatibility` after E4-PDFMD-1. |

### Validation evidence

1. `backend/engine/parser_test.go` covers one-match enabled-rule detection, the 512-KiB probe, explicit full-document rules, short `前言`, long/punctuation title retention, negative lookaheads, no-match explicit rules, 10-KiB pseudo chunks, short-tail merge, and UTF-8/GB18030 fallback content.
2. `backend/services/localbook/importer_test.go` proves that an explicit unmatched TXT rule produces a normal empty preview and can be archived as a zero-chapter local book without manufacturing a chapter.
3. `backend/api/api_test.go` proves direct preview keeps its import token after an explicit no-match rule, returns `200` with an empty catalogue, then successfully retries and imports the same staged upload.
4. `backend/api/workspace_import_stage_contract_test.go` proves LocalStore and WebDAV token retries continue after the mounted source has been removed, return the empty staged preview without an error, and import the valid retry snapshot.
5. `frontend/tests/overlayBookImport.test.mjs` and `frontend/tests/localBookImportStagingContract.test.mjs` prove the client retains the token and displays the actionable Chinese retry hint instead of a blank/generic import state.
6. `scripts/smoke/local-book-import-contract.mjs` passed against a temporary real server at desktop `1440×900`, mobile `390×844`, and mobile `360×800`: automatic preview, explicit-rule `400`, visible retry hint, disabled import, valid staged retry and re-enabled import. Full backend tests, frontend tests and production build also pass. Docker volume compatibility remains the release gate for this slice.

### Completed implementation boundary

This slice changed only TXT automatic detection, matcher gating, preface/fallback chunking and staged-preview retry handling. EPUB/PDF/UMD/CBZ parsing, uploaded-byte bounds, archive layout, public import routes and existing imported records were not migrated or rewritten.

## Immediate P1 contract: Index mobile sidebar and workspace shell

Status: implemented and browser-revalidated on 2026-07-13 for the extracted sidebar and mobile-shelf slice. The larger Index-scene convergence is recorded separately below.

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
| Title spacing/scale | Real-browser computed style is 24px left/right padding and 20px title font at both 390px and 360px viewports. | Matches the extracted upstream mobile geometry. | `aligned` |
| Group wrapper | Real-browser group bounds retain 24px left and right insets at both target mobile widths. | Matches the extracted upstream group geometry. | `aligned` |
| Book rows | Real-browser computed style is 20px horizontal row padding with an 84×112 cover at both target mobile widths. | Matches the extracted upstream visible geometry without horizontal overflow. | `aligned` |
| Layout model | OpenReader uses CSS grid/list rows and chip buttons instead of Element tabs/desktop `.book` flex. | Acceptable only if visible geometry and operations remain upstream-compatible. | `technical-stack-equivalent` |
| Empty/loading rows | OpenReader adds skeleton/empty states. | Acceptable enhancement; must not alter normal loaded shelf geometry. | `acceptable-change` |

### Completed verification gates for this shelf-geometry slice

1. Mobile Home CSS resolves to the upstream insets and dimensions: title 24px side inset, compact 20px title, group 24px side margins, rows `10px 20px`, and `84px × 112px` covers.
2. The browser contract locks these rendered values, guarding future visual drift without tying assertions to one CSS implementation.
3. The Index mobile browser smoke verifies at 390×844 and 360×800:
   - title left/right insets are approximately 24px;
   - group wrapper side insets are approximately 24px;
   - first book row left/right padding is approximately 20px;
   - cover box is approximately 84×112;
   - no horizontal overflow.
4. The larger Index scene convergence and BookInfo consolidation remain separate P1 slices.

### Current OpenReader evidence and classification

| Layer | Current evidence | Difference | Classification |
|---|---|---|---|
| Sidebar frame | `frontend/src/layouts/AppLayout.vue` uses `.app-sidebar` fixed left, width `var(--app-sidebar-width)`, and `.app-sidebar-scroll` for the scrollable content. | Structurally capable of matching upstream. Need assert width source and mobile transitions. | `technical-stack-equivalent` |
| Bottom icons | `sidebar-bottom-icons` is outside `.app-sidebar-scroll`, so scroll does not move it. | This is already aligned with the upstream fixed-bottom structure, but tests should lock it so future edits do not regress. | `aligned` |
| Bottom icon drag behavior | Mobile CSS applies a counter-transform using `--mobile-nav-drag-offset`. | Upstream moves the whole navigation frame during drag, but the user explicitly requested GitHub/day-night controls not to slide with side-panel dragging. Keep this as a documented OpenReader UX difference. | `acceptable-change` |
| Gesture width | `useAppMobileNavigation.js` uses `navigationWidth = 260` and a separate `dragLimit = 270`. | Upstream drag window and visual sidebar width are independently preserved. | `aligned` |
| Drag style | Hidden + 80px drag yields `marginLeft: -190px`; the 270px endpoint yields `0px`. | Matches upstream `moveX - 270` behavior. | `aligned` |
| Touch guards | Current composable keeps the 20px edge guard and vertical-dominance passthrough. | Aligned and should be retained. | `aligned` |
| Route/action close | `runNavAction()` and sidebar search navigation close mobile sidebar after every route/action. | Upstream Index does not navigate between separate pages for these workspace panels, but shelf click does close the sidebar. This is part of the larger P1 scene-convergence work; for this slice, do not add new closures beyond the existing workspace click behavior. | `partial` |
| Workspace click close | `.app-workspace @click="closeMobileNavigation"` mirrors upstream shelf click close. | Keep, but make sure sidebar controls/bottom buttons do not pass the click into workspace. | `aligned` |
| Tests | `frontend/tests/appMobileNavigation.test.mjs` asserts 260px visual width, 270px gesture boundary, -190px at 80px opening drag, and edge/vertical guards. | `scripts/smoke/index-mobile-sidebar-contract.mjs` verifies rendered geometry and interactions at 390×844 and 360×800. | `aligned` |

### Completed sidebar verification gates

1. The mobile sidebar visual width (260px) is separated from the upstream gesture window (270px).
2. `useAppMobileNavigation` drag style and clamp tests assert:
   - static `navigationStyle` keeps `--mobile-nav-width: 260px`;
   - hidden + 80px right-drag yields `marginLeft: -190px`;
   - hidden + 270px right-drag is accepted;
   - hidden + 271px right-drag is ignored/clamped according to the upstream window;
   - open + 270px left-drag is accepted.
3. DOM/CSS structure keeps `.sidebar-bottom-icons` outside `.app-sidebar-scroll`, with absolute fixed-bottom positioning and interactive children.
4. The real-browser mobile smoke verifies that it:
   - opens the sidebar by menu and by drag at 390×844;
   - verifies content scrolling does not move GitHub/day-night buttons relative to the sidebar frame;
   - verifies workspace tap closes the sidebar;
   - verifies bottom icon click does not close the sidebar by propagation.
5. This remains an incremental P1 shell-alignment slice. Larger Index convergence is separately tracked.

### 2026-07-13 sidebar revalidation

This review revisited the fixed upstream `Index.vue` touch handler and CSS, then compared it with `useAppMobileNavigation.js`, `AppLayout.vue`, `appMobileNavigation.test.mjs`, and the real-browser sidebar contract. The earlier `must-fix` evidence above was historical: the implementation and its tests already contain the required separation of a 260px sidebar from the 270px gesture window.

- Unit contract: all five navigation tests pass, including `80px → -190px`, acceptance at 270px, rejection beyond the range, the 20px edge guard, and vertical-scroll passthrough.
- Browser contract: `index-mobile-sidebar-contract.mjs` passed at 390×844 and 360×800. It confirms default `-260px` hidden state, 260px rendered width, 270px drag endpoint, workspace close behavior, zero scroll movement for bottom controls, and no click-through from the theme button.
- Allowed difference: OpenReader counter-transforms the bottom GitHub/theme controls during a drawer drag so they remain visually fixed. This follows the user's explicit request and does not alter their fixed-bottom/independent-scroll relationship to the sidebar.

This is verification only; it does not create a new Docker candidate because no production code changed.

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
- The 2026-07-15 P1-E4 audit restored the upstream first image-only spine cover: a title-less `titlepage.xhtml` is now kept as “封面” through preview, import, chapter rows and the protected iframe resource route.
- The EPUB browser contract is covered by `scripts/smoke/reader-epub-contract.mjs` at 1440×900, 390×844, and 360×800.
- E4-EPUB-2 was committed and published after the full local gate: Git `8f5e979`, GHCR `ghcr.io/changshengyu/openreader:8f5e979` and `:latest`, multi-architecture index `sha256:1f17a4a028742515c065d00995df8e2f109a87386f9e5e221f4033851663de34`.

Remaining Reader P0 release gate:

- Browser suite completed from the fresh production build on 2026-07-11: `reader-mobile-contract.mjs`, `reader-tts-contract.mjs`, `reader-image-contract.mjs`, `reader-audio-contract.mjs`, and `reader-continuous-contract.mjs` passed at desktop `1440×900` plus mobile `390×844` / `360×800`; `reader-epub-contract.mjs` passed at those same viewports against a temporary isolated Go API/import service. The EPUB smoke now accepts `SMOKE_VIEWPORTS` so each viewport can be run independently without losing a real failure to the runner time limit.
- Git-traceable commit and local Docker/volume compatibility gate are complete for the E4-EPUB-2 slice; its GHCR release is recorded in [`epub-fragment-p1e4-contract.md`](epub-fragment-p1e4-contract.md). E4-PDFMD-1 now also has its UI/data/browser contract implementation and is awaiting its own Docker cut; the remaining P1-E4 functional work is the complete old-volume fixture.

### 2026-07-11 focused audit: Reader tool-layer exceptions and TTS bar

Status: TTS/read-bar exception implemented and browser-validated; primary panel/dialog coexistence remains a separate Reader P0 gate.

Authoritative upstream evidence is `web/src/views/Reader.vue`: `showToolBar` defaults to `true`; the four primary popovers (`popBookShelfVisible`, `popBookSourceVisible`, `popCataVisible`, `readSettingsVisible`) cause reader-content clicks to return without changing it. In contrast, the `showReadBar` watcher sets `showToolBar = false` when the read-aloud/TTS bar opens, and center clicks explicitly guard on `!showReadBar` before toggling tools.

| State / action | Upstream transition | Current OpenReader evidence | Classification / next test |
|---|---|---|---|
| Enter mobile Reader | `showToolBar = true`. | `mobileChromeVisible = ref(true)`. | `aligned`; retain entry regression. |
| Open shelf/source/catalog/settings | The popover becomes visible; content click returns while it is open and does not toggle `showToolBar`. | `useReaderPrimaryPanels` opens/toggles the named primary panel without changing `mobileChromeVisible`; `useReaderPointer` returns whenever a primary panel is open. | `aligned` for the tool-layer invariant. |
| Open bookmarks/search/book-info | Upstream invokes its dialog flow rather than one of the four primary popover flags; it does not set `showToolBar = false`. | OpenReader opens App-level bookmark/search/book-info overlays without changing `mobileChromeVisible`. | `aligned`, subject to real dialog click-through verification. |
| Open TTS/read-aloud bar | `showReadBar = true` triggers `showToolBar = false`; its own fixed bar takes control of the lower reader area. | `toggleTTSBar()` sets `ttsBarRequested` and hides mobile chrome only on open; its layout mode converts `flip` to `page` and reserves the upstream `280px`/`80px` content clearance. | `aligned` for the read-bar exception. |
| Center tap while TTS bar is shown | The normal center-menu branch is skipped because it only toggles when `!showReadBar`; ordinary non-slide edge page actions remain available. | `useReaderPointer` receives `ttsBarVisible`, suppresses only a computed `toggle-chrome` action, and keeps non-slide page actions intact. | `aligned`; pointer unit and real-browser tests cover this distinction. |
| Page/touch/keyboard while primary panel is open | Upstream panel branch returns before page logic; keyboard handling also returns for visible primary popovers. | Pointer and keyboard guards use the primary-panel state. | `aligned` for the primary-panel branch; add TTS-bar-specific keyboard/pointer checks separately. |

Completed TTS gate: state tests plus `scripts/smoke/reader-tts-contract.mjs` passed at `1440×900`, `390×844`, and `360×800`. The next Reader P0 gate remains real-browser verification that shelf/source/catalog/settings/bookmarks/search/book-info preserve visible tools and block click-through in each corresponding overlay/panel.

### 2026-07-11 focused inventory: Reader BookInfo dialog branch

Status: implemented structure re-verified on 2026-07-11; no application rewrite was necessary.

| Concern | Upstream authority and behavior | Current OpenReader evidence | Classification / required test |
|---|---|---|---|
| Ownership | `web/src/App.vue` hosts one root `<BookInfo v-model="showBookInfoDialog" />`; an event stores the selected book then opens that global dialog. It is not one of Reader's four primary popovers. | `OverlayBookInfo.vue` hosts one root `BookInfoDialog`; `useReaderPanels.openBookInfo()` passes the active reader book to the shared overlay store. | `technical-stack-equivalent`; Reader must not create a competing workspace or local BookInfo state. |
| Mobile geometry | Upstream `BookInfo.vue` uses `el-dialog`, `:fullscreen="$store.state.miniInterface"`, and title `书籍信息`. | `BookInfoDialog.vue` uses `el-dialog`, title `书籍信息`, 480px desktop width, and the same shared mini-interface/fullscreen predicate. | `aligned` for mobile dialog ownership/geometry. |
| Tool-layer interaction | Opening the global BookInfo dialog does not mutate Reader's `showToolBar`; the root dialog owns its click surface. | Reader opens the global overlay without changing `mobileChromeVisible`; the dialog is expected to intercept pointer events above Reader. | `must-verify`: mobile top tools remain visible and a dialog click cannot toggle/page Reader. |
| Reader shortcut | The fixed baseline's Reader toolbar does not expose a separate direct BookInfo float action; BookInfo is opened from the global App/event flow. | OpenReader exposes `书籍信息` in its Reader float tool strip, matching the user-provided upstream visual expectation while routing to the same root dialog. The reader-only status label is passed as context but intentionally not rendered by the dialog variant, because upstream BookInfo does not show it. | `intentional user-requested shortcut`; it must retain global-dialog behavior and not become a Drawer/workspace. |

Validation: the expanded `scripts/smoke/reader-mobile-contract.mjs` passed at `1440×900`, `390×844`, and `360×800`. It opens BookInfo from the Reader tool strip, verifies the active title/author, shared root-dialog ownership, no reader workspace/Drawer, mobile fullscreen geometry and chrome click-through protection, plus desktop rail persistence. It closes the dialog before continuing the existing cache/center-tap checks, so the active reading flow stays intact.

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

### P1-B follow-up inventory: retire unreachable standalone result shells (2026-07-13)

The route and overlay work completed after the original P1-B record means several rows in the initial matrix are now historical evidence, not outstanding work: `/search` and `/discover` already redirect to the root workspace; `/sources`, `/settings`, `/local-store`, and `/books/:id` likewise resolve to root workspace overlay/intents; and the shared `OverlayBookInfo` remains the only BookInfo owner.

The remaining structural gap is narrower but real. `Search.vue` and `Discover.vue` are now mounted only by `Home.vue` as root-workspace result bodies, yet both still carry their former `embedded` / non-embedded templates, route watchers, and page-only controls. Those branches are unreachable from the product router and preserve an incorrect second-page architecture that can drift from `Index.vue`.

| Concern | Upstream contract | Current evidence | Classification | Required test before code |
|---|---|---|---|---|
| Result-body ownership | `Index.vue` owns search and Explore as in-place shelf replacements; no standalone result product page exists. | `Home.vue` is canonical, and `Search.vue` / `Discover.vue` are only imported there, but both expose an obsolete optional standalone mode. | `must-fix` | Static contract: neither result body accepts an `embedded` prop, renders a `!embedded` branch, nor observes legacy page-route queries. |
| Legacy links | Old URLs may survive only as redirects preserving query intent. | Router already redirects `/search` and `/discover` to `/?workspace=…`; sidebar calls the workspace state directly. | `aligned` | Retain redirect and root-body assertions; prove no result component needs a route scene to initialize. |
| Result behavior | Search, local search, Explore, pagination, BookInfo/add/read, and return-to-shelf stay in the one Index scene. | The shared Pinia workspace state already supplies those transitions. | `aligned` pending cleanup | Existing state/route contracts plus the P1-B browser smoke must continue to pass after deletion. |

Allowed difference: the Vue 3 components remain separate implementation files for maintainability, but they are strictly root-workspace result bodies, not routable pages. No API, data, parser, user preference, or reader-route behavior changes in this cleanup.

Implementation record: completed on 2026-07-13. `Home.vue` now mounts the two result bodies without an `embedded` compatibility prop. `Search.vue` and `Discover.vue` always initialize from `indexWorkspace`, render only the root-workspace header/body structure, and no longer retain legacy route-query watchers or standalone page controls. The dead local-result bulk-selection widgets were removed with that unreachable page shell; the still-supported per-book import action remains. Static contracts protect the no-prop/no-standalone/no-route-query boundary. Validation passed: backend `go test ./...`, frontend 360-test suite, production build, and `index-workspace-contract.mjs` against real Chrome at 1440×900, 390×844, and 360×800 (legacy redirects, repeated sidebar search, BookInfo group confirmation, Explore, shelf return, and overflow).

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
| Failure detection | `Index.vue#showFailureBookSource()` calls `getInvalidBookSources()`, opens the same manager in failure view, and filters the failures that the backend recorded during normal use. `BookController#getInvalidBookSources` reads that user-scoped cache; invalid entries expire after 600 seconds. Opening the view does not run a new source request. | Entering the failure intent must only select the failure view. A user must explicitly press the bounded health-test command to start live checks. OpenReader has no persisted normal-use invalid-source cache yet, so the initial failure list is empty until P2 adds the equivalent user-scoped runtime-error cache; that missing cache is a separate backend/data gap, not permission to auto-test every source. |
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
| Health/failure | Current `/sources/batch-test` performs bounded concurrent tests and returns explicit rows; `failedOnly`/disable-failed are page controls. The prior `health` route intent also auto-started this request when the panel opened. | The bounded API and explicit results are a safety improvement, but automatically testing on entry conflicts with upstream's cached-failure transition and creates unexpected requests. | `must-fix`: route/overlay health intent only enables failure filtering; the explicit “失效检测” command owns live testing. P2 must add a user-scoped 600-second-equivalent normal-use failure cache before claiming full upstream failure-list parity. |
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
- Failure transition contract: opening `/sources?action=health` redirects to the root source overlay and enables the failure view without sending `POST /sources/batch-test`; pressing “失效检测” is the sole live-check trigger and may then populate the bounded structured summary. The missing persisted normal-use failure cache is tracked as a P2 backend/data contract rather than simulated by this UI intent.

### P1-C implementation record

Status: implemented and validated on 2026-07-10.

- **C1 — shared controller ownership.** The former route-owned `Sources.vue` has been moved to `frontend/src/components/workspace/SourceManager.vue`. `OverlaySources.vue` is the only host and owns the single Pinia state pair `sourceManageVisible` / `sourceManageIntent`. Reopening with another intent replaces the intent; closing resets it to `manage`.
- **C2 — Index convergence and old URLs.** Every AppLayout source action opens that shared overlay. `/sources`, `/sources?action=import|health|debug`, and `/sources?panel=remote` now redirect to the root workspace with `overlay=sources` and a normalized `sourceAction`, retaining all unrelated query keys. Closing the overlay removes only these intent keys.
- **C3 — lifecycle and mobile behavior.** The manager is full-screen on compact screens, remains in the same root-workspace scene, receives `openreader:sources-update` reload events, and does not close or receive clicks through to the mobile sidebar. Local and remote import continue to use a selection preview before any mutation; health and three-step debug remain nested manager dialogs.
- **Allowed differences.** The current Vue 3/Pinia dialog, structured editor, bounded *user-triggered* health test, guided in-overlay debug, Go/SQLite authorization, and user-scoped persisted remote URL are retained runtime/safety improvements. No book-source API, parser, database, or backup contract changed in P1-C.
- **Known follow-up / audit correction (2026-07-12).** Earlier P1-C evidence incorrectly treated an automatic health request on the legacy `health` intent as compatible. The upstream entry loads its existing 600-second invalid-source cache and never starts a fresh test. OpenReader's health intent was corrected to enable only failure filtering; the smoke contract now verifies zero `batch-test` calls before an explicit “失效检测” click, then verifies that the manual test creates the structured summary. P2 must still provide an equivalent user-scoped normal-use error cache for full failure-list parity.

### 2026-07-12 P2 invalid-source cache inventory

Status: implemented and validated on 2026-07-12. Authority is fixed `reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`: `BookController.kt#getInvalidBookSourceCache`, `isInvalidBookSource`, `addInvalidBookSource`, `getInvalidBookSources`, `searchBookWithSource`; `Index.vue#showFailureBookSource`; and `vuex.js#addFailureBookSource`.

| Contract layer | Upstream behavior | Current OpenReader behavior | Required disposition |
|---|---|---|---|
| Storage/TTL | User-namespace cache keyed by source URL, error/time payload, fixed 600 seconds; only unexpired rows are read. | No user-scoped normal-use failure cache. Manual health state exists only while SourceManager is mounted. | `must-fix`: additive user/source SQLite runtime-cache rows with 600-second expiry; never back up or mutate source configuration. |
| Read API | `POST /reader3/getInvalidBookSources` returns current user's cached rows; Index then merges them into current source data. | No API; health intent sets `failedOnly` but begins empty. | `must-fix`: canonical `GET /api/sources/invalid` plus a JWT-protected legacy adapter; no request test starts from this read. |
| Failure recording | Server request exceptions and client-detected timeout/network failures update the cache; ordinary search skips cached sources. | Search, candidate, refresh/change-source, explore and chapter errors return/aggregate failures but do not leave current-user state; the next ordinary search retries them immediately. | `must-fix`: record true request failures at authenticated handler boundaries, skip active entries in normal multi-source flows, do not record empty result/cancellation. |
| Visible state | “失效书源” opens the existing manager, showing current source metadata plus error; no second request/route. | Same overlay and non-autotest intent are already implemented, but only manual results populate `health`. | `partial`: load the cache into existing `health`/`failedOnly` state on the `health` intent; retain manual bounded health check as an explicit action. |
| Error privacy | Upstream persists `Exception.toString()`. | Go errors can contain request/remote details depending on branch. | `acceptable-change`: store and return only bounded generic classes such as timeout/request failure; never disclose credentials, headers, full query strings, response bodies, host paths or tokens. |

Implementation record: `SourceFailure` is an additive SQLite runtime cache keyed by JWT user/source, with UTC 600-second expiry and safe error classes. Normal source failures are recorded at authenticated request boundaries, then suppress that user's ordinary search/candidate retry; manual checking remains permitted. The health overlay reads `GET /sources/invalid` into its current failed-only UI without starting `batch-test`; a legacy reader3 read adapter is retained. Source updates/deletes/import/default restoration clean derived rows. API isolation/expiry/cancellation/edit tests, frontend state tests, production build and the 1440×900/390×844/360×800 browser smoke pass. This is an allowed Go/SQLite/JWT/security adaptation, not a backup or source-format change.

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

Status: implemented and browser-validated on 2026-07-13.

- **Authenticated stream contract.** `POST /books/:id/cache/stream` validates the same owner/bounded request as the legacy REST cache endpoint before opening `text/event-stream`. It emits a per-chapter `message`, terminal `end`, or client-safe terminal `error`. The legacy `/cache` endpoint remains for deployed clients and bounded batch cache operations remain an explicit OpenReader extension.
- **Cancellation boundary.** The stream's request context is propagated into source content fetch and pagination. Browser `AbortController` cancellation or a client disconnect stops before scheduling another chapter fetch, retains only already completed cache files, and deliberately skips a final shelf-update broadcast for the incomplete operation.
- **BookManage interaction.** The current remote book's cache button now becomes `停止 n/total`; activating it a second time aborts only that book's stream. Vue uses authenticated `fetch` SSE parsing rather than `EventSource`, so the JWT is never placed in a URL. A terminal stream error is surfaced through the existing BookManage error path; successful completion merges the returned shelf item.
- **Evidence.** Go contracts cover success/progress/end, owner rejection before stream opening, total source failure/error and cancellation without next-chapter scheduling. Frontend contracts cover SSE framing/error handling plus active-book progress and stop behavior. The real-Chrome `book-management-dialog-contract.mjs` passed at 1440×900, 390×844 and 360×800: streamed completion reaches `已缓存 2/2 章`, BookManage remains mounted while BookInfo/BookGroup coexist, compact dialogs are fullscreen, panel clicks do not close the mobile sidebar, and no horizontal overflow is present.

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

### 2026-07-12 P1-E visual/state re-audit: LocalStore, WebDAV and backup

Status: implemented and validated on 2026-07-12. The prior route-convergence result remains valid, but its drawer-shell equivalence claim was corrected by this implementation.

| Concern | Fixed upstream behavior | Current OpenReader evidence | Classification / required result |
|---|---|---|---|
| LocalStore container | `Index.vue` mounts `LocalStore.vue` in an `el-dialog`, using the normal `dialogWidth`/`dialogTop` on desktop and `fullscreen` for the compact/mobile interface. Opening retains the Index scene. | `OverlayLocalStore.vue` uses an `el-drawer`: `rtl`, 82% desktop width; `btt`, 88% mobile height. Its `destroy-on-close` recreates `LocalStore.vue`, so the existing current-directory root reset is correct. | `must-fix` visual/interaction structure: preserve the shared overlay owner and root reset, but restore a centred desktop dialog and true mobile fullscreen dialog. |
| WebDAV container and reset | `Index.vue` mounts `WebDAV.vue` in the same dialog/fullscreen pattern. Its `show` watcher calls `showWebdavFile('/')` on every open, so the root directory is the entry state. | `OverlayWebDAV.vue` uses the same 82%/88% drawer shell but does **not** destroy its browser. `WebDAVBrowser.vue` loads only on mount, leaving a previously visited directory active after close/reopen. | `must-fix` state and structure: dialog/fullscreen shell plus a root reload/reset on every open. Do not change its JWT, private-root, staged-preview or restore APIs. |
| Backup trigger and restore | `Index.vue#backupToWebdav` asks whether to overwrite the backup before issuing the request. `WebDAV.vue#restoreFromWebdav` asks for confirmation before recovery and refreshes Index data afterwards. | `OverlayBackups.vue` presents a richer backup list, but `useOverlayBackups.run()` sends the write request immediately and its uploaded-package restore also writes immediately. The WebDAV-browser restore path already confirms and restores shared state. | `must-fix` action transition: keep the richer list as an allowed enhancement, but require an explicit overwrite confirmation before `triggerBackup()` and a recovery confirmation before an uploaded package is sent. Present it in the same dialog/fullscreen family. |
| Workspace settings / account / cache | Upstream shows these Index controls inside its long-lived navigation; it does not define a separate LocalStore/WebDAV-style file dialog contract. | `OverlayWorkspaceSettings.vue` is a drawer with an extracted settings body. | `unknown`, deliberately out of this visual batch: retain root ownership and revisit its exact upstream layout with the sidebar/settings audit, rather than falsely applying the file-manager dialog contract. |

Required contracts before implementation:

1. Component/state tests: LocalStore and WebDAV visible shells are `el-dialog` with desktop centred width and mobile fullscreen; closing clears the active file path/selection; opening WebDAV twice requests the root both times.
2. Backup test: canceling the overwrite confirmation makes no backup API call; confirming makes exactly one call and refreshes the list. Canceling uploaded-package recovery sends no restore API call; confirmation preserves `applyRestoreResult`.
3. Browser smoke at 1440×900, 390×844 and 360×800: legacy root intents open a dialog over the unchanged workspace; mobile dialogs cover the viewport; close/reopen shows the root; dialog clicks do not pass through to the sidebar; no horizontal overflow.
4. Preserve all already-passed private storage, staged-token, import-preview, route-cleanup and Docker-volume contracts. This is a UI/state migration only; no data/API migration is authorized.

### P1-E3 implementation record

- **Dialog/fullscreen shell.** `OverlayLocalStore`, `OverlayWebDAV` and `OverlayBackups` now use the same centred desktop `el-dialog` family as the upstream workspace; their compact-interface state comes from `GlobalOverlayHost` and uses true Element Plus fullscreen rather than an 88%-height bottom drawer. The root Index workspace and legacy overlay route intents remain mounted underneath.
- **Root entry state.** All three file-operation dialogs use `destroy-on-close`. LocalStore keeps its already-correct remount/root behavior; WebDAV now remounts and calls its existing root-list load each time it opens, eliminating the stale last-directory state.
- **Destructive backup transitions.** `useOverlayBackups` accepts explicit preflight confirmations. The overlay now confirms before writing the WebDAV backup and before uploading a recovery package; cancellation performs neither network write nor state application. The richer backup-list/download UI remains an allowed OpenReader enhancement, while the upstream overwrite/recovery decision boundary is restored.
- **Evidence.** Frontend unit contracts now cover dialog ownership/mobile fullscreen, WebDAV remount state, and both backup confirmation branches. `npm test` passed **350** tests and `npm run build` passed. `workspace-operation-contract.mjs` passed in Chrome at `1440×900`, `390×844`, and `360×800`, including desktop centring, compact fullscreen, root reload on WebDAV reopen, old-link intent cleanup, no sidebar click-through and no horizontal overflow. The current backend suite remains green (no backend code changed in this slice). Local image `openreader-local-check:workspace-p1-e3` (`sha256:9727051cb4aecd51e9f723a5c09a543d06f9691f4548e0a7c7b0178e0c5ca18e`, revision label `dirty-workspace-p1-e3`) passed `docker-volume-backup-smoke.sh`.
- **Allowed differences / unfinished work.** User-private storage, staged import snapshots, JWT transport and the richer backup list remain deliberate OpenReader adaptations. Workspace settings, RSS, replace rules and user management retain their separately audited drawers until their own upstream visual/state batches; they were intentionally excluded from this file-operation migration.

A Git-traceable commit is still required before this validated local candidate may be retagged and pushed to GHCR.

### 2026-07-12 P2 storage UI/import re-audit

Status: contract extracted from the fixed upstream `LocalStore.vue`, `WebDAV.vue` and `Index.vue`. No LocalStore/WebDAV application code is changed by this audit section.

| Concern | Fixed upstream behavior | Current OpenReader evidence | Classification / required result |
|---|---|---|---|
| Root dialog labels | `LocalStore.vue` opens as `书仓文件管理`; `WebDAV.vue` opens as `WebDAV文件管理`. The file manager itself does not add a competing title above the list. | The already-correct root `el-dialog` shells are titled `本地书仓` and `WebDAV`; embedded bodies add their own `文件管理`/`WebDAV 文件管理` headers. | `must-fix`: keep the reconstructed centred/fullscreen dialog shell and lifecycle, but restore the upstream root labels and remove competing embedded manager titles. The current path may remain as a breadcrumb, not a second scene title. |
| LocalStore large-result and search gate | Upstream lists the current directory, filters only after a trimmed keyword exceeds two characters, renders the first **101** rows, then exposes one `加载更多 N 个结果` affordance which reveals the complete already-loaded result. Opening/navigating resets the gate. | Current non-recursive listing and sorting match the required entry state, but filtering begins at one character, the initial cap is 100, and `再显示` advances in 100-row chunks. | `must-fix`: reproduce the upstream >2-character filter threshold, 101-row initial cap and one-action reveal-all state. Optional recursive listing/extension filtering remain documented OpenReader enhancements and must default to off/empty. |
| Import format reachability | Upstream LocalStore accepts TXT/EPUB/UMD/**CBZ**; WebDAV accepts TXT/EPUB/UMD; direct import accepts the four LocalStore formats. | `storageImportable.js` now preserves those two distinct source gates. `OverlayBookImport.vue` exposes `TXT/EPUB/UMD/CBZ` and rejects forced PDF/Markdown/`.text` selections before preview. The backend still parses those three extensions only for old direct clients and existing archives. | `aligned UI + explicit data compatibility`: no frontend entry point may expose the extra historical formats. |
| Storage data/security | Upstream has shared filesystem paths and URL-token downloads. | OpenReader uses authenticated raw WebDAV, `canAccessStore`, user-private descendants, immutable preview tokens, atomic bounded writes and user-scoped backup/restore. Existing Go/data/Docker tests cover this behavior. | `allowed runtime/security adaptation`: do not weaken, move or rewrite mounted roots, JWT transport, private scope or staged-preview behavior during this frontend slice. |

Required pre-implementation tests:

1. Static/component tests: root dialogs retain `el-dialog`/compact fullscreen and use the two exact upstream labels; embedded managers no longer duplicate those labels; LocalStore's initial list semantics are 101 rows, keyword threshold `> 2`, and a one-action reveal-all.
2. Format contract: direct upload and LocalStore upload accept `.cbz`; a WebDAV `comic.cbz` listing row is importable alongside all permitted runtime formats. The test must keep `.md`/`.pdf` as intentional OpenReader additions rather than accidentally narrowing the set to upstream only.
3. Browser gate at 1440×900, 390×844 and 360×800: open legacy LocalStore/WebDAV intents, verify dialog title/centring/fullscreen, navigate a root listing containing a CBZ row, and close/reopen without stale state or sidebar click-through.
4. Preserve the existing full backend/frontend suites and Docker volume/backup smoke. No backend routes, persisted data, storage roots or parser behavior are authorized to change for this visual reachability slice.

Planned implementation order: add the failing component/helper/browser contracts; align LocalStore's gate and the three format selectors/classifiers; change root labels and remove the duplicate embedded titles; then run the full storage browser and mounted-volume checks before publishing a user-verification image.

### P2 storage UI/import implementation record

- **Upstream file-manager entry.** The existing root `el-dialog` and compact fullscreen lifecycle are retained. LocalStore is now titled `书仓文件管理` and WebDAV `WebDAV文件管理`; the embedded bodies no longer duplicate those scene titles, while their breadcrumb/path context remains available.
- **LocalStore state gate.** A trimmed one- or two-character keyword deliberately leaves the loaded current-directory list unchanged. At three characters it filters; the initial render is exactly 101 rows, and one `加载更多 N 个结果` action reveals all already-loaded matches. Any load/navigation/filter change resets that gate. Recursive scan and extension filter remain opt-in OpenReader additions.
- **CBZ reachability and legacy formats.** Direct import and LocalStore expose the upstream CBZ entry while WebDAV intentionally does not. `.text`, Markdown and PDF remain readable only through the documented historical-data/API compatibility path; they are no longer advertised by the visible direct-import chooser and no backend parser/archive data path changed.
- **Evidence.** Focused contracts plus the full frontend suite (**357** tests), production build and `git diff --check` pass. The real-Chrome `workspace-operation-contract.mjs` passed at `1440×900`, `390×844` and `360×800`, including legacy intent routing, upstream labels, visible CBZ rows, root reload, fullscreen/centering, no horizontal overflow and mobile modal click interception. Backend and Docker-volume evidence is required before this slice can be released.

Remaining storage audit work is deliberately separate: archive expanded-size/entry-count limits, parser-work bounds, automatic staged-token expiry cleanup, and a refreshed data/backup review. This UI/import reachability slice does not claim those data/security tasks are complete.

### 2026-07-12 P2 parser/staged-preview/backup safety re-audit

Status: extracted from the current Go import, archive and restore paths. This is a security/runtime adaptation, not a reader-dev visible-flow replacement. No application code is changed by this audit section.

| Concern | Current evidence | Required non-destructive result |
|---|---|---|
| EPUB initial import parser | `engine.ParseEPUBWithRule()` creates a ZIP reader and `readZipFile()` uses an unbounded `io.ReadAll`. The later Reader EPUB resource extractor has entry/path/expanded-size limits, but that does not protect preview/import before a book is created. | `must-fix`: preflight every initial EPUB ZIP before XML/HTML parsing: archive bytes, entry count, canonical path uniqueness/safety, per-entry uncompressed bytes and total expanded bytes. Every read must be bounded even if ZIP metadata is malformed. A rejection happens before archive creation, SQLite writes or staged-token consumption. |
| CBZ initial import parser | `engine.ParseCBZ()` already rejects unsafe paths, symlinks, duplicate normalized paths, more than 20,000 entries, entries over 128 MiB and total expansion over 2 GiB. | `partial`: retain the proven CBZ protections, but route the local-import parser through one named limit policy so configured import-parser limits cannot diverge from EPUB/PDF/UMD behavior. Existing CBZ reader resource protections remain independent and must not be weakened. |
| UMD and PDF parser work | `ParseUMD()` trusts the declared `uint32` chapter count before allocating its offset table; `ParsePDF()` walks every page and appends all extracted text without page/text budget. File input is bounded, but malicious metadata can still cause disproportionate allocation/CPU. | `must-fix`: validate UMD table arithmetic and a bounded chapter count before allocation; cap PDF pages and extracted text. Return a client-safe parse-limit error rather than creating a partial book or exhausting the process. |
| TXT/Markdown and shared parser policy | Upload/preview data are capped by `OPENREADER_MAX_IMPORT_BYTES`, and Go regex is RE2; format parsers do not currently receive a common configurable work-limit contract. | `must-fix`: add additive environment-configured parser limits with safe defaults. Existing default imports and persisted books stay readable; limits apply only to a newly parsed/explicitly re-parsed input. |
| Expired staged preview data | Each stage is user-private and expires after 24 hours. `loadStagedLocalImport()` removes an expired token when used, while `stageLocalImport()` only cleans that one user directory when another preview is created; orphan pairs can persist on an idle server. | `must-fix`: cleanup scans all `cache/import-previews/<user-id>/` directories at startup and on a bounded periodic schedule, removes only expired valid token pairs and stale/orphan stage files, never touches `library/`, SQLite data or an unexpired active preview. Cleanup failures are logged/ignored without blocking startup. |
| Backup ZIP restore | Uploaded-package and WebDAV restore now preflight a bounded ZIP and dispatch only its validated byte map. | `aligned + security improvement`: preserve supported `bookSource.json`, shelf, progress, RSS, bookmarks and replace-rule formats while rejecting unsafe/over-budget archives before mutation. |

Required contracts before implementation:

1. Engine fixtures create entry-count, per-entry and expanded-size ZIP violations plus a valid EPUB/CBZ control. Parser rejection must occur before an imported archive or SQLite book row exists.
2. Malformed UMD chapter counts and oversized PDF page/text fixtures fail quickly with a deterministic parse-limit error; ordinary TXT/EPUB/PDF/UMD/CBZ fixtures retain the current chapter/title results.
3. Stage-cleanup fixtures prove expired token pairs and orphan files are deleted from another idle user's directory, while a fresh valid pair remains loadable and no mounted `library/`/WebDAV file is touched.
4. Config compatibility fixture proves absent new environment values use documented defaults; no database schema, backup JSON field, mounted root or existing book is migrated.

Planned delivery order: (A) common import-parser limits, EPUB/UMD/PDF guards and staged-preview cleanup scheduling; (B) a separate backup ZIP-reader contract and restore hardening; (C) full Go/frontend/browser/Docker-volume regression before the next storage release.

### P2 parser/staged-preview implementation record

- **Additive limit policy.** New `OPENREADER_MAX_ARCHIVE_ENTRIES`, `OPENREADER_MAX_ARCHIVE_ENTRY_BYTES`, `OPENREADER_MAX_ARCHIVE_EXPANDED_BYTES`, `OPENREADER_MAX_PDF_PAGES`, `OPENREADER_MAX_PARSED_TEXT_BYTES` and `OPENREADER_MAX_UMD_CHAPTERS` values have safe positive defaults. The existing `OPENREADER_MAX_IMPORT_BYTES` remains the compressed input/staging bound; absent values do not rewrite data or change any persisted setting. Lazy recovery of an already archived local book uses a wider but still bounded compatibility policy, so new-import limits are not silently retroactive.
- **Safe parse-before-write.** Local-book importer preview/import now sends CBZ, EPUB, PDF and UMD through one limit policy before `ArchiveImportedBook`, SQLite mutations, category writes, broadcasts or token consumption. EPUB validates ZIP paths/symlinks/duplicates/count/per-entry/total expansion and bounds every member read; CBZ retains equivalent checks under the policy. UMD rejects excessive counts before offset allocation and decreasing offsets; PDF caps pages and extracted text.
- **Derived-cache lifecycle.** Startup invokes a full `cache/import-previews/<user-id>/` cleanup and a cancellable hourly worker repeats it. It removes only expired/invalid token pairs and aged orphan `.book` files; fresh valid previews, LocalStore/WebDAV sources, library archives, backups and SQLite data are untouched.
- **Evidence.** Engine fixtures cover ZIP count/per-entry/expanded/path rejection, UMD declared-count rejection and PDF extracted-text limits; importer coverage proves a rejected archive creates no book or library archive; API coverage proves cleanup reaches expired and orphaned files in otherwise idle user directories while preserving a fresh token. Full `go test ./...` passes. Frontend/Docker gates are still required before this backend slice can be released.

The backup ZIP reader/restore path was kept as the next separate data-contract submodule; its existing formats remain unchanged by this parser/staged-preview implementation.

### 2026-07-13 P2 local-import UMD binary compatibility audit

This audit directly compares the fixed upstream `LocalBook.kt`, `UmdFile.kt`, `me/ag2s/umdlib/umd/UmdReader.java`, `UmdChapters.java`, `UmdHeader.java`, and `UmdUtils.java` with OpenReader's `backend/engine/umd_parser.go`. It supersedes the earlier limits-only UMD assessment: current limits protect allocation, but the parser does not recognize the reader-dev UMD wire format at all.

| Concern | reader-dev fixed behavior | Current OpenReader behavior | Classification / required result |
| --- | --- | --- | --- |
| File signature | `UmdHeader.buildHeader()` writes little-endian `0xde9a9b89` (`89 9b 9a de`), followed by `#` sections. | `ParseUMDWithLimits` requires the unrelated ASCII prefix `#TEXTNOV`. | `must-fix`: standard reader-dev UMD files fail before catalogue parsing. Detect the upstream signature first. |
| Header and chapters | `UmdReader` walks `#` sections and `$` additional sections; type `0x83` carries chapter byte offsets, type `0x84` carries UTF-16LE titles plus zlib-compressed UTF-16LE content chunks. `UmdFile` exposes every title by index and obtains chapter text from the decompressed concatenated body. | Current parser treats following bytes as one flat offset/title/content table, decodes GBK, and never processes sections or zlib chunks. | `must-fix`: reproduce the upstream text-UMD catalogue/content contract, including `U+2029 → \n`; reject unsupported image UMD rather than returning invented chapters. |
| Bounded work | Upstream has no service-side bounds. | Current `LocalBookParseLimits` already bounds declared chapters, input bytes and parsed text policy, but it has no bound for accumulated decompressed UMD chunks. | `required security adaptation`: validate every section/additional length, count and offset before allocation; stream zlib with a total decoded-byte budget; malformed/truncated/compressed-over-budget input returns `ErrLocalBookParseLimit`/a safe parse error before staging consumption, archive writes or SQLite rows. |
| Existing imports | Existing OpenReader books normally read materialized chapter cache and must not be reparsed during upgrade. An early OpenReader-only `#TEXTNOV` parser may have produced old archives, although it is not reader-dev compatible. | Replacing the parser must not rewrite `data/`, `cache/`, `library/`, SQLite rows or existing chapter files. | `data-compatible`: leave persisted books untouched; use the standard parser for new preview/import and explicit refresh, with the old parser retained only as a documented fallback for legacy OpenReader pseudo-UMD input if it is still encountered. |
| Fixture evidence | The upstream checkout contains its UMD writer, but no committed `.umd` sample fixture. | Current tests prove only a malicious declared-count rejection, not successful reader-dev UMD import. | `must-fix before implementation`: add a deterministic byte fixture builder that follows the upstream writer (header, offsets, titles, one/multiple compressed chunks), and golden expected title/content output. |

Required test gate before changing application code:

1. Standard upstream-style UMD fixture imports through direct upload, LocalStore preview and WebDAV preview without any later network read; its title, author, ordered chapter titles and `U+2029`-normalized content are exact.
2. A multi-chunk body preserves ordered content across the zlib chunk boundary; offset/title count disagreement, malformed sections, truncated `$` payloads, bad zlib and an image-type UMD fail deterministically without a partial book.
3. A decompression bomb and excessive offset/title count fail before unbounded allocation or archive/database write. Error responses retain a retryable staged import token but reveal neither filesystem paths nor raw binary data.
4. Existing legacy local-book rows/cache remain readable without a migration; an explicit refresh uses the chosen standard/fallback parser and cannot cross user-scoped staged bytes.

Implementation sequence: add test-only upstream-compatible UMD fixture builder and failing golden/API contracts; implement a bounded section reader plus zlib accumulator; retain a narrow `#TEXTNOV` compatibility fallback only after the standard signature check; then run parser, API, storage-preview, full backend/frontend/browser and Docker-volume gates before an image release.

Implementation record (2026-07-13): `ParseUMDWithLimits` now detects the reader-dev `89 9b 9a de` signature before the narrowly retained legacy prefix, reads `#`/`$` segments, accepts writer-produced `F1` chunk separators and the terminal `81` check table, decodes UTF-16LE metadata/titles/content, and normalizes `U+2029` to a newline. It rejects image UMD, malformed/truncated segments, inconsistent offsets/titles, invalid zlib and bounded decoded-content overflow before importer persistence. `backend/engine/umd_parser_contract_test.go` builds the exact upstream writer structure, including multi-chunk bodies, while `backend/api/umd_import_contract_test.go` proves direct upload, LocalStore and WebDAV preview/confirm all import the same staged byte snapshot even after the mounted source file is removed. The failed direct-preview contract keeps only a caller-scoped retry token and does not expose a host path. The isolated `#TEXTNOV` fallback remains covered for existing OpenReader pseudo-UMD inputs; it is an explicit data-compatibility allowance, not an upstream format claim. Full Go tests, frontend tests and production build pass; mounted-volume/Docker publication is the remaining release gate.

### 2026-07-12 P2 backup restore data/safety re-audit

Status: contract extracted from upstream `Index.vue#backupToWebdav`, `WebDAV.vue#restoreFromWebdav`, `WebdavController.kt`, and OpenReader's backup/restore paths; implemented and awaiting Docker-volume release validation.

| Concern | Fixed upstream behavior | Current OpenReader evidence | Required result |
|---|---|---|---|
| User decision and workspace transition | Upstream asks before overwriting a backup and before recovery; successful recovery refreshes the Index scene. | `OverlayBackups` and `WebDAVBrowser` already require confirmation, while `applyRestoreResult` refreshes settings/shelf state and server broadcasts restore events. | `aligned + allowed enhancement`: retain the dialog/list/download UI, explicit confirmations, multi-user scope and event refresh. This slice is backend archive hardening, not a visual rewrite. |
| Restore input boundary | Upstream restores a WebDAV ZIP selected by the user. | Uploaded package restore performs unbounded `io.ReadAll`; WebDAV restore performs unbounded `os.ReadFile`. Raw WebDAV uploads have a size cap, but legacy mounted files and multipart restore bypass one shared restore policy. | `must-fix`: use one compressed-byte cap for both routes, reject oversized uploads/files before body allocation, and return a client-safe `413` or `400` without database writes. |
| ZIP archive structure | Upstream backup consists of source, shelf, category, RSS, setting/progress and related JSON records. | `restoreLegadoBackupData` accepts arbitrary ZIP entry paths and loops three times using suffix matching; there is no entry-count/per-entry/total-expanded validation. Restore helpers each use unbounded `io.ReadAll`. | `must-fix`: normalize and validate every ZIP entry before dispatch, bound entry count, each JSON entry and total decoded bytes, and use one reader budget for every helper read. Preserve reader-dev/Legado/OpenReader filenames and allowed `bookProgress/` compatibility entries. |
| Error and mutation semantics | The upstream client treats recovery as one confirmed operation followed by a refresh. | Current helper failures are commonly discarded (`n, _`), so malformed rows can yield a successful partial result without a diagnostic; existing idempotent upsert compatibility must remain. | `must-fix`: structural/budget/read failures fail before any restore mutation. Per-record legacy validation may remain best-effort only after a valid archive plan is accepted, and response counts keep their current schema. Do not delete existing user data or change backup JSON names. |
| Stored WebDAV selection | Upstream exposes restore only for `.zip` rows. | UI offers restore for `.zip`, but the API accepts any rooted file path. | `must-fix`: backend requires a normalized `.zip` filename before reading. This closes direct API misuse without changing visible successful flows. |

Required pre-implementation contracts:

1. Multipart and WebDAV restore reject compressed input beyond the configured restore cap before `ReadAll`/database effects; existing valid backup fixtures still restore with the current count/result schema.
2. ZIP fixtures reject unsafe path, duplicate canonical path, excessive entry count, excessive entry bytes and excessive total expansion; malformed archive failure creates/updates no user source, shelf, setting, category, RSS, progress, bookmark or replace-rule row.
3. Reader-dev/Legado compatibility fixtures retain top-level and documented nested `bookProgress/` JSON handling; normal WebDAV backup/restore and restore-triggered sync events retain existing behavior.
4. All helper reads share one archive budget; no raw ZIP filename, mount path, credential or untrusted JSON body appears in client error text.

Implementation record: both restore endpoints require a ZIP and apply one compressed limit. `backupRestoreArchive` rejects unsafe paths, symlinks, duplicate canonical names and over-budget entry/count/expanded archives while fully reading every member before dispatch; the restore loops use those validated bytes. Structural errors return a client-safe `400`/`413` before a source, shelf, setting, category, RSS, progress, bookmark or rule can change. Existing per-record decoding remains best-effort after a valid archive plan, preserving legacy restore count semantics. `backup_restore_contract_test.go` adds malicious/no-mutation and route-size coverage; existing reader-dev/Legado/OpenReader fixtures retain their count/event behavior. Full Go/frontend checks and mounted-volume Docker smoke remain the release gate.

## P1-E2 pre-implementation audit: workspace storage, backup, RSS and user-space semantics

Status: audited on 2026-07-11 from the fixed upstream baseline `changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`; implementation is now in progress against the contracts below. The table records the pre-implementation evidence and remains the authority for the remaining RSS, parser-bound and browser gates.

### Authority and scene contract

| Upstream authority | Required behavior | Current OpenReader evidence | Difference / priority |
|---|---|---|---|
| `web/src/views/Index.vue` (`showLocalStoreManageDialog`, `showWebDAVManageDialog`, `backupToWebdav`, user-space/cache/RSS entries) | All operations remain in the Index scene. Opening storage or an operation dialog does not replace the shelf scene; backup asks for confirmation; user-management entries are only visible in manager mode. | P1-E owns root overlay route intents; `AppLayout.vue` exposes “管理用户空间” only when `profile.role === 'admin'`, while pasted legacy intents still meet the authoritative backend `403`. | `aligned security adaptation`: explicit role-gated visibility plus backend authorization preserve the manager-only contract. |
| `web/src/components/LocalStore.vue` | Open resets to the root folder; list only the current directory, navigates directories, caps a large result until “加载更多”, supports search, selected delete/import, upload and then opens the shared book-import preview. Importable upstream extensions are TXT/EPUB/UMD/CBZ. | `LocalStore.vue` is an embedded root overlay with breadcrumbs, current-directory default, preview-before-write and a 100 item display cap. Direct unsupported files return a predictable item error; every successful preview stores an immutable user-scoped input token for confirmation. | P1 current-directory and deterministic P2 preview are implemented. `.text/.md/.pdf` are historical-data/API compatibility formats only and are not surfaced by LocalStore/WebDAV/direct-import UI. Upload/parser byte limits and automatic expiry cleanup are still required. |
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

### 2026-07-12 RSS visual/ownership re-audit

Status: audit complete; do not change RSS application code until the following contracts are added. The earlier data/state implementation record remains valid, but its classification of the combined drawer as an allowed UI equivalent is withdrawn.

| Concern | Fixed upstream behavior | Current OpenReader evidence | Classification / required result |
|---|---|---|---|
| Source entry shell | `RssSourceList.vue` is a root `el-dialog`, with a source-only grid. Desktop uses the shared small dialog width; the compact interface is fullscreen. | `OverlayRSS.vue` is an 82%-wide desktop/right drawer and 88%-height bottom drawer. `RSSManager.vue` combines source management and article list into one two-column surface. | `must-fix`: use an upstream-style centred source dialog and fullscreen compact dialog. The source dialog must not expose a permanent split-pane article list. |
| Source → article-list transition | Source click emits `showRssArticleListDialog(source)`; `RssArticleList.vue` opens its own dialog, parses the initial sort, resets page state and fetches page one. | `selectSource()` currently refreshes correctly but renders rows in the same combined manager. | `must-fix` visible ownership: preserve the corrected refresh/cache state machine, but move its article-list UI into a separate root dialog opened by source selection. |
| Article → content transition | A row fetches content and opens a third `RssArticle.vue` dialog; image click opens the image viewer. Closing the article-list dialog resets its own sort/list/page state. | OpenReader uses an article `el-dialog` nested below the manager and an image viewer. | `must-fix` dialog chain: retain sanitized content, per-user read/favorite state and safe external links, but make content a sibling/root dialog instead of a child surface coupled to a drawer. Close transitions must remain independently resettable. |
| Data/safety enhancements | Upstream has client cache and URL-based APIs. | Current IDs, persisted user-scoped cache, unread/favorite filters, late-response guards, sanitization and `noopener,noreferrer` are already covered. | `allowed adaptation`: retain these behaviors while changing only visual ownership and dialog transitions. |

Required contracts before implementation:

1. Static/component contract: `OverlayRSS` is a source-only `el-dialog`, receives the compact fullscreen flag, and mounts distinct article-list and article-content dialog owners; no root RSS drawer remains.
2. Controller contract: source selection creates the existing first-sort/page-one refresh sequence and opens the article-list dialog; closing that dialog clears only article-list transient state; closing source dialog clears all RSS state; article content/image close never reopen or mutate another dialog.
3. Browser sequence at 1440×900, 390×844 and 360×800: root RSS → source selection → refreshed article list → article content → image preview → close content → close list → close source → reopen. Assert desktop centring, mobile fullscreen, no stale list/dialog state, no click-through to the Index sidebar, no horizontal overflow and no duplicate refresh.
4. Keep the existing RSS parser/API/backup contracts unchanged. This batch is a frontend visual ownership migration and is not authorization to rewrite RSS persistence or remote fetching.

Planned P2-RSS implementation order: first extract the article-list and article-content dialog states from `RSSManager` without changing its source/sort refresh functions; then replace `OverlayRSS` with the three-dialog root chain; finally update the three-viewport smoke and run full frontend/backend/Docker validation. A Git-traceable commit is required before any release image.

### P2-RSS visual implementation record

- **Three-dialog chain.** `OverlayRSS` is now the upstream-style centred source `el-dialog` (compact/mobile fullscreen) rather than a Drawer. `RSSManager` keeps the existing source/sort/cache controller but renders only source management in that root dialog; source selection opens a distinct article-list dialog, and article selection opens the existing independent content dialog. The image viewer remains above content.
- **Transition preservation.** Opening the source dialog no longer auto-opens its first article list. Selecting a source still resets sort/page/article state, opens the list, reads scoped cached rows and performs exactly its existing selected-source refresh. Closing article list clears only its transient article/sort state; closing source clears the full RSS workspace and all nested surfaces.
- **Regression repair.** The shared `wideDrawerDirection`/`wideDrawerSize` calculations were retained for the intentionally un-migrated settings, user-management and replacement-rule drawers; the file/RSS Dialog migrations no longer accidentally leave those props undefined.
- **Current evidence.** Static/controller contracts in `rssWorkspaceContract.test.mjs`, the full frontend suite (**350** tests) and `npm run build` pass. `scripts/smoke/rss-workspace-contract.mjs` passed in Chrome at `1440×900`, `390×844` and `360×800`, covering source → article list → content → image → close/reopen, one refresh per selected source, desktop centring and mobile fullscreen. The global `workspace-operation-contract.mjs` also passed at all three sizes after the RSS root became a dialog. No backend API/data code changed in this visual slice.
- **Release evidence.** Git commit `7da8509b030db7ce8d4ccb6daf86985257312eed` is published to `main`. A local candidate built from it passed `docker-volume-backup-smoke.sh`; the locally built multi-platform release was then published to `ghcr.io/changshengyu/openreader:7da8509` and `latest` with OCI index digest `sha256:d0c7e22b74c6c75b1b213744af9f73e260aa9688cac35f48459c941b53c630f7` (`linux/amd64`, `linux/arm64`). Pulling that exact GHCR tag and running the mounted-volume/backup smoke passed again.

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
- **Browser completion.** `scripts/smoke/reader-mobile-contract.mjs` now creates a real DOM selection in the Reader, confirms “添加过滤规则”, and verifies that only the independent editor opens with the unmodified selected text and active-book scope. It verifies desktop rails, mobile fullscreen/tool-layer coexistence and click interception at 1440×900, 390×844 and 360×800. The manager/browser Docker gates also passed in the `57b1dc0` release: local amd64/arm64 build, GHCR manifest inspection and mounted data/cache/library backup-restart smoke.

### 2026-07-12 P2 manager visual/state re-audit: ReplaceRule and UserManage

Status: manager-shell slice implemented and validated on 2026-07-12. The existing replace-rule semantics and selected-text editor work remain valid; the Reader gesture that opens the direct editor remains a separate P2 browser scenario.

| Concern | Fixed upstream behavior | Current OpenReader evidence | Classification / required result |
|---|---|---|---|
| Replace-rule manager shell | `App.vue` mounts `ReplaceRule.vue` as a root `el-dialog` using the shared desktop width/top and compact fullscreen. Its independent `ReplaceRuleForm` is another root dialog and can open directly from Reader. | `OverlayReplaceRules.vue` keeps the independent editor/dialog and direct Reader handoff, but the manager table is an 82% right drawer / 88% bottom drawer. | `must-fix`: manager becomes a centred desktop dialog and compact fullscreen dialog. Preserve the existing root-addressable editor; opening it directly from Reader must not open the manager. |
| Replace-rule close state | Closing upstream manager ends that dialog's selection surface; the separate form owns its own close/cancel state. | Drawer state remains mounted between opens, so selection/table state can survive while the hidden manager is reused. | `must-fix`: manager uses `destroy-on-close` or equivalent explicit reset. Its close must not dismiss a direct editor that is already active. |
| User-management shell | `App.vue` mounts `UserManage.vue` as a root `el-dialog`, desktop shared width/top and compact fullscreen; adding a user opens a separate `AddUser` dialog. Index exposes manager actions only in manager mode. | `OverlayUserManagement.vue` has a separate create-user dialog and AppLayout already hides the action for non-admin users, but the manager table is an 82%/88% drawer. | `must-fix`: root user manager becomes a desktop dialog / compact fullscreen dialog with reset-on-close. Keep Go role checks, user-private storage permissions and the separate create-user form. |
| Extended controls | Upstream exposes namespace, login time, WebDAV/local-store flags and batch delete. | OpenReader uses roles, source/store capability switches, book/source counts and cleanup-inactive tools. | `allowed runtime adaptation`: do not remove current multi-user security/capability controls solely to copy upstream columns. Only container, visibility, lifecycle and mobile interaction must align. |

Required contracts before implementation:

1. Static/component tests: both manager roots are `el-dialog`, no Drawer remains, each receives the common compact fullscreen flag and destroys/reset state on close; their existing editor/create-user dialogs remain independent.
2. State tests: closing a manager clears selection and stale table rows; a Reader-originated replace-rule editor stays visible when the manager is absent; reopening a manager performs one fresh list load.
3. Browser gate at 1440×900, 390×844 and 360×800: old settings intent → manager dialog → close → root; desktop centred/mobile fullscreen, no horizontal overflow or sidebar click-through. For replace rules, additionally open the direct editor from Reader context without the manager. For users, retain administrator-only sidebar visibility and backend 403 for pasted legacy intents.
4. No API/data migration is authorized: preserve existing REST validation, ID ordering, JWT/WebSocket events, role enforcement and backup compatibility.

### P2 manager-shell implementation record (2026-07-12)

- `OverlayReplaceRules` and `OverlayUserManagement` now use the same root `el-dialog` pattern as upstream: centred desktop dialogs (`min(1120px, calc(100vw - 48px))`) and compact full-screen dialogs. The independent replacement editor and add-user dialogs remain separate surfaces.
- Closing a manager destroys its shell and calls an explicit manager-only reset. It clears table rows, selections, pending refreshes and manager loading states without closing an active direct replacement editor or create-user form. A monotonically increasing request token also prevents an old list response from repopulating a closed/reopened manager.
- `workspaceOperationRouteContract` statically protects the dialog/fullscreen/destroy/reset contract; composable state tests protect manager-only reset behavior. `workspace-operation-contract.mjs` now opens old settings intents, validates desktop centring/mobile fullscreen, root-route close behavior, no horizontal overflow and modal click interception at 1440×900, 390×844 and 360×800.
- Evidence: frontend `npm test` (353 passing), production `npm run build`, `git diff --check`, and the three-viewport real-Chrome workspace-operation smoke all passed. The smoke was corrected to match the actual RSS title and current Element Plus drawer close class; its mobile assertion now verifies the modal overlay blocks click-through, rather than attempting to click a sidebar hidden behind that overlay.
- The end-to-end Reader text-selection gesture → direct ReplaceRule editor smoke is now complete at all three release viewports. Broader UserManage role/permission API and backup contracts remain preserved and are not reopened by this manager-shell-only change.

## P2 UserManage behavior/data compatibility contract

Status: audited, implemented and validated on 2026-07-12 for the extracted P2 slice. Authority is fixed upstream `web/src/components/UserManage.vue`, `AddUser.vue`, `App.vue`, and the user-storage controller behavior in `changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`.

| Concern | Upstream behavior | Current OpenReader evidence | Classification / required result |
|---|---|---|---|
| Root ownership and mobile shell | `App.vue` owns one `UserManage` root dialog and a separate `AddUser` dialog; both use common desktop sizing and compact fullscreen. | `OverlayUserManagement` and its independent create dialog now follow that ownership, centred desktop dialog and compact fullscreen pattern. | `aligned` for shell; retain the independent create form and manager-only sidebar entry. |
| Protected account | Upstream marks the `default` namespace as non-selectable and hides its WebDAV/local-store switches, so the manager cannot delete or alter the system account from the row controls. | OpenReader excludes every administrator/current account from selection/deletion; protected rows now render no mutable capability or reset control, and update/reset routes return `403`. | `aligned security adaptation`: existing administrator rows are preserved, but never mutable from this manager. |
| New-user authority | Upstream `AddUser` accepts only username and password; it creates an ordinary user. There is no manager role selector. | OpenReader's create form has no role selector and `POST /admin/users` creates `user` rows only; a supplied `role: admin` returns `400`. | `aligned`: existing administrators remain upgrade-compatible, but the product flow cannot mint new administrators. Capability switches/limits remain allowed multi-user extensions. |
| Visible account metadata | Upstream rows show username, last login and registration time, formatted for the manager. | OpenReader now displays `lastActiveAt` and `createdAt` on desktop and mobile; missing/zero activity has the deterministic `未登录` state. | `aligned` while retaining current role/count/limit information as a multi-user extension. |
| User-scoped source actions | Upstream can delete selected users' private book sources and choose a user's sources as the defaults for future users. | OpenReader book sources are one global table with per-user edit permission, not per-user source rows; direct equivalents would violate deployed source/data semantics. | `intentional data-model redesign`: do not fabricate destructive per-user source actions. Document global-source ownership and retain source permission controls instead. |
| Visibility and pasted intents | Upstream only exposes the manager to its management context. | OpenReader hides “管理用户空间” for non-admin profiles, but a pasted legacy `/settings?panel=admin` intent still opens the root dialog and receives the authoritative backend `403`. | `acceptable security behavior`: hide the entry, preserve API `403`, render a non-privileged error state without stale manager rows or console errors. |
| Cleanup and limits | Upstream does not include activity cleanup, book/source limits or roles. | OpenReader adds inactive-user cleanup, role/count/limit fields and JWT authorization. | `allowed multi-user/security extension`: retain these controls only if protected accounts remain immutable and every destructive operation remains explicitly confirmed. |

Required contracts before implementation:

1. API contract: non-admin callers receive `403` for every `/admin/*` endpoint; create rejects `role: admin`; update/reset rejects protected admins; protected users remain excluded from batch delete; ordinary-user create/update/delete remains transactional and broadcasts one update event after commit.
2. Frontend state contract: protected rows have no actionable switches/reset controls; a failed ordinary-user permission update reloads the authoritative row; manager-created draft defaults to `role: user` and has no role selector; newly created row and list reset behavior stay stable.
3. Browser contract at 1440×900, 390×844 and 360×800: admin opens legacy intent → root dialog → create ordinary user → metadata/permission controls → close; non-admin sidebar omits the entry and pasted legacy intent shows the safe `403` state without stale content or click-through.
4. No schema or destructive data migration: existing admins, JWT tokens, counts, settings, private storage and global source rows survive unchanged.

### P2 UserManage implementation record (2026-07-12)

- Manager-created accounts now always use the ordinary `user` role. The form no longer exposes a role selector; the existing API keeps its path/schema but rejects `role: admin` with `400 BAD_REQUEST`.
- Every existing administrator is a protected management row. It cannot be selected/deleted, has no capability switches or password-reset action, and the server rejects direct update/reset attempts with `403 FORBIDDEN`. Ordinary-user capability/limit updates, reset and transactional deletion remain unchanged.
- The manager now displays recent activity and registration time on desktop and mobile. Missing/legacy zero activity renders `未登录`, while current roles/counts/source/store capability extensions remain visible.
- `scripts/smoke/user-management-contract.mjs` verifies admin and non-admin behavior at 1440×900, 390×844 and 360×800: legacy intent redirect, centred/fullscreen dialog, protected controls, ordinary-user creation, time metadata, hidden non-admin sidebar entry, and safe 403 empty state. Focused Go and frontend contracts passed before the browser gate.

## P2 bookmark compatibility contract

Status: re-audited, implemented and validated on 2026-07-12 for the extracted P2 slice. Authority is fixed upstream `web/src/components/Bookmark.vue`, `BookmarkForm.vue`, `App.vue`, `views/Reader.vue`, `src/main/java/com/htmake/reader/api/controller/BookmarkController.kt`, and `src/main/java/io/legado/app/data/entities/Bookmark.kt`.

| Concern | Upstream behavior | Current OpenReader evidence | Required result |
|---|---|---|---|
| Dialog ownership and scope | `App.vue` owns one Bookmark manager and one independent BookmarkForm. Reader opens the manager for its current merged shelf book; the manager filters stored rows by `bookName`/`bookAuthor`, supports import, selection delete, edit and jump. | `GlobalOverlayHost` owns `OverlayBookmarks` plus `OverlayBookmarkForm`; Reader passes the current book and the manager supports the same actions. | `technical-equivalent`: retain the root Vue 3 dialogs and current-book entry. Close/reset and panel click isolation must remain verified. |
| Form fields | Upstream form keeps book name, author, chapter and captured content read-only; only the note is editable. It rejects absent book identity or captured content. | Current global form renders the same read-only context and note editor, but backend direct create can accept an empty excerpt and an arbitrary `chapterId`. | `must-fix`: form stays as-is structurally; API must reject empty context, clamp location values, and verify any supplied chapter belongs to the owned book before persistence. |
| Multiple bookmarks and identity | Upstream JSON controller accidentally replaces a row by matching only `bookName`/`bookAuthor`, despite the manager exposing multiple selection/delete rows. | SQLite uses stable IDs and allows multiple current-user bookmarks per book. | `intentional technical adaptation`: preserve multiple ID-backed bookmarks and never collapse/de-duplicate deployed data. List/import/delete semantics must still present the upstream manager flow. |
| List order | Upstream shows its persisted array order; saving an existing row replaces in place without moving that slot. | Current API orders by `chapter_index, offset, created_at`, which changes visible sequence from insertion order. | `must-fix`: manager/API order uses stable bookmark creation order (`id ASC`/creation order). An edit must not reorder the list. |
| Local mutation order | Upstream appends a new bookmark to its persisted array; replacing an existing row writes back to the same array slot. The manager never renders a newer entry before older persisted entries merely because it was just imported. | OpenReader API now lists `id ASC` and its update path replaces in place, but `useBookBookmarks.create()` and `importPayloads()` optimistically call `prependBookmarks()`. Before a later sync reload, a newly created/imported row appears ahead of earlier IDs. | `must-fix`: client mutations append returned new rows in API/creation order. The manager must be `id ASC` both immediately after a create/import and after a reload; replacements remain in place. |
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
5. Frontend state contract: a create/import response is appended in returned creation order before any WebSocket or timed refresh; an edit replaces only its own slot and preserves surrounding IDs.
6. Browser gate at 1440×900, 390×844 and 360×800: Reader selected text → bookmark form → save; manager → edit/note/import/batch delete/jump; dialog/mobile tool layers coexist and clicks do not pass through to page turning.

Allowed differences: Vue 3 root dialogs, SQLite IDs/multiple-bookmark support, JWT scopes, transactional batches, explicit current-book legacy import, and offset/percent fast restore. The paragraph-context capture/fallback, visible ordering, form validation and text-reader interaction flow are not optional.

### P2 bookmark implementation record (2026-07-11)

- `GET /api/books/:id/bookmarks` now returns a caller-owned book's records in immutable creation/ID order; note edits no longer move a row.
- New and batch bookmark writes require paragraph context, normalize numeric location fields, verify an optional chapter belongs to the caller's book, and validate an entire batch before its one SQLite transaction starts. The editor only changes `note`, leaving a saved reader context immutable.
- Reader text selection now uses the upstream-style punctuation/whitespace-insensitive paragraph matcher. It only opens the bookmark form for a selection matching one or two full rendered paragraphs, and saves a five-paragraph/roughly-150-character context rather than the raw selected string. Text-reader jumps retain offset/percent as a fast path and carry that context for post-render recovery.
- `bookmarks.json` exports in stable creation order. Modern exports restore with their original `createdAt` plus context as the idempotency identity, so independently created bookmarks at one chapter/offset remain separate; legacy rows without timestamps retain a narrower location/content fallback. Restore rebinds a matching current-book chapter ID by its saved index.
- Audit correction and implementation (2026-07-12): the API/backup list order is `id ASC`. The frontend optimistic create/import helper now uses `appendBookmarks()` rather than prepending rows, so the manager preserves creation order immediately and after any later reload. Replacement remains in the existing array slot.
- Browser completion: `scripts/smoke/reader-mobile-contract.mjs` now creates a real Reader selection, chooses “添加书签”, verifies the captured paragraph context in the root form, saves it, then verifies manager edit, JSON import, immediate creation order, batch deletion and context/position jump. Desktop 1440×900, mobile 390×844 and 360×800 all passed; mobile form/dialog clicks preserved the visible Reader tool layer without click-through.

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
| Availability | `Reader.vue` `ttsSupportedForChapter` and `readerTTSBarVisible()` require speech support and exclude EPUB, audio, and rendered comic/image chapters. | OpenReader detects comic content through its Vue chapter presentation rather than upstream's `isCarToon` flag. | `technical-stack-equivalent` |
| Read bar visibility | `Reader.vue` keeps `ttsBarRequested` separate from `tts.state.playing`; opening it hides mobile chrome, and closing it does not invent a chrome-reopen transition. | Matches upstream requirement that opening the read bar does not start speech and applies the read-bar exception to the otherwise visible mobile tool layer. | `aligned` |
| Paging/slide mode coupling | `readerEffectiveMode()` maps a requested read bar plus `flip` mode to `page`, while leaving native scroll modes unchanged. | This is the Vue equivalent of upstream `isSlideRead() === false` during `showReadBar`; it restores the configured `flip` mode on close. | `aligned` |
| Read-bar content clearance | Reader CSS sets content bottom space to `280px` expanded and `80px` collapsed while the bar is active. | Variables are applied to desktop scroll padding and mobile rendered-body padding rather than upstream's inline `chapterTheme()` style. | `technical-stack-equivalent` |
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
- Completed in the 2026-07-11 follow-up: opening the read bar hides only mobile reader chrome, suppresses only center/menu toggle actions, converts mobile `flip` to the upstream non-slide page branch, and reserves `280px`/`80px` of visible content space when expanded/collapsed. Closing restores the configured read mode but does not reopen mobile chrome.
- Completed in the 2026-07-11 follow-up: comic/image chapters explicitly suppress TTS, matching upstream `!isCarToon` eligibility.
- Completed in this slice: `scripts/smoke/reader-tts-contract.mjs` verifies the real-browser TTS bar contract with a mocked `speechSynthesis`.
- Completed in this slice: TTS paragraph source now comes from rendered DOM headings/paragraphs rather than only splitting plain text, and starts from the active or first visible paragraph.
- Completed in this slice: TTS previous/next now restarts the target DOM paragraph and can cross chapter boundaries.
- Completed in this slice: `speechSynthesis` utterance errors are surfaced as `朗读错误: ...`.
- Validation note: focused reader mode/pointer/TTS unit tests and production build passed. `scripts/smoke/reader-tts-contract.mjs` passed in Chrome at `1440×900`, `390×844`, and `360×800`, covering desktop rail persistence, mobile default tools, TTS chrome exception, center-tap guard, flip→page→flip transition, expanded/collapsed `280px`/`80px` clearance, paragraph controls/highlight, speech error handling and close behavior.

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
## 2026-07-13 BookInfo action-state audit (historical pre-fix baseline)

Upstream authority is `reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`
`web/src/components/BookInfo.vue`. Its `isInShelf` computed value compares the
active book URL to the shelf list and is the sole action predicate. `Index.vue`
and `Reader.vue` only select the book then open the same global BookInfo dialog.

| Behavior | Upstream contract | Pre-fix OpenReader | Classification at audit time |
|---|---|---|---|
| Existing shelf book | Shows existing shelf properties (cover update, follow update, group/local actions); no read/detail action in the dialog. | Search/Discover and legacy detail hydration inject read/detail action arrays. | `must-fix` |
| Unshelved result | The BookInfo property area shows only `加入书架`; it is not a “join and read” menu. | Search/Discover inject `加入书架` and `加入并阅读`; temporary Reader lacks the same shared add path. | `must-fix` |
| Add success | The saved shelf record becomes the new BookInfo state without changing the current scene. | Each source screen reopens a separate BookInfo configuration with `开始阅读`. | `must-fix` |
| Reader entry | Merges current reading book with matching shelf record then opens the global dialog; no Reader route/tool state change. | Saved Reader is close; temporary Reader must receive the same unshelved add branch. | `must-fix` |
| `/books/:id` | No upstream route exists. | OpenReader redirect is required compatibility, but its injected `开始阅读` action is not. | `acceptable-change` |

The implemented result and verification are recorded in section 10 of
`docs/compat/index-search-p1b-contract.md` and in the following browser inventory.

## 2026-07-13 P1-B BookInfo five-entry browser inventory and result

Upstream evidence was rechecked before changing the current browser contracts:

- `reader-dev/web/src/views/Index.vue#toDetail` sends a result card's non-cover
  area into Reader, while the cover uses `@click.stop="showBookInfoDialog(book)"`.
  `Index.vue#showBookInfoDialog` only opens the shared dialog.
- `reader-dev/web/src/views/Reader.vue#showReadingBookInfo` merges the current
  reading book with the same-URL shelf book, then opens that same dialog without
  changing the Reader route or tool layer.
- `reader-dev/web/src/components/BookInfo.vue#isInShelf` is the only action
  predicate; the unshelved property area contains a single `加入书架` action.

| Entry / contract | Implemented OpenReader evidence | Classification | Verification |
|---|---|---|---|
| Search and explore result card | `RemoteBookResultGroups.vue` now has only cover `preview` and card `read`; the non-upstream `查看信息` button is removed. | `resolved must-fix` | `remoteReaderEntryContract` plus `index-workspace-contract` click the cover for BookInfo and the body for temporary Reader. |
| Search BookInfo add | `OverlayBookInfo` remains the only transaction owner; the workspace smoke now checks cancel = zero write, confirm = one write, shelf-state replacement and no Reader navigation. | `resolved must-fix` | 1440×900, 390×844, 360×800. |
| Explore BookInfo | The explore smoke opens the cover's shared BookInfo and checks no secondary action and no route change on close. | `resolved must-fix` | 1440×900, 390×844, 360×800. |
| Saved Reader BookInfo | `reader-mobile-contract.mjs` supplies a same-URL shelf row and checks no `加入书架` or navigation action while preserving reader UI. | `resolved must-fix` | Desktop, 390×844, 360×800. |
| Temporary Reader BookInfo | `remote-reader-contract.mjs` opens BookInfo from the temporary Reader before persistence, verifies the single add action and unchanged temporary route/tool state. | `resolved must-fix` | 1440×900, 390×844, 360×800. |
| Legacy `/books/:id` | The mobile sidebar contract now closes BookInfo, verifies `bookInfo` query removal and checks no injected read action. | `resolved must-fix` | 390×844, 360×800. |

Allowed differences remain limited to the compatibility redirect and the user-approved
multi-category confirmation. This inventory and its real-browser contracts are the
completion evidence for the P1-B BookInfo five-entry slice.
