# Upgrading Nekzus

This guide covers upgrading Nekzus between versions, including backup procedures, migration steps, and rollback strategies.

---

## Upgrade Overview

Nekzus follows semantic versioning (SemVer) for releases:

| Version Type | Format | Description |
|--------------|--------|-------------|
| Major | `X.0.0` | Breaking changes, requires migration |
| Minor | `0.X.0` | New features, backward compatible |
| Patch | `0.0.X` | Bug fixes, backward compatible |

### Version Compatibility Matrix

| From Version | To Version | Migration Required | Notes |
|--------------|------------|-------------------|-------|
| 0.x.x | 1.0.0 | Yes | Breaking changes |
| 1.x.x | 1.y.y | No | Automatic schema updates |
| Any | Same major | No | Safe upgrade path |

!!! info "Current Version"
    Check your current version with:
    ```bash
    ./nekzus --version
    ```
    Or via the API:
    ```bash
    curl -s https://localhost:8443/api/v1/status | jq .version
    ```

---

## Pre-Upgrade Checklist

Before upgrading, complete the following checklist to ensure a smooth upgrade process.

### 1. Backup Your Data

!!! danger "Critical Step"
    Always create a full backup before upgrading. This is your safety net for rollback.

=== "Docker Deployment"

    ```bash
    # Stop the container gracefully
    docker compose stop nexus

    # Create timestamped backup directory
    BACKUP_DIR="./backups/$(date +%Y%m%d_%H%M%S)"
    mkdir -p "$BACKUP_DIR"

    # Backup database
    cp ./data/nexus.db "$BACKUP_DIR/"

    # Backup configuration
    cp ./configs/config.yaml "$BACKUP_DIR/"

    # Backup TLS certificates (if using custom certs)
    cp -r ./certs "$BACKUP_DIR/" 2>/dev/null || true

    # Backup toolbox data
    cp -r ./data/toolbox "$BACKUP_DIR/" 2>/dev/null || true

    echo "Backup created at: $BACKUP_DIR"
    ```

=== "Binary Deployment"

    ```bash
    # Stop the service
    sudo systemctl stop nekzus

    # Create timestamped backup directory
    BACKUP_DIR="/opt/nekzus/backups/$(date +%Y%m%d_%H%M%S)"
    sudo mkdir -p "$BACKUP_DIR"

    # Backup database
    sudo cp /opt/nekzus/data/nexus.db "$BACKUP_DIR/"

    # Backup configuration
    sudo cp /opt/nekzus/configs/config.yaml "$BACKUP_DIR/"

    # Backup TLS certificates
    sudo cp -r /opt/nekzus/certs "$BACKUP_DIR/" 2>/dev/null || true

    # Backup toolbox data
    sudo cp -r /opt/nekzus/data/toolbox "$BACKUP_DIR/" 2>/dev/null || true

    echo "Backup created at: $BACKUP_DIR"
    ```

=== "Kubernetes Deployment"

    ```bash
    # Scale down deployment
    kubectl scale deployment nekzus --replicas=0 -n nekzus

    # Wait for pods to terminate
    kubectl wait --for=delete pod -l app=nekzus -n nekzus --timeout=60s

    # Backup PersistentVolumeClaim data
    # Option 1: Use volume snapshot (if supported)
    cat <<EOF | kubectl apply -f -
    apiVersion: snapshot.storage.k8s.io/v1
    kind: VolumeSnapshot
    metadata:
      name: nekzus-backup-$(date +%Y%m%d%H%M%S)
      namespace: nekzus
    spec:
      volumeSnapshotClassName: your-snapshot-class
      source:
        persistentVolumeClaimName: nekzus-data
    EOF

    # Option 2: Copy data from a temporary pod
    kubectl run backup-pod --image=busybox --restart=Never -n nekzus \
      --overrides='{"spec":{"containers":[{"name":"backup","image":"busybox","command":["sleep","3600"],"volumeMounts":[{"name":"data","mountPath":"/data"}]}],"volumes":[{"name":"data","persistentVolumeClaim":{"claimName":"nekzus-data"}}]}}'

    kubectl wait --for=condition=Ready pod/backup-pod -n nekzus
    kubectl cp nekzus/backup-pod:/data ./backup-data
    kubectl delete pod backup-pod -n nekzus
    ```

### 2. Check Compatibility

Before upgrading, verify:

- [ ] Read the release notes for breaking changes
- [ ] Check configuration file compatibility
- [ ] Verify Go version requirements (Go 1.25+ required)
- [ ] Review deprecated features that may be removed
- [ ] Test upgrade in a staging environment first

### 3. Verify System Requirements

| Requirement | Minimum | Recommended |
|-------------|---------|-------------|
| CPU | 1 core | 2+ cores |
| RAM | 256 MB | 512 MB+ |
| Disk | 100 MB + data | 1 GB+ |
| Go (source build) | 1.25 | Latest |
| Docker | 20.10+ | Latest |

### 4. Document Current State

```bash
# Save current configuration state
./nekzus -config configs/config.yaml -validate 2>&1 > upgrade-precheck.log

# Record current routes and apps
curl -s https://localhost:8443/api/v1/routes > routes-backup.json
curl -s https://localhost:8443/api/v1/apps > apps-backup.json
curl -s https://localhost:8443/api/v1/devices > devices-backup.json
```

---

## Docker Upgrades

### Standard Upgrade (Minor/Patch)

=== "Docker Compose"

    ```bash
    # Pull the latest image
    docker compose pull

    # Recreate container with new image
    docker compose up -d

    # Verify upgrade
    docker compose logs -f nexus
    ```

=== "Docker CLI"

    ```bash
    # Pull new image
    docker pull ghcr.io/nstalgic/nekzus:latest

    # Stop existing container
    docker stop nekzus

    # Remove old container (data is preserved in volumes)
    docker rm nekzus

    # Start with new image
    docker run -d \
      --name nekzus \
      -p 8443:8443 \
      -v nekzus-data:/app/data \
      -v nekzus-config:/app/configs \
      -v /var/run/docker.sock:/var/run/docker.sock:ro \
      ghcr.io/nstalgic/nekzus:latest
    ```

### Major Version Upgrade

For major version upgrades (e.g., v1.x to v2.x), follow these additional steps:

```bash
# 1. Stop the current container
docker compose stop nexus

# 2. Create a full backup (see Pre-Upgrade Checklist)
BACKUP_DIR="./backups/major-upgrade-$(date +%Y%m%d)"
mkdir -p "$BACKUP_DIR"
cp -r ./data "$BACKUP_DIR/"
cp -r ./configs "$BACKUP_DIR/"

# 3. Review breaking changes in CHANGELOG
# Read: https://github.com/nstalgic/nekzus/releases

# 4. Update configuration file (if required)
# See Configuration Changes section below

# 5. Pull new major version
docker compose pull

# 6. Run database migrations (automatic on startup)
docker compose up -d

# 7. Monitor logs for migration status
docker compose logs -f nexus | head -100

# 8. Verify all routes and apps are accessible
curl -s https://localhost:8443/api/v1/healthz
```

### Pinning to Specific Versions

For production environments, pin to specific versions instead of `latest`:

```yaml title="docker-compose.yml"
services:
  nexus:
    image: ghcr.io/nstalgic/nekzus:v1.2.3  # Pin to specific version
    # ... rest of configuration
```

### Handling Breaking Changes

When Docker image changes include breaking configuration:

```bash
# Compare configuration schemas
docker run --rm ghcr.io/nstalgic/nekzus:v1.0.0 -config-schema > old-schema.json
docker run --rm ghcr.io/nstalgic/nekzus:v2.0.0 -config-schema > new-schema.json

# Use diff to identify changes
diff old-schema.json new-schema.json
```

---

## Binary Upgrades

### From Pre-built Binaries

=== "Linux (amd64)"

    ```bash
    # Download new version
    VERSION="v1.2.3"
    curl -LO "https://github.com/nstalgic/nekzus/releases/download/${VERSION}/nekzus_linux_amd64.tar.gz"

    # Stop current service
    sudo systemctl stop nekzus

    # Backup current binary
    sudo cp /usr/local/bin/nekzus /usr/local/bin/nekzus.bak

    # Extract and install new binary
    tar -xzf nekzus_linux_amd64.tar.gz
    sudo mv nekzus /usr/local/bin/

    # Verify installation
    nekzus --version

    # Start service
    sudo systemctl start nekzus
    sudo systemctl status nekzus
    ```

=== "macOS (arm64)"

    ```bash
    # Download new version
    VERSION="v1.2.3"
    curl -LO "https://github.com/nstalgic/nekzus/releases/download/${VERSION}/nekzus_darwin_arm64.tar.gz"

    # Stop current process
    pkill nekzus

    # Backup current binary
    cp /usr/local/bin/nekzus /usr/local/bin/nekzus.bak

    # Extract and install
    tar -xzf nekzus_darwin_arm64.tar.gz
    sudo mv nekzus /usr/local/bin/

    # Verify and start
    nekzus --version
    nekzus -config /path/to/config.yaml &
    ```

=== "Windows"

    ```powershell
    # Download new version
    $VERSION = "v1.2.3"
    Invoke-WebRequest -Uri "https://github.com/nstalgic/nekzus/releases/download/$VERSION/nekzus_windows_amd64.zip" -OutFile "nekzus.zip"

    # Stop current service
    Stop-Service nekzus

    # Backup current binary
    Copy-Item "C:\Program Files\nekzus\nekzus.exe" "C:\Program Files\nekzus\nekzus.exe.bak"

    # Extract and install
    Expand-Archive -Path "nekzus.zip" -DestinationPath "C:\Program Files\nekzus" -Force

    # Start service
    Start-Service nekzus
    ```

### Building from Source

```bash
# Clone or update repository
git clone https://github.com/nstalgic/nekzus.git
cd nekzus

# Checkout specific version
git fetch --all --tags
git checkout v1.2.3

# Build with version information
go build -ldflags="-s -w -X main.version=v1.2.3" -o nekzus ./cmd/nekzus

# Verify build
./nekzus --version
```

!!! tip "Build Requirements"
    Building from source requires:

    - Go 1.25 or later
    - GCC (for CGO/SQLite support)
    - Node.js 20+ (for web UI)

    Install dependencies on Debian/Ubuntu:
    ```bash
    sudo apt-get install gcc libc6-dev libsqlite3-dev
    ```

---

## Database Migrations

Nekzus uses SQLite with automatic schema migrations. Migrations run automatically on startup.

### How Migrations Work

The database schema is managed through idempotent migrations in `internal/storage/storage.go`:

1. **Table Creation**: `CREATE TABLE IF NOT EXISTS` ensures tables are created only if missing
2. **Index Creation**: `CREATE INDEX IF NOT EXISTS` for performance optimization

### Migration Process

```
Startup
   |
   v
Open Database
   |
   v
Run Migrations (in order)
   |
   +-- Create core tables (apps, routes, devices, proposals)
   +-- Create health tables (service_health)
   +-- Create activity tables (activity_events)
   +-- Create certificate tables (certificates, certificate_history)
   +-- Create API key tables (api_keys)
   +-- Create notification tables (notifications)
   +-- Create toolbox tables (toolbox_deployments)
   +-- Create federation tables (federation_peers, federation_services)
   +-- Create script tables (scripts, script_executions, workflows)
   +-- Add new columns to existing tables (if needed)
   +-- Create indexes
   |
   v
Application Ready
```

### Current Schema Tables

| Table | Purpose | Added In |
|-------|---------|----------|
| `apps` | Application catalog | v0.1.0 |
| `routes` | Proxy route definitions | v0.1.0 |
| `devices` | Paired mobile devices | v0.1.0 |
| `proposals` | Discovery proposals | v0.1.0 |
| `service_health` | Service health status | v0.2.0 |
| `activity_events` | Activity feed | v0.3.0 |
| `certificates` | TLS certificate storage | v0.4.0 |
| `certificate_history` | Certificate audit log | v0.4.0 |
| `api_keys` | API key management | v0.5.0 |
| `notifications` | Push notifications | v0.6.0 |
| `request_logs` | Request analytics | v0.6.0 |
| `audit_logs` | Security audit trail | v0.6.0 |
| `toolbox_deployments` | Deployed services | v0.7.0 |
| `federation_peers` | Federation cluster peers | v0.8.0 |
| `federation_services` | Federated services | v0.8.0 |
| `system_secrets` | Auto-generated secrets | v0.9.0 |
| `proxy_session_cookies` | WebView cookie persistence | v0.10.0 |
| `scripts` | Script definitions | v0.11.0 |
| `script_executions` | Script execution history | v0.11.0 |
| `workflows` | Workflow definitions | v0.11.0 |
| `workflow_executions` | Workflow execution history | v0.11.0 |
| `script_schedules` | Scheduled script runs | v0.11.0 |

### Checking Migration Status

```bash
# View current schema
sqlite3 ./data/nexus.db ".schema"

# List all tables
sqlite3 ./data/nexus.db ".tables"

# Check specific table schema
sqlite3 ./data/nexus.db ".schema devices"

# View row counts
sqlite3 ./data/nexus.db "SELECT 'apps', COUNT(*) FROM apps UNION ALL SELECT 'routes', COUNT(*) FROM routes UNION ALL SELECT 'devices', COUNT(*) FROM devices;"
```

### Manual Recovery (Emergency)

If automatic migration fails, you can inspect and fix the database manually:

```bash
# Backup first
cp ./data/nexus.db ./data/nexus.db.backup

# Connect to database
sqlite3 ./data/nexus.db

# View current schema
.schema

# Check for issues
PRAGMA integrity_check;

# Exit
.quit
```

!!! warning "Schema Compatibility"
    Manual schema changes may cause issues with future upgrades. Only use manual migrations as a last resort and document any changes made.

---

## Configuration Changes

### Reviewing Configuration Changes

When upgrading, compare your configuration against the example:

```bash
# Download latest example config
curl -O https://raw.githubusercontent.com/nstalgic/nekzus/main/configs/config.example.yaml

# Compare with your config
diff -u configs/config.yaml configs/config.example.yaml
```

### Deprecated Options

The following configuration options are deprecated and will be removed in future versions:

| Deprecated Option | Replacement | Removed In |
|-------------------|-------------|------------|
| `toolbox.catalog_path` | `toolbox.catalog_dir` | v2.0.0 |

??? warning "Deprecated: toolbox.catalog_path"
    The YAML-based catalog (`catalog_path`) is deprecated in favor of Compose-based catalogs (`catalog_dir`).

    **Before (deprecated):**
    ```yaml
    toolbox:
      enabled: true
      catalog_path: "./configs/toolbox-catalog.yaml"
    ```

    **After (recommended):**
    ```yaml
    toolbox:
      enabled: true
      catalog_dir: "./toolbox"
    ```

    See the [Toolbox documentation](../features/toolbox.md) for migration details.

### New Configuration Options

When upgrading, review new options that may enhance your deployment:

=== "v0.11.0+ Scripts"

    ```yaml
    # Script execution (new in v0.11.0)
    scripts:
      enabled: false
      directory: "./scripts"
      default_timeout: 300
      max_output_bytes: 10485760
    ```

=== "v0.10.0+ Cookie Persistence"

    ```yaml
    # Route-level cookie persistence (new in v0.10.0)
    routes:
      - id: "route:app"
        path_base: "/apps/myapp/"
        to: "http://myapp:8080"
        persist_cookies: true  # NEW: Persist cookies for mobile WebView
    ```

=== "v0.9.0+ Auto JWT Secret"

    ```yaml
    # JWT secret is now optional - auto-generated if not provided
    auth:
      issuer: "nekzus"
      audience: "nekzus-mobile"
      # hs256_secret: ""  # Optional: auto-generated and stored in DB
    ```

=== "v0.8.0+ Federation"

    ```yaml
    # Federation for multi-instance deployments
    federation:
      enabled: false
      cluster_secret: ""
      gossip_port: 7946
      mdns_enabled: true
      bootstrap_peers: []
      sync:
        full_sync_interval: "300s"
        anti_entropy_period: "60s"
      allow_remote_routes: false
    ```

### Environment Variable Changes

| Variable | Status | Notes |
|----------|--------|-------|
| `NEKZUS_JWT_SECRET` | Current | JWT signing secret |
| `NEKZUS_BOOTSTRAP_TOKEN` | Current | Bootstrap token |
| `NEKZUS_DATABASE_PATH` | Current | Database path |
| `NEKZUS_TOOLBOX_HOST_DATA_DIR` | Current | Host data directory |
| `NEKZUS_HOST_ROOT_PATH` | Current | Host root for metrics |
| `ENVIRONMENT` | Current | Set to `production` for strict validation |

---

## Rollback Procedures

If an upgrade fails, follow these rollback procedures.

### Docker Rollback

=== "Using Previous Image Tag"

    ```bash
    # Stop current container
    docker compose stop nexus

    # Edit docker-compose.yml to use previous version
    # Change: image: ghcr.io/nstalgic/nekzus:latest
    # To:     image: ghcr.io/nstalgic/nekzus:v1.1.0

    # Start with previous version
    docker compose up -d

    # Verify rollback
    docker compose logs nexus | head -20
    ```

=== "Using Backup"

    ```bash
    # Stop current container
    docker compose down

    # Restore backup
    BACKUP_DIR="./backups/20240101_120000"
    cp "$BACKUP_DIR/nexus.db" ./data/
    cp "$BACKUP_DIR/config.yaml" ./configs/

    # Start with previous version image
    docker compose up -d
    ```

### Binary Rollback

```bash
# Stop service
sudo systemctl stop nekzus

# Restore backup binary
sudo cp /usr/local/bin/nekzus.bak /usr/local/bin/nekzus

# Restore backup database
BACKUP_DIR="/opt/nekzus/backups/20240101_120000"
sudo cp "$BACKUP_DIR/nexus.db" /opt/nekzus/data/

# Restore configuration
sudo cp "$BACKUP_DIR/config.yaml" /opt/nekzus/configs/

# Start service
sudo systemctl start nekzus
```

### Database Rollback

If only the database needs rollback:

```bash
# Stop service
docker compose stop nexus  # or: sudo systemctl stop nekzus

# Restore database from backup
cp ./backups/20240101_120000/nexus.db ./data/nexus.db

# Start service
docker compose up -d  # or: sudo systemctl start nekzus
```

!!! danger "Schema Downgrade Warning"
    Rolling back to an older version after schema migrations may cause data loss or application errors. Always test rollbacks in a staging environment first.

---

## Breaking Changes by Version

### v1.0.0 (Upcoming)

!!! danger "Major Release"
    Version 1.0.0 will include breaking changes. Migration guides will be provided.

**Planned Breaking Changes:**

- Removal of deprecated `toolbox.catalog_path` option
- API response format changes for consistency
- Configuration schema updates

### Current Development (v0.x)

No breaking changes in patch releases. Minor releases may add new features but maintain backward compatibility.

---

## Post-Upgrade Verification

After upgrading, verify the installation is working correctly.

### Health Checks

```bash
# Check overall health
curl -s https://localhost:8443/api/v1/healthz | jq .

# Expected output:
# {
#   "status": "healthy",
#   "version": "v1.2.3",
#   "uptime": "1m30s",
#   "checks": {
#     "database": "healthy",
#     "discovery": "healthy",
#     ...
#   }
# }
```

### API Verification

```bash
# Verify routes are accessible
curl -s https://localhost:8443/api/v1/routes | jq 'length'

# Verify apps are accessible
curl -s https://localhost:8443/api/v1/apps | jq 'length'

# Verify devices are preserved
curl -s https://localhost:8443/api/v1/devices | jq 'length'

# Test proxy functionality
curl -I https://localhost:8443/apps/your-app/
```

### Service Discovery

```bash
# Check discovery is running
curl -s https://localhost:8443/api/v1/proposals | jq 'length'

# Verify Docker discovery (if enabled)
docker ps  # Compare with discovered services
```

### Metrics Verification

```bash
# Check Prometheus metrics endpoint
curl -s https://localhost:8443/metrics | head -20

# Verify key metrics are present
curl -s https://localhost:8443/metrics | grep "nekzus_http_requests_total"
```

### WebSocket Verification

```javascript
// Test WebSocket connection (browser console or wscat)
const ws = new WebSocket('wss://localhost:8443/ws');
ws.onopen = () => console.log('Connected');
ws.onmessage = (e) => console.log('Message:', e.data);
ws.onerror = (e) => console.error('Error:', e);
```

### Log Analysis

```bash
# Check for errors in logs
docker compose logs nexus 2>&1 | grep -i error

# Check for migration completion
docker compose logs nexus 2>&1 | grep -i migration

# Check for warnings
docker compose logs nexus 2>&1 | grep -i warning
```

---

## Upgrade Automation

For automated deployments, consider these patterns:

### CI/CD Pipeline Example

```yaml title=".github/workflows/upgrade.yml"
name: Upgrade Nekzus

on:
  workflow_dispatch:
    inputs:
      version:
        description: 'Version to upgrade to'
        required: true
        default: 'latest'

jobs:
  upgrade:
    runs-on: self-hosted
    steps:
      - name: Backup
        run: |
          BACKUP_DIR="/opt/backups/$(date +%Y%m%d_%H%M%S)"
          mkdir -p "$BACKUP_DIR"
          cp /opt/nekzus/data/nexus.db "$BACKUP_DIR/"
          cp /opt/nekzus/configs/config.yaml "$BACKUP_DIR/"

      - name: Pull new image
        run: docker pull ghcr.io/nstalgic/nekzus:${{ inputs.version }}

      - name: Upgrade
        run: |
          cd /opt/nekzus
          docker compose up -d

      - name: Verify
        run: |
          sleep 10
          curl -f https://localhost:8443/api/v1/healthz || exit 1

      - name: Rollback on failure
        if: failure()
        run: |
          cd /opt/nekzus
          docker compose down
          # Restore from backup
          cp /opt/backups/$(ls -t /opt/backups | head -1)/nexus.db /opt/nekzus/data/
          docker compose up -d
```

### Pre-Upgrade Script

```bash title="scripts/pre-upgrade.sh"
#!/bin/bash
set -euo pipefail

# Pre-upgrade checks and backup
BACKUP_DIR="./backups/pre-upgrade-$(date +%Y%m%d_%H%M%S)"

echo "Creating backup at $BACKUP_DIR..."
mkdir -p "$BACKUP_DIR"

# Backup data
cp ./data/nexus.db "$BACKUP_DIR/" 2>/dev/null || true
cp ./configs/config.yaml "$BACKUP_DIR/" 2>/dev/null || true
cp -r ./certs "$BACKUP_DIR/" 2>/dev/null || true

# Export current state
curl -sf https://localhost:8443/api/v1/routes > "$BACKUP_DIR/routes.json" || true
curl -sf https://localhost:8443/api/v1/apps > "$BACKUP_DIR/apps.json" || true

# Health check
if curl -sf https://localhost:8443/api/v1/healthz > /dev/null; then
    echo "Current instance is healthy"
else
    echo "WARNING: Current instance may not be healthy"
fi

echo "Pre-upgrade backup complete: $BACKUP_DIR"
```

---

## Getting Help

If you encounter issues during upgrade:

1. **Check Logs**: Review application logs for error messages
2. **GitHub Issues**: Search or open an issue at [github.com/nstalgic/nekzus/issues](https://github.com/nstalgic/nekzus/issues)
3. **Rollback**: Use the rollback procedures if the upgrade fails
4. **Community**: Join discussions in GitHub Discussions

!!! tip "Include Version Information"
    When reporting upgrade issues, include:

    - Previous version
    - Target version
    - Error messages from logs
    - Configuration file (sanitized)
    - Platform (Docker, Linux, macOS, etc.)
