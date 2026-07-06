---
name: openreader-security
description: Security review checklist for OpenReader changes involving auth, JWT, SSRF, remote fetch, uploads, WebDAV, archive extraction, parser DoS, path traversal, secrets, or multi-user data isolation.
---

# OpenReader Security

Use this skill for security-sensitive changes or reviews.

## Mandatory review areas

- SSRF: remote source URLs, RSS URLs, cover URLs, WebDAV import URLs, redirects, private IP ranges if reachable from server.
- Path traversal: local store, uploads, cache, library, backup download, WebDAV paths, archive entries.
- Secrets: JWT secret, WebDAV credentials, source headers/cookies, logs, backup files.
- Upload spoofing: filename extension, MIME assumptions, zip/epub entries, oversized files.
- Parser DoS: huge TXT/PDF/UMD files, decompression bombs, pathological regex, infinite pagination, unlimited redirects.
- Multi-user isolation: every user-owned read/write must scope by authenticated user ID unless intentionally admin/global.

## Required implementation posture

- Fail closed when a path, URL, token, or parser rule cannot be validated.
- Bound network and parser work by timeout and size.
- Do not log tokens, passwords, cookies, authorization headers, or WebDAV credentials.
- Do not expose host filesystem paths in normal API errors.
- Keep admin-only operations behind explicit role checks.

## Security handoff

For security-sensitive changes, update or reference `docs/security-review-checklist.md` and state which risks were checked.
