import Tabs from "@theme/Tabs";
import TabItem from "@theme/TabItem";

# Quick Start

Get Nekzus running in under 5 minutes. This guide focuses on the fastest path to a working setup with service discovery and reverse proxy functionality.

---

## Start Nekzus

The fastest way to get started is with a single Docker command.

### One-Liner (Docker)

```bash
docker run -d \
  --name nekzus \
  -p 8080:8080 \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  -v nekzus-data:/app/data \
  nstalgic/nekzus:latest
```

That's it. Nekzus is now running with:

- Web dashboard on port 8080
- Docker container discovery enabled
- Persistent data storage
- Default configuration

:::tip[Verify it's running]

```bash
curl http://localhost:8080/api/v1/healthz
```
Expected response: `ok`

:::


---

## Access the Web Dashboard

Open your browser and navigate to:

**[http://localhost:8080](http://localhost:8080)**

You'll see the Nekzus dashboard with:

| Page | Description |
|------|-------------|
| **Overview** | System status and quick actions |
| **Discovery** | Auto-discovered services and proposals |
| **Routes** | Active proxy routes |
| **Catalog** | Available applications |
| **Settings** | Theme customization (7 themes available) |

:::tip[Keyboard Shortcut]

Press <kbd>Ctrl</kbd>+<kbd>K</kbd> (or <kbd>Cmd</kbd>+<kbd>K</kbd> on macOS) to open the theme switcher.

:::


---

## Enable Service Discovery

Nekzus automatically discovers Docker containers with the right labels. Add these labels to any container you want discovered.

### Docker Container Labels

Add these labels to your `docker-compose.yml` or `docker run` command:

<Tabs>
<TabItem value="docker-compose-yml" label="docker-compose.yml">


```yaml title="docker-compose.yml"
services:
  my-app:
    image: myapp:latest
    labels:
      nekzus.enable: "true"
      nekzus.app.name: "My Application"
      nekzus.app.id: "myapp"
      nekzus.route.path: "/apps/myapp/"
```

</TabItem>
<TabItem value="docker-run" label="docker run">


```bash
docker run -d \
  --name my-app \
  --label nekzus.enable=true \
  --label nekzus.app.name="My Application" \
  --label nekzus.app.id=myapp \
  --label nekzus.route.path="/apps/myapp/" \
  myapp:latest
```

</TabItem>
</Tabs>

### Label Reference

| Label | Required | Description |
|-------|----------|-------------|
| `nekzus.enable` | Yes | Set to `"true"` to enable discovery |
| `nekzus.app.name` | No | Display name in dashboard |
| `nekzus.app.id` | No | Unique identifier (auto-generated if omitted) |
| `nekzus.route.path` | No | Proxy path (e.g., `/apps/myapp/`) |

### Example: Grafana with Discovery

```yaml title="docker-compose.yml"
services:
  grafana:
    image: grafana/grafana:latest
    container_name: grafana
    ports:
      - "3000:3000"
    labels:
      nekzus.enable: "true"
      nekzus.app.name: "Grafana"
      nekzus.app.id: "grafana"
      nekzus.route.path: "/apps/grafana/"
    networks:
      - nekzus-network

networks:
  nekzus-network:
    external: true
    name: nekzus-network
```

:::note[Network Connectivity]

For Nekzus to proxy requests to your containers, they must be on a network accessible to Nekzus. The easiest approach is to put them on the same Docker network.

:::


### View Discovered Services

After adding labels and restarting your container:

1. Open the **Discovery** page in the dashboard
2. View pending proposals
3. Click **Approve** to create a route automatically

Or via API:

```bash
# List discovered proposals
curl http://localhost:8080/api/v1/discovery/proposals

# Approve a proposal
curl -X POST http://localhost:8080/api/v1/discovery/proposals/{id}/approve
```

---

## Add a Route Manually

You can also add routes manually without discovery labels. This is useful for external services or services running outside Docker.

### Via Configuration File

Create or edit your configuration file:

```yaml title="config.yaml"
routes:
  - id: "route:my-service"
    app_id: "my-service"
    path_base: "/apps/my-service/"
    to: "http://192.168.1.100:8080"
    strip_prefix: true
    websocket: true
    scopes: []

apps:
  - id: "my-service"
    name: "My External Service"
    endpoints:
      lan: "http://192.168.1.100:8080"
```

Then start Nekzus with the config:

```bash
docker run -d \
  --name nekzus \
  -p 8080:8080 \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  -v $(pwd)/config.yaml:/app/configs/config.yaml:ro \
  -v nekzus-data:/app/data \
  nstalgic/nekzus:latest \
  --config /app/configs/config.yaml
```

### Route Configuration Options

| Option | Type | Description |
|--------|------|-------------|
| `id` | string | Unique route identifier (e.g., `route:myapp`) |
| `app_id` | string | Associated application ID |
| `path_base` | string | URL path prefix (e.g., `/apps/myapp/`) |
| `to` | string | Upstream service URL |
| `strip_prefix` | boolean | Remove path prefix before forwarding |
| `websocket` | boolean | Enable WebSocket proxying |
| `scopes` | array | Required authentication scopes |

---

## Test the Proxy

Once you have a route configured (either via discovery or manually), test the proxy functionality.

### Basic Proxy Test

```bash
# Replace 'grafana' with your app ID
curl http://localhost:8080/apps/grafana/
```

### Using the Route Tester

The web dashboard includes a built-in route testing tool:

1. Open the **Route Tester** page
2. Select a route from the dropdown
3. Enter a request path
4. Click **Send Request**
5. View the full request/response details

### WebSocket Proxy Test

For WebSocket-enabled routes:

```bash
# Using websocat (install: cargo install websocat)
websocat ws://localhost:8080/apps/myapp/ws

# Using wscat (install: npm install -g wscat)
wscat -c ws://localhost:8080/apps/myapp/ws
```

### Verify Proxy Headers

Nekzus automatically adds forwarding headers:

```bash
curl -v http://localhost:8080/apps/myapp/ 2>&1 | grep -i x-forwarded
```

Expected headers on upstream requests:

- `X-Forwarded-For`: Client IP address
- `X-Forwarded-Host`: Original host header
- `X-Forwarded-Proto`: Original protocol (http/https)

---

## Production Configuration

For production deployments, configure secure secrets:

<Tabs>
<TabItem value="environment-variables" label="Environment Variables">


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

</TabItem>
<TabItem value="-env-file" label=".env File">


```bash title=".env"
NEKZUS_JWT_SECRET=your-secure-random-secret-min-32-chars
NEKZUS_BOOTSTRAP_TOKEN=your-bootstrap-token
NEKZUS_BASE_URL=https://your-server:8443
```

```bash
docker run -d \
  --name nekzus \
  --env-file .env \
  -p 8080:8080 \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  -v nekzus-data:/app/data \
  nstalgic/nekzus:latest
```

</TabItem>
</Tabs>

:::warning[Security Notice]

Always use strong, randomly generated secrets in production:

- `NEKZUS_JWT_SECRET`: Minimum 32 characters
- `NEKZUS_BOOTSTRAP_TOKEN`: Used for device pairing

:::


---

## Common Commands

### Container Management

```bash
# View logs
docker logs -f nekzus

# Restart
docker restart nekzus

# Stop
docker stop nekzus

# Remove
docker rm -f nekzus
```

### API Endpoints

```bash
# Health check
curl http://localhost:8080/healthz

# Instance info
curl http://localhost:8080/api/v1/admin/info

# List apps
curl http://localhost:8080/api/v1/apps

# List routes
curl http://localhost:8080/api/v1/routes

# List discovery proposals
curl http://localhost:8080/api/v1/discovery/proposals
```

---

## Troubleshooting

<details>
<summary>Docker discovery not finding containers</summary>


**Check 1**: Ensure Docker socket is mounted correctly:
```bash
docker inspect nekzus | grep -A5 Mounts
```

**Check 2**: Verify container labels are correct:
```bash
docker inspect my-app | grep -A10 Labels
```

**Check 3**: Check Nekzus logs:
```bash
docker logs nekzus 2>&1 | grep -i discovery
```

</details>


<details>
<summary>Proxy returns 502 Bad Gateway</summary>


**Check 1**: Verify the upstream service is running:
```bash
curl http://upstream-host:port/
```

**Check 2**: Ensure network connectivity:
```bash
docker exec nekzus ping upstream-host
```

**Check 3**: Containers must be on the same Docker network or use host networking.

</details>


<details>
<summary>Cannot access dashboard from other devices</summary>


**Check 1**: Bind to all interfaces:
```bash
-e NEKZUS_ADDR=":8080"  # Listens on all interfaces
```

**Check 2**: Ensure firewall allows port 8080:
```bash
# Linux (ufw)
sudo ufw allow 8080

# Linux (firewalld)
sudo firewall-cmd --add-port=8080/tcp --permanent
```

**Check 3**: Set `NEKZUS_BASE_URL` for QR code pairing:
```bash
-e NEKZUS_BASE_URL="http://192.168.1.100:8080"
```

</details>


---

## Next Steps

Now that Nekzus is running, explore these features:

<div className="card-grid">
  <div className="card">
    <h3>⚙️ Configuration Reference</h3>
    <p>Learn about all configuration options including authentication, TLS, and advanced routing.</p>
    <a href="../getting-started/configuration">→ Configuration</a>
  </div>
  <div className="card">
    <h3>🐳 Docker Compose Guide</h3>
    <p>Production-ready Docker Compose setup with Caddy for TLS termination.</p>
    <a href="../guides/docker-compose">→ Docker Compose</a>
  </div>
  <div className="card">
    <h3>🔍 Discovery Features</h3>
    <p>Deep dive into Docker, Kubernetes, and mDNS service discovery.</p>
    <a href="../features/discovery">→ Discovery</a>
  </div>
  <div className="card">
    <h3>🧰 Toolbox</h3>
    <p>One-click deployment of popular services from the built-in catalog.</p>
    <a href="../features/toolbox">→ Toolbox</a>
  </div>
  <div className="card">
    <h3>📡 API Reference</h3>
    <p>Complete API documentation for integrations and automation.</p>
    <a href="../reference/api">→ API Reference</a>
  </div>
  <div className="card">
    <h3>🖥️ Platform Guides</h3>
    <p>Specific instructions for Synology, Unraid, Proxmox, and Raspberry Pi.</p>
    <a href="../platforms/">→ Platforms</a>
  </div>
</div>

---

## Quick Reference

### Essential Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `NEKZUS_ADDR` | `:8080` | Server listen address |
| `NEKZUS_JWT_SECRET` | - | JWT signing secret (32+ chars) |
| `NEKZUS_BOOTSTRAP_TOKEN` | - | Bootstrap authentication token |
| `NEKZUS_BASE_URL` | auto-detect | Public URL for QR pairing |
| `NEKZUS_DATABASE_PATH` | `./data/nexus.db` | SQLite database path |

### Discovery Labels

| Label | Example | Description |
|-------|---------|-------------|
| `nekzus.enable` | `"true"` | Enable discovery |
| `nekzus.app.name` | `"My App"` | Display name |
| `nekzus.app.id` | `"myapp"` | Unique identifier |
| `nekzus.route.path` | `"/apps/myapp/"` | Proxy path |

### Default Ports

| Port | Protocol | Description |
|------|----------|-------------|
| 8080 | HTTP | Web dashboard and API |
| 8443 | HTTPS | Secure access (with Caddy) |
| 7946 | TCP/UDP | Federation gossip (optional) |
