# Federation

Nekzus Federation enables multiple instances to form a peer-to-peer cluster, sharing their service catalogs automatically. This allows services discovered on one instance to be accessible from any instance in the federation.

---

## Overview

Federation uses a gossip-based protocol (powered by HashiCorp's memberlist) to form clusters and synchronize service catalogs across peers. Each peer maintains its own local services and shares them with connected peers through eventual consistency.

### Architecture

```d2
direction: right

peer1: Peer 1 {
  grid-columns: 1
  catalog: Catalog Syncer
  storage: Service Catalog
}

gossip: Gossip Protocol {
  style.fill: "#7c3aed"
  style.font-color: "#ffffff"
  grid-columns: 1
  memberlist: Memberlist
  vector: Vector Clocks
}

peer2: Peer 2 {
  grid-columns: 1
  catalog: Catalog Syncer
  storage: Service Catalog
}

peer1 <-> gossip <-> peer2
```

### Key Features

- **Peer-to-peer clustering**: No central coordinator required
- **Eventual consistency**: Service catalogs converge across all peers
- **Vector clock conflict resolution**: Deterministic handling of concurrent updates
- **Anti-entropy sync**: Periodic full state exchange ensures consistency
- **Tombstone propagation**: Deleted services are properly removed from all peers
- **Gossip protocol**: Efficient message dissemination using HashiCorp memberlist
- **WebSocket events**: Real-time UI updates when federated services change
- **Metrics integration**: Prometheus metrics for monitoring federation health

---

## Configuration

Federation is configured in the `federation` section of the Nekzus configuration file.

### Basic Configuration

```yaml
federation:
  enabled: true
  local_peer_id: "nxs_homeserver"    # Unique peer identifier
  local_peer_name: "Home Server"      # Human-readable name
  api_address: "192.168.1.100:8080"   # API address for this instance

  # Gossip network settings
  gossip_bind_addr: "0.0.0.0"
  gossip_bind_port: 7946
  gossip_advertise_addr: "192.168.1.100"
  gossip_advertise_port: 7946

  # Security
  cluster_secret: "your-secret-key-32-chars"  # Must match all peers

  # Bootstrap peers to connect to on startup
  bootstrap_peers:
    - "192.168.1.101:7946"
    - "192.168.1.102:7946"
```

### Advanced Configuration

```yaml
federation:
  # ... basic config ...

  # Timing settings
  full_sync_interval: 5m      # How often to trigger full state sync
  anti_entropy_period: 30s    # Memberlist anti-entropy interval
  peer_timeout: 30s           # Mark peers offline after this duration

  # mDNS peer discovery (optional)
  mdns_enabled: false
  mdns_service_name: "_nekzus-peer._tcp"
```

### Configuration Reference

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | false | Enable federation |
| `local_peer_id` | string | required | Unique identifier for this peer (e.g., `nxs_homeserver`) |
| `local_peer_name` | string | required | Human-readable peer name |
| `api_address` | string | required | API address for remote connections |
| `gossip_bind_addr` | string | "0.0.0.0" | Address to bind gossip listener |
| `gossip_bind_port` | int | 7946 | Port for gossip protocol |
| `gossip_advertise_addr` | string | required | Address to advertise to peers |
| `gossip_advertise_port` | int | 7946 | Port to advertise to peers |
| `cluster_secret` | string | required | Shared secret (min 32 chars) for cluster authentication |
| `bootstrap_peers` | []string | [] | Addresses of peers to join on startup |
| `full_sync_interval` | duration | 5m | Interval for periodic full catalog sync (min 1m) |
| `anti_entropy_period` | duration | 30s | Memberlist anti-entropy interval |
| `peer_timeout` | duration | 30s | Duration before marking a peer offline |
| `mdns_enabled` | bool | false | Enable mDNS peer discovery |

---

## How It Works

### Peer Discovery

Peers can discover each other through:

1. **Bootstrap peers**: Explicitly configured addresses in the config
2. **mDNS discovery**: Automatic local network discovery (when enabled)
3. **Gossip propagation**: New peers discovered through existing cluster members

### Service Catalog Synchronization

When a service is discovered locally:

1. The local CatalogSyncer increments its vector clock
2. A gossip message is broadcast to connected peers
3. Remote peers receive the message via memberlist
4. Each peer resolves conflicts using vector clocks
5. The winning version is stored and events are published

### Conflict Resolution

Federation uses **vector clocks** for conflict resolution:

- Each peer maintains a logical clock counter
- Updates include the originator's vector clock
- Conflicts are resolved by comparing clocks:
  - If clock A happened-before clock B, B wins
  - If clocks are concurrent, the peer with higher ID wins (deterministic)
  - Tombstones (deletions) take precedence over updates

### Anti-Entropy

To ensure eventual consistency:

- Memberlist periodically exchanges full state between peers
- `LocalState()` serializes the entire catalog
- `MergeRemoteState()` processes incoming state
- Missing or outdated services are automatically synced

---

## Monitoring

### Prometheus Metrics

Federation exposes the following metrics:

| Metric | Type | Description |
|--------|------|-------------|
| `nekzus_federation_peers_active` | Gauge | Number of active federation peers |
| `nekzus_federation_services_total` | Gauge | Total federated services by origin |
| `nekzus_federation_sync_total` | Counter | Total sync operations (by type/status) |
| `nekzus_federation_sync_errors_total` | Counter | Sync errors (by type/reason) |
| `nekzus_federation_sync_duration_seconds` | Histogram | Sync operation duration |
| `nekzus_federation_messages_sent_total` | Counter | Total gossip messages sent |
| `nekzus_federation_messages_received_total` | Counter | Total gossip messages received |

### WebSocket Events

The following events are published for real-time UI updates:

| Event | Description |
|-------|-------------|
| `peer_joined` | A new peer joined the cluster |
| `peer_left` | A peer left the cluster |
| `peer_online` | A peer came back online |
| `peer_offline` | A peer went offline |
| `federation_service_added` | A remote service was added |
| `federation_service_removed` | A remote service was removed |

---

## API Endpoints

### Get Peers

```http
GET /api/federation/peers
```

Returns the list of connected federation peers.

**Response:**
```json
{
  "peers": [
    {
      "id": "nxs_homeserver",
      "name": "Home Server",
      "address": "192.168.1.100",
      "gossip_addr": "192.168.1.100:7946",
      "status": "online",
      "last_seen": "2024-01-15T10:30:00Z"
    }
  ]
}
```

### Get Federated Catalog

```http
GET /api/federation/catalog
```

Returns all services from all federated peers.

**Response:**
```json
{
  "services": [
    {
      "service_id": "grafana",
      "origin_peer_id": "nxs_homeserver",
      "app": {
        "id": "grafana",
        "name": "Grafana",
        "icon": "chart"
      },
      "confidence": 0.95,
      "last_seen": "2024-01-15T10:30:00Z"
    }
  ]
}
```

### Trigger Manual Sync

```http
POST /api/federation/sync
```

Triggers a manual full catalog synchronization.

---

## Troubleshooting

### Common Issues

**Peers not connecting:**

1. Verify `cluster_secret` matches on all peers
2. Check firewall allows UDP on gossip port (default 7946)
3. Ensure `gossip_advertise_addr` is reachable from other peers
4. Check logs for memberlist connection errors

**Services not syncing:**

1. Verify peers are connected (`GET /api/federation/peers`)
2. Check vector clocks in logs for conflict resolution issues
3. Trigger manual sync (`POST /api/federation/sync`)
4. Review anti-entropy period configuration

**High latency sync:**

1. Reduce `full_sync_interval` for faster convergence
2. Check network connectivity between peers
3. Monitor `federation_sync_duration_seconds` metric

### Debug Logging

Enable debug logging for federation:

```yaml
logging:
  level: debug
```

This will show detailed memberlist and catalog sync operations.
