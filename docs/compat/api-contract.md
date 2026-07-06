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
