# syntax=docker/dockerfile:1.7

# Builder stage
FROM golang:1.27.0-trixie AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY VERSION ./
COPY cmd ./cmd
COPY internal ./internal

ARG TARGETOS=linux
ARG TARGETARCH
RUN --mount=type=bind,source=.git,target=/app/.git,readonly \
    VERSION="$(tr -d '\r\n' < VERSION)" && \
    COMMIT="$(git rev-parse --verify 'HEAD^{commit}')" && \
    test -n "$VERSION" && \
    test -n "$COMMIT" && \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -trimpath \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
    -o golfs ./cmd/golfs

# Production stage
FROM debian:trixie-slim AS production

LABEL org.opencontainers.image.authors="Sayak Mukhopadhyay" \
      org.opencontainers.image.title="GOLFS" \
      org.opencontainers.image.description="Stateless Git LFS server for GitHub.com repositories and private S3-compatible object storage" \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.url="https://github.com/kode-blox/golfs"

RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/* && \
    update-ca-certificates

RUN groupadd --gid 1000 app && \
    useradd --uid 1000 --gid app --shell /usr/sbin/nologin --create-home app

WORKDIR /app

COPY --chown=app:app --from=builder /app/golfs ./golfs

USER app
EXPOSE 8080 9090

CMD ["./golfs"]
