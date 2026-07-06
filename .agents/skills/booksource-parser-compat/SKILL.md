---
name: booksource-parser-compat
description: Book source and parser compatibility workflow for OpenReader. Use when changing online sources, CSS selectors, XPath-like rules, RSS, chapter parsing, content cleaning, import preview/import, or local book parsing.
---

# Book Source Parser Compatibility

Use this skill for parser behavior changes.

## Workflow

1. Preserve reader-dev parsing behavior unless the user explicitly accepts a difference.
2. Add fixture input:
   - HTML/XML for source and RSS parsing;
   - TXT/Markdown/EPUB/PDF/UMD samples for import;
   - JSON source rules when relevant.
3. Add expected parsed output.
4. Test selector, XPath-like rule, regex, charset, pagination, chapter splitting, and content cleanup behavior.
5. Keep remote fetchers bounded by timeout, size limit, and redirect limit.

## Regression focus

- Search result extraction.
- Book info extraction.
- Catalog extraction and sorting.
- Chapter content extraction and cleanup.
- Local TXT catalog detection and reparse/import token reuse.
