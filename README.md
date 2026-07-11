<p align="center"><a href="README_CN.md">中文</a></p>

# OpenReader

A self-hosted, lightweight ebook reader with multi-device sync. Read your own books, from anywhere.

Everyone is welcome to use OpenReader and actively submit [Issues](https://github.com/changshengyu/openreader/issues) with bug reports and suggestions.

![](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go)
![](https://img.shields.io/badge/Vue-3.5-4FC08D?logo=vue.js)
![](https://img.shields.io/badge/SQLite-WAL-brightgreen)
![](https://img.shields.io/badge/Docker-ready-2496ED?logo=docker)

## Features

- **Multi-format Import** — TXT, EPUB, Markdown, PDF, UMD files with automatic chapter detection
- **Online Sources** — Add custom book sources (CSS selectors / XPath), browse catalogs, and pull chapters from the web
- **Reading Experience** — Upstream-aligned reading modes: vertical paging, horizontal swiping, and vertical scrolling. Bookmarks, reading progress, and chapter caching
- **Content Cleaning** — Regex-based replace rules to clean up ad text, watermarks, and formatting noise
- **Library Management** — Categories, search, batch operations, and local file storage with WebDAV access
- **RSS Reader** — Subscribe to feeds and read articles within the app
- **Book Discovery** — Explore mode to browse online source catalogs
- **Backup & Restore** — Backup/restore to WebDAV, import Legado-compatible backups
- **Multi-User** — JWT-based authentication, admin dashboard, per-user activity tracking
- **Single Binary** — Go backend serves both API and frontend static files. One container, zero fuss.

## Quick Start

### Docker

```bash
cp .env.example .env
# Edit .env and set a secure OPENREADER_JWT_SECRET
docker compose up -d
```

Open `http://localhost:8080`. Register an account and start reading.

### Publish Docker Image

Development builds default to `linux/arm64`, which is faster for Apple Silicon Macs:

```bash
docker login ghcr.io
./scripts/docker-build-push.sh
```

For final releases, publish a multi-arch manifest for both Intel/AMD servers and Apple Silicon Macs:

```bash
RELEASE=1 ./scripts/docker-build-push.sh
```

Useful overrides:

```bash
TAG=manual-test ./scripts/docker-build-push.sh
IMAGE=ghcr.io/changshengyu/openreader TAG=$(git rev-parse --short HEAD) ./scripts/docker-build-push.sh
PUSH=0 PLATFORMS=linux/arm64 ./scripts/docker-build-push.sh
PLATFORMS=linux/amd64,linux/arm64 ./scripts/docker-build-push.sh
docker buildx imagetools inspect ghcr.io/changshengyu/openreader:latest
```

The script passes `VERSION`, `VCS_REF`, and `BUILD_DATE` into the Go binary and OCI image labels, so `/api/health` and the Settings page show the actual build metadata instead of `unknown`.

### Local Development

**Backend:**

```bash
cd backend
go mod tidy
go run .
```

**Frontend:**

```bash
cd frontend
npm install
npm run dev
```

- Frontend: `http://localhost:5173`
- API: `http://localhost:8080`
- Health check: `http://localhost:8080/api/health`

### Running Tests

```bash
cd backend && go test ./...
cd frontend && npm run build
```

## Persistent Data

| Directory | Purpose |
|-----------|---------|
| `data/` | SQLite database — users, books, bookmarks, progress |
| `cache/` | Per-chapter content cache for fast reading |
| `library/` | Imported original files and local store |

All three are mounted as volumes in Docker. Backup these directories to migrate.

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `OPENREADER_ADDR` | `:8080` | Server listen address |
| `OPENREADER_DATA_DIR` | `data` | Data directory path |
| `OPENREADER_CACHE_DIR` | `cache` | Cache directory path |
| `OPENREADER_LIBRARY_DIR` | `library` | Library directory path |
| `OPENREADER_DB` | `data/openreader.db` | SQLite database path |
| `OPENREADER_JWT_SECRET` | *(required)* | JWT signing secret — use a long random string |
| `OPENREADER_CORS_ORIGIN` | `http://localhost:5173` | CORS allowed origin |
| `OPENREADER_PUBLIC_DIR` | `public` | Frontend static files directory |
| `OPENREADER_MAX_IMPORT_BYTES` | `134217728` (128 MiB) | Maximum bytes accepted for one local-book or LocalStore/WebDAV upload, preview, or import; adjust only when the host has sufficient memory/disk |

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Backend | Go 1.24, Gin, GORM, SQLite (WAL mode) |
| Frontend | Vue 3, Vite, Pinia, Vue Router, Element Plus |
| Realtime | Gorilla WebSocket (sync channel) |
| Parsing | goquery (CSS selectors), custom regex chapter detection |
| Deployment | Docker multi-stage build, single Alpine container |

## Acknowledgments

This project is a refactor and rewrite based on [changshengyu/reader-dev](https://github.com/changshengyu/reader-dev), a maintained fork of the original Reader project. We are grateful to all upstream authors and contributors for their work and inspiration.

## License

[GPL v3](LICENSE)
