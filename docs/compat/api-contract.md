# OpenReader Compatibility API Contract

Status: working contract. Keep this file updated when endpoint semantics change.

## Global rules

- Public API root: `/api`.
- Auth: `Authorization: Bearer <jwt>` for protected `/api` endpoints.
- WebDAV root: `/webdav`.
- Sync WebSocket: `/ws/sync`.
- Expected error shape for handled failures: JSON object with `error`.
- User-owned resources must be scoped to the authenticated user unless documented as admin/global.

## Public endpoints

| Method | Path | Purpose | Compatibility notes |
|---|---|---|---|
| `GET` | `/api/health` | Health and build metadata. | OpenReader runtime addition; keep stable for Docker/probes. |
| `POST` | `/api/auth/register` | Create user; first user becomes admin. | OpenReader multi-user addition. |
| `POST` | `/api/auth/login` | Return JWT and user object. | OpenReader auth addition; invalid credentials return `401`. |

## Protected endpoint groups

| Group | Representative paths | Contract notes |
|---|---|---|
| User/settings/admin | `/api/me`, `/api/settings/:key`, `/api/admin/users` | Settings are per user. Admin endpoints require admin role. |
| Sources | `/api/sources`, `/api/sources/import`, `/api/sources/:id/test*` | Preserve reader3-compatible source fields and parser semantics. |
| Bookshelf | `/api/books`, `/api/books/:id`, `/api/books/batch`, `/api/books/export` | Book operations must not cross user boundaries. |
| Reader content | `/api/books/:id/chapters`, `/api/books/:id/chapters/:index/content` | Content fetch should use cache when valid and return stable chapter data. |
| Reader legacy search | `/api/reader3/searchBookContent` | Compatibility endpoint; keep until old clients/routes no longer need it. |
| Progress | `/api/progress/:bookID`, `/api/progress` | Progress writes must be conflict-safe and user scoped. |
| Bookmarks | `/api/books/:id/bookmarks`, `/api/bookmarks/:id` | Bookmark CRUD and batch operations remain user/book scoped. |
| Local store | `/api/local-store*` | All paths must stay rooted under configured local store/library paths. |
| Import | `/api/imports/books/preview`, `/api/imports/books`, `/api/imports/txt` | Preview may return `importToken`; import must be able to reuse staged content. |
| Uploads | `/api/uploads` | Uploaded assets must be validated and rooted under data uploads. |
| Cache | `/api/cache/stats`, `/api/cache`, `/api/books/:id/cache` | Cache operations must not delete unrelated user data. |
| Replace rules | `/api/replace-rules*` | Batch and test semantics should match upstream-visible replacement behavior. |
| RSS | `/api/rss/sources`, `/api/rss/articles` | Remote fetch limits and parser safety apply. |
| Explore | `/api/explore/sources`, `/api/explore/:sourceId` | Browse source catalogs with bounded pagination/fetch behavior. |
| Backup/WebDAV import | `/api/backup/*`, `/api/webdav/import-*` | Backup/restore must preserve existing data and report clear compatibility failures. |

## P1-D4 shelf-operation API contract

Status: extracted 2026-07-10. These routes retain their OpenReader paths while matching the fixed reader-dev shelf-operation behavior through a JWT/user-scoped adaptation.

| Method / path | Request | Success / side effects | Auth and errors |
|---|---|---|---|
| `PUT /api/books/:id/category` | `{ "categoryId": number }` or `{ "categoryIds": number[] }` | Replaces the shelf book's categories atomically, updates legacy primary `categoryId`, and emits one `bookshelf_update` after commit. The BookGroup set UI must not call this with an empty selection; direct API empty-array compatibility remains explicitly documented only if an ungrouped-book workflow needs it. | Owner only. `400` for malformed/foreign category, `404` for foreign/missing book, `500` only before an unsuccessful transaction can alter rows. |
| `POST /api/books/batch` | `{ "action": "delete"\|"category"\|"category-add"\|"category-remove"\|"cache"\|"clear-cache", "bookIds": number[], ... }` | Category and delete actions are transactional. Delete removes category links, chapters, bookmarks, progress, scoped browser-cache references and post-commit derived files; category actions emit one scoped `bookshelf_update`. Cache actions keep bounded request limits and emit affected shelf items only after durable cache state. | Owner only. Invalid/foreign category ids fail without mutation. Foreign book ids never expose or mutate another user's record; the response must distinguish an empty owned selection from a successful cross-user mutation. |
| `DELETE /api/books/:id` | None | Removes the caller's book rows in one transaction, broadcasts `bookshelf_delete` after commit, then prunes only that book's unreferenced remote cache files and private imported archive directory. | Owner only; `404` for another user's id. Failure before commit leaves all rows/files unchanged. Post-commit derived-cache cleanup is idempotent and must not delete another user's/shared path. |
| `POST /api/books/:id/cache` | `{ "chapterIndex"?: number, "all"?: boolean, "count"?: number }` | Remote books cache a bounded chapter window and return `{ cached, requested, book }`; local books return the existing no-server-cache result. A completed mutation publishes the refreshed shelf item. | Owner only; malformed payload `400`, missing book `404`. Count is bounded to protect server resources. |
| `GET /api/cache/stats` | None | Returns only the authenticated user's remote cache counts/size. The response never includes an absolute host cache path. | JWT required; it must not reveal another user's chapter count, filename, or root. |
| `DELETE /api/cache` | None | Clears only the authenticated user's remote chapter-cache references in a transaction, then removes only cache files left unreferenced by all chapter rows; emits a current-user shelf refresh after commit. | JWT required; no other user's database cache state or still-referenced file may be removed. |
| `POST /api/books/export` | `{ "bookIds": number[], "format": "txt"\|"epub"\|"json" }` | A single local book returns its archived original file. Remote books retain TXT/EPUB export. JSON and multi-book ZIP are explicit OpenReader extensions and remain user-scoped/bounded. | JWT required. Empty/foreign-only selections are client errors; safe `Content-Disposition` names must not expose host paths. |
| `POST /api/books/:id/refresh`, `POST /api/books/:id/refresh-local`, `POST /api/books/:id/change-source` | Existing route bodies | Replace chapter rows atomically. Only after commit, prune superseded derived caches while preserving `OriginalFile`, `chapters.json`, `bookSource.json`, local-store/WebDAV source files, and valid progress/bookmark recovery. Broadcast the merged shelf item after durable writes. | Owner only. Parse/fetch errors leave current catalogue/cache metadata usable and do not delete source files. |

The upstream uses namespace-specific JSON storage and SSE cache progress. OpenReader's REST/SQLite adaptation is allowed only where it preserves the visible action semantics, current-user isolation, durable event ordering, and bounded resource use described above.

## EPUB reader resource contract

### Authenticated chapter response

`GET /api/books/:id/chapters/:index/content`

| Field | Contract |
|---|---|
| Method/path | Existing `GET /api/books/:id/chapters/:index/content`; no path change. |
| Auth | Existing `Authorization: Bearer <jwt>` requirement. The book lookup remains scoped to the authenticated user. |
| Request | Existing numeric book ID and zero-based chapter index. |
| Text response | `200` JSON keeps `chapter` and `content`; adds `"format": "text"`. |
| EPUB response | `200` JSON keeps `chapter` and searchable plain-text `content`; adds `"format": "epub"`, `resourceUrl`, and RFC3339 `resourceExpiresAt`. |
| Side effects | For EPUB, may safely extract/rebuild a derived resource tree and backfill the chapter's canonical `resourcePath`. It must not alter the archived source EPUB. |
| `400` | Invalid book/chapter parameter. |
| `404` | Book/chapter/source archive is not available to the current user. |
| `422` | EPUB exists but is corrupt, unsafe, unsupported, or exceeds extraction limits. |
| `500` | Unexpected persistence or filesystem failure. |
| Error body | `{ "error": "<stable client-safe message>" }`; never include a host filesystem path or token. |

The EPUB additions are backward-compatible JSON fields. Existing clients that only consume `chapter` and `content` continue to work.

Example:

```json
{
  "chapter": {
    "id": 7,
    "bookId": 3,
    "index": 0,
    "title": "第一章"
  },
  "content": "第一章\n正文……",
  "format": "epub",
  "resourceUrl": "/api/epub-resource/<capability>/OEBPS/chapter-1.xhtml",
  "resourceExpiresAt": "2026-07-06T12:00:00Z"
}
```

### Capability-protected EPUB resources

`GET /api/epub-resource/:capability/*resourcePath`

| Field | Contract |
|---|---|
| Auth | Does not accept or require the login Bearer token. Authorization is the signed path capability returned by the protected chapter endpoint. |
| Capability scope | One user ID, one book ID, one source fingerprint/extracted version, read-only access, and a bounded expiration. It is signed with a purpose-separated key derived from `OPENREADER_JWT_SECRET`; it is never interchangeable with a login JWT. |
| Path | `resourcePath` is URL-decoded once, normalized as an EPUB POSIX path, and resolved strictly below that book/version's derived extraction root. |
| Success | `200` with a supported XHTML/HTML, CSS, image, SVG, or font MIME type. `HEAD` may return the same headers without a body. |
| XHTML | Dynamically receives the OpenReader iframe bridge and restrictive security headers. The archived/extracted source file is not modified in place. |
| Relative assets | The capability remains a stable path segment so relative chapter CSS/image/font/link URLs stay within the same authorized root. |
| `400` | Malformed capability or unsafe/malformed resource path. |
| `403` | Invalid signature, expired capability, wrong purpose, wrong archive version, or book ownership no longer matches. |
| `404` | Scoped book/resource no longer exists. |
| `415` | Resource media type is not on the EPUB reader allowlist. |
| Error body | JSON for API-style failures. Iframe failures remain non-blank because the parent detects the resource load failure and displays the reader retry state. |

Security headers include at minimum:

- `X-Content-Type-Options: nosniff`;
- `Referrer-Policy: no-referrer`;
- a CSP that permits only the injected bridge script and same-capability local styles/images/fonts/data resources;
- no permissive cross-origin credential policy.

The route must not log the capability value. Application access logs should redact the capability path segment.

## CBZ reader resource contract

### Authenticated chapter response

`GET /api/books/:id/chapters/:index/content`

| Field | Contract |
|---|---|
| Method/path | Existing `GET /api/books/:id/chapters/:index/content`; no path change. |
| Auth | Existing `Authorization: Bearer <jwt>` requirement. The book lookup remains scoped to the authenticated user. |
| Request | Existing numeric book ID and zero-based chapter index. |
| CBZ response | `200` JSON keeps `chapter` and `content`; adds `"format": "cbz"`, `resourceUrl`, and RFC3339 `resourceExpiresAt`. `content` remains compatible with the upstream image chapter shape and contains an `<img>` tag pointing at `resourceUrl`. |
| Side effects | May verify/recover the chapter `resourcePath` from the preserved archive. It must not modify the original CBZ archive. |
| `400` | Invalid book/chapter parameter or unsafe archive path. |
| `404` | Book/chapter/source archive/page is not available to the current user. |
| `415` | The selected CBZ entry is not a supported image media type. |
| `422` | CBZ exists but is corrupt, unsafe, unsupported, or exceeds parser/resource limits. |
| `500` | Unexpected persistence or filesystem failure. |
| Error body | `{ "error": "<stable client-safe message>" }`; never include a host filesystem path or token. |

The CBZ additions are backward-compatible JSON fields. Existing clients that only consume `content` will see upstream-style image HTML.

Example:

```json
{
  "chapter": {
    "id": 9,
    "bookId": 4,
    "index": 0,
    "title": "001.jpg",
    "resourcePath": "pages/001.jpg"
  },
  "content": "<img src=\"/api/cbz-resource/<capability>/pages/001.jpg\" />",
  "format": "cbz",
  "resourceUrl": "/api/cbz-resource/<capability>/pages/001.jpg",
  "resourceExpiresAt": "2026-07-06T12:00:00Z"
}
```

### Capability-protected CBZ image resources

`GET /api/cbz-resource/:capability/*resourcePath`

| Field | Contract |
|---|---|
| Auth | Does not accept or require the login Bearer token. Authorization is the signed path capability returned by the protected chapter endpoint. |
| Capability scope | One user ID, one book ID, one source fingerprint, read-only access, and a bounded expiration. It is signed with a purpose-separated key derived from `OPENREADER_JWT_SECRET`; it is never interchangeable with a login JWT or EPUB capability. |
| Path | `resourcePath` is URL-decoded once, normalized as a ZIP/POSIX path, and resolved strictly to an image entry inside that book's preserved CBZ archive. |
| Success | `200` with a supported image MIME type. `HEAD` may return the same headers without a body. |
| `400` | Malformed capability or unsafe/malformed resource path. |
| `403` | Invalid signature, expired capability, wrong purpose, wrong archive fingerprint, or book ownership no longer matches. |
| `404` | Scoped book/resource no longer exists. |
| `415` | Resource media type is not on the CBZ image allowlist. |
| Error body | JSON for handled failures. Reader displays a retryable chapter error rather than a blank page when the image cannot be resolved. |

Security headers include at minimum:

- `X-Content-Type-Options: nosniff`;
- `Referrer-Policy: no-referrer`;
- `Cross-Origin-Resource-Policy: same-origin`;
- private short-lived cache headers.

The route must not log the capability value. Application access logs should redact the capability path segment.

## Audio reader resource contract

### Authenticated chapter response

`GET /api/books/:id/chapters/:index/content`

| Field | Contract |
|---|---|
| Method/path | Existing `GET /api/books/:id/chapters/:index/content`; no path change. |
| Auth | Existing `Authorization: Bearer <jwt>` requirement. The book lookup remains scoped to the authenticated user. |
| Detection | Audio reading applies to books whose `type` is `1`, matching upstream `readingBook.type === 1`. |
| Audio response | `200` JSON keeps `chapter` and `content`; adds `"format": "audio"`, `resourceUrl`, and RFC3339 `resourceExpiresAt`. `content` remains the same audio URL string for clients that already read it directly. |
| Resource source | For remote audio chapters, the audio URL may remain a source-provided HTTP(S) URL if it is already safe for direct browser playback. For local/private library audio, return a same-origin signed resource URL. |
| Side effects | No text cache rewrite is required. Progress writes store the current playback second as the chapter offset, matching upstream `durChapterPos` behavior. |
| `400` | Invalid book/chapter parameter, unsafe audio URL, or malformed local resource path. |
| `404` | Book/chapter/source audio is not available to the current user. |
| `415` | Resource media type is not on the audio allowlist. |
| `500` | Unexpected persistence, source, or filesystem failure. |
| Error body | `{ "error": "<stable client-safe message>" }`; never include host filesystem paths, signed tokens, cookies, or source credentials. |

Example:

```json
{
  "chapter": {
    "id": 12,
    "bookId": 5,
    "index": 0,
    "title": "第一集"
  },
  "content": "/api/audio-resource/<capability>/tracks/001.mp3",
  "format": "audio",
  "resourceUrl": "/api/audio-resource/<capability>/tracks/001.mp3",
  "resourceExpiresAt": "2026-07-07T12:00:00Z"
}
```

### Capability-protected local audio resources

`GET /api/audio-resource/:capability/*resourcePath`

| Field | Contract |
|---|---|
| Auth | Does not accept or require the login Bearer token. Authorization is the signed path capability returned by the protected chapter endpoint. |
| Capability scope | One user ID, one book ID, one source fingerprint, read-only access, and a bounded expiration. It is signed with a purpose-separated key derived from `OPENREADER_JWT_SECRET`; it is not interchangeable with login, EPUB, or CBZ capabilities. |
| Path | `resourcePath` is URL-decoded once, normalized, and resolved strictly below that book's local library root or approved archive-derived resource root. |
| Local chapter resolution | A local/private audio chapter may identify its media file through chapter content, `chapter.url`, or `chapter.resourcePath`. Absolute filesystem paths and relative paths are accepted only after they resolve under the authenticated book's library root. Remote HTTP(S) URLs continue to use the safe direct remote contract above. |
| Success | `200` with a supported audio MIME type. `HEAD` returns the same client-relevant headers without a body. Byte-range requests return `206` with `Content-Range` when serving local files so browsers can seek efficiently. |
| `400` | Malformed capability or unsafe/malformed resource path. |
| `403` | Invalid signature, expired capability, wrong purpose, wrong source fingerprint, or book ownership no longer matches. |
| `404` | Scoped book/resource no longer exists. |
| `415` | Resource media type is not on the audio allowlist. |

Security headers include at minimum:

- `X-Content-Type-Options: nosniff`;
- `Referrer-Policy: no-referrer`;
- `Cross-Origin-Resource-Policy: same-origin`;
- private short-lived cache headers.

Remote audio URLs must be validated before they are returned to the browser: only HTTP(S), no embedded credentials, no JavaScript/data/file schemes, and no server-side credential leakage.

Implementation tests must cover:

- remote HTTP(S) audio remains unchanged and does not leak the login JWT;
- local/private audio chapter responses return `/api/audio-resource/<capability>/<path>`;
- `GET`, `HEAD`, and `Range` requests serve only allow-listed audio media under the scoped book library root;
- modified, expired, wrong-purpose, wrong-user/book, traversal, missing-file, and unsupported-media requests fail with client-safe errors;
- access logs redact `/api/audio-resource/<capability>/...` the same way EPUB/CBZ resource capabilities are redacted.

## WebDAV contract

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/webdav/*path` | List/download. |
| `PUT` | `/webdav/*path` | Upload/write. |
| `MKCOL` | `/webdav/*path` | Create directory. |
| `MOVE` | `/webdav/*path` | Rename/move. |
| `DELETE` | `/webdav/*path` | Delete. |

WebDAV paths must be normalized, rooted, and protected from traversal.

## Compatibility rule

If a refactor changes frontend routes, API paths should stay stable unless an old path is kept as a redirect/shim. Document removals before deleting compatibility behavior.
