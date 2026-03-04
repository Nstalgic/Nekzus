# Kubernetes Runtime Architecture

This document describes the internal architecture and design of the Kubernetes container runtime support in Nekzus.

## Overview

Nekzus abstracts container runtime operations behind a unified interface, enabling both Docker and Kubernetes backends with identical feature sets. This architecture allows seamless switching between runtimes and mixed-runtime deployments.

---

## Runtime Abstraction

### Core Interfaces

The runtime abstraction is defined in `internal/runtime/`:

```go
// Runtime represents a container runtime (Docker, Kubernetes)
type Runtime interface {
    Name() string
    Type() RuntimeType
    Ping(ctx context.Context) error

    ContainerManager
    LogStreamer
    StatsCollector
    Inspector
    BulkOperator
}

type ContainerManager interface {
    List(ctx context.Context, opts ListOptions) ([]Container, error)
    Start(ctx context.Context, id ContainerID) error
    Stop(ctx context.Context, id ContainerID, timeout *time.Duration) error
    Restart(ctx context.Context, id ContainerID, timeout *time.Duration) error
}

type LogStreamer interface {
    StreamLogs(ctx context.Context, id ContainerID, opts LogOptions) (io.ReadCloser, error)
}

type StatsCollector interface {
    GetStats(ctx context.Context, id ContainerID) (*Stats, error)
    GetBatchStats(ctx context.Context, ids []ContainerID) ([]Stats, error)
}
```

### Container Identification

Containers are identified using a unified `ContainerID` struct:

```go
type ContainerID struct {
    Runtime   RuntimeType
    ID        string // Docker: container ID, K8s: pod name
    Namespace string // K8s only (empty for Docker)
}
```

### Container States

| State | Description |
|-------|-------------|
| `running` | Container/pod is running |
| `stopped` | Container/pod is stopped |
| `paused` | Container is paused (Docker) |
| `restarting` | Container is restarting |
| `pending` | Pod is pending (Kubernetes) |
| `failed` | Pod has failed (Kubernetes) |

---

## Runtime Registry

The `Registry` manages available container runtimes:

```go
type Registry struct {
    mu       sync.RWMutex
    runtimes map[RuntimeType]Runtime
    primary  RuntimeType
}

func (r *Registry) Get(t RuntimeType) (Runtime, error)
func (r *Registry) GetPrimary() Runtime
func (r *Registry) SelectForContainer(id ContainerID) (Runtime, error)
```

The registry automatically routes requests based on:
1. Explicit `runtime` query parameter
2. Path format (K8s-style with namespace)
3. Primary runtime setting

---

## Kubernetes Implementation

### Pod Lifecycle Management

| Operation | Implementation |
|-----------|----------------|
| Start | Scale Deployment to 1 replica |
| Stop | Scale Deployment to 0 replicas |
| Restart | Delete pod (controller recreates) |

### Owner Resolution

For lifecycle operations, the runtime resolves pod ownership:

1. Check pod's `ownerReferences`
2. If owned by ReplicaSet, find parent Deployment
3. Scale the top-level owner
4. For standalone pods (no owner), delete and warn

### Statistics Collection

Kubernetes statistics use the Metrics Server API:

```go
func (k *KubernetesRuntime) GetStats(ctx context.Context, id ContainerID) (*Stats, error) {
    // Check Metrics Server availability
    // Get PodMetrics
    // Aggregate container metrics
    // Convert to runtime.Stats
}
```

Graceful degradation:
- Returns `ErrMetricsUnavailable` if Metrics Server not installed
- Handler returns empty stats with warning message

---

## Handler Integration

Handlers use the runtime registry to manage containers:

```go
type ContainerHandler struct {
    runtimes *runtime.Registry
    storage  *storage.Store
}

func (h *ContainerHandler) HandleListContainers(w http.ResponseWriter, r *http.Request) {
    // Determine runtime from query param or use primary
    rt := h.runtimes.GetPrimary()
    if runtimeParam := r.URL.Query().Get("runtime"); runtimeParam != "" {
        rt, _ = h.runtimes.Get(runtime.RuntimeType(runtimeParam))
    }

    containers, err := rt.List(r.Context(), runtime.ListOptions{})
    // ...
}
```

---

## API Path Formats

### Docker-Style (Backward Compatible)

```
POST /api/v1/containers/{container_id}/start?runtime=docker
POST /api/v1/containers/{container_id}/stop?runtime=kubernetes&namespace=default
```

### Kubernetes-Style (Namespace in Path)

```
POST /api/v1/containers/{namespace}/{pod}/start
POST /api/v1/containers/{namespace}/{pod}/restart
POST /api/v1/containers/{namespace}/{pod}/stop
```

---

## Configuration

### RuntimesConfig

```yaml
runtimes:
  primary: docker  # or kubernetes
  docker:
    enabled: true
    socket_path: ""  # empty = default
  kubernetes:
    enabled: false
    kubeconfig: ""  # empty = in-cluster
    context: ""
    namespaces: []  # empty = all namespaces
    metrics_server: true
    metrics_cache_ttl: "30s"
```

### Validation

- Defaults `primary` to docker if not set
- Validates duration fields
- Warns if primary runtime is not enabled

---

## Toolbox Kubernetes Deployment

The Toolbox can deploy services to Kubernetes using the `KubernetesDeployer`:

```go
type KubernetesDeployer struct {
    clientset kubernetes.Interface
    namespace string
}

func (d *KubernetesDeployer) Deploy(ctx context.Context, template *types.ServiceTemplate, opts DeployOptions) (*DeployResult, error) {
    // Parse template format
    // Apply manifests
    // Wait for deployment ready
    // Return service info
}
```

Deployments create:
- Kubernetes Deployment (scaled to 0 initially)
- Kubernetes Service (ClusterIP)

---

## Directory Structure

```
internal/runtime/
    interfaces.go      # Core interfaces
    types.go           # Shared types
    errors.go          # Runtime errors
    registry.go        # Runtime registry
    docker/
        runtime.go       # Docker implementation
        containers.go    # List, Start, Stop, Restart
        logs.go          # Log streaming
        stats.go         # Stats collection
        inspect.go       # Container inspection
        convert.go       # Type conversion
    kubernetes/
        runtime.go       # Kubernetes implementation
        containers.go    # Pod operations
        logs.go          # Log streaming
        stats.go         # Metrics Server integration
        inspect.go       # Pod inspection
        export.go        # YAML export
        convert.go       # Type conversion
```

---

## Testing

### Unit Tests

Each runtime has comprehensive unit tests using mock clients:

```go
// Docker tests use mock Docker client
func TestDockerRuntime_List(t *testing.T) { ... }

// Kubernetes tests use fake k8s clientset
func TestKubernetesRuntime_List(t *testing.T) { ... }
```

### Integration Tests

Optional integration tests require:
- Docker Desktop for Docker runtime
- kind or k3s for Kubernetes runtime

```bash
# Run with race detector
go test -race ./internal/runtime/...

# Integration tests
go test -race ./internal/runtime/... -tags=integration
```

---

## Performance Considerations

### Metrics Caching

Pod metrics are cached to reduce Kubernetes API calls:
- Default cache duration: 30 seconds
- Configurable via `kubernetes.metrics_cache_ttl`

### Batch Operations

Use batch endpoints for efficiency:
- `GET /api/v1/containers/stats` - Batch stats fetch
- `POST /api/v1/containers/bulk/restart` - Concurrent with limit

### Concurrency

Bulk operations use limited concurrency:
- Maximum 5 concurrent operations
- Prevents API rate limiting

---

## Error Handling

### Common Errors

| Error | Description |
|-------|-------------|
| `ErrRuntimeNotFound` | Requested runtime not registered |
| `ErrContainerNotFound` | Container/pod not found |
| `ErrMetricsUnavailable` | Metrics Server not available |
| `ErrPermissionDenied` | RBAC permission denied |

### Graceful Degradation

- Stats unavailable: Return empty stats with warning
- Metrics Server missing: Log warning, return placeholder values
- RBAC errors: Return specific error message with troubleshooting hint

---

## Related Documentation

- [Kubernetes Overview](index.md) - Setup and usage guide
- [Container API Reference](../reference/api-containers.md) - Complete API documentation
- [Configuration Reference](../reference/configuration.md) - All configuration options
