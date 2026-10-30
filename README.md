<p align="center">English · <a href="README_CN.md">中文</a></p>

# OpenReader

**A self-hosted web reader for novels and ebooks.** Open the same address on your phone, tablet, or computer and pick up where you left off: your bookshelf, bookmarks, and reading progress sync across every device.

It works with online book sources in the Legado (阅读) format and can import local TXT, EPUB, and other files. Everything stays on your own server.

[![Docker image](https://github.com/changshengyu/openreader/actions/workflows/docker-publish.yml/badge.svg)](https://github.com/changshengyu/openreader/actions/workflows/docker-publish.yml)
![Runs on amd64 and arm64](https://img.shields.io/badge/platform-amd64%20%7C%20arm64-2496ED?logo=docker&logoColor=white)
[![License: GPL v3](https://img.shields.io/badge/license-GPL%20v3-blue)](LICENSE)

> [!NOTE]
> OpenReader is under active development, and the everyday reading features are ready to use. Back up before upgrading, and please report problems in [Issues](https://github.com/changshengyu/openreader/issues).
>
> The interface is currently in Chinese only. This guide quotes on-screen labels in Chinese, followed by an English translation.

[Quick start](#quick-start) · [Using OpenReader](#using-openreader) · [Backup and migration](#backup-and-migration) · [Upgrading](#upgrading) · [Configuration](#configuration) · [FAQ](#faq)

## Features

- **Read anywhere**: works in any modern browser and adapts to phones, tablets, and desktops. Add it to your home screen to open it full-screen like an app.
- **Progress that follows you**: stop reading on your computer and continue on your phone. Reader settings follow your account too.
- **Online book sources**: import Legado or reader-compatible sources, search one source or many, browse categories and rankings, switch sources in one tap, and cache chapters ahead of time.
- **Local books**: upload TXT, EPUB, UMD, and CBZ (comics) files, and preview or adjust chapter detection when importing TXT.
- **A comfortable reader**: vertical paging, horizontal page turns, or continuous scrolling; adjust the font, size, line height, page width, theme, and background, or upload your own fonts. It also offers Simplified/Traditional Chinese conversion, read-aloud, auto page turning, bookmarks, full-text search, and replacement rules that strip ads and watermarks.
- **RSS**: subscribe to and read feeds right next to your bookshelf.
- **Backup and migration**: automatic daily backups, direct restore of Legado and reader-dev backups, and a built-in WebDAV server.
- **Multiple users**: family and friends get separate accounts and bookshelves, and administrators manage accounts and permissions.
- **Easy to run**: a single Docker container that runs on x86 and ARM (NAS boxes, Raspberry Pi, Apple Silicon Macs, and more).

## Quick start

You need a computer, server, or NAS with [Docker](https://docs.docker.com/get-docker/) installed. There is no source code to download and nothing to build.

### 1. Start the server

**Option A: Docker Compose (recommended)**

```bash
mkdir openreader && cd openreader
curl -fsSLO https://raw.githubusercontent.com/changshengyu/openreader/main/docker-compose.yml
docker compose up -d
```

Your data is stored in `data/`, `cache/`, and `library/` in this folder, which makes backups and moves easy.

**Option B: a single `docker run` command**

```bash
docker run -d --name openreader --restart unless-stopped -p 8080:8080 -v openreader-data:/app/data -v openreader-cache:/app/cache -v openreader-library:/app/library ghcr.io/changshengyu/openreader:latest
```

Your data lives in three Docker-managed volumes, so removing and recreating the container doesn't lose anything.

The same image supports x86 (amd64) and ARM (arm64), and Docker picks the right one automatically.

### 2. Create the administrator account

Open `http://<server-ip>:8080` in a browser (or <http://localhost:8080> on the same machine) and click **创建新账号** (Create account).

- **The first account becomes the administrator.** Accounts registered later are regular users.
- Usernames need at least 5 characters and may contain only letters and digits. Passwords need at least 8 characters.

> [!WARNING]
> Anyone who can open the page can create an account. Register your own administrator account right after the first start, and read [Exposing OpenReader to the internet](#exposing-openreader-to-the-internet) before making it reachable from outside your home network.

### 3. Add books

OpenReader doesn't ship with any book sources. Pick either way to get started:

- **Online sources**: in the left sidebar, choose **书源设置 → 导入书源** (Book sources → Import sources) and select a source JSON file, or choose **远程书源** (Remote sources) to import from a link. Then type a title into the search box at the top left and open a result to start reading, or click **加入书架** (Add to shelf).
- **Local books**: choose **书架设置 → 导入书籍** (Bookshelf → Import books) and drop in TXT, EPUB, UMD, or CBZ files, up to 64 at a time.

On phones the sidebar starts hidden; tap the menu button at the top left to open it.

### 4. Use it like an app on your phone (optional)

Open the same address in your phone's browser, sign in, and add it to your home screen:

- **iPhone / iPad**: in Safari, tap Share, then Add to Home Screen.
- **Android**: in Chrome or another browser, open the menu and choose Add to Home screen or Install app.

The home-screen icon then opens OpenReader full-screen. Your phone must be able to reach the server, either on the same network or through the internet or a VPN.

## Using OpenReader

### Getting around

- **Left sidebar**: search and search settings, recently read, book sources (书源设置), bookshelf (书架设置), account (用户空间), WebDAV, and cache management.
- **Top right of the bookshelf**: **编辑** (Edit) to manage books, **刷新** (Refresh) to check for updates, **RSS** for feeds, and **书海** (Explore) to browse sources by category and ranking.

### While reading

- **Show the menu**: tap the middle of the screen. The menu holds the table of contents, source switching, bookmarks, book details, full-text search, read-aloud, auto reading, chapter caching, and settings.
- **Turn pages**: tap the top or bottom area (left or right in horizontal mode), swipe, or use the mouse wheel. Turn on **显示翻页区域** (Show tap zones) in the settings to see the zones.
- **Keyboard shortcuts** (desktop):

| Key | Action |
|---|---|
| Space, Page Down | Next page |
| Page Up | Previous page |
| ↑ / ↓ | Previous / next page (except in horizontal mode) |
| ← / → | Previous / next chapter (previous / next page in horizontal mode) |
| Home / End | Jump to the start / end of the chapter |
| Esc | Back to the bookshelf |

The **reader settings** let you change:

- Page mode: 上下滑动 (vertical paging), 左右滑动 (horizontal, phones only), 上下滚动 (scrolling), and 上下滚动2 (scrolling that hides chapters you've finished)
- Font, size, weight, line height, paragraph spacing, page width, and brightness
- Day and night themes with several backgrounds, plus your own background images and fonts (TTF, OTF, WOFF, WOFF2)
- Simplified/Traditional conversion, minimal mode, auto page-turn speed, and what tapping the screen does
- **过滤规则管理** (Filter rules): replacement rules that remove ads, watermarks, and stray characters from the text

**Read-aloud** uses the voices built into your browser and operating system. You can choose a voice, adjust the speed and pitch, and set a sleep timer. **Cache chapters** saves the next 50, 100, or all chapters on the server, so you can keep reading when a source site is unreliable.

### Syncing between devices

Sign in with the same account on each device. Your bookshelf, reading progress, bookmarks, and reader settings are stored on the server, so another device opens right where you stopped.

If a device shows outdated reader settings, choose **用户空间 → 同步用户配置** (Account → Sync settings) to pull them manually. **备份用户配置** (Back up settings) saves the current device's settings to your account.

### Book sources

- OpenReader is compatible with Legado and reader source JSON, including CSS, XPath, JSONPath, and regex rules.
- **Sources that need JavaScript (including WebJS and WebView rules) are not supported.** The import preview points them out.
- **失效书源** (Broken sources) checks which sources no longer work, and **调试书源** (Debug source) shows the search, catalog, and chapter results step by step.
- Administrators can choose **设为默认书源** (Set as default sources) for an account in user management. Other users can then restore that default set from **书源管理** (Manage sources).
- For security, sources can't reach your local network or the server itself by default. If a source runs on your NAS, see [Book sources on your local network](#book-sources-on-your-local-network).

### Local books and the library folder

- **导入书籍** (Import books) adds uploaded files straight to your bookshelf. Each file can be up to 128 MB by default.
- **浏览书仓** (Browse library) manages a library folder on the server, which suits large collections. Upload files in the page, or copy them into the folder on the server and import them from the page. With Docker Compose, the administrator's library is `library/localStore/` in the deployment folder; other users get `library/localStore/users/<username>/`.
- PDF and Markdown books imported by earlier versions remain readable. New imports support TXT, EPUB, UMD, and CBZ.

### RSS

Click **RSS** at the top right of the bookshelf to add feeds or import a feed JSON file, then browse and read articles.

### Users and permissions

- In **用户空间 → 管理用户空间** (Account → Manage users), administrators can add users, reset passwords, delete users in bulk, and decide for each user whether they may edit sources, use the library folder, and use WebDAV.
- Each user's bookshelf, progress, bookmarks, sources, and WebDAV files are kept separate.

## Backup and migration

### Automatic backups

Every day at 23:50, OpenReader saves a `backup_<date>_<time>.zip` for each user in that user's WebDAV space. It contains sources, bookshelf, groups, RSS, settings, bookmarks, reading progress, and replacement rules, **but not original local-book files**.

- The time follows the container's time zone, which is UTC unless you [set the time zone](#set-the-time-zone).
- Old backups are not deleted automatically. Remove them in **WebDAV → 文件管理** (File manager) when needed.

### Manual backups

| Method | What it contains | Use it for |
|---|---|---|
| **WebDAV → 保存备份** (Save backup) | Your account data (same as the automatic backup), without original local-book files | Routine backups |
| **WebDAV → 保存完整可移植备份** (Save portable backup) | Account data plus original local-book files and custom covers, backgrounds, and fonts | Moving an account to another OpenReader server |
| Stop the container and copy `data/`, `cache/`, and `library/` | Everything, for every user | Full-server backups and moves |

Download backup files from **WebDAV → 文件管理** (File manager). A portable backup can't be created while the account has audio folders, missing original files, or invalid custom assets.

### Migrate from Legado or reader-dev

1. Export a backup (`backup*.zip`) from the app or server you're leaving. These backups usually don't include original TXT, EPUB, or other local-book files, so keep those separately.
2. Sign in to OpenReader, open **WebDAV → 文件管理 → 上传文件** (File manager → Upload files), and upload the ZIP as is. There's no need to unzip or rename it.
3. Click **恢复** (Restore) next to the file and wait for the result.
4. Re-import your local books with **导入书籍** (Import books).
5. Check your bookshelf, sources, bookmarks, and progress before retiring the old service.

Notes:

- Data is restored into **the account you're signed in to**, and old account passwords are not carried over. If the destination account already has data, back it up first.
- Sources are skipped when the current account isn't allowed to edit sources.
- Don't copy a reader-dev database file into OpenReader; the formats are different.

### Move to another OpenReader server

- **The whole server (all users)**: stop the container and copy `data/`, `cache/`, `library/`, and `docker-compose.yml` (plus any environment changes) to the new machine, then start the same or a newer image. Retire the old machine only after you've confirmed you can sign in and open both local and online books.
- **One account**: on the old server, choose **保存完整可移植备份** (Save portable backup) and download the `portable_backup_*.zip`. On the new server, sign in to the destination account, upload the file, and click **恢复** (Restore).

### Use other WebDAV clients

OpenReader includes a WebDAV server, and each user gets a separate space:

- Address: `http://<server-ip>:8080/reader3/webdav/` (`/webdav/` also works)
- Username and password: your OpenReader account

For example, you can point the Legado app's WebDAV backup at it. Backups the app uploads then appear in the file manager, ready to restore. Reading progress can also sync between the app and OpenReader over WebDAV (the book must already be on both shelves, and books are matched by URL or by title and author). This integration is still being refined, so feedback is welcome.

## Upgrading

> [!TIP]
> Back up before upgrading. Refreshing the browser doesn't upgrade anything; the container itself has to be updated.

**Docker Compose** (run these in the folder that contains `docker-compose.yml`):

```bash
docker compose stop
tar -czf "../openreader-backup-$(date +%Y%m%d-%H%M%S).tar.gz" data cache library docker-compose.yml
docker compose pull
docker compose up -d
```

The second line writes a full backup to the parent folder. It contains every user's data, so keep it somewhere safe. On Linux, put `sudo` in front of `tar` if you get permission errors.

**docker run**: first save and download a portable backup in the web page, then run:

```bash
docker pull ghcr.io/changshengyu/openreader:latest
docker stop openreader
docker rm openreader
```

Then run the `docker run` command from [step 1](#1-start-the-server) again. The three data volumes are kept.

**Check the version**: the sidebar shows the running version next to the app name, 阅读. You can also open `http://<server-ip>:8080/api/health` and look at `version` and `commit`.

**Pinning and rolling back**: besides `latest`, every release gets a 7-character commit tag, listed on the [package page](https://github.com/changshengyu/openreader/pkgs/container/openreader). Replace `latest` in `image` with a specific tag (such as `ghcr.io/changshengyu/openreader:<commit>`) to stay on that version. To roll back, stop the container, restore all three folders from the pre-upgrade backup, and start the older tag. Never mix database files from different versions. Versions before `4c40a4a` don't use `data/instance-secret`, so after going back to one of them, devices that signed in on the newer version have to sign in again.

## Configuration

After editing `docker-compose.yml`, run `docker compose up -d` to apply the change. With `docker run`, add `-e NAME=value` options to the command, then remove the old container and run the command again.

### Change the port

Change the left-hand number in `ports`, for example to `"18080:8080"`, then open `http://<server-ip>:18080`. Leave the right-hand `8080` as it is.

### Set the time zone

```yaml
    environment:
      TZ: Asia/Shanghai
```

The time zone controls when the daily backup runs and the time shown in backup file names.

### Exposing OpenReader to the internet

The defaults are fine inside your home network. Before making OpenReader reachable from the internet:

1. **Keep the `data` folder private.** On first start OpenReader creates its own random signing key, `data/instance-secret`, and signs login sessions and resource links with it. Anyone who can read that file could forge them, so don't share it, and keep it together with the rest of `data/` when you back up or move the server. You don't need to set `OPENREADER_JWT_SECRET`. If you set it anyway, use a long random value (for example the output of `openssl rand -hex 32`, never an example from a guide): it replaces the generated key, and every device has to sign in again once.
2. **Put it behind an HTTPS reverse proxy** such as Caddy or Nginx, instead of exposing port 8080 directly.
3. **Mind open registration.** Anyone who can open the page can currently create an account. To keep strangers out, put OpenReader behind a VPN (such as Tailscale or WireGuard) or allow only trusted IP addresses. Administrators can also delete unknown accounts in user management.

Caddy obtains HTTPS certificates automatically, and the minimal configuration is three lines:

```caddyfile
reader.example.com {
    reverse_proxy 127.0.0.1:8080
}
```

<details>
<summary>Nginx example</summary>

Real-time sync needs WebSocket forwarding (`/ws/`), and uploads need a larger body-size limit:

```nginx
server {
    listen 443 ssl;
    server_name reader.example.com;
    # Add your ssl_certificate and ssl_certificate_key here

    client_max_body_size 512m;  # allow large book and backup uploads

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

Optional: set `OPENREADER_TRUSTED_PROXIES` to your reverse proxy's own IP address or network range (comma-separated) so that logs and rate limiting see visitors' real IP addresses. List only the proxy, never visitor networks. If you're unsure, leave it empty; OpenReader works fine without it.

### Book sources on your local network

For security, book sources and RSS feeds can't reach the server itself or your local network by default. If a source runs on your NAS or another machine at home, allow only its hostname or IP address:

```yaml
    environment:
      OPENREADER_SOURCE_NETWORK_ALLOWLIST: "nas.home,192.168.50.20"
```

### Network proxies

OpenReader ignores the container's `HTTP_PROXY`, `HTTPS_PROXY`, and `ALL_PROXY` variables. Use a source's own proxy setting, or route traffic at the system level instead. Fake-IP modes in tools such as Clash may require allowing the matching network range; real-IP mode is simpler and safer when it's available.

### All environment variables

Most installations never need these. Change them only for a specific need.

<details>
<summary>General</summary>

| Variable | Default | Description |
|---|---|---|
| `OPENREADER_JWT_SECRET` | Not set: a random key is generated in `data/instance-secret` | Signs login sessions and resource links. Optional; see [Exposing OpenReader to the internet](#exposing-openreader-to-the-internet) |
| `OPENREADER_TRUSTED_PROXIES` | empty | Your reverse proxy's own IP addresses or network ranges, comma-separated. Leave empty for direct access |
| `OPENREADER_SOURCE_NETWORK_ALLOWLIST` | empty | Local-network hostnames, IP addresses, or ranges that book sources may reach, comma-separated |
| `OPENREADER_CHECK_INTERVAL` | `30m` | How often the bookshelf is checked for updates |
| `OPENREADER_RATE_LIMIT_PER_MINUTE` | `6000` | Request limit per client per minute |
| `OPENREADER_ADDR` | `:8080` | Listen address |
| `OPENREADER_DATA_DIR` | `data` | Data folder |
| `OPENREADER_CACHE_DIR` | `cache` | Cache folder |
| `OPENREADER_LIBRARY_DIR` | `library` | Library folder |
| `OPENREADER_LOCAL_STORE_DIR` | `library/localStore` | Library-folder root used by 浏览书仓 (Browse library) |
| `OPENREADER_DB` | `data/openreader.db` | SQLite database path |
| `OPENREADER_PUBLIC_DIR` | `public` | Frontend files folder |
| `OPENREADER_CORS_ORIGIN` | `http://localhost:5173` | Only used in development, when the frontend and API run separately |

The Docker image already points the folders at container paths such as `/app/data`, `/app/cache`, and `/app/library`. Don't change the folder-related variables there.

</details>

<details>
<summary>File-size and network limits</summary>

| Variable | Default | What it controls |
|---|---:|---|
| `OPENREADER_SOURCE_REQUEST_TIMEOUT_SECONDS` | `15` | Timeout in seconds for one book-source or RSS request |
| `OPENREADER_MAX_SOURCE_RESPONSE_BYTES` | `16777216` (16 MiB) | Maximum size of one remote response |
| `OPENREADER_MAX_SOURCE_REDIRECTS` | `5` | Maximum redirects for one remote request |
| `OPENREADER_MAX_SOURCE_RETRIES` | `3` | Maximum attempts for a retryable remote request |
| `OPENREADER_MAX_IMPORT_BYTES` | `134217728` (128 MiB) | Maximum size of an uploaded local-book file |
| `OPENREADER_MAX_ARCHIVE_ENTRIES` | `20000` | Maximum number of files inside one book archive |
| `OPENREADER_MAX_ARCHIVE_ENTRY_BYTES` | `134217728` (128 MiB) | Maximum unpacked size of one file inside a book archive |
| `OPENREADER_MAX_ARCHIVE_EXPANDED_BYTES` | `536870912` (512 MiB) | Maximum total unpacked size of one book archive |
| `OPENREADER_MAX_PDF_PAGES` | `10000` | Maximum pages parsed from one PDF |
| `OPENREADER_MAX_PARSED_TEXT_BYTES` | `268435456` (256 MiB) | Maximum text kept while parsing a local book |
| `OPENREADER_MAX_PARSED_CHAPTERS` | `100000` | Maximum chapters produced from a local book |
| `OPENREADER_MAX_UMD_CHAPTERS` | `100000` | UMD-specific chapter limit (kept for older configurations) |
| `OPENREADER_MAX_BACKUP_RESTORE_BYTES` | `134217728` (128 MiB) | Maximum size of an uploaded regular backup ZIP |
| `OPENREADER_MAX_BACKUP_ARCHIVE_ENTRIES` | `5000` | Maximum number of files inside a regular backup |
| `OPENREADER_MAX_BACKUP_ARCHIVE_ENTRY_BYTES` | `16777216` (16 MiB) | Maximum unpacked size of one file inside a regular backup |
| `OPENREADER_MAX_BACKUP_ARCHIVE_EXPANDED_BYTES` | `134217728` (128 MiB) | Maximum total unpacked size of a regular backup |
| `OPENREADER_MAX_PORTABLE_BACKUP_BYTES` | `536870912` (512 MiB) | Maximum size of a portable backup |
| `OPENREADER_MAX_PORTABLE_ARCHIVE_ENTRIES` | `10000` | Maximum number of files inside a portable backup |
| `OPENREADER_MAX_PORTABLE_ARCHIVE_ENTRY_BYTES` | `268435456` (256 MiB) | Maximum unpacked size of one file inside a portable backup |
| `OPENREADER_MAX_PORTABLE_ARCHIVE_EXPANDED_BYTES` | `536870912` (512 MiB) | Maximum total unpacked size of a portable backup |
| `OPENREADER_MAX_CHAPTER_IMAGES` | `64` | Maximum images cached for one chapter |
| `OPENREADER_MAX_CHAPTER_IMAGE_BYTES` | `8388608` (8 MiB) | Maximum size of one chapter image |
| `OPENREADER_MAX_CHAPTER_IMAGE_TOTAL_BYTES` | `33554432` (32 MiB) | Maximum total image size for one chapter |
| `OPENREADER_CHAPTER_IMAGE_TIMEOUT_SECONDS` | `12` | Download timeout in seconds for one chapter image |
| `OPENREADER_MAX_CHAPTER_IMAGE_REDIRECTS` | `3` | Maximum redirects for one chapter image |
| `OPENREADER_MAX_COVER_IMAGE_BYTES` | `8388608` (8 MiB) | Maximum size of one cover image |
| `OPENREADER_MAX_COVER_CACHE_BYTES` | `268435456` (256 MiB) | Total cover-cache size before older covers are removed |
| `OPENREADER_COVER_IMAGE_TIMEOUT_SECONDS` | `3` | Download timeout in seconds for one cover image |
| `OPENREADER_MAX_COVER_IMAGE_REDIRECTS` | `3` | Maximum redirects for one cover image |

</details>

## FAQ

**Search finds nothing, or a source doesn't work.**
First check the source in **失效书源** (Broken sources). Sources that need JavaScript aren't supported; sources on your local network must be [allowed first](#book-sources-on-your-local-network); and the source website may be blocking your server's IP address. **调试书源** (Debug source) shows exactly which step fails.

**The sidebar says 后端未连接 (backend not connected).**
The page can't reach the server. Check that the container is running (`docker compose ps` or `docker ps`), then read the logs with `docker logs openreader`.

**I forgot my password.**
Ask an administrator to click **重置密码** (Reset password) in user management. Administrators should keep their own password somewhere safe.

**Can other people register on my OpenReader?**
Yes. To keep strangers out, see [Exposing OpenReader to the internet](#exposing-openreader-to-the-internet).

**Can I read offline?**
The page needs a connection to your OpenReader server. Chapters you cache ahead of time stay readable even when the source website is down.

**Will upgrading lose my data?**
No, as long as you keep `data/`, `cache/`, and `library/` (or the three Docker volumes). Backing up before an upgrade is still a good idea.

**Where is my data stored?**

| Folder | Contents | When backing up |
|---|---|---|
| `data/` | The database, uploaded images and fonts, and each user's WebDAV files and automatic backups | Always keep |
| `cache/` | Chapter cache, import previews, and similar files that can mostly be rebuilt | Copy it along for a full-server move |
| `library/` | Original files of imported local books, and the library folder | Always keep |

**How does this relate to reader-dev and the Legado app?**
OpenReader rebuilds the web reading experience of [reader-dev](https://github.com/changshengyu/reader-dev) (a maintained fork of [hectorqin/reader](https://github.com/hectorqin/reader)) in Go and Vue 3, and its book-source format is compatible with the [Legado (阅读)](https://github.com/gedoor/legado) app. It can import backups from both, but it can't use a reader-dev database directly.

## Development

OpenReader is built with Go (Gin, GORM, SQLite) and Vue 3 (Vite, Pinia, Element Plus), and both parts ship in the same Docker image. To run it from source you need Go 1.26.10 or newer (with CGO enabled for SQLite) and Node.js 22.

```bash
git clone https://github.com/changshengyu/openreader.git
cd openreader
```

Start the backend (<http://localhost:8080>):

```bash
cd backend
go run .
```

From the repository root, open another terminal and start the frontend (<http://localhost:5173>):

```bash
cd frontend
npm ci
npm run dev
```

Run the tests:

```bash
(cd backend && go test ./...)
(cd frontend && npm test && npm run build)
```

GitHub Actions tests and publishes the Docker image whenever `main` is updated. More developer documentation lives in [`docs/`](docs/), and the browser regression checks are described in [`docs/regression-smoke.md`](docs/regression-smoke.md).

## Acknowledgments

OpenReader's features and interactions follow [changshengyu/reader-dev](https://github.com/changshengyu/reader-dev) and the original [Reader](https://github.com/hectorqin/reader) project, and its book-source ecosystem comes from the [Legado (阅读)](https://github.com/gedoor/legado) community. Thanks to all upstream authors and contributors.

## License

[GPL v3](LICENSE)
