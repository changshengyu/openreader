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
SQLite fixture (with newer EPUB columns removed), a local TXT archive with no
derived content, and a separately mounted `/retired-host` directory containing
readable stale absolute source/cache decoys. The container must:

- migrate the old SQLite rows without losing progress or bookmarks;
- recover chapter text from `library/`, not either retired-host decoy;
- refresh without changing the archive SHA-256;
- trigger and restore a logical backup without changing the mounted archive;
- remain readable after a full container restart.

The historical fixture intentionally covers the old-volume path/security and
transaction boundary. EPUB, UMD and CBZ old-volume format fixtures remain
separate P1-E4 work and must not be claimed by this TXT smoke.

This is not a substitute for full restore validation. It is the minimum release gate for Docker volume and backup regressions.
