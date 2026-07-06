---
name: openreader-parser
description: Parser and import guardrails for OpenReader book sources, CSS selectors, XPath-like rules, RSS, TXT/EPUB/PDF/Markdown/UMD imports, chapter catalogs, cache, and content cleanup.
---

# OpenReader Parser

Use this skill when modifying source parsing, chapter parsing, import preview/import, RSS parsing, content cleanup, or cache generation.

## Compatibility rules

- Treat upstream `reader-dev` rules as semantic reference for book source fields and reader3-compatible APIs.
- Preserve existing local book import data and staged import tokens.
- Do not make catalog parsing depend on network speed once the file has been uploaded or staged.
- Reparse/import flows should reuse staged data instead of requiring repeated uploads.

## Remote source rules

- CSS selector and rule changes must be covered by tests with fixture HTML.
- Handle charset, headers, pagination, relative URLs, redirects, and empty results explicitly.
- Remote requests must respect timeout, size limit, redirect limit, and safe scheme/host rules.
- Do not let a single bad source block unrelated source searches.

## Local import rules

- TXT catalog detection must be deterministic for the same file and rule.
- EPUB/ZIP-style formats must guard against zip-slip and decompression bombs.
- PDF/UMD/TXT parsers must have size and time considerations; avoid unbounded reads into memory unless already bounded.
- Preview errors should preserve enough staged context for the user to adjust rules and retry.

## Required checks

Run targeted parser tests plus full backend tests:

```bash
cd backend && go test ./engine ./services/localbook ./api
cd backend && go test ./...
```
