---
name: openreader-docker-release
description: Local Docker release workflow for OpenReader. Use before building or publishing GHCR images, tagging releases, validating image metadata, or reporting Docker release progress.
---

# OpenReader Docker Release

Use this skill before publishing Docker images.

## Release policy

- Build locally. Do not use cloud Docker builds.
- Publish only after a complete module passes backend, frontend, browser, and Docker gates appropriate to the change.
- Preserve upgrade compatibility for mounted `data/`, `cache/`, and `library/`.

## Standard commands

Development image:

```bash
./scripts/docker-build-push.sh
```

Release image:

```bash
RELEASE=1 ./scripts/docker-build-push.sh
```

Inspect:

```bash
docker buildx imagetools inspect ghcr.io/changshengyu/openreader:latest
```

## Required release report

Include:

- commit SHA and image tags;
- digest;
- completed items;
- allowed differences from upstream;
- unfinished items;
- validation summary;
- Docker/volume/backup compatibility result.
