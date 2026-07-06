---
name: openreader-regression
description: Regression validation workflow for OpenReader changes. Use before committing, releasing Docker, or claiming a refactor module is complete.
---

# OpenReader Regression

Use this skill before handoff, commit, or release.

## Default gate

```bash
cd backend && go test ./...
cd frontend && npm test
cd frontend && npm run build
```

## UI gate

For frontend behavior, run a real browser check over the affected flow. Minimum reader checks:

- desktop `1440x900`;
- mobile `390x844`;
- mobile `360x800`.

Reader-specific checks:

- mobile tool layer default visible;
- panel open does not hide tool layer;
- center tap toggles tool layer only when intended;
- panel clicks do not pass through;
- left/right reader whitespace symmetric within 1px;
- native continuous scroll and click paging both work as configured.

## Release gate

Before Docker publish:

```bash
PUSH=0 ./scripts/docker-build-push.sh
scripts/docker-volume-backup-smoke.sh
```

For final GHCR publish:

```bash
RELEASE=1 ./scripts/docker-build-push.sh
```

Record every skipped check with a concrete reason.
