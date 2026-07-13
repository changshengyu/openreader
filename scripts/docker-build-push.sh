#!/usr/bin/env sh
set -eu

IMAGE="${IMAGE:-ghcr.io/changshengyu/openreader}"
TAG="${TAG:-$(git rev-parse --short HEAD)}"
VERSION="${VERSION:-$TAG}"
VCS_REF="${VCS_REF:-$(git rev-parse HEAD)}"
BUILD_DATE="${BUILD_DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
PLATFORMS="${PLATFORMS:-}"
PUSH="${PUSH:-1}"
RELEASE="${RELEASE:-0}"
# Docker Desktop/OrbStack's buildx --push can report a successful local build
# without leaving a remote GHCR manifest. Formal multi-arch releases therefore
# default to the host-network OCI publisher. Set HOST_OCI_PUSH=0 to opt back
# into buildx --push, or HOST_OCI_PUSH=1 to use this path for a non-release tag.
HOST_OCI_PUSH="${HOST_OCI_PUSH:-}"
if [ -z "$HOST_OCI_PUSH" ]; then
  if [ "$RELEASE" = "1" ]; then
    HOST_OCI_PUSH=1
  else
    HOST_OCI_PUSH=0
  fi
fi
case "$HOST_OCI_PUSH" in
  0|1) ;;
  *)
    echo "HOST_OCI_PUSH must be 0 or 1" >&2
    exit 2
    ;;
esac
OCI_ARCHIVE="${OCI_ARCHIVE:-}"
GO_VENDOR_DIR="${GO_VENDOR_DIR:-}"
BUILD_PROGRESS="${BUILD_PROGRESS:-auto}"

if [ -z "$GO_VENDOR_DIR" ]; then
  GO_VENDOR_DIR="$(mktemp -d -t openreader-go-vendor)"
  REMOVE_GO_VENDOR_DIR=1
else
  REMOVE_GO_VENDOR_DIR=0
fi

cleanup() {
  if [ "${REMOVE_GO_VENDOR_DIR:-0}" = "1" ]; then
    rm -rf "$GO_VENDOR_DIR"
  fi
}
trap cleanup EXIT INT TERM

(cd backend && go mod vendor -o "$GO_VENDOR_DIR")

if [ "$PUSH" = "1" ]; then
  if [ "$HOST_OCI_PUSH" = "1" ]; then
    if [ -z "$OCI_ARCHIVE" ]; then
      OCI_ARCHIVE="$(mktemp -t openreader-oci)"
      REMOVE_OCI_ARCHIVE=1
    else
      REMOVE_OCI_ARCHIVE=0
    fi
    OUTPUT_FLAG="--output type=oci,dest=$OCI_ARCHIVE"
  else
    OUTPUT_FLAG="--push"
  fi
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
  --progress "$BUILD_PROGRESS" \
  --platform "$PLATFORMS" \
  --build-context "go_vendor=$GO_VENDOR_DIR" \
  -t "$IMAGE:latest" \
  -t "$IMAGE:$TAG" \
  --build-arg "VERSION=$VERSION" \
  --build-arg "VCS_REF=$VCS_REF" \
  --build-arg "BUILD_DATE=$BUILD_DATE" \
  $OUTPUT_FLAG \
  .

if [ "$PUSH" = "1" ] && [ "$HOST_OCI_PUSH" = "1" ]; then
  if [ ! -s "$OCI_ARCHIVE" ]; then
    echo "Docker OCI archive is empty; refusing to publish" >&2
    exit 1
  fi
  OCI_CLEANUP_FLAG=""
  if [ "${REMOVE_OCI_ARCHIVE:-0}" = "1" ]; then
    OCI_CLEANUP_FLAG="--remove-archive"
  fi
  node ./scripts/docker-oci-push.mjs --archive "$OCI_ARCHIVE" --image "$IMAGE" --tag "$TAG" --tag latest $OCI_CLEANUP_FLAG
fi
