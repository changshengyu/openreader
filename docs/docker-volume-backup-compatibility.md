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

The historical fixture covers old-volume path/security, relative-cache migration and transaction
boundaries for all four supported local archive formats. It does not replace cross-user Docker or
portable archive-backup contracts.

This is not a substitute for full restore validation. It is the minimum release gate for Docker volume and backup regressions.
