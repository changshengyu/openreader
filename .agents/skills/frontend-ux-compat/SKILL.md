---
name: frontend-ux-compat
description: Frontend UX compatibility workflow for OpenReader. Use when changing Vue pages, routing, Pinia stores, reader UI, bookshelf, settings, import, WebDAV, sidebars, touch gestures, keyboard handling, or local persisted state.
---

# Frontend UX Compatibility

Use this skill for user-facing frontend changes.

## Workflow

1. Identify upstream reader-dev UI behavior and files.
2. Identify current OpenReader UI behavior and files.
3. Preserve routes and query parameters where possible; otherwise add compatibility redirects.
4. Preserve reading progress and local persisted state semantics.
5. Preserve keyboard, mouse, touch, and panel coexistence behavior for reader pages.
6. Document intentional UX differences.
7. Add or update unit tests and a real-browser smoke check.

## Reader mobile minimum

- Tool layer visible by default.
- Center tap toggles tool layer.
- Opening settings/catalog/bookmarks/source/shelf/search/cache/book info does not hide tools unless upstream does.
- Panel click does not pass through.
- Horizontal content whitespace remains symmetric within 1px.

## Checks

Run:

```bash
cd frontend && npm test
cd frontend && npm run build
```

Use `scripts/smoke/openreader-smoke.mjs` or an equivalent Playwright probe for affected flows.
