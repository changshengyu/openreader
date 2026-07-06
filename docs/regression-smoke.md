# Browser Smoke Checks

Use this document with `openreader-regression` for UI changes.

## Script

```bash
TARGET_URL=http://127.0.0.1:8080 node scripts/smoke/openreader-smoke.mjs
```

For a specific reader page:

```bash
TARGET_URL=http://127.0.0.1:8080 \
SMOKE_READER_URL=http://127.0.0.1:8080/books/1/read \
node scripts/smoke/openreader-smoke.mjs
```

The script expects Playwright to be available in the current Node environment. If it is missing, install it in the environment used for smoke testing or run an equivalent browser probe.

## Required manual/automated coverage for reader work

- Desktop: `1440x900`.
- Mobile: `390x844`.
- Mobile: `360x800`.
- Check for blank page, console errors, and failed core API requests.
- For reader mobile, verify default tool layer, panel coexistence, center tap behavior, and symmetric content padding.
