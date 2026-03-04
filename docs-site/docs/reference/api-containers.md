import Tabs from "@theme/Tabs";
import TabItem from "@theme/TabItem";

# Container API Reference

Complete API reference for container management in Nekzus. These endpoints enable lifecycle management, monitoring, log streaming, and configuration export for containers across Docker and Kubernetes runtimes.

---

## Overview

Nekzus provides a unified container management API that abstracts differences between Docker and Kubernetes runtimes. The API supports:

- **Lifecycle operations:** Start, stop, and restart containers
- **Monitoring:** Real-time resource statistics (CPU, memory, network)
- **Log streaming:** WebSocket-based live log tailing
- **Bulk operations:** Operate on multiple containers simultaneously
- **Configuration export:** Export containers to Docker Compose format

:::note[Container Visibility]

By default, the container list only includes containers that are linked to approved routes in Nekzus. This ensures you only see containers that are actively managed by the gateway.

:::


### Runtime Support

Nekzus supports multiple container runtimes through a unified abstraction layer.

| Runtime | Description | Container ID Format |
|---------|-------------|---------------------|
| `docker` | Docker containers | 12-character hex ID (e.g., `abc123def456`) |
| `kubernetes` | Kubernetes pods | Pod name (e.g., `my-app-7d8f9b6c5d-x2k4m`) |

Use the `runtime` query parameter to target a specific runtime. If omitted, the primary configured runtime is used.

---

## Authentication

Container endpoints support both IP-based and JWT authentication.

| Source | Authentication |
|--------|----------------|
| Local network | No authentication required |
| External | JWT token with `write:*` scope for mutations |

---

## Rate Limiting

Container endpoints share a rate limit pool:

| Category | Rate Limit | Burst |
|----------|------------|-------|
| Container operations | 30 req/min | 20 |

---

## List Containers

### GET /api/v1/containers

Lists all containers from configured runtimes that are linked to approved routes.

**Authentication:** IP-based (local) or JWT

| Parameter | Type | Description |
|-----------|------|-------------|
| `runtime` | query | Filter by runtime: `docker` or `kubernetes` |
| `namespace` | query | Kubernetes namespace filter |

<Tabs>
<TabItem value="request" label="Request">

```bash
curl https://localhost:8443/api/v1/containers
```

</TabItem>
<TabItem value="request-kubernetes-" label="Request (Kubernetes)">

```bash
curl "https://localhost:8443/api/v1/containers?runtime=kubernetes&namespace=default"
```

</TabItem>
<TabItem value="response" label="Response">

```json
[
  {
    "id": "abc123def456",
    "name": "grafana",
    "image": "grafana/grafana:latest",
    "state": "running",
    "status": "Up 2 hours",
    "created": 1701388800,
    "ports": [
      {
        "ip": "0.0.0.0",
        "privatePort": 3000,
        "publicPort": 3000,
        "type": "tcp"
      }
    ],
    "labels": {
      "nekzus.enable": "true",
      "nekzus.app.id": "grafana"
    },
    "runtime": "docker",
    "namespace": ""
  }
]
```

</TabItem>
</Tabs>

### Response Fields

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Container ID (12-character short form) |
| `name` | string | Container name |
| `image` | string | Container image with tag |
| `state` | string | Container state (see states below) |
| `status` | string | Human-readable status string |
| `created` | number | Creation timestamp (Unix) |
| `ports` | array | Port mappings |
| `labels` | object | Container labels |
| `runtime` | string | Runtime type (`docker` or `kubernetes`) |
| `namespace` | string | Kubernetes namespace (empty for Docker) |

### Container States

| State | Description |
|-------|-------------|
| `running` | Container is running |
| `stopped` | Container is stopped |
| `paused` | Container is paused (Docker) |
| `restarting` | Container is restarting |
| `pending` | Pod is pending (Kubernetes) |
| `failed` | Pod has failed (Kubernetes) |
| `created` | Container is created but not started |
| `exited` | Container has exited |

---

## Inspect Container

### GET /api/v1/containers/\{containerId\}

Returns detailed information about a specific container.

**Authentication:** IP-based (local) or JWT

<Tabs>
<TabItem value="request" label="Request">

```bash
curl https://localhost:8443/api/v1/containers/abc123def456
```

</TabItem>
<TabItem value="request-kubernetes-" label="Request (Kubernetes)">

```bash
curl "https://localhost:8443/api/v1/containers/my-pod?runtime=kubernetes&namespace=default"
```

</TabItem>
<TabItem value="response-docker-" label="Response (Docker)">

```json
{
  "Id": "abc123def456789...",
  "Name": "/grafana",
  "State": {
    "Status": "running",
    "Running": true,
    "Pid": 12345,
    "StartedAt": "2025-01-15T10:30:00Z"
  },
  "Config": {
    "Image": "grafana/grafana:latest",
    "Env": [
      "GF_SECURITY_ADMIN_PASSWORD=***"
    ],
    "Labels": {
      "nekzus.app.id": "grafana"
    }
  },
  "NetworkSettings": {
    "IPAddress": "172.17.0.5",
    "Ports": {
      "3000/tcp": [{"HostIp": "0.0.0.0", "HostPort": "3000"}]
    }
  }
}
```

</TabItem>
</Tabs>

:::info[Runtime-Specific Response]

For Docker containers, the response returns the raw Docker API format for backward compatibility. For Kubernetes pods, a normalized `ContainerDetails` format is returned.

:::


---

## Container Lifecycle

Container lifecycle operations are **asynchronous**. They return `202 Accepted` immediately and send completion notifications via WebSocket.

### POST /api/v1/containers/\{containerId\}/start

Starts a stopped container.

**Authentication:** IP-based (local) or JWT with `write:*` scope

<Tabs>
<TabItem value="request" label="Request">

```bash
curl -X POST https://localhost:8443/api/v1/containers/abc123def456/start
```

</TabItem>
<TabItem value="request-kubernetes-" label="Request (Kubernetes)">

```bash
curl -X POST "https://localhost:8443/api/v1/containers/my-pod/start?runtime=kubernetes&namespace=default"
```

</TabItem>
<TabItem value="response-202" label="Response 202">

```json
{
  "status": "accepted",
  "containerId": "abc123def456",
  "message": "Container start initiated",
  "timestamp": 1701388800
}
```

</TabItem>
</Tabs>

**WebSocket Notification (Success):**
```json
{
  "type": "container.start.completed",
  "data": {
    "containerId": "abc123def456",
    "status": "started",
    "message": "Container started successfully",
    "timestamp": 1701388801
  }
}
```

**WebSocket Notification (Failure):**
```json
{
  "type": "container.start.failed",
  "data": {
    "containerId": "abc123def456",
    "error": "CONTAINER_START_FAILED",
    "message": "Failed to start container",
    "timestamp": 1701388801
  }
}
```

### POST /api/v1/containers/\{containerId\}/stop

Stops a running container with an optional grace period.

**Authentication:** IP-based (local) or JWT with `write:*` scope

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `timeout` | query | 10 | Grace period in seconds (1-300) |

<Tabs>
<TabItem value="request" label="Request">

```bash
curl -X POST "https://localhost:8443/api/v1/containers/abc123def456/stop?timeout=30"
```

</TabItem>
<TabItem value="response-202" label="Response 202">

```json
{
  "status": "accepted",
  "containerId": "abc123def456",
  "message": "Container stop initiated",
  "timeout": 30,
  "timestamp": 1701388800
}
```

</TabItem>
</Tabs>

:::warning[Timeout Validation]

The timeout must be between 1 and 300 seconds. Values outside this range return a `400 Bad Request` error with code `INVALID_TIMEOUT`.

:::


**WebSocket Notification:**
```json
{
  "type": "container.stop.completed",
  "data": {
    "containerId": "abc123def456",
    "status": "stopped",
    "message": "Container stopped successfully",
    "timestamp": 1701388810
  }
}
```

:::note[Health Status Update]

When a container is stopped, the associated app is immediately marked as unhealthy. This triggers a `health_change` WebSocket notification to all connected devices.

:::


### POST /api/v1/containers/\{containerId\}/restart

Restarts a container with an optional grace period for the stop phase.

**Authentication:** IP-based (local) or JWT with `write:*` scope

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `timeout` | query | 10 | Grace period in seconds (1-300) |

<Tabs>
<TabItem value="request" label="Request">

```bash
curl -X POST https://localhost:8443/api/v1/containers/abc123def456/restart
```

</TabItem>
<TabItem value="response-202" label="Response 202">

```json
{
  "status": "accepted",
  "containerId": "abc123def456",
  "message": "Container restart initiated",
  "timestamp": 1701388800
}
```

</TabItem>
</Tabs>

**WebSocket Notification:**
```json
{
  "type": "container.restart.completed",
  "data": {
    "containerId": "abc123def456",
    "status": "restarted",
    "message": "Container restarted successfully",
    "timestamp": 1701388815
  }
}
```

---

## Container Statistics

### GET /api/v1/containers/\{containerId\}/stats

Returns real-time resource usage statistics for a container.

**Authentication:** IP-based (local) or JWT

<Tabs>
<TabItem value="request" label="Request">

```bash
curl https://localhost:8443/api/v1/containers/abc123def456/stats
```

</TabItem>
<TabItem value="response" label="Response">

```json
{
  "containerId": "abc123def456",
  "cpu": {
    "usage": 12.5,
    "coresUsed": 0.25,
    "totalCores": 4.0
  },
  "memory": {
    "usage": 45.2,
    "used": 483729408,
    "limit": 1073741824,
    "available": 590012416
  },
  "network": {
    "rx": 1048576,
    "tx": 524288
  },
  "timestamp": 1701388800
}
```

</TabItem>
</Tabs>

### Response Fields

| Field | Type | Description |
|-------|------|-------------|
| `containerId` | string | Container ID |
| `cpu.usage` | number | CPU usage percentage (0-100 per core) |
| `cpu.coresUsed` | number | Number of CPU cores being used |
| `cpu.totalCores` | number | Total available CPU cores |
| `memory.usage` | number | Memory usage percentage (0-100) |
| `memory.used` | number | Memory used in bytes |
| `memory.limit` | number | Memory limit in bytes |
| `memory.available` | number | Available memory in bytes |
| `network.rx` | number | Total bytes received |
| `network.tx` | number | Total bytes transmitted |
| `timestamp` | number | Stats collection timestamp (Unix) |

### GET /api/v1/containers/stats

Returns resource statistics for all running containers.

**Authentication:** IP-based (local) or JWT

<Tabs>
<TabItem value="request" label="Request">

```bash
curl https://localhost:8443/api/v1/containers/stats
```

</TabItem>
<TabItem value="response" label="Response">

```json
{
  "containers": [
    {
      "containerId": "abc123def456",
      "cpu": {"usage": 12.5, "coresUsed": 0.25, "totalCores": 4.0},
      "memory": {"usage": 45.2, "used": 483729408, "limit": 1073741824, "available": 590012416},
      "network": {"rx": 1048576, "tx": 524288},
      "timestamp": 1701388800
    },
    {
      "containerId": "def789abc123",
      "cpu": {"usage": 5.2, "coresUsed": 0.1, "totalCores": 4.0},
      "memory": {"usage": 22.1, "used": 237502464, "limit": 1073741824, "available": 836239360},
      "network": {"rx": 524288, "tx": 262144},
      "timestamp": 1701388800
    }
  ],
  "timestamp": 1701388800
}
```

</TabItem>
</Tabs>

:::note[Filtered Results]

Only containers linked to approved apps are included in batch stats. Stopped containers are excluded.

:::


---

## Bulk Operations

Bulk operations execute on multiple containers concurrently with a maximum of 5 concurrent operations.

### POST /api/v1/containers/start-all

Starts all stopped containers.

**Authentication:** IP-based (local) or JWT with `write:*` scope

<Tabs>
<TabItem value="request" label="Request">

```bash
curl -X POST https://localhost:8443/api/v1/containers/start-all
```

</TabItem>
<TabItem value="response" label="Response">

```json
{
  "message": "Start all operation completed",
  "total": 3,
  "success": 2,
  "failed": 1,
  "results": [
    {"id": "abc123def456", "name": "grafana", "success": true},
    {"id": "def789abc123", "name": "prometheus", "success": true},
    {"id": "ghi456jkl789", "name": "redis", "success": false, "error": "container already running"}
  ]
}
```

</TabItem>
</Tabs>

### POST /api/v1/containers/stop-all

Stops all running containers.

**Authentication:** IP-based (local) or JWT with `write:*` scope

<Tabs>
<TabItem value="request" label="Request">

```bash
curl -X POST https://localhost:8443/api/v1/containers/stop-all
```

</TabItem>
<TabItem value="response" label="Response">

```json
{
  "message": "Stop all operation completed",
  "total": 2,
  "success": 2,
  "failed": 0,
  "results": [
    {"id": "abc123def456", "name": "grafana", "success": true},
    {"id": "def789abc123", "name": "prometheus", "success": true}
  ]
}
```

</TabItem>
</Tabs>

### POST /api/v1/containers/restart-all

Restarts all containers.

**Authentication:** IP-based (local) or JWT with `write:*` scope

<Tabs>
<TabItem value="request" label="Request">

```bash
curl -X POST https://localhost:8443/api/v1/containers/restart-all
```

</TabItem>
<TabItem value="response" label="Response">

```json
{
  "message": "Restart all operation completed",
  "total": 3,
  "success": 3,
  "failed": 0,
  "results": [
    {"id": "abc123def456", "name": "grafana", "success": true},
    {"id": "def789abc123", "name": "prometheus", "success": true},
    {"id": "ghi456jkl789", "name": "redis", "success": true}
  ]
}
```

</TabItem>
</Tabs>

### POST /api/v1/containers/batch

Performs an operation on a specific set of containers.

**Authentication:** IP-based (local) or JWT with `write:*` scope

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `action` | string | Yes | Operation: `start`, `stop`, or `restart` |
| `containerIds` | array | Yes | List of container IDs |
| `timeout` | number | No | Grace period in seconds (for stop/restart) |

<Tabs>
<TabItem value="request" label="Request">

```bash
curl -X POST https://localhost:8443/api/v1/containers/batch \
  -H "Content-Type: application/json" \
  -d '{
    "action": "restart",
    "containerIds": ["abc123def456", "def789abc123"],
    "timeout": 15
  }'
```

</TabItem>
<TabItem value="response" label="Response">

```json
{
  "message": "Batch operation completed",
  "total": 2,
  "success": 2,
  "failed": 0,
  "results": [
    {"id": "abc123def456", "name": "grafana", "success": true},
    {"id": "def789abc123", "name": "prometheus", "success": true}
  ]
}
```

</TabItem>
</Tabs>

### Error Responses

| Error Code | HTTP Status | Description |
|------------|-------------|-------------|
| `INVALID_ACTION` | 400 | Action must be `start`, `stop`, or `restart` |
| `EMPTY_CONTAINER_LIST` | 400 | At least one container ID is required |
| `NO_RUNTIME` | 500 | No container runtime configured |
| `CONTAINER_LIST_FAILED` | 500 | Failed to list containers |

---

## Container Logs

Container logs are streamed via WebSocket for real-time updates.

### WebSocket Log Streaming

To subscribe to container logs, send a message over an authenticated WebSocket connection.

#### Subscribe to Logs

```json
{
  "type": "container.logs.subscribe",
  "data": {
    "containerId": "abc123def456",
    "tail": 100,
    "follow": true,
    "timestamps": true
  }
}
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `containerId` | string | Required | Container ID |
| `tail` | number | 100 | Number of lines from end (max 1000) |
| `follow` | boolean | true | Stream new logs in real-time |
| `timestamps` | boolean | false | Include timestamps in output |

#### Log Stream Messages

**Stream Started:**
```json
{
  "type": "container.logs.started",
  "data": {
    "containerId": "abc123def456",
    "timestamp": 1701388800
  }
}
```

**Log Data:**
```json
{
  "type": "container.logs",
  "data": {
    "containerId": "abc123def456",
    "stream": "stdout",
    "message": "2025-01-15T10:30:00Z INFO  Starting application...",
    "timestamp": 1701388800
  }
}
```

| Field | Description |
|-------|-------------|
| `stream` | Output stream: `stdout` or `stderr` |
| `message` | Log line content |

**Stream Ended:**
```json
{
  "type": "container.logs.ended",
  "data": {
    "containerId": "abc123def456",
    "reason": "stopped",
    "timestamp": 1701388900
  }
}
```

| Reason | Description |
|--------|-------------|
| `stopped` | Client unsubscribed or disconnected |
| `container_stopped` | Container stopped running |
| `error` | Error occurred during streaming |

**Stream Error:**
```json
{
  "type": "container.logs.error",
  "data": {
    "containerId": "abc123def456",
    "error": "CONTAINER_NOT_FOUND",
    "message": "Container not found",
    "timestamp": 1701388800
  }
}
```

#### Unsubscribe from Logs

```json
{
  "type": "container.logs.unsubscribe",
  "data": {
    "containerId": "abc123def456"
  }
}
```

:::note[Memory Optimization]

Log streaming uses buffer pooling (32KB buffers) to reduce garbage collection pressure during high-volume streaming. Frames larger than 1MB are skipped to prevent memory abuse.

:::


---

## Export Configuration

Export container configurations to Docker Compose format for migration or backup.

### POST /api/v1/containers/\{containerId\}/export

Exports a single container configuration to Docker Compose format.

**Authentication:** IP-based (local) or JWT

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `sanitize_secrets` | boolean | true | Replace secret values with placeholders |
| `include_volumes` | boolean | true | Include volume definitions |
| `include_networks` | boolean | true | Include network definitions |

<Tabs>
<TabItem value="request" label="Request">

```bash
curl -X POST https://localhost:8443/api/v1/containers/abc123def456/export \
  -H "Content-Type: application/json" \
  -d '{
    "sanitize_secrets": true,
    "include_volumes": true,
    "include_networks": true
  }'
```

</TabItem>
<TabItem value="response" label="Response">

```json
{
  "format": "docker-compose",
  "content": "services:\n  grafana:\n    image: grafana/grafana:latest\n    ports:\n      - \"3000:3000\"\n    environment:\n      - GF_SECURITY_ADMIN_PASSWORD=${GF_SECURITY_ADMIN_PASSWORD}\n    volumes:\n      - grafana-data:/var/lib/grafana\n\nvolumes:\n  grafana-data:\n",
  "filename": "grafana-compose.yaml",
  "warnings": ["Secret 'GF_SECURITY_ADMIN_PASSWORD' was sanitized"],
  "env_content": "GF_SECURITY_ADMIN_PASSWORD=your_password_here\n",
  "env_filename": "grafana.env"
}
```

</TabItem>
</Tabs>

### POST /api/v1/containers/\{containerId\}/export/preview

Generates a preview of the export without triggering download.

**Authentication:** IP-based (local) or JWT

The request and response format is identical to the export endpoint.

### POST /api/v1/containers/batch/export

Exports multiple containers to a single Docker Compose stack.

**Authentication:** IP-based (local) or JWT

| Parameter | Type | Description |
|-----------|------|-------------|
| `format` | query | Output format: `json` (default) or `zip` |

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `container_ids` | array | Yes | List of container IDs to export |
| `stack_name` | string | No | Stack name (default: `exported-stack`) |
| `sanitize_secrets` | boolean | No | Replace secrets with placeholders (default: true) |
| `include_volumes` | boolean | No | Include volumes (default: true) |
| `include_networks` | boolean | No | Include networks (default: true) |

<Tabs>
<TabItem value="request-json-" label="Request (JSON)">

```bash
curl -X POST https://localhost:8443/api/v1/containers/batch/export \
  -H "Content-Type: application/json" \
  -d '{
    "container_ids": ["abc123def456", "def789abc123"],
    "stack_name": "my-stack"
  }'
```

</TabItem>
<TabItem value="request-zip-" label="Request (ZIP)">

```bash
curl -X POST "https://localhost:8443/api/v1/containers/batch/export?format=zip" \
  -H "Content-Type: application/json" \
  -d '{
    "container_ids": ["abc123def456", "def789abc123"],
    "stack_name": "my-stack"
  }' \
  -o my-stack.zip
```

</TabItem>
<TabItem value="response-json-" label="Response (JSON)">

```json
{
  "format": "docker-compose",
  "content": "services:\n  grafana:\n    image: grafana/grafana:latest\n    ...\n  prometheus:\n    image: prom/prometheus:latest\n    ...\n",
  "filename": "my-stack-compose.yaml",
  "warnings": [],
  "env_content": "GF_SECURITY_ADMIN_PASSWORD=your_password_here\n",
  "env_filename": "my-stack.env"
}
```

</TabItem>
</Tabs>

:::info[Partial Success]

If some containers fail to inspect, the export continues with available containers and returns `206 Partial Content` with warnings.

:::


### POST /api/v1/containers/batch/export/preview

Generates a preview of the batch export.

The request format is identical to the batch export endpoint.

---

## Error Codes

| Code | HTTP Status | Description |
|------|-------------|-------------|
| `CONTAINER_NOT_FOUND` | 404 | Container does not exist |
| `CONTAINER_START_FAILED` | 500 | Failed to start container |
| `CONTAINER_STOP_FAILED` | 500 | Failed to stop container |
| `CONTAINER_RESTART_FAILED` | 500 | Failed to restart container |
| `CONTAINER_ALREADY_STARTED` | 409 | Container is already running |
| `CONTAINER_ALREADY_STOPPED` | 409 | Container is already stopped |
| `CONTAINER_INSPECT_FAILED` | 500 | Failed to inspect container |
| `CONTAINER_STATS_FAILED` | 500 | Failed to get container stats |
| `CONTAINER_LIST_FAILED` | 500 | Failed to list containers |
| `INVALID_CONTAINER_ID` | 400 | Container ID is required |
| `INVALID_RUNTIME` | 400 | Invalid runtime specified |
| `INVALID_TIMEOUT` | 400 | Timeout must be between 1 and 300 seconds |
| `NO_RUNTIME` | 500 | No container runtime configured |
| `RUNTIME_UNAVAILABLE` | 503 | Container runtime not available |
| `EXPORT_FAILED` | 500 | Failed to export container configuration |
| `BATCH_EXPORT_FAILED` | 500 | Failed to batch export containers |
| `ALL_CONTAINERS_FAILED` | 404 | Failed to inspect any containers |

---

## Kubernetes Path Format

For Kubernetes pods, an alternative path format is supported:

```
/api/v1/containers/{namespace}/{pod}/{action}
```

<Tabs>
<TabItem value="example-start-pod" label="Example: Start Pod">

```bash
curl -X POST https://localhost:8443/api/v1/containers/default/my-app-7d8f9b6c5d-x2k4m/start
```

</TabItem>
<TabItem value="example-get-pod-logs" label="Example: Get Pod Logs">

```json
{
  "type": "container.logs.subscribe",
  "data": {
    "containerId": "my-app-7d8f9b6c5d-x2k4m",
    "namespace": "default"
  }
}
```

</TabItem>
</Tabs>

When using this path format, the runtime defaults to `kubernetes` automatically.

---

## WebSocket Message Types Summary

| Message Type | Direction | Description |
|--------------|-----------|-------------|
| `container.logs.subscribe` | Client -> Server | Subscribe to log streaming |
| `container.logs.unsubscribe` | Client -> Server | Unsubscribe from log streaming |
| `container.logs.started` | Server -> Client | Log stream started |
| `container.logs` | Server -> Client | Log data |
| `container.logs.ended` | Server -> Client | Log stream ended |
| `container.logs.error` | Server -> Client | Log stream error |
| `container.start.completed` | Server -> Client | Container started |
| `container.start.failed` | Server -> Client | Container start failed |
| `container.stop.completed` | Server -> Client | Container stopped |
| `container.stop.failed` | Server -> Client | Container stop failed |
| `container.restart.completed` | Server -> Client | Container restarted |
| `container.restart.failed` | Server -> Client | Container restart failed |
