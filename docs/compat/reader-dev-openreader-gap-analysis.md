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
| Reader content formats | Upstream `Content.vue` handles text, images/comic-like content, EPUB iframe documents, audio-related branches, and cross-chapter behavior. | Current `ReaderChapterContent.vue` handles text/images/volume blocks; imported EPUB chapters are flattened to text before rendering. | EPUB document structure, styles, images, links, iframe events, and scroll restoration are lost. EPUB is confirmed `must-fix`; audio/TTS and remaining cross-chapter edges still need separate extraction. | `must-fix` for EPUB; `unknown` for remaining formats | EPUB archive/resource/API/iframe fixture tests and browser smoke; separate contracts for audio/TTS and cross-chapter edges. |
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

### 2026-07-06 implementation note

Implemented in commit work following this contract:

- Mobile reader tool layer now defaults to visible and initial chapter loading no longer hides it.
- Opening reader tools/panels no longer forces the mobile tool layer closed.
- Mobile shelf, catalog, bookmark, search, source, cache, and settings panels now use a toolbar-coexisting full-width workspace instead of visible bottom `el-drawer`.
- Settings panel coexistence, no visible mobile drawer, center tap behavior, and 16px mobile reader geometry are covered by `scripts/smoke/reader-mobile-contract.mjs`.
- Ordinary text rendering now preserves safe upstream inline HTML semantics via sanitized `v-html`, while keeping plain text for search/progress.
- Image markup now marks chapters with upstream comic semantics, and CBZ-like book URLs hide the chapter title to match upstream `Content.vue`.
- Unit tests that previously encoded “panel open hides toolbar” were rewritten.

Still pending in Reader P0:

- Implement the EPUB contract below.
- Complete separate `Content.vue` parity reviews for image lazy-loading/CBZ import edge cases, audio/TTS media controls, and continuous cross-chapter edge cases.
- The final Reader P0 acceptance image remains pending. Intermediate validation image `ca43409` has been published and is not the final Reader P0 release.

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

### Required tests before implementation

Backend/API tests:

1. Import a fixture EPUB containing XHTML, relative CSS, image, font, same-document hash, and cross-chapter link; preserve both searchable text and canonical resource paths.
2. Existing imported EPUB rows without new metadata recover resource paths from `OriginalFile` without schema/data loss.
3. Chapter content returns the additive EPUB response while ordinary text responses remain backward-compatible.
4. A valid capability serves chapter XHTML and relative resources with correct MIME/security headers.
5. Another user/book, an expired or modified capability, stale archive version, traversal path, missing resource, and unsupported active content are rejected.
6. ZIP-slip, symlink, duplicate path, entry-count, per-entry-size, and total-expanded-size fixtures fail without partial files outside the extraction root.
7. Corrupt/missing archives and extraction failures return stable non-blank error responses.

Frontend unit/contract tests:

1. EPUB detection continues to use `originalFile`/`libraryPath` rather than the synthetic `local://book_<id>` URL.
2. `format: epub` renders one dedicated iframe and never renders ordinary paragraph blocks.
3. Bridge origin/source validation rejects messages not sent by the active iframe/resource origin.
4. `inited`, `load`, `setHeight`, `click`, `clickHash`, `keydown`, and `previewImageList` produce the upstream state transitions.
5. Reader style changes are sent to the live iframe; height is clamped to at least 80% viewport.
6. Cross-document links update chapter index/title/progress; position is restored again after iframe load.
7. Loading/capability failures show a retryable reader error and cannot become a silent blank page.

Real-browser gate:

1. Open the fixture at 1440×900, 390×844, and 360×800.
2. Confirm XHTML typography, relative CSS/image/font loads, image preview, internal hash, cross-chapter link, keyboard/click navigation, and saved-position restoration.
3. Confirm no `401`, blank page, cross-user resource access, duplicate center-click toggle, horizontal shift, or change to the already-established toolbar/panel coexistence contract.

Deferred from this EPUB slice:

- TTS/read-aloud and auto-reading behavior: upstream hides some reader controls for EPUB while `Content.vue` contains generic autoplay methods. This requires a separate evidence pass before claiming parity.
- Search result navigation into exact text inside iframe content.
- Remaining CBZ lazy-loading and continuous cross-chapter edge cases.

## Required workflow for each future module

1. Use `readerdev-compat-inventory`.
2. Update this file or a focused `docs/compat/*.md` contract.
3. Add/update tests for `must-fix` behavior.
4. Implement OpenReader changes.
5. Run module gate and record allowed differences.
6. Publish Git commits promptly. Publish Docker after any coherent, fully verified slice suitable for user validation; a complete module boundary remains preferred.
