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
