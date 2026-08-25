<p align="center"><a href="README_CN.md">中文</a></p>

# OpenReader

A self-hosted ebook reader with multi-device sync, online book sources, local book import, WebDAV, RSS, and a reader experience aligned with reader-dev.

Everyone is welcome to use OpenReader and actively submit [Issues](https://github.com/changshengyu/openreader/issues) with bug reports and suggestions.

![Go 1.24+](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go)
![Vue 3.5](https://img.shields.io/badge/Vue-3.5-4FC08D?logo=vue.js)
![SQLite WAL](https://img.shields.io/badge/SQLite-WAL-brightgreen)
![Status: WIP](https://img.shields.io/badge/status-WIP-F59E0B)
![Docker ready](https://img.shields.io/badge/Docker-ready-2496ED?logo=docker)
[![Build and publish Docker image](https://github.com/changshengyu/openreader/actions/workflows/docker-publish.yml/badge.svg)](https://github.com/changshengyu/openreader/actions/workflows/docker-publish.yml)

> [!IMPORTANT]
> OpenReader is an independent Go/Vue refactor and rewrite of [changshengyu/reader-dev](https://github.com/changshengyu/reader-dev), not a drop-in replacement for its executable or database. Upstream behavior at commit [`fa22f271`](https://github.com/changshengyu/reader-dev/commit/fa22f271849d45f93349ae1636223e27b16a4691) is the compatibility baseline. The refactor is active; see the [compatibility audit](docs/compat/refactor-audit-matrix.md) for current module status.
>
> JavaScript, WebJS, and WebView book-source execution is intentionally unsupported in the server process. Those fields are preserved during import/export, but sources that require them return an explicit unsupported-rule error. CSS, JSONPath, XPath, regex, composite, pagination, and bounded variable rules are supported as documented in the [online source parser contract](docs/compat/online-booksource-parser.md).

## Development Status (WIP)

OpenReader is still a **work in progress**, not a finished compatibility release. As of 2026-08-25, the fixed-baseline audit estimates about **99% contract and test coverage**. This percentage measures reviewed upstream contracts and implemented regression coverage, not defect-free readiness or completion on every device.

- **P0 Reader mainline:** reader controls, paging and continuous scrolling, phone/tablet/desktop layouts, settings, bookmarks, content search, EPUB, CBZ, audio, TTS, authentication recovery, source switching, and chapter caching are implemented and regression-tested; real-device sign-off is still ongoing.
- **P1 Index mainline:** bookshelf, search/explore, BookInfo, BookManage, BookGroup, imports, LocalStore, WebDAV, source management, refresh, progress, and legacy routes are covered; real-device sign-off is still ongoing.
- **P2 backend/data:** multi-user isolation, source ownership, backup/portable v2, WebDAV, RSS, replace rules, bookmarks, user management, WebSocket, fetch security, parser budgets, and major request boundaries are covered.
- **Remaining work:** long-tail REST action audits, remaining second-pass upstream reviews, device-reported visible differences, and final device verification. JavaScript/WebView source execution remains an explicit security limitation rather than pending silent compatibility.

See the [full compatibility audit](docs/compat/refactor-audit-matrix.md) for evidence, remaining work, and the meaning of each status.

## Features

- **Local books** — Import TXT, EPUB, UMD, and CBZ; preview and customize TXT chapter rules. Existing Markdown/PDF archives from earlier OpenReader versions remain readable.
- **Online sources** — Import and manage reader-dev/Legado-compatible sources, search across sources, browse catalogs, switch sources, and cache chapters.
- **Upstream-aligned reader** — Vertical paging, horizontal swiping, and continuous vertical scrolling; desktop, phone, and tablet layouts; bookmarks, in-book search, progress sync, themes, typography, auto-reading, and TTS.
- **Bookshelf workspace** — Groups, search, batch operations, metadata editing, local storage, WebDAV file management, and cross-client refresh.
- **Content cleanup** — Ordered regular-expression replacement rules for advertisements, watermarks, and formatting noise.
- **RSS and discovery** — Import RSS sources, browse articles, and explore source catalogs.
- **Backup and restore** — reader-dev/Legado-compatible logical ZIP restore, OpenReader logical backups, and portable v2 backups containing recoverable local-book originals and supported custom appearance assets.
- **Multi-user** — JWT authentication, per-user data isolation, source/store/WebDAV permissions, and administrator management.
- **Single container** — One Go binary serves the API and built Vue application; SQLite runs in WAL mode.

## Quick Start

### Docker Compose

```bash
git clone https://github.com/changshengyu/openreader.git
cd openreader
docker compose up -d
curl -fsS http://localhost:8080/api/health
```

Published images are a `linux/amd64` and `linux/arm64` OCI index. `docker compose up` pulls the image and Docker automatically selects the matching platform; users do not need Go, Node.js, QEMU, or a local image build. To fetch it explicitly, run `docker pull ghcr.io/changshengyu/openreader:latest`.

Open `http://localhost:8080`. The first account registered in an empty installation becomes the administrator; later accounts are regular users.

The defaults run without extra environment configuration. Edit only the values that differ on your host:

| Compose setting | Default | When to change it |
|---|---|---|
| `image` | `ghcr.io/changshengyu/openreader:latest` | Replace `latest` with a commit tag when you need a pinned release. |
| `ports` | `8080:8080` | Change the left-hand `8080` when the host port is already occupied. Keep the container port unchanged. |
| `./data` | SQLite, uploads, and backups | Change the left-hand path when persistent data belongs on another disk or directory. |
| `./cache` | Regenerable content/import cache | Change the left-hand path when cache belongs on another disk or directory. |
| `./library` | Imported originals and LocalStore | Change the left-hand path when the book library belongs on another disk or directory. |

Keep the container-side `/app/data`, `/app/cache`, and `/app/library` paths unchanged. Do not remove or replace the three host directories when recreating the container.

### Upgrade an Existing OpenReader Deployment

First make a cold backup. Stopping the container ensures the SQLite database and its WAL files are captured consistently:

```bash
docker compose stop openreader
tar -czf "../openreader-volume-backup-$(date +%Y%m%d-%H%M%S).tar.gz" data cache library docker-compose.yml
docker compose pull openreader
docker compose up -d --force-recreate openreader
curl -fsS http://localhost:8080/api/health
```

The archive contains user data; store it with restricted access. The bundled Compose file uses `pull_policy: always`. The `version` and `commit` returned by `/api/health` identify the code actually running; refreshing the browser does not update a container.

For a controlled rollout, pin `ghcr.io/changshengyu/openreader:<commit>` instead of `latest`. To roll back safely, stop the candidate, restore the complete pre-upgrade snapshot into empty persistent directories, pin the previous image, and start again. Do not merge an old SQLite snapshot into directories already written by a newer container.

## Migration Guide

Choose the migration path by what you are moving:

| Source | Recommended method | What it preserves |
|---|---|---|
| reader-dev or Legado | Restore its logical `backup*.zip` into the target OpenReader account | Supported sources, bookshelf entries, groups, RSS, bookmarks, replace rules, and progress present in the archive |
| Another OpenReader account/host | Create and restore an **OpenReader portable backup** | Logical account data plus recoverable local-book originals and supported custom covers/backgrounds/fonts |
| A complete OpenReader installation | Cold-copy `data/`, `cache/`, and `library/` together, plus deployment configuration | All users, SQLite data, uploads, backups, original local books, and cached content |
| An older OpenReader version on the same host | Keep the same three mounts and recreate the container with the new image | Existing installation data through additive startup migrations |

### Migrate from reader-dev

1. **Create a final backup in reader-dev.** Use its existing backup/WebDAV action to produce a `backup*.zip`. Download that ZIP and also keep an independent copy of the old reader-dev data directory and every original local book.
2. **Keep the old service unchanged.** Do not point OpenReader at the reader-dev database or overwrite `data/openreader.db`; the database schemas and filesystem layouts are different.
3. **Start OpenReader and create the destination account.** On a new installation, register the administrator first. For a multi-user reader-dev deployment, restore one user's archive while signed in as the corresponding OpenReader account; accounts and passwords are not contained in the logical backup.
4. **Upload the archive.** Open the sidebar and choose **WebDAV → 文件管理 (File Manager)**, click **上传文件 (Upload Files)**, and upload the original ZIP without unpacking or renaming its JSON files.
5. **Restore it.** Click **恢复 (Restore)** on the uploaded ZIP and confirm. OpenReader recognizes reader-dev/Legado names such as `bookSource.json`, `bookshelf.json` or `myBookShelf.json`, `bookGroup.json`, `rssSources.json`, `bookmark.json`, `replaceRule.json`, and `bookProgress/*.json`.
6. **Read the restore summary.** A user without source-edit permission still gets personal data restored, but sources are skipped and the UI reports that explicitly. Ask an administrator to grant source-edit permission before retrying if those sources are required.
7. **Re-import local originals.** A normal reader-dev/Legado backup does **not** contain original TXT/EPUB/UMD/CBZ files. Upload and import those originals separately. OpenReader portable backups can carry supported local originals, but only after the books already live in OpenReader.
8. **Verify before switching traffic.** Compare bookshelf and source counts, open several remote and local books, check bookmarks and progress, and test one source refresh. Keep the old deployment and its backup until this verification is complete.

Restore maps data to the currently authenticated OpenReader account and allocates destination IDs; IDs, user records, passwords, JWT sessions, WebDAV credentials, and host paths from the old installation are never reused. Restoration is transactional for supported logical data, but it is still a write operation—back up a non-empty destination first.

### Move OpenReader to Another Host

For an exact whole-instance move:

1. Stop the source container.
2. Copy `data/`, `cache/`, and `library/` together while the service is stopped. Copy the Compose file and any custom environment overrides separately.
3. Start the destination with the same or a newer OpenReader image and the same mount paths.
4. Check `/api/health`, sign in, open local and remote books, and only then retire the source host.

For a single-account move, use **WebDAV → 保存完整可移植备份 (Save Portable Backup)**, download the resulting `portable_backup_*.zip`, upload it to the target account's WebDAV file manager, and click **恢复 (Restore)**. Portable generation intentionally fails instead of producing a partial package when a referenced local original or supported custom asset is missing. Local audio directories are not included.

### Backup Types and Limits

- `backup_*.zip` is a logical account backup. It is compatible with supported reader-dev/Legado JSON artifacts, but it does not contain the SQLite database or local-book originals.
- `portable_backup_*.zip` is an OpenReader portable v2 package. It adds validated local-book originals and supported current-user appearance assets; portable v1 packages remain restorable.
- A cold snapshot of all three persistent directories is the only complete system-level backup for all users.
- Never copy a reader-dev SQLite database over `data/openreader.db`, and never restore only one of OpenReader's three directories as if it were a complete instance migration.

## Persistent Data

| Directory | Purpose | Backup notes |
|---|---|---|
| `data/` | SQLite database, uploaded assets, and per-user WebDAV/backup files | Required for every full backup |
| `cache/` | Chapter/content and import-preview cache | Regenerable in many cases, but copy it for an exact move |
| `library/` | Imported original files and LocalStore content | Required to preserve local books |

The administrator keeps the historical WebDAV root under `data/webdav/`; regular users are isolated under user-specific descendants. The WebDAV protocol is available at both `/webdav/` and the reader-dev-compatible `/reader3/webdav/` path.

## Configuration

The bundled Compose file already supplies the container paths through the image defaults. These service variables are mainly for source development, custom orchestration, or deliberate limit tuning:

| Variable | Default | Description |
|---|---|---|
| `OPENREADER_ADDR` | `:8080` | Server listen address |
| `OPENREADER_DATA_DIR` | `data` | Data directory |
| `OPENREADER_CACHE_DIR` | `cache` | Cache directory |
| `OPENREADER_LIBRARY_DIR` | `library` | Library directory |
| `OPENREADER_LOCAL_STORE_DIR` | `library/localStore` | LocalStore root |
| `OPENREADER_DB` | `data/openreader.db` | SQLite database path |
| `OPENREADER_JWT_SECRET` | compatibility fallback | JWT/capability signing secret; the bundled same-origin Compose deployment does not require an override |
| `OPENREADER_CORS_ORIGIN` | `http://localhost:5173` | CORS response origin for split frontend/API development; irrelevant to normal same-origin container access |
| `OPENREADER_PUBLIC_DIR` | `public` | Built frontend directory |
| `OPENREADER_CHECK_INTERVAL` | `30m` | Scheduled bookshelf/source check interval |
| `OPENREADER_RATE_LIMIT_PER_MINUTE` | `6000` | Per-client API request limit |
| `OPENREADER_SOURCE_NETWORK_ALLOWLIST` | empty | Comma-separated exact hosts, IPs, or CIDRs allowed to reach non-public networks |

<details>
<summary>Parser, network, backup, and asset safety limits</summary>

| Variable | Default | What it controls |
|---|---:|---|
| `OPENREADER_SOURCE_REQUEST_TIMEOUT_SECONDS` | `15` | Timeout for one remote book-source or RSS request |
| `OPENREADER_MAX_SOURCE_RESPONSE_BYTES` | `16777216` (16 MiB) | Maximum remote response body size |
| `OPENREADER_MAX_SOURCE_REDIRECTS` | `5` | Maximum remote redirect count |
| `OPENREADER_MAX_SOURCE_RETRIES` | `3` | Maximum attempts for a retryable remote request |
| `OPENREADER_MAX_IMPORT_BYTES` | `134217728` (128 MiB) | Maximum uploaded local-book/import file size |
| `OPENREADER_MAX_ARCHIVE_ENTRIES` | `20000` | Maximum files in an imported book archive |
| `OPENREADER_MAX_ARCHIVE_ENTRY_BYTES` | `134217728` (128 MiB) | Maximum expanded size of one imported archive entry |
| `OPENREADER_MAX_ARCHIVE_EXPANDED_BYTES` | `536870912` (512 MiB) | Maximum total expanded size of an imported archive |
| `OPENREADER_MAX_PDF_PAGES` | `10000` | Maximum pages parsed from one PDF |
| `OPENREADER_MAX_PARSED_TEXT_BYTES` | `268435456` (256 MiB) | Maximum decoded text retained during local-book parsing |
| `OPENREADER_MAX_PARSED_CHAPTERS` | `100000` | Maximum chapters produced by a local parser |
| `OPENREADER_MAX_UMD_CHAPTERS` | `100000` | UMD-specific chapter limit and compatibility fallback |
| `OPENREADER_MAX_BACKUP_RESTORE_BYTES` | `134217728` (128 MiB) | Maximum uploaded logical-backup ZIP size |
| `OPENREADER_MAX_BACKUP_ARCHIVE_ENTRIES` | `5000` | Maximum entries accepted in a logical backup |
| `OPENREADER_MAX_BACKUP_ARCHIVE_ENTRY_BYTES` | `16777216` (16 MiB) | Maximum expanded size of one logical-backup entry |
| `OPENREADER_MAX_BACKUP_ARCHIVE_EXPANDED_BYTES` | `134217728` (128 MiB) | Maximum total expanded logical-backup size |
| `OPENREADER_MAX_PORTABLE_BACKUP_BYTES` | `536870912` (512 MiB) | Maximum portable backup package size |
| `OPENREADER_MAX_PORTABLE_ARCHIVE_ENTRIES` | `10000` | Maximum entries accepted in a portable backup |
| `OPENREADER_MAX_PORTABLE_ARCHIVE_ENTRY_BYTES` | `268435456` (256 MiB) | Maximum expanded size of one portable-backup entry |
| `OPENREADER_MAX_PORTABLE_ARCHIVE_EXPANDED_BYTES` | `536870912` (512 MiB) | Maximum total expanded portable-backup size |
| `OPENREADER_MAX_CHAPTER_IMAGES` | `64` | Maximum remote images cached for one chapter |
| `OPENREADER_MAX_CHAPTER_IMAGE_BYTES` | `8388608` (8 MiB) | Maximum size of one chapter image |
| `OPENREADER_MAX_CHAPTER_IMAGE_TOTAL_BYTES` | `33554432` (32 MiB) | Maximum total image bytes cached for one chapter |
| `OPENREADER_CHAPTER_IMAGE_TIMEOUT_SECONDS` | `12` | Timeout for one chapter-image fetch |
| `OPENREADER_MAX_CHAPTER_IMAGE_REDIRECTS` | `3` | Maximum redirects for one chapter image |
| `OPENREADER_MAX_COVER_IMAGE_BYTES` | `8388608` (8 MiB) | Maximum downloaded cover-image size |
| `OPENREADER_MAX_COVER_CACHE_BYTES` | `268435456` (256 MiB) | Maximum total cover-cache size before eviction |
| `OPENREADER_COVER_IMAGE_TIMEOUT_SECONDS` | `3` | Timeout for one cover-image fetch |
| `OPENREADER_MAX_COVER_IMAGE_REDIRECTS` | `3` | Maximum redirects for one cover image |

</details>

Book-source and RSS requests reject loopback, private, link-local, cloud metadata, benchmark, documentation, and other special-use networks by default. For a trusted LAN source, allow only the exact host or address when possible:

```yaml
environment:
  OPENREADER_SOURCE_NETWORK_ALLOWLIST: "nas.home,192.168.50.20"
```

The shared fetcher intentionally ignores ambient `HTTP_PROXY`, `HTTPS_PROXY`, and `ALL_PROXY`. Use the source's explicit proxy setting or a TUN/system route. Fake-IP DNS ranges such as `198.18.0.0/15` require an explicit exception and therefore authorize the entire range; prefer real-IP/Redir-Host DNS when practical.

## Development

### Local Development

```bash
cd backend
go mod tidy
go run .
```

```bash
cd frontend
npm install
npm run dev
```

- Frontend: `http://localhost:5173`
- API: `http://localhost:8080`
- Health check: `http://localhost:8080/api/health`

### Validation

```bash
cd backend && go test ./...
cd frontend && npm test
cd frontend && npm run build
```

Reader and workspace changes additionally require real-browser smoke checks. On eligible `main` updates, the repository workflow repeats these backend/frontend gates, builds a native candidate, validates fresh and historical mounted volumes plus portable backups, and only then publishes the multi-platform image.

<details>
<summary>Maintainer: optional local Docker build and release fallback</summary>

Normal releases are handled by [GitHub Actions](.github/workflows/docker-publish.yml), which publishes `latest` and the seven-character commit tag. The local script remains available for development or release recovery. On Apple Silicon, its development default is `linux/arm64`:

```bash
docker login ghcr.io
./scripts/docker-build-push.sh
```

Publish the final `linux/amd64` and `linux/arm64` index with:

```bash
RELEASE=1 ./scripts/docker-build-push.sh
docker buildx imagetools inspect ghcr.io/changshengyu/openreader:latest
```

Useful overrides include `TAG`, `IMAGE`, `PUSH=0`, `PLATFORMS`, `BUILD_PROGRESS=plain`, and `HOST_OCI_PUSH`. Both release paths embed `VERSION`, `VCS_REF`, and `BUILD_DATE`; a manually published fallback must pass the same validation and volume/backup compatibility gates as the workflow.

</details>

## Tech Stack

| Layer | Technology |
|---|---|
| Backend | Go 1.24, Gin, GORM, SQLite WAL |
| Frontend | Vue 3.5, Vite, Pinia, Vue Router, Element Plus |
| Realtime | Gorilla WebSocket |
| Parsing | goquery, reader-dev/Legado compatibility adapters, local format parsers |
| Deployment | Multi-stage Docker build, single Alpine runtime container |

## Acknowledgments

OpenReader is based on the behavior and work of [changshengyu/reader-dev](https://github.com/changshengyu/reader-dev), a maintained fork of the original Reader project. We are grateful to all upstream authors and contributors.

## License

[GPL v3](LICENSE)
