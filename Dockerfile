# Pin official base image digests so local and CI builds remain reproducible when a
# registry's mutable-tag metadata endpoint is temporarily unavailable.
# Frontend assets are architecture-independent. Build them on the native
# runner platform so multi-platform releases never execute Node under QEMU.
FROM --platform=$BUILDPLATFORM node:22-alpine3.23@sha256:baf676f7d0e552f3231945c2f979055ca121bce128c152f7a34e6bd1728b1c5a AS frontend-builder
WORKDIR /src/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM golang:1.26.10-alpine3.23@sha256:107301490755aa2ea1b0b43788b6e62cff2faae45a538028b193becc06aaffba AS backend-builder
RUN apk add --no-cache build-base
WORKDIR /src/backend
ARG TARGETOS=linux
ARG TARGETARCH
COPY backend/go.mod backend/go.sum* ./
# `go_vendor` is generated before the build by the release workflow or local
# script. This keeps dependency resolution outside BuildKit while retaining the
# reproducible module graph from go.mod/go.sum.
COPY --from=go_vendor / /src/backend/vendor
ARG BUILD_DATE=unknown
ARG VCS_REF=unknown
ARG VERSION=dev
COPY backend/ ./
RUN CGO_ENABLED=1 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-$(go env GOARCH)} go build -mod=vendor -trimpath -ldflags="-s -w -X openreader/backend/api.Version=${VERSION} -X openreader/backend/api.Commit=${VCS_REF} -X openreader/backend/api.BuildDate=${BUILD_DATE}" -o /out/openreader .

FROM alpine:3.23.6@sha256:85fe1e81d6758c208f3e1eed4338a1997e19d4be002d4dd32d3100c9a8c010a0
# The pinned official 3.23.6 base still ships zlib 1.3.2-r0 and musl 1.2.5-r23. Install
# only the exact v3.23 main maintenance fixes at build time: apk verifies the signed
# repository index, the release OS gate pins the package identities, and nothing
# upgrades at container start. Kept before the ARGs so the validated layer is reused
# by the published build.
RUN apk add --no-cache 'zlib=1.4-r0' 'musl=1.2.5-r24' 'musl-utils=1.2.5-r24'
ARG BUILD_DATE=unknown
ARG VCS_REF=unknown
LABEL org.opencontainers.image.title="OpenReader" \
      org.opencontainers.image.description="OpenReader web reader service" \
      org.opencontainers.image.source="https://github.com/changshengyu/openreader" \
      org.opencontainers.image.created="${BUILD_DATE}" \
      org.opencontainers.image.revision="${VCS_REF}"
WORKDIR /app
ENV OPENREADER_ADDR=:8080 \
    OPENREADER_DATA_DIR=/app/data \
    OPENREADER_CACHE_DIR=/app/cache \
    OPENREADER_LIBRARY_DIR=/app/library \
    OPENREADER_LOCAL_STORE_DIR=/app/library/localStore \
    OPENREADER_DB=/app/data/openreader.db \
    OPENREADER_PUBLIC_DIR=/app/public \
    GIN_MODE=release
# The runtime base's own ca-certificates-bundle provides the CA bundle (copying
# the builder's would overwrite an APK-owned file whenever the builder's Alpine
# point release differs); time/tzdata is embedded in the binary, so the zlib
# fix above is the runtime stage's only package install.
COPY --from=backend-builder /out/openreader /app/openreader
COPY --from=frontend-builder /src/frontend/dist /app/public
RUN mkdir -p /app/data /app/cache /app/library
EXPOSE 8080
CMD ["/app/openreader"]
