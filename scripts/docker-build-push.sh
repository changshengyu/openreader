#!/usr/bin/env sh
set -eu

IMAGE="${IMAGE:-ghcr.io/changshengyu/openreader}"
TAG="${TAG:-$(git rev-parse --short HEAD)}"
VERSION="${VERSION:-$TAG}"
VCS_REF="${VCS_REF:-$(git rev-parse HEAD)}"
BUILD_DATE="${BUILD_DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
PLATFORMS="${PLATFORMS:-}"
PUSH="${PUSH:-1}"

if [ "$PUSH" = "1" ]; then
  OUTPUT_FLAG="--push"
  PLATFORMS="${PLATFORMS:-linux/amd64,linux/arm64}"
else
  OUTPUT_FLAG="--load"
  PLATFORMS="${PLATFORMS:-linux/$(go env GOARCH)}"
fi

docker buildx build \
  --platform "$PLATFORMS" \
  -t "$IMAGE:latest" \
  -t "$IMAGE:$TAG" \
  --build-arg "VERSION=$VERSION" \
  --build-arg "VCS_REF=$VCS_REF" \
  --build-arg "BUILD_DATE=$BUILD_DATE" \
  $OUTPUT_FLAG \
  .
