# Installation

This guide covers how to install Nekzus on your system. Choose the installation method that best fits your environment.

## System Requirements

### Minimum Requirements

| Component | Requirement |
|-----------|-------------|
| CPU | 1 core (2+ recommended) |
| RAM | 256 MB (512 MB+ recommended) |
| Disk | 100 MB for application, additional for data |
| Network | Access to local network services |

### Software Requirements

=== "Docker (Recommended)"

    - Docker 20.10+ or compatible container runtime
    - Docker Compose v2.0+ (for compose deployments)
    - Network access to Docker Hub or your container registry

=== "Building from Source"

    - Go 1.25+
    - Node.js 20+ and npm (for web UI)
    - GCC and libc6-dev (for SQLite CGO support)
    - Make (optional, for convenience commands)
    - Git

### Supported Platforms

Nekzus runs on:

- **Linux**: amd64, arm64 (including Raspberry Pi 4+)
- **macOS**: Apple Silicon (arm64), Intel (amd64)
- **Windows**: amd64 (via Docker or WSL2)
- **NAS Systems**: Synology, Unraid, QNAP (via Docker)
- **Kubernetes**: Any cluster with Docker registry access

---

## Installation Methods

### Docker (Recommended)

Docker is the recommended installation method for most users. It provides a consistent environment with minimal setup.

#### Quick Start

```bash
docker run -d \
  --name nekzus \
  -p 8080:8080 \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  -v nekzus-data:/app/data \
  nstalgic/nekzus:latest
```

This command:

- Starts Nekzus in the background
- Exposes the web UI on port 8080
- Mounts the Docker socket for container discovery (read-only)
- Creates a persistent volume for data

#### With Environment Variables

For production deployments, configure security settings:

```bash
docker run -d \
  --name nekzus \
  -p 8080:8080 \
  -e NEKZUS_JWT_SECRET="$(openssl rand -base64 32)" \
  -e NEKZUS_BOOTSTRAP_TOKEN="$(openssl rand -base64 24)" \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  -v nekzus-data:/app/data \
  nstalgic/nekzus:latest
```

!!! warning "Security Notice"
    Always use strong, randomly generated secrets for `NEKZUS_JWT_SECRET` (minimum 32 characters) and `NEKZUS_BOOTSTRAP_TOKEN` in production environments.

---

### Docker Compose

Docker Compose is ideal for production deployments with TLS termination via Caddy.

#### Basic Setup

1. **Create a project directory:**

    ```bash
    mkdir nekzus && cd nekzus
    ```

2. **Create `docker-compose.yml`:**

    ```yaml
    services:
      nexus:
        image: nstalgic/nekzus:latest
        container_name: nekzus
        environment:
          NEKZUS_JWT_SECRET: "${NEKZUS_JWT_SECRET:-change-this-to-strong-secret-min-32-chars}"
          NEKZUS_BOOTSTRAP_TOKEN: "${NEKZUS_BOOTSTRAP_TOKEN:-change-this-bootstrap-token}"
          NEKZUS_ADDR: ":8080"
        ports:
          - "8080:8080"
        volumes:
          - /var/run/docker.sock:/var/run/docker.sock:ro
          - nexus-data:/app/data
        restart: unless-stopped
        healthcheck:
          test: ["/app/nekzus", "--health"]
          interval: 30s
          timeout: 5s
          retries: 3
          start_period: 10s

    volumes:
      nexus-data:
    ```

3. **Create `.env` file with secure secrets:**

    ```bash
    echo "NEKZUS_JWT_SECRET=$(openssl rand -base64 32)" > .env
    echo "NEKZUS_BOOTSTRAP_TOKEN=$(openssl rand -base64 24)" >> .env
    ```

4. **Start the service:**

    ```bash
    docker compose up -d
    ```

#### Production Setup with TLS (Caddy)

For production with automatic HTTPS via Caddy:

```yaml title="docker-compose.yml"
services:
  nexus:
    image: nstalgic/nekzus:latest
    container_name: nekzus
    networks:
      - nekzus
    environment:
      NEKZUS_JWT_SECRET: "${NEKZUS_JWT_SECRET}"
      NEKZUS_BOOTSTRAP_TOKEN: "${NEKZUS_BOOTSTRAP_TOKEN}"
      NEKZUS_BASE_URL: "${NEKZUS_BASE_URL:-https://localhost:8443}"
      NEKZUS_ADDR: ":8080"
    command: ["--config", "/app/configs/config.yaml", "--insecure-http"]
    volumes:
      - ./config.yaml:/app/configs/config.yaml:ro
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - nexus-data:/app/data
    restart: unless-stopped
    healthcheck:
      test: ["/app/nekzus", "--health"]
      interval: 30s
      timeout: 5s
      retries: 3
    deploy:
      resources:
        limits:
          cpus: '2.0'
          memory: 1G

  caddy:
    image: caddy:2.8-alpine
    container_name: nekzus-caddy
    depends_on:
      - nexus
    networks:
      - nekzus
    ports:
      - "8443:8443"
      - "80:80"
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile:ro
      - caddy-data:/data
      - caddy-config:/config
    restart: unless-stopped

networks:
  nekzus:
    driver: bridge

volumes:
  nexus-data:
  caddy-data:
  caddy-config:
```

```title="Caddyfile"
:8443 {
    reverse_proxy nexus:8080
    tls internal
}

:80 {
    redir https://{host}:8443{uri} permanent
}
```

---

### Building from Source

Build Nekzus from source for development or custom deployments.

#### Prerequisites

Install the required dependencies:

=== "macOS"

    ```bash
    # Install Homebrew if not installed
    /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"

    # Install dependencies
    brew install go node
    ```

=== "Ubuntu/Debian"

    ```bash
    # Install Go (check https://go.dev/dl/ for latest version)
    wget https://go.dev/dl/go1.25.linux-amd64.tar.gz
    sudo tar -C /usr/local -xzf go1.25.linux-amd64.tar.gz
    echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
    source ~/.bashrc

    # Install Node.js
    curl -fsSL https://deb.nodesource.com/setup_20.x | sudo -E bash -
    sudo apt-get install -y nodejs

    # Install build dependencies for SQLite
    sudo apt-get install -y gcc libc6-dev libsqlite3-dev make
    ```

=== "Fedora/RHEL"

    ```bash
    # Install Go
    sudo dnf install golang

    # Install Node.js
    sudo dnf install nodejs npm

    # Install build dependencies
    sudo dnf install gcc glibc-devel sqlite-devel make
    ```

#### Build Steps

1. **Clone the repository:**

    ```bash
    git clone https://github.com/nstalgic/nekzus.git
    cd nekzus
    ```

2. **Build the web UI and Go binary:**

    ```bash
    # Build everything (recommended)
    make build-all

    # Or build separately:
    make build-web  # Build React dashboard
    make build      # Build Go binary
    ```

3. **Verify the build:**

    ```bash
    ./bin/nekzus --version
    ```

#### Running from Source

```bash
# Run with TLS (requires certificates)
make run

# Run without TLS (development only)
make run-insecure

# Run with custom config
./bin/nekzus --config configs/config.yaml --insecure-http
```

---

### Demo Environment

The fastest way to try Nekzus with example services:

```bash
# Clone the repository
git clone https://github.com/nstalgic/nekzus.git
cd nekzus

# Start demo with web UI + test services
make demo

# Access the web dashboard
open http://localhost:8080

# View logs
make demo-logs

# Stop demo
make demo-down
```

The demo environment includes:

- Nekzus with embedded web dashboard
- 2 auto-discovered test services
- Docker discovery enabled
- Ready in approximately 30 seconds

---

## First-Time Setup

After installation, complete these steps:

### 1. Access the Web Dashboard

Open your browser and navigate to:

- **Docker/Compose**: `http://localhost:8080`
- **With TLS**: `https://localhost:8443`
- **Remote access**: `http://<server-ip>:8080`

### 2. Configure Environment Variables

For production deployments, set these environment variables:

| Variable | Description | Required |
|----------|-------------|----------|
| `NEKZUS_JWT_SECRET` | JWT signing secret (32+ characters) | Yes |
| `NEKZUS_BOOTSTRAP_TOKEN` | Bootstrap authentication token | Yes |
| `NEKZUS_ADDR` | Server listen address (default: `:8080`) | No |
| `NEKZUS_BASE_URL` | Public URL for QR code pairing | No |
| `NEKZUS_TLS_CERT` | Path to TLS certificate | No |
| `NEKZUS_TLS_KEY` | Path to TLS private key | No |
| `NEKZUS_DATABASE_PATH` | SQLite database path | No |

### 3. Enable Service Discovery

Nekzus can automatically discover services. Enable discovery by:

**Docker Discovery** (requires Docker socket access):

```yaml
discovery:
  docker:
    enabled: true
    socket_path: "unix:///var/run/docker.sock"
    poll_interval: "30s"
```

**mDNS Discovery** (for network services):

```yaml
discovery:
  mdns:
    enabled: true
    scan_interval: "60s"
    services:
      - "_http._tcp"
      - "_https._tcp"
```

### 4. Mobile App Pairing (Optional)

Generate a QR code for mobile app pairing:

```bash
# If running from source
make qr

# Or via API
curl http://localhost:8080/api/v1/auth/qr
```

---

## Verification

Verify your installation is working correctly:

### Health Check

```bash
curl http://localhost:8080/healthz
```

Expected response:

```json
{"status":"healthy"}
```

### API Status

```bash
curl http://localhost:8080/api/v1/admin/info
```

### Service Discovery

```bash
curl http://localhost:8080/api/v1/apps
```

### Web Dashboard

Open `http://localhost:8080` in your browser. You should see the Nekzus dashboard with:

- Overview page showing system status
- Discovery page showing found services
- Settings for theme customization

---

## Troubleshooting

### Common Issues

??? question "Container fails to start"

    Check the container logs:

    ```bash
    docker logs nekzus
    ```

    Common causes:

    - Port 8080 already in use
    - Invalid environment variables
    - Docker socket permission issues

??? question "Docker discovery not working"

    Ensure the Docker socket is mounted correctly:

    ```bash
    docker run -d \
      -v /var/run/docker.sock:/var/run/docker.sock:ro \
      ...
    ```

    On some systems, you may need to run the container as root or add the container user to the docker group.

??? question "Permission denied on data volume"

    The container runs as root by default. If using a bind mount:

    ```bash
    sudo chown -R 0:0 /path/to/data
    ```

??? question "Cannot access from other devices"

    Ensure:

    - The port is exposed correctly (`-p 8080:8080`)
    - Firewall allows incoming connections on port 8080
    - `NEKZUS_BASE_URL` is set to a reachable address for QR pairing

### Getting Help

If you encounter issues:

1. Check the [Troubleshooting Guide](../guides/troubleshooting.md)
2. Search [existing issues](https://github.com/nstalgic/nekzus/issues)
3. Open a new issue with logs and configuration details

---

## Next Steps

- [Quick Start Guide](quick-start.md) - Get started with basic configuration
- [Configuration Reference](configuration.md) - Detailed configuration options
- [Platform Guides](../platforms/index.md) - Platform-specific instructions
- [Docker Compose Guide](../guides/docker-compose.md) - Advanced Docker Compose setups
