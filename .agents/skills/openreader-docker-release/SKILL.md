---
name: openreader-docker-release
description: Docker release workflow for OpenReader. Use before building or publishing GHCR images, tagging releases, validating image metadata, or reporting Docker release progress.
---

# OpenReader Docker Release

Use this skill before publishing Docker images.

## Release policy

- Use the repository GitHub Actions workflow as the trusted release path. It must pass backend, frontend, Compose, fresh-volume, historical-volume, and backup gates before pushing the multi-platform index.
- Keep the local build script as a development and recovery path. Consumers should pull the published image instead of compiling it locally.
- Publish after a coherent validation slice passes backend, frontend, browser, and Docker gates appropriate to the change. A complete module boundary is preferred but not required when the user wants intermediate verification.
- Push Git commits to GitHub before or together with the Docker publish so the image can be traced to a remote commit.
- Preserve upgrade compatibility for mounted `data/`, `cache/`, and `library/`.

## Standard commands

Optional local development image:

```bash
./scripts/docker-build-push.sh
```

Manual release fallback:

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
