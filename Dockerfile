# --- Web UI Build Stage
FROM node:20-alpine AS web-build

WORKDIR /web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# --- Go Build stage
# Use Debian-based image for better compatibility with distroless runtime
FROM golang:1.25-bookworm AS build

# Install build dependencies for CGO and SQLite
RUN apt-get update && apt-get install -y \
    gcc \
    libc6-dev \
    libsqlite3-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

# Copy built web UI into Go source tree for embedding
COPY --from=web-build /web/dist ./cmd/nekzus/webdist

RUN go mod tidy

# Build with CGO enabled (required for SQLite)
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=1 go build -o /out/nekzus ./cmd/nekzus

# --- Runtime stage
# Using base-debian12 with libc for SQLite support
FROM gcr.io/distroless/base-debian12:nonroot

WORKDIR /app
COPY --from=build /out/nekzus /app/nekzus

# Run as root to access Docker socket
# TODO: In production, use Docker socket proxy or add user to docker group
USER root:root

EXPOSE 8080
ENTRYPOINT ["/app/nekzus"]
