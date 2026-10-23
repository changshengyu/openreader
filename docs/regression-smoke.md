# Browser Smoke Checks

Use this document with `openreader-regression` for UI changes.

## Script

Install the pinned crash-safe headless browser once after `npm install`:

```bash
npm install
npm run smoke:install-browser
```

All scripts use `scripts/smoke/playwright-runtime.mjs`. By default it launches Playwright's
Chromium Headless Shell, not the macOS system Google Chrome application. This avoids GUI app
registration crashes and keeps the user's normal Chrome profile untouched. `CDP_URL` and
`CHROME_PATH` are explicit overrides only.

```bash
TARGET_URL=http://127.0.0.1:8080 node scripts/smoke/openreader-smoke.mjs
```

For a specific reader page:

```bash
TARGET_URL=http://127.0.0.1:8080 \
SMOKE_READER_URL=http://127.0.0.1:8080/books/1/read \
node scripts/smoke/openreader-smoke.mjs
```

For the mocked mobile Reader contract:

```bash
TARGET_URL=http://127.0.0.1:5173 node scripts/smoke/reader-mobile-contract.mjs
```

The repository tooling package pins the Playwright version used by the shared runtime, independently
from the frontend dependencies copied into Docker. On macOS the browser process
may need to run outside a restricted sandbox so it can register its Mach rendezvous port.

## Release gate

`.github/workflows/docker-publish.yml` runs `scripts/smoke/release-browser-smoke.sh` right after the
native validation image is built, before the arm64 build and the publish step. The script starts a
fresh container from `openreader-ci:<sha>` (no volumes, Docker-assigned loopback port) and runs
`openreader-smoke.mjs` in the pinned headless shell: a page error, console error, `/api/` 5xx or empty
`#app` at any of the three viewports fails the release. `release-browser-smoke.test.mjs` checks the
workflow wiring and the checker's failure handling on owned pages.

Images are not built locally. To check a local frontend build in the release runtime, serve it
through a published image:

```bash
docker pull ghcr.io/changshengyu/openreader:latest
IMAGE=ghcr.io/changshengyu/openreader:latest SMOKE_PUBLIC_DIR=frontend/dist ./scripts/smoke/release-browser-smoke.sh
```

## Required manual/automated coverage for reader work

- Desktop: `1440x900`.
- Mobile: `390x844`.
- Mobile: `360x800`.
- Check for blank page, console errors, and failed core API requests.
- For reader mobile, verify default tool layer, panel coexistence, center tap behavior, and symmetric content padding.
