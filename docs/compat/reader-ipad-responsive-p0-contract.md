# Reader iPad / wide-touch responsive P0 contract

Date: 2026-07-19

Fixed upstream: `changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`

This pass reopens the iPad conclusion recorded for `b8b70f9`. That commit fixed a split state in
which OpenReader mounted mobile panels at iPad widths while mobile CSS remained hidden behind a
`750px` media query. It did not verify whether a wide iPad should have entered the mobile scene in
the first place. The reported iPad Pro failure shows that this distinction is user-visible: the
content-sized mobile settings and other primary panels occupy the upper part of the wide tablet,
and the expected desktop close/switch controls are replaced by the mobile interaction.

## Fixed-upstream evidence

- `web/src/plugins/helper.js#isMiniInterface()` returns only `window.innerWidth <= 750`.
- `web/src/App.vue` calls that function on startup and resize and commits the result through
  `setMiniInterface`.
- `web/src/plugins/vuex.js#setMiniInterface` uses that width result for adaptive mode; a
  non-adaptive page mode forces mini mode.
- `web/src/views/Reader.vue` therefore uses desktop rails and desktop popovers at iPad Pro CSS
  widths such as `1024` and `1366`; the four full-width mini popovers are only used at `750px` or
  below (or when the user explicitly forces the mobile page mode).
- Upstream `ReadSettings.vue` independently bounds the settings list at `45vh`; shelf, source and
  catalog use a `300px` inner scroll region.

## Current-state audit before implementation

| Contract | Fixed upstream | Current OpenReader | Classification / action |
|---|---|---|---|
| Adaptive breakpoint | Width-only, `window.innerWidth <= 750`. | `isMobileLikeViewport()` returns true for any mobile UA and for `MacIntel + maxTouchPoints > 1`, regardless of width. | **must-fix**: adaptive mode must be width-only. A wide iPad must not be forced into the mobile Reader or fullscreen workspace dialogs. |
| Explicit mobile mode | A non-adaptive page mode forces mini mode. | Persisted `pageMode === 'mobile'` forces mini mode. | **allowed runtime mapping / must-preserve**: users may explicitly request phone mode on an iPad. |
| Reader at 1024/1366px | Desktop rails, desktop progress and desktop primary popovers. | Mobile top/bottom chrome and full-width, top-anchored primary panels. | **wrong scene selection**: this is the direct cause of the reported upper-half overlay behavior. |
| Other dialogs at 1024/1366px | Non-fullscreen dialogs because mini mode is false. | `GlobalOverlayHost`, BookInfo, edit dialogs and SourceManager all consume the same UA-driven mini state and can become fullscreen/mobile on iPad. | **must-fix through the shared responsive predicate**, not one-off dialog CSS. |
| Narrow phone / narrow split view | Mini mode at `<=750px`. | Mini mode at `<=750px`. | **must-preserve** at 390×844, 360×800 and a 750px boundary. |
| Orientation / resize | Recomputed from current width. | Recomputed from current width, but UA keeps iPad permanently mini. | **must-fix**: 1024↔750 boundary changes scene once per resize without leaving both desktop and mobile controls mounted. |
| Mobile primary content bounds | Settings list `45vh`; shelf/source/catalog inner list `300px`. | Same inner bounds, but primary root/body use visible overflow. | **separate hardening**: phone-mode content must remain within the dynamic viewport and retain a reliable close path; this does not justify classifying a wide adaptive iPad as mobile. |

## Test-first requirements

1. Unit contract for the shared responsive predicate:
   - adaptive/automatic mode is desktop at `751`, `1024` and `1366` even with an iPad UA/touch;
   - adaptive mode is mobile at `750`, `390` and `360`;
   - explicit `mobile` mode remains mobile at `1024` and `1366`.
2. Replace the existing source test that encodes “wide iPad must be mini”; a regression test must
   instead lock the single shared width predicate used by Reader, Index and global dialogs.
3. Real-browser iPad Pro portrait `1024×1366` and landscape `1366×1024`, iPad UA + touch:
   - adaptive mode mounts desktop rails/progress and no mobile chrome;
   - shelf, source, catalog and settings open as bounded desktop workspaces and close from their
     visible desktop tool/close controls;
   - bookmark, search and BookInfo are bounded non-fullscreen dialogs with visible close controls;
   - no panel exceeds the visual viewport or creates horizontal overflow.
4. Real-browser 390×844 and 360×800 must retain the mobile toolbar, full-width primary panels,
   same-tool close, click-away close, scroll ownership and symmetric 16px text margins.
5. Explicit mobile mode at 1024×1366 must still mount the coherent mobile scene and every primary
   panel must remain closable. This is an allowed user-selected difference, not adaptive behavior.

## Implementation boundary

- Change the shared responsive decision; do not add iPad-specific CSS or UA exceptions to each
  Reader/dialog component.
- Preserve stored settings, routes, reading progress and all content-format behavior.
- The earlier semantic `mini-interface` class fix remains useful for explicit mobile mode at wide
  widths and must not be reverted.
- If phone-mode panel overflow is reproduced after the scene-selection fix, bound the common
  workspace root/body and keep scrolling inside the panel; do not hide the close control.
