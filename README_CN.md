<p align="center"><a href="README.md">English</a></p>

# OpenReader

轻量级、自部署、多端同步的小说阅读器，支持在线书源、本地书导入、WebDAV、RSS，并持续对齐 reader-dev 的阅读体验。

![Go 1.24+](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go)
![Vue 3.5](https://img.shields.io/badge/Vue-3.5-4FC08D?logo=vue.js)
![SQLite WAL](https://img.shields.io/badge/SQLite-WAL-brightgreen)
![项目状态：WIP](https://img.shields.io/badge/status-WIP-F59E0B)
![Docker ready](https://img.shields.io/badge/Docker-ready-2496ED?logo=docker)
[![自动构建并发布 Docker 镜像](https://github.com/changshengyu/openreader/actions/workflows/docker-publish.yml/badge.svg)](https://github.com/changshengyu/openreader/actions/workflows/docker-publish.yml)

> [!IMPORTANT]
> **OpenReader 仍在开发中。** 核心功能已经可用，但升级前仍建议备份数据，并欢迎通过 [Issues](https://github.com/changshengyu/openreader/issues) 反馈问题。
>
> 本项目参考 [reader-dev](https://github.com/changshengyu/reader-dev) 的使用体验重新实现，可以导入其备份，但不能直接使用或覆盖 reader-dev 的数据库。

## 开发状态（WIP）

截至 2026-08-25，按功能清单和自动化测试估算，开发进度约为 **99%**。这个数字表示主要功能已经完成并有测试覆盖，不代表软件没有缺陷，也不代表所有设备都已验证。

- **已经可用：** 阅读器、书架、搜索与换源、本地书导入、WebDAV、备份恢复、RSS、多用户和跨设备进度同步。
- **仍在完善：** 少量不常用接口、与 reader-dev 的细节差异，以及更多手机、平板和桌面设备上的实际体验。
- **已知限制：** 服务器不会执行 JavaScript、WebJS 或 WebView 书源规则。依赖这些规则的书源无法使用；其他常见规则的支持范围见[书源规则说明](docs/compat/online-booksource-parser.md)。

详细开发记录见[兼容性进度表](docs/compat/refactor-audit-matrix.md)。

## 功能特性

- **本地书籍** — 导入 TXT、EPUB、UMD、CBZ，支持 TXT 目录预览与自定义目录规则；旧版 OpenReader 已有的 Markdown/PDF 存档仍可读取。
- **在线书源** — 导入和管理 reader-dev/Legado 兼容书源，多书源搜索、浏览目录、切换书源并缓存章节。
- **阅读器** — 支持上下翻页、左右翻页和连续滚动，适配桌面、手机和平板，并提供书签、正文搜索、进度同步、主题、排版、自动阅读和语音朗读。
- **书架工作台** — 分组、搜索、批量操作、编辑书名和作者、本地书仓、WebDAV 文件管理和跨设备刷新。
- **内容清洗** — 使用替换规则去除广告、水印和排版噪音。
- **RSS 与探索** — 导入 RSS 源、浏览文章和探索书源目录。
- **备份恢复** — 可以恢复 reader-dev/Legado 备份，也可以创建包含账号数据、本地书和自定义外观资源的 OpenReader 可移植备份。
- **多用户** — 用户数据相互隔离，并提供书源、书仓、WebDAV 权限和管理员功能。
- **简单部署** — 前端、后端和数据库都在一个 Docker 容器中运行。

## 快速开始

先安装 Docker。部署不需要克隆源码，也不需要在本地构建镜像；任选下面一种方式即可。Compose 方式还需要系统支持 `docker compose` 命令。

### 方式一：Docker Compose（推荐）

新建一个空的部署目录，只下载 [`docker-compose.yml`](https://raw.githubusercontent.com/changshengyu/openreader/main/docker-compose.yml)：

```bash
curl -fsSLO https://raw.githubusercontent.com/changshengyu/openreader/main/docker-compose.yml
docker compose up -d
curl -fsS http://localhost:8080/api/health
```

这种方式会在 `docker-compose.yml` 旁创建 `data/`、`cache/`、`library/`，便于备份和整实例迁移。

### 方式二：单行 `docker run`

下面的命令不需要任何项目文件，数据保存在三个由 Docker 管理的数据卷中：

```bash
docker run -d --name openreader --restart unless-stopped -p 8080:8080 -v openreader-data:/app/data -v openreader-cache:/app/cache -v openreader-library:/app/library ghcr.io/changshengyu/openreader:latest
```

重新创建容器不会删除这些数据卷。升级时不要删除 `openreader-data`、`openreader-cache`、`openreader-library`。

同一个镜像同时支持常见的 64 位 x86 和 ARM 主机，Docker 会自动选择正确版本。普通用户不需要源码、Go、Node.js 或本地构建环境。

打开 `http://localhost:8080`。空数据库中注册的第一个账号会成为管理员，后续注册账号为普通用户。

使用 Compose 时，默认配置无需额外设置环境变量即可启动。只修改与你的主机不一致的项目：

| Compose 配置 | 默认值 | 何时需要修改 |
|---|---|---|
| `image` | `ghcr.io/changshengyu/openreader:latest` | 需要固定版本时，将 `latest` 替换为指定版本标签。 |
| `ports` | `8080:8080` | 宿主机 8080 已被占用时，只修改左侧端口；容器端口保持不变。 |
| `./data` | SQLite、上传资源和备份 | 持久数据需要放到其他磁盘或目录时，修改左侧路径。 |
| `./cache` | 可重建的正文/导入缓存 | 缓存需要放到其他磁盘或目录时，修改左侧路径。 |
| `./library` | 本地书原文件和本地书仓 | 书库需要放到其他磁盘或目录时，修改左侧路径。 |

容器内 `/app/data`、`/app/cache`、`/app/library` 三个路径不要修改。重新创建容器时，不要删除或替换宿主机上的三个持久化目录。

### 升级已有 OpenReader

升级前先停止容器并备份三个数据目录，避免在数据库正在写入时复制文件：

```bash
docker compose stop openreader
tar -czf "../openreader-volume-backup-$(date +%Y%m%d-%H%M%S).tar.gz" data cache library docker-compose.yml
docker compose pull openreader
docker compose up -d --force-recreate openreader
curl -fsS http://localhost:8080/api/health
```

备份文件包含用户数据，请妥善保管。升级后可查看 `/api/health` 返回的 `version` 和 `commit`，确认容器已经运行新版本；刷新浏览器页面不会升级容器。

希望手动控制升级时间时，可以把镜像标签从 `latest` 改为具体版本。需要回退时，先停止容器，再完整恢复升级前的三个数据目录并使用旧镜像；不要把新旧数据库文件混在一起。

如果使用单行 `docker run` 部署，请先在 OpenReader 中下载完整可移植备份，然后执行：

```bash
docker pull ghcr.io/changshengyu/openreader:latest
docker stop openreader
docker rm openreader
```

最后重新执行上面的单行启动命令。三个 Docker 数据卷会继续保留。

## 迁移指南

请根据迁移对象选择方式：

| 来源 | 推荐方式 | 可保留内容 |
|---|---|---|
| reader-dev 或 Legado | 上传并恢复原项目生成的 `backup*.zip` | 备份中的书源、书架、分组、RSS、书签、替换规则和阅读进度 |
| 另一 OpenReader 账号/主机 | 创建并恢复 **OpenReader 完整可移植备份** | 账号数据、本地书原文件和支持的自定义封面、背景、字体 |
| 整套 OpenReader 实例 | 停止容器后，同时复制 `data/`、`cache/`、`library/` 和部署配置 | 所有用户、数据库、上传资源、备份、本地书原文件和缓存正文 |
| 同一主机上的旧版 OpenReader | 保留三个数据目录，用新镜像重新创建容器 | 原有账号、设置和书库数据 |

### 从原 reader-dev 项目迁移

1. 在 reader-dev 中生成并下载最终的 `backup*.zip`，同时单独保存所有本地书原文件。
2. 启动一个新的 OpenReader 实例并创建目标账号。不要把 reader-dev 的数据库复制到 OpenReader，两者格式不同。
3. 登录目标账号，打开 **WebDAV → 文件管理 → 上传文件**，上传原始 ZIP；不需要解压或修改文件名。
4. 在文件列表中点击 **恢复**，完成后检查页面显示的恢复结果。普通用户没有书源编辑权限时，书源会被跳过。
5. reader-dev/Legado 备份通常不包含 TXT、EPUB、UMD、CBZ 原文件，请单独重新导入这些书。
6. 确认书架、书源、书签和进度正常，并打开几本远程书和本地书测试。确认无误前不要删除旧服务和原始备份。

恢复内容会写入当前登录的 OpenReader 账号，不会迁移旧账号密码。目标账号已有数据时，请先为它创建备份。

### 将 OpenReader 迁移到另一台主机

完整迁移整套实例时：

1. 停止源主机上的 OpenReader 容器。
2. 在服务停止期间同时复制 `data/`、`cache/` 和 `library/`；另行复制 Compose 文件和自行设置的环境变量覆盖项。
3. 目标主机使用相同或更新的 OpenReader 镜像，并保持相同容器内挂载路径。
4. 检查 `/api/health`，登录并分别打开本地书和远程书，验证完成后再停用源主机。

只迁移单个账号时，在源端选择 **WebDAV → 保存完整可移植备份**，下载生成的 `portable_backup_*.zip`，然后在目标账号中上传并恢复。本地音频目录不会包含在这个备份中。

### 备份方式

- `backup_*.zip`：账号数据备份，不包含本地书原文件。
- `portable_backup_*.zip`：账号数据加本地书原文件和支持的自定义外观资源，适合迁移单个账号。
- `data/`、`cache/`、`library/`：停止容器后完整复制这三个目录，才能备份整套实例和所有用户。

不要把 reader-dev 的 SQLite 数据库复制到 OpenReader，也不要只复制三个数据目录中的一部分。

## 持久化数据

| 目录 | 用途 | 备份说明 |
|---|---|---|
| `data/` | SQLite 数据库、上传资源、各用户 WebDAV/备份文件 | 每次完整备份都必须保存 |
| `cache/` | 章节正文和导入预览缓存 | 多数可重建，但精确迁移时应复制 |
| `library/` | 导入的本地书原文件和本地书仓内容 | 保留本地书时必须保存 |

每个用户的 WebDAV 文件相互隔离。为了兼容旧客户端，WebDAV 地址同时支持 `/webdav/` 和 `/reader3/webdav/`。

## 配置

普通 Docker 部署不需要设置环境变量。只有在源码开发、自定义目录、连接局域网书源或调整文件大小限制时，才需要查看下面的高级配置。

<details>
<summary>高级环境变量</summary>

| 变量 | 默认值 | 说明 |
|---|---|---|
| `OPENREADER_ADDR` | `:8080` | 服务监听地址 |
| `OPENREADER_DATA_DIR` | `data` | 数据目录 |
| `OPENREADER_CACHE_DIR` | `cache` | 缓存目录 |
| `OPENREADER_LIBRARY_DIR` | `library` | 书库目录 |
| `OPENREADER_LOCAL_STORE_DIR` | `library/localStore` | 本地书仓目录 |
| `OPENREADER_DB` | `data/openreader.db` | SQLite 数据库路径 |
| `OPENREADER_JWT_SECRET` | 内置默认值 | 用于签名登录状态和受保护资源链接；标准 Compose 部署无需修改 |
| `OPENREADER_CORS_ORIGIN` | `http://localhost:5173` | 仅用于前端和 API 分开运行的开发环境；标准 Docker 部署无需修改 |
| `OPENREADER_PUBLIC_DIR` | `public` | 已构建前端目录 |
| `OPENREADER_CHECK_INTERVAL` | `30m` | 书架/书源定时检查间隔 |
| `OPENREADER_RATE_LIMIT_PER_MINUTE` | `6000` | 单客户端 API 每分钟请求上限 |
| `OPENREADER_TRUSTED_PROXIES` | 空 | 直接访问时保持为空。使用反向代理时，只填写代理服务器自身的 IP 或网段，多个值用英文逗号分隔；不要填写访客网段。 |
| `OPENREADER_SOURCE_NETWORK_ALLOWLIST` | 空 | 允许访问的局域网书源主机、IP 或网段，多个值用英文逗号分隔 |

**文件和网络限制**

| 变量 | 默认值 | 控制内容 |
|---|---:|---|
| `OPENREADER_SOURCE_REQUEST_TIMEOUT_SECONDS` | `15` | 单次远程书源或 RSS 请求超时秒数 |
| `OPENREADER_MAX_SOURCE_RESPONSE_BYTES` | `16777216`（16 MiB） | 单个远程响应正文上限 |
| `OPENREADER_MAX_SOURCE_REDIRECTS` | `5` | 单次远程请求最大重定向次数 |
| `OPENREADER_MAX_SOURCE_RETRIES` | `3` | 可重试远程请求的最大尝试次数 |
| `OPENREADER_MAX_IMPORT_BYTES` | `134217728`（128 MiB） | 上传本地书/导入文件的大小上限 |
| `OPENREADER_MAX_ARCHIVE_ENTRIES` | `20000` | 单个导入书籍归档的文件数量上限 |
| `OPENREADER_MAX_ARCHIVE_ENTRY_BYTES` | `134217728`（128 MiB） | 单个导入归档条目的解压大小上限 |
| `OPENREADER_MAX_ARCHIVE_EXPANDED_BYTES` | `536870912`（512 MiB） | 单个导入归档的总解压大小上限 |
| `OPENREADER_MAX_PDF_PAGES` | `10000` | 单个 PDF 的最大解析页数 |
| `OPENREADER_MAX_PARSED_TEXT_BYTES` | `268435456`（256 MiB） | 本地书解析期间保留的解码文本上限 |
| `OPENREADER_MAX_PARSED_CHAPTERS` | `100000` | 本地书解析器可生成的章节数量上限 |
| `OPENREADER_MAX_UMD_CHAPTERS` | `100000` | UMD 专用章节上限及兼容回退值 |
| `OPENREADER_MAX_BACKUP_RESTORE_BYTES` | `134217728`（128 MiB） | 上传逻辑备份 ZIP 的大小上限 |
| `OPENREADER_MAX_BACKUP_ARCHIVE_ENTRIES` | `5000` | 逻辑备份允许的归档条目数量上限 |
| `OPENREADER_MAX_BACKUP_ARCHIVE_ENTRY_BYTES` | `16777216`（16 MiB） | 单个逻辑备份条目的解压大小上限 |
| `OPENREADER_MAX_BACKUP_ARCHIVE_EXPANDED_BYTES` | `134217728`（128 MiB） | 逻辑备份的总解压大小上限 |
| `OPENREADER_MAX_PORTABLE_BACKUP_BYTES` | `536870912`（512 MiB） | portable 备份包大小上限 |
| `OPENREADER_MAX_PORTABLE_ARCHIVE_ENTRIES` | `10000` | portable 备份允许的归档条目数量上限 |
| `OPENREADER_MAX_PORTABLE_ARCHIVE_ENTRY_BYTES` | `268435456`（256 MiB） | 单个 portable 备份条目的解压大小上限 |
| `OPENREADER_MAX_PORTABLE_ARCHIVE_EXPANDED_BYTES` | `536870912`（512 MiB） | portable 备份的总解压大小上限 |
| `OPENREADER_MAX_CHAPTER_IMAGES` | `64` | 单章可缓存的远程图片数量上限 |
| `OPENREADER_MAX_CHAPTER_IMAGE_BYTES` | `8388608`（8 MiB） | 单张章节图片大小上限 |
| `OPENREADER_MAX_CHAPTER_IMAGE_TOTAL_BYTES` | `33554432`（32 MiB） | 单章缓存图片的总大小上限 |
| `OPENREADER_CHAPTER_IMAGE_TIMEOUT_SECONDS` | `12` | 单张章节图片抓取超时秒数 |
| `OPENREADER_MAX_CHAPTER_IMAGE_REDIRECTS` | `3` | 单张章节图片最大重定向次数 |
| `OPENREADER_MAX_COVER_IMAGE_BYTES` | `8388608`（8 MiB） | 单张下载封面大小上限 |
| `OPENREADER_MAX_COVER_CACHE_BYTES` | `268435456`（256 MiB） | 封面缓存触发淘汰前的总大小上限 |
| `OPENREADER_COVER_IMAGE_TIMEOUT_SECONDS` | `3` | 单张封面抓取超时秒数 |
| `OPENREADER_MAX_COVER_IMAGE_REDIRECTS` | `3` | 单张封面最大重定向次数 |

出于安全考虑，书源和 RSS 默认不能访问本机或局域网地址。如果书源位于自己的 NAS 或局域网服务器，可以只放行它的主机名或 IP：

```yaml
environment:
  OPENREADER_SOURCE_NETWORK_ALLOWLIST: "nas.home,192.168.50.20"
```

OpenReader 不读取容器中的 `HTTP_PROXY`、`HTTPS_PROXY` 或 `ALL_PROXY`。需要代理时，请使用书源自己的代理设置或系统网络路由。使用 Clash 等工具的 fake-IP 模式时可能需要额外放行对应网段；能使用真实 IP 模式时更简单也更安全。

</details>

## 开发

### 本地开发

```bash
git clone https://github.com/changshengyu/openreader.git
cd openreader
```

启动后端：

```bash
cd backend
go run .
```

在仓库根目录另开一个终端启动前端：

```bash
cd frontend
npm ci
npm run dev
```

- 前端：`http://localhost:5173`
- API：`http://localhost:8080`
- 健康检查：`http://localhost:8080/api/health`

### 验证

```bash
cd backend && go test ./...
cd frontend && npm test
cd frontend && npm run build
```

修改阅读器或书架界面后，还应在桌面和手机尺寸的真实浏览器中测试。GitHub Actions 会在发布镜像前再次运行后端、前端和数据兼容性检查。

<details>
<summary>维护者：可选的本地 Docker 构建与发布回退</summary>

正常发布由 [GitHub Actions](.github/workflows/docker-publish.yml) 完成，同时生成 `latest` 和版本标签。本地脚本只用于开发或自动发布失败时的备用方案；Apple Silicon 默认构建 `linux/arm64`：

```bash
docker login ghcr.io
./scripts/docker-build-push.sh
```

正式发布 `linux/amd64` 与 `linux/arm64` 双架构索引：

```bash
RELEASE=1 ./scripts/docker-build-push.sh
docker buildx imagetools inspect ghcr.io/changshengyu/openreader:latest
```

需要时可以通过 `TAG`、`IMAGE`、`PUSH` 和 `PLATFORMS` 调整标签、镜像地址和目标架构。手工发布前必须完成与 GitHub Actions 相同的测试和数据兼容性检查。

</details>

## 技术栈

| 层级 | 技术 |
|---|---|
| 后端 | Go 1.24、Gin、GORM、SQLite WAL |
| 前端 | Vue 3.5、Vite、Pinia、Vue Router、Element Plus |
| 实时通信 | Gorilla WebSocket |
| 内容解析 | goquery、reader-dev/Legado 兼容适配器、本地格式解析器 |
| 部署 | Docker 多阶段构建、Alpine 单运行容器 |

## 致谢

OpenReader 基于 [changshengyu/reader-dev](https://github.com/changshengyu/reader-dev) 的行为与成果进行重构，后者是原 Reader 项目的持续维护 fork。感谢所有上游作者与贡献者。

## 许可证

[GPL v3](LICENSE)
