import Tabs from "@theme/Tabs";
import TabItem from "@theme/TabItem";

# Configuration

This guide covers all configuration options for Nekzus. Configuration can be provided via YAML files, environment variables, or a combination of both.

## Configuration File

Nekzus uses YAML configuration files. The default location is `configs/config.yaml`, but you can specify a custom path using the `-config` flag:

```bash
./nekzus -config /path/to/config.yaml
```

### File Format

Both YAML and JSON formats are supported. The parser automatically detects the format based on file extension:

| Extension | Format |
|-----------|--------|
| `.yaml`, `.yml` | YAML |
| `.json` | JSON |

:::tip[YAML is Recommended]

YAML is the preferred format for its readability and support for comments. All examples in this documentation use YAML.

:::


---

## Configuration Sections

### Server

Configure the HTTP server, listening address, and TLS certificates.

```yaml title="config.yaml"
server:
  # Listen address (host:port or :port)
  addr: ":8080"

  # Base URL for QR code pairing and external access
  # Leave empty for auto-detection of local network IP
  base_url: ""

  # TLS certificate paths
  # Leave empty to run behind a reverse proxy or use -insecure-http flag
  tls_cert: ""
  tls_key: ""
```

#### TLS Options

<Tabs>
<TabItem value="option-1-behind-reverse-proxy" label="Option 1: Behind Reverse Proxy">


Leave TLS settings empty when running behind Caddy, nginx, or Traefik:

```yaml
server:
  addr: ":8080"
  tls_cert: ""
  tls_key: ""
```

</TabItem>
<TabItem value="option-2-auto-generated-certificates" label="Option 2: Auto-Generated Certificates">


Specify paths for auto-generated self-signed certificates:

```yaml
server:
  addr: ":8443"
  tls_cert: "./certs/server-cert.pem"
  tls_key: "./certs/server-key.pem"
```

If the files do not exist, Nekzus generates them on startup.

</TabItem>
<TabItem value="option-3-custom-certificates" label="Option 3: Custom Certificates">


Provide your own certificates (e.g., from Let's Encrypt):

```yaml
server:
  addr: ":8443"
  tls_cert: "/etc/letsencrypt/live/example.com/fullchain.pem"
  tls_key: "/etc/letsencrypt/live/example.com/privkey.pem"
```

</TabItem>
</Tabs>

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `addr` | string | `:8443` | Server listen address |
| `base_url` | string | auto-detect | Base URL for external access |
| `tls_cert` | string | `""` | Path to TLS certificate |
| `tls_key` | string | `""` | Path to TLS private key |
| `http_redirect_addr` | string | `""` | Address for HTTP to HTTPS redirect |

---

### Authentication

Configure JWT authentication and token settings.

```yaml title="config.yaml"
auth:
  # JWT issuer claim
  issuer: "nekzus"

  # JWT audience claim
  audience: "nekzus-mobile"

  # JWT signing secret (32+ characters required)
  # STRONGLY RECOMMENDED: Use NEKZUS_JWT_SECRET environment variable instead
  hs256_secret: ""

  # Default scopes applied to routes without explicit scopes
  default_scopes: []
```

:::warning[JWT Secret Security]

- Must be at least 32 characters
- Avoid weak patterns like "dev", "test", "example" in production
- Use environment variable `NEKZUS_JWT_SECRET` for sensitive deployments
- If not provided, a secure secret is auto-generated on startup

:::


| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `issuer` | string | `nekzus` | JWT issuer claim |
| `audience` | string | `nekzus-mobile` | JWT audience claim |
| `hs256_secret` | string | auto-generate | JWT signing secret (32+ chars) |
| `default_scopes` | []string | `[]` | Default scopes for routes |

---

### Bootstrap Tokens

Configure initial authentication tokens for device pairing.

```yaml title="config.yaml"
bootstrap:
  # List of valid bootstrap tokens
  tokens:
    - "your-secure-bootstrap-token"
```

:::info[Bootstrap Token Behavior]

- Tokens are optional; if not provided, devices can pair using the QR code flow
- Tokens can be added via the `NEKZUS_BOOTSTRAP_TOKEN` environment variable
- Multiple tokens are supported for different devices or purposes

:::


| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `tokens` | []string | `[]` | Valid bootstrap tokens |

---

### Storage

Configure the SQLite database location.

```yaml title="config.yaml"
storage:
  # Path to SQLite database file
  database_path: "./data/nexus.db"
```

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `database_path` | string | `./data/nexus.db` | SQLite database file path |

---

### Discovery

Configure automatic service discovery from Docker, mDNS, and Kubernetes.

```yaml title="config.yaml"
discovery:
  enabled: true

  # Docker container discovery
  docker:
    enabled: true
    socket_path: "unix:///var/run/docker.sock"
    poll_interval: "30s"
    network_mode: "all"  # all, first, or preferred
    networks: []         # Specific networks (for preferred mode)
    exclude_networks: [] # Networks to ignore

  # mDNS/Bonjour discovery
  mdns:
    enabled: true
    scan_interval: "60s"
    services:
      - "_http._tcp"
      - "_https._tcp"
      - "_homeassistant._tcp"

  # Kubernetes service discovery
  kubernetes:
    enabled: false
    kubeconfig: ""       # Path to kubeconfig (empty for in-cluster)
    namespaces:          # Namespaces to watch (empty for all)
      - "default"
      - "apps"
    poll_interval: "30s"
```

#### Docker Discovery

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | bool | `false` | Enable Docker discovery |
| `socket_path` | string | `unix:///var/run/docker.sock` | Docker socket path |
| `poll_interval` | duration | `30s` | Poll interval |
| `network_mode` | string | `all` | Network selection mode |
| `networks` | []string | `[]` | Networks to scan (for preferred mode) |
| `exclude_networks` | []string | `[]` | Networks to exclude |

**Network Modes:**

| Mode | Description |
|------|-------------|
| `all` | Discover services on all container networks |
| `first` | Use only the first network found |
| `preferred` | Use networks from the `networks` list, fallback to first |

#### mDNS Discovery

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | bool | `false` | Enable mDNS discovery |
| `scan_interval` | duration | `60s` | Scan interval |
| `services` | []string | `["_http._tcp", "_https._tcp"]` | Service types to discover |

#### Kubernetes Discovery

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | bool | `false` | Enable Kubernetes discovery |
| `kubeconfig` | string | `""` | Path to kubeconfig (empty for in-cluster) |
| `namespaces` | []string | `[]` | Namespaces to watch (empty for all) |
| `poll_interval` | duration | `30s` | Poll interval |

:::tip[Kubernetes Auto-Discovery]

Nekzus uses smart label inference to auto-discover services:

- **Explicit labels**: Services with `nekzus.enable=true` (confidence: 0.95)
- **Ingress-exposed**: Services via Kubernetes Ingress (confidence: 1.00)
- **LoadBalancer/NodePort**: Services with standard K8s labels (confidence: 0.70)
- **Helm charts**: Services with Helm labels (confidence: 0.60)

:::


---

### Toolbox

Configure the one-click service deployment system.

```yaml title="config.yaml"
toolbox:
  enabled: true

  # Directory containing Compose-based service templates
  catalog_dir: "./toolbox"

  # Directory for persistent service data
  data_dir: "./data/toolbox"

  # Host path for Docker-in-Docker scenarios
  host_data_dir: ""

  # Automatically create routes for deployed services
  auto_route: true

  # Automatically start containers after deployment
  auto_start: true
```

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | bool | `true` | Enable toolbox feature |
| `catalog_dir` | string | `./toolbox` | Compose templates directory |
| `data_dir` | string | `./data/toolbox` | Service data directory |
| `host_data_dir` | string | `""` | Host path for DinD bind mounts |
| `auto_route` | bool | `true` | Auto-create routes |
| `auto_start` | bool | `true` | Auto-start containers |

:::warning[Deprecated Option]

`catalog_path` (YAML-based catalog) is deprecated. Use `catalog_dir` for Compose-based templates.

:::


---

### Federation

Configure peer-to-peer federation for multi-instance deployments.

```yaml title="config.yaml"
federation:
  enabled: false

  # Shared secret for peer authentication (32+ characters)
  # Generate with: openssl rand -base64 32
  cluster_secret: ""

  # Gossip protocol port (TCP/UDP)
  gossip_port: 7946

  # Enable mDNS peer discovery
  mdns_enabled: true

  # Bootstrap peers for initial cluster formation
  bootstrap_peers: []
    # - "192.168.1.101:7946"
    # - "homelab-server-2:7946"

  # Synchronization settings
  sync:
    full_sync_interval: "300s"  # 5 minutes
    anti_entropy_period: "60s"  # 1 minute

  # DANGER: Allow proxying to remote peer services
  allow_remote_routes: false
```

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | bool | `false` | Enable federation |
| `cluster_secret` | string | `""` | Shared peer authentication secret |
| `gossip_port` | int | `7946` | Gossip protocol port |
| `mdns_enabled` | bool | `true` | Enable mDNS peer discovery |
| `bootstrap_peers` | []string | `[]` | Initial peer addresses |
| `sync.full_sync_interval` | duration | `300s` | Full catalog sync interval |
| `sync.anti_entropy_period` | duration | `60s` | Anti-entropy repair interval |
| `allow_remote_routes` | bool | `false` | Allow remote route proxying |

:::danger[Security Warning]

Setting `allow_remote_routes: true` allows proxying to any service discovered by federated peers. Only enable this in trusted network environments.

:::


---

### Runtimes

Configure container runtime settings for management operations.

```yaml title="config.yaml"
runtimes:
  # Primary runtime: docker or kubernetes
  primary: "docker"

  # Docker runtime settings
  docker:
    enabled: true
    socket_path: ""  # Empty for default

  # Kubernetes runtime settings
  kubernetes:
    enabled: false
    kubeconfig: ""           # Path to kubeconfig
    context: ""              # Kubernetes context
    namespaces: []           # Namespaces to manage
    metrics_server: true     # Use Metrics Server for stats
    metrics_cache_ttl: "30s" # Cache TTL for metrics
```

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `primary` | string | `docker` | Primary runtime (`docker` or `kubernetes`) |
| `docker.enabled` | bool | `true` | Enable Docker runtime |
| `docker.socket_path` | string | `unix:///var/run/docker.sock` | Docker socket path |
| `kubernetes.enabled` | bool | `false` | Enable Kubernetes runtime |
| `kubernetes.kubeconfig` | string | `""` | Kubeconfig path |
| `kubernetes.context` | string | `""` | Kubernetes context |
| `kubernetes.namespaces` | []string | `[]` | Namespaces to manage |
| `kubernetes.metrics_server` | bool | `true` | Use Metrics Server |
| `kubernetes.metrics_cache_ttl` | duration | `30s` | Metrics cache TTL |

---

### Metrics

Configure Prometheus metrics endpoint.

```yaml title="config.yaml"
metrics:
  enabled: true
  path: "/metrics"
```

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | bool | `true` | Enable metrics endpoint |
| `path` | string | `/metrics` | Metrics endpoint path |

:::note[Metrics Collection]

- Metrics are always collected internally for observability
- This setting only controls the HTTP endpoint exposure
- The `enabled` flag can be hot-reloaded without restart
- The `path` cannot be changed without restart

:::


---

### Health Checks

Configure service health monitoring.

```yaml title="config.yaml"
health_checks:
  enabled: true
  interval: "30s"          # Check frequency
  timeout: "5s"            # Request timeout
  unhealthy_threshold: 3   # Failures before unhealthy
  path: "/"                # Default health check path

  # Per-service overrides
  per_service:
    grafana:
      path: "/api/health"
      interval: "30s"
      timeout: "5s"
    homeassistant:
      path: "/api/"
      interval: "60s"
```

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | bool | `true` | Enable health checks |
| `interval` | duration | `10s` | Default check interval |
| `timeout` | duration | `5s` | Default request timeout |
| `unhealthy_threshold` | int | `2` | Consecutive failures threshold |
| `path` | string | `/` | Default health check path |
| `per_service` | map | `{}` | Per-service configuration overrides |

---

### Notifications

Configure WebSocket notification system for mobile devices.

```yaml title="config.yaml"
notifications:
  enabled: false

  # Notification queue settings
  queue:
    worker_count: 4
    buffer_size: 1000
    retry_interval: "30s"
    max_retries: 3

  # Offline device detection
  offline_detection:
    enabled: false
    check_interval: "1m"
    offline_threshold: "5m"
```

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | bool | `false` | Enable notifications |
| `queue.worker_count` | int | `4` | Worker goroutines |
| `queue.buffer_size` | int | `1000` | Channel buffer size |
| `queue.retry_interval` | duration | `30s` | Retry interval |
| `queue.max_retries` | int | `3` | Maximum retries |
| `offline_detection.enabled` | bool | `false` | Enable offline detection |
| `offline_detection.check_interval` | duration | `1m` | Check frequency |
| `offline_detection.offline_threshold` | duration | `5m` | Offline threshold |

---

### Backup

Configure automatic database backups.

```yaml title="config.yaml"
backup:
  enabled: true
  directory: "./data/backups"
  schedule: "24h"      # Backup interval
  retention: 7         # Backups to keep
```

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | bool | `true` | Enable automatic backups |
| `directory` | string | `./data/backups` | Backup directory |
| `schedule` | duration | `24h` | Backup interval |
| `retention` | int | `7` | Number of backups to retain |

---

### Scripts

Configure user script execution.

```yaml title="config.yaml"
scripts:
  enabled: false
  directory: "./scripts"
  default_timeout: 300      # seconds
  max_output_bytes: 10485760  # 10MB
```

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | bool | `false` | Enable script execution |
| `directory` | string | `./scripts` | Scripts directory |
| `default_timeout` | int | `300` | Default timeout (seconds) |
| `max_output_bytes` | int | `10485760` | Max output size (bytes) |

---

### System

Configure system-level settings for host metrics collection.

```yaml title="config.yaml"
system:
  host_root_path: "/mnt/host"  # Mount point for host filesystem
```

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `host_root_path` | string | `""` | Path to mounted host root filesystem for host metrics |

:::tip[Container Deployments]

When running in a container, set `host_root_path` to the mount point of the host filesystem (e.g., `/mnt/host`) to report accurate host system metrics (CPU, RAM, disk) instead of container metrics.

:::


---

### Routes and Apps

Define static routes and applications.

```yaml title="config.yaml"
routes:
  - id: "route:grafana"
    app_id: "grafana"
    path_base: "/apps/grafana/"
    to: "http://127.0.0.1:3000"
    scopes: ["access:grafana"]
    websockets: true
    strip_prefix: true

apps:
  - id: "grafana"
    name: "Grafana"
    icon: "chart"
    tags: ["monitoring", "dashboards"]
    endpoints:
      lan: "http://127.0.0.1:3000"
```

#### Route Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `id` | string | required | Unique route identifier |
| `app_id` | string | required | Associated app ID |
| `path_base` | string | required | URL path prefix (must start with `/`) |
| `to` | string | required | Upstream service URL |
| `scopes` | []string | `[]` | Required authorization scopes |
| `public_access` | bool | `false` | Bypass authentication |
| `websockets` | bool | `false` | Enable WebSocket proxying |
| `strip_prefix` | bool | `false` | Strip path prefix before proxying |
| `strip_response_cookies` | bool | `false` | Remove Set-Cookie headers |
| `rewrite_cookie_paths` | bool | `false` | Rewrite cookie paths |
| `rewrite_html` | bool | `false` | Rewrite HTML for SPA assets |
| `persist_cookies` | bool | `false` | Persist cookies for mobile |
| `health_check_path` | string | `""` | Custom health check path |
| `health_check_timeout` | duration | `""` | Custom health check timeout |
| `health_check_interval` | duration | `""` | Custom health check interval |
| `expected_status_codes` | []int | `[200-299]` | Valid health status codes |

#### App Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `id` | string | required | Unique app identifier |
| `name` | string | required | Display name |
| `icon` | string | `""` | Icon identifier |
| `tags` | []string | `[]` | Searchable tags |
| `endpoints` | map | `{}` | Network-specific endpoints |

---

## Environment Variables

Environment variables override configuration file values. Use these for sensitive data and deployment flexibility.

| Variable | Config Path | Description |
|----------|-------------|-------------|
| `NEKZUS_ADDR` | `server.addr` | Server listen address |
| `NEKZUS_BASE_URL` | `server.base_url` | Base URL for external access |
| `NEKZUS_TLS_CERT` | `server.tls_cert` | TLS certificate path |
| `NEKZUS_TLS_KEY` | `server.tls_key` | TLS private key path |
| `NEKZUS_JWT_SECRET` | `auth.hs256_secret` | JWT signing secret |
| `NEKZUS_BOOTSTRAP_TOKEN` | `bootstrap.tokens` | Bootstrap token (appended to list) |
| `NEKZUS_DATABASE_PATH` | `storage.database_path` | SQLite database path |
| `NEKZUS_TOOLBOX_HOST_DATA_DIR` | `toolbox.host_data_dir` | Host data directory for DinD |
| `NEKZUS_HOST_ROOT_PATH` | `system.host_root_path` | Host root path for metrics |
| `NEKZUS_DISCOVERY_ENABLED` | `discovery.enabled` | Enable discovery (`true`/`1`) |
| `NEKZUS_DISCOVERY_DOCKER_ENABLED` | `discovery.docker.enabled` | Enable Docker discovery |
| `NEKZUS_TOOLBOX_ENABLED` | `toolbox.enabled` | Enable toolbox |
| `NEKZUS_METRICS_ENABLED` | `metrics.enabled` | Enable metrics endpoint |
| `ENVIRONMENT` | - | Set to `production` for strict validation |

### Debug Variables

| Variable | Description |
|----------|-------------|
| `NEKZUS_DEBUG` | Enable debug logging (`true`/`1`) |
| `NEKZUS_DEBUG_TOKENS` | Enable debug token platform (`1`) |
| `NEKZUS_BOOTSTRAP_ALLOW_ANY` | Allow any bootstrap token in development (`1`) |

:::warning[NEKZUS_BOOTSTRAP_ALLOW_ANY]

The `NEKZUS_BOOTSTRAP_ALLOW_ANY=1` setting only works when `ENVIRONMENT=development` is explicitly set. Attempting to use it in production results in an error.

:::


### Certificate Injection

For Kubernetes and other orchestrated environments:

| Variable | Default | Description |
|----------|---------|-------------|
| `NEKZUS_CA_CERT` | `/certs/ca.crt` | Path to CA certificate |
| `NEKZUS_CERT` | `/certs/cert.crt` | Path to service certificate |
| `NEKZUS_KEY` | `/certs/cert.key` | Path to service private key |
| `NEKZUS_CERT_DIR` | `/certs` | Certificate directory |

:::info[Production Environment Variables]

```bash
export NEKZUS_JWT_SECRET=$(openssl rand -base64 32)
export NEKZUS_BOOTSTRAP_TOKEN=$(openssl rand -base64 24)
export ENVIRONMENT=production
```

:::


---

## Hot Reload

Nekzus supports hot reloading for certain configuration changes without restart:

**Hot Reloadable:**

- `metrics.enabled` - Enable/disable metrics endpoint
- `discovery.*` - Discovery settings
- `health_checks.*` - Health check settings
- `routes` and `apps` - Route and app definitions

**Requires Restart:**

- `server.addr` - Listen address
- `server.tls_*` - TLS certificates
- `metrics.path` - Metrics endpoint path
- `auth.*` - Authentication settings
- `storage.*` - Database settings
- `federation.*` - Federation settings

To trigger a reload, send a `SIGHUP` signal:

```bash
kill -HUP $(pidof nekzus)
```

---

## Example Configurations

### Minimal Development Setup

```yaml title="config.yaml"
server:
  addr: ":8080"

discovery:
  enabled: true
  docker:
    enabled: true
```

Run with:
```bash
./nekzus -config config.yaml -insecure-http
```

### Production with TLS

```yaml title="config.yaml"
server:
  addr: ":8443"
  base_url: "https://nexus.example.com:8443"
  tls_cert: "./certs/server-cert.pem"
  tls_key: "./certs/server-key.pem"

auth:
  issuer: "nekzus"
  audience: "nekzus-mobile"
  # Use NEKZUS_JWT_SECRET environment variable

storage:
  database_path: "./data/nexus.db"

backup:
  enabled: true
  directory: "./data/backups"
  schedule: "12h"
  retention: 14

discovery:
  enabled: true
  docker:
    enabled: true
    poll_interval: "30s"

metrics:
  enabled: true
  path: "/metrics"

health_checks:
  enabled: true
  interval: "30s"
  timeout: "5s"
  unhealthy_threshold: 3
```

### Docker Compose Deployment

```yaml title="config.yaml"
server:
  addr: ":8080"

discovery:
  enabled: true
  docker:
    enabled: true
    socket_path: "unix:///var/run/docker.sock"

toolbox:
  enabled: true
  catalog_dir: "./toolbox"
  data_dir: "./data/toolbox"
  host_data_dir: "/opt/nekzus/data/toolbox"  # Host path for volumes
  auto_route: true
  auto_start: true
```

### Kubernetes Cluster

```yaml title="config.yaml"
server:
  addr: ":8080"

discovery:
  enabled: true
  kubernetes:
    enabled: true
    kubeconfig: ""  # Uses in-cluster config
    namespaces:
      - "default"
      - "production"
      - "staging"
    poll_interval: "30s"

runtimes:
  primary: "kubernetes"
  kubernetes:
    enabled: true
    namespaces:
      - "default"
      - "production"
    metrics_server: true
```

### Multi-Instance Federation

```yaml title="config.yaml (Server 1 - Main)"
server:
  addr: ":8080"

federation:
  enabled: true
  cluster_secret: "your-32-character-or-longer-secret"
  gossip_port: 7946
  mdns_enabled: true
  bootstrap_peers: []  # Main server, no bootstrap needed
  sync:
    full_sync_interval: "300s"
    anti_entropy_period: "60s"
```

```yaml title="config.yaml (Server 2 - Secondary)"
server:
  addr: ":8080"

federation:
  enabled: true
  cluster_secret: "your-32-character-or-longer-secret"
  gossip_port: 7946
  mdns_enabled: true
  bootstrap_peers:
    - "192.168.1.100:7946"  # Main server address
```

---

## Validation

Configuration is validated on load with the following checks:

- **Server address**: Valid `host:port` or `:port` format
- **JWT secret**: Minimum 32 characters, no weak patterns in production
- **TLS**: Both `tls_cert` and `tls_key` required if either is set
- **Durations**: Valid Go duration format (e.g., `30s`, `5m`, `1h`)
- **Routes**: Required fields (`id`, `path_base`, `to`), paths start with `/`
- **Apps**: Required fields (`id`, `name`)
- **Metrics path**: Must start with `/`, cannot conflict with `/api/*`

Validation errors are reported at startup with clear messages indicating the problematic configuration.
