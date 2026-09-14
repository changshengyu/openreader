# Reader desktop pagination device feedback P0 contract

Status: inventory-complete / implementation-pending

Fixed upstream baseline: `changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`

## Device feedback

Desktop text reading still differs from reader-dev when paging by click and moving with the mouse wheel. Existing smoke checks prove that both inputs work, but they do not prove the upstream hit regions, scroll host, or chapter-boundary behavior.

## Authoritative upstream surface

- `web/src/views/Reader.vue#eventHandler`: the center menu region is the middle 40% of both axes. In a vertical mode, the upper 30% pages backward and the lower 30% pages forward.
- `web/src/views/Reader.vue#nextPage/prevPage/scrollContent`: click paging moves the document scroll root by `windowSize.height - scrollOffset` over `animateMSTime`, using power-3 ease-in-out and rejecting overlapping click animations.
- `web/src/views/Reader.vue#activated/deactivated`: scroll observation is attached to `window`; reader-dev does not intercept wheel input to synthesize a page or chapter change.
- `web/src/plugins/config.js`: default vertical page animation is 300ms; `clickMethod` defaults to automatic.

## Compatibility matrix

| Layer | Fixed upstream | Current OpenReader at `42fcc27` | Decision |
|---|---|---|---|
| Desktop vertical scroll host | The browser document root owns vertical text scrolling. | Only mobile vertical text uses `createDocumentReaderScrollViewport`; desktop uses the nested `.reader-content` scroller. | `must-fix`; desktop text `page`, `scroll`, and `scroll2` must use the document root. |
| Automatic click regions | Center is 40% x 40%; vertical previous/next regions are the upper/lower 30%. | `ReaderClickZones` uses 35% / 30% / 35% vertically and 24% / 52% / 24% horizontally. | `must-fix`; use 30% / 40% / 30% on both axes, including the visible region overlay. |
| Click page distance and easing | One viewport minus two lines and two paragraph gaps, 300ms cubic ease-in-out by default. | `readerScrollStep` and `createReaderScrollAnimator` are semantically aligned. | `aligned`; retain the existing formula and animator. |
| Repeated click during animation | A second click is ignored while `transforming` is true. | The animator rejects a second animation while active. | `aligned`; retain. |
| Native wheel movement | No wheel interception; the browser scrolls the document continuously. Wheel input does not change chapters at a boundary. | Vertical wheel cancels click animation and calls previous/next page at the scroll boundary, which can change chapters. | `must-fix`; vertical wheel stays native and must not synthesize chapter navigation. Cancelling an active discrete animation when genuine native wheel movement begins is an allowed input-arbitration adaptation. |
| Chapter transition | Click at the end, the explicit chapter-end action, or navigation controls change chapters. | Click does this, but wheel boundary interception adds an extra path. | `must-fix`; remove only the wheel-only transition path. |
| Running chapter header | Upstream only projects the title into its mini top bar. | User explicitly requested the running chapter ordinal/title on desktop and mobile. | `user-requested acceptable-change`; preserve the fixed desktop header while restoring document scrolling. |
| Fixed formats | EPUB/audio have independent hosts and interaction rules; comics have image-specific behavior. | Separate effective-mode branches exist. | `out-of-scope`; do not move EPUB, audio, or comic readers to document scroll in this slice. |

## Required red tests

1. `shouldUseDocumentReaderScroll` selects the document root for desktop ordinary text in `page`, `scroll`, and `scroll2`, while retaining the existing fixed-format exclusions.
2. Vertical wheel input in the middle and at both boundaries never calls previous/next page and never prevents the native event.
3. Desktop automatic click geometry uses exact 30% previous, 40% center, and 30% next bands on the active axis.
4. A browser smoke at 1440x900 and 1024x1366 proves that the document root is the sole vertical scroll host, a lower/upper click moves exactly one configured step with the cubic animation, center clicks do not page, native wheel moves continuously, and wheel input at either chapter boundary does not change chapter.
5. Re-run mobile page, continuous reader, settings-position, inline-cache, EPUB, CBZ, audio, and TTS regression contracts because the scroll host utility is shared.

## Implementation boundary

- Preserve the user-requested difference: finger/wheel movement is native and continuous, while click movement remains discrete and animated.
- Preserve the user-requested running chapter header and numeric setting steppers.
- Do not change persisted mode names, progress payloads, chapter indexes, API behavior, or local-book data.
- Do not claim device closure until the published image is verified by the reporting user.
