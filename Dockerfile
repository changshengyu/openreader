FROM node:22-alpine AS frontend-builder
WORKDIR /src/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM golang:1.24-alpine AS backend-builder
RUN apk add --no-cache build-base
WORKDIR /src/backend
ARG TARGETOS=linux
ARG TARGETARCH
COPY backend/go.mod backend/go.sum* ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=1 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-$(go env GOARCH)} go build -trimpath -ldflags="-s -w" -o /out/openreader .

FROM alpine:3.20
ARG BUILD_DATE=unknown
ARG VCS_REF=unknown
LABEL org.opencontainers.image.title="OpenReader" \
      org.opencontainers.image.description="OpenReader web reader service" \
      org.opencontainers.image.source="https://github.com/changshengyu/openreader" \
      org.opencontainers.image.created="${BUILD_DATE}" \
      org.opencontainers.image.revision="${VCS_REF}"
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
ENV OPENREADER_ADDR=:8080 \
    OPENREADER_DATA_DIR=/app/data \
    OPENREADER_CACHE_DIR=/app/cache \
    OPENREADER_LIBRARY_DIR=/app/library \
    OPENREADER_LOCAL_STORE_DIR=/app/library/localStore \
    OPENREADER_DB=/app/data/openreader.db \
    OPENREADER_PUBLIC_DIR=/app/public \
    GIN_MODE=release
COPY --from=backend-builder /out/openreader /app/openreader
COPY --from=frontend-builder /src/frontend/dist /app/public
RUN mkdir -p /app/data /app/cache /app/library
EXPOSE 8080
CMD ["/app/openreader"]
