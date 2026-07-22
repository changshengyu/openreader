# Reader iPad primary-panel dismissal P0 contract

Date: 2026-07-19

Fixed upstream: `changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`

Status: shared implementation and automated frontend gates complete; real-browser touch and Docker
verification pending because the current local Chromium/Docker approval quota is unavailable.

This is a focused correction to `reader-ipad-responsive-p0-contract.md`. The shared width-only
`<=750px` adaptive decision remains correct, but the previous release did not prove the upstream
outside-tap close path on a touch iPad.

## Fixed-upstream evidence

- Wide adaptive iPad widths use the desktop Reader scene because `isMiniInterface()` depends only
  on `window.innerWidth <= 750`.
- Reader shelf, source, catalog and settings use click-triggered Element popovers whose reference
  tool remains above the reading content.
- Clicking the active reference again or clicking outside the popover closes it. The outside click
  is consumed by the popover interaction and must not page, toggle Reader chrome or reach content.
- The settings list remains bounded to `45vh`; shelf/source/catalog keep their 300px scroll region.

## Current regression audit

| Contract | Fixed upstream | Current OpenReader | Classification / action |
|---|---|---|---|
| Wide iPad scene | Desktop at 1024/1366 adaptive widths. | Correct since `39a5244`. | `aligned`; do not reopen the responsive predicate. |
| Same-tool close | Active desktop rail tool can close its panel. | Implemented and browser-tested. | `aligned`; preserve. |
| Outside touch close | Tap outside closes the popover without reaching Reader content. | `.reader-workspace-dismiss` and `ReaderClickZones` both declare `z-index: 2`. The filtered page currently creates another stacking context, but the intended order is implicit and the previous smoke closed panels only through the same rail tool. | **must-fix/test**: make the shared desktop stack order explicit and prove touch hit-testing. |
| Visible close path | Upstream relies on same-tool/outside click. | The desktop shared workspace has no visible close control; an iPad user reported that the upper panel could not be closed, and the same ambiguity affects all four primary panels. | **user-requested usability hardening**: add one always-visible, touch-sized close control to the shared desktop workspace while retaining both upstream close paths. |
| Panel bounds | Settings list is 45vh; other primary lists are 300px. | Correct inner bounds, but they do not compensate for a broken outside close target. | `aligned`; preserve sizing and scroll ownership. |
| All four primary panels | Shelf/source/catalog/settings share the same close behavior. | All use `ReaderDesktopWorkspacePanel`, so the defect is shared. | **must-fix once**; no per-panel CSS patches. |
| Phone/forced-mobile scene | Tool strip remains above the primary popover and click-away layer remains above content. | Mobile popover 10, dismiss 9, chrome 11, content click zones 2. | `aligned`; keep unchanged except shared tests may prove it. |

## Test-first requirements

1. A source contract must require a strict desktop stack order: content click zones below the
   dismissal surface, dismissal surface below the workspace, and workspace below the retained
   desktop rails.
2. Every desktop primary workspace must expose exactly one visible close button with an accessible
   name and at least a 44×44px touch target; activating it closes only the workspace.
3. Real browser at iPad Pro portrait `1024×1366` and landscape `1366×1024`, with touch enabled:
   open each of shelf/source/catalog/settings, dispatch a touch-like pointer tap in the visible
   outside region, and prove the panel closes while chapter/page/chrome state does not change.
4. Repeat explicit-close and same-tool close for every panel so the dismissal surface never blocks
   the rails and the close target never scrolls out of view.
5. Keep panel width, settings `45vh`, the 300px list bounds, and document horizontal overflow
   checks from the previous contract.
6. Re-run 390×844 and 360×800 mobile panel coexistence and click-away checks to prevent a desktop
   stacking fix from regressing the mobile scene.

## Implementation boundary

- Fix the shared desktop workspace stack; do not add iPad UA branches or duplicate close handlers.
- The dismissal surface must consume pointer/touch/click events and emit only `close`.
- Preserve route, progress, reading mode, scroll position and tool-layer state.
- The visible close control is an explicit user-requested usability improvement. It supplements,
  rather than replaces, upstream same-tool and outside-tap dismissal.

## Implementation evidence

- `ReaderDesktopWorkspacePanel.vue` now owns one fixed 44×44 close control shared by shelf, source,
  catalog and settings. It stays outside each panel's scroll region and consumes its pointer/click.
- Desktop stack order is explicit: content click zones z2, outside-dismiss z3, workspace z4 and
  retained left/right rails z5. Same-tool switching/closing therefore remains available.
- The settings `45vh` list, three 300px list viewports, adaptive `<=750px` decision and mobile
  popover structure were not changed.
- `readerDesktopPrimaryPopoverContract.test.mjs` first failed on the missing control and implicit
  stack, then passed after the shared fix. The merged Reader batch passes the full frontend suite
  509/509 and the production build; the only build note is the pre-existing Element Plus large-chunk warning.
- `reader-mobile-contract.mjs` now tests explicit touch close, outside touch close without changing
  scroll/page/route, and same-tool close for all four panels in iPad portrait and landscape. Real
  Chromium passes at iPad Pro `1024×1366` and `1366×1024`, phone `390×844` and `360×800`, and
  forced-mobile iPad `1024×1366` (`reader desktop/mobile/adaptive-iPad/forced-mobile-iPad contract
  smoke passed`).
