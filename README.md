<p align="center">
  <a href="#english">English</a> | <a href="#chinese">中文</a>
</p>

---

<a id="english"></a>

# OpenReader

A self-hosted, lightweight ebook reader with multi-device sync. Read your own books, from anywhere.

![](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go)
![](https://img.shields.io/badge/Vue-3.5-4FC08D?logo=vue.js)
![](https://img.shields.io/badge/SQLite-WAL-brightgreen)
![](https://img.shields.io/badge/Docker-ready-2496ED?logo=docker)

## Features

- **Multi-format Import** — TXT, EPUB, Markdown, PDF, UMD files with automatic chapter detection
- **Online Sources** — Add custom book sources (CSS selectors / XPath), browse catalogs, and pull chapters from the web
- **Reading Experience** — Three reading modes: scroll, paginated, and page-turn. Bookmarks, reading progress, and chapter caching
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
docker compose up --build
```

Open `http://localhost:8080`. Register an account and start reading.

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

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Backend | Go 1.24, Gin, GORM, SQLite (WAL mode) |
| Frontend | Vue 3, Vite, Pinia, Vue Router, Element Plus |
| Realtime | Gorilla WebSocket (sync channel) |
| Parsing | goquery (CSS selectors), custom regex chapter detection |
| Deployment | Docker multi-stage build, single Alpine container |

## Acknowledgments

This project is a refactor and rewrite of [hectorqin/reader](https://github.com/hectorqin/reader), an excellent open-source reading platform. We are grateful for the original author's work and inspiration.

## License

[GPL v3](LICENSE)

---

<a id="chinese"></a>

# OpenReader

轻量级、自部署、多端同步的小说阅读器。读自己的书，随时随地。

![](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go)
![](https://img.shields.io/badge/Vue-3.5-4FC08D?logo=vue.js)
![](https://img.shields.io/badge/SQLite-WAL-brightgreen)
![](https://img.shields.io/badge/Docker-ready-2496ED?logo=docker)

## 功能特性

- **多格式导入** — 支持 TXT、EPUB、Markdown、PDF、UMD，自动识别中文章节标题
- **在线书源** — 自定义书源（CSS 选择器 / XPath），浏览目录，在线拉取章节内容
- **阅读体验** — 三种阅读模式：滚动、翻页、分页。支持书签、阅读进度和章节缓存
- **内容清洗** — 基于正则的替换规则，去除广告、水印和排版噪音
- **书架管理** — 分类、搜索、批量操作，本地文件存储并支持 WebDAV 访问
- **RSS 阅读器** — 订阅 RSS 源，在应用内阅读文章
- **书籍发现** — 探索模式浏览在线书源目录
- **备份恢复** — 备份/恢复至 WebDAV，支持导入 Legado 兼容备份
- **多用户** — JWT 身份认证，管理后台，用户活动追踪
- **单文件部署** — Go 后端同时托管 API 和前端静态文件。一个容器即可运行。

## 快速开始

### Docker 部署

```bash
cp .env.example .env
# 编辑 .env，将 OPENREADER_JWT_SECRET 改为安全随机字符串
docker compose up --build
```

打开 `http://localhost:8080`，注册账号即可开始阅读。

### 本地开发

**后端：**

```bash
cd backend
go mod tidy
go run .
```

**前端：**

```bash
cd frontend
npm install
npm run dev
```

- 前端：`http://localhost:5173`
- API：`http://localhost:8080`
- 健康检查：`http://localhost:8080/api/health`

### 运行测试

```bash
cd backend && go test ./...
cd frontend && npm run build
```

## 持久化目录

| 目录 | 用途 |
|------|------|
| `data/` | SQLite 数据库 — 用户、书籍、书签、阅读进度 |
| `cache/` | 章节正文缓存，加速阅读页加载 |
| `library/` | 导入的原始文件及本地书库 |

三个目录均在 Docker 中以卷形式挂载。迁移时备份这三个目录即可。

## 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `OPENREADER_ADDR` | `:8080` | 服务监听地址 |
| `OPENREADER_DATA_DIR` | `data` | 数据目录路径 |
| `OPENREADER_CACHE_DIR` | `cache` | 缓存目录路径 |
| `OPENREADER_LIBRARY_DIR` | `library` | 书库目录路径 |
| `OPENREADER_DB` | `data/openreader.db` | SQLite 数据库路径 |
| `OPENREADER_JWT_SECRET` | *(必填)* | JWT 签名密钥 — 请使用长随机字符串 |
| `OPENREADER_CORS_ORIGIN` | `http://localhost:5173` | CORS 允许来源 |
| `OPENREADER_PUBLIC_DIR` | `public` | 前端静态文件目录 |

## 技术栈

| 层级 | 技术 |
|------|------|
| 后端 | Go 1.24、Gin、GORM、SQLite (WAL 模式) |
| 前端 | Vue 3、Vite、Pinia、Vue Router、Element Plus |
| 实时通信 | Gorilla WebSocket (同步通道) |
| 内容解析 | goquery (CSS 选择器)、正则分章 |
| 部署 | Docker 多阶段构建、Alpine 单容器 |

## 致谢

本项目是对 [hectorqin/reader](https://github.com/hectorqin/reader) 的重构与重写。感谢原作者的优秀工作和灵感。

## 许可证

[GPL v3](LICENSE)
