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
| Reader mobile toolbar state | `web/src/views/Reader.vue`: `showToolBar: true`; center tap toggles; panel open branches return without hiding toolbar. | `frontend/src/views/Reader.vue`: `mobileChromeVisible = ref(false)`; `useReaderTools` and `useReaderPanels` hide chrome on panel actions. | Directly causes mobile reader mismatch and blank/operation confusion reports. | `must-fix` for P0 | Contract tests for default visible and panel coexistence; mobile browser smoke. |
| Reader mobile panel structure | Upstream uses Element popovers with `popper-class="popper-component"`; mini interface popper is full-width and coexists with toolbar. | Current reader uses multiple mobile bottom `el-drawer` instances. | Drawer architecture changes z-index, hit testing, toolbar coexistence, and upstream layout. | `must-fix` for P0 | Browser tests for settings/catalog/source/shelf panel + toolbar visible. |
| Reader mobile content geometry | Upstream mini `.chapter` uses `width: 100vw`, `padding: 0 16px`, `box-sizing: border-box`, `text-align: justify`; slide mode also uses 16px content margins. | Current mobile CSS uses `padding: 42px 22px ...`; paragraph justification/spacing does not fully match upstream. | Causes asymmetric left/right whitespace and paragraph layout drift. | `must-fix` for P0 | DOM geometry probe for left/right padding within 1px across 390×844 and 360×800. |
| Reader scrolling vs click paging | Upstream has page/scroll modes with discrete click navigation. | User requested continuous native finger/wheel scrolling while click paging remains segmented. | Intentional UX improvement if it does not change mode selection semantics. | `acceptable-change` | Browser scroll continuity probe; click paging regression tests. |
| Reader settings controls | Upstream uses controls that are easier to distinguish visually; user requested minus/value/plus controls instead of current easy-to-mis-tap slider behavior. | Current setting stepper exists but must be rechecked against upstream layout/state. | Allowed UX adaptation, but values/defaults/state must match upstream. | `acceptable-change` | Unit tests for value bounds; browser setting interaction test. |
| Reader content formats | Upstream `Content.vue` handles text, images/comic-like content, EPUB, audio/TTS-related branches, and cross-chapter behavior. | Current `ReaderChapterContent.vue` and composables partially reimplement content presentation. | Some formats and edge states are not yet proven equivalent. | `unknown` / likely `must-fix` | Fixture tests per content type; browser render smoke. |
| BookInfo | Upstream has one `web/src/components/BookInfo.vue` used from workspace flows. | Current has `BookDetail.vue`, `BookInfoDialog.vue`, `BookInfoPanel.vue`, `OverlayBookInfo.vue`. | Duplicate logic risks inconsistent actions/search/read/source behavior. | `must-fix` for P1 | Single BookInfo action contract; search/shelf/reader reuse tests. |
| Bookshelf/BookManage/BookGroup | Upstream: `BookShelf.vue`, `BookManage.vue`, `BookGroup.vue` under Index workspace. | Current: `Home.vue`, overlay management components, categories/store utilities. | Some enhancements may be valid, but workflow and mobile sidebar behavior need upstream comparison. | `unknown` | Workspace browser flows; category/order tests. |
| Mobile Index sidebar | Upstream sidebar width/drag/fixed bottom buttons must be extracted from `Index.vue` and related CSS. | Current `AppLayout.vue` and mobile navigation had reported drag/fixed-button mismatch. | User-visible mismatch: GitHub/day-night buttons should not slide with drawer content. | `must-fix` for P1 | Mobile drag smoke; fixed-bottom button geometry probe. |
| Search/explore/source flow | Upstream Index integrates search/explore/source and BookInfo transitions. | Current has separate `Search.vue`, `Discover.vue`, `Sources.vue` pages. | Flow fragmentation can change API order, panel state, and back behavior. | `must-fix` for P1 | Search → result group → BookInfo → add/read browser test. |
| Online source parsing | Upstream reader3-compatible source semantics live across web components and Java backend. | Current Go parser in `backend/engine/source_*` has tests and compatibility shims. | Must continue fixture-based extraction; do not infer equivalence from passing current tests alone. | `unknown` | HTML fixture/golden tests for search/info/toc/content. |
| Local import catalog parsing | Upstream behavior needs deterministic catalog extraction independent of network. | Current staged upload token flow reduces repeated upload/network dependency. | Staged token is acceptable enhancement; catalog detection still needs upstream fixture comparison. | `acceptable-change` + `unknown` | TXT/EPUB/Markdown/PDF/UMD fixture tests; reparse without upload test. |
| Replace rules/content cleanup | Upstream `ReplaceRule.vue` and backend semantics need extraction. | Current Go endpoints and overlays exist. | Rule ordering, scope, and test output may differ. | `unknown` | Golden rule application tests; UI batch action tests. |
| RSS | Upstream `RssSourceList.vue`, `RssArticleList.vue`, `RssArticle.vue`. | Current `RSSManager.vue`, overlays, Go RSS parser. | UI and parser semantics need mapping. | `unknown` | RSS fixture parser tests; source/article browser smoke. |
| WebDAV/local store | Upstream `WebDAV.vue`, `LocalStore.vue` and server storage behavior. | Current Go WebDAV/local-store endpoints and browser component exist. | Path safety and workflow compatibility both need explicit contract. | `unknown` | Path traversal tests; upload/list/import browser smoke; Docker volume smoke. |
| Backup/restore | Upstream backup flows and reader-dev formats require extraction. | Current OpenReader backup service and Legado restore exist. | Must preserve OpenReader data and document reader-dev/Legado import semantics. | `unknown` | Restore testdata; backup list/download/restore tests. |
| Auth/user management | Upstream user management components include `AddUser.vue`, `UserManage.vue`; OpenReader adds JWT. | Current JWT/multi-user/admin endpoints are intentional runtime adaptation. | Auth model differs intentionally, but UI/workspace placement and permission behavior need checks. | `intentional-redesign` + `unknown` | Auth dialog and admin browser smoke; user-scope API tests. |
| Docker/runtime | Upstream ships Java/Gradle/Docker variants. | Current single Go binary + frontend dist in Alpine, env-driven volumes. | Intentional deployment redesign. Must preserve local Docker build and volume compatibility. | `intentional-redesign` | `PUSH=0 ./scripts/docker-build-push.sh`; `scripts/docker-volume-backup-smoke.sh`. |

## Immediate P0 contract: Reader mobile

| Behavior | Upstream evidence | Current evidence | Required action |
|---|---|---|---|
| Tool layer default | `Reader.vue` data contains `showToolBar: true`. | `Reader.vue` contains `mobileChromeVisible = ref(false)`. | Set mobile toolbar default to visible and test it. |
| Panel open | Upstream click handler returns when popovers/settings are visible; opening a panel does not hide `showToolBar`. | `useReaderTools.openMobileTool` and `useReaderPanels` set `mobileChromeVisible.value = false`. | Remove panel/action coupling to toolbar visibility. |
| Mobile panel container | Upstream mobile popper is full-width via `.popper-component`. | Current mobile uses bottom drawers for shelf/toc/source/settings/search/cache/bookmark. | Rebuild mobile panels as toolbar-coexisting popovers/workspaces. |
| Horizontal layout | Upstream mini chapter padding is 16px and justified. | Current mobile content padding is 22px with altered vertical padding. | Restore upstream 16px geometry and paragraph semantics. |
| Tests | Upstream behavior is source contract. | Existing tests assert toolbar hides after panel actions. | Delete/rewrite conflicting tests. |

## Required workflow for each future module

1. Use `readerdev-compat-inventory`.
2. Update this file or a focused `docs/compat/*.md` contract.
3. Add/update tests for `must-fix` behavior.
4. Implement OpenReader changes.
5. Run module gate and record allowed differences.
6. Publish Docker only after a complete module is ready.
