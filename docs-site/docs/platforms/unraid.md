# Unraid

This guide covers installing and configuring Nekzus on Unraid servers. Unraid provides an excellent Docker-based environment that makes deploying Nekzus straightforward.

---

## Requirements

### System Requirements

| Component | Minimum | Recommended |
|-----------|---------|-------------|
| Unraid Version | 6.10+ | 6.12+ |
| RAM | 256 MB available | 512 MB available |
| Disk Space | 100 MB | 500 MB (with data) |
| Docker | Enabled | Enabled |

### Prerequisites

Before installing Nekzus, ensure:

1. **Docker is enabled** in Unraid Settings
2. **Community Applications plugin is installed** (recommended for easiest installation)
3. **Network access** to Docker Hub or your container registry

To enable Docker:

1. Navigate to **Settings** > **Docker**
2. Set **Enable Docker** to **Yes**
3. Click **Apply**

---

## Installation Methods

### Method 1: Community Applications (Recommended)

The easiest way to install Nekzus on Unraid is through Community Applications.

:::note[Community Applications Plugin]

If you haven't installed the Community Applications plugin, install it first from the Unraid forums or by navigating to **Apps** and following the prompts.

:::


#### Installation Steps

1. Navigate to the **Apps** tab in Unraid
2. Search for **Nekzus**
3. Click **Install**
4. Configure the template variables (see [Template Variables](#template-variables))
5. Click **Apply**

---

### Method 2: Docker Template Manual Setup

If Nekzus is not available in Community Applications, create a Docker template manually.

#### Step 1: Add Container

1. Navigate to **Docker** tab
2. Click **Add Container**
3. Toggle **Basic View** to **Off** (advanced mode)

#### Step 2: Configure Container

Enter the following settings:

**Basic Settings:**

| Field | Value |
|-------|-------|
| Name | `nekzus` |
| Repository | `nstalgic/nekzus:latest` |
| Network Type | `bridge` (or `host` for full network access) |

**Port Mappings:**

| Container Port | Host Port | Description |
|----------------|-----------|-------------|
| 8080 | 8080 | Web UI and API |

Click **Add another Path, Port, Variable, Label or Device** to add each mapping.

#### Step 3: Add Path Mappings

| Config Type | Name | Container Path | Host Path | Access Mode |
|-------------|------|----------------|-----------|-------------|
| Path | Data | `/app/data` | `/mnt/user/appdata/nekzus/data` | Read/Write |
| Path | Config | `/app/configs` | `/mnt/user/appdata/nekzus/configs` | Read/Write |
| Path | Docker Socket | `/var/run/docker.sock` | `/var/run/docker.sock` | Read Only |
| Path | Toolbox | `/app/toolbox` | `/mnt/user/appdata/nekzus/toolbox` | Read/Write |

:::warning[Docker Socket Security]

Mounting the Docker socket grants Nekzus read access to your Docker environment for service discovery. The socket is mounted read-only for security.

:::


#### Step 4: Add Environment Variables

| Variable | Value | Description |
|----------|-------|-------------|
| `NEKZUS_JWT_SECRET` | *Generate a 32+ character secret* | JWT signing secret |
| `NEKZUS_BOOTSTRAP_TOKEN` | *Generate a secure token* | Bootstrap authentication |
| `NEKZUS_ADDR` | `:8080` | Listen address |
| `NEKZUS_BASE_URL` | `http://YOUR_UNRAID_IP:8080` | Base URL for QR pairing |

Generate secure secrets using:

```bash
# Generate JWT secret (run in Unraid terminal)
openssl rand -base64 32

# Generate bootstrap token
openssl rand -base64 24
```

#### Step 5: Apply and Start

1. Click **Apply**
2. The container will download and start
3. Access Nekzus at `http://YOUR_UNRAID_IP:8080`

---

### Method 3: Command Line

For users comfortable with the Unraid terminal, install via command line.

#### Basic Installation

```bash
docker run -d \
  --name nekzus \
  --restart unless-stopped \
  -p 8080:8080 \
  -v /mnt/user/appdata/nekzus/data:/app/data \
  -v /mnt/user/appdata/nekzus/configs:/app/configs \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  -e NEKZUS_JWT_SECRET="$(openssl rand -base64 32)" \
  -e NEKZUS_BOOTSTRAP_TOKEN="$(openssl rand -base64 24)" \
  -e NEKZUS_ADDR=":8080" \
  nstalgic/nekzus:latest
```

#### Production Installation with Custom Config

```bash
# Create directories
mkdir -p /mnt/user/appdata/nekzus/{data,configs,toolbox}

# Create config file (optional)
cat > /mnt/user/appdata/nekzus/configs/config.yaml << 'EOF'
server:
  addr: ":8080"
  base_url: "http://YOUR_UNRAID_IP:8080"

discovery:
  enabled: true
  docker:
    enabled: true
    socket_path: "unix:///var/run/docker.sock"
    poll_interval: "30s"
  mdns:
    enabled: true
    scan_interval: "60s"

toolbox:
  enabled: true
  catalog_dir: "/app/toolbox"
  data_dir: "/app/data/toolbox"

storage:
  database_path: "/app/data/nexus.db"
EOF

# Run container with config
docker run -d \
  --name nekzus \
  --restart unless-stopped \
  -p 8080:8080 \
  -v /mnt/user/appdata/nekzus/data:/app/data \
  -v /mnt/user/appdata/nekzus/configs:/app/configs:ro \
  -v /mnt/user/appdata/nekzus/toolbox:/app/toolbox \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  -e NEKZUS_JWT_SECRET="$(openssl rand -base64 32)" \
  -e NEKZUS_BOOTSTRAP_TOKEN="$(openssl rand -base64 24)" \
  nstalgic/nekzus:latest \
  --config /app/configs/config.yaml --insecure-http
```

---

## Configuration

### Template Variables

When configuring Nekzus in Unraid's Docker UI, use these settings:

#### Required Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `NEKZUS_JWT_SECRET` | *None* | JWT signing secret. Must be 32+ characters. Generate with `openssl rand -base64 32` |
| `NEKZUS_BOOTSTRAP_TOKEN` | *None* | Token for initial device pairing. Generate with `openssl rand -base64 24` |

#### Optional Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `NEKZUS_ADDR` | `:8080` | Listen address (port binding) |
| `NEKZUS_BASE_URL` | Auto-detected | Public URL for QR code pairing (e.g., `http://192.168.1.100:8080`) |
| `NEKZUS_DATABASE_PATH` | `/app/data/nexus.db` | SQLite database location |
| `NEKZUS_TLS_CERT` | *Empty* | Path to TLS certificate (for direct HTTPS) |
| `NEKZUS_TLS_KEY` | *Empty* | Path to TLS private key |

### Path Mappings Explained

| Container Path | Recommended Host Path | Purpose |
|----------------|----------------------|---------|
| `/app/data` | `/mnt/user/appdata/nekzus/data` | SQLite database, backups, runtime data |
| `/app/configs` | `/mnt/user/appdata/nekzus/configs` | Configuration files |
| `/app/toolbox` | `/mnt/user/appdata/nekzus/toolbox` | Service deployment templates |
| `/var/run/docker.sock` | `/var/run/docker.sock` | Docker API for service discovery |

### Network Types

Choose the appropriate network type for your setup:

#### Bridge Mode (Default)

```
Network Type: bridge
Port: 8080 -> 8080
```

- Standard Docker networking
- Port mapping required
- Works with most setups
- Recommended for most users

#### Host Mode

```
Network Type: host
```

- Container uses host network directly
- No port mapping needed
- Better for mDNS discovery
- Required if discovering services on multiple Docker networks

#### Custom Bridge Network

For integration with other containers:

```
Network Type: Custom: br0
```

- Uses Unraid's custom bridge
- Allows direct IP assignment
- Useful for accessing from other VLANs

---

## Security Configuration

### Reverse Proxy with Nginx Proxy Manager

For HTTPS access with automatic certificates, use Nginx Proxy Manager.

#### Prerequisites

1. Install Nginx Proxy Manager from Community Applications
2. Configure your domain DNS to point to your Unraid server
3. Forward ports 80 and 443 on your router

#### Configuration Steps

1. Open Nginx Proxy Manager web UI
2. Navigate to **Proxy Hosts** > **Add Proxy Host**
3. Configure the proxy:

    **Details Tab:**

    | Field | Value |
    |-------|-------|
    | Domain Names | `nexus.yourdomain.com` |
    | Scheme | `http` |
    | Forward Hostname/IP | `YOUR_UNRAID_IP` |
    | Forward Port | `8080` |
    | Websockets Support | Enabled |

    **SSL Tab:**

    | Field | Value |
    |-------|-------|
    | SSL Certificate | Request a new SSL Certificate |
    | Force SSL | Enabled |
    | HTTP/2 Support | Enabled |

4. Click **Save**

5. Update Nekzus environment variable:

    ```
    NEKZUS_BASE_URL=https://nexus.yourdomain.com
    ```

#### WebSocket Configuration

Nekzus uses WebSocket for real-time updates. Ensure WebSocket support is enabled in your proxy configuration.

For Nginx Proxy Manager, the "Websockets Support" toggle handles this automatically.

For manual Nginx configuration:

```nginx
location / {
    proxy_pass http://YOUR_UNRAID_IP:8080;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}
```

### Reverse Proxy with Traefik

If using Traefik as your reverse proxy:

#### Docker Labels Method

Add these labels to your Nekzus container:

```yaml
labels:
  - "traefik.enable=true"
  - "traefik.http.routers.nexus.rule=Host(`nexus.yourdomain.com`)"
  - "traefik.http.routers.nexus.entrypoints=websecure"
  - "traefik.http.routers.nexus.tls.certresolver=letsencrypt"
  - "traefik.http.services.nexus.loadbalancer.server.port=8080"
```

#### Via Unraid Docker UI

1. Edit the Nekzus container
2. Add labels using the **Add another Path, Port, Variable, Label or Device** button
3. Select **Label** for each entry

### Reverse Proxy with Caddy

For Caddy users, add to your Caddyfile:

```caddyfile
nexus.yourdomain.com {
    reverse_proxy YOUR_UNRAID_IP:8080
}
```

### Cloudflare Tunnel (Zero Trust)

For remote access without port forwarding:

1. Install Cloudflare Tunnel container on Unraid
2. Create a tunnel in Cloudflare Zero Trust dashboard
3. Add a public hostname pointing to `http://YOUR_UNRAID_IP:8080`
4. Update `NEKZUS_BASE_URL` to your Cloudflare domain

---

## Toolbox Integration

Nekzus includes a Toolbox feature for one-click service deployment. On Unraid, configure it properly:

### Host Data Directory

When running Nekzus in Docker, the Toolbox needs to know the host paths for volume mappings:

```bash
# In container environment variables
NEKZUS_HOST_DATA_DIR=/mnt/user/appdata/nekzus/data/toolbox
```

Or in config file:

```yaml
toolbox:
  enabled: true
  catalog_dir: "/app/toolbox"
  data_dir: "/app/data/toolbox"
  host_data_dir: "/mnt/user/appdata/nekzus/data/toolbox"
```

### Adding Custom Templates

Place Docker Compose templates in `/mnt/user/appdata/nekzus/toolbox/`:

```
/mnt/user/appdata/nekzus/toolbox/
  myservice/
    docker-compose.yml
```

Each template needs proper labels:

```yaml
services:
  myservice:
    image: vendor/myservice:latest
    labels:
      nekzus.toolbox.name: "My Service"
      nekzus.toolbox.category: "media"
      nekzus.toolbox.description: "Description here"
```

---

## Troubleshooting

### Container Fails to Start

**Check container logs:**

```bash
docker logs nekzus
```

**Common causes:**

| Issue | Solution |
|-------|----------|
| Port 8080 in use | Change host port mapping or stop conflicting container |
| Invalid secrets | Ensure `NEKZUS_JWT_SECRET` is 32+ characters |
| Permission denied | Check path mappings and Docker socket permissions |

### Docker Discovery Not Working

**Verify Docker socket is mounted:**

```bash
docker inspect nekzus | grep docker.sock
```

**Check socket permissions:**

```bash
ls -la /var/run/docker.sock
```

On Unraid, the Docker socket should be accessible. If using a custom Docker network, ensure the container can reach other containers.

### Cannot Access Web UI

1. **Check container is running:**

    ```bash
    docker ps | grep nekzus
    ```

2. **Verify port mapping:**

    ```bash
    docker port nekzus
    ```

3. **Check firewall (if enabled):**

    Unraid typically does not have a firewall enabled by default. If you have installed one, ensure port 8080 is allowed.

4. **Try accessing directly:**

    ```bash
    curl http://localhost:8080/healthz
    ```

### Services Not Discovered

**For Docker containers:**

Add discovery labels to your containers:

```yaml
labels:
  nekzus.enable: "true"
  nekzus.app.name: "My App"
  nekzus.app.id: "myapp"
```

**For mDNS services:**

Ensure `discovery.mdns.enabled: true` in config and that the container has access to the local network (use host networking if needed).

### Database Errors

**Check data directory permissions:**

```bash
ls -la /mnt/user/appdata/nekzus/data/
```

**Verify volume mount:**

```bash
docker inspect nekzus | grep -A5 Mounts
```

### High Memory Usage

Set resource limits in the Docker template:

1. Edit container in Docker tab
2. Scroll to **Extra Parameters**
3. Add: `--memory=512m --memory-swap=1g`

---

## Updates

### Via Unraid Docker UI (Recommended)

1. Navigate to **Docker** tab
2. Click the Nekzus container icon
3. Select **Check for Updates**
4. If an update is available, click **Update**

### Automatic Updates with Watchtower

Install Watchtower for automatic container updates:

```bash
docker run -d \
  --name watchtower \
  --restart unless-stopped \
  -v /var/run/docker.sock:/var/run/docker.sock \
  containrrr/watchtower \
  --cleanup \
  --schedule "0 4 * * *" \
  nekzus
```

This checks for updates daily at 4 AM.

### Manual Update via CLI

```bash
# Pull latest image
docker pull nstalgic/nekzus:latest

# Stop and remove old container
docker stop nekzus
docker rm nekzus

# Recreate with same settings (use your original run command)
docker run -d \
  --name nekzus \
  ... (your configuration)
```

### Version Pinning

For production stability, pin to a specific version:

```
Repository: nstalgic/nekzus:1.0.0
```

Check the [releases page](https://github.com/nstalgic/nekzus/releases) for available versions.

---

## Backup and Restore

### Backup

Nekzus data is stored in `/mnt/user/appdata/nekzus/`. Include this directory in your Unraid backup strategy.

**Manual backup:**

```bash
# Stop container for consistent backup
docker stop nekzus

# Create backup
tar -czvf nekzus-backup-$(date +%Y%m%d).tar.gz \
  /mnt/user/appdata/nekzus/

# Restart container
docker start nekzus
```

### Restore

```bash
# Stop container
docker stop nekzus

# Remove existing data (optional)
rm -rf /mnt/user/appdata/nekzus/*

# Restore from backup
tar -xzvf nekzus-backup-YYYYMMDD.tar.gz -C /

# Start container
docker start nekzus
```

### Automated Backups

Nekzus includes built-in backup functionality:

```yaml
backup:
  enabled: true
  directory: "/app/data/backups"
  schedule: "24h"
  retention: 7
```

Backups are stored in `/mnt/user/appdata/nekzus/data/backups/`.

---

## Performance Tuning

### Resource Allocation

For optimal performance on Unraid:

| Container Size | RAM | CPU |
|---------------|-----|-----|
| Small (< 10 services) | 256 MB | 0.5 core |
| Medium (10-50 services) | 512 MB | 1 core |
| Large (50+ services) | 1 GB | 2 cores |

### SSD Storage

For best database performance, place the data directory on an SSD cache pool:

```
Host Path: /mnt/cache/appdata/nekzus/data
```

Or use Unraid's cache settings for the appdata share.

---

## Integration with Unraid Services

### Homepage Dashboard

Add Nekzus to Homepage:

```yaml
- Nekzus:
    icon: mdi-api
    href: http://YOUR_UNRAID_IP:8080
    description: API Gateway
    widget:
      type: customapi
      url: http://YOUR_UNRAID_IP:8080/api/v1/admin/info
```

### Uptime Kuma

Monitor Nekzus health:

- **Monitor Type:** HTTP(s)
- **URL:** `http://YOUR_UNRAID_IP:8080/healthz`
- **Expected Status:** 200

---

## Next Steps

- [Quick Start Guide](../getting-started/quick-start) - Basic configuration walkthrough
- [Configuration Reference](../reference/configuration) - All configuration options
- [Toolbox Guide](../features/toolbox) - One-click service deployment
- [Docker Discovery](../features/discovery) - Automatic service discovery
