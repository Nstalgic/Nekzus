import Tabs from "@theme/Tabs";
import TabItem from "@theme/TabItem";

# Configuration Reference

This is the complete configuration reference for Nekzus. For a quick introduction to configuration, see the [Getting Started Guide](../getting-started/configuration).

---

## File Format

Nekzus supports both YAML and JSON configuration files. The format is determined by file extension:

| Extension | Format |
|-----------|--------|
| `.yaml`, `.yml` | YAML |
| `.json` | JSON |

```bash
# Specify configuration file path
./nekzus -config /path/to/config.yaml

# Run without TLS (development only)
./nekzus -config config.yaml -insecure-http
```

:::tip[YAML Recommended]

YAML is the preferred format for its readability and comment support. All examples in this reference use YAML.

:::


---

## Configuration Hierarchy

Configuration values are resolved in the following priority order:

1. **Environment Variables** - Highest priority, overrides all other sources
2. **Configuration File** - Values from YAML/JSON config file
3. **Auto-detection/Defaults** - Built-in defaults and runtime detection

---

## Server Configuration

The `server` section configures the HTTP/HTTPS server, listening address, TLS certificates, and base URL for external access.

```yaml
server:
  addr: ":8443"
  base_url: ""
  tls_cert: ""
  tls_key: ""
  http_redirect_addr: ""
```

### Server Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `addr` | string | `:8443` | Server listen address in `host:port` or `:port` format |
| `base_url` | string | auto-detect | Base URL for QR code pairing and external access |
| `tls_cert` | string | `""` | Path to TLS certificate file (PEM format) |
| `tls_key` | string | `""` | Path to TLS private key file (PEM format) |
| `http_redirect_addr` | string | `""` | Address for HTTP to HTTPS redirect server |

### TLS Configuration

<Tabs>
<TabItem value="behind-reverse-proxy" label="Behind Reverse Proxy">


When running behind a reverse proxy (Caddy, nginx, Traefik), leave TLS settings empty:

```yaml
server:
  addr: ":8080"
  tls_cert: ""
  tls_key: ""
```

Use the `-insecure-http` flag when starting the server.

</TabItem>
<TabItem value="auto-generated-certificates" label="Auto-Generated Certificates">


Specify paths for auto-generated self-signed certificates. Nekzus creates them on startup if they do not exist:

```yaml
server:
  addr: ":8443"
  tls_cert: "./certs/server-cert.pem"
  tls_key: "./certs/server-key.pem"
```

</TabItem>
<TabItem value="custom-certificates" label="Custom Certificates">


Provide your own certificates (e.g., from Let's Encrypt):

```yaml
server:
  addr: ":8443"
  tls_cert: "/etc/letsencrypt/live/example.com/fullchain.pem"
  tls_key: "/etc/letsencrypt/live/example.com/privkey.pem"
```

</TabItem>
</Tabs>

### Base URL Resolution

The base URL is resolved in priority order:

1. `NEKZUS_BASE_URL` environment variable
2. `server.base_url` in configuration file
3. Auto-detection from local network IP

:::info[Auto-Detection]

When auto-detected, the base URL is constructed as `{protocol}://{local_ip}{addr}` where protocol is determined by TLS configuration.

:::


### Validation Rules

- `addr` must be valid `host:port` or `:port` format
- Port must be between 0 and 65535
- If `tls_cert` is set, `tls_key` is required (and vice versa)

---

## Authentication Configuration

The `auth` section configures JWT-based authentication.

```yaml
auth:
  issuer: "nekzus"
  audience: "nekzus-mobile"
  hs256_secret: ""
  default_scopes: []
```

### Authentication Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `issuer` | string | `nekzus` | JWT issuer claim |
| `audience` | string | `nekzus-mobile` | JWT audience claim |
| `hs256_secret` | string | auto-generate | HMAC-SHA256 signing secret (minimum 32 characters) |
| `default_scopes` | []string | `[]` | Default scopes applied to routes without explicit scopes |

### JWT Secret Requirements

:::warning[Production Security]

The JWT secret is critical for security. Follow these requirements:

- Minimum 32 characters
- Avoid weak patterns: `dev`, `test`, `change-me`, `example`
- Use environment variable `NEKZUS_JWT_SECRET` for sensitive deployments
- If not provided, a cryptographically secure secret is auto-generated on startup

:::


<details>
<summary>Secret Validation</summary>

In production environments (when `ENVIRONMENT` is not `development`, `dev`, `test`, or empty), weak secret patterns are rejected with an error.

</details>


---

## Bootstrap Tokens

The `bootstrap` section configures initial authentication tokens for device pairing.

```yaml
bootstrap:
  tokens:
    - "your-secure-bootstrap-token"
    - "another-valid-token"
```

### Bootstrap Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `tokens` | []string | `[]` | List of valid bootstrap tokens for device pairing |

:::info[QR Code Pairing]

Bootstrap tokens are optional. If not provided, devices can pair using the QR code flow from the web UI, which generates short-lived tokens.

:::


---

## Storage Configuration

The `storage` section configures SQLite database persistence.

```yaml
storage:
  database_path: "./data/nexus.db"
```

### Storage Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `database_path` | string | `./data/nexus.db` | Path to SQLite database file |

---

## Discovery Configuration

The `discovery` section enables automatic service discovery from Docker containers, mDNS services, and Kubernetes resources.

```yaml
discovery:
  enabled: true

  docker:
    enabled: true
    socket_path: "unix:///var/run/docker.sock"
    poll_interval: "30s"
    network_mode: "all"
    networks: []
    exclude_networks: []

  mdns:
    enabled: true
    scan_interval: "60s"
    services:
      - "_http._tcp"
      - "_https._tcp"
      - "_homeassistant._tcp"

  kubernetes:
    enabled: false
    kubeconfig: ""
    namespaces: []
    poll_interval: "30s"
```

### Discovery Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | bool | `false` | Enable service discovery |

### Docker Discovery

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | bool | `false` | Enable Docker container discovery |
| `socket_path` | string | `unix:///var/run/docker.sock` | Docker socket path |
| `poll_interval` | duration | `30s` | Interval between discovery polls |
| `network_mode` | string | `all` | Network selection mode: `all`, `first`, or `preferred` |
| `networks` | []string | `[]` | Specific networks to scan (required for `preferred` mode) |
| `exclude_networks` | []string | `[]` | Networks to exclude from discovery |

#### Network Modes

| Mode | Description |
|------|-------------|
| `all` | Discover services on all container networks |
| `first` | Use only the first network found |
| `preferred` | Use networks from `networks` list, fallback to first |

:::warning[Preferred Mode]

When using `network_mode: preferred`, the `networks` list must contain at least one network name.

:::


### mDNS Discovery

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | bool | `false` | Enable mDNS/Bonjour discovery |
| `scan_interval` | duration | `60s` | Interval between mDNS scans |
| `services` | []string | `["_http._tcp", "_https._tcp"]` | Service types to discover |

### Kubernetes Discovery

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | bool | `false` | Enable Kubernetes service discovery |
| `kubeconfig` | string | `""` | Path to kubeconfig (empty for in-cluster config) |
| `namespaces` | []string | `[]` | Namespaces to watch (empty for all) |
| `poll_interval` | duration | `30s` | Interval between discovery polls |

<details>
<summary>Kubernetes Auto-Discovery</summary>

Nekzus uses smart label inference to auto-discover Kubernetes services:

| Discovery Method | Confidence Score |
|-----------------|------------------|
| Ingress-exposed services | 1.00 |
| Explicit `nekzus.enable=true` label | 0.95 |
| LoadBalancer/NodePort with standard K8s labels | 0.70 |
| Helm charts with standard labels | 0.60 |
| Other auto-discovered services | 0.50 |

System namespaces (`kube-system`, `kube-public`, `kube-node-lease`) are automatically excluded.

</details>


---

## Toolbox Configuration

The `toolbox` section enables one-click service deployment using Docker Compose templates.

```yaml
toolbox:
  enabled: true
  catalog_dir: "./toolbox"
  data_dir: "./data/toolbox"
  host_data_dir: ""
  auto_route: true
  auto_start: true
```

### Toolbox Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | bool | `false` | Enable toolbox feature |
| `catalog_dir` | string | `./toolbox` | Directory containing Compose-based service templates |
| `data_dir` | string | `./data/toolbox` | Base directory for service data volumes (container path) |
| `host_data_dir` | string | `""` | Host path for Docker-in-Docker bind mounts |
| `auto_route` | bool | `true` | Automatically create routes for deployed services |
| `auto_start` | bool | `true` | Automatically start containers after deployment |

:::warning[Deprecated: catalog_path]

The `catalog_path` option (YAML-based catalog) is deprecated and will be removed in a future release. Use `catalog_dir` for Compose-based templates.

:::


<details>
<summary>Docker-in-Docker</summary>

When running Nekzus inside a Docker container, set `host_data_dir` to the host path that maps to `data_dir` inside the container. This ensures volume bind mounts work correctly.

</details>


---

## Federation Configuration

The `federation` section enables peer-to-peer federation for multi-instance deployments.

```yaml
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

### Federation Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | bool | `false` | Enable peer-to-peer federation |
| `cluster_secret` | string | required | Shared secret for peer authentication (minimum 32 characters) |
| `gossip_port` | int | `7946` | Port for gossip protocol (TCP/UDP) |
| `mdns_enabled` | bool | `true` | Enable mDNS peer discovery |
| `bootstrap_peers` | []string | `[]` | Initial peer addresses (`host:port` format) |
| `allow_remote_routes` | bool | `false` | Allow proxying requests to remote peer services |

### Sync Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `full_sync_interval` | duration | `300s` | Interval for complete catalog synchronization |
| `anti_entropy_period` | duration | `60s` | Interval for anti-entropy repair |

:::danger[Security Warning: allow_remote_routes]

Setting `allow_remote_routes: true` allows proxying requests to **any service** discovered by federated peers. Only enable this in trusted network environments where all peers are under your control.

:::


<details>
<summary>Multi-Server Setup</summary>

```yaml title="Server 1 (Main)"
federation:
  enabled: true
  cluster_secret: "your-32-character-or-longer-secret"
  gossip_port: 7946
  mdns_enabled: true
  bootstrap_peers: []  # Main server, no bootstrap needed
```

```yaml title="Server 2 (Secondary)"
federation:
  enabled: true
  cluster_secret: "your-32-character-or-longer-secret"
  gossip_port: 7946
  mdns_enabled: true
  bootstrap_peers:
    - "192.168.1.100:7946"  # Main server address
```

</details>


---

## Runtimes Configuration

The `runtimes` section configures container runtime settings for management operations (list, start, stop, restart, stats).

```yaml
runtimes:
  primary: "docker"

  docker:
    enabled: true
    socket_path: ""

  kubernetes:
    enabled: false
    kubeconfig: ""
    context: ""
    namespaces: []
    metrics_server: true
    metrics_cache_ttl: "30s"
```

### Runtime Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `primary` | string | `docker` | Primary runtime for container operations (`docker` or `kubernetes`) |

### Docker Runtime

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | bool | `true` | Enable Docker runtime |
| `socket_path` | string | `unix:///var/run/docker.sock` | Docker socket path (empty for default) |

### Kubernetes Runtime

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | bool | `false` | Enable Kubernetes runtime |
| `kubeconfig` | string | `""` | Path to kubeconfig (empty for in-cluster or default) |
| `context` | string | `""` | Kubernetes context (empty for current context) |
| `namespaces` | []string | `[]` | Namespaces to manage (empty for all) |
| `metrics_server` | bool | `true` | Use Metrics Server for pod statistics |
| `metrics_cache_ttl` | duration | `30s` | Cache TTL for pod metrics |

---

## Metrics Configuration

The `metrics` section configures the Prometheus metrics endpoint.

```yaml
metrics:
  enabled: true
  path: "/metrics"
```

### Metrics Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | bool | `true` | Enable/disable the metrics HTTP endpoint |
| `path` | string | `/metrics` | Metrics endpoint path |

:::note[Internal Metrics Collection]

Metrics are **always** collected internally for observability. The `enabled` setting only controls whether the HTTP endpoint is exposed.

:::


### Validation Rules

- `path` must start with `/`
- `path` cannot conflict with API paths (`/api/v1/healthz`, `/api/*`)

### Hot Reload

- `enabled` can be hot-reloaded without restart
- `path` requires a restart to take effect

---

## Health Checks Configuration

The `health_checks` section configures service health monitoring.

```yaml
health_checks:
  enabled: true
  interval: "30s"
  timeout: "5s"
  unhealthy_threshold: 3
  path: "/"
  per_service:
    grafana:
      path: "/api/health"
      interval: "30s"
      timeout: "5s"
```

### Health Check Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | bool | `true` | Enable service health monitoring |
| `interval` | duration | `10s` | Default check interval |
| `timeout` | duration | `5s` | Default HTTP request timeout |
| `unhealthy_threshold` | int | `2` | Consecutive failures before marking unhealthy |
| `path` | string | `/` | Default health check path |
| `per_service` | map | `{}` | Per-service configuration overrides |

### Per-Service Overrides

| Option | Type | Description |
|--------|------|-------------|
| `path` | string | Health check endpoint path |
| `interval` | duration | Check interval for this service |
| `timeout` | duration | Request timeout for this service |

---

## Notifications Configuration

The `notifications` section configures WebSocket notifications for mobile devices.

```yaml
notifications:
  enabled: false
  ack_timeout: "30s"

  queue:
    worker_count: 4
    buffer_size: 1000
    retry_interval: "30s"
    max_retries: 3

  offline_detection:
    enabled: false
    check_interval: "1m"
    offline_threshold: "5m"
```

### Notification Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | bool | `false` | Enable notification system |
| `ack_timeout` | duration | `30s` | Timeout waiting for client acknowledgment |

### Queue Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `worker_count` | int | `4` | Number of worker goroutines |
| `buffer_size` | int | `1000` | Channel buffer size |
| `retry_interval` | duration | `30s` | Retry interval for failed notifications |
| `max_retries` | int | `3` | Maximum retry attempts |

### Offline Detection Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | bool | `false` | Enable offline device detection |
| `check_interval` | duration | `1m` | How often to check for offline devices |
| `offline_threshold` | duration | `5m` | Duration after which device is considered offline |

---

## Backup Configuration

The `backup` section configures automatic database backups.

```yaml
backup:
  enabled: true
  directory: "./data/backups"
  schedule: "24h"
  retention: 7
```

### Backup Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | bool | `true` | Enable automatic backups |
| `directory` | string | `./data/backups` | Directory to store backup files |
| `schedule` | duration | `24h` | Backup interval (e.g., `24h`, `12h`, `1h`) |
| `retention` | int | `7` | Number of backups to keep (older backups are deleted) |

---

## Scripts Configuration

The `scripts` section configures user script execution.

```yaml
scripts:
  enabled: false
  directory: "./scripts"
  default_timeout: 300
  max_output_bytes: 10485760
```

### Scripts Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | bool | `false` | Enable script execution feature |
| `directory` | string | `./scripts` | Directory containing user scripts |
| `default_timeout` | int | `300` | Default script timeout in seconds |
| `max_output_bytes` | int | `10485760` | Maximum output size in bytes (10 MB) |

---

## System Configuration

The `system` section configures system-level settings.

```yaml
system:
  host_root_path: ""
```

### System Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `host_root_path` | string | `""` | Path to mounted host root filesystem for host metrics |

:::info[Container Metrics]

When running in a container, set `host_root_path` to the mount point of the host filesystem (e.g., `/mnt/host`) to report accurate host system metrics (CPU, RAM, disk) instead of container metrics.

:::


---

## Routes Configuration

The `routes` section defines static reverse proxy route mappings.

```yaml
routes:
  - id: "route:grafana"
    app_id: "grafana"
    path_base: "/apps/grafana/"
    to: "http://127.0.0.1:3000"
    scopes: ["access:grafana"]
    websockets: true
    strip_prefix: true
```

### Route Options

| Option | Type | Default | Required | Description |
|--------|------|---------|----------|-------------|
| `id` | string | - | Yes | Unique route identifier |
| `app_id` | string | - | No | Associated application ID |
| `path_base` | string | - | Yes | URL path prefix (must start with `/`) |
| `to` | string | - | Yes | Upstream service URL |
| `container_id` | string | `""` | No | Explicit link to Docker/Kubernetes container |
| `scopes` | []string | `[]` | No | Required authorization scopes |
| `public_access` | bool | `false` | No | Bypass authentication (explicit opt-out) |
| `websockets` | bool | `false` | No | Enable WebSocket proxying |
| `strip_prefix` | bool | `false` | No | Strip path prefix before proxying |
| `strip_response_cookies` | bool | `false` | No | Remove Set-Cookie headers from upstream |
| `rewrite_cookie_paths` | bool | `false` | No | Rewrite cookie paths when strip_prefix is enabled |
| `rewrite_html` | bool | `false` | No | Rewrite HTML responses to fix absolute asset paths |
| `persist_cookies` | bool | `false` | No | Persist cookies for mobile webview session continuity |

### Route Health Check Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `health_check_path` | string | `""` | Custom health check endpoint path |
| `health_check_timeout` | duration | `""` | Custom health check timeout |
| `health_check_interval` | duration | `""` | Custom health check interval |
| `expected_status_codes` | []int | `[200-299]` | Valid health check status codes |

### Scope Resolution

Scopes are resolved in the following priority:

1. If `public_access: true`, no scopes are required
2. If `scopes` is set on the route, those scopes are used
3. Otherwise, `auth.default_scopes` from config is used

---

## Apps Configuration

The `apps` section defines static application definitions.

```yaml
apps:
  - id: "grafana"
    name: "Grafana"
    icon: "chart"
    tags: ["monitoring", "dashboards"]
    endpoints:
      lan: "http://127.0.0.1:3000"
```

### App Options

| Option | Type | Default | Required | Description |
|--------|------|---------|----------|-------------|
| `id` | string | - | Yes | Unique application identifier |
| `name` | string | - | Yes | Display name |
| `icon` | string | `""` | No | Icon identifier |
| `tags` | []string | `[]` | No | Searchable tags |
| `endpoints` | map | `{}` | No | Network-specific endpoint URLs |

---

## Environment Variables

Environment variables override configuration file values. Use these for sensitive data and deployment flexibility.

### Complete Environment Variable Reference

| Variable | Config Path | Type | Description |
|----------|-------------|------|-------------|
| `NEKZUS_ADDR` | `server.addr` | string | Server listen address |
| `NEKZUS_BASE_URL` | `server.base_url` | string | Base URL for external access |
| `NEKZUS_TLS_CERT` | `server.tls_cert` | string | TLS certificate path |
| `NEKZUS_TLS_KEY` | `server.tls_key` | string | TLS private key path |
| `NEKZUS_JWT_SECRET` | `auth.hs256_secret` | string | JWT signing secret |
| `NEKZUS_BOOTSTRAP_TOKEN` | `bootstrap.tokens` | string | Bootstrap token (appended to list) |
| `NEKZUS_DATABASE_PATH` | `storage.database_path` | string | SQLite database path |
| `NEKZUS_TOOLBOX_HOST_DATA_DIR` | `toolbox.host_data_dir` | string | Host data directory for DinD |
| `NEKZUS_HOST_ROOT_PATH` | `system.host_root_path` | string | Host root path for metrics |
| `NEKZUS_DISCOVERY_ENABLED` | `discovery.enabled` | bool | Enable discovery (`true`/`1`) |
| `NEKZUS_DISCOVERY_DOCKER_ENABLED` | `discovery.docker.enabled` | bool | Enable Docker discovery |
| `NEKZUS_TOOLBOX_ENABLED` | `toolbox.enabled` | bool | Enable toolbox |
| `NEKZUS_METRICS_ENABLED` | `metrics.enabled` | bool | Enable metrics endpoint |
| `ENVIRONMENT` | - | string | Environment mode (`production`, `development`) |

### Development/Debug Variables

| Variable | Description |
|----------|-------------|
| `NEKZUS_DEBUG` | Enable debug logging (`true`/`1`) |
| `NEKZUS_DEBUG_TOKENS` | Enable debug token platform (`1`) |
| `NEKZUS_BOOTSTRAP_ALLOW_ANY` | Allow any bootstrap token in development (`1`) |

### Certificate Injection Variables

These variables are set automatically when deploying services via the toolbox:

| Variable | Default Value | Description |
|----------|---------------|-------------|
| `NEKZUS_CA_CERT` | `/certs/ca.crt` | Path to CA certificate |
| `NEKZUS_CERT` | `/certs/cert.crt` | Path to service certificate |
| `NEKZUS_KEY` | `/certs/cert.key` | Path to service private key |
| `NEKZUS_CERT_DIR` | `/certs` | Certificate directory |

:::warning[NEKZUS_BOOTSTRAP_ALLOW_ANY]

The `NEKZUS_BOOTSTRAP_ALLOW_ANY=1` setting only works when `ENVIRONMENT=development` is explicitly set. Attempting to use it in production results in an error.

:::


---

## Validation

Configuration is validated on load with comprehensive checks. Invalid configuration prevents startup with clear error messages.

### Validation Rules Summary

| Section | Rule |
|---------|------|
| `server.addr` | Valid `host:port` or `:port` format, port 0-65535 |
| `server.tls_*` | Both `tls_cert` and `tls_key` required if either is set |
| `auth.hs256_secret` | Minimum 32 characters, no weak patterns in production |
| `discovery.docker.network_mode` | Must be `all`, `first`, or `preferred` |
| `discovery.docker.networks` | Required when `network_mode` is `preferred` |
| `runtimes.primary` | Must be `docker` or `kubernetes` |
| `metrics.path` | Must start with `/`, cannot conflict with `/api/*` |
| `routes[].id` | Required |
| `routes[].path_base` | Required, must start with `/` |
| `routes[].to` | Required |
| `apps[].id` | Required |
| `apps[].name` | Required |
| All duration fields | Valid Go duration format (e.g., `30s`, `5m`, `1h`) |

---

## Hot Reload

Nekzus supports hot reloading for certain configuration changes without restart.

### Hot Reloadable Settings

- `metrics.enabled` - Enable/disable metrics endpoint
- `discovery.*` - All discovery settings
- `health_checks.*` - All health check settings
- `routes` - Route definitions
- `apps` - Application definitions

### Settings Requiring Restart

- `server.addr` - Listen address
- `server.tls_*` - TLS certificates
- `metrics.path` - Metrics endpoint path
- `auth.*` - All authentication settings
- `storage.*` - Database settings
- `federation.*` - Federation settings

### Triggering Hot Reload

Send a `SIGHUP` signal to trigger configuration reload:

```bash
kill -HUP $(pidof nekzus)
```

---

## Example Configurations

### Minimal Development

```yaml
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

```yaml
server:
  addr: ":8443"
  base_url: "https://nekzus.example.com:8443"
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

```yaml
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
  host_data_dir: "/opt/nekzus/data/toolbox"
  auto_route: true
  auto_start: true

system:
  host_root_path: "/mnt/host"
```

### Kubernetes Cluster

```yaml
server:
  addr: ":8080"

discovery:
  enabled: true
  kubernetes:
    enabled: true
    kubeconfig: ""
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
    metrics_cache_ttl: "30s"
```

### Full Featured Configuration

<details>
<summary>Complete Configuration Example</summary>

```yaml
server:
  addr: ":8443"
  base_url: "https://nekzus.local:8443"
  tls_cert: "./certs/server-cert.pem"
  tls_key: "./certs/server-key.pem"

auth:
  issuer: "nekzus"
  audience: "nekzus-mobile"
  # hs256_secret: set via NEKZUS_JWT_SECRET
  default_scopes: []

bootstrap:
  tokens:
    - "secure-bootstrap-token-1"

storage:
  database_path: "./data/nexus.db"

backup:
  enabled: true
  directory: "./data/backups"
  schedule: "24h"
  retention: 7

notifications:
  enabled: true
  ack_timeout: "30s"
  queue:
    worker_count: 4
    buffer_size: 1000
    retry_interval: "30s"
    max_retries: 3
  offline_detection:
    enabled: true
    check_interval: "1m"
    offline_threshold: "5m"

discovery:
  enabled: true
  docker:
    enabled: true
    socket_path: "unix:///var/run/docker.sock"
    poll_interval: "30s"
    network_mode: "all"
  mdns:
    enabled: true
    scan_interval: "60s"
    services:
      - "_http._tcp"
      - "_https._tcp"
      - "_homeassistant._tcp"
  kubernetes:
    enabled: false

toolbox:
  enabled: true
  catalog_dir: "./toolbox"
  data_dir: "./data/toolbox"
  auto_route: true
  auto_start: true

runtimes:
  primary: "docker"
  docker:
    enabled: true
  kubernetes:
    enabled: false

metrics:
  enabled: true
  path: "/metrics"

health_checks:
  enabled: true
  interval: "30s"
  timeout: "5s"
  unhealthy_threshold: 3
  path: "/"
  per_service:
    grafana:
      path: "/api/health"
      interval: "30s"
      timeout: "5s"

scripts:
  enabled: false
  directory: "./scripts"
  default_timeout: 300
  max_output_bytes: 10485760

routes:
  - id: "route:grafana"
    app_id: "grafana"
    path_base: "/apps/grafana/"
    to: "http://grafana:3000"
    scopes: ["access:grafana"]
    websockets: true
    strip_prefix: true

apps:
  - id: "grafana"
    name: "Grafana"
    icon: "chart"
    tags: ["monitoring", "dashboards"]
    endpoints:
      lan: "http://grafana:3000"
```

</details>


---

## Duration Format

All duration fields use Go's duration format:

| Unit | Suffix | Example |
|------|--------|---------|
| Nanoseconds | `ns` | `500ns` |
| Microseconds | `us` or `us` | `100us` |
| Milliseconds | `ms` | `500ms` |
| Seconds | `s` | `30s` |
| Minutes | `m` | `5m` |
| Hours | `h` | `24h` |

Durations can be combined: `1h30m`, `2m30s`
