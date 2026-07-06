# OpenReader Security Review Checklist

Use this checklist for security-sensitive changes and release reviews.

## Authentication and authorization

- [ ] `OPENREADER_JWT_SECRET` is required and not logged.
- [ ] Protected endpoints require valid JWT.
- [ ] Admin endpoints check admin role.
- [ ] User-owned rows are scoped by authenticated user ID.
- [ ] Batch operations cannot affect another user’s data.

## SSRF and remote fetches

- [ ] Source/RSS/cover/WebDAV remote URLs validate scheme.
- [ ] Redirect count is bounded.
- [ ] Request timeout is set.
- [ ] Response body size is bounded.
- [ ] Private network access is considered when server-side fetches are user-controlled.
- [ ] Headers/cookies are not logged.

## Path traversal and files

- [ ] Every user path is cleaned and joined under an allowed root.
- [ ] Final resolved path is verified to remain under the allowed root.
- [ ] Local store, uploads, cache, backups, and WebDAV all use rooted paths.
- [ ] Backup downloads only expose expected backup files.
- [ ] API errors do not leak host filesystem paths.

## Uploads and archive formats

- [ ] File size limits are enforced before expensive parsing.
- [ ] File extension/MIME assumptions are not trusted alone.
- [ ] EPUB/ZIP entries reject absolute paths and `..` traversal.
- [ ] Decompressed size and file count are bounded.
- [ ] Temporary staged import files are per user and cleaned after success/expiry.

## Parser DoS

- [ ] TXT/PDF/UMD/Markdown parsers avoid unbounded memory growth.
- [ ] Regex rules cannot trigger catastrophic work on large content without guardrails.
- [ ] Source pagination has a stop condition.
- [ ] A bad source cannot block unrelated searches indefinitely.

## Release note

For each release, record which checklist sections were relevant and which tests/probes covered them.

## EPUB iframe/resource review

Apply this section to Reader P0 EPUB work:

- [ ] The iframe URL never contains the login JWT or Authorization header value.
- [ ] The EPUB resource capability is signed with a purpose-separated key and is scoped to user, book, source fingerprint, read-only purpose, and expiry.
- [ ] Capability comparison/signature verification is constant-time through the standard crypto library.
- [ ] Invalid, expired, modified, stale-version, deleted-book, or ownership-changed capabilities fail closed.
- [ ] Capability path segments are redacted from application logs and never returned in error text.
- [ ] Every resource path is decoded once, normalized as a POSIX archive path, and verified below the scoped extraction root.
- [ ] ZIP entries reject absolute paths, drive prefixes, NUL bytes, `..`, symlinks, duplicate/conflicting paths, and writes through existing symlinks.
- [ ] Entry count, per-entry expanded size, and total expanded size are bounded before/during extraction.
- [ ] Extraction uses a staging directory and only exposes an atomically completed version.
- [ ] XHTML is served without EPUB-authored active scripts; the reader bridge is injected dynamically rather than written into the archived source.
- [ ] CSP blocks remote network loads and untrusted scripts while allowing scoped local CSS/images/fonts and required inline reader styles.
- [ ] MIME types are allowlisted and responses set `nosniff` and `no-referrer`.
- [ ] Multi-user tests prove one user's capability cannot read another user's book or resource tree.
- [ ] Parent `message` handlers verify both the active iframe window and expected same-origin resource origin.
