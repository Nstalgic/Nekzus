# Synology NAS

This guide covers deploying Nekzus on Synology NAS devices using DSM (DiskStation Manager). Synology NAS provides an excellent platform for running Nekzus as a central API gateway for your home lab or small business network.

---

## Requirements

### Supported DSM Versions

| DSM Version | Docker Support | Recommended Method |
|-------------|---------------|-------------------|
| DSM 7.2+ | Container Manager | Container Manager (GUI) |
| DSM 7.0 - 7.1 | Docker | Docker Package (GUI) |
| DSM 6.2+ | Docker | Docker Package (GUI) or SSH |

!!! warning "DSM 6.x End of Life"
    DSM 6.x is approaching end of life. Consider upgrading to DSM 7.2+ for the best experience and security updates.

### Hardware Requirements

| Component | Minimum | Recommended |
|-----------|---------|-------------|
| CPU | Intel x86_64 or ARM64 | Intel Celeron J4125 or better |
| RAM | 256 MB available | 512 MB+ available |
| Storage | 100 MB for container | 1 GB+ for data and logs |
| Network | 100 Mbps | Gigabit Ethernet |

### Supported NAS Models

Nekzus runs on any Synology NAS that supports Docker:

**Intel-based (Recommended):**

- DS923+, DS723+, DS423+
- DS920+, DS720+, DS420+
- DS1821+, DS1621+, DS1520+
- RS1221+, RS1219+, RS820+
- And other Plus/XS series models

**ARM-based:**

- DS223j, DS223 (ARM64)
- DS220j, DS218 (ARM64)
- Some older models may have limited performance

!!! note "Check Docker Compatibility"
    Verify your model supports Docker by checking the [Synology Docker compatibility list](https://www.synology.com/en-us/dsm/packages/ContainerManager) or searching for "Container Manager" in Package Center.

---

## Installation Methods

### Method 1: Container Manager (DSM 7.2+)

Container Manager is Synology's modern container management interface, replacing the older Docker package. This is the recommended method for DSM 7.2 and newer.

#### Step 1: Install Container Manager

1. Open **Package Center** from the DSM desktop
2. Search for **Container Manager**
3. Click **Install**
4. Wait for installation to complete

<!-- Screenshot placeholder: Package Center showing Container Manager -->

#### Step 2: Create Project Directory

Before creating the container, set up a directory structure for persistent data:

1. Open **File Station**
2. Navigate to a shared folder (e.g., `docker`)
3. Create a new folder: `nekzus`
4. Inside `nekzus`, create:
    - `data` - for database and persistent storage
    - `config` - for configuration files (optional)

Your structure should look like:

```
/volume1/docker/nekzus/
├── data/
└── config/
```

#### Step 3: Create the Container

=== "Using Container Manager GUI"

    1. Open **Container Manager**
    2. Go to **Registry** in the left sidebar
    3. Search for `nstalgic/nekzus`
    4. Select the image and click **Download**
    5. Choose the `latest` tag and click **Select**
    6. Wait for the download to complete

    <!-- Screenshot placeholder: Container Manager Registry search -->

    7. Go to **Image** in the left sidebar
    8. Select `nstalgic/nekzus:latest`
    9. Click **Run**

    <!-- Screenshot placeholder: Container Manager Image list -->

=== "Using Docker Compose Project"

    Container Manager supports Docker Compose projects:

    1. Open **Container Manager**
    2. Go to **Project** in the left sidebar
    3. Click **Create**
    4. Set **Project name**: `nekzus`
    5. Set **Path**: `/volume1/docker/nekzus`
    6. Select **Create docker-compose.yml**
    7. Paste the following configuration:

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
          - /volume1/docker/nekzus/data:/app/data
        restart: unless-stopped
        healthcheck:
          test: ["/app/nekzus", "--health"]
          interval: 30s
          timeout: 5s
          retries: 3
          start_period: 10s
    ```

    8. Click **Next** and review settings
    9. Click **Done** to create and start the project

#### Step 4: Configure Container Settings

If using the GUI method (not Compose), configure these settings in the container creation wizard:

**General Settings:**

- **Container Name**: `nekzus`
- **Enable auto-restart**: Checked
- **Enable resource limitation**: Optional (see Resource Limits below)

<!-- Screenshot placeholder: Container Manager general settings -->

**Port Settings:**

| Local Port | Container Port | Protocol |
|------------|---------------|----------|
| 8080 | 8080 | TCP |

<!-- Screenshot placeholder: Container Manager port settings -->

**Volume Settings:**

| Host Path | Container Path | Mode |
|-----------|---------------|------|
| `/volume1/docker/nekzus/data` | `/app/data` | Read/Write |
| `/var/run/docker.sock` | `/var/run/docker.sock` | Read Only |

!!! warning "Docker Socket Access"
    Mounting the Docker socket (`/var/run/docker.sock`) enables container discovery. This is optional but required for automatic service discovery. The socket is mounted read-only for security.

<!-- Screenshot placeholder: Container Manager volume settings -->

**Environment Variables:**

| Variable | Value | Description |
|----------|-------|-------------|
| `NEKZUS_JWT_SECRET` | (generate secure value) | JWT signing secret (32+ chars) |
| `NEKZUS_BOOTSTRAP_TOKEN` | (generate secure value) | Bootstrap authentication token |
| `NEKZUS_ADDR` | `:8080` | Server listen address |

Generate secure values using:

```bash
# Run in SSH or terminal
openssl rand -base64 32  # For NEKZUS_JWT_SECRET
openssl rand -base64 24  # For NEKZUS_BOOTSTRAP_TOKEN
```

<!-- Screenshot placeholder: Container Manager environment variables -->

**Resource Limits (Optional):**

- **CPU**: 50% (1 core equivalent on 2-core system)
- **Memory**: 512 MB

#### Step 5: Start the Container

1. Review all settings
2. Click **Done** or **Apply**
3. The container will start automatically
4. Check the container status in **Container** list

<!-- Screenshot placeholder: Container Manager container list showing running status -->

---

### Method 2: Docker Package (DSM 7.0-7.1)

For DSM versions before 7.2, use the Docker package.

#### Step 1: Install Docker

1. Open **Package Center**
2. Search for **Docker**
3. Click **Install**

#### Step 2: Download the Image

1. Open **Docker** from the application menu
2. Go to **Registry**
3. Search for `nstalgic/nekzus`
4. Select and click **Download**
5. Choose `latest` tag

#### Step 3: Create Container

1. Go to **Image** tab
2. Select `nstalgic/nekzus:latest`
3. Click **Launch**
4. Configure settings as described in Method 1, Step 4
5. Click **Apply**

---

### Method 3: Command Line via SSH

For advanced users or automation, deploy via SSH.

#### Step 1: Enable SSH

1. Go to **Control Panel** > **Terminal & SNMP**
2. Enable **SSH service**
3. Set a non-standard port for security (e.g., 2222)
4. Click **Apply**

!!! warning "SSH Security"
    Change the default SSH port and use key-based authentication for production environments. Disable SSH when not in use.

#### Step 2: Connect via SSH

```bash
ssh admin@your-nas-ip -p 2222
```

#### Step 3: Create Directory Structure

```bash
# Create project directory
sudo mkdir -p /volume1/docker/nekzus/data
sudo chown -R $(id -u):$(id -g) /volume1/docker/nekzus
```

#### Step 4: Generate Secure Secrets

```bash
# Generate and save secrets
export NEKZUS_JWT_SECRET=$(openssl rand -base64 32)
export NEKZUS_BOOTSTRAP_TOKEN=$(openssl rand -base64 24)

# Save to .env file for persistence
cat > /volume1/docker/nekzus/.env << EOF
NEKZUS_JWT_SECRET=${NEKZUS_JWT_SECRET}
NEKZUS_BOOTSTRAP_TOKEN=${NEKZUS_BOOTSTRAP_TOKEN}
EOF

# Secure the file
chmod 600 /volume1/docker/nekzus/.env
```

#### Step 5: Run with Docker

**Simple deployment:**

```bash
sudo docker run -d \
  --name nekzus \
  --restart unless-stopped \
  -p 8080:8080 \
  -e NEKZUS_JWT_SECRET="$NEKZUS_JWT_SECRET" \
  -e NEKZUS_BOOTSTRAP_TOKEN="$NEKZUS_BOOTSTRAP_TOKEN" \
  -e NEKZUS_ADDR=":8080" \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  -v /volume1/docker/nekzus/data:/app/data \
  nstalgic/nekzus:latest
```

**With Docker Compose:**

Create `/volume1/docker/nekzus/docker-compose.yml`:

```yaml
services:
  nexus:
    image: nstalgic/nekzus:latest
    container_name: nekzus
    environment:
      NEKZUS_JWT_SECRET: "${NEKZUS_JWT_SECRET}"
      NEKZUS_BOOTSTRAP_TOKEN: "${NEKZUS_BOOTSTRAP_TOKEN}"
      NEKZUS_ADDR: ":8080"
    ports:
      - "8080:8080"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - /volume1/docker/nekzus/data:/app/data
    restart: unless-stopped
    healthcheck:
      test: ["/app/nekzus", "--health"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 10s
```

Then start:

```bash
cd /volume1/docker/nekzus
sudo docker compose up -d
```

#### Step 6: Verify Deployment

```bash
# Check container status
sudo docker ps | grep nekzus

# Check logs
sudo docker logs nekzus

# Test health endpoint
curl http://localhost:8080/healthz
```

---

## Configuration

### Port Mapping

Nekzus uses the following ports:

| Port | Protocol | Purpose | Required |
|------|----------|---------|----------|
| 8080 | TCP | HTTP API and Web UI | Yes |
| 8443 | TCP | HTTPS (if using internal TLS) | Optional |
| 7946 | TCP/UDP | Federation gossip (if enabled) | Optional |

#### Changing the Default Port

To use a different port (e.g., 9080):

=== "Container Manager GUI"

    Edit the container and change port mapping from `8080:8080` to `9080:8080`

=== "Docker Compose"

    ```yaml
    ports:
      - "9080:8080"
    ```

=== "Docker CLI"

    ```bash
    -p 9080:8080
    ```

### Volume Mounts

| Host Path | Container Path | Purpose |
|-----------|---------------|---------|
| `/volume1/docker/nekzus/data` | `/app/data` | Database and persistent storage |
| `/volume1/docker/nekzus/config` | `/app/configs` | Custom configuration files |
| `/var/run/docker.sock` | `/var/run/docker.sock` | Docker discovery (optional) |

#### Data Directory Structure

The data volume contains:

```
/app/data/
├── nexus.db          # SQLite database
├── backups/          # Automatic backups
└── toolbox/          # Toolbox service data
```

!!! tip "Backup Location"
    The `nexus.db` file contains all your configuration, devices, and routes. Ensure this is included in your Synology backup strategy using Hyper Backup or similar.

### Docker Socket Access

Docker socket access enables automatic container discovery. Without it, you must manually configure services.

**Enabling Discovery:**

Mount the Docker socket read-only:

```yaml
volumes:
  - /var/run/docker.sock:/var/run/docker.sock:ro
```

**Discovery Labels:**

Add labels to your other Docker containers for automatic discovery:

```yaml
labels:
  nekzus.enable: "true"
  nekzus.app.name: "My Service"
  nekzus.app.id: "myservice"
  nekzus.route.path: "/apps/myservice/"
```

### Network Configuration

#### Bridge Network (Default)

The default bridge network works for most setups:

```yaml
networks:
  default:
    driver: bridge
```

#### Host Network Mode

For direct network access (required for mDNS discovery):

```yaml
services:
  nexus:
    network_mode: host
```

!!! note "Host Network Limitations"
    Host network mode bypasses Docker networking. Port settings in the compose file are ignored - the container binds directly to host ports.

#### Custom Bridge Network

For communication with other containers:

```yaml
services:
  nexus:
    networks:
      - nekzus-network

networks:
  nekzus-network:
    driver: bridge
    name: nekzus-network
```

#### Macvlan Network

For a dedicated IP address on your LAN:

```yaml
networks:
  lan:
    driver: macvlan
    driver_opts:
      parent: eth0
    ipam:
      config:
        - subnet: 192.168.1.0/24
          gateway: 192.168.1.1
          ip_range: 192.168.1.240/28
```

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `NEKZUS_JWT_SECRET` | JWT signing secret (32+ chars required) | None (required) |
| `NEKZUS_BOOTSTRAP_TOKEN` | Bootstrap authentication token | None (required) |
| `NEKZUS_ADDR` | Server listen address | `:8080` |
| `NEKZUS_BASE_URL` | Public URL for QR pairing | Auto-detected |
| `NEKZUS_DATABASE_PATH` | SQLite database path | `/app/data/nexus.db` |
| `NEKZUS_TLS_CERT` | TLS certificate path | None |
| `NEKZUS_TLS_KEY` | TLS private key path | None |

### Custom Configuration File

For advanced configuration, mount a custom config file:

1. Copy the example configuration:

    ```bash
    # Via SSH
    sudo docker cp nekzus:/app/configs/config.example.yaml \
      /volume1/docker/nekzus/config/config.yaml
    ```

2. Edit the configuration:

    ```bash
    nano /volume1/docker/nekzus/config/config.yaml
    ```

3. Mount the config file:

    ```yaml
    volumes:
      - /volume1/docker/nekzus/config/config.yaml:/app/configs/config.yaml:ro
    ```

4. Add the config flag to the command:

    ```yaml
    command: ["--config", "/app/configs/config.yaml"]
    ```

---

## Security Considerations

### Firewall Rules

Configure the Synology firewall to control access to Nekzus:

1. Go to **Control Panel** > **Security** > **Firewall**
2. Click **Edit Rules**
3. Add rules for Nekzus:

**Allow from local network only:**

| Source IP | Port | Protocol | Action |
|-----------|------|----------|--------|
| 192.168.1.0/24 | 8080 | TCP | Allow |
| All | 8080 | TCP | Deny |

**Allow specific IPs:**

| Source IP | Port | Protocol | Action |
|-----------|------|----------|--------|
| 192.168.1.100 | 8080 | TCP | Allow |
| 192.168.1.101 | 8080 | TCP | Allow |
| All | 8080 | TCP | Deny |

<!-- Screenshot placeholder: Synology Firewall rules configuration -->

### Reverse Proxy with Synology Web Station

Use Synology's built-in reverse proxy for HTTPS access:

#### Step 1: Enable Web Station

1. Open **Package Center**
2. Install **Web Station** if not already installed

#### Step 2: Configure Reverse Proxy

1. Go to **Control Panel** > **Login Portal** > **Advanced**
2. Click **Reverse Proxy**
3. Click **Create**

**Reverse Proxy Settings:**

| Field | Value |
|-------|-------|
| Description | Nekzus |
| Source Protocol | HTTPS |
| Source Hostname | nexus.local (or your domain) |
| Source Port | 443 |
| Destination Protocol | HTTP |
| Destination Hostname | localhost |
| Destination Port | 8080 |

<!-- Screenshot placeholder: Synology Reverse Proxy configuration -->

4. Click **Save**

#### Step 3: Configure Custom Headers (Optional)

Click on the new proxy rule, then **Custom Header** > **Create** > **WebSocket**

This adds the required headers for WebSocket support:

- `Upgrade: $http_upgrade`
- `Connection: $connection_upgrade`

### HTTPS Setup

#### Option 1: Synology Let's Encrypt Certificate

If you have a domain pointing to your NAS:

1. Go to **Control Panel** > **Security** > **Certificate**
2. Click **Add**
3. Select **Add a new certificate**
4. Select **Get a certificate from Let's Encrypt**
5. Enter your domain and email
6. Apply the certificate to the reverse proxy

#### Option 2: Self-Signed Certificate

For local network use:

1. Go to **Control Panel** > **Security** > **Certificate**
2. Click **Add**
3. Select **Add a new certificate**
4. Select **Create self-signed certificate**
5. Fill in certificate details
6. Apply to the reverse proxy

#### Option 3: Internal TLS with Nekzus

Configure Nekzus to handle TLS directly:

```yaml
services:
  nexus:
    ports:
      - "8443:8443"
    volumes:
      - /volume1/docker/nekzus/certs:/app/certs:ro
    environment:
      NEKZUS_TLS_CERT: "/app/certs/cert.pem"
      NEKZUS_TLS_KEY: "/app/certs/key.pem"
```

Generate certificates:

```bash
openssl req -x509 -newkey rsa:4096 \
  -keyout /volume1/docker/nekzus/certs/key.pem \
  -out /volume1/docker/nekzus/certs/cert.pem \
  -days 365 -nodes \
  -subj "/CN=nekzus"
```

### User Permissions

On Synology, Docker typically runs as root. For enhanced security:

1. Create a dedicated user for Docker:

    ```bash
    sudo synouser --add docker-user "" "" "" "" ""
    ```

2. Add to the docker group:

    ```bash
    sudo synogroup --add docker docker-user
    ```

3. Verify Docker socket permissions:

    ```bash
    ls -la /var/run/docker.sock
    # Should show: srw-rw---- 1 root docker ...
    ```

---

## Troubleshooting

### Container Fails to Start

**Check logs:**

```bash
sudo docker logs nekzus
```

**Common causes:**

??? question "Port already in use"

    Another application is using port 8080.

    **Solution:** Change the host port:
    ```yaml
    ports:
      - "9080:8080"
    ```

    Or find and stop the conflicting service:
    ```bash
    sudo netstat -tlnp | grep 8080
    ```

??? question "Volume permission denied"

    The container cannot write to the data volume.

    **Solution:** Fix permissions:
    ```bash
    sudo chown -R 0:0 /volume1/docker/nekzus/data
    sudo chmod -R 755 /volume1/docker/nekzus/data
    ```

??? question "Invalid JWT secret"

    The JWT secret is too short or contains weak patterns.

    **Solution:** Generate a strong secret:
    ```bash
    openssl rand -base64 32
    ```

### Docker Discovery Not Working

??? question "No containers discovered"

    Docker socket is not mounted or not accessible.

    **Solution 1:** Verify socket mount:
    ```bash
    sudo docker inspect nekzus | grep -A5 "Mounts"
    ```

    **Solution 2:** Check socket permissions:
    ```bash
    ls -la /var/run/docker.sock
    ```

    **Solution 3:** Ensure containers have discovery labels:
    ```yaml
    labels:
      nekzus.enable: "true"
    ```

??? question "Socket permission denied"

    The container cannot access the Docker socket.

    **Solution:** On some Synology models, run the container as root:
    ```yaml
    user: "0:0"
    ```

### Network Connectivity Issues

??? question "Cannot access from other devices"

    **Verify:**

    1. Container is running: `sudo docker ps`
    2. Port is exposed: `sudo netstat -tlnp | grep 8080`
    3. Firewall allows traffic: Check **Control Panel** > **Security** > **Firewall**
    4. Test locally first: `curl http://localhost:8080/healthz`

??? question "WebSocket connections fail"

    If using Synology reverse proxy, WebSocket headers must be configured.

    **Solution:** Add custom headers in reverse proxy settings:

    1. Edit the reverse proxy rule
    2. Go to **Custom Header**
    3. Click **Create** > **WebSocket**

??? question "mDNS discovery not finding services"

    mDNS requires host network mode or special configuration.

    **Solution:** Use host network mode:
    ```yaml
    network_mode: host
    ```

### Performance Issues

??? question "High CPU usage"

    **Check discovery interval:**
    ```yaml
    discovery:
      docker:
        poll_interval: "60s"  # Increase from default 30s
    ```

    **Limit container resources:**
    ```yaml
    deploy:
      resources:
        limits:
          cpus: '1.0'
          memory: 512M
    ```

??? question "Slow response times"

    **Check container resources:**
    ```bash
    sudo docker stats nekzus
    ```

    **Increase memory limit if needed:**
    ```yaml
    deploy:
      resources:
        limits:
          memory: 1G
    ```

### Database Issues

??? question "Database locked error"

    SQLite database is locked by another process.

    **Solution 1:** Restart the container:
    ```bash
    sudo docker restart nekzus
    ```

    **Solution 2:** Check for orphan processes:
    ```bash
    sudo fuser /volume1/docker/nekzus/data/nexus.db
    ```

??? question "Database corruption"

    Restore from automatic backup:

    ```bash
    # List backups
    ls -la /volume1/docker/nekzus/data/backups/

    # Stop container
    sudo docker stop nekzus

    # Restore backup
    cp /volume1/docker/nekzus/data/backups/nexus-backup-YYYYMMDD.db \
       /volume1/docker/nekzus/data/nexus.db

    # Start container
    sudo docker start nekzus
    ```

### Viewing Logs

**Container Manager GUI:**

1. Go to **Container** list
2. Select `nekzus`
3. Click **Details**
4. Go to **Log** tab

**Command Line:**

```bash
# View recent logs
sudo docker logs nekzus

# Follow logs in real-time
sudo docker logs -f nekzus

# View last 100 lines
sudo docker logs --tail 100 nekzus

# View logs with timestamps
sudo docker logs -t nekzus
```

---

## Updates

### Updating via Container Manager (DSM 7.2+)

#### Automatic Updates

1. Go to **Container Manager** > **Registry**
2. Search for `nstalgic/nekzus`
3. Select the image
4. Click **Download** to pull the latest version
5. Go to **Container** list
6. Stop the `nekzus` container
7. Select **Action** > **Reset**
8. Choose the new image version
9. Start the container

#### Using Projects (Compose)

```bash
# Via SSH
cd /volume1/docker/nekzus
sudo docker compose pull
sudo docker compose up -d
```

Or in Container Manager:

1. Go to **Project**
2. Select `nekzus`
3. Click **Action** > **Build**

### Updating via Docker Package (DSM 7.0-7.1)

1. Open **Docker**
2. Go to **Registry**
3. Download the latest `nstalgic/nekzus` image
4. Go to **Container**
5. Stop and delete the old container
6. Create a new container from the updated image

### Updating via SSH

```bash
# Pull the latest image
sudo docker pull nstalgic/nekzus:latest

# Stop and remove the old container
sudo docker stop nekzus
sudo docker rm nekzus

# Start with the new image
sudo docker run -d \
  --name nekzus \
  --restart unless-stopped \
  -p 8080:8080 \
  --env-file /volume1/docker/nekzus/.env \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  -v /volume1/docker/nekzus/data:/app/data \
  nstalgic/nekzus:latest
```

### Update with Docker Compose

```bash
cd /volume1/docker/nekzus

# Pull latest image
sudo docker compose pull

# Recreate container with new image
sudo docker compose up -d

# Remove old images (optional)
sudo docker image prune -f
```

### Version Pinning

For production stability, pin to a specific version:

```yaml
services:
  nexus:
    image: nstalgic/nekzus:v1.2.3  # Specific version
```

Check available versions at [Docker Hub](https://hub.docker.com/r/nstalgic/nekzus/tags) or GitHub releases.

### Backup Before Updating

Always backup before major updates:

```bash
# Stop the container
sudo docker stop nekzus

# Backup data directory
sudo tar -czvf /volume1/docker/backups/nekzus-backup-$(date +%Y%m%d).tar.gz \
  /volume1/docker/nekzus/data

# Proceed with update
sudo docker pull nstalgic/nekzus:latest
```

---

## DSM Integration Tips

### Scheduled Tasks

Create scheduled tasks for maintenance:

1. Go to **Control Panel** > **Task Scheduler**
2. Click **Create** > **Scheduled Task** > **User-defined script**

**Daily backup script:**

```bash
#!/bin/bash
BACKUP_DIR=/volume1/docker/backups
DATE=$(date +%Y%m%d)

# Backup Nekzus data
tar -czvf $BACKUP_DIR/nekzus-$DATE.tar.gz \
  /volume1/docker/nekzus/data

# Keep only last 7 days
find $BACKUP_DIR -name "nekzus-*.tar.gz" -mtime +7 -delete
```

**Weekly cleanup script:**

```bash
#!/bin/bash
# Clean up old Docker images
docker image prune -af --filter "until=168h"
```

### Hyper Backup Integration

Include Nekzus data in Hyper Backup:

1. Open **Hyper Backup**
2. Create or edit a backup task
3. Add `/volume1/docker/nekzus/data` to the backup selection

### Resource Monitor

Monitor Nekzus resource usage:

1. Open **Resource Monitor**
2. Go to **Docker** tab (DSM 7.2+)
3. Find `nekzus` container
4. View CPU, Memory, and Network usage

---

## Next Steps

- [Quick Start Guide](../getting-started/quick-start.md) - Configure your first services
- [Configuration Reference](../reference/configuration.md) - Detailed configuration options
- [Toolbox Guide](../features/toolbox.md) - Deploy services with one click
- [Troubleshooting Guide](../guides/troubleshooting.md) - General troubleshooting
