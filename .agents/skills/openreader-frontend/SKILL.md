---
name: openreader-frontend
description: Frontend guardrails for OpenReader Vue 3, Pinia, Element Plus, reader UI, Index workspace, mobile behavior, routing, and upstream visual/interaction alignment.
---

# OpenReader Frontend

Use this skill for frontend work in `frontend/`.

## Product contract

- Upstream `reader-dev` behavior is the baseline for visible flows.
- Current rewritten components, routes, composables, and tests are not proof of correctness.
- Before refactoring a module, map upstream files, state defaults, transitions, and visible layout.
- Old routes may remain as compatibility redirects; do not preserve wrong page structures as product architecture.

## Reader-specific rules

- Mobile reader tools are visible by default on entering the reader.
- Center content tap toggles tool visibility.
- Opening settings, catalog, bookmarks, source, shelf, search, cache, or book info must not hide the tool layer unless upstream does so for that exact branch.
- Panel clicks must not pass through to content and toggle tools.
- Mobile reader body must keep symmetric horizontal geometry and upstream typography semantics.
- Preserve user-requested improvement: native continuous finger/wheel scrolling while click paging remains paged.
- Preserve user-requested setting controls: minus/value/plus controls instead of high-mis-tap sliders.

## Index/workspace rules

- Re-align toward upstream `Index + Reader` scene structure.
- Keep one shared BookInfo flow across shelf, search, and reader.
- Keep sidebars, source management, local store, WebDAV, import, RSS, replace rules, and user/admin flows as workspace responsibilities unless a documented compatibility shim is needed.

## Required checks before handoff

Run from `frontend/`:

```bash
npm test
npm run build
```

For UI behavior changes, run a real-browser smoke check using `scripts/smoke/openreader-smoke.mjs` or an equivalent Playwright probe and record viewport coverage.
