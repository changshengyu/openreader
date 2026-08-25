<p align="center"><a href="README.md">English</a></p>

# OpenReader

轻量级、自部署、多端同步的小说阅读器，支持在线书源、本地书导入、WebDAV、RSS，并持续对齐 reader-dev 的阅读体验。

欢迎各位使用 OpenReader，并积极提交 [Issues](https://github.com/changshengyu/openreader/issues) 反馈问题与建议。

![Go 1.24+](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go)
![Vue 3.5](https://img.shields.io/badge/Vue-3.5-4FC08D?logo=vue.js)
![SQLite WAL](https://img.shields.io/badge/SQLite-WAL-brightgreen)
![项目状态：WIP](https://img.shields.io/badge/status-WIP-F59E0B)
![Docker ready](https://img.shields.io/badge/Docker-ready-2496ED?logo=docker)
[![自动构建并发布 Docker 镜像](https://github.com/changshengyu/openreader/actions/workflows/docker-publish.yml/badge.svg)](https://github.com/changshengyu/openreader/actions/workflows/docker-publish.yml)

> [!IMPORTANT]
> OpenReader 是基于 [changshengyu/reader-dev](https://github.com/changshengyu/reader-dev) 行为进行的独立 Go/Vue 重构与重写，并非其可执行文件或数据库的原位替代品。项目以 [`fa22f271`](https://github.com/changshengyu/reader-dev/commit/fa22f271849d45f93349ae1636223e27b16a4691) 为固定兼容基线，目前仍在持续重构；各模块现状见[兼容性审查矩阵](docs/compat/refactor-audit-matrix.md)。
>
> 服务端进程有意不执行 JavaScript、WebJS 和 WebView 书源规则。导入/导出会保留这些字段，但依赖它们的书源会返回明确的规则不支持错误。CSS、JSONPath、XPath、正则、组合、分页和受限变量规则的支持范围见[在线书源解析合同](docs/compat/online-booksource-parser.md)。

## 开发状态（WIP）

OpenReader 仍是**开发中的项目**，不是已经完成全部兼容验收的稳定版本。截至 2026-08-25，固定基准审查估算约有 **99% 的合同与测试覆盖**。该比例表示已提取上游合同并实现回归验证的覆盖度，不代表零缺陷，也不代表所有设备均已验收完成。

- **P0 Reader 主链：** 阅读工具层、分页与连续滚动、手机/平板/桌面布局、设置、书签、正文搜索、EPUB、CBZ、音频、TTS、登录恢复、换源和章节缓存均已实现并通过回归；仍在等待更多真实设备签收。
- **P1 Index 主链：** 书架、搜索/探索、BookInfo、BookManage、BookGroup、导入、本地书仓、WebDAV、书源管理、刷新、进度与旧路由均已覆盖；仍在等待更多真实设备签收。
- **P2 后端与数据：** 多用户隔离、书源 ownership、备份/portable v2、WebDAV、RSS、替换规则、书签、用户管理、WebSocket、抓取安全、解析预算和主要请求边界均已覆盖。
- **剩余工作：** 长尾 REST 动作审查、尚未完成的第二轮上游复审、真实设备反馈暴露的可见差异及最终设备验收。JavaScript/WebView 书源执行仍是明确的安全限制，不是被静默隐藏的兼容能力。

完整证据、剩余项和状态定义见[全量兼容性审查矩阵](docs/compat/refactor-audit-matrix.md)。

## 功能特性

- **本地书籍** — 导入 TXT、EPUB、UMD、CBZ，支持 TXT 目录预览与自定义目录规则；旧版 OpenReader 已有的 Markdown/PDF 存档仍可读取。
- **在线书源** — 导入和管理 reader-dev/Legado 兼容书源，多书源搜索、浏览目录、切换书源并缓存章节。
- **对齐上游的阅读器** — 上下滑动、左右滑动、连续上下滚动；分别适配桌面、手机和平板；支持书签、正文搜索、进度同步、主题、排版、自动阅读和 TTS。
- **书架工作台** — 分组、搜索、批量操作、元数据编辑、本地书仓、WebDAV 文件管理和跨客户端刷新。
- **内容清洗** — 按顺序执行正则替换规则，去除广告、水印和排版噪音。
- **RSS 与探索** — 导入 RSS 源、浏览文章和探索书源目录。
- **备份恢复** — 恢复 reader-dev/Legado 兼容逻辑 ZIP，生成 OpenReader 逻辑备份，以及包含可恢复本地书原文件和受支持自定义外观资源的 portable v2 备份。
- **多用户** — JWT 身份认证、用户数据隔离、书源/书仓/WebDAV 权限和管理员用户管理。
- **单容器部署** — 一个 Go 二进制同时提供 API 和 Vue 页面，SQLite 使用 WAL 模式。

## 快速开始

部署不需要克隆源码，也不需要在本地构建镜像。任选下面一种方式即可。

### 方式一：Docker Compose（推荐）

新建一个空的部署目录，只下载 [`docker-compose.yml`](https://raw.githubusercontent.com/changshengyu/openreader/main/docker-compose.yml)：

```bash
curl -fsSLO https://raw.githubusercontent.com/changshengyu/openreader/main/docker-compose.yml
docker compose up -d
curl -fsS http://localhost:8080/api/health
```

这种方式会在 `docker-compose.yml` 旁创建 `data/`、`cache/`、`library/`，便于备份和整实例迁移。

### 方式二：单行 `docker run`

下面的命令不需要任何项目文件，持久数据保存在三个 Docker named volume 中：

```bash
docker run -d --name openreader --restart unless-stopped -p 8080:8080 -v openreader-data:/app/data -v openreader-cache:/app/cache -v openreader-library:/app/library ghcr.io/changshengyu/openreader:latest
```

重新创建容器不会删除这些 named volume。升级时不要删除 `openreader-data`、`openreader-cache`、`openreader-library`。

已发布镜像是包含 `linux/amd64` 与 `linux/arm64` 的 OCI 索引。Docker 会自动拉取镜像并选择与主机匹配的架构；普通用户不需要源码仓库、Go、Node.js、QEMU 或本地镜像构建。需要显式拉取时可执行 `docker pull ghcr.io/changshengyu/openreader:latest`。

打开 `http://localhost:8080`。空数据库中注册的第一个账号会成为管理员，后续注册账号为普通用户。

使用 Compose 时，默认配置无需额外设置环境变量即可启动。只修改与你的主机不一致的项目：

| Compose 配置 | 默认值 | 何时需要修改 |
|---|---|---|
| `image` | `ghcr.io/changshengyu/openreader:latest` | 需要固定版本时，将 `latest` 替换为 commit 标签。 |
| `ports` | `8080:8080` | 宿主机 8080 已被占用时，只修改左侧端口；容器端口保持不变。 |
| `./data` | SQLite、上传资源和备份 | 持久数据需要放到其他磁盘或目录时，修改左侧路径。 |
| `./cache` | 可重建的正文/导入缓存 | 缓存需要放到其他磁盘或目录时，修改左侧路径。 |
| `./library` | 本地书原文件和 LocalStore | 书库需要放到其他磁盘或目录时，修改左侧路径。 |

容器内 `/app/data`、`/app/cache`、`/app/library` 三个路径不要修改。重新创建容器时，不要删除或替换宿主机上的三个持久化目录。

### 升级已有 OpenReader

升级前先制作冷备份。停止容器可以确保 SQLite 数据库及其 WAL 文件被一致地保存：

```bash
docker compose stop openreader
tar -czf "../openreader-volume-backup-$(date +%Y%m%d-%H%M%S).tar.gz" data cache library docker-compose.yml
docker compose pull openreader
docker compose up -d --force-recreate openreader
curl -fsS http://localhost:8080/api/health
```

该归档包含用户数据，请限制访问并妥善保管。仓库自带的 Compose 文件设置了 `pull_policy: always`。`/api/health` 返回的 `version` 与 `commit` 才代表正在运行的代码；浏览器强制刷新不能升级容器。

需要可控发布时，建议固定使用 `ghcr.io/changshengyu/openreader:<commit>`，不要直接跟随 `latest`。如需安全回滚，应停止新容器，把升级前的完整快照恢复到空的持久化目录，固定旧镜像后再启动。不要把旧 SQLite 快照合并到已经被新容器写入的目录中。

## 迁移指南

请根据迁移对象选择方式：

| 来源 | 推荐方式 | 可保留内容 |
|---|---|---|
| reader-dev 或 Legado | 将原项目的逻辑 `backup*.zip` 恢复到目标 OpenReader 账号 | 备份中存在的受支持书源、书架记录、分组、RSS、书签、替换规则和进度 |
| 另一 OpenReader 账号/主机 | 创建并恢复 **OpenReader 完整可移植备份** | 账号逻辑数据，以及可恢复的本地书原文件和受支持的自定义封面/背景/字体 |
| 整套 OpenReader 实例 | 停机后同时复制 `data/`、`cache/`、`library/` 和部署配置 | 所有用户、SQLite 数据、上传资源、备份、本地书原文件和缓存正文 |
| 同一主机上的旧版 OpenReader | 保留原有三个挂载目录，用新镜像重新创建容器 | 通过启动时加性迁移保留现有实例数据 |

### 从原 reader-dev 项目迁移

1. **在 reader-dev 中制作最终备份。** 使用原项目已有的备份/WebDAV 操作生成 `backup*.zip` 并下载；同时单独备份 reader-dev 数据目录和所有本地书原文件。
2. **保留旧服务不动。** 不要让 OpenReader 直接使用 reader-dev 数据库，也不要用它覆盖 `data/openreader.db`；两者的数据库结构和文件布局不同。
3. **启动 OpenReader 并创建目标账号。** 全新实例先注册管理员。原 reader-dev 如果有多个用户，应登录对应的 OpenReader 账号后逐个恢复；逻辑备份不包含账号和密码。
4. **上传原备份。** 打开侧边栏中的 **WebDAV → 文件管理**，点击 **上传文件**，直接上传原 ZIP，不需要解压，也不要改动其中 JSON 文件名。
5. **执行恢复。** 在该 ZIP 所在行点击 **恢复** 并确认。OpenReader 可识别 `bookSource.json`、`bookshelf.json` 或 `myBookShelf.json`、`bookGroup.json`、`rssSources.json`、`bookmark.json`、`replaceRule.json`、`bookProgress/*.json` 等 reader-dev/Legado 文件名。
6. **检查恢复摘要。** 没有书源编辑权限的普通用户仍会恢复个人数据，但书源会被跳过，界面会明确提示。如需书源，应先由管理员授予书源编辑权限再重试。
7. **重新导入本地原文件。** 普通 reader-dev/Legado 逻辑备份**不包含** TXT/EPUB/UMD/CBZ 原文件，需要另外上传并导入。OpenReader 的 portable 备份可以携带受支持的本地原文件，但前提是这些书已经迁入 OpenReader。
8. **切换访问流量前进行验收。** 对比书架和书源数量，打开数本远程书和本地书，检查书签、进度，并测试一次书源刷新。完成验收前保留旧服务和原始备份。

恢复数据会归属到当前登录的 OpenReader 账号，并重新分配目标数据库 ID；旧实例中的数据库 ID、用户记录、密码、JWT 会话、WebDAV 凭据和宿主机路径都不会复用。受支持的逻辑数据会事务性恢复，但恢复仍是写操作——目标账号已有数据时请先备份。

### 将 OpenReader 迁移到另一台主机

完整迁移整套实例时：

1. 停止源主机上的 OpenReader 容器。
2. 在服务停止期间同时复制 `data/`、`cache/` 和 `library/`；另行复制 Compose 文件和自行设置的环境变量覆盖项。
3. 目标主机使用相同或更新的 OpenReader 镜像，并保持相同容器内挂载路径。
4. 检查 `/api/health`，登录并分别打开本地书和远程书，验证完成后再停用源主机。

只迁移单个账号时，在源端使用 **WebDAV → 保存完整可移植备份**，下载生成的 `portable_backup_*.zip`，上传到目标账号的 WebDAV 文件管理器后点击 **恢复**。如果引用的本地原文件或受支持的自定义资源缺失，portable 生成会主动失败，避免产生残缺备份。本地音频目录不会包含在 portable 包中。

### 备份类型与边界

- `backup_*.zip` 是账号逻辑备份，兼容受支持的 reader-dev/Legado JSON 数据，但不包含 SQLite 数据库和本地书原文件。
- `portable_backup_*.zip` 是 OpenReader portable v2 包，在逻辑数据之外增加经过校验的本地书原文件和当前用户受支持的外观资源；旧 portable v1 仍可恢复。
- 只有停机后完整复制三个持久化目录，才能得到涵盖所有用户的系统级完整备份。
- 不要把 reader-dev 的 SQLite 数据库复制到 `data/openreader.db`，也不要只恢复 OpenReader 三个目录中的某一个并将其视为完整实例迁移。

## 持久化数据

| 目录 | 用途 | 备份说明 |
|---|---|---|
| `data/` | SQLite 数据库、上传资源、各用户 WebDAV/备份文件 | 每次完整备份都必须保存 |
| `cache/` | 章节正文和导入预览缓存 | 多数可重建，但精确迁移时应复制 |
| `library/` | 导入的本地书原文件和 LocalStore 内容 | 保留本地书时必须保存 |

管理员沿用历史 WebDAV 根 `data/webdav/`，普通用户使用各自隔离的子目录。WebDAV 协议同时提供 `/webdav/` 和兼容 reader-dev 的 `/reader3/webdav/` 路径。

## 配置

仓库自带的 Compose 已通过镜像默认值提供容器路径。以下服务变量主要用于源码开发、自定义编排或有意识地调整资源上限：

| 变量 | 默认值 | 说明 |
|---|---|---|
| `OPENREADER_ADDR` | `:8080` | 服务监听地址 |
| `OPENREADER_DATA_DIR` | `data` | 数据目录 |
| `OPENREADER_CACHE_DIR` | `cache` | 缓存目录 |
| `OPENREADER_LIBRARY_DIR` | `library` | 书库目录 |
| `OPENREADER_LOCAL_STORE_DIR` | `library/localStore` | 本地书仓根目录 |
| `OPENREADER_DB` | `data/openreader.db` | SQLite 数据库路径 |
| `OPENREADER_JWT_SECRET` | 兼容回退值 | JWT/资源 capability 签名密钥；同源的标准 Compose 部署无需覆盖 |
| `OPENREADER_CORS_ORIGIN` | `http://localhost:5173` | 前端/API 分离开发时的 CORS 响应来源；普通同源容器访问无需修改 |
| `OPENREADER_PUBLIC_DIR` | `public` | 已构建前端目录 |
| `OPENREADER_CHECK_INTERVAL` | `30m` | 书架/书源定时检查间隔 |
| `OPENREADER_RATE_LIMIT_PER_MINUTE` | `6000` | 单客户端 API 每分钟请求上限 |
| `OPENREADER_SOURCE_NETWORK_ALLOWLIST` | 空 | 允许访问非公网目标的精确主机、IP 或 CIDR，使用英文逗号分隔 |

<details>
<summary>解析、网络、备份和资源安全上限</summary>

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

</details>

书源和 RSS 请求默认拒绝回环、私网、链路本地、云 metadata、benchmark、documentation 等特殊用途网络。访问可信局域网书源时，尽量只放行精确主机名或地址：

```yaml
environment:
  OPENREADER_SOURCE_NETWORK_ALLOWLIST: "nas.home,192.168.50.20"
```

共享抓取器有意忽略进程中的 `HTTP_PROXY`、`HTTPS_PROXY`、`ALL_PROXY`，请使用书源自己的代理设置或 TUN/系统路由。`198.18.0.0/15` 等 fake-IP DNS 网段需要显式放行，这会授权整个网段；条件允许时优先使用 real-IP/Redir-Host DNS。

## 开发

### 本地开发

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

- 前端：`http://localhost:5173`
- API：`http://localhost:8080`
- 健康检查：`http://localhost:8080/api/health`

### 验证

```bash
cd backend && go test ./...
cd frontend && npm test
cd frontend && npm run build
```

阅读器和工作台修改还必须执行真实浏览器冒烟测试。符合触发条件的 `main` 更新会由仓库 workflow 重复执行后端/前端闸门，构建原生候选镜像，验证新卷、历史卷和 portable 备份后，才发布多架构镜像。

<details>
<summary>维护者：可选的本地 Docker 构建与发布回退</summary>

正常发布由 [GitHub Actions](.github/workflows/docker-publish.yml) 完成，同时生成 `latest` 和七位 commit 标签。本地脚本保留用于开发或发布故障回退；Apple Silicon 开发期默认使用 `linux/arm64`：

```bash
docker login ghcr.io
./scripts/docker-build-push.sh
```

正式发布 `linux/amd64` 与 `linux/arm64` 双架构索引：

```bash
RELEASE=1 ./scripts/docker-build-push.sh
docker buildx imagetools inspect ghcr.io/changshengyu/openreader:latest
```

常用覆盖参数包括 `TAG`、`IMAGE`、`PUSH=0`、`PLATFORMS`、`BUILD_PROGRESS=plain` 和 `HOST_OCI_PUSH`。两种发布路径都会写入 `VERSION`、`VCS_REF`、`BUILD_DATE`；手工回退发布也必须通过与 workflow 相同的验证和卷/备份兼容性闸门。

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
