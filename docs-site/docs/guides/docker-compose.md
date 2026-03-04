import Tabs from "@theme/Tabs";
import TabItem from "@theme/TabItem";

# Docker Compose Deployment

This guide covers deploying Nekzus using Docker Compose, from basic development setups to production-ready configurations with TLS, monitoring, and resource management.

---

## Overview

Docker Compose simplifies multi-container deployments by defining services, networks, and volumes in a single YAML file. Nekzus provides several Compose configurations:

| Configuration | Use Case | TLS | Monitoring |
|--------------|----------|-----|------------|
| Basic | Development, testing | No | No |
| Production | Live deployments | Yes (Caddy) | Optional |
| Federation | Multi-instance clusters | Yes | Yes |

---

## Basic Setup

Get started quickly with a minimal Docker Compose configuration.

### Quick Start

1. **Create a project directory:**

    ```bash
    mkdir nekzus && cd nekzus
    ```

2. **Create `docker-compose.yml`:**

    ```yaml title="docker-compose.yml"
    services:
      nexus:
        image: nstalgic/nekzus:latest
        container_name: nekzus
        ports:
          - "8080:8080"
        environment:
          NEKZUS_JWT_SECRET: "${NEKZUS_JWT_SECRET:-change-this-to-strong-secret-min-32-chars}"
          NEKZUS_BOOTSTRAP_TOKEN: "${NEKZUS_BOOTSTRAP_TOKEN:-change-this-bootstrap-token}"
          NEKZUS_ADDR: ":8080"
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

3. **Generate secure secrets:**

    ```bash
    echo "NEKZUS_JWT_SECRET=$(openssl rand -base64 32)" > .env
    echo "NEKZUS_BOOTSTRAP_TOKEN=$(openssl rand -base64 24)" >> .env
    ```

4. **Start the service:**

    ```bash
    docker compose up -d
    ```

5. **Verify it's running:**

    ```bash
    curl http://localhost:8080/api/v1/healthz
    # Expected: ok
    ```

:::tip[Docker Socket Access]

Mounting the Docker socket (`/var/run/docker.sock`) enables automatic container discovery. If you don't need this feature, you can remove this volume mount for improved security.

:::


---

## Production Setup with TLS

For production deployments, use Caddy as a reverse proxy to handle TLS termination. This approach separates concerns and provides automatic HTTPS.

### Architecture

```d2
direction: right

internet: Internet/LAN
caddy: Caddy\n(TLS Termination)
nexus: Nekzus\n(port 8080) {
  style.fill: "#7c3aed"
  style.font-color: "#ffffff"
}

internet -> caddy: ":443 HTTPS"
caddy -> nexus: "HTTP"
```

### Production Compose File

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
      start_period: 10s
    deploy:
      resources:
        limits:
          cpus: '2.0'
          memory: 1G
        reservations:
          cpus: '0.5'
          memory: 256M
    logging:
      driver: "json-file"
      options:
        max-size: "10m"
        max-file: "3"

  caddy:
    image: caddy:2.8-alpine
    container_name: nekzus-caddy
    depends_on:
      nexus:
        condition: service_started
    networks:
      - nekzus
    environment:
      - NEKZUS_HOST=nexus
    ports:
      - "8443:8443"  # HTTPS
      - "80:80"      # HTTP (redirects to HTTPS)
    volumes:
      - ./deployments/Caddyfile:/etc/caddy/Caddyfile:ro
      - ./deployments/tls:/tls:ro
      - caddy-data:/data
      - caddy-config:/config
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "caddy", "validate", "--config", "/etc/caddy/Caddyfile"]
      interval: 30s
      timeout: 5s
      retries: 3
    deploy:
      resources:
        limits:
          cpus: '1.0'
          memory: 256M
        reservations:
          cpus: '0.25'
          memory: 64M
    logging:
      driver: "json-file"
      options:
        max-size: "10m"
        max-file: "3"

networks:
  nekzus:
    driver: bridge
    name: nekzus-network

volumes:
  nexus-data:
    name: nekzus-data
  caddy-data:
    name: nekzus-caddy-data
  caddy-config:
    name: nekzus-caddy-config
```

### Caddyfile Configuration

<Tabs>
<TabItem value="self-signed-local-network-" label="Self-Signed (Local Network)">


```title="Caddyfile"
:8443 {
    reverse_proxy nexus:8080
    tls internal
}

:80 {
    redir https://{host}:8443{uri} permanent
}
```

</TabItem>
<TabItem value="let-s-encrypt-public-domain-" label="Let's Encrypt (Public Domain)">


```title="Caddyfile"
nexus.example.com {
    reverse_proxy nexus:8080
    # Automatic Let's Encrypt certificates
}

http://nexus.example.com {
    redir https://{host}{uri} permanent
}
```

</TabItem>
<TabItem value="custom-certificates" label="Custom Certificates">


```title="Caddyfile"
:8443 {
    reverse_proxy nexus:8080
    tls /certs/cert.pem /certs/key.pem
}

:80 {
    redir https://{host}:8443{uri} permanent
}
```

</TabItem>
</Tabs>

:::warning[Security Notice]

The `--insecure-http` flag is safe when Nexus is behind a TLS-terminating reverse proxy like Caddy. Never expose an `--insecure-http` instance directly to the internet.

:::


---

## Environment Variables

### Using .env Files

Docker Compose automatically loads variables from a `.env` file in the same directory as `docker-compose.yml`.

```bash title=".env"
# Security (REQUIRED - generate strong random values)
NEKZUS_JWT_SECRET=your-secure-jwt-secret-minimum-32-characters
NEKZUS_BOOTSTRAP_TOKEN=your-secure-bootstrap-token-here

# Server Configuration
NEKZUS_ADDR=:8080
NEKZUS_BASE_URL=https://192.168.1.100:8443

# Optional: Database path
NEKZUS_DATABASE_PATH=/app/data/nexus.db
```

### Generating Secure Secrets

<Tabs>
<TabItem value="openssl" label="OpenSSL">


```bash
# Generate JWT secret (32+ characters)
echo "NEKZUS_JWT_SECRET=$(openssl rand -base64 32)" > .env

# Generate bootstrap token
echo "NEKZUS_BOOTSTRAP_TOKEN=$(openssl rand -base64 24)" >> .env
```

</TabItem>
<TabItem value="python" label="Python">


```bash
# Generate JWT secret
python3 -c "import secrets; print(f'NEKZUS_JWT_SECRET={secrets.token_urlsafe(32)}')" > .env

# Generate bootstrap token
python3 -c "import secrets; print(f'NEKZUS_BOOTSTRAP_TOKEN={secrets.token_urlsafe(24)}')" >> .env
```

</TabItem>
<TabItem value="manual" label="Manual">


```bash
# Create .env manually
cat > .env << 'EOF'
NEKZUS_JWT_SECRET=replace-with-strong-32-char-secret-value
NEKZUS_BOOTSTRAP_TOKEN=replace-with-strong-bootstrap-token
NEKZUS_BASE_URL=https://your-server-ip:8443
EOF
```

</TabItem>
</Tabs>

### Complete Environment Reference

| Variable | Description | Required | Default |
|----------|-------------|----------|---------|
| `NEKZUS_JWT_SECRET` | JWT signing secret (32+ chars) | Yes | None |
| `NEKZUS_BOOTSTRAP_TOKEN` | Bootstrap authentication token | Yes | None |
| `NEKZUS_ADDR` | Server listen address | No | `:8080` |
| `NEKZUS_BASE_URL` | Public URL for QR pairing | No | Auto-detect |
| `NEKZUS_TLS_CERT` | Path to TLS certificate | No | None |
| `NEKZUS_TLS_KEY` | Path to TLS private key | No | None |
| `NEKZUS_DATABASE_PATH` | SQLite database path | No | `./data/nexus.db` |

:::danger[Secret Security]

- Never commit `.env` files to version control
- Add `.env` to your `.gitignore`
- Use strong, randomly generated secrets
- Rotate secrets periodically in production

:::


---

## Volume Management

### Data Persistence

Nekzus stores data in SQLite and requires persistent volumes for:

| Volume | Purpose | Critical |
|--------|---------|----------|
| `/app/data` | SQLite database, backups | Yes |
| `/app/configs` | Configuration files | No (can be bind-mounted) |

### Named Volumes vs Bind Mounts

<Tabs>
<TabItem value="named-volumes-recommended-" label="Named Volumes (Recommended)">


```yaml
services:
  nexus:
    volumes:
      - nexus-data:/app/data

volumes:
  nexus-data:
    name: nekzus-data
```

**Advantages:**

- Docker manages storage location
- Works across different hosts
- Easy backup with `docker volume` commands

</TabItem>
<TabItem value="bind-mounts" label="Bind Mounts">


```yaml
services:
  nexus:
    volumes:
      - ./data:/app/data
      - ./config.yaml:/app/configs/config.yaml:ro
```

**Advantages:**

- Direct filesystem access
- Easy to inspect and edit
- Simpler backup with standard tools

</TabItem>
</Tabs>

### Backup Strategies

<details>
<summary>Automated Backups with Docker Volume</summary>


```bash
# Create backup of named volume
docker run --rm \
  -v nekzus-data:/source:ro \
  -v $(pwd)/backups:/backup \
  alpine tar czf /backup/nexus-$(date +%Y%m%d).tar.gz -C /source .

# Restore from backup
docker run --rm \
  -v nekzus-data:/target \
  -v $(pwd)/backups:/backup:ro \
  alpine tar xzf /backup/nexus-20240101.tar.gz -C /target
```

</details>


<details>
<summary>Built-in Backup Configuration</summary>


Nekzus includes automatic database backups. Configure in `config.yaml`:

```yaml
backup:
  enabled: true
  directory: "./data/backups"
  schedule: "24h"
  retention: 7
```

</details>


<details>
<summary>Bind Mount Backup Script</summary>


```bash title="backup.sh"
#!/bin/bash
BACKUP_DIR="./backups"
DATA_DIR="./data"
DATE=$(date +%Y%m%d_%H%M%S)

mkdir -p "$BACKUP_DIR"

# Stop for consistent backup (optional)
docker compose stop nexus

# Create backup
tar czf "$BACKUP_DIR/nexus-$DATE.tar.gz" -C "$DATA_DIR" .

# Restart
docker compose start nexus

# Keep only last 7 backups
ls -t "$BACKUP_DIR"/nexus-*.tar.gz | tail -n +8 | xargs rm -f 2>/dev/null

echo "Backup created: $BACKUP_DIR/nexus-$DATE.tar.gz"
```

</details>


---

## Networking

### Bridge Networks

Docker Compose creates a default bridge network for inter-container communication. For production, define explicit networks:

```yaml
services:
  nexus:
    networks:
      - frontend
      - backend

  caddy:
    networks:
      - frontend

  database:
    networks:
      - backend

networks:
  frontend:
    driver: bridge
    name: nekzus-frontend
  backend:
    driver: bridge
    name: nekzus-backend
    internal: true  # No external access
```

### Port Exposure

<Tabs>
<TabItem value="development" label="Development">


```yaml
services:
  nexus:
    ports:
      - "8080:8080"  # Exposed to host
```

</TabItem>
<TabItem value="production-behind-proxy-" label="Production (Behind Proxy)">


```yaml
services:
  nexus:
    expose:
      - "8080"  # Only accessible within Docker network
    networks:
      - internal

  caddy:
    ports:
      - "443:443"
      - "80:80"
    networks:
      - internal
```

</TabItem>
<TabItem value="specific-interface" label="Specific Interface">


```yaml
services:
  nexus:
    ports:
      - "127.0.0.1:8080:8080"  # Only localhost
      - "192.168.1.100:8080:8080"  # Specific IP
```

</TabItem>
</Tabs>

### Docker Socket for Discovery

To enable automatic container discovery, mount the Docker socket:

```yaml
services:
  nexus:
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
```

:::warning[Security Consideration]

The Docker socket provides root-level access to the host. Mount it read-only (`:ro`) and consider using a Docker socket proxy for additional security in high-security environments.

:::


---

## Resource Limits

### CPU and Memory Constraints

Define resource limits to prevent runaway containers:

```yaml
services:
  nexus:
    deploy:
      resources:
        limits:
          cpus: '2.0'
          memory: 1G
        reservations:
          cpus: '0.5'
          memory: 256M
```

### Recommended Resources

| Service | Min CPU | Max CPU | Min Memory | Max Memory |
|---------|---------|---------|------------|------------|
| Nexus | 0.5 | 2.0 | 256 MB | 1 GB |
| Caddy | 0.25 | 1.0 | 64 MB | 256 MB |

### Logging Configuration

Prevent disk exhaustion with log rotation:

```yaml
services:
  nexus:
    logging:
      driver: "json-file"
      options:
        max-size: "10m"
        max-file: "3"
```

---

## Health Checks

### Container Health Monitoring

Nekzus includes a built-in health check endpoint:

```yaml
services:
  nexus:
    healthcheck:
      test: ["/app/nekzus", "--health"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 10s
```

### Health Check Options

| Parameter | Description | Recommended |
|-----------|-------------|-------------|
| `interval` | Time between checks | 30s |
| `timeout` | Max time for check | 5s |
| `retries` | Failures before unhealthy | 3 |
| `start_period` | Grace period at start | 10s |

### Dependency Health Checks

Wait for dependencies to be healthy before starting:

```yaml
services:
  nexus:
    depends_on:
      database:
        condition: service_healthy

  caddy:
    depends_on:
      nexus:
        condition: service_healthy
```

### Monitoring Health Status

```bash
# Check container health
docker compose ps

# Detailed health status
docker inspect nekzus --format='{{.State.Health.Status}}'

# Health check logs
docker inspect nekzus --format='{{json .State.Health.Log}}' | jq
```

---

## Multi-Service Examples

### Nexus with Monitoring Stack

Deploy Nekzus with Prometheus and Grafana for observability:

```yaml title="docker-compose.monitoring.yml"
services:
  nexus:
    image: nstalgic/nekzus:latest
    container_name: nekzus
    networks:
      - nekzus
      - monitoring
    environment:
      NEKZUS_JWT_SECRET: "${NEKZUS_JWT_SECRET}"
      NEKZUS_BOOTSTRAP_TOKEN: "${NEKZUS_BOOTSTRAP_TOKEN}"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - nexus-data:/app/data
    restart: unless-stopped
    healthcheck:
      test: ["/app/nekzus", "--health"]
      interval: 30s
      timeout: 5s
      retries: 3

  prometheus:
    image: prom/prometheus:latest
    container_name: nekzus-prometheus
    networks:
      - monitoring
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml:ro
      - prometheus-data:/prometheus
    command:
      - '--config.file=/etc/prometheus/prometheus.yml'
      - '--storage.tsdb.path=/prometheus'
      - '--web.console.libraries=/usr/share/prometheus/console_libraries'
      - '--web.console.templates=/usr/share/prometheus/consoles'
    restart: unless-stopped

  grafana:
    image: grafana/grafana:latest
    container_name: nekzus-grafana
    networks:
      - monitoring
    environment:
      GF_SECURITY_ADMIN_PASSWORD: "${GRAFANA_PASSWORD:-admin}"
    volumes:
      - grafana-data:/var/lib/grafana
    ports:
      - "3000:3000"
    restart: unless-stopped

networks:
  nekzus:
    driver: bridge
  monitoring:
    driver: bridge

volumes:
  nexus-data:
  prometheus-data:
  grafana-data:
```

```yaml title="prometheus.yml"
global:
  scrape_interval: 15s
  evaluation_interval: 15s

scrape_configs:
  - job_name: 'nexus'
    static_configs:
      - targets: ['nexus:8080']
    metrics_path: /metrics
```

### Federation Cluster

Deploy multiple Nexus instances for high availability and distributed service discovery:

```yaml title="docker-compose.federation.yml"
services:
  nexus-1:
    image: nstalgic/nekzus:latest
    container_name: nekzus-1
    hostname: nexus-1
    networks:
      - federation
    ports:
      - "8080:8080"
      - "7946:7946/tcp"
      - "7946:7946/udp"
    environment:
      NEKZUS_ID: "nexus-instance-1"
      NEKZUS_JWT_SECRET: "${NEKZUS_JWT_SECRET}"
      NEKZUS_BOOTSTRAP_TOKEN: "${NEKZUS_BOOTSTRAP_TOKEN}"
      NEKZUS_FEDERATION_ENABLED: "true"
      NEKZUS_CLUSTER_SECRET: "${NEKZUS_CLUSTER_SECRET}"
      NEKZUS_GOSSIP_PORT: "7946"
    volumes:
      - ./configs/instance1.yaml:/app/configs/config.yaml:ro
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - federation-data-1:/app/data
    restart: unless-stopped

  nexus-2:
    image: nstalgic/nekzus:latest
    container_name: nekzus-2
    hostname: nexus-2
    networks:
      - federation
    ports:
      - "8081:8080"
      - "7947:7946/tcp"
      - "7947:7946/udp"
    environment:
      NEKZUS_ID: "nexus-instance-2"
      NEKZUS_JWT_SECRET: "${NEKZUS_JWT_SECRET}"
      NEKZUS_BOOTSTRAP_TOKEN: "${NEKZUS_BOOTSTRAP_TOKEN}"
      NEKZUS_FEDERATION_ENABLED: "true"
      NEKZUS_CLUSTER_SECRET: "${NEKZUS_CLUSTER_SECRET}"
      NEKZUS_GOSSIP_PORT: "7946"
    volumes:
      - ./configs/instance2.yaml:/app/configs/config.yaml:ro
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - federation-data-2:/app/data
    depends_on:
      - nexus-1
    restart: unless-stopped

networks:
  federation:
    driver: bridge
    name: nekzus-federation
    ipam:
      config:
        - subnet: 172.30.0.0/16

volumes:
  federation-data-1:
  federation-data-2:
```

---

## Troubleshooting

### Common Issues

<details>
<summary>Container fails to start</summary>


**Check the logs:**

```bash
docker compose logs nexus
```

**Common causes:**

- Port already in use: `Error: listen tcp :8080: bind: address already in use`
- Invalid environment variables
- Missing required secrets
- Permission issues with mounted volumes

**Solutions:**

```bash
# Check if port is in use
lsof -i :8080

# Verify environment variables
docker compose config

# Check volume permissions
ls -la ./data
```

</details>


<details>
<summary>Docker discovery not working</summary>


**Verify Docker socket is mounted:**

```bash
docker compose exec nexus ls -la /var/run/docker.sock
```

**Check discovery status:**

```bash
curl http://localhost:8080/api/v1/discovery/status
```

**Common causes:**

- Docker socket not mounted or wrong path
- Permission denied on Docker socket
- Discovery disabled in configuration

**Solutions:**

```yaml
# Ensure socket is mounted read-only
volumes:
  - /var/run/docker.sock:/var/run/docker.sock:ro
```

On some systems (especially rootless Docker), you may need additional configuration.

</details>


<details>
<summary>Health check failing</summary>


**Check health status:**

```bash
docker inspect nekzus --format='{{json .State.Health}}' | jq
```

**View health check logs:**

```bash
docker inspect nekzus --format='{{range .State.Health.Log}}{{.Output}}{{end}}'
```

**Common causes:**

- Container still starting (check `start_period`)
- Application crashed internally
- Health endpoint path changed

</details>


<details>
<summary>Cannot connect from other devices</summary>


**Verify port binding:**

```bash
# Check if bound to all interfaces
docker compose ps
netstat -tlnp | grep 8080
```

**Common causes:**

- Port bound to localhost only (`127.0.0.1:8080:8080`)
- Firewall blocking connections
- `NEKZUS_BASE_URL` not set for QR pairing

**Solutions:**

```yaml
ports:
  - "8080:8080"  # Bind to all interfaces
```

```bash
# Allow through firewall (Linux)
sudo ufw allow 8080/tcp

# macOS
# Check System Preferences > Security & Privacy > Firewall
```

</details>


<details>
<summary>Caddy TLS certificate issues</summary>


**Check Caddy logs:**

```bash
docker compose logs caddy
```

**Common causes:**

- DNS not resolving for Let's Encrypt
- Port 80/443 blocked by firewall
- Rate limited by Let's Encrypt

**Solutions:**

```title="Caddyfile"
# Use internal certificates for local network
:8443 {
    reverse_proxy nexus:8080
    tls internal
}
```

</details>


<details>
<summary>Volume permission denied</summary>


**Check current permissions:**

```bash
ls -la ./data
docker compose exec nexus ls -la /app/data
```

**Fix permissions:**

```bash
# For bind mounts
sudo chown -R $(id -u):$(id -g) ./data

# Or allow container user
sudo chown -R 0:0 ./data
```

</details>


### Debugging Commands

```bash
# View real-time logs
docker compose logs -f

# Execute shell in container
docker compose exec nexus sh

# Check container resource usage
docker stats nekzus

# Inspect container configuration
docker inspect nekzus

# View network configuration
docker network inspect nekzus-network

# List all volumes
docker volume ls | grep nekzus

# Restart specific service
docker compose restart nexus

# Rebuild and restart
docker compose up -d --build nexus

# Clean restart (removes volumes)
docker compose down -v && docker compose up -d
```

### Log Analysis

```bash
# Filter logs by severity
docker compose logs nexus 2>&1 | grep -i error

# Follow logs with timestamps
docker compose logs -f -t nexus

# Last 100 lines
docker compose logs --tail 100 nexus

# Export logs to file
docker compose logs nexus > nexus.log 2>&1
```

---

## Next Steps

- [Configuration Reference](../getting-started/configuration) - Detailed configuration options
- [Toolbox Guide](../features/toolbox) - One-click service deployment
- [API Reference](../reference/api) - REST API documentation
