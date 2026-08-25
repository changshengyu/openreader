<p align="center"><a href="README_CN.md">中文</a></p>

# OpenReader

A self-hosted ebook reader with multi-device sync, online book sources, local book import, WebDAV, RSS, and a reader experience aligned with reader-dev.

![Go 1.24+](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go)
![Vue 3.5](https://img.shields.io/badge/Vue-3.5-4FC08D?logo=vue.js)
![SQLite WAL](https://img.shields.io/badge/SQLite-WAL-brightgreen)
![Status: WIP](https://img.shields.io/badge/status-WIP-F59E0B)
![Docker ready](https://img.shields.io/badge/Docker-ready-2496ED?logo=docker)
[![Build and publish Docker image](https://github.com/changshengyu/openreader/actions/workflows/docker-publish.yml/badge.svg)](https://github.com/changshengyu/openreader/actions/workflows/docker-publish.yml)

> [!IMPORTANT]
> **OpenReader is still under active development.** Core features are available, but you should back up your data before upgrades and report problems through [Issues](https://github.com/changshengyu/openreader/issues).
>
> This project recreates the experience of [reader-dev](https://github.com/changshengyu/reader-dev). It can import reader-dev backups, but it cannot use or replace a reader-dev database directly.

## Development Status (WIP)

As of 2026-08-25, development is estimated at about **99%** based on the feature checklist and automated tests. This means the main features are implemented and tested; it does not mean the software is defect-free or verified on every device.

- **Available now:** reader, bookshelf, search and source switching, local-book imports, WebDAV, backup and restore, RSS, multi-user support, and reading-progress sync.
- **Still being improved:** less common API actions, small differences from reader-dev, and hands-on testing across more phones, tablets, and desktop browsers.
- **Known limitation:** the server does not run JavaScript, WebJS, or WebView book-source rules. Sources that depend on those rules will not work; see the [book-source rule guide](docs/compat/online-booksource-parser.md) for supported rule types.

See the [compatibility progress table](docs/compat/refactor-audit-matrix.md) for detailed development records.

## Features

- **Local books** — Import TXT, EPUB, UMD, and CBZ; preview and customize TXT chapter rules. Existing Markdown/PDF archives from earlier OpenReader versions remain readable.
- **Online sources** — Import and manage reader-dev/Legado-compatible sources, search across sources, browse catalogs, switch sources, and cache chapters.
- **Reader** — Vertical or horizontal paging, continuous scrolling, desktop/phone/tablet layouts, bookmarks, full-book search, progress sync, themes, typography, auto-read, and text-to-speech.
- **Bookshelf workspace** — Groups, search, batch operations, title and author editing, local storage, WebDAV file management, and cross-device refresh.
- **Content cleanup** — Replacement rules for removing advertisements, watermarks, and formatting noise.
- **RSS and discovery** — Import RSS sources, browse articles, and explore source catalogs.
- **Backup and restore** — Restore reader-dev/Legado backups or create an OpenReader portable backup containing account data, local books, and supported appearance assets.
- **Multi-user** — Isolated user data, source/library/WebDAV permissions, and administrator tools.
- **Simple deployment** — The frontend, backend, and database run in one Docker container.

## Quick Start

Install Docker first. No source checkout or local image build is required; choose either method below. The Compose method also requires the `docker compose` command.

### Option 1: Docker Compose (Recommended)

Create an empty deployment directory and download only the [Compose file](https://raw.githubusercontent.com/changshengyu/openreader/main/docker-compose.yml):

```bash
curl -fsSLO https://raw.githubusercontent.com/changshengyu/openreader/main/docker-compose.yml
docker compose up -d
curl -fsS http://localhost:8080/api/health
```

This method creates `data/`, `cache/`, and `library/` beside `docker-compose.yml`, making backups and whole-instance migration straightforward.

### Option 2: One `docker run` Command

This command needs no project files and stores persistent data in three Docker-managed volumes:

```bash
docker run -d --name openreader --restart unless-stopped -p 8080:8080 -v openreader-data:/app/data -v openreader-cache:/app/cache -v openreader-library:/app/library ghcr.io/changshengyu/openreader:latest
```

These Docker volumes survive container recreation. Do not delete `openreader-data`, `openreader-cache`, or `openreader-library` when upgrading the container.

The same image supports common 64-bit x86 and ARM hosts, and Docker selects the correct version automatically. Users do not need the source code, Go, Node.js, or a local build environment.

Open `http://localhost:8080`. The first account registered in an empty installation becomes the administrator; later accounts are regular users.

For the Compose method, the defaults run without extra environment configuration. Edit only the values that differ on your host:

| Compose setting | Default | When to change it |
|---|---|---|
| `image` | `ghcr.io/changshengyu/openreader:latest` | Replace `latest` with a specific version tag when you need to pin a release. |
| `ports` | `8080:8080` | Change the left-hand `8080` when the host port is already occupied. Keep the container port unchanged. |
| `./data` | SQLite, uploads, and backups | Change the left-hand path when persistent data belongs on another disk or directory. |
| `./cache` | Regenerable content/import cache | Change the left-hand path when cache belongs on another disk or directory. |
| `./library` | Imported originals and local library files | Change the left-hand path when the book library belongs on another disk or directory. |

Keep the container-side `/app/data`, `/app/cache`, and `/app/library` paths unchanged. Do not remove or replace the three host directories when recreating the container.

### Upgrade an Existing OpenReader Deployment

Stop the container and back up all three data directories before upgrading so files are not copied while the database is being written:

```bash
docker compose stop openreader
tar -czf "../openreader-volume-backup-$(date +%Y%m%d-%H%M%S).tar.gz" data cache library docker-compose.yml
docker compose pull openreader
docker compose up -d --force-recreate openreader
curl -fsS http://localhost:8080/api/health
```

The archive contains user data, so store it securely. After upgrading, check the `version` and `commit` returned by `/api/health` to confirm that the new container is running. Refreshing the browser does not upgrade a container.

To control when upgrades happen, replace `latest` with a specific version tag. To roll back, stop the container, restore all three pre-upgrade data directories, and start the previous image. Do not mix old and new database files.

For a one-command `docker run` installation, first download a portable backup from OpenReader, then run:

```bash
docker pull ghcr.io/changshengyu/openreader:latest
docker stop openreader
docker rm openreader
```

Run the one-line start command above again. The three Docker volumes remain available.

## Migration Guide

Choose the migration path by what you are moving:

| Source | Recommended method | What it preserves |
|---|---|---|
| reader-dev or Legado | Upload and restore the original `backup*.zip` | Sources, bookshelf entries, groups, RSS, bookmarks, replace rules, and reading progress contained in the backup |
| Another OpenReader account/host | Create and restore an **OpenReader portable backup** | Account data, local-book originals, and supported custom covers, backgrounds, and fonts |
| A complete OpenReader installation | Stop the container, then copy `data/`, `cache/`, `library/`, and the deployment configuration together | All users, database data, uploads, backups, original local books, and cached content |
| An older OpenReader version on the same host | Keep the three data directories and recreate the container with the new image | Existing accounts, settings, and library data |

### Migrate from reader-dev

1. Create and download a final `backup*.zip` in reader-dev, and separately save every original local-book file.
2. Start a new OpenReader installation and create the destination account. Do not copy the reader-dev database into OpenReader; the formats are different.
3. Sign in to the destination account, open **WebDAV → 文件管理 (File Manager) → 上传文件 (Upload Files)**, and upload the original ZIP without unpacking or renaming it.
4. Click **恢复 (Restore)** beside the uploaded file, then review the result shown on the page. Sources are skipped when the current user does not have permission to edit them.
5. Normal reader-dev/Legado backups do not include original TXT, EPUB, UMD, or CBZ files. Import those books separately.
6. Check the bookshelf, sources, bookmarks, and reading progress, then open several remote and local books. Keep the old installation and backup until everything is verified.

Restored content belongs to the currently signed-in OpenReader account; old account passwords are not transferred. Back up a destination account before restoring into it when it already contains data.

### Move OpenReader to Another Host

For an exact whole-instance move:

1. Stop the source container.
2. Copy `data/`, `cache/`, and `library/` together while the service is stopped. Copy the Compose file and any custom environment overrides separately.
3. Start the destination with the same or a newer OpenReader image and the same mount paths.
4. Check `/api/health`, sign in, open local and remote books, and only then retire the source host.

To move one account, choose **WebDAV → 保存完整可移植备份 (Save Portable Backup)**, download the resulting `portable_backup_*.zip`, then upload and restore it in the destination account. Local audio directories are not included in this backup.

### Backup Types

- `backup_*.zip`: account data only; it does not contain original local-book files.
- `portable_backup_*.zip`: account data plus local-book originals and supported appearance assets; use it to move one account.
- `data/`, `cache/`, and `library/`: stop the container and copy all three directories to back up the complete installation and every user.

Never copy a reader-dev SQLite database into OpenReader, and do not copy only part of the three OpenReader data directories.

## Persistent Data

| Directory | Purpose | Backup notes |
|---|---|---|
| `data/` | SQLite database, uploaded assets, and per-user WebDAV/backup files | Required for every full backup |
| `cache/` | Chapter/content and import-preview cache | Regenerable in many cases, but copy it for an exact move |
| `library/` | Imported original files and local-library content | Required to preserve local books |

Each user's WebDAV files are isolated. For compatibility with older clients, both `/webdav/` and `/reader3/webdav/` are supported.

## Configuration

Normal Docker deployments do not need environment variables. Open the advanced section only when developing from source, changing storage paths, connecting to a trusted LAN source, or adjusting file-size limits.

<details>
<summary>Advanced environment variables</summary>

| Variable | Default | Description |
|---|---|---|
| `OPENREADER_ADDR` | `:8080` | Server listen address |
| `OPENREADER_DATA_DIR` | `data` | Data directory |
| `OPENREADER_CACHE_DIR` | `cache` | Cache directory |
| `OPENREADER_LIBRARY_DIR` | `library` | Library directory |
| `OPENREADER_LOCAL_STORE_DIR` | `library/localStore` | Local-library directory |
| `OPENREADER_DB` | `data/openreader.db` | SQLite database path |
| `OPENREADER_JWT_SECRET` | built-in default | Signs login sessions and protected resource links; the standard Compose deployment does not require a change |
| `OPENREADER_CORS_ORIGIN` | `http://localhost:5173` | Used only when the frontend and API run separately during development; the standard Docker deployment does not require a change |
| `OPENREADER_PUBLIC_DIR` | `public` | Built frontend directory |
| `OPENREADER_CHECK_INTERVAL` | `30m` | Scheduled bookshelf/source check interval |
| `OPENREADER_RATE_LIMIT_PER_MINUTE` | `6000` | Per-client API request limit |
| `OPENREADER_SOURCE_NETWORK_ALLOWLIST` | empty | Trusted LAN source hosts, IP addresses, or network ranges, separated by commas |

**File and network limits**

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

For security, book-source and RSS requests cannot access the local machine or private network by default. If a trusted source runs on your NAS or LAN, allow only its hostname or IP address:

```yaml
environment:
  OPENREADER_SOURCE_NETWORK_ALLOWLIST: "nas.home,192.168.50.20"
```

OpenReader does not read the container's `HTTP_PROXY`, `HTTPS_PROXY`, or `ALL_PROXY` settings. Use the source's own proxy option or a system network route instead. Fake-IP modes used by tools such as Clash may require allowing the corresponding network range; real-IP mode is simpler and safer when available.

</details>

## Development

### Local Development

```bash
git clone https://github.com/changshengyu/openreader.git
cd openreader
```

Start the backend:

```bash
cd backend
go run .
```

From the repository root, open another terminal and start the frontend:

```bash
cd frontend
npm ci
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

After changing the reader or bookshelf UI, also test it in a real browser at desktop and phone sizes. GitHub Actions repeats backend, frontend, and data-compatibility checks before publishing an image.

<details>
<summary>Maintainer: optional local Docker build and release fallback</summary>

Normal releases are handled by [GitHub Actions](.github/workflows/docker-publish.yml), which publishes `latest` and a version tag. The local script is only for development or recovery when automated publishing is unavailable. On Apple Silicon, it builds `linux/arm64` by default:

```bash
docker login ghcr.io
./scripts/docker-build-push.sh
```

Publish the final `linux/amd64` and `linux/arm64` index with:

```bash
RELEASE=1 ./scripts/docker-build-push.sh
docker buildx imagetools inspect ghcr.io/changshengyu/openreader:latest
```

Use `TAG`, `IMAGE`, `PUSH`, and `PLATFORMS` when you need to change the tag, image location, or target platforms. A manual release must pass the same tests and data-compatibility checks as GitHub Actions.

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
