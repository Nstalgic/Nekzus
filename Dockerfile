# --- Web UI Build Stage
FROM node:20-alpine AS web-build

WORKDIR /web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# --- Go Build stage
# Run on the build platform natively, cross-compile for target arch
ARG BUILDPLATFORM
FROM --platform=${BUILDPLATFORM:-linux/amd64} golang:1.25-bookworm AS build

ARG TARGETPLATFORM
ARG TARGETARCH

# Install build dependencies for CGO and SQLite (native + cross-compile)
RUN apt-get update && apt-get install -y \
    gcc \
    libc6-dev \
    libsqlite3-dev \
    && if [ "$TARGETARCH" = "arm64" ] && [ "$(dpkg --print-architecture)" != "arm64" ]; then \
        dpkg --add-architecture arm64 && \
        apt-get update && \
        apt-get install -y \
            gcc-aarch64-linux-gnu \
            libc6-dev-arm64-cross \
            libsqlite3-dev:arm64; \
    fi \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

# Copy built web UI into Go source tree for embedding
COPY --from=web-build /web/dist ./cmd/nekzus/webdist

RUN go mod tidy

# Build with CGO enabled (required for SQLite)
# Use cross-compiler for arm64 to avoid slow QEMU emulation
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    if [ "$TARGETARCH" = "arm64" ] && [ "$(dpkg --print-architecture)" != "arm64" ]; then \
        CC=aarch64-linux-gnu-gcc CGO_ENABLED=1 GOOS=linux GOARCH=arm64 \
        go build -o /out/nekzus ./cmd/nekzus; \
    else \
        CGO_ENABLED=1 go build -o /out/nekzus ./cmd/nekzus; \
    fi

# --- Runtime stage
# Using base-debian12 with libc for SQLite support
FROM gcr.io/distroless/base-debian12:nonroot

WORKDIR /app
COPY --from=build /out/nekzus /app/nekzus
COPY --from=build /src/LICENSE /app/LICENSE

# Run as root to access Docker socket
# TODO: In production, use Docker socket proxy or add user to docker group
USER root:root

EXPOSE 8080
ENTRYPOINT ["/app/nekzus"]
