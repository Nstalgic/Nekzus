# Nekzus

[![License: GPL v3](https://img.shields.io/badge/License-GPLv3-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![Docker](https://img.shields.io/badge/Docker-Ready-2496ED?logo=docker&logoColor=white)](https://hub.docker.com/r/nstalgic/nekzus)

A secure API gateway and reverse proxy for local network services with auto-discovery, JWT authentication, WebSocket support, and a modern web dashboard.

**[Website](https://nekzus.io)** | **[Documentation](https://docs.nekzus.io)**

## How It Works

Nekzus sits between your clients (browsers, mobile apps) and your backend services. It automatically discovers services running in Docker and Kubernetes, then proposes them for approval before routing traffic. mDNS/Bonjour discovery is scaffolded and planned for a future release. All requests pass through the gateway where they are authenticated (JWT or API key), proxied to the correct backend, and monitored for health. The web dashboard gives you full visibility and control over routes, devices, containers, and certificates.

```
Clients ──▶ Nekzus Gateway ──▶ Backend Services
              │                   ├── Docker containers
              │                   ├── Kubernetes pods
              ├── Auth (JWT)      └── mDNS services
              ├── Health checks
              ├── Metrics
              └── Web Dashboard
```

## Try It Out

The fastest way to see Nekzus in action is the demo environment:

```bash
# Clone the repo
git clone https://github.com/Nstalgic/Nekzus.git
cd Nekzus

# Start with test services
make demo

# View logs
make demo-logs

# Stop
make demo-down
```

This spins up Nekzus alongside example services so you can explore the dashboard and API immediately.

## Features

### Core Gateway
- **Reverse Proxy** - HTTP/HTTPS with caching, path rewriting, and header management
- **WebSocket Proxy** - Full RFC 6455 support with bidirectional communication
- **SSE Streaming** - Server-Sent Events with immediate flush support
- **Health Monitoring** - Per-service health checks with configurable thresholds

### Service Discovery
- **Docker** - Auto-discover containers via labels with multi-network support
- **Kubernetes** - Pod/service discovery with Istio, Helm chart recognition
- **mDNS/Bonjour** - Network service discovery (planned — scaffolded but not yet functional)
- **Proposal System** - Review and approve discovered services before routing

### Authentication & Security
- **JWT Authentication** - Device-specific tokens with scope-based access control
- **QR Code Pairing** - Mobile device onboarding with TLS certificate pinning
- **API Keys** - Webhook and external integration support
- **IP Allowlist** - Conditional auth bypass for local network
- **TLS** - Auto-generated or imported certificates, TLS 1.2+ enforced

### Operations
- **Toolbox** - One-click Docker Compose service deployment
- **Backup & Restore** - Scheduled database snapshots with selective restoration
- **Container Management** - Start, stop, restart, inspect Docker containers
- **Script Execution** - Run shell/Python scripts with scheduling and workflows
- **Federation** - P2P clustering with gossip protocol and catalog sync

### Observability
- **Prometheus Metrics** - HTTP, proxy, auth, and discovery metrics
- **Real-time Events** - WebSocket streaming for discovery, health, and activity
- **Audit Logs** - Track admin actions and system events
- **Notifications** - Push notifications to paired devices

### Web Dashboard
- **15 Themes** - Default, Obsidian Dark, Nord Frost, Carbon Neutral, Classic Green, Cyan, Amber, Gruvbox, Gruvbox Light, Tokyo Night, Tokyo Night Storm, Catppuccin Mocha, Pipboy, Pipboy Green, Retro
- **Full Management** - Routes, devices, discovery, containers, certificates, backups
- **Webhook Tester** - Test webhook endpoints with request/response inspection
- **Responsive** - Desktop, tablet, and mobile support

### Mobile App (PRTL)
PRTL is a companion mobile app for managing your Nekzus instance from iOS and Android.
- **App & Service Management** - Browse, manage, and launch apps from your Nekzus catalog
- **Real-time Monitoring** - Container health, service controls, and log viewing via WebSocket
- **Push Notifications** - Alerts for proposals, discoveries, and webhook events
- **QR Code Pairing** - 2-step device verification with certificate pinning
- **Biometric Auth** - Face ID, Touch ID, and fingerprint support
- **10+ Themes** - Cyberpunk, Catppuccin, Retro Futurism, Neubrutalism, and more with 13 alternate app icons
- **Offline Support** - Background sync for uninterrupted access

## Quick Start

### Docker (Recommended)

```bash
docker run -d --name nekzus \
  -p 8080:8080 \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  -v nekzus-data:/app/data \
  nstalgic/nekzus:latest
```

Access the dashboard at `http://localhost:8080`

### Docker Compose (Production)

The repository includes a [`docker-compose.yml`](docker-compose.yml) with a Caddy reverse proxy for TLS termination.

```bash
# Generate secrets
export NEKZUS_JWT_SECRET=$(openssl rand -base64 32)
export NEKZUS_BOOTSTRAP_TOKEN=$(openssl rand -base64 24)

# Start with TLS via Caddy
docker compose up -d

# Access dashboard
open https://localhost:8443
```

### Local Development

```bash
# Build web UI and Go binary
make build-all

# Run without TLS (dev only)
make run-insecure

# Web UI with hot reload
make dev-web
```

## Configuration

Configuration via YAML file or environment variables.

### Key Environment Variables

| Variable | Description |
|----------|-------------|
| `NEKZUS_ADDR` | Listen address (default: `:8443`) |
| `NEKZUS_BASE_URL` | Public URL for mobile clients |
| `NEKZUS_JWT_SECRET` | JWT signing secret (min 32 chars) |
| `NEKZUS_BOOTSTRAP_TOKEN` | Initial pairing token |
| `NEKZUS_TLS_CERT` | TLS certificate path |
| `NEKZUS_TLS_KEY` | TLS private key path |
| `NEKZUS_DATABASE_PATH` | SQLite database location |

### Config File Sections

```yaml
server:          # Address, TLS, base URL
auth:            # JWT settings, default scopes
storage:         # Database path
discovery:       # Docker, Kubernetes, mDNS settings
health_checks:   # Service monitoring
metrics:         # Prometheus endpoint
toolbox:         # One-click service deployment
notifications:   # Push notification settings
backup:          # Scheduled backups
federation:      # P2P clustering
scripts:         # Script execution
routes:          # Static route definitions
apps:            # Application catalog
```

See [`configs/config.example.yaml`](configs/config.example.yaml) for full reference.

## API Overview

All `/api/v1/*` endpoints require a valid JWT token unless otherwise noted. The health and metrics endpoints are public.

### Health & Metrics (Public)
- `GET /healthz` - Health check
- `GET /metrics` - Prometheus metrics

### Authentication
- `POST /api/v1/auth/login` - Web login
- `POST /api/v1/auth/pair` - Device pairing
- `GET /api/v1/auth/qr` - QR code for mobile pairing

### Discovery
- `GET /api/v1/discovery/proposals` - List discovered services
- `POST /api/v1/discovery/proposals/{id}/approve` - Approve service

### Management
- `GET /api/v1/apps` - List applications
- `GET /api/v1/routes` - List routes
- `GET /api/v1/devices` - List paired devices
- `GET /api/v1/containers` - List Docker containers

### Real-time
- `WS /api/v1/ws` - WebSocket for events and notifications

### Proxy
- `/apps/*` - Reverse proxy to registered services

## Service Discovery

### Docker Labels

```yaml
labels:
  nekzus.enable: "true"
  nekzus.app.name: "My App"
  nekzus.app.id: "myapp"
  nekzus.route.path: "/apps/myapp/"
```

### Kubernetes

```yaml
discovery:
  kubernetes:
    enabled: true
    namespaces: ["default", "production"]
    poll_interval: "10s"
```

Features: Istio detection, namespace label inheritance, Helm chart recognition, ingress discovery.

### mDNS (Planned)

> **Note:** mDNS discovery is scaffolded but not yet functional. The configuration is accepted but no services will be discovered until a library such as `github.com/hashicorp/mdns` or `github.com/grandcat/zeroconf` is integrated.

```yaml
discovery:
  mdns:
    enabled: false
    services: ["_http._tcp", "_https._tcp"]
```

## Project Structure

```
nekzus/
├── cmd/nekzus/          # Main application
├── internal/
│   ├── auth/            # JWT, API keys, bootstrap tokens
│   ├── config/          # YAML config with hot reload
│   ├── discovery/       # Docker, Kubernetes, mDNS workers
│   ├── proxy/           # Reverse proxy, WebSocket, caching
│   ├── toolbox/         # Compose-based service deployment
│   ├── storage/         # SQLite persistence
│   └── ...
├── web/                 # React dashboard
├── toolbox/             # Service templates
├── configs/             # Example configurations
└── docs/                # Documentation
```

## Development

### Prerequisites

- Go 1.25+
- Node.js 20+ (for web UI)
- Docker (optional)

### Commands

```bash
make build        # Build Go binary
make build-web    # Build React UI
make build-all    # Build both
make test         # Run tests
make test-short   # Run unit tests only
make fmt          # Format code
make lint         # Run linter
make help         # Show all available targets
```

### Testing

```bash
# All tests with race detector
go test -race ./...

# Specific package
go test -race ./internal/proxy/... -v
```

## Requirements

| Component | Minimum | Recommended |
|-----------|---------|-------------|
| CPU | 1 core | 2+ cores |
| RAM | 256 MB | 512 MB+ |
| Disk | 100 MB | + storage for data |

**Platforms:** Linux, macOS, Windows (WSL2), Synology, Unraid, Proxmox, Raspberry Pi

## Contributing

Contributions are welcome! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines on how to get started, submit pull requests, and report issues.

## Support

- **Bug reports & feature requests**: [GitHub Issues](https://github.com/Nstalgic/Nekzus/issues)
- **Questions & discussions**: [GitHub Discussions](https://github.com/Nstalgic/Nekzus/discussions)

## License

This project is licensed under the [GNU General Public License v3.0](LICENSE).
