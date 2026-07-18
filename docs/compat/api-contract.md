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
| Sources | `/api/sources`, `/api/sources/import`, `/api/sources/:id/test*` | Preserve reader3-compatible source fields and parser semantics. Test endpoints keep their existing authenticated `200` response shape (`results`/`chapters`/`content` plus `error`), but may write `source_failures` only for remote request errors. Local unsupported or invalid parser rules remain visible in `error` and never suppress the source. |
| Bookshelf | `/api/books`, `/api/books/:id`, `/api/books/batch`, `/api/books/export` | Book operations must not cross user boundaries. |
| Reader content | `/api/books/:id/chapters`, `/api/books/:id/chapters/:index/content` | Content fetch uses a valid cache first and returns stable chapter data. On a remote cache miss, one single `nextContentUrl` that resolves to the adjacent catalog chapter is a chapter boundary, not continuation content. A blank text `contentRule` remains the existing client-safe `502` response but must not cache page HTML or create a `source_failures` row; audio sources retain their approved blank-rule media-URL behavior. |
| Reader legacy search | `/api/reader3/searchBookContent` | Compatibility endpoint; keep until old clients/routes no longer need it. |
| Progress | `/api/progress/:bookID`, `/api/progress` | Progress writes must be conflict-safe and user scoped. |
| Bookmarks | `/api/books/:id/bookmarks`, `/api/bookmarks/:id` | Bookmark CRUD and batch operations remain user/book scoped. |
| Local store | `/api/local-store*` | All paths must stay rooted under configured local store/library paths. |
| Import | `/api/imports/books/preview`, `/api/imports/books`, `/api/imports/txt` | Preview may return `importToken`; import must be able to reuse staged content. |
| Uploads | `/api/uploads` | Uploaded assets must be validated, rooted under data uploads and user-scoped for new writes/deletes; legacy global upload URLs remain readable. See [`bookinfo-shelf-mutations-p2-contract.md`](bookinfo-shelf-mutations-p2-contract.md). |
| Cache | `/api/cache/stats`, `/api/cache`, `/api/books/:id/cache` | Cache operations must not delete unrelated user data. |
| Replace rules | `/api/replace-rules*` | See the P2 replace-rule contract below: stable name-upsert order and upstream-visible plain/regex/scope semantics. |
| RSS | `/api/rss/sources`, `/api/rss/articles` | Remote fetch limits and parser safety apply. |
| Explore | `/api/explore/sources`, `/api/explore/:sourceId` | Browse source catalogs with bounded pagination/fetch behavior. |
| Backup/WebDAV import | `/api/backup/*`, `/api/webdav/import-*` | Backup/restore must preserve existing data and report clear compatibility failures. |

## P1-B workspace search API contract

Status: implemented for the P1-B search-default/error slice on 2026-07-13 from fixed reader-dev `Index.vue`, `config.js`, and `BookController.kt`. OpenReader keeps its authenticated REST path and source-ID representation, but restores the upstream search defaults and error semantics.

| Method / path | Request | Success / side effects | Errors / compatibility adapter |
|---|---|---|---|
| `POST /api/search` | Authenticated JSON `{keyword, sourceIds?, concurrentCount?, page?, lastIndex?, searchSize?}`. `sourceIds` is the current user-scoped adaptation of upstream all/group/single source selection. | A single selected source uses `page`; multiple selected sources use `lastIndex` as a cursor over the original ordered `sourceIds` sequence. Response stays `{list,page,lastIndex,hasMore}`. Multi-source results are deduplicated; individual failed sources are recorded/skipped under the existing source-failure contract. Failure-suppressed sources are skipped without renumbering `lastIndex`; if every otherwise configured source is currently suppressed, this remains a successful empty result. | Blank keyword is `400 {error:"keyword is required"}`. No enabled/selected source must return a handled error whose frontend semantics are **“未配置书源”**, rather than a successful empty list. Existing direct clients retain the top-level `{error}` shape. |

The omitted/zero `concurrentCount` default is **24**. This is the upstream workspace default; a positive caller-provided count remains bounded by the selected source count. OpenReader does not add a `/reader3/searchBookMulti` product dependency: the frontend maps upstream `multi`/`bookSourceGroup` to ordered `sourceIds` and keeps the deployed REST response shape.

## P2-Parser-3A source-script error contract

Status: implemented and verified on 2026-07-13 against fixed reader-dev `BaseSource.kt`, `AnalyzeUrl.kt` and `WebBook.kt`. This is a Go/JWT security adaptation, not a route redesign.

| Affected existing route family | Trigger | Required response and side effect |
| --- | --- | --- |
| Search and explore (`/api/search`, `/api/explore/*`) | Source `header` starts with `@js:`/`<js>` or `loginCheckJs` is non-blank. | Keep the route's current status and top-level `error`; append `code: "source_rule_unsupported"` and `stage: "search"` or `"explore"`. Reject before any remote request and do not create a source-failure cache row. |
| Remote add/refresh/change-source and reader catalogue | The same source-script trigger. | Keep existing route status and `error`; append `code: "source_rule_unsupported"`, `stage: "book_info"`. No remote request, cache mutation or source-failure row. |
| Reader chapter content | The same source-script trigger. | Keep the existing `502` response and `error`; append `code: "source_rule_unsupported"`, `stage: "content"`. No remote request, chapter-cache write or source-failure row. |
| Source debug (`/api/sources/:id/test*`) | The same source-script trigger. | Retain authenticated `200` debug envelopes with their existing result field plus redacted `error`, `code: "source_rule_unsupported"` and the relevant stage. No remote request and no source-failure row. |

The response must never contain the script, source header, cookie, URL query, remote response body or a host path. Static JSON headers remain supported. `preUpdateJs`, `content.webJs`, option `webJs`, and `sourceRegex` are preserved but are not included in this trigger because the fixed upstream call graph does not consume them; a later implementation needs a fresh contract.

## P2 invalid-source cache API contract

Status: implemented and tested on 2026-07-12 from fixed reader-dev `BookController.kt`, `Index.vue` and `vuex.js`. See [source-failure-cache.md](source-failure-cache.md) for data and state details.

| Method / path | Request | Success / side effects | Auth and errors |
|---|---|---|---|
| `GET /api/sources/invalid` | None. | Returns `[]` or current-user, unexpired source records merged with `{errorMessage,failedAt,expiresAt}`. It never starts a source request. Expired/deleted/edited-source rows are pruned/ignored. | JWT required. `401` invalid/missing session; `500` only before a database read can complete. No source credentials, query string, remote response body, host path or internal error appears in `errorMessage`. |
| `POST /api/reader3/getInvalidBookSources` | No body. | Compatibility adapter for the same caller-scoped 600-second failures; no new frontend flow may depend on this legacy path. | JWT required and same isolation/error rules as the canonical route. |

Source-facing routes retain their current response schemas. Only a real remote source-request failure may create/update exactly one current-user failure cache row after its request has failed; a blank result, a rule syntax/unsupported-rule/configuration error, and a client-cancelled context do not. During its 600-second TTL the same caller's normal multi-source search/candidate flow skips that source, while an explicit health check may still probe it and may record a configuration failure for its health result.

## P2 backup restore archive contract

Status: implemented on 2026-07-12 after comparison with reader-dev `Index.vue`, `WebDAV.vue`, `WebdavController.kt`, and `UserController.kt`. The upstream confirmation-before-restore workflow and both reading-app and OpenReader JSON export formats remain unchanged. OpenReader keeps its JWT/WebDAV API adaptation and adds server-side bounds before any persisted data is changed.

| Method / path | Request | Success / side effects | Errors / safety contract |
|---|---|---|---|
| `POST /api/backup/restore-legado` | Authenticated multipart field `file`; filename must end in `.zip` (case-insensitive). | `200` preserves existing result counts for sources, RSS, settings, categories, shelf, progress, bookmarks, and replacement rules. All historical JSON locations (`myBookShelf.json`, `bookshelf.json`, nested `bookProgress/`, and OpenReader files) remain readable. | `400 {error}` for missing/non-ZIP/invalid archive, `413 {error}` when compressed input exceeds `OPENREADER_MAX_BACKUP_RESTORE_BYTES` (128 MiB default). Archive headers and fully bounded reads are checked before the first database update: unsafe traversal, symlink, duplicate normalized paths, excessive member count, member size, or total expansion fail with no partial restore. |
| `POST /api/backup/restore-webdav` | Authenticated JSON `{path}`; the caller-scoped WebDAV path must reference a `.zip` file. | Same restore and count semantics as uploaded restore; the existing UI confirmation remains the overwrite/restore decision point. | `400` if file/path is missing, directory, non-ZIP, or archive validation fails; `413` for an oversized file. The response never exposes server paths or ZIP parser details. |

Configuration defaults: `OPENREADER_MAX_BACKUP_RESTORE_BYTES=134217728`, `OPENREADER_MAX_BACKUP_ARCHIVE_ENTRIES=5000`, `OPENREADER_MAX_BACKUP_ARCHIVE_ENTRY_BYTES=16777216`, and `OPENREADER_MAX_BACKUP_ARCHIVE_EXPANDED_BYTES=134217728`. These are an allowed OpenReader security improvement; they do not change the exported data schema or user-visible restore sequence.

### P1-E4 portable local archive extension

Status: implemented. Reader-dev has no local-file backup format, so this is a deliberately named
OpenReader extension rather than a change to either row above. The full format, scoped-root,
collision, staged-restore and limit contract is
[`portable-local-archive-backup-p1e4-contract.md`](portable-local-archive-backup-p1e4-contract.md).

| Method / path | Request | Success / side effects | Errors / compatibility |
|---|---|---|---|
| `POST /api/backup/portable/trigger` | Authenticated, no body. | Writes caller-scoped `portable_backup_*.zip`; returns `{message,path,name,format:"openreader-portable-v1",localBooks}`. The package retains the ordinary logical JSON entries and adds the v1 manifest plus verified private original archives. | `409` for missing/unsafe archive or Type=1 local audio, `413` for a configured output/archive limit, no host path in errors. `POST /api/backup/trigger` remains logical only. |
| `GET /api/backup/list`, `GET /api/backup/download/:name` | Existing authentication/name path. | List adds the optional `format` (`logical` or `openreader-portable-v1`); download accepts the existing `backup_*.zip` and additive `portable_backup_*.zip` only in the caller root. | Existing clients may ignore `format`; traversal, other prefixes and cross-user files remain rejected. |
| `POST /api/backup/restore-legado`, `POST /api/backup/restore-webdav` | Existing multipart or scoped WebDAV request. | Manifest detection dispatches v1 packages to full logical + local archive recovery and returns additive `localBooks`. Legacy reader-dev/Legado/OpenReader ZIPs retain their original restore path. | Unknown/invalid v1, collision, hash, ZIP/path or parse failure returns a client-safe error before package-controlled data mutation; a portable identity collision is `409`. |

Configuration defaults are additive: `OPENREADER_MAX_PORTABLE_BACKUP_BYTES=536870912`,
`OPENREADER_MAX_PORTABLE_ARCHIVE_ENTRIES=10000`,
`OPENREADER_MAX_PORTABLE_ARCHIVE_ENTRY_BYTES=268435456`, and
`OPENREADER_MAX_PORTABLE_ARCHIVE_EXPANDED_BYTES=536870912`. Portable archive parsing uses this
independent per-file bound rather than the smaller interactive-upload cap; normal imports retain
their own `OPENREADER_MAX_IMPORT_BYTES` policy.

## P2 UserManage API contract

Status: implemented for the account/permission/deletion slice on 2026-07-17 from fixed reader-dev `UserManage.vue`, `AddUser.vue`, and `UserController.kt`. Book-source ownership actions remain a separate P2 dependency because OpenReader's source table is global.

| Method / path | Request | Success / side effects | Auth and errors |
|---|---|---|---|
| `GET /api/admin/users` | None. | Returns manager-visible rows with stable current fields: `id`, `username`, `role`, limits/capabilities/counts, `lastActiveAt`, and `createdAt`. | JWT administrator only. A non-admin gets `403` `{"error":{"code":"FORBIDDEN","message":"admin access required"}}`; no user rows leak. |
| `POST /api/admin/users` | `{username,password,canEditSources?,canAccessStore?,canAccessWebdav?,bookLimit?,sourceLimit?}`. `role` may be absent or `user`; `admin` is rejected. | `201` creates exactly one ordinary user and broadcasts one `users_update` after commit. New LocalStore/WebDAV permissions default to `true` unless explicitly set; results expose effective `canAccessStore` and `canAccessWebdav`. Existing administrator rows are never changed/migrated. | Administrator only. Username must be at least 5 ASCII letters/digits and not `default`; password must be at least 8 characters. Invalid input/role assignment `400`; duplicate `409`. |
| `PUT /api/admin/users/:id` | Any explicit subset of ordinary-user capability/limit fields, including independent `canAccessStore` and `canAccessWebdav`. | Updates only supplied fields, returns effective permissions, then broadcasts one post-commit update. Updating either workspace permission never changes the other. | Administrator only. `403` when `:id` is an administrator, `404` missing id, `400` malformed body. |
| `PUT /api/admin/users/:id/password` | `{password}` with at least eight characters. | Changes one ordinary user's password and broadcasts once. | Administrator only. `403` for administrator target, `404` missing id, `400` invalid password/body. |
| `POST /api/admin/users/batch-delete` | `{ids:number[]}`. | Deletes only selected ordinary users and every user-owned SQLite row in one transaction. Only after commit it removes the validated private WebDAV, LocalStore, imported-library, and upload descendants; response includes safe numeric `cleanupFailures` without a host path. Emits one update after commit. | Administrator only. `400` empty/protected-only input; the current user and every administrator are excluded. A post-commit cleanup failure never rolls back the completed row deletion. |
| `POST /api/admin/cleanup-inactive` | None. | Compatibility-only background action: finds inactive ordinary users and calls the same complete deletion plan; it is not exposed in the upstream-aligned UI. | Administrator only; administrators are never deleted. |

The upstream has user-specific source-default/delete actions. OpenReader's source table is intentionally global, so no equivalent endpoint is added; source editing remains governed by `canEditSources` and existing source routes. All handled errors retain the global `{error}` shape.

## P0 local TXT preview and staged-reparse contract

Status: TXT matching and raw-byte token reuse were implemented on 2026-07-11. The parsed-snapshot and latest-request-only lifecycle correction identified on 2026-07-18 is now implemented and validated without changing the response schema; see [`local-book-import-catalog-p0-contract.md`](local-book-import-catalog-p0-contract.md).

| Method / path | Request | Success and side effects | Errors / retry contract |
|---|---|---|---|
| `POST /api/imports/books/preview` | Multipart `file`, optional `title`, `author`, `tocRule`, or an existing `importToken` instead of `file`. JWT required. | A new upload creates a caller-scoped immutable stage before parsing. Successful response is `200` with `{title,author,chapterCount,chapters,importToken}`. The server atomically records the bounded parsed result for that token and exact rule without changing the public response. An empty `tocRule` uses the automatic first-512,000-byte probe and may return pseudo chapters for text without a TOC. An explicit TXT `tocRule` with no match is likewise a normal `200` empty catalogue, retaining the token for a rule refresh or empty-catalog confirmation. EPUB preview records catalogue metadata/resource boundaries only; it does not materialize every XHTML body merely to return titles. | Unsupported/invalid input remains `400` `{error,importToken}`. A failed reparse must not replace the last successful parsed snapshot. No durable book rows/library archives are created during preview. |
| `POST /api/imports/books` | Same multipart/token fields plus existing category fields. JWT required. | `201` creates the book only from the staged bytes or submitted upload. When token/rule/hash match a successful preview, confirmation consumes that parsed catalogue instead of executing the catalogue parser again. Old full-content snapshots and stages without a snapshot remain accepted/rebuilt lazily. EPUB may prepare its caller-owned bounded derived resource tree below the new archive before commit so the first Reader request does not rehash/re-extract it. A consumed staged token and all derived snapshot files are deleted only after a successful import. A local TXT book with an explicit unmatched rule is allowed to persist with zero chapter rows, matching upstream local-book confirmation semantics. | `400` for unsupported/parser/archive-policy extraction errors; the client-safe extraction message cannot expose a host path. Host storage/database failures remain generic `500`. Failed confirmation leaves an existing stage and its last successful snapshot reusable until normal expiry. No book row, broadcast, consumed token or orphan new library directory may survive a failed confirmation. |
| `POST /api/local-store/import-preview`, `POST /api/webdav/import-preview` | JSON `{paths}` or `{items:[{path,title,author,tocRule}]}`. JWT/store permission required. | `200 {items}`. Each readable item is copied once to a caller-scoped immutable stage; success item contains `{path,book,importToken}`. | A parser failure remains an item-level `{path,error,importToken}` in the `200` envelope. The token remains valid for a later `{items:[{path,importToken,tocRule}]}` preview/import; mounted-file mutation/removal cannot affect that retry. |
| `POST /api/local-store/import`, `POST /api/webdav/import` | Existing paths/items/category body. | Successful staged item uses the original preview bytes and deletes its token after durable import. | Item-level parser failures retain the staged token and do not create book/cache rows. A container response does not expose paths outside the caller's scoped store. |

## P1-E3 workspace file-manager API compatibility

Status: implemented and regression-tested on 2026-07-13. The fixed reader-dev contract is represented by the P1-E3 workspace UI; existing OpenReader paths remain available for deployed clients even when their corresponding operation is removed from the new workbench.

| Method / path | Workbench request / response contract | Compatibility and security rule |
|---|---|---|
| `GET /api/local-store?path=<relative>` | The rebuilt LocalStore sends only the current relative directory and expects `{path,items}`. Every item includes `name`, `path`, `size`, `lastModified`, `isDir`, and an internal parser-capability `importable` flag. | The normal UI never sends `recursive`; hidden dot-name entries are omitted. A legacy client may still send `recursive=1` until separately retired. All paths remain caller-rooted. |
| `POST /api/local-store/upload` | Authenticated multipart body has `path` plus one or more `file` parts. A successful multi-file write returns `201 {paths:[...]}` in submitted order; one-file callers also receive the stable `path` first item field during the compatibility window. | Upload is file management, not parser admission: any basename-safe regular file may be stored. Each file remains bounded and atomically staged beside its target; invalid/oversized data returns a client-safe `400`/`413` and must not truncate an existing destination. |
| `POST /api/local-store/directory`, `PUT /api/local-store/rename`, `GET /api/local-store/download` | Existing authenticated request/response shapes stay unchanged. | They are legacy/API compatibility routes only; P1-E3 LocalStore UI must not create these calls. |
| `PUT /webdav/<path>` | The rebuilt WebDAV picker performs one authenticated, bounded PUT per selected file and refreshes the current directory after all successful writes. | Multiple visible selections are an UI-level sequence over the existing raw WebDAV contract. Every PUT retains normal bearer auth, caller-rooting and atomic staging. |
| `MKCOL /webdav/<path>`, `MOVE /webdav/<path>` | Existing raw WebDAV compatibility methods remain unchanged. | P1-E3 WebDAV UI must not surface new-directory or rename controls. |
| `POST /api/local-store/import-preview`, `POST /api/webdav/import-preview` | The P1-E2 controller may only be launched by reader-dev-visible suffixes: LocalStore `txt/epub/umd/cbz`, WebDAV `txt/epub/umd`. | Go can retain additional direct-parser formats for existing books/clients, but they cannot reappear as a workbench button without a later documented product decision. |

This API shape is an allowed Go/JWT adaptation: reader-dev returns an empty chapter list in its controller response for a local `TocEmptyException`; OpenReader preserves deployed REST paths and represents the same explicit no-match case as a successful empty preview. Actual malformed/unsupported parser failures remain `400` while preserving retryable staged context in direct and storage-backed flows.

## P2 replace-rule API contract

Status: extracted 2026-07-11 from fixed `reader-dev` `ReplaceRuleController.kt`, `ReplaceRule.vue`, `ReplaceRuleForm.vue`, and `Reader.vue`. OpenReader keeps REST/SQLite/JWT routes but must preserve the user-visible rule pipeline.

| Method / path | Request and validation | Success / side effects | Auth and errors |
|---|---|---|---|
| `GET /api/replace-rules` | None. | Returns only the caller's rules in stable insertion order (`id ASC`), never update-time order. Compatibility output retains `enabled` plus legacy-readable `isEnabled`. | JWT required; `500` only for a read failure. |
| `POST /api/replace-rules` | `{name, pattern, replacement, scope, isRegex, enabled|isEnabled}`. Name, pattern and scope are required; a missing `isRegex` means plain text; regex must compile under the reader-compatible case-insensitive mode. | Current-user name-upsert. Appending returns `201`; replacing the existing same-name row in place returns `200`, without moving pipeline order. Emits `replace_rules_update` after commit. | JWT required; `400` for missing fields/invalid regex; no cross-user lookup. |
| `PUT /api/replace-rules/:id` | Same validated fields. | Updates only the owned ID and does not change its stable position. Emits one post-commit update event. | JWT required; `400` invalid body/regex, `404` missing/foreign ID, `409` when renaming to another existing current-user name. |
| `POST /api/replace-rules/batch` | JSON array. Blank name/pattern rows retain the upstream-compatible `skipped` result. Every accepted rule must have a scope and valid plain/regex mode before any accepted row is written. | Transactional current-user name-upsert in input order, returning `{rules,created,updated,skipped}`. A malformed regex rejects the batch without a partial accepted-row write. | JWT required; `400` malformed array/regex/scope, `500` before a failed transaction can mutate state. |
| `POST /api/replace-rules/test` | `{pattern,replacement,isRegex,text}` using the same compiler/mode as real Reader content. | Returns `{input,output,changed}` only; no persistence or sync event. | JWT required; `400` invalid regex or missing pattern/text. |
| `DELETE /api/replace-rules/:id`, `POST /api/replace-rules/batch-delete` | Existing ID paths/payload. | Delete only owned rows, retain ordered `deletedIds`, and emit after durable deletion. | JWT required; single missing/foreign ID is `404`; invalid empty batch is `400`. |

Reader content applies enabled matching rules only to text chapters, in the same listed order: plain text changes the first occurrence; regex changes every case-insensitive occurrence. EPUB and audio content bypass the pipeline. A legacy persisted empty scope remains global only to avoid breaking existing OpenReader data; any successful edit/import writes an explicit non-empty scope.

## P1-D4 shelf-operation API contract

Status: extracted 2026-07-10. These routes retain their OpenReader paths while matching the fixed reader-dev shelf-operation behavior through a JWT/user-scoped adaptation.

| Method / path | Request | Success / side effects | Auth and errors |
|---|---|---|---|
| `PUT /api/books/:id/category` | `{ "categoryId": number }` or `{ "categoryIds": number[] }` | Replaces the shelf book's categories atomically, updates legacy primary `categoryId`, and emits one `bookshelf_update` after commit. The BookGroup set UI must not call this with an empty selection; direct API empty-array compatibility remains explicitly documented only if an ungrouped-book workflow needs it. | Owner only. `400` for malformed/foreign category, `404` for foreign/missing book, `500` only before an unsuccessful transaction can alter rows. |
| `POST /api/books/batch` | `{ "action": "delete"\|"category"\|"category-add"\|"category-remove"\|"cache"\|"clear-cache", "bookIds": number[], ... }` | Category and delete actions are transactional. Delete removes category links, chapters, bookmarks, progress, scoped browser-cache references and post-commit derived files; category actions emit one scoped `bookshelf_update`. Cache actions keep bounded request limits and emit affected shelf items only after durable cache state. | Owner only. Invalid/foreign category ids fail without mutation. Foreign book ids never expose or mutate another user's record; the response must distinguish an empty owned selection from a successful cross-user mutation. |
| `DELETE /api/books/:id` | None | Removes the caller's book rows in one transaction, broadcasts `bookshelf_delete` after commit, then prunes only that book's unreferenced remote cache files and private imported archive directory. | Owner only; `404` for another user's id. Failure before commit leaves all rows/files unchanged. Post-commit derived-cache cleanup is idempotent and must not delete another user's/shared path. |
| `POST /api/books/:id/cache` | `{ "chapterIndex"?: number, "all"?: boolean, "count"?: number }` | Remote books cache a bounded chapter window and return `{ cached, requested, book }`; local books return the existing no-server-cache result. A completed mutation publishes the refreshed shelf item. | Owner only; malformed payload `400`, missing book `404`. Count is bounded to protect server resources. |
| `POST /api/books/:id/cache/stream` | Same bounded cache body as `/cache`; authenticated `fetch` request with a readable response body. | Returns `text/event-stream`: each `message` event carries `{ bookId, cached, requested, total, chapterIndex, failed }`; terminal `end` carries `{ bookId, cached, requested, failed, book }`; a terminal `error` carries a client-safe error. Aborting the authenticated request is the cancellation action: the server stops scheduling further chapter fetches and does not send a shelf update for an incomplete job. The client must not put a JWT in a query string. | Owner only. Validation failures are ordinary JSON `400`/`404` before the stream opens. A disconnected/cancelled request is not an API error and leaves already-written bounded cache entries usable. |
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
| EPUB response | `200` JSON keeps `chapter` and searchable plain-text `content`; adds `"format": "epub"`, `resourceUrl`, and RFC3339 `resourceExpiresAt`. Plain text may be generated and cached for only the requested chapter; Reader iframe preparation must not wait for unrelated chapter text. A fragment chapter keeps canonical `chapter.resourcePath` plus nullable `resourceFragment`/`resourceEndFragment`; its `resourceUrl` includes an encoded `#resourceFragment` for iframe location. |
| Side effects | For EPUB, may safely reuse/extract/rebuild a derived resource tree, lazily create only the requested chapter's text cache, and backfill canonical `resourcePath` plus nullable fragment metadata. A valid complete marker whose source size/mtime still match may avoid a redundant SHA-256 pass; any identity change or invalid marker must rehash/rebuild and invalidate the old capability. It must not alter the archived source EPUB. |
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
| Capability scope | One user ID, one book ID, one source fingerprint/extracted version, read-only access, and a bounded expiration. For a fragment chapter it additionally signs the canonical XHTML path and nullable start/end fragment ids used to slice that one document; it is never interchangeable with a login JWT. |
| Path | `resourcePath` is URL-decoded once, normalized as an EPUB POSIX path, and resolved strictly below that book/version's derived extraction root. |
| Success | `200` with a supported XHTML/HTML, CSS, image, SVG, or font MIME type. `HEAD` may return the same headers without a body. |
| XHTML | Dynamically receives the OpenReader iframe bridge and restrictive security headers. When the capability is for a fragment chapter, the served document contains only the upstream-equivalent start/end DOM range; sibling CSS/image/font resources retain the same capability root and are not sliced. The archived/extracted source file is not modified in place. |
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

### CBZ bookshelf and book-detail cover projection

`POST /api/imports/books`, `POST /api/local-store/import`, `POST /api/webdav/import`,
`GET /api/books`, `GET /api/books/:id`, and any existing `bookshelf_update` payload retain their
current book/book-list JSON shapes. For a local CBZ with no `customCoverUrl`, the existing
`coverUrl` field is projected at response time to
`/api/cbz-resource/<capability>/<first-safe-archive-image>`.

The source image is the first safe image encountered in CBZ archive order, matching
reader-dev `CbzFile.parseBookInfo`; it is intentionally independent of the lexicographically
sorted chapter catalogue. The response capability is bound to the current user, book and
archive fingerprint, expires normally, and remains readable without appending the login JWT.
`coverUrl` capability values and archive member paths are **not** written to `books`,
`chapters.json`, `bookSource.json`, backups, WebDAV metadata, or logs. A user-supplied
`customCoverUrl` remains the frontend's first-choice cover and is never overwritten.

If the archived CBZ is unavailable, malformed, unsafe, over budget, or has no supported image,
the stable book endpoint stays successful with its stored/empty `coverUrl`; it must not turn a
normal bookshelf response into an archive or host-path error.

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

WebDAV paths must be normalized, rooted, and protected from traversal. Every raw WebDAV method requires the standard `Authorization: Bearer <JWT>` header: missing/invalid credentials return `401` before filesystem access; an authenticated user whose effective `canAccessWebdav` permission is false receives `403` before path parsing or file mutation. The browser uses header-based authenticated requests and must never append a JWT to a download URL.

## Workspace storage access contract

`/api/local-store*` requires authenticated `canAccessStore`; raw `/webdav/*`, `/api/webdav/import*`, and `/api/backup/*` require authenticated effective `canAccessWebdav`. A missing/invalid token returns `401`; the relevant disabled capability returns `403`; handlers must perform that check before validating a supplied path, parsing a multipart body, reading an archive, or creating a backup. Direct `/api/imports/books*` remains an authenticated bookshelf action and is not made dependent on either workspace-entry permission.

Storage resolves without destructive migration: administrators continue to use the existing LocalStore/WebDAV roots so mounted legacy files remain readable; regular users resolve below `users/<safe-username>/` within the same mounts. Generated backup list/download/restore follows that same scope, and scheduled backups are generated per user. Direct LocalStore/WebDAV imports may carry a user-scoped `importToken` returned by preview; on confirmation it authoritatively selects the immutable staged bytes rather than rereading the mutable storage path.

`POST /api/auth/register` uses the same new-account input rule as manager-created
users. This does not invalidate existing data: already-persisted accounts with a short
password or legacy username remain able to use `POST /api/auth/login`; validation runs
only when creating a new account. The protected `admin` first-account behavior remains
an allowed OpenReader runtime adaptation.

## Bookmark contract

| Method / path | Request | Success / side effects | Auth and errors |
|---|---|---|---|
| `GET /api/books/:id/bookmarks` | None | Lists the caller-owned book's independent bookmarks in stable `id ASC` creation order. | JWT and book ownership required; foreign/missing book is `404`. |
| `POST /api/books/:id/bookmarks` | Reader location plus non-empty `excerpt`/paragraph context and optional `chapterId`. | Creates one immutable location/context record and broadcasts after the durable write. Numeric position fields are normalized. | JWT/current-book required; empty context, oversize data, and a chapter belonging to another book are `400`. |
| `POST /api/books/:id/bookmarks/batch` | Array of the same payload shape. | Validates every row before one transaction; request order becomes creation order and a bad row leaves no valid prefix behind. | JWT/current-book required; empty/malformed batches or any invalid row return `400`. |
| `PUT /api/bookmarks/:id` | `{ "note": string }` | Edits the note only; the original book, chapter, offset, title, and paragraph context remain unchanged. | JWT/current-user required; absent row `404`, oversize note `400`. |
| Backup / restore `bookmarks.json` | Existing JSON shape, including `bookTitle` and `bookUrl`. | Exports ID/creation order; restores modern timestamped rows idempotently without merging independent same-location bookmarks, and remaps a matching destination chapter by index. | Per-user restore scope applies. Legacy rows without timestamps remain readable through the narrow fallback identity. |

## Reader book-content search contract

| Method / path | Request | Success / side effects | Auth and errors |
|---|---|---|---|
| `GET /api/books/:id/search` | `q` (or legacy `keyword`), optional `paged`, `lastIndex`, `chapterLimit`, `matchLimit`, `scanUntilMatch`, and local/remote work bounds. | Lists caller-owned book matches in source chapter order. A cursor always represents the last fully scanned chapter: all matches from that final chapter are returned before a later request can start at its successor. Response preserves `{ list, lastIndex, hasMore, total }` and additionally reports explicit `incomplete`, `unavailableChapters`, and `truncated` states. | JWT/current-book required; blank query `400`, foreign/missing book `404`. Request cancellation stops remote fetch scheduling without writing a false successful result. A returned incomplete page is `200` with a client-safe state, not a host/source error. |
| `GET` / `POST /api/reader3/searchBookContent` | Existing `url`/`bookUrl`, `keyword`, `lastIndex`, `size` aliases. | Keeps legacy `{ isSuccess, data: { list, lastIndex, hasMore, total } }` response and upstream URL lookup behavior. Additive `incomplete`, `unavailableChapters`, and `truncated` data fields expose a safety-bound partial scan without breaking existing clients. | JWT required; legacy validation errors remain `isSuccess: false` messages for deployed Reader3 clients. Like the modern route, request cancellation must flow into chapter loading, stop later remote fetches, and return without fabricating a successful page. |

OpenReader retains bounded remote/local scanning and case-insensitive normalized matching as runtime/security adaptations. A bound may never silently advance `lastIndex` past omitted same-chapter matches: it must set `truncated: true`, and the UI must say that results are incomplete. Unavailable remote content is likewise surfaced by `incomplete/unavailableChapters` rather than as a false “没有匹配内容”. The frontend must pass an `AbortSignal`; closing the search dialog, replacing its keyword/book, or resetting its state aborts the active transport request without treating the intentional abort as a visible search error. Closing and reopening the same book preserves completed keyword/results/scroll state, matching the upstream root-dialog lifecycle.

## P1-B remote temporary-reader contract (implemented slice)

Reader-dev permits a search/explore result to enter Reader before it has been
added to the shelf. Its Vuex `readingBook` is an in-memory reading context, not
a saved shelf row. OpenReader adds a server-owned expiring session so Vue 3 can
preserve that behavior without treating `POST /api/books/remote` as a read
operation. This slice is implemented and covered by an API contract test plus
the desktop and two mobile browser flows on 2026-07-13.

| Method / path | Request | Success / side effects | Auth and errors |
|---|---|---|---|
| `POST /api/reader/remote-sessions` | `{ sourceId, bookUrl, title, author?, coverUrl?, intro?, kind?, wordCount?, variable? }`. `sourceName` is display-only and ignored for authorization. | Validates the caller-visible source and normalized bounded variable map; resolves BookInfo + TOC once with the current source snapshot; returns `201 { id, expiresAt, book, chapters }`. `book.id` is always `0`; its opaque `variable` remains available only for a later explicit add-to-shelf. The server stores only a user-bound, opaque, expiring runtime session; it creates **no** Book, Chapter, Progress, Bookmark, cache file, backup record, or websocket bookshelf event. | JWT required. Missing `sourceId`/`bookUrl`/`title` or invalid variables: `400`; unavailable source: `404`; parser/request failure: safe `502 { error, code?, stage: "book_info" }`; no raw rule/header/cookie/URL-query detail is exposed. Source-request failures may enter the caller's existing short-lived source-failure cache. |
| `GET /api/reader/remote-sessions/:id` | Opaque session id. | Returns the original normalized `{ id, expiresAt, book, chapters }` without reparsing or persisting it. `Cache-Control: no-store`. | JWT required. Unknown or another user's id: `404`; expired id: `410 { error: "remote reader session expired" }`. |
| `GET /api/reader/remote-sessions/:id/chapters/:index/content` | Opaque session id and non-negative chapter ordinal. | Uses only the server-stored source snapshot, book variables and chapter variables to fetch/parse that TOC row. Returns the normal Reader `{ chapter, content, format }` shape (and the existing safe remote-audio fields when applicable). It must never accept a client-supplied chapter URL or source rule. Refreshes the bounded idle expiry but never writes a shelf cache/chapter/progress row. | JWT/session binding required; malformed index `400`; unknown/foreign session `404`; expired `410`; missing chapter `404`; parser/request failure `502 { error, code?, stage: "content" }`. Cancellation stops further source work and returns no synthetic success. |

### Runtime and frontend boundary

- Session IDs are high-entropy opaque values, held server-side only; they are never JWTs, never appear in a source URL, never enter backup/WebDAV/export data, and use `Cache-Control: no-store`. The implemented idle TTL is 30 minutes and the absolute lifetime is four hours. Expiration returns `410`, never a silent account/session logout.
- The source snapshot and fetchable variable state stay server-side. The frontend receives only presentation metadata, an opaque session id, and the existing opaque variable needed for a later explicit add-to-shelf; it never turns that field into a request URL. Temporary sessions deliberately do **not** save browser-local or server progress, and must not call `/progress/:bookId`, bookmark, cache, category, source-change, refresh, or any other shelf-ID endpoint with a fabricated ID.
- Search and Explore must call this same session creation endpoint and use the same reader route form, e.g. `/reader/remote/:sessionId`; neither flow may call `POST /books/remote` until the user explicitly chooses the canonical BookInfo “加入书架” action. Adding later remains the existing category-confirmed transaction and can forward the returned opaque `variable` field.
- Reader controls that require a durable shelf record (bookmark creation, group editing, cache/clear cache, durable progress, source change/refresh) must be either temporarily unavailable with an explicit “加入书架后可用” state or receive a separately documented temporary-session contract. They must never fail as a hidden `404` caused by a synthetic book ID.

Implemented tests: `backend/api/remote_reader_contract_test.go` proves user isolation, content loading and zero Book/Chapter/Progress/Bookmark writes. `scripts/smoke/remote-reader-contract.mjs` proves Search cover → BookInfo, Search body → temporary Reader, and zero persistent writer requests at 1440×900, 390×844 and 360×800. Remaining API test coverage is listed in P1-B follow-up: request validation before fetch; expiry; safe parser error redaction; cancellation; source-failure cache; and variable propagation across multiple chapters.

## Compatibility rule

If a refactor changes frontend routes, API paths should stay stable unless an old path is kept as a redirect/shim. Document removals before deleting compatibility behavior.

## P2 parser structured-error contract (P2-Parser-2A implemented)

Status: implemented and API-tested on 2026-07-13. Reader-dev has no equivalent REST envelope, so OpenReader preserves deployed transport semantics while making parser failures machine-readable and safe. This remains independent from the separately implemented P2-Parser-1G persistent-variable migration.

| Path family | Existing stable behavior | Additive contract |
|---|---|---|
| `GET /api/books/:id/chapters/:index/content` | Remote failure remains `502 {"error":"failed to load chapter content"}`. | Implemented optional `code` (`source_rule_invalid`, `source_rule_unsupported`, `source_request_failed`, `content_unavailable`) and `stage: "content"`. |
| `POST /api/search` (single paged source), `GET /api/explore/:sourceId`, `POST /api/books/remote`, source change/refresh | Existing status and top-level `error` remain stable. | Implemented stable `error` text plus optional `code`/`stage` (`search`, `explore`, `book_info`); raw Go/source-request detail is never serialized. |
| `/api/sources/:id/test*` and batch test | Existing authenticated `200` shape includes its result payload plus `error`/`message`. | Implemented optional `code`/`stage` (`search`, `toc`, `content`) without changing `200` or result fields. Debug messages never include variable values, rule source, request URL query, response body, cookie, authorization header, JWT, WebDAV secret or filesystem path. |

`code` and `stage` are optional additive fields. Legacy frontend paths continue to use `error`; no parser error becomes an authentication failure, and only `engine.IsSourceRequestError` may enter `source_failures`. `backend/api/source_error_contract_test.go` proves remote request failures are redacted for paged search, explore, source debug and remote-book creation, while an invalid content rule keeps its existing `502` text and receives `source_rule_invalid` / `content`.

## P2 parser persistent-variable contract (P2-Parser-1G implemented)

| Path / payload | Additive behavior | Compatibility and safety |
| --- | --- | --- |
| Search / explore result | Result objects may include opaque `variable` JSON. | Existing consumers may ignore it. The frontend forwards it only through the existing add-remote-book payload; it is never rendered as HTML or interpreted by JavaScript. |
| `POST /api/books/remote` | Optional `variable` accepts the bounded JSON string map and seeds BookInfo/TOC parsing. Returned book keeps normal shape with optional `variable`. | Omitted values remain empty. Malformed/non-string/oversized maps return existing-style `400` with safe `error`/`code`/`stage`, before a remote request. |
| Remote refresh/change-source and chapter content | The server reads/writes optional Book/Chapter variables around existing parser calls. Chapter content stores the returned Book/Chapter map atomically with its cache path. | Existing paths and successful response bodies do not change. A source semantics change clears obsolete state rather than translating or exposing it. |
| Backup restore | `bookshelf.json.variable` and optional `chapterVariables.json` are accepted. | Old archives need neither field. New maps are fully validated before restore mutation and target only the authenticated destination user's source-name-resolved book/chapters; source/book/chapter database IDs are never variable identity. |
