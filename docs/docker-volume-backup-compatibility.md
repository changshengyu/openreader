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
```

## What the smoke checks

- Container starts with explicit mounted `data`, `cache`, and `library` directories.
- `/api/health` responds.
- User registration/login works with the mounted database.
- Backup trigger creates a downloadable/listed backup entry.
- Container can be stopped and restarted against the same mounted directories.
- Health and login still work after restart.

This is not a substitute for full restore validation. It is the minimum release gate for Docker volume and backup regressions.
