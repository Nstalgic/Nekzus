# Nekzus Kubernetes Integration

This guide covers running Nekzus with Kubernetes as a container runtime.

## Overview

Nekzus supports multiple container runtimes through a unified abstraction layer. Kubernetes support enables:

- Pod lifecycle management (start, stop, restart)
- Pod log streaming
- Resource usage statistics (CPU, memory, network)
- YAML manifest export
- Toolbox service deployment

## Prerequisites

- Kubernetes cluster (v1.21+)
- kubectl configured with cluster access
- Optional: Metrics Server for resource statistics

## Quick Start

### 1. Apply RBAC Configuration

```bash
kubectl apply -f docs/examples/rbac.yaml
```

For Toolbox deployment support:
```bash
kubectl apply -f docs/examples/toolbox-rbac.yaml
```

### 2. Configure Nekzus

Add Kubernetes runtime to your configuration:

```yaml
# config.yaml
runtimes:
  primary: docker  # or kubernetes
  kubernetes:
    enabled: true
    kubeconfig: ""  # Empty uses in-cluster config or ~/.kube/config
    namespace: ""   # Empty queries all namespaces

discovery:
  kubernetes:
    enabled: true
    label_selector: "nekzus.enable=true"
```

### 3. Start Nekzus

```bash
./nekzus -config config.yaml
```

## API Usage

### Container Operations

The API supports both Docker-style and Kubernetes-style paths:

```bash
# Docker-style (backward compatible)
POST /api/v1/containers/{container_id}/start?runtime=docker
POST /api/v1/containers/{container_id}/stop?runtime=kubernetes&namespace=default

# Kubernetes-style (namespace in path)
POST /api/v1/containers/{namespace}/{pod}/start
POST /api/v1/containers/{namespace}/{pod}/restart
POST /api/v1/containers/{namespace}/{pod}/stop
```

### List Containers

```bash
# List all containers from all runtimes
GET /api/v1/containers

# Filter by runtime
GET /api/v1/containers?runtime=kubernetes

# Filter by namespace
GET /api/v1/containers?runtime=kubernetes&namespace=production
```

### Get Stats

```bash
# Docker container stats
GET /api/v1/containers/{container_id}/stats

# Kubernetes pod stats
GET /api/v1/containers/{namespace}/{pod}/stats
```

### Export Manifests

```bash
# Export pod to YAML
GET /api/v1/export?runtime=kubernetes&namespace=default&pods=nginx-abc123
```

## Kubernetes Semantics

### Lifecycle Operations

| Operation | Docker | Kubernetes |
|-----------|--------|------------|
| Start | Starts stopped container | Scales Deployment to 1 replica |
| Stop | Stops running container | Scales Deployment to 0 replicas |
| Restart | Restarts container | Deletes pod (controller recreates it) |

### Pod Discovery

Nekzus discovers pods using label selectors:

```yaml
# Pod labels for discovery
metadata:
  labels:
    nekzus.enable: "true"
    nekzus.app.id: "my-app"
    nekzus.app.name: "My Application"
```

### Resource Statistics

Kubernetes statistics require the Metrics Server:

```bash
# Install Metrics Server
kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml
```

If Metrics Server is unavailable, stats endpoints return placeholder values.

## Toolbox Deployment

The Toolbox feature can deploy services to Kubernetes:

```bash
POST /api/v1/toolbox/deploy
{
  "serviceId": "grafana",
  "serviceName": "my-grafana",
  "envVars": {
    "GF_SECURITY_ADMIN_PASSWORD": "admin123"
  },
  "runtime": "kubernetes",
  "namespace": "nekzus-apps"
}
```

Deployments create:
- Kubernetes Deployment (scaled to 0 initially)
- Kubernetes Service (ClusterIP)

## RBAC Requirements

### Minimum Permissions (Read-Only)

```yaml
rules:
  - apiGroups: [""]
    resources: ["pods", "pods/log"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["apps"]
    resources: ["deployments", "replicasets"]
    verbs: ["get", "list"]
  - apiGroups: ["metrics.k8s.io"]
    resources: ["pods"]
    verbs: ["get", "list"]
```

### Full Management

```yaml
rules:
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["get", "list", "watch", "delete"]
  - apiGroups: ["apps"]
    resources: ["deployments"]
    verbs: ["get", "list", "watch", "update", "patch"]
```

### Toolbox Deployment

```yaml
rules:
  - apiGroups: ["apps"]
    resources: ["deployments"]
    verbs: ["create", "get", "list", "watch", "update", "patch", "delete"]
  - apiGroups: [""]
    resources: ["services"]
    verbs: ["create", "get", "list", "delete"]
```

See `docs/examples/rbac.yaml` for complete configurations.

## Troubleshooting

### Common Issues

#### "runtime not found" Error

Ensure Kubernetes is enabled in configuration:

```yaml
runtimes:
  kubernetes:
    enabled: true
```

#### Permission Denied

Check RBAC permissions:

```bash
kubectl auth can-i list pods --as=system:serviceaccount:default:nekzus
kubectl auth can-i get deployments --as=system:serviceaccount:default:nekzus
```

#### No Metrics Available

Install or verify Metrics Server:

```bash
kubectl get pods -n kube-system | grep metrics-server
kubectl top pods
```

#### Pods Not Discovered

Verify pod labels:

```bash
kubectl get pods -l nekzus.enable=true
```

### Debug Logging

Enable debug logging for runtime operations:

```yaml
logging:
  level: debug
  format: json
```

## Mixed Runtime Deployment

Nekzus supports running with both Docker and Kubernetes simultaneously:

```yaml
runtimes:
  primary: docker
  docker:
    enabled: true
  kubernetes:
    enabled: true
    namespace: ""  # All namespaces
```

The API automatically routes requests based on:
1. Explicit `runtime` query parameter
2. Path format (K8s-style with namespace)
3. Primary runtime setting

## Performance Considerations

### Batch Operations

Use batch endpoints for multiple containers:

```bash
# Batch stats (more efficient than individual calls)
GET /api/v1/containers/stats?runtime=kubernetes

# Bulk restart (concurrent with limit)
POST /api/v1/containers/bulk/restart
{
  "containerIds": ["pod1", "pod2", "pod3"],
  "runtime": "kubernetes",
  "namespace": "default"
}
```

### Metrics Caching

Pod metrics are cached briefly to reduce API calls:
- Default cache duration: 30 seconds
- Configurable via `kubernetes.metrics_cache_ttl`

## Security

### Service Account Best Practices

1. Use namespace-scoped Roles when possible
2. Limit permissions to required operations
3. Use separate ServiceAccounts for different functions
4. Audit RBAC bindings regularly

### Network Policies

Consider restricting Nekzus network access:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: nekzus-policy
spec:
  podSelector:
    matchLabels:
      app: nekzus
  policyTypes:
    - Egress
  egress:
    - to:
        - namespaceSelector: {}
      ports:
        - port: 443  # Kubernetes API
        - port: 6443 # Kubernetes API
```

## Next Steps

- [API Documentation](../reference/api) - Complete API reference
- [Toolbox Guide](../features/toolbox) - Service deployment
- [Testing Guide](../development/testing) - Running tests
