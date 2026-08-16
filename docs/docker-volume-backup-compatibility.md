# Docker, Volume, and Backup Compatibility

OpenReader releases must preserve existing mounted data:

- `data/`: SQLite database, uploads, backup files.
- `cache/`: chapter/content cache.
- `library/`: imported originals and local store content.

## Local compatibility smoke

Run after a local image build:

```bash
PUSH=0 ./scripts/docker-build-push.sh
scripts/docker-volume-backup-smoke.sh
```

Optional overrides:

```bash
IMAGE=ghcr.io/changshengyu/openreader:latest scripts/docker-volume-backup-smoke.sh
PORT=18080 scripts/docker-volume-backup-smoke.sh
KEEP_OPENREADER_SMOKE=1 scripts/docker-volume-backup-smoke.sh
HISTORICAL_VOLUME=1 IMAGE=ghcr.io/changshengyu/openreader:latest scripts/docker-volume-backup-smoke.sh
```

## What the smoke checks

- Container starts with explicit mounted `data`, `cache`, and `library` directories.
- `/api/health` responds.
- User registration/login works with the mounted database.
- Backup trigger creates a downloadable/listed backup entry.
- Container can be stopped and restarted against the same mounted directories.
- Health and login still work after restart.

When `HISTORICAL_VOLUME=1` is set, the script additionally builds an old on-disk
SQLite fixture (with newer EPUB columns removed), a relative-path TXT archive plus stale-absolute
EPUB/standard reader-dev UMD/CBZ archives with no derived content, and a separately mounted
`/retired-host` directory containing readable source/cache decoys. It also creates one book whose
legal `cache/legacy-cache/chapter.txt` differs from its archive fallback. The container must:

- migrate the old SQLite rows without losing progress or bookmarks;
- recover each format from `library/`, not either retired-host decoy (including a CBZ resource read);
- refresh each TXT/EPUB/UMD/CBZ format archive without changing its SHA-256;
- copy the legal relative cache to its book's `content/legacy-cache/chapter.txt`, persist that
  relative cache field, remove the old cache only after migration, and keep it readable through
  backup restore and restart;
- trigger and restore a logical backup without changing the mounted archive;
- remain readable after a full container restart.

The fixture also preloads a second user with an independent local archive. Both users must see only
their own shelf entries; cross-user chapter reads and local refreshes return 404. The owner backup
restore/restart must leave the second user's archive SHA-256, chapter cache path and readability
unchanged.

The historical fixture covers old-volume path/security, relative-cache migration, existing-user
isolation and transaction boundaries for all four supported local archive formats. It also starts a
second, fresh mounted `data/cache/library` tuple, exports the owner's portable package, restores it
through the ordinary upload endpoint, and proves portable export → transfer → restore →
read/refresh → restart without changing either volume's original archive hashes. The destination
contains only the authenticated owner’s recovered local books; it never receives the second user's
archive. See
[`portable-local-archive-backup-p1e4-contract.md`](compat/portable-local-archive-backup-p1e4-contract.md).

This is not a substitute for full restore validation. It is the minimum release gate for Docker volume and backup regressions.

## 2026-08-16 LocalStore/WebDAV boundary release

Release `65a9870` ran the fresh and `HISTORICAL_VOLUME=1` variants sequentially against the locally built candidate.
Both passed, including TXT/EPUB/UMD/CBZ/relative-cache, owner isolation, logical restore, portable v1/v2 appearance
assets, cross-user restore and restart. The candidate also passed the LocalStore and WebDAV declared/chunked HTTP,
symlink/FIFO, token-only and private restore-snapshot probes against mounted host directories.

The image was built locally for `linux/amd64` and `linux/arm64`, then published as
`ghcr.io/changshengyu/openreader:65a9870` and `latest`. Both tags resolve to OCI index
`sha256:255c81b43dbb7f49c707d6c609b920aa183b730401ad1c1ca32157eb0a945c71`; a GHCR-pulled arm64 container
reported commit `65a987049d6a9bff7feeb2618f7257620cd896a9` from `/api/health`.

## 2026-08-16 Direct local import boundary release

Release `429444a` reran the fresh and `HISTORICAL_VOLUME=1` variants sequentially against the locally built
candidate. Both passed, including TXT/EPUB/UMD/CBZ/relative-cache, owner isolation, logical restore, portable v1/v2
assets, cross-user restore and restart. The same candidate passed direct declared/chunked multipart admission,
authentication priority, strict field shape, disk-backed temporary-file cleanup, token-only aliases and 1440x900,
390x844, 360x800 single/batch/sequential browser flows.

The image was built locally for `linux/amd64` and `linux/arm64`, then published as
`ghcr.io/changshengyu/openreader:429444a` and `latest`. Both tags resolve to OCI index
`sha256:41f430a5fbf944b9a1dcf25aec6c9f6e92a11a3ff75e395d1a73120da5a6f4d5`; a GHCR-pulled arm64 container
reported commit `429444a83ddbe774070c8832ec9d33390037852f` from `/api/health`.

## 2026-08-16 Remote-work request boundary release

Release `6157466` passed the fresh portable-v1/v2-assets, cross-user and restart gates against the locally built
arm64 candidate. The first ordinary `HISTORICAL_VOLUME=1` run encountered one transient HTTP 404 after fixture
creation; an immediate traced rerun against the same image passed TXT/EPUB/UMD/CBZ, relative-cache migration,
owner isolation and historical backup/restore. The failure was not reproducible and is retained here rather than
being hidden by the successful rerun.

The candidate also passed full Go/race/vet, frontend 737/737, production build, the three-viewport real-Go
remote-work browser contract and the existing four-viewport source-debug contract. It was built locally for
`linux/amd64` and `linux/arm64`, then published as `ghcr.io/changshengyu/openreader:6157466` and `latest`. Both tags
resolve to OCI index `sha256:1e890a60a1b75879dd99074b1da13b17f91bbd4173e945b92cb8cec0fe8001b6`; platform
manifests are `sha256:02d2055bec076f2590e2a952c9dffad84c55c959965ea1f3dd212da5ef9ff424` for amd64 and
`sha256:84f17fca80e57e00bdd833c00d300b3cbd31dbaf706dd3ae20d475812037ff53` for arm64. Both image labels report
revision `6157466687c15d2ce48007443992523dd6a26834`.

## 2026-08-16 BookSource local import multipart release

Release `3f3c9c8` passed the fresh portable-v1/v2-assets, cross-user and restart gates against the locally built
arm64 candidate. The first ordinary `HISTORICAL_VOLUME=1` run encountered one transient HTTP 404 after fixture
creation; an immediate traced rerun against the same image passed TXT/EPUB/UMD/CBZ, relative-cache migration,
owner isolation and historical backup/restore. The failure was not reproducible and is retained here rather than
being hidden by the successful rerun. The separate source-ownership gate also passed legacy migration, COW,
administrator/private roots, logical/portable restore and restart.

The candidate passed full Go/race/vet, frontend 738/738, production build and the 1440x900, 390x844, 360x800
real-Go BookSource import browser contract. Direct API coverage included authentication priority, declared/chunked
envelope overflow, exact 16 MiB file and +1 boundaries, strict part shape, disk-backed temporary-file cleanup and
zero mutation on rejection.

The image was built locally for `linux/amd64` and `linux/arm64`, then published as
`ghcr.io/changshengyu/openreader:3f3c9c8` and `latest`. Both tags resolve to OCI index
`sha256:62ee55ffab7859aef4334f8fb8dd31520953521da494edd5f37cc56741731070`; platform manifests are
`sha256:a47a179afdc0356a84ac808148c0a930a40ac80856a5ecf5be3f267b3037036c` for amd64 and
`sha256:8298c35b5d3d43000a68a52fbd612a48c5e079a1ba9fbaf80e4677b67081e339` for arm64. Both image labels report
revision `3f3c9c8461e60a12dd0ba08ce4a4f95860dbf319`; a forced GHCR pull returned that revision from `/api/health`.
