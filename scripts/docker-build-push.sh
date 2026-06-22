#!/usr/bin/env sh
set -eu

IMAGE="${IMAGE:-ghcr.io/changshengyu/openreader}"
TAG="${TAG:-$(git rev-parse --short HEAD)}"
VERSION="${VERSION:-$TAG}"
VCS_REF="${VCS_REF:-$(git rev-parse HEAD)}"
BUILD_DATE="${BUILD_DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
GOPROXY="${GOPROXY:-https://proxy.golang.org,direct}"
GOSUMDB="${GOSUMDB:-sum.golang.org}"
PLATFORMS="${PLATFORMS:-}"
PUSH="${PUSH:-1}"
RELEASE="${RELEASE:-0}"

if [ "$PUSH" = "1" ]; then
  OUTPUT_FLAG="--push"
  if [ -z "$PLATFORMS" ]; then
    if [ "$RELEASE" = "1" ]; then
      PLATFORMS="linux/amd64,linux/arm64"
    else
      PLATFORMS="linux/arm64"
    fi
  fi
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
  --build-arg "GOPROXY=$GOPROXY" \
  --build-arg "GOSUMDB=$GOSUMDB" \
  $OUTPUT_FLAG \
  .
