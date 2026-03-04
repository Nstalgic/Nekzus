import Tabs from "@theme/Tabs";
import TabItem from "@theme/TabItem";

# CLI Reference

This reference documents all command-line flags and options for the `nekzus` binary.

## Command Structure

Nekzus uses a simple command structure with flags to control behavior:

```bash
nekzus [flags]
```

The binary is designed to run as a long-running server process. All configuration can be done via command-line flags, environment variables, or configuration files.

---

## Flags and Options

### Primary Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--config` | string | `configs/config.example.yaml` | Path to configuration file (YAML or JSON) |
| `--insecure-http` | bool | `false` | Serve HTTP without TLS (development only) |
| `--health` | bool | `false` | Perform health check against running server and exit |
| `--health-addr` | string | `http://localhost:8080` | Address for health check endpoint |

### Flag Details

#### `--config`

Specifies the path to the configuration file. Supports YAML (`.yaml`, `.yml`) and JSON (`.json`) formats.

```bash title="Using a custom configuration file"
nekzus --config /etc/nekzus/config.yaml
```

```bash title="Using JSON configuration"
nekzus --config /path/to/config.json
```

:::note[Configuration Priority]

Configuration is loaded in this order (later sources override earlier):

1. Default values (built-in)
2. Configuration file (`--config`)
3. Environment variables (`NEKZUS_*`)

:::


#### `--insecure-http`

Disables TLS and serves plain HTTP. This flag is intended for development environments only.

```bash title="Development mode without TLS"
nekzus --config configs/config.yaml --insecure-http
```

:::warning[Security Warning]

Never use `--insecure-http` in production. It disables HTTPS encryption, exposing all traffic including authentication tokens.

:::


When this flag is set, the server:

- Ignores `tls_cert` and `tls_key` configuration values
- Listens on plain HTTP instead of HTTPS
- Logs a warning about insecure operation

#### `--health`

Performs a health check against a running Nekzus instance and exits. This is designed for use in container health checks (Docker, Kubernetes).

```bash title="Health check against local server"
nekzus --health
```

```bash title="Health check against remote server"
nekzus --health --health-addr https://192.168.1.100:8443
```

The health check:

- Calls the `/api/v1/healthz` endpoint
- Uses a 5-second timeout
- Skips TLS certificate verification (for self-signed certificates)
- Returns exit code `0` for healthy, `1` for unhealthy

#### `--health-addr`

Specifies the address to check when running with `--health`. Use this when the server is running on a non-default port or remote host.

```bash title="Check remote server health"
nekzus --health --health-addr https://nexus.local:8443
```

---

## Environment Variables

Environment variables override configuration file values. All environment variables use the `NEKZUS_` prefix.

### Core Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `NEKZUS_ADDR` | Server listen address (e.g., `:8080`, `0.0.0.0:8443`) | `:8443` |
| `NEKZUS_TLS_CERT` | Path to TLS certificate file | (none) |
| `NEKZUS_TLS_KEY` | Path to TLS private key file | (none) |
| `NEKZUS_JWT_SECRET` | JWT signing secret (minimum 32 characters) | (auto-generated) |
| `NEKZUS_BOOTSTRAP_TOKEN` | Additional bootstrap token for device pairing | (none) |
| `NEKZUS_DATABASE_PATH` | Path to SQLite database file | `./data/nexus.db` |
| `NEKZUS_BASE_URL` | Public URL for QR code pairing | (auto-detected) |
| `NEKZUS_DEBUG` | Enable debug logging (`true` or `1`) | `false` |

### Feature Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `NEKZUS_DISCOVERY_ENABLED` | Enable service discovery | (from config) |
| `NEKZUS_DISCOVERY_DOCKER_ENABLED` | Enable Docker discovery | (from config) |
| `NEKZUS_TOOLBOX_ENABLED` | Enable toolbox functionality | (from config) |
| `NEKZUS_TOOLBOX_HOST_DATA_DIR` | Host path for toolbox data (Docker-in-Docker) | (none) |
| `NEKZUS_METRICS_ENABLED` | Enable Prometheus metrics endpoint | (from config) |
| `NEKZUS_HOST_ROOT_PATH` | Host root path for system metrics | (none) |

### Environment Variable Examples

<Tabs>
<TabItem value="minimal-setup" label="Minimal Setup">


```bash
export NEKZUS_ADDR=":8080"
export NEKZUS_JWT_SECRET="$(openssl rand -base64 32)"
nekzus --insecure-http
```

</TabItem>
<TabItem value="production-setup" label="Production Setup">


```bash
export NEKZUS_ADDR=":8443"
export NEKZUS_TLS_CERT="/etc/nekzus/cert.pem"
export NEKZUS_TLS_KEY="/etc/nekzus/key.pem"
export NEKZUS_JWT_SECRET="your-strong-secret-min-32-characters"
export NEKZUS_BOOTSTRAP_TOKEN="your-bootstrap-token"
export NEKZUS_DATABASE_PATH="/var/lib/nekzus/nexus.db"
export NEKZUS_BASE_URL="https://nexus.example.com"
nekzus --config /etc/nekzus/config.yaml
```

</TabItem>
<TabItem value="docker-container" label="Docker Container">


```bash
docker run -d \
  -e NEKZUS_ADDR=":8080" \
  -e NEKZUS_JWT_SECRET="$(openssl rand -base64 32)" \
  -e NEKZUS_BOOTSTRAP_TOKEN="my-bootstrap-token" \
  -e NEKZUS_DISCOVERY_ENABLED="true" \
  -e NEKZUS_DISCOVERY_DOCKER_ENABLED="true" \
  -p 8080:8080 \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  nstalgic/nekzus:latest
```

</TabItem>
</Tabs>

---

## Exit Codes

Nekzus uses standard Unix exit codes:

| Code | Meaning | Description |
|------|---------|-------------|
| `0` | Success | Normal shutdown or healthy status (with `--health`) |
| `1` | Error | Configuration error, initialization failure, or unhealthy status |

### Exit Code Scenarios

**Exit code 0:**

- Server receives SIGINT or SIGTERM and shuts down gracefully
- Health check (`--health`) reports server is healthy

**Exit code 1:**

- Configuration file not found or invalid
- Configuration validation fails
- Database initialization fails
- TLS certificate loading fails
- Health check (`--health`) reports server is unhealthy or unreachable

---

## Signals

Nekzus handles the following Unix signals:

| Signal | Behavior |
|--------|----------|
| `SIGINT` (Ctrl+C) | Graceful shutdown with 30-second timeout |
| `SIGTERM` | Graceful shutdown with 30-second timeout |

### Graceful Shutdown Sequence

When a shutdown signal is received:

1. Stop accepting new connections
2. Stop configuration watcher
3. Stop discovery workers
4. Stop service health checker
5. Stop scheduled jobs (backups, scripts)
6. Stop notification queue
7. Stop federation peer manager
8. Drain and close WebSocket connections
9. Close HTTP server with active request draining
10. Close Docker client
11. Close database connection

---

## Usage Examples

### Basic Server Start

```bash title="Start with default configuration"
nekzus --config configs/config.yaml
```

### Development Mode

```bash title="Start without TLS for local development"
nekzus --config configs/config.yaml --insecure-http
```

### Production Deployment

```bash title="Production with custom paths"
nekzus --config /etc/nekzus/config.yaml
```

### Container Health Check

Use in Docker health check or Kubernetes liveness probe:

<Tabs>
<TabItem value="docker" label="Docker">


```yaml title="docker-compose.yml"
services:
  nexus:
    image: nstalgic/nekzus:latest
    healthcheck:
      test: ["/app/nekzus", "--health"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 10s
```

</TabItem>
<TabItem value="kubernetes" label="Kubernetes">


```yaml title="deployment.yaml"
spec:
  containers:
    - name: nekzus
      livenessProbe:
        exec:
          command:
            - /app/nekzus
            - --health
        initialDelaySeconds: 10
        periodSeconds: 30
        timeoutSeconds: 5
      readinessProbe:
        exec:
          command:
            - /app/nekzus
            - --health
        initialDelaySeconds: 5
        periodSeconds: 10
```

</TabItem>
<TabItem value="shell-script" label="Shell Script">


```bash title="health-check.sh"
#!/bin/bash
if nekzus --health --health-addr http://localhost:8080; then
    echo "Server is healthy"
    exit 0
else
    echo "Server is unhealthy"
    exit 1
fi
```

</TabItem>
</Tabs>

---

## Make Targets

When running from source, the Makefile provides convenience targets:

| Target | Command | Description |
|--------|---------|-------------|
| `make run` | `go run ./cmd/nekzus --config configs/config.yaml` | Run with TLS |
| `make run-insecure` | `go run ./cmd/nekzus --config configs/config.yaml --insecure-http` | Run without TLS |
| `make build` | `go build -o bin/nekzus ./cmd/nekzus` | Build binary |
| `make build-all` | Build web UI and Go binary | Complete build |
| `make demo` | Start demo environment with Docker Compose | Demo with example services |

### Running from Source

```bash title="Development workflow"
# Build everything
make build-all

# Run with TLS (auto-generates certificates)
make run

# Run without TLS (development)
make run-insecure

# Run demo environment with test services
make demo
```

---

## Debug Mode

Enable debug logging for troubleshooting:

<Tabs>
<TabItem value="environment-variable" label="Environment Variable">


```bash
NEKZUS_DEBUG=true nekzus --config configs/config.yaml
```

</TabItem>
<TabItem value="in-docker" label="In Docker">


```bash
docker run -e NEKZUS_DEBUG=true nstalgic/nekzus:latest
```

</TabItem>
</Tabs>

Debug mode enables:

- Verbose structured logging at DEBUG level
- Additional context in log messages
- Performance timing information

---

## Version Information

The version is embedded at build time via ldflags:

```bash title="Check version"
nekzus --version  # Note: version flag not exposed, check logs on startup
```

The version is displayed in:

- Server startup logs (`nekzus starting version=X.X.X`)
- API endpoint `/api/v1/admin/info`
- Prometheus metrics (`nekzus_build_info`)

### Build with Custom Version

```bash title="Build with version tag"
go build -ldflags="-X main.version=v1.2.3" -o nekzus ./cmd/nekzus
```

---

## Configuration File Reference

For detailed configuration file options, see the [Configuration Reference](configuration).

Quick reference for commonly used configuration sections:

```yaml title="Minimal configuration"
server:
  addr: ":8080"

auth:
  issuer: "nekzus"
  audience: "nekzus-mobile"
  # hs256_secret: Set via NEKZUS_JWT_SECRET env var

storage:
  database_path: "./data/nexus.db"

discovery:
  enabled: true
  docker:
    enabled: true
```

---

## Troubleshooting

### Common Issues

<details>
<summary>Server fails to start with 'failed to load TLS certificate'</summary>


This occurs when TLS certificate paths are configured but files are missing or invalid.

**Solutions:**

1. Use `--insecure-http` for development
2. Ensure certificate files exist at the specified paths
3. Remove `tls_cert` and `tls_key` from config to auto-generate certificates

```bash
# Development without TLS
nekzus --config configs/config.yaml --insecure-http
```

</details>


<details>
<summary>Health check returns unhealthy</summary>


Check connectivity and ensure the server is running:

```bash
# Check if server is responding
curl -k https://localhost:8443/api/v1/healthz

# Check with verbose health output
nekzus --health --health-addr https://localhost:8443
```

</details>


<details>
<summary>Configuration validation fails</summary>


Common validation errors:

- `hs256_secret must be at least 32 characters`: Use a longer JWT secret
- `server.addr: invalid address format`: Use format `:8080` or `0.0.0.0:8080`
- `routes[X].path_base must start with /`: Ensure route paths start with `/`

Run with debug mode to see detailed validation errors:

```bash
NEKZUS_DEBUG=true nekzus --config configs/config.yaml
```

</details>


<details>
<summary>Environment variables not being applied</summary>


Environment variables are applied after loading the config file. Verify:

1. Variable names use the `NEKZUS_` prefix
2. Boolean values use `true`/`false` or `1`/`0`
3. No typos in variable names

```bash
# List all NEKZUS_ environment variables
env | grep NEKZUS_
```

</details>


---

## See Also

- [Configuration Reference](configuration) - Detailed configuration file options
- [Installation Guide](../getting-started/installation) - Installation methods
- [Docker Compose Guide](../guides/docker-compose) - Docker deployment patterns
- [Troubleshooting Guide](../guides/troubleshooting) - Common issues and solutions
