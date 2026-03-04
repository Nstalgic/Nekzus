import Tabs from "@theme/Tabs";
import TabItem from "@theme/TabItem";

# Service Discovery

Nekzus provides automatic service discovery to find and catalog services running on your local network. The discovery system supports multiple sources including Docker containers, mDNS/Bonjour services, and Kubernetes clusters.

---

## Overview

The discovery system uses a **proposal-based workflow** where discovered services are presented as proposals for administrator approval before being added to the service catalog and route table.

### Architecture

```d2
direction: right

workers: Discovery Workers {
  grid-columns: 1
  docker: Docker
  mdns: mDNS
  k8s: Kubernetes
}

manager: Discovery Manager {
  grid-columns: 1
  dedup: Deduplication
  confidence: Confidence Scoring
  events: Event Publishing
  dismissed: Dismissed Tracking
  notify: WebSocket Notify
}

store: Proposal Store {
  style.fill: "#7c3aed"
  style.font-color: "#ffffff"
}

workers -> manager -> store
```

### Key Features

- **Multi-source discovery**: Docker, mDNS, and Kubernetes
- **Confidence scoring**: Prioritizes explicitly labeled services
- **Proposal workflow**: Review before adding to catalog
- **Deduplication**: Prevents duplicate proposals
- **Self-exclusion**: Automatically excludes Nekzus containers
- **Network filtering**: Fine-grained Docker network control
- **Label inheritance**: Kubernetes namespace-level configuration
- **WebSocket notifications**: Real-time proposal updates

---

## Discovery Sources

Nekzus supports three discovery sources, each with its own worker process.

<Tabs>
<TabItem value="docker" label="Docker">


Docker discovery scans running containers using the Docker API and creates proposals based on container labels and exposed ports.

**Features:**

- Automatic HTTP port detection via probing
- Multi-network support with filtering
- Self-container exclusion
- System container filtering
- Label-based configuration

</TabItem>
<TabItem value="mdns" label="mDNS">


mDNS (Bonjour/Zeroconf) discovery scans for services advertising themselves on the local network.

**Features:**

- Scans configurable service types
- TXT record metadata extraction
- IPv4 and IPv6 support
- Periodic scanning interval

:::note[Implementation Status]

mDNS discovery requires integration with an mDNS library (e.g., `github.com/hashicorp/mdns`). The worker starts but does not discover services until a library is integrated.

:::

</TabItem>
<TabItem value="kubernetes" label="Kubernetes">


Kubernetes discovery scans Services and Ingresses across specified namespaces.

**Features:**

- Namespace filtering and label inheritance
- Ingress-based discovery with TLS detection
- Istio/Service Mesh detection
- Helm chart recognition (20+ charts)
- Smart label inference
- Backend service filtering

</TabItem>
</Tabs>

---

## Docker Discovery

Docker discovery is the most commonly used discovery method, automatically finding services running in Docker containers.

### Configuration

```yaml
discovery:
  enabled: true
  docker:
    enabled: true
    socket_path: "unix:///var/run/docker.sock"
    poll_interval: "30s"

    # Network mode: "all" | "first" | "preferred"
    network_mode: "all"

    # Optional: Only discover on specific networks
    networks:
      - "app-network"
      - "web-tier"

    # Optional: Exclude specific networks
    exclude_networks:
      - "bridge"
      - "host"
```

### Configuration Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | boolean | `true` | Enable Docker discovery |
| `socket_path` | string | `unix:///var/run/docker.sock` | Docker socket path |
| `poll_interval` | duration | `30s` | How often to scan for containers |
| `network_mode` | string | `all` | Network selection mode |
| `networks` | []string | `[]` | Include only these networks |
| `exclude_networks` | []string | `[]` | Exclude these networks |

### Network Modes

| Mode | Behavior |
|------|----------|
| `all` | Use all container networks (after filtering) |
| `first` | Use only the first available network |
| `preferred` | Use first network from `networks` list, fallback to first available |

### Container Labels Reference

Add these labels to your Docker containers to control discovery behavior:

#### Core Labels

| Label | Type | Default | Description |
|-------|------|---------|-------------|
| `nekzus.enable` | boolean | - | Explicitly enable (`true`) or disable (`false`) discovery |
| `nekzus.app.id` | string | container name | Unique application identifier |
| `nekzus.app.name` | string | container name | Display name in catalog |
| `nekzus.app.icon` | string | - | Icon URL or emoji |
| `nekzus.app.tags` | string | auto-generated | Comma-separated tags |

#### Route Labels

| Label | Type | Default | Description |
|-------|------|---------|-------------|
| `nekzus.route.path` | string | `/apps/{app_id}/` | Proxy path base |
| `nekzus.route.scopes` | string | `access:{app_id}` | Comma-separated required scopes |
| `nekzus.route.strip_prefix` | boolean | `true` | Strip path prefix before proxying |
| `nekzus.route.rewrite_html` | boolean | `true` | Rewrite HTML for SPA support |
| `nekzus.route.websocket` | boolean | `false` | Enable WebSocket proxying |
| `nekzus.scheme` | string | auto-detected | Protocol: `http` or `https` |

#### Port Discovery Labels

| Label | Type | Default | Description |
|-------|------|---------|-------------|
| `nekzus.primary_port` | integer | - | Discover only this specific port |
| `nekzus.discover.all_ports` | boolean | `false` | Discover all TCP ports (bypass HTTP filter) |

#### System Labels

| Label | Type | Description |
|-------|------|-------------|
| `nekzus.skip` | boolean | Skip this container entirely |
| `nekzus.test` | string | Mark as test container (bypasses system container filter) |

### Docker Compose Example

```yaml
version: "3.8"

services:
  grafana:
    image: grafana/grafana:latest
    container_name: grafana
    ports:
      - "3000:3000"
    labels:
      # Enable discovery
      nekzus.enable: "true"

      # Application metadata
      nekzus.app.id: "grafana"
      nekzus.app.name: "Grafana Dashboard"
      nekzus.app.icon: "https://grafana.com/favicon.ico"
      nekzus.app.tags: "monitoring,metrics,dashboard"

      # Route configuration
      nekzus.route.path: "/apps/grafana/"
      nekzus.route.scopes: "access:grafana,read:metrics"
      nekzus.route.websocket: "true"
      nekzus.route.strip_prefix: "true"

      # Scheme override
      nekzus.scheme: "http"
    networks:
      - monitoring

  # Multi-port service example
  api-service:
    image: my-api:latest
    ports:
      - "8080:8080"
      - "9090:9090"
    labels:
      nekzus.enable: "true"
      nekzus.app.id: "api"
      nekzus.app.name: "API Service"
      # Only discover port 8080 (skip metrics port 9090)
      nekzus.primary_port: "8080"

networks:
  monitoring:
    driver: bridge
```

### HTTP Port Detection

Docker discovery uses HTTP probing to detect web services on non-standard ports:

1. **Known non-HTTP ports are skipped**: SSH (22), MySQL (3306), PostgreSQL (5432), Redis (6379), etc.
2. **HTTP HEAD requests** are sent to candidate ports
3. **Any HTTP response** (including 4xx/5xx) indicates an HTTP service
4. **Timeout**: 2 seconds per probe

:::tip[Override Port Detection]

Use `nekzus.primary_port` to specify exactly which port to use, or `nekzus.discover.all_ports: "true"` to discover all TCP ports regardless of protocol.

:::


### Confidence Scoring

Docker discovery assigns confidence scores based on container metadata:

| Condition | Score |
|-----------|-------|
| Base score (any container) | 0.50 |
| Has `nekzus.app.id` label | 0.85 |
| Has `nekzus.enable: "true"` | 0.95 |
| Well-known image (nginx, grafana, etc.) | +0.20 |
| Common web ports (80, 8080, 3000) | +0.10 |
| Traefik/Caddy labels present | +0.15 |

Maximum score: 1.0

---

## mDNS Discovery

mDNS (Multicast DNS) discovery finds services advertising themselves via Bonjour/Zeroconf on the local network.

### Configuration

```yaml
discovery:
  enabled: true
  mdns:
    enabled: true
    scan_interval: "60s"
    services:
      - "_http._tcp"
      - "_https._tcp"
      - "_homeassistant._tcp"
```

### Configuration Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | boolean | `true` | Enable mDNS discovery |
| `scan_interval` | duration | `60s` | How often to scan for services |
| `services` | []string | see below | Service types to discover |

### Default Service Types

When no services are specified, the following types are scanned:

- `_http._tcp` - HTTP services
- `_https._tcp` - HTTPS services
- `_ssh._tcp` - SSH services
- `_smb._tcp` - SMB/CIFS file shares
- `_printer._tcp` - Network printers
- `_workstation._tcp` - Workstations

### TXT Record Metadata

mDNS services can provide metadata via TXT records:

| TXT Key | Description |
|---------|-------------|
| `app_id` | Application identifier |
| `app_name` | Display name |
| `path` | Route path base |
| `icon` | Icon URL |
| `tags` | Comma-separated tags |
| `scopes` | Required scopes |
| `scheme` | Protocol override |
| `nekzus_enable` | Explicit enable flag |

### Confidence Scoring (mDNS)

| Condition | Score |
|-----------|-------|
| Base score (any mDNS service) | 0.70 |
| Has `app_id` TXT record | 0.85 |
| Has `nekzus_enable: "true"` | 0.95 |
| Known service type (Home Assistant, HomeKit) | +0.10 |

---

## Kubernetes Discovery

Kubernetes discovery finds Services and Ingresses in your cluster, with smart inference for common patterns.

### Configuration

```yaml
discovery:
  enabled: true
  kubernetes:
    enabled: true
    kubeconfig: ""  # Empty for in-cluster or default kubeconfig
    poll_interval: "30s"
    namespaces:
      - "default"
      - "apps"
      - "production"
```

### Configuration Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | boolean | `false` | Enable Kubernetes discovery |
| `kubeconfig` | string | `""` | Path to kubeconfig (empty for auto-detect) |
| `poll_interval` | duration | `30s` | How often to scan |
| `namespaces` | []string | `[""]` | Namespaces to watch (empty = all) |

### Labels and Annotations

Kubernetes services and ingresses support the same labels as Docker, applied as Kubernetes labels or annotations:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: grafana
  namespace: monitoring
  labels:
    nekzus.enable: "true"
    nekzus.app.id: "grafana"
  annotations:
    nekzus.app.name: "Grafana Dashboard"
    nekzus.app.tags: "monitoring,metrics"
    nekzus.route.path: "/apps/grafana/"
spec:
  type: ClusterIP
  ports:
    - port: 3000
  selector:
    app: grafana
```

:::note[Labels vs Annotations]

Use **annotations** for values that may contain special characters or exceed label length limits. Labels are checked first, then annotations.

:::


### Namespace-Level Configuration

Namespaces can provide default configuration that services inherit:

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: production
  labels:
    nekzus.enable: "true"
    nekzus.app.tags: "production"
    nekzus.route.scopes: "access:production"
    istio-injection: enabled
```

Services in this namespace automatically inherit:

- Enabled status (unless explicitly disabled)
- Tags (`production`)
- Scopes (`access:production`)
- Istio detection

### Auto-Detection Features

Kubernetes discovery automatically detects services based on several criteria:

#### Service Type Detection

| Service Type | Auto-Discovered When |
|--------------|---------------------|
| LoadBalancer | Has identifying labels (`app.kubernetes.io/name`, etc.) |
| NodePort | Has identifying labels |
| ClusterIP | Has frontend/UI component labels |

#### Component Detection

Services with these `app.kubernetes.io/component` values are auto-discovered:

- `frontend`
- `ui`
- `web`
- `dashboard`

#### Helm Chart Recognition

The following Helm charts are automatically recognized and tagged:

| Chart | Tags Added |
|-------|------------|
| `grafana` | `monitoring`, `grafana` |
| `prometheus` | `monitoring`, `prometheus` |
| `argocd` | `cicd`, `argocd` |
| `loki` | `logging`, `loki` |
| `tempo` | `tracing`, `tempo` |
| `jaeger` | `tracing`, `jaeger` |
| `jenkins` | `cicd`, `jenkins` |
| `gitlab` | `cicd`, `gitlab` |
| `harbor` | `registry`, `harbor` |
| `vault` | `secrets`, `vault` |
| `consul` | `service-mesh`, `consul` |
| `linkerd` | `service-mesh`, `linkerd` |

#### Istio/Service Mesh Detection

Services with Istio sidecar injection are automatically discovered:

- Namespace label: `istio-injection: enabled`
- Service label: `istio-injection: enabled`
- Service label: `istio.io/rev`
- Service label: `service.istio.io/canonical-name`
- Annotation: `sidecar.istio.io/inject: "true"`

#### Backend Service Filtering

These chart types are NOT auto-discovered (considered backend services):

- Databases: `postgresql`, `mysql`, `mariadb`, `mongodb`, `cassandra`
- Caches: `redis`, `memcached`
- Message queues: `kafka`, `rabbitmq`, `nats`
- Infrastructure: `etcd`, `consul`, `vault`, `elasticsearch`

### Ingress Discovery

Ingresses are discovered when they have:

- Explicit `nekzus.enable: "true"` label
- Standard Kubernetes labels (`app.kubernetes.io/name`)

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: grafana
  namespace: monitoring
  labels:
    nekzus.enable: "true"
  annotations:
    nekzus.app.name: "Grafana"
spec:
  tls:
    - hosts:
        - grafana.example.com
      secretName: grafana-tls
  rules:
    - host: grafana.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: grafana
                port:
                  number: 3000
```

### Confidence Scoring (Kubernetes)

| Condition | Score |
|-----------|-------|
| Base score (any service) | 0.50 |
| Has `nekzus.app.id` label | 0.85 |
| Has `nekzus.enable: "true"` | 0.95 |
| Ingress-exposed service | 0.80 |
| Ingress with explicit enable | 0.95 |
| Istio/Service Mesh detected | +0.20 |
| Recognized Helm chart | +0.20 |
| LoadBalancer/NodePort type | +0.10 |
| Common web ports | +0.10 |
| TLS configured on Ingress | +0.05 |

### Skipped Namespaces

The following system namespaces are always skipped unless services explicitly enable discovery:

- `kube-system`
- `kube-public`
- `kube-node-lease`

---

## Proposal Workflow

All discovered services go through a proposal workflow before being added to the catalog.

### Proposal States

```d2
direction: right

start: {
  shape: circle
  style.fill: "#000"
}
pending: Pending {
  style.fill: "#fef3c7"
}
active: Active {
  style.fill: "#d1fae5"
}
dismissed: Dismissed {
  style.fill: "#fee2e2"
}

start -> pending: Discovered
pending -> active: Approve
pending -> dismissed: Dismiss
```

### Proposal Lifecycle

1. **Discovery**: Worker finds a service
2. **Deduplication**: Manager checks if proposal already exists
3. **Pending**: Proposal awaits administrator action
4. **Action**:
   - **Approve**: Creates app and route, removes proposal
   - **Dismiss**: Marks as dismissed, prevents re-discovery

### Proposal Structure

```json
{
  "id": "proposal_docker_http_grafana_3000",
  "source": "docker",
  "detectedScheme": "http",
  "detectedHost": "172.17.0.5",
  "detectedPort": 3000,
  "availablePorts": [
    {"port": 3000, "scheme": "http"}
  ],
  "confidence": 0.95,
  "suggestedApp": {
    "id": "grafana",
    "name": "Grafana Dashboard",
    "icon": "https://grafana.com/favicon.ico",
    "tags": ["monitoring", "docker"],
    "endpoints": {
      "lan": "http://172.17.0.5:3000"
    }
  },
  "suggestedRoute": {
    "routeId": "route:grafana",
    "appId": "grafana",
    "pathBase": "/apps/grafana/",
    "to": "http://172.17.0.5:3000",
    "scopes": ["access:grafana"],
    "stripPrefix": true
  },
  "securityNotes": [
    "Discovered via Docker API",
    "JWT required",
    "Private network address"
  ]
}
```

### Rediscovery

To clear dismissed proposals and trigger a fresh discovery scan:

```bash
curl -X POST https://localhost:8443/api/v1/discovery/rediscover \
  -H "Authorization: Bearer $TOKEN"
```

This clears both dismissed and pending proposals, allowing previously dismissed services to be rediscovered.

---

## API Reference

### List Proposals

Returns all pending discovery proposals.

```http
GET /api/v1/discovery/proposals
Authorization: Bearer <token>
```

**Response:**

```json
[
  {
    "id": "proposal_docker_http_grafana_3000",
    "source": "docker",
    "confidence": 0.95,
    ...
  }
]
```

### Approve Proposal

Approves a proposal, creating the app and route.

```http
POST /api/v1/discovery/proposals/{proposalId}/approve
Authorization: Bearer <token>
Content-Type: application/json

{
  "port": 3000  // Optional: select specific port from availablePorts
}
```

**Response:**

```json
{
  "status": "approved",
  "id": "proposal_docker_http_grafana_3000",
  "app": { ... },
  "route": { ... }
}
```

### Dismiss Proposal

Dismisses a proposal, preventing it from reappearing.

```http
POST /api/v1/discovery/proposals/{proposalId}/dismiss
Authorization: Bearer <token>
```

**Response:**

```json
{
  "status": "dismissed",
  "id": "proposal_docker_http_grafana_3000"
}
```

### Trigger Rediscovery

Clears all proposals and triggers a fresh scan.

```http
POST /api/v1/discovery/rediscover
Authorization: Bearer <token>
```

**Response:**

```json
{
  "status": "success",
  "message": "Rediscovery triggered. Discovery workers will scan for new services.",
  "dismissedCleared": 5,
  "activeCleared": 2
}
```

---

## Troubleshooting

### Docker Discovery Issues

<details>
<summary>Docker discovery not finding containers</summary>


**Check Docker socket access:**

```bash
# Verify socket path
ls -la /var/run/docker.sock

# Test Docker API access
curl --unix-socket /var/run/docker.sock http://localhost/containers/json
```

**Check discovery configuration:**

```yaml
discovery:
  enabled: true
  docker:
    enabled: true
    socket_path: "unix:///var/run/docker.sock"
```

**Check logs for errors:**

```bash
docker logs nekzus 2>&1 | grep -i discovery
```

</details>


<details>
<summary>Container discovered but wrong port detected</summary>


Use explicit port labeling:

```yaml
labels:
  nekzus.primary_port: "8080"
```

Or discover all ports:

```yaml
labels:
  nekzus.discover.all_ports: "true"
```

</details>


<details>
<summary>Nekzus container appearing in proposals</summary>


The system container filter should exclude Nekzus containers automatically. If not:

1. Check container name contains `nekzus` or `nekzus`
2. Add explicit skip label:

```yaml
labels:
  nekzus.skip: "true"
```

</details>


<details>
<summary>Network filtering not working</summary>


Verify network names match exactly:

```bash
docker network ls
```

Check configuration:

```yaml
discovery:
  docker:
    networks:
      - "my-network"  # Must match exactly
    network_mode: "preferred"
```

</details>


### Kubernetes Discovery Issues

<details>
<summary>Kubernetes discovery not connecting</summary>


**Check kubeconfig:**

```bash
# Verify kubectl works
kubectl get services --all-namespaces

# Check kubeconfig path
echo $KUBECONFIG
ls -la ~/.kube/config
```

**For in-cluster deployment:**

Ensure the ServiceAccount has proper RBAC permissions:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: nekzus-discovery
rules:
  - apiGroups: [""]
    resources: ["services", "namespaces"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["networking.k8s.io"]
    resources: ["ingresses"]
    verbs: ["get", "list", "watch"]
```

</details>


<details>
<summary>Services not being discovered in Kubernetes</summary>


1. Check if namespace is in the watch list
2. Verify service has identifying labels
3. Check if it's a system namespace (kube-system)
4. Add explicit enable label:

```yaml
labels:
  nekzus.enable: "true"
```

</details>


<details>
<summary>Ingress discovery not working</summary>


Ingresses need one of:

- `nekzus.enable: "true"` label
- `app.kubernetes.io/name` label

```yaml
metadata:
  labels:
    nekzus.enable: "true"
```

</details>


### General Issues

<details>
<summary>Proposals keep reappearing after dismissal</summary>


Proposals use deterministic IDs based on source, host, and port. If the ID changes, a new proposal is created.

**Common causes:**

- Container IP changed (use container name instead)
- Port changed
- Discovery configuration changed

**Solution:**

Use stable identifiers like container names or add explicit labels.

</details>


<details>
<summary>High CPU usage from discovery</summary>


Increase poll intervals:

```yaml
discovery:
  docker:
    poll_interval: "60s"  # Increase from 30s
  kubernetes:
    poll_interval: "60s"
  mdns:
    scan_interval: "120s"  # Increase from 60s
```

</details>


<details>
<summary>WebSocket not receiving discovery events</summary>


1. Verify WebSocket connection is established
2. Check authentication token is valid
3. Monitor WebSocket for `proposal_approved`, `proposal_dismissed` events

```javascript
ws.onmessage = (event) => {
  const msg = JSON.parse(event.data);
  if (msg.type === 'discovery' || msg.type === 'proposal_approved') {
    console.log('Discovery event:', msg);
  }
};
```

</details>


---

## Best Practices

### Label Your Containers

Always use explicit labels for production services:

```yaml
labels:
  nekzus.enable: "true"
  nekzus.app.id: "my-service"
  nekzus.app.name: "My Service"
```

### Use Network Filtering

Isolate discovery to specific networks:

```yaml
discovery:
  docker:
    networks:
      - "frontend-network"
    exclude_networks:
      - "bridge"
      - "host"
```

### Kubernetes Namespace Organization

Use namespace-level labels for environment-wide settings:

```yaml
# Namespace
metadata:
  labels:
    nekzus.enable: "true"
    nekzus.app.tags: "staging"
```

### Security Considerations

1. **Review proposals carefully** before approving
2. **Use scopes** to limit access to services
3. **Check security notes** in proposals
4. **Monitor port exposure warnings** for Docker services
5. **Use TLS** for upstream services when possible

### Performance Tuning

For large environments:

- Increase `poll_interval` to reduce API load
- Filter networks/namespaces to reduce scan scope
- Use explicit labels instead of relying on auto-detection
