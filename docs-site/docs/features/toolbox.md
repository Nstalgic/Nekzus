import Tabs from "@theme/Tabs";
import TabItem from "@theme/TabItem";

# Toolbox

The Toolbox is Nekzus's one-click service deployment system that allows you to deploy pre-configured Docker Compose applications directly from the web dashboard. Services are defined using standard Docker Compose files with special labels that provide metadata and enable automatic routing.

---

## Overview

The Toolbox system provides:

- **One-click deployments** - Deploy complex applications without writing configuration
- **Pre-configured services** - Curated catalog of self-hosted applications
- **Auto-routing** - Automatic route creation for deployed services
- **Auto-discovery** - Deployed services are automatically discovered by Nekzus
- **Environment variable injection** - Dynamic configuration at deployment time
- **Persistent storage** - Volume management for data persistence

:::info[Compose-Based Architecture]

The Toolbox uses standard Docker Compose files, making it easy to add new services or customize existing ones. No proprietary format required.

:::


---

## Configuration

Enable the Toolbox in your `config.yaml`:

```yaml
toolbox:
  enabled: true
  catalog_dir: "./toolbox"           # Directory containing service templates
  data_dir: "./data/toolbox"         # Directory for persistent data volumes
  host_data_dir: ""                  # Host path for Docker-in-Docker (optional)
  auto_route: true                   # Automatically create routes for services
  auto_start: true                   # Start containers after deployment
```

| Option | Description | Default |
|--------|-------------|---------|
| `enabled` | Enable/disable the Toolbox feature | `true` |
| `catalog_dir` | Path to directory containing service templates | `./toolbox` |
| `data_dir` | Path for storing persistent volume data | `./data/toolbox` |
| `host_data_dir` | Host path when running in Docker-in-Docker | (empty) |
| `auto_route` | Create proxy routes automatically | `true` |
| `auto_start` | Start containers after deployment | `true` |

---

## Catalog Structure

The Toolbox catalog is organized as a directory of service subdirectories, each containing a `docker-compose.yml` file:

```
toolbox/
├── grafana/
│   └── docker-compose.yml
├── vaultwarden/
│   └── docker-compose.yml
├── memos/
│   └── docker-compose.yml
├── n8n/
│   └── docker-compose.yml
├── syncthing/
│   └── docker-compose.yml
├── dozzle/
│   └── docker-compose.yml
└── ...
```

Each subdirectory name becomes the service ID used in the API.

### Available Services

Nekzus includes a growing catalog of pre-configured services:

| Service | Category | Description |
|---------|----------|-------------|
| Grafana | monitoring | Metrics visualization dashboards |
| Dozzle | monitoring | Real-time Docker log viewer |
| Vaultwarden | security | Bitwarden-compatible password manager |
| Memos | productivity | Privacy-first note-taking service |
| Outline | productivity | Team knowledge base and wiki |
| n8n | automation | Workflow automation platform |
| Syncthing | storage | P2P file synchronization |
| Vikunja | productivity | Open-source task management |
| Gitea | development | Self-hosted Git service |
| PhotoPrism | media | AI-powered photo management |
| AdGuard Home | network | Network-wide ad blocking |

---

## Creating a Toolbox Service

To add a new service to the Toolbox catalog:

### Step 1: Create Directory Structure

```bash
mkdir toolbox/myservice
```

### Step 2: Create docker-compose.yml

Create a Docker Compose file with Toolbox labels:

```yaml
services:
  myservice:
    image: vendor/myservice:latest
    container_name: ${SERVICE_NAME:-myservice}
    ports:
      - "${APP_PORT:-8080}:8080"
    volumes:
      - myservice-data:/data
    environment:
      ADMIN_PASSWORD: "${ADMIN_PASSWORD:?Required}"
      BASE_URL: "${BASE_URL}"
    restart: unless-stopped
    networks:
      - nekzus-test-network
    labels:
      # Toolbox metadata (required)
      nekzus.toolbox.name: "My Service"
      nekzus.toolbox.category: "productivity"
      nekzus.toolbox.description: "Brief description of the service"

      # Toolbox metadata (optional)
      nekzus.toolbox.icon: "https://example.com/icon.png"
      nekzus.toolbox.tags: "tag1,tag2,tag3"
      nekzus.toolbox.documentation: "https://docs.example.com"
      nekzus.toolbox.image_url: "https://hub.docker.com/r/vendor/myservice"
      nekzus.toolbox.repository_url: "https://github.com/vendor/myservice"

      # Discovery labels (for auto-routing)
      nekzus.enable: "true"
      nekzus.app.id: "myservice"
      nekzus.app.name: "My Service"
      nekzus.route.path: "/apps/myservice/"
      nekzus.route.strip_prefix: "true"

networks:
  nekzus-test-network:
    external: true

volumes:
  myservice-data:
    driver: local
    labels:
      nekzus.toolbox: "true"
```

---

## Toolbox Labels Reference

### Required Labels

| Label | Description | Example |
|-------|-------------|---------|
| `nekzus.toolbox.name` | Display name for the service | `"Grafana"` |
| `nekzus.toolbox.category` | Service category | `"monitoring"` |
| `nekzus.toolbox.description` | Brief description | `"Metrics visualization"` |

### Optional Labels

| Label | Description | Example |
|-------|-------------|---------|
| `nekzus.toolbox.icon` | Icon URL (CDN or direct link) | `"https://cdn.../grafana.png"` |
| `nekzus.toolbox.tags` | Comma-separated search tags | `"monitoring,metrics,dashboards"` |
| `nekzus.toolbox.documentation` | Link to official docs | `"https://grafana.com/docs/"` |
| `nekzus.toolbox.image_url` | Docker Hub or registry page | `"https://hub.docker.com/..."` |
| `nekzus.toolbox.repository_url` | Source code repository | `"https://github.com/..."` |

### Discovery Labels (Auto-Routing)

| Label | Description | Example |
|-------|-------------|---------|
| `nekzus.enable` | Enable auto-discovery | `"true"` |
| `nekzus.app.id` | Unique application ID | `"grafana"` |
| `nekzus.app.name` | Display name | `"Grafana"` |
| `nekzus.app.icon` | Icon URL for dashboard | `"https://..."` |
| `nekzus.route.path` | Route path prefix | `"/apps/grafana/"` |
| `nekzus.route.strip_prefix` | Strip prefix before proxying | `"true"` |
| `nekzus.route.rewrite_html` | Rewrite HTML for sub-path hosting | `"true"` |

---

## Environment Variables

Toolbox services use environment variable substitution for dynamic configuration.

### Variable Syntax

| Syntax | Description | Example |
|--------|-------------|---------|
| `${VAR}` | Simple substitution | `${ADMIN_USER}` |
| `${VAR:-default}` | Default value if not set | `${APP_PORT:-8080}` |
| `${VAR:?error}` | Required, error if not set | `${SECRET:?Required}` |

### Auto-Injected Variables

Nekzus automatically injects these variables at deployment time:

| Variable | Description | Example Value |
|----------|-------------|---------------|
| `SERVICE_NAME` | Container name | `"my-grafana"` |
| `BASE_URL` | Nekzus base URL | `"https://192.168.1.100:8443"` |
| `APP_PORT` | Custom port (if specified) | `"3001"` |

### Variable Type Inference

Environment variables are automatically classified by type for the UI:

- **Password fields**: Variables containing `password`, `secret`, or `token`
- **Number fields**: Variables containing `port`
- **URL fields**: Variables containing `url` or `host`
- **Text fields**: All other variables

### Example: Environment Configuration

```yaml
environment:
  # Simple variable with default
  GF_SERVER_HTTP_PORT: "3000"

  # Variable from user input with default
  GF_PORT: "${GF_PORT:-3000}"

  # Required password (no default)
  GF_ADMIN_PASSWORD: "${GF_ADMIN_PASSWORD:?Admin password is required}"

  # Uses auto-injected BASE_URL
  GF_SERVER_ROOT_URL: "${BASE_URL}/apps/grafana/"

  # Optional with empty default
  GF_INSTALL_PLUGINS: "${GF_INSTALL_PLUGINS:-}"
```

---

## Deployment Workflow

### 1. Browse Catalog

Use the web dashboard or API to browse available services:

<Tabs>
<TabItem value="web-dashboard" label="Web Dashboard">

Navigate to **Toolbox** in the sidebar to view the service catalog with filtering by category.

</TabItem>
<TabItem value="api" label="API">

```bash
curl https://localhost:8443/api/v1/toolbox/services
```

</TabItem>
</Tabs>

### 2. Configure Deployment

Select a service and configure:

- **Service Name**: Custom name for your deployment
- **Environment Variables**: Required and optional configuration
- **Custom Port**: Override the default host port (optional)
- **Custom Image**: Use a different Docker image version (optional)

### 3. Deploy

<Tabs>
<TabItem value="web-dashboard" label="Web Dashboard">

Click **Deploy** to start the deployment. Progress is shown in real-time.

</TabItem>
<TabItem value="api" label="API">

```bash
curl -X POST https://localhost:8443/api/v1/toolbox/deploy \
  -H "Content-Type: application/json" \
  -d '{
    "service_id": "grafana",
    "service_name": "my-grafana",
    "env_vars": {
      "GF_ADMIN_PASSWORD": "secure-password-here"
    },
    "auto_start": true
  }'
```

</TabItem>
</Tabs>

### 4. Monitor Status

Track deployment progress:

```bash
curl https://localhost:8443/api/v1/toolbox/deployments/{deployment_id}
```

Deployment statuses:

| Status | Description |
|--------|-------------|
| `pending` | Deployment created, waiting to start |
| `deploying` | Docker operations in progress |
| `deployed` | Service running successfully |
| `failed` | Deployment failed (check error_message) |
| `stopped` | Service stopped |

---

## Auto-Routing

When `auto_route: true` is enabled, deployed services are automatically:

1. **Discovered** by Docker discovery when the container starts
2. **Registered** with the appropriate route based on discovery labels
3. **Proxied** through Nekzus at the configured path

### Route Configuration via Labels

```yaml
labels:
  nekzus.enable: "true"              # Enable discovery
  nekzus.app.id: "grafana"           # Unique app ID
  nekzus.route.path: "/apps/grafana/" # Route path
  nekzus.route.strip_prefix: "true"  # Remove path prefix
```

After deployment, access the service at:

```
https://your-nekzus-host:8443/apps/grafana/
```

---

## Managing Deployed Services

### List Deployments

<Tabs>
<TabItem value="all-deployments" label="All Deployments">

```bash
curl https://localhost:8443/api/v1/toolbox/deployments
```

</TabItem>
<TabItem value="filter-by-status" label="Filter by Status">

```bash
curl https://localhost:8443/api/v1/toolbox/deployments?status=deployed
```

</TabItem>
</Tabs>

### Get Deployment Details

```bash
curl https://localhost:8443/api/v1/toolbox/deployments/{deployment_id}
```

### Remove Deployment

<Tabs>
<TabItem value="keep-volumes" label="Keep Volumes">

```bash
curl -X DELETE https://localhost:8443/api/v1/toolbox/deployments/{deployment_id}
```

</TabItem>
<TabItem value="remove-volumes" label="Remove Volumes">

```bash
curl -X DELETE \
  "https://localhost:8443/api/v1/toolbox/deployments/{deployment_id}?removeVolumes=true"
```

</TabItem>
</Tabs>

:::warning[Data Loss]

Using `removeVolumes=true` permanently deletes all data stored in the service's volumes.

:::


---

## API Reference

### Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/toolbox/services` | List all available services |
| `GET` | `/api/v1/toolbox/services?category=monitoring` | Filter services by category |
| `GET` | `/api/v1/toolbox/services/{id}` | Get service details |
| `POST` | `/api/v1/toolbox/deploy` | Deploy a service |
| `GET` | `/api/v1/toolbox/deployments` | List all deployments |
| `GET` | `/api/v1/toolbox/deployments?status=deployed` | Filter deployments by status |
| `GET` | `/api/v1/toolbox/deployments/{id}` | Get deployment status |
| `DELETE` | `/api/v1/toolbox/deployments/{id}` | Remove deployment |

### Request: Deploy Service

```json
{
  "service_id": "grafana",
  "service_name": "my-grafana",
  "env_vars": {
    "GF_ADMIN_PASSWORD": "secure-password"
  },
  "auto_start": true,
  "custom_port": 3001,
  "custom_image": "grafana/grafana:10.0.0"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `service_id` | string | Yes | Template ID from catalog |
| `service_name` | string | Yes | Custom name for deployment |
| `env_vars` | object | No | Environment variables |
| `auto_start` | boolean | No | Start container after creation |
| `custom_port` | integer | No | Override default host port |
| `custom_image` | string | No | Override Docker image |

### Response: Deployment Created

```json
{
  "deployment_id": "deploy_1703505600000000000",
  "status": "pending",
  "message": "Deployment 'my-grafana' initiated"
}
```

### Response: Deployment Status

```json
{
  "id": "deploy_1703505600000000000",
  "service_template_id": "grafana",
  "service_name": "my-grafana",
  "status": "deployed",
  "project_name": "deploy_1703505600000000000",
  "env_vars": {
    "GF_ADMIN_PASSWORD": "***"
  },
  "deployed_at": "2024-12-25T10:00:00Z",
  "created_at": "2024-12-25T09:59:55Z",
  "updated_at": "2024-12-25T10:00:00Z"
}
```

---

## Example Service Configurations

### Monitoring: Grafana

```yaml
services:
  grafana:
    image: grafana/grafana:latest
    container_name: ${SERVICE_NAME:-grafana}
    ports:
      - "${GF_PORT:-3000}:3000"
    volumes:
      - grafana-data:/var/lib/grafana
    environment:
      GF_SERVER_ROOT_URL: "${BASE_URL}/apps/grafana/"
      GF_SECURITY_ADMIN_PASSWORD: "${GF_ADMIN_PASSWORD}"
      GF_INSTALL_PLUGINS: "${GF_INSTALL_PLUGINS:-}"
    restart: unless-stopped
    networks:
      - nekzus-test-network
    labels:
      nekzus.toolbox.name: "Grafana"
      nekzus.toolbox.icon: "https://cdn.jsdelivr.net/gh/walkxcode/dashboard-icons/png/grafana.png"
      nekzus.toolbox.category: "monitoring"
      nekzus.toolbox.tags: "monitoring,metrics,dashboards,visualization"
      nekzus.toolbox.description: "Beautiful monitoring and analytics dashboards"
      nekzus.toolbox.documentation: "https://grafana.com/docs/grafana/latest/"

      nekzus.enable: "true"
      nekzus.app.id: "grafana"
      nekzus.app.name: "Grafana"
      nekzus.route.path: "/apps/grafana/"
      nekzus.route.strip_prefix: "true"

networks:
  nekzus-test-network:
    external: true

volumes:
  grafana-data:
    driver: local
    labels:
      nekzus.toolbox: "true"
```

### Security: Vaultwarden

```yaml
services:
  vaultwarden:
    image: vaultwarden/server:latest
    container_name: ${SERVICE_NAME:-vaultwarden}
    ports:
      - "${VAULTWARDEN_PORT:-8080}:80"
    volumes:
      - vaultwarden-data:/data
    environment:
      DOMAIN: "${BASE_URL}/apps/vaultwarden"
      SIGNUPS_ALLOWED: "${SIGNUPS_ALLOWED:-true}"
      ADMIN_TOKEN: "${ADMIN_TOKEN:?Required - generate with openssl rand -base64 48}"
      WEBSOCKET_ENABLED: "true"
    restart: unless-stopped
    networks:
      - nekzus-test-network
    labels:
      nekzus.toolbox.name: "Vaultwarden"
      nekzus.toolbox.icon: "https://cdn.jsdelivr.net/gh/walkxcode/dashboard-icons/png/vaultwarden.png"
      nekzus.toolbox.category: "security"
      nekzus.toolbox.tags: "passwords,security,bitwarden,vault,secrets"
      nekzus.toolbox.description: "Lightweight Bitwarden-compatible password manager"
      nekzus.toolbox.documentation: "https://github.com/dani-garcia/vaultwarden/wiki"

      nekzus.enable: "true"
      nekzus.app.id: "vaultwarden"
      nekzus.app.name: "Vaultwarden"
      nekzus.route.path: "/apps/vaultwarden/"
      nekzus.route.strip_prefix: "true"

networks:
  nekzus-test-network:
    external: true

volumes:
  vaultwarden-data:
    driver: local
    labels:
      nekzus.toolbox: "true"
```

### Automation: n8n

```yaml
services:
  n8n:
    image: n8nio/n8n:latest
    container_name: ${SERVICE_NAME:-n8n}
    ports:
      - "${N8N_PORT:-5678}:5678"
    volumes:
      - n8n-data:/home/node/.n8n
    environment:
      N8N_HOST: "0.0.0.0"
      N8N_PORT: "5678"
      WEBHOOK_URL: "${BASE_URL}/apps/n8n/"
      N8N_EDITOR_BASE_URL: "${BASE_URL}/apps/n8n/"
      N8N_BASIC_AUTH_ACTIVE: "true"
      N8N_BASIC_AUTH_USER: "${N8N_USER:-admin}"
      N8N_BASIC_AUTH_PASSWORD: "${N8N_PASSWORD:?Required}"
      GENERIC_TIMEZONE: "${TIMEZONE:-UTC}"
    restart: unless-stopped
    networks:
      - nekzus-test-network
    labels:
      nekzus.toolbox.name: "n8n"
      nekzus.toolbox.icon: "https://cdn.jsdelivr.net/gh/walkxcode/dashboard-icons/png/n8n.png"
      nekzus.toolbox.category: "automation"
      nekzus.toolbox.tags: "automation,workflow,integration,zapier-alternative"
      nekzus.toolbox.description: "Workflow automation tool"
      nekzus.toolbox.documentation: "https://docs.n8n.io/"

      nekzus.enable: "true"
      nekzus.app.id: "n8n"
      nekzus.app.name: "n8n"
      nekzus.route.path: "/apps/n8n/"
      nekzus.route.strip_prefix: "true"

networks:
  nekzus-test-network:
    external: true

volumes:
  n8n-data:
    driver: local
    labels:
      nekzus.toolbox: "true"
```

---

## Service Categories

Organize services into categories for easier browsing:

| Category | Description | Examples |
|----------|-------------|----------|
| `monitoring` | Metrics, logs, observability | Grafana, Dozzle, Prometheus |
| `productivity` | Notes, tasks, knowledge bases | Memos, Outline, Vikunja |
| `security` | Password managers, secrets | Vaultwarden |
| `automation` | Workflows, integrations | n8n, Huginn |
| `storage` | File sync, backup | Syncthing, Nextcloud |
| `media` | Photos, videos, music | PhotoPrism, Jellyfin |
| `development` | Git, CI/CD, IDEs | Gitea, Drone |
| `network` | DNS, VPN, ad-blocking | AdGuard Home, Pi-hole |

---

## Best Practices

### Security

:::warning[Secure Secrets]

Always use `${VAR:?error}` syntax for sensitive values to ensure they are provided at deployment time. Never commit secrets to version control.

:::


- Use strong passwords with the `:?` required syntax
- Enable HTTPS for services handling sensitive data
- Regularly update container images
- Review security notes in service templates

### Networking

- Use external networks for inter-service communication
- Configure `BASE_URL` correctly for reverse proxy setups
- Test routes after deployment

### Data Persistence

- Use named volumes for persistent data
- Back up volume data regularly
- Label volumes with `nekzus.toolbox: "true"` for identification

### Resource Management

- Monitor container resource usage
- Set memory limits for resource-intensive services
- Use appropriate restart policies

---

## Troubleshooting

### Deployment Fails with "Port Conflict"

The requested port is already in use. Solutions:

1. Use `custom_port` to specify a different port
2. Stop the conflicting service
3. Check with `docker ps` for running containers

### Service Not Accessible After Deployment

1. Verify the container is running: `docker ps`
2. Check container logs: `docker logs <container_name>`
3. Confirm discovery labels are correct
4. Verify the route was created in Nekzus

### Environment Variable Not Substituted

1. Ensure variable name matches exactly (case-sensitive)
2. Check for typos in `${VAR}` syntax
3. Verify the variable is passed in `env_vars` during deployment

### Container Keeps Restarting

1. Check container logs for errors
2. Verify required environment variables are set
3. Ensure volume paths exist and are writable
4. Check for port conflicts

---

## Related Documentation

- [Discovery](discovery) - Learn about service auto-discovery
- [Configuration Reference](../reference/configuration) - Full configuration options
- [API Reference](../reference/api) - Complete API documentation
- [Docker Compose Guide](../guides/docker-compose) - Deployment best practices
