# Troubleshooting

This guide covers common issues you may encounter when running Nekzus and provides solutions to resolve them.

---

## Quick Diagnostics

Before diving into specific issues, run these diagnostic commands:

```bash
# Check container status
docker ps -a | grep nekzus

# View recent logs
docker logs nekzus --tail 100

# Check health endpoint
curl -k https://localhost:8443/api/v1/healthz

# Enable debug logging
docker exec nekzus sh -c "export NEKZUS_DEBUG=true"
```

---

## Installation Issues

### Container Fails to Start

<details>
<summary>Container exits immediately after starting</summary>


**Symptoms:**

- Container status shows `Exited (1)` or similar
- `docker logs` shows configuration or initialization errors

**Common Causes:**

1. **Invalid configuration file**
2. **Missing required environment variables**
3. **Database initialization failure**

**Solution:**

Check the container logs for specific error messages:

```bash
docker logs nekzus 2>&1 | head -50
```

Common log messages and fixes:

| Log Message | Cause | Solution |
|-------------|-------|----------|
| `JWT secret must be at least 32 characters` | JWT secret too short | Set `NEKZUS_JWT_SECRET` to a 32+ character string |
| `failed to create database directory` | Permission denied | Check volume mount permissions |
| `failed to load TLS certificate` | Invalid certificate files | Verify certificate paths and format |
| `JWT secret contains weak pattern` | Insecure secret detected | Use a strong random secret in production |

</details>


<details>
<summary>Container keeps restarting in a loop</summary>


**Symptoms:**

- Container shows `Restarting` status
- Health checks consistently fail

**Solution:**

1. Check if the health check endpoint is accessible:

    ```bash
    docker exec nekzus wget -q -O- http://localhost:8080/api/v1/healthz
    ```

2. Verify resource limits are not too restrictive:

    ```yaml
    # docker-compose.yml
    deploy:
      resources:
        limits:
          memory: 1G  # Minimum recommended
        reservations:
          memory: 256M
    ```

3. Check if the database is corrupted (see [Database Issues](#database-issues))

</details>


### Port Conflicts

<details>
<summary>Error: bind: address already in use</summary>


**Symptoms:**

- Container fails to start
- Error message mentions port binding failure

**Solution:**

1. Identify what's using the port:

    ```bash
    # Check port 8443 (HTTPS)
    sudo lsof -i :8443
    # or
    sudo netstat -tulpn | grep 8443
    ```

2. Either stop the conflicting service or change Nekzus ports:

    ```yaml
    # docker-compose.yml
    ports:
      - "9443:8443"  # Use port 9443 instead
      - "9080:80"
    ```

3. Update your `NEKZUS_BASE_URL` to match the new port:

    ```bash
    NEKZUS_BASE_URL=https://your-server:9443
    ```

</details>


### Permission Denied Errors

<details>
<summary>Permission denied when accessing files or Docker socket</summary>


**Symptoms:**

- Errors related to file permissions
- Docker discovery not working
- Database write failures

**Solution:**

1. **For database directory:**

    ```bash
    # Create data directory with correct permissions
    mkdir -p ./data
    chmod 755 ./data

    # If running as non-root user
    chown 1000:1000 ./data
    ```

2. **For Docker socket access:**

    ```bash
    # Add read-only mount with correct permissions
    docker run -v /var/run/docker.sock:/var/run/docker.sock:ro nekzus

    # On Linux, ensure user is in docker group
    sudo usermod -aG docker $USER
    ```

3. **For certificate files:**

    ```bash
    # Ensure certificates are readable
    chmod 644 ./certs/server.crt
    chmod 600 ./certs/server.key
    ```

</details>


### Docker Socket Access

<details>
<summary>Docker discovery shows 'Docker socket unavailable'</summary>


**Symptoms:**

- Log message: `failed to create Docker client`
- Discovery shows no containers
- WebSocket event: `Docker Discovery - Docker socket unavailable`

**Solution:**

1. Verify Docker socket is mounted:

    ```yaml
    # docker-compose.yml
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
    ```

2. Check socket path (varies by platform):

    | Platform | Socket Path |
    |----------|-------------|
    | Linux | `/var/run/docker.sock` |
    | macOS (Docker Desktop) | `/var/run/docker.sock` |
    | Windows (WSL2) | `/var/run/docker.sock` |
    | Podman | `/run/podman/podman.sock` |
    | Rootless Docker | `/run/user/1000/docker.sock` |

3. For custom socket paths, set in config:

    ```yaml
    # config.yaml
    discovery:
      docker:
        enabled: true
        socket_path: "unix:///run/user/1000/docker.sock"
    ```

</details>


---

## Discovery Issues

### Docker Discovery Not Finding Containers

<details>
<summary>Containers are running but not appearing in discovery</summary>


**Symptoms:**

- Running containers not showing in proposals
- Log shows `scanning containers` but no proposals created

**Possible Causes:**

1. **Container on different network**
2. **Container has no HTTP ports**
3. **Container is explicitly disabled**
4. **Container is a system container (filtered)**

**Solution:**

1. Check container labels:

    ```bash
    docker inspect <container> --format '{{json .Config.Labels}}' | jq
    ```

2. Ensure container has `nekzus.enable: "true"` label or expose HTTP ports:

    ```yaml
    # docker-compose.yml for your service
    labels:
      - "nekzus.enable=true"
      - "nekzus.app.id=myapp"
      - "nekzus.app.name=My Application"
    ```

3. Check network configuration:

    ```yaml
    # config.yaml
    discovery:
      docker:
        enabled: true
        networks:
          - nekzus-network  # Only scan specific networks
        exclude_networks:
          - host
          - none
    ```

4. Enable debug logging to see why containers are skipped:

    ```bash
    NEKZUS_DEBUG=true docker logs nekzus 2>&1 | grep -i "skipping"
    ```

</details>


<details>
<summary>Container discovered but HTTP probe fails</summary>


**Symptoms:**

- Log shows: `skipping port - HTTP probe failed`
- Container has exposed ports but none are discovered

**Solution:**

Nekzus probes ports to verify they serve HTTP. For non-standard setups:

1. **Force discovery of specific port:**

    ```yaml
    labels:
      - "nekzus.primary_port=3000"
    ```

2. **Discover all TCP ports (skip probing):**

    ```yaml
    labels:
      - "nekzus.discover.all_ports=true"
    ```

3. **Check if service is ready:** The container might need time to initialize:

    ```bash
    # Check if port responds
    docker exec <container> wget -q --spider http://localhost:3000
    ```

</details>


### mDNS Discovery Failures

<details>
<summary>mDNS discovery not finding any services</summary>


**Symptoms:**

- Log shows: `worker started - not fully implemented`
- No mDNS services discovered

**Current Status:**

mDNS discovery is not fully implemented in the current version. The worker starts but does not actively discover services.

**Workaround:**

1. Use Docker discovery for containerized services
2. Manually configure static routes for mDNS services:

    ```yaml
    # config.yaml
    routes:
      - route_id: "homeassistant"
        app_id: "homeassistant"
        path_base: "/apps/homeassistant/"
        to: "http://homeassistant.local:8123"

    apps:
      - id: "homeassistant"
        name: "Home Assistant"
        icon: "https://example.com/ha-icon.png"
    ```

</details>


### Kubernetes Service Discovery Problems

<details>
<summary>Kubernetes discovery shows 'failed to create Kubernetes config'</summary>


**Symptoms:**

- Log message: `failed to create Kubernetes config`
- Kubernetes services not discovered

**Solution:**

1. **When running inside Kubernetes cluster:**

    Ensure proper RBAC permissions:

    ```yaml
    # kubernetes/rbac.yaml
    apiVersion: rbac.authorization.k8s.io/v1
    kind: ClusterRole
    metadata:
      name: nekzus
    rules:
      - apiGroups: [""]
        resources: ["services", "namespaces"]
        verbs: ["get", "list", "watch"]
      - apiGroups: ["networking.k8s.io"]
        resources: ["ingresses"]
        verbs: ["get", "list", "watch"]
    ```

2. **When running outside cluster:**

    Mount kubeconfig file:

    ```yaml
    # docker-compose.yml
    volumes:
      - ~/.kube/config:/app/.kube/config:ro

    environment:
      KUBECONFIG: /app/.kube/config
    ```

3. **Configure in config.yaml:**

    ```yaml
    discovery:
      kubernetes:
        enabled: true
        kubeconfig: "/app/.kube/config"
        namespaces:
          - default
          - production
    ```

</details>


<details>
<summary>Services discovered but cannot be accessed</summary>


**Symptoms:**

- Kubernetes services appear in catalog
- Proxy returns 502 Bad Gateway

**Solution:**

1. Verify network connectivity between Nekzus and cluster:

    ```bash
    # From Nexus container
    docker exec nekzus nslookup myservice.default.svc.cluster.local
    docker exec nekzus curl http://myservice.default.svc.cluster.local:8080
    ```

2. Ensure Nekzus can resolve Kubernetes DNS:

    ```yaml
    # docker-compose.yml
    dns:
      - 10.96.0.10  # kube-dns service IP
    ```

</details>


---

## Authentication Issues

### JWT Token Errors

<details>
<summary>Error: 'TOKEN_EXPIRED' (Code 1001)</summary>


**Symptoms:**

- API returns 401 with error code `TOKEN_EXPIRED`
- Mobile app shows authentication expired

**Solution:**

1. **Mobile app:** The app should automatically attempt to refresh the token
2. **If refresh fails:** Re-pair the device by scanning a new QR code
3. **For long-running scripts:** Use API keys instead of JWT tokens

**Token Lifetime Configuration:**

```yaml
# config.yaml
auth:
  token_ttl: "24h"    # Access token lifetime
  refresh_ttl: "720h"  # Refresh token lifetime (30 days)
```

</details>


<details>
<summary>Error: 'TOKEN_INVALID' (Code 1002)</summary>


**Symptoms:**

- API returns 401 with error code `TOKEN_INVALID`
- Token rejected as malformed

**Common Causes:**

1. **JWT secret mismatch:** Secret changed after token was issued
2. **Token corruption:** Token was modified or truncated
3. **Wrong issuer/audience:** Token from different Nekzus instance

**Solution:**

1. Verify JWT secret consistency:

    ```bash
    # JWT secret should be the same across restarts
    docker exec nekzus printenv | grep JWT_SECRET
    ```

2. Re-pair affected devices with a fresh bootstrap token

</details>


<details>
<summary>Error: 'DEVICE_REVOKED' (Code 1004)</summary>


**Symptoms:**

- Device cannot authenticate
- Previously working device suddenly rejected

**Solution:**

The device was explicitly revoked by an administrator.

1. Check revocation in the web UI under **Devices**
2. To restore access, delete and re-pair the device:

    ```bash
    # Generate new bootstrap token
    curl -X POST https://localhost:8443/api/v1/auth/bootstrap/generate
    ```

</details>


### Mobile App Pairing Failures

<details>
<summary>QR code scanning works but pairing fails</summary>


**Symptoms:**

- Mobile app scans QR code successfully
- Pairing request returns error
- Log shows: `failed pairing attempt`

**Possible Causes:**

1. **Bootstrap token expired** (5-minute default lifetime)
2. **Token already used** (one-time use)
3. **Rate limiting** triggered

**Solution:**

1. Generate a fresh QR code (old ones expire after 5 minutes)

2. Check for rate limiting:

    ```bash
    docker logs nekzus 2>&1 | grep -i "rate"
    ```

3. Wait 1 minute and retry if rate limited

</details>


<details>
<summary>Mobile app cannot reach Nekzus server</summary>


**Symptoms:**

- QR code contains correct URL
- Mobile app shows connection error

**Solution:**

1. **Verify network connectivity:**
    - Mobile device must be on the same network
    - Check firewall rules allow port 8443

2. **Verify base URL configuration:**

    ```bash
    # Should return your server's LAN IP, not localhost
    docker logs nekzus 2>&1 | grep "base_url"
    ```

3. **Fix base URL if incorrect:**

    ```bash
    NEKZUS_BASE_URL=https://192.168.1.100:8443
    ```

4. **Certificate issues:** Mobile apps may reject self-signed certificates. Either:
    - Use a trusted certificate (Let's Encrypt)
    - Accept the certificate warning on first connection

</details>


### API Key Problems

<details>
<summary>API key returns 401 Unauthorized</summary>


**Symptoms:**

- API key was working, now returns 401
- Header `X-API-Key` is set correctly

**Common Causes:**

1. **Key revoked or expired**
2. **Insufficient scopes**
3. **Key not found in database**

**Solution:**

1. Check key status in the web UI under **Settings > API Keys**

2. Verify key has required scopes:

    ```bash
    # Key needs appropriate scopes for the endpoint
    # e.g., "write:*" for deployment operations
    ```

3. Create a new key if the old one is compromised or expired

</details>


### IP Allowlist Issues

<details>
<summary>Request rejected even from local network</summary>


**Symptoms:**

- Requests from LAN return 401
- Log shows: `Failed to parse IP from RemoteAddr`

**Solution:**

1. **Check if behind reverse proxy:** When using Caddy/nginx, the real client IP may not be forwarded:

    ```
    # Caddyfile - forward real IP
    header_up X-Real-IP {remote_host}
    header_up X-Forwarded-For {remote_host}
    ```

2. **Docker network ranges:** Ensure Docker bridge networks are recognized:

    The following ranges are automatically recognized as local:

    - `127.0.0.0/8` (loopback)
    - `10.0.0.0/8` (private)
    - `172.16.0.0/12` (private + Docker)
    - `192.168.0.0/16` (private)

</details>


---

## Proxy Issues

### WebSocket Connection Failures

<details>
<summary>WebSocket upgrade fails with 'WebSocket hijacking not supported'</summary>


**Symptoms:**

- WebSocket connections return 500 error
- Log shows: `WebSocket hijacking not supported`

**Solution:**

This typically occurs when middleware interferes with the connection hijacking.

1. Ensure the route has WebSocket enabled:

    ```yaml
    routes:
      - path_base: /apps/grafana/
        to: http://grafana:3000
        websocket: true  # Required for WebSocket support
    ```

2. Check if reverse proxy supports WebSocket upgrade:

    ```
    # Caddyfile
    @websocket header Connection *Upgrade*
    @websocket header Upgrade websocket
    reverse_proxy @websocket {upstream}
    ```

</details>


<details>
<summary>WebSocket connects but data not flowing</summary>


**Symptoms:**

- WebSocket handshake succeeds (101 Switching Protocols)
- No messages received after connection

**Possible Causes:**

1. **Firewall blocking WebSocket frames**
2. **Proxy timeout too short**
3. **Target service not sending data**

**Solution:**

1. Increase timeouts if needed:

    ```yaml
    # Route-level timeout configuration
    routes:
      - path_base: /apps/grafana/
        websocket: true
        # WebSocket connections have no default timeout
    ```

2. Check upstream service is sending data:

    ```bash
    # Test direct connection to upstream
    websocat ws://grafana:3000/api/live/ws
    ```

</details>


### Proxy Timeouts

<details>
<summary>Error: 'Gateway Timeout' (504)</summary>


**Symptoms:**

- Requests hang then return 504
- Log shows timeout errors

**Common Causes:**

1. **Upstream service slow to respond**
2. **DNS resolution taking too long**
3. **Network connectivity issues**

**Solution:**

1. Check upstream service health:

    ```bash
    # Direct request to upstream
    docker exec nekzus curl -v --max-time 5 http://upstream:8080/
    ```

2. Verify DNS resolution:

    ```bash
    docker exec nekzus nslookup upstream-service
    ```

3. Server timeouts are configured in the application:

    - Read timeout: 15 seconds
    - Write timeout: 30 seconds
    - Idle timeout: 120 seconds

</details>


<details>
<summary>Error: 'Bad Gateway' (502)</summary>


**Symptoms:**

- Proxy returns 502
- Upstream service appears to be running

**Common Causes and Solutions:**

| Error Label | Cause | Solution |
|-------------|-------|----------|
| `connection_refused` | Upstream not listening | Check if service is running and port is correct |
| `connection_reset` | Upstream closed connection | Check upstream logs for errors |
| `host_unreachable` | Network issue | Verify container networking |
| `dns_error` | Cannot resolve hostname | Check DNS configuration |

**Debug Steps:**

```bash
# 1. Check if upstream container is running
docker ps | grep <upstream>

# 2. Test connectivity
docker exec nekzus ping <upstream-hostname>

# 3. Test HTTP connection
docker exec nekzus curl -v http://<upstream>:<port>/
```

</details>


### SSL/TLS Certificate Errors

<details>
<summary>Error: 'x509: certificate signed by unknown authority'</summary>


**Symptoms:**

- Proxy to HTTPS upstream fails
- Log shows certificate validation error

**Solution:**

For self-signed upstream certificates, configure the route to skip verification:

```yaml
routes:
  - path_base: /apps/myservice/
    to: https://myservice:8443
    tls_skip_verify: true  # Only for trusted internal services
```

:::warning[Security Note]

Only use `tls_skip_verify` for trusted internal services. For external services, install proper CA certificates.

:::

</details>


<details>
<summary>Mobile app rejects self-signed certificate</summary>


**Symptoms:**

- Mobile app cannot connect
- Certificate pinning failure

**Solution:**

1. **Recommended:** Use a trusted certificate (Let's Encrypt via Caddy)

2. **Alternative:** Generate certificate with proper SANs:

    ```bash
    # Certificate should include your server's IP and hostname
    openssl req -x509 -newkey rsa:4096 -nodes \
      -keyout server.key -out server.crt -days 365 \
      -subj "/CN=nekzus" \
      -addext "subjectAltName=DNS:nekzus,IP:192.168.1.100"
    ```

3. The QR code pairing process includes certificate SPKI for pinning

</details>


### Path Rewriting Problems

<details>
<summary>Application returns 404 for assets or API calls</summary>


**Symptoms:**

- Main page loads but assets (CSS, JS) fail
- API calls to wrong path

**Common Causes:**

1. **Application expects to run at root path**
2. **Asset paths are absolute, not relative**

**Solution:**

1. Configure `strip_prefix` based on application needs:

    ```yaml
    routes:
      # For apps that can handle base paths:
      - path_base: /apps/myapp/
        strip_prefix: true  # /apps/myapp/api -> /api

      # For apps that expect full path:
      - path_base: /apps/legacy/
        strip_prefix: false  # /apps/legacy/api -> /apps/legacy/api
    ```

2. Enable HTML rewriting for apps with hardcoded paths:

    ```yaml
    routes:
      - path_base: /apps/myapp/
        rewrite_html: true  # Rewrites absolute paths in HTML
    ```

3. Some applications need environment configuration:

    ```yaml
    # For the upstream application
    environment:
      BASE_URL: /apps/myapp
      PUBLIC_PATH: /apps/myapp/
    ```

</details>


---

## Database Issues

### SQLite Lock Errors

<details>
<summary>Error: 'database is locked'</summary>


**Symptoms:**

- Intermittent errors about database locking
- Operations fail under load

**Solution:**

Nekzus uses WAL mode and connection pooling to handle concurrent access. If you still see lock errors:

1. **Check for external database access:**

    ```bash
    # Ensure no other processes are accessing the database
    lsof +D /path/to/data/
    ```

2. **Verify WAL mode is enabled:**

    ```bash
    docker exec nekzus sqlite3 /data/nexus.db "PRAGMA journal_mode;"
    # Should return: wal
    ```

3. **Increase busy timeout (already set to 5 seconds):**

    The application sets `PRAGMA busy_timeout=5000` by default.

4. **Check disk space:**

    ```bash
    df -h /path/to/data/
    ```

</details>


### Database Corruption Recovery

<details>
<summary>Error: 'database disk image is malformed'</summary>


**Symptoms:**

- Database operations fail
- Application won't start

**Solution:**

:::danger[Data Loss Risk]

Database corruption may result in data loss. Always maintain backups.

:::

1. **Stop the container:**

    ```bash
    docker stop nekzus
    ```

2. **Attempt recovery:**

    ```bash
    # Backup corrupted database
    cp /data/nexus.db /data/nexus.db.corrupt

    # Attempt to recover
    sqlite3 /data/nexus.db ".recover" | sqlite3 /data/nexus-recovered.db

    # Verify recovered database
    sqlite3 /data/nexus-recovered.db "PRAGMA integrity_check;"

    # Replace if recovery succeeded
    mv /data/nexus-recovered.db /data/nexus.db
    ```

3. **Restore from backup (if recovery fails):**

    ```bash
    # List available backups
    ls -la /data/backups/

    # Restore latest backup
    cp /data/backups/nexus-backup-latest.db /data/nexus.db
    ```

4. **Start fresh (last resort):**

    ```bash
    rm /data/nexus.db /data/nexus.db-wal /data/nexus.db-shm
    docker start nekzus
    # Re-pair all devices
    ```

</details>


### Migration Failures

<details>
<summary>Error: 'migration failed' on startup</summary>


**Symptoms:**

- Application fails to start
- Log shows migration error

**Solution:**

1. **Check the specific migration error:**

    ```bash
    docker logs nekzus 2>&1 | grep -i "migration"
    ```

2. **Common migration issues:**

    | Error | Cause | Solution |
    |-------|-------|----------|
    | `table already exists` | Interrupted migration | Delete and let it recreate |
    | `no such column` | Schema mismatch | Restore from backup |
    | `constraint failed` | Data integrity issue | Check database contents |

3. **Manual migration reset (caution - data loss):**

    ```bash
    # Backup first
    cp /data/nexus.db /data/nexus.db.backup

    # Remove and restart
    rm /data/nexus.db
    docker restart nekzus
    ```

</details>


---

## Performance Issues

### High Memory Usage

<details>
<summary>Container using excessive memory</summary>


**Symptoms:**

- Container exceeds memory limits
- OOM kills observed
- Memory grows over time

**Solution:**

1. **Check current memory usage:**

    ```bash
    docker stats nekzus
    ```

2. **Health check includes memory monitoring:**

    The application monitors memory and reports health as degraded above 512MB.

3. **Configure resource limits:**

    ```yaml
    # docker-compose.yml
    deploy:
      resources:
        limits:
          memory: 1G
        reservations:
          memory: 256M
    ```

4. **Check for connection leaks:**

    ```bash
    docker exec nekzus wget -qO- http://localhost:8080/metrics | grep connections
    ```

</details>


### Slow Response Times

<details>
<summary>API requests taking longer than expected</summary>


**Symptoms:**

- High latency on API calls
- Proxied requests slow

**Diagnostic Steps:**

1. **Check Prometheus metrics:**

    ```bash
    curl -s http://localhost:8080/metrics | grep http_request_duration
    ```

2. **Check if it's the proxy or API:**

    ```bash
    # Direct API call
    time curl https://localhost:8443/api/v1/apps

    # Proxied request
    time curl https://localhost:8443/apps/grafana/
    ```

3. **Check upstream service health:**

    Visit **Dashboard > Service Health** to see upstream response times.

4. **Enable request tracing:**

    ```bash
    NEKZUS_DEBUG=true docker restart nekzus
    ```

</details>


### Connection Pooling

<details>
<summary>Too many connections to upstream services</summary>


**Symptoms:**

- Upstream services rejecting connections
- "connection reset" errors under load

**Solution:**

Nekzus uses Go's `http.Transport` with default pooling:

- Max idle connections: 100
- Max connections per host: 100
- Idle connection timeout: 90 seconds

For high-traffic scenarios, ensure upstream services can handle the connection count.

</details>


---

## Logging and Debugging

### Enabling Debug Logs

<details>
<summary>How to enable verbose logging</summary>


**Solution:**

1. **Via environment variable:**

    ```bash
    docker run -e NEKZUS_DEBUG=true nekzus
    # or
    docker run -e NEKZUS_DEBUG=1 nekzus
    ```

2. **In docker-compose.yml:**

    ```yaml
    environment:
      NEKZUS_DEBUG: "true"
    ```

3. **Debug output includes:**
    - HTTP request details
    - WebSocket frame information
    - Discovery processing details
    - Authentication flow details

</details>


### Reading Container Logs

<details>
<summary>How to effectively read and filter logs</summary>


**Useful Log Commands:**

```bash
# Last 100 lines
docker logs nekzus --tail 100

# Follow logs in real-time
docker logs nekzus -f

# Logs since specific time
docker logs nekzus --since 1h

# Filter for errors only
docker logs nekzus 2>&1 | grep -i "error"

# Filter by component
docker logs nekzus 2>&1 | grep "component=discovery"
docker logs nekzus 2>&1 | grep "component=proxy"
docker logs nekzus 2>&1 | grep "component=auth"
```

</details>


### Common Log Messages

<details>
<summary>Understanding common log messages</summary>


**Informational Messages:**

| Message | Meaning |
|---------|---------|
| `storage initialized` | Database connected successfully |
| `registered docker worker` | Docker discovery active |
| `scanning containers` | Docker discovery running |
| `new proposal` | Service discovered, awaiting approval |
| `config reload: completed successfully` | Hot reload succeeded |

**Warning Messages:**

| Message | Meaning | Action |
|---------|---------|--------|
| `failed to create docker discovery worker` | Docker unavailable | Check Docker socket mount |
| `docker discovery will be disabled` | Continuing without Docker | Mount Docker socket if needed |
| `only docker network ip found` | Host networking issue | Check NEKZUS_BASE_URL |
| `invalid ack_timeout, using default` | Config parse error | Check config syntax |

**Error Messages:**

| Message | Meaning | Action |
|---------|---------|--------|
| `migration failed` | Database schema error | Check database permissions |
| `failed to load TLS certificate` | Certificate issue | Verify cert files exist and are valid |
| `JWT secret must be at least 32 characters` | Security requirement | Use longer secret |

</details>


---

## Getting Help

If you cannot resolve your issue using this guide:

1. **Check existing issues:** [GitHub Issues](https://github.com/nstalgic/nekzus/issues)

2. **Gather diagnostic information:**

    ```bash
    # System information
    docker version
    docker info
    uname -a

    # Container status
    docker ps -a | grep nekzus
    docker inspect nekzus

    # Recent logs
    docker logs nekzus --tail 200 > nekzus-logs.txt 2>&1

    # Health check
    curl -k https://localhost:8443/api/v1/health
    ```

3. **Create a new issue** with:
    - Description of the problem
    - Steps to reproduce
    - Expected vs actual behavior
    - Diagnostic information gathered above
    - Configuration (redact secrets)
