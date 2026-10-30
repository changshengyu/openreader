<p align="center"><a href="README.md">English</a> · 中文</p>

# OpenReader

**自己部署的网页小说阅读器。** 手机、平板、电脑打开同一个网址就能看书，书架、书签和阅读进度在所有设备间自动同步。

支持「阅读（Legado）」格式的在线书源，也能导入 TXT、EPUB 等本地书。所有数据都保存在你自己的服务器上。

[![Docker 镜像](https://github.com/changshengyu/openreader/actions/workflows/docker-publish.yml/badge.svg)](https://github.com/changshengyu/openreader/actions/workflows/docker-publish.yml)
![支持 amd64 与 arm64](https://img.shields.io/badge/platform-amd64%20%7C%20arm64-2496ED?logo=docker&logoColor=white)
[![许可证 GPL v3](https://img.shields.io/badge/license-GPL%20v3-blue)](LICENSE)

> [!NOTE]
> OpenReader 仍在积极开发中，日常阅读需要的功能已经可用。升级前请先备份；遇到问题欢迎在 [Issues](https://github.com/changshengyu/openreader/issues) 反馈。

[快速开始](#快速开始) · [使用指南](#使用指南) · [备份与迁移](#备份与迁移) · [升级](#升级) · [常用设置](#常用设置) · [常见问题](#常见问题)

## 能做什么

- **随时随地看书**：浏览器打开就能用，界面自动适配手机、平板和电脑；还可以「添加到主屏幕」，像 App 一样全屏打开。
- **进度自动同步**：电脑上看到哪，换到手机上接着看；阅读设置也跟着账号走。
- **在线书源**：导入「阅读（Legado）」/ reader 格式的书源，单源或多源搜索、按分类和排行找书、一键换源、提前缓存章节。
- **本地书**：上传 TXT、EPUB、UMD、CBZ（漫画）文件；导入 TXT 时可以预览和调整章节划分。
- **好用的阅读器**：上下滑动、左右翻页、连续滚动多种方式；字体、字号、行距、页宽、主题和背景随心调整，还能上传自己的字体；支持简繁转换、语音朗读、自动翻页、书签、全文搜索，以及用替换规则去掉广告和水印。
- **RSS 订阅**：在书架旁边直接订阅和阅读文章。
- **备份与迁移**：每天自动备份；可以直接恢复「阅读」App 和 reader-dev 的备份；内置 WebDAV 服务。
- **多人使用**：家人朋友各用各的账号，书架互不干扰；管理员可以管理账号和权限。
- **部署简单**：只需一个 Docker 容器，x86 和 ARM 设备（NAS、树莓派、Apple Silicon Mac 等）都能运行。

## 快速开始

需要一台已经安装 [Docker](https://docs.docker.com/get-docker/) 的电脑、服务器或 NAS。不需要下载源码，也不需要自己编译。

### 1. 启动服务

**方式一：Docker Compose（推荐）**

```bash
mkdir openreader && cd openreader
curl -fsSLO https://raw.githubusercontent.com/changshengyu/openreader/main/docker-compose.yml
docker compose up -d
```

数据保存在当前目录的 `data/`、`cache/`、`library/` 三个文件夹里，备份和搬家都很方便。

**方式二：一条 `docker run` 命令**

```bash
docker run -d --name openreader --restart unless-stopped -p 8080:8080 -v openreader-data:/app/data -v openreader-cache:/app/cache -v openreader-library:/app/library ghcr.io/changshengyu/openreader:latest
```

数据保存在 Docker 管理的三个数据卷里，删除并重建容器不会丢失数据。

同一个镜像同时支持 x86（amd64）和 ARM（arm64），Docker 会自动选择合适的版本。

### 2. 注册管理员账号

用浏览器打开 `http://服务器IP:8080`（在本机运行就是 <http://localhost:8080>），点「创建新账号」完成注册。

- **第一个注册的账号自动成为管理员**，之后注册的都是普通用户。
- 用户名至少 5 位，只能使用英文字母和数字；密码至少 8 位。

> [!WARNING]
> 任何能打开这个网址的人都可以注册账号。启动后请立即注册你自己的管理员账号；打算从外网访问的话，请先阅读[在公网上使用](#在公网上使用)。

### 3. 把书放上书架

OpenReader 不自带书源，可以任选一种方式开始：

- **在线书源**：在左侧菜单点「书源设置 → 导入书源」，选择书源 JSON 文件；或者点「远程书源」，填入书源链接。导入后在左上角的搜索框输入书名，点开搜索结果即可阅读，也可以点「加入书架」。
- **本地书**：点「书架设置 → 导入书籍」，拖入 TXT / EPUB / UMD / CBZ 文件，一次最多 64 个。

手机上的侧边栏默认收起，点左上角的菜单按钮就能打开。

### 4. 在手机上像 App 一样使用（可选）

用手机浏览器打开同一个网址并登录，然后把它添加到主屏幕：

- **iPhone / iPad**：在 Safari 中点「分享」→「添加到主屏幕」。
- **Android**：在 Chrome 等浏览器的菜单中点「添加到主屏幕」或「安装应用」。

之后从桌面图标打开就是全屏阅读界面。手机需要能访问到你的服务器：在同一个局域网内，或者通过公网、VPN 访问。

## 使用指南

### 界面一览

- **左侧边栏**：搜索与搜索设置、最近阅读、书源设置、书架设置、用户空间、WebDAV 和缓存管理。
- **书架右上角**：「编辑」整理书籍，「刷新」检查更新，「RSS」打开订阅，「书海」按书源的分类和排行找书。

### 阅读时的操作

- **呼出菜单**：点击屏幕中间。菜单里有目录、书源（换源）、书签、书籍信息、搜索正文、朗读、自动阅读、缓存章节和设置。
- **翻页**：点击屏幕的上下区域（左右翻页模式下是左右区域），或者用手指滑动、鼠标滚轮。在设置里打开「显示翻页区域」可以看到各个区域的范围。
- **键盘快捷键**（电脑）：

| 按键 | 作用 |
|---|---|
| 空格、PageDown | 下一页 |
| PageUp | 上一页 |
| ↑ / ↓ | 上一页 / 下一页（左右翻页模式除外） |
| ← / → | 上一章 / 下一章（左右翻页模式下为上一页 / 下一页） |
| Home / End | 跳到本章开头 / 结尾 |
| Esc | 返回书架 |

**阅读设置**里可以调整：

- 翻页方式：上下滑动、左右滑动（仅手机）、上下滚动、上下滚动2
- 字体、字号、字重、行高、段落间距、页面宽度和亮度
- 白天 / 黑夜主题和多种背景，也可以上传自己的背景图片和字体（TTF、OTF、WOFF、WOFF2）
- 简繁转换、简洁模式、自动翻页速度，以及点击屏幕时的翻页方式
- 过滤规则管理：用替换规则去掉正文里的广告、水印和多余字符

**朗读**使用浏览器和系统自带的语音，可以选择语音、调节语速和音调，还能定时停止。**缓存章节**会把后面 50 章、100 章或全部章节提前缓存到服务器，书源网站不稳定时也能继续读。

### 多设备同步

在每台设备上登录同一个账号即可。书架、阅读进度、书签和阅读设置都保存在服务器上：在一台设备上读到的位置，另一台设备打开时会自动接上。

如果某台设备上的阅读设置没有更新，可以点「用户空间 → 同步用户配置」手动拉取；「备份用户配置」则会把当前设备的设置保存到账号里。

### 书源

- 兼容「阅读（Legado）」和 reader 的书源 JSON，支持 CSS、XPath、JSONPath、正则等常见规则。
- **不支持需要执行 JavaScript 的书源**（包括 WebJS、WebView 规则），导入时会给出提示。
- 「失效书源」可以检查哪些书源已经不能用；「调试书源」可以一步步查看搜索、目录、正文的解析结果。
- 管理员可以在「管理用户空间」里把某个账号的书源「设为默认书源」，其他用户在「书源管理」里就能一键恢复默认书源。
- 出于安全考虑，书源默认不能访问局域网和本机地址。书源搭在自己的 NAS 上时，请看[访问局域网里的书源](#访问局域网里的书源)。

### 本地书与书仓

- 「导入书籍」上传后会直接加入书架，单个文件默认最大 128 MB。
- 「浏览书仓」用来管理服务器上的书库文件夹，适合存放大量书籍：可以在页面上传，也可以直接把文件放进服务器上的文件夹，再从页面导入书架。用 Compose 部署时，管理员的书仓是部署目录下的 `library/localStore/`，其他用户的是 `library/localStore/users/<用户名>/`。
- 旧版本导入的 PDF、Markdown 书仍然可以继续阅读；新导入支持 TXT、EPUB、UMD、CBZ。

### RSS 订阅

点书架右上角的「RSS」，可以新增 RSS 源或导入 RSS 源的 JSON 文件，然后浏览和阅读文章。

### 多用户与权限

- 管理员在「用户空间 → 管理用户空间」里可以新增用户、重置密码、批量删除用户，并单独控制每个用户能否编辑书源、使用书仓和 WebDAV。
- 每个人的书架、进度、书签、书源和 WebDAV 文件都相互隔离。

## 备份与迁移

### 自动备份

每天 23:50，OpenReader 会为每个用户生成一份 `backup_日期_时间.zip`，放在该用户的 WebDAV 空间里。备份包含书源、书架、分组、RSS、设置、书签、阅读进度和替换规则，**不包含本地书原文件**。

- 时间按容器时区计算，默认是 UTC（相当于北京时间早上 7:50），可以[设置时区](#设置时区)。
- 旧备份不会自动删除，需要时可以在「WebDAV → 文件管理」里清理。

### 手动备份

| 方式 | 包含内容 | 适合 |
|---|---|---|
| 「WebDAV → 保存备份」 | 当前账号的数据（同自动备份），不含本地书原文件 | 日常备份 |
| 「WebDAV → 保存完整可移植备份」 | 账号数据，加上本地书原文件和自定义封面、背景、字体 | 把账号搬到另一台 OpenReader |
| 停止容器后复制 `data/`、`cache/`、`library/` | 所有用户的全部数据 | 整机备份、搬家 |

备份文件可以在「WebDAV → 文件管理」里下载。账号里有音频目录、缺失的原文件或无效的自定义资源时，无法生成完整可移植备份。

### 从「阅读（Legado）」或 reader-dev 迁移

1. 在原来的 App 或服务里导出备份文件（`backup*.zip`）。这类备份一般不包含 TXT、EPUB 等本地书原文件，请另外保存好。
2. 登录 OpenReader，打开「WebDAV → 文件管理 → 上传文件」，上传这个 ZIP（不用解压，也不用改名）。
3. 在文件列表里点这个文件旁边的「恢复」，等待页面显示恢复结果。
4. 用「导入书籍」重新导入本地书。
5. 确认书架、书源、书签和进度都正常后，再停用旧服务。

注意：

- 数据会恢复到**当前登录的账号**，旧账号的密码不会迁移。目标账号里已经有数据时，建议先备份。
- 当前账号没有编辑书源的权限时，书源会被跳过。
- 不要把 reader-dev 的数据库文件复制给 OpenReader，两者格式不同。

### 搬到另一台 OpenReader

- **整个实例（所有用户）**：停止容器，把 `data/`、`cache/`、`library/` 和 `docker-compose.yml`（以及你改过的环境变量）一起复制到新机器，用相同或更新的镜像启动。确认能登录、本地书和在线书都能打开后，再停用旧机器。
- **单个账号**：在旧实例点「保存完整可移植备份」，下载生成的 `portable_backup_*.zip`；在新实例登录目标账号后上传，再点「恢复」。

### 配合其他 WebDAV 客户端

OpenReader 内置 WebDAV 服务，每个用户的空间相互独立：

- 地址：`http://服务器IP:8080/reader3/webdav/`（也可以用 `/webdav/`）
- 用户名和密码：你的 OpenReader 账号

例如，把「阅读（Legado）」App 的 WebDAV 备份地址设成它，App 上传的备份就会出现在「文件管理」里，可以直接恢复。阅读进度也可以通过 WebDAV 在 App 和 OpenReader 之间同步（书需要已经在两边的书架上，按书籍链接或书名加作者对应）。这项联动还在完善中，欢迎反馈使用情况。

## 升级

> [!TIP]
> 升级前请先备份。只刷新网页不会升级，需要更新容器。

**Docker Compose**（在 `docker-compose.yml` 所在的目录执行）：

```bash
docker compose stop
tar -czf "../openreader-backup-$(date +%Y%m%d-%H%M%S).tar.gz" data cache library docker-compose.yml
docker compose pull
docker compose up -d
```

第二行会在上一级目录生成一份完整备份，里面有所有用户的数据，请妥善保管。Linux 上如果提示权限不足，在 `tar` 前面加上 `sudo`。

**docker run**：先在页面里保存并下载一份完整可移植备份，然后执行：

```bash
docker pull ghcr.io/changshengyu/openreader:latest
docker stop openreader
docker rm openreader
```

再执行一次[第 1 步](#1-启动服务)里的 `docker run` 命令，三个数据卷会继续保留。

**确认版本**：左侧边栏「阅读」旁边会显示当前版本号；也可以访问 `http://服务器IP:8080/api/health`，查看 `version` 和 `commit`。

**固定版本与回退**：除了 `latest`，每次发布还会有一个 7 位提交号标签，可以在[镜像页面](https://github.com/changshengyu/openreader/pkgs/container/openreader)查看。把 `image` 里的 `latest` 换成具体标签（如 `ghcr.io/changshengyu/openreader:<提交号>`）即可固定版本。回退时，先停止容器、完整恢复升级前备份的三个文件夹，再用旧标签启动；不要把新旧数据库文件混在一起。`4c40a4a` 之前的版本不使用 `data/instance-secret`，回退到这些版本后，在新版本上登录过的设备需要重新登录。

## 常用设置

修改 `docker-compose.yml` 后执行 `docker compose up -d` 生效。使用 `docker run` 的话，把对应的 `-e 变量=值` 加进命令里，删除旧容器后重新运行。

### 修改端口

把 `ports` 左边的数字改掉，例如 `"18080:8080"`，然后访问 `http://服务器IP:18080`。右边的 `8080` 不要改。

### 设置时区

```yaml
    environment:
      TZ: Asia/Shanghai
```

时区会影响每天自动备份的时间，以及备份文件名里的时间。

### 在公网上使用

只在家里局域网使用时，默认配置就能用。需要从外网访问时，请做好下面几件事：

1. **保管好 `data` 文件夹。** 首次启动时 OpenReader 会自动生成随机签名密钥 `data/instance-secret`，用它签名登录状态和资源链接。能读到这个文件的人就能伪造它们，所以不要外传；备份或迁移服务器时，让它和 `data/` 里的其他文件放在一起。不需要设置 `OPENREADER_JWT_SECRET`。如果仍要设置，请用足够长的随机字符串（例如 `openssl rand -hex 32` 的输出，不要照抄教程里的示例值）：它会取代自动生成的密钥，所有设备需要重新登录一次。
2. **通过 HTTPS 反向代理访问**（如 Caddy、Nginx），不要把 8080 端口直接暴露到公网。
3. **注意开放注册。** 目前任何能打开网页的人都能注册账号。不希望陌生人注册时，可以把服务放在 VPN（如 Tailscale、WireGuard）后面，或者只允许信任的 IP 访问；管理员也可以在「管理用户空间」里删除不认识的账号。

Caddy 会自动申请 HTTPS 证书，最简单的配置只有三行：

```caddyfile
reader.example.com {
    reverse_proxy 127.0.0.1:8080
}
```

<details>
<summary>Nginx 配置示例</summary>

实时同步需要转发 WebSocket（`/ws/`），并放宽上传大小限制：

```nginx
server {
    listen 443 ssl;
    server_name reader.example.com;
    # ssl_certificate 和 ssl_certificate_key 按你的证书填写

    client_max_body_size 512m;  # 允许上传大文件和备份

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    location /ws/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_read_timeout 1h;
    }
}
```

</details>

可选：把 `OPENREADER_TRUSTED_PROXIES` 设为反向代理自己的 IP 或网段（多个用英文逗号分隔），日志和访问频率限制就能识别访客的真实 IP。只填代理的地址，不要填访客的网段；不确定就留空，留空也能正常使用。

### 访问局域网里的书源

出于安全考虑，书源和 RSS 默认不能访问本机和局域网地址。如果书源搭在自己的 NAS 或局域网服务器上，只放行它的主机名或 IP：

```yaml
    environment:
      OPENREADER_SOURCE_NETWORK_ALLOWLIST: "nas.home,192.168.50.20"
```

### 网络代理

OpenReader 不会读取容器里的 `HTTP_PROXY`、`HTTPS_PROXY`、`ALL_PROXY`。需要代理时，请使用书源自身的代理设置或系统层面的网络路由。使用 Clash 等工具的 fake-IP 模式时，可能需要额外放行对应的网段；能用真实 IP 模式会更简单、更安全。

### 全部环境变量

普通部署不需要设置下面这些变量，只在有特殊需要时调整。

<details>
<summary>基本设置</summary>

| 变量 | 默认值 | 说明 |
|---|---|---|
| `OPENREADER_JWT_SECRET` | 不设置：自动生成并保存在 `data/instance-secret` | 用于签名登录状态和资源链接，可不设置，见[在公网上使用](#在公网上使用) |
| `OPENREADER_TRUSTED_PROXIES` | 空 | 反向代理自己的 IP 或网段，多个用英文逗号分隔；直接访问时留空 |
| `OPENREADER_SOURCE_NETWORK_ALLOWLIST` | 空 | 允许书源访问的局域网主机名、IP 或网段，多个用英文逗号分隔 |
| `OPENREADER_CHECK_INTERVAL` | `30m` | 定时检查书架更新的间隔 |
| `OPENREADER_RATE_LIMIT_PER_MINUTE` | `6000` | 单个客户端每分钟的请求上限 |
| `OPENREADER_ADDR` | `:8080` | 服务监听地址 |
| `OPENREADER_DATA_DIR` | `data` | 数据目录 |
| `OPENREADER_CACHE_DIR` | `cache` | 缓存目录 |
| `OPENREADER_LIBRARY_DIR` | `library` | 书库目录 |
| `OPENREADER_LOCAL_STORE_DIR` | `library/localStore` | 书仓目录 |
| `OPENREADER_DB` | `data/openreader.db` | SQLite 数据库路径 |
| `OPENREADER_PUBLIC_DIR` | `public` | 前端文件目录 |
| `OPENREADER_CORS_ORIGIN` | `http://localhost:5173` | 只在前后端分开运行的开发环境中使用 |

Docker 镜像已经把各个目录设置为 `/app/data`、`/app/cache`、`/app/library` 等容器内路径，请不要修改这些目录相关的变量。

</details>

<details>
<summary>文件大小和网络限制</summary>

| 变量 | 默认值 | 控制内容 |
|---|---:|---|
| `OPENREADER_SOURCE_REQUEST_TIMEOUT_SECONDS` | `15` | 单次书源或 RSS 请求的超时秒数 |
| `OPENREADER_MAX_SOURCE_RESPONSE_BYTES` | `16777216`（16 MiB） | 单个远程响应的大小上限 |
| `OPENREADER_MAX_SOURCE_REDIRECTS` | `5` | 单次远程请求的最大重定向次数 |
| `OPENREADER_MAX_SOURCE_RETRIES` | `3` | 可重试远程请求的最大尝试次数 |
| `OPENREADER_MAX_IMPORT_BYTES` | `134217728`（128 MiB） | 上传本地书文件的大小上限 |
| `OPENREADER_MAX_ARCHIVE_ENTRIES` | `20000` | 单个书籍压缩包内的文件数量上限 |
| `OPENREADER_MAX_ARCHIVE_ENTRY_BYTES` | `134217728`（128 MiB） | 书籍压缩包内单个文件解压后的大小上限 |
| `OPENREADER_MAX_ARCHIVE_EXPANDED_BYTES` | `536870912`（512 MiB） | 单个书籍压缩包解压后的总大小上限 |
| `OPENREADER_MAX_PDF_PAGES` | `10000` | 单个 PDF 的最大解析页数 |
| `OPENREADER_MAX_PARSED_TEXT_BYTES` | `268435456`（256 MiB） | 解析本地书时保留的文本大小上限 |
| `OPENREADER_MAX_PARSED_CHAPTERS` | `100000` | 本地书解析出的章节数量上限 |
| `OPENREADER_MAX_UMD_CHAPTERS` | `100000` | UMD 专用章节上限（兼容旧配置） |
| `OPENREADER_MAX_BACKUP_RESTORE_BYTES` | `134217728`（128 MiB） | 上传的普通备份 ZIP 的大小上限 |
| `OPENREADER_MAX_BACKUP_ARCHIVE_ENTRIES` | `5000` | 普通备份内的文件数量上限 |
| `OPENREADER_MAX_BACKUP_ARCHIVE_ENTRY_BYTES` | `16777216`（16 MiB） | 普通备份内单个文件解压后的大小上限 |
| `OPENREADER_MAX_BACKUP_ARCHIVE_EXPANDED_BYTES` | `134217728`（128 MiB） | 普通备份解压后的总大小上限 |
| `OPENREADER_MAX_PORTABLE_BACKUP_BYTES` | `536870912`（512 MiB） | 完整可移植备份的大小上限 |
| `OPENREADER_MAX_PORTABLE_ARCHIVE_ENTRIES` | `10000` | 完整可移植备份内的文件数量上限 |
| `OPENREADER_MAX_PORTABLE_ARCHIVE_ENTRY_BYTES` | `268435456`（256 MiB） | 完整可移植备份内单个文件解压后的大小上限 |
| `OPENREADER_MAX_PORTABLE_ARCHIVE_EXPANDED_BYTES` | `536870912`（512 MiB） | 完整可移植备份解压后的总大小上限 |
| `OPENREADER_MAX_CHAPTER_IMAGES` | `64` | 单章最多缓存的图片数量 |
| `OPENREADER_MAX_CHAPTER_IMAGE_BYTES` | `8388608`（8 MiB） | 单张章节图片的大小上限 |
| `OPENREADER_MAX_CHAPTER_IMAGE_TOTAL_BYTES` | `33554432`（32 MiB） | 单章图片的总大小上限 |
| `OPENREADER_CHAPTER_IMAGE_TIMEOUT_SECONDS` | `12` | 单张章节图片的下载超时秒数 |
| `OPENREADER_MAX_CHAPTER_IMAGE_REDIRECTS` | `3` | 单张章节图片的最大重定向次数 |
| `OPENREADER_MAX_COVER_IMAGE_BYTES` | `8388608`（8 MiB） | 单张封面的大小上限 |
| `OPENREADER_MAX_COVER_CACHE_BYTES` | `268435456`（256 MiB） | 封面缓存的总大小上限，超出后自动清理旧封面 |
| `OPENREADER_COVER_IMAGE_TIMEOUT_SECONDS` | `3` | 单张封面的下载超时秒数 |
| `OPENREADER_MAX_COVER_IMAGE_REDIRECTS` | `3` | 单张封面的最大重定向次数 |

</details>

## 常见问题

**搜不到书，或者书源用不了？**
先在「失效书源」里检查书源是否还有效。需要 JavaScript 的书源不受支持；书源在局域网里时需要[先放行](#访问局域网里的书源)；书源网站也可能屏蔽了服务器所在的 IP。用「调试书源」可以看到具体是哪一步出了错。

**侧边栏显示「后端未连接」？**
说明网页连不上服务器。先确认容器正在运行（`docker compose ps` 或 `docker ps`），再用 `docker logs openreader` 查看日志。

**忘记密码怎么办？**
请管理员在「管理用户空间」里点「重置密码」。管理员自己的密码请务必记牢。

**别人能注册我的 OpenReader 吗？**
能。不希望陌生人注册，请看[在公网上使用](#在公网上使用)。

**能离线看书吗？**
网页需要连接到你的 OpenReader 服务器。提前「缓存章节」后，即使书源网站打不开，也能继续阅读已缓存的章节。

**升级会丢数据吗？**
只要保留 `data/`、`cache/`、`library/`（或三个数据卷），升级不会丢数据。升级前仍建议先备份。

**数据存在哪里？**

| 文件夹 | 内容 | 备份时 |
|---|---|---|
| `data/` | 数据库、上传的图片和字体、每个用户的 WebDAV 文件和自动备份 | 必须保留 |
| `cache/` | 章节缓存、导入预览等，大多可以重新生成 | 整机搬家时一起复制 |
| `library/` | 导入的本地书原文件和书仓 | 必须保留 |

**和 reader-dev、「阅读」App 是什么关系？**
OpenReader 用 Go 和 Vue 3 重新实现了 [reader-dev](https://github.com/changshengyu/reader-dev)（[hectorqin/reader](https://github.com/hectorqin/reader) 的维护分支）的网页阅读体验，书源格式与「[阅读（Legado）](https://github.com/gedoor/legado)」App 兼容。它可以导入两者的备份，但不能直接使用 reader-dev 的数据库。

## 参与开发

技术栈：Go（Gin、GORM、SQLite）+ Vue 3（Vite、Pinia、Element Plus），前后端打包在同一个 Docker 镜像里。本地运行需要 Go 1.26.10 或更新版本（需启用 CGO，用于 SQLite）和 Node.js 22。

```bash
git clone https://github.com/changshengyu/openreader.git
cd openreader
```

启动后端（<http://localhost:8080>）：

```bash
cd backend
go run .
```

在仓库根目录另开一个终端，启动前端（<http://localhost:5173>）：

```bash
cd frontend
npm ci
npm run dev
```

运行测试：

```bash
(cd backend && go test ./...)
(cd frontend && npm test && npm run build)
```

镜像由 GitHub Actions 在 `main` 分支更新后自动测试并发布。更多开发文档在 [`docs/`](docs/) 目录，浏览器回归测试见 [`docs/regression-smoke.md`](docs/regression-smoke.md)。

## 致谢

OpenReader 的功能和交互参考了 [changshengyu/reader-dev](https://github.com/changshengyu/reader-dev) 和原 [Reader](https://github.com/hectorqin/reader) 项目，书源生态来自「[阅读（Legado）](https://github.com/gedoor/legado)」社区。感谢所有上游作者和贡献者。

## 许可证

[GPL v3](LICENSE)
