import Tabs from "@theme/Tabs";
import TabItem from "@theme/TabItem";

# API Reference

Complete API reference for Nekzus. All endpoints are documented with request/response formats, authentication requirements, and example usage.

---

## Overview

Nekzus provides a RESTful API for managing services, devices, containers, and system configuration. The API follows standard HTTP conventions and returns JSON responses.

### Base URL

```
https://localhost:8443/api/v1    # HTTPS (recommended)
http://localhost:8080/api/v1     # HTTP (development only, requires --insecure-http flag)
```

### API Version

All responses include an `X-API-Version` header indicating the current API version.

### Content Type

All requests and responses use `application/json` unless otherwise specified.

---

## Authentication

Nekzus supports multiple authentication methods depending on the use case.

### Authentication Methods

| Method | Use Case | Header Format |
|--------|----------|---------------|
| JWT Token | Mobile apps, external clients | `Authorization: Bearer <jwt-token>` |
| Bootstrap Token | Initial device pairing | `Authorization: Bearer <bootstrap-token>` |
| API Key | External integrations, CI/CD | `X-API-Key: nekzus_<64-char-hex>` |
| IP-Based | Local network requests | No header required for localhost |

### JWT Tokens

JWT tokens are obtained through the device pairing flow and are valid for 12 hours.

```http
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
```

:::info[Token Refresh]

Use the `/api/v1/auth/refresh` endpoint to obtain a new token before expiration. The refresh endpoint accepts both valid and recently expired tokens.

:::


### Bootstrap Tokens

Short-lived tokens (5 minutes) used only for initial device pairing. Obtained from QR code scanning.

### API Keys

Permanent tokens for external integrations. Format: `nekzus_<64-character-hex-string>`

```http
X-API-Key: nekzus_a1b2c3d4e5f6789012345678901234567890abcdef1234567890abcdef123456
```

:::warning[API Key Security]

The full API key is only shown once at creation. Store it securely as it cannot be retrieved again.

:::


### Permission Scopes

| Scope | Description |
|-------|-------------|
| `read:catalog` | Read app catalog and routes |
| `read:events` | Subscribe to real-time events |
| `read:*` | All read permissions |
| `write:*` | All write permissions |
| `access:admin` | Administrative access |

---

## Rate Limiting

All endpoints are rate-limited per IP address. Exceeding limits returns `429 Too Many Requests`.

| Endpoint Category | Rate Limit | Burst |
|-------------------|------------|-------|
| Health checks | 10 req/sec | 50 |
| Authentication | 10 req/min | 10 |
| QR code generation | 1 req/sec | 5 |
| WebSocket connections | 6 req/min | 3 |
| Device management | 30 req/min | 30 |
| Container operations | 30 req/min | 20 |
| General API | 30 req/min | 30 |

Rate limit headers are included in responses:

```http
X-RateLimit-Limit: 30
X-RateLimit-Remaining: 29
X-RateLimit-Reset: 1736949720
```

---

## Error Handling

All errors return a consistent JSON structure:

```json
{
  "error": {
    "code": "ERROR_CODE",
    "message": "Human-readable error message"
  }
}
```

### Common Error Codes

| Code | HTTP Status | Description |
|------|-------------|-------------|
| `UNAUTHORIZED` | 401 | Authentication required |
| `FORBIDDEN` | 403 | Insufficient permissions |
| `NOT_FOUND` | 404 | Resource not found |
| `METHOD_NOT_ALLOWED` | 405 | HTTP method not supported |
| `RATE_LIMIT_EXCEEDED` | 429 | Too many requests |
| `INTERNAL_ERROR` | 500 | Server error |
| `STORAGE_UNAVAILABLE` | 503 | Storage service unavailable |

---

## Health Endpoints

Health check endpoints for monitoring and orchestration. No authentication required.

### GET /healthz

Returns detailed health status.

<Tabs>
<TabItem value="request" label="Request">

```bash
curl https://localhost:8443/healthz
```

</TabItem>
<TabItem value="response-200" label="Response 200">

```
ok
```

</TabItem>
<TabItem value="response-503" label="Response 503">

```
unhealthy
```

</TabItem>
</Tabs>

### GET /livez

Kubernetes liveness probe. Returns 200 if the process is alive.

<Tabs>
<TabItem value="request" label="Request">

```bash
curl https://localhost:8443/livez
```

</TabItem>
<TabItem value="response" label="Response">

```
ok
```

</TabItem>
</Tabs>

### GET /readyz

Kubernetes readiness probe. Returns 200 if ready to serve traffic.

<Tabs>
<TabItem value="request" label="Request">

```bash
curl https://localhost:8443/readyz
```

</TabItem>
<TabItem value="response-200" label="Response 200">

```
ok
```

</TabItem>
<TabItem value="response-503" label="Response 503">

```
not ready
```

</TabItem>
</Tabs>

---

## Authentication Endpoints

### GET /api/v1/auth/qr

Generates a QR code for mobile app pairing. The QR code contains a minimal payload with the base URL and a short pairing code. The mobile app uses this code to retrieve the full pairing configuration via `GET /api/v1/pair/{code}`.

**Authentication:** None
**Rate Limit:** 1 req/sec, burst 5

| Parameter | Type | Description |
|-----------|------|-------------|
| `format` | query | Response format: `json` (default) or `png` |

<Tabs>
<TabItem value="request-json-" label="Request (JSON)">

```bash
curl https://localhost:8443/api/v1/auth/qr
```

</TabItem>
<TabItem value="request-png-" label="Request (PNG)">

```bash
curl https://localhost:8443/api/v1/auth/qr?format=png -o qr.png
```

</TabItem>
<TabItem value="response-json-" label="Response (JSON)">

```json
{
  "qr": {
    "u": "https://192.168.1.100:8443",
    "c": "ABCD1234"
  },
  "code": "ABCD1234"
}
```

</TabItem>
</Tabs>

:::info[QR Code Contents]

The QR code encodes a minimal JSON payload:

- `u`: Base URL of the Nexus instance
- `c`: Short pairing code (8 characters, valid for 5 minutes)

The `code` field is also returned separately for manual entry when QR scanning is not possible.

:::


### POST /api/v1/pair

Redeems a pairing code and returns the full pairing configuration. This is the second step of the v2 pairing flow after scanning the QR code.

**Authentication:** None
**Rate Limit:** 10 req/min (per-IP) + global rate limiting
**Required Header:** `X-Pairing-Request: true`

| Parameter | Type | Description |
|-----------|------|-------------|
| `code` | body | 8-character pairing code from QR scan |

<Tabs>
<TabItem value="request" label="Request">

```bash
curl -X POST https://localhost:8443/api/v1/pair \
  -H "Content-Type: application/json" \
  -H "X-Pairing-Request: true" \
  -d '{"code": "ABCD1234"}'
```

</TabItem>
<TabItem value="response-200" label="Response 200">

```json
{
  "baseUrl": "https://192.168.1.100:8443",
  "name": "Nekzus @ MacBook-Pro",
  "spkiPins": ["sha256/AbCdEf123456..."],
  "bootstrapToken": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "capabilities": ["discovery", "websocket", "containers"],
  "nexusId": "nexus_abc123",
  "expiresAt": 1736950000
}
```

</TabItem>
<TabItem value="response-404" label="Response 404">

```json
{
  "error": {
    "code": "INVALID_CODE",
    "message": "Invalid or expired pairing code"
  }
}
```

</TabItem>
<TabItem value="response-429" label="Response 429">

```json
{
  "error": {
    "code": "RATE_LIMITED",
    "message": "Too many pairing attempts, please try again later"
  }
}
```

</TabItem>
</Tabs>

:::warning[Security Features]

- **Single Use:** Codes are invalidated after first successful redemption
- **5-Minute Expiry:** Codes expire 5 minutes after generation
- **Rate Limiting:** Both per-IP and global rate limits are enforced
- **Code Locking:** Codes are locked after 5 failed redemption attempts
- **Required Header:** The `X-Pairing-Request: true` header prevents CSRF attacks

:::


### POST /api/v1/auth/pair

Pairs a new device using a bootstrap token from QR code.

**Authentication:** Bootstrap token
**Rate Limit:** 10 req/min

<Tabs>
<TabItem value="request" label="Request">

```bash
curl -X POST https://localhost:8443/api/v1/auth/pair \
  -H "Authorization: Bearer bootstrap_a1b2c3d4e5f6" \
  -H "Content-Type: application/json" \
  -d '{
    "device": {
      "id": "my-iphone-123",
      "model": "iPhone 15 Pro",
      "platform": "ios",
      "pushToken": "apns_token_xyz"
    }
  }'
```

</TabItem>
<TabItem value="response-200" label="Response 200">

```json
{
  "accessToken": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "expiresIn": 43200,
  "scopes": ["read:catalog", "read:events"],
  "deviceId": "my-iphone-123"
}
```

</TabItem>
</Tabs>

**Supported Platforms:** `ios`, `android`, `web`, `desktop`, `linux`, `macos`, `windows`

### POST /api/v1/auth/refresh

Refreshes an existing JWT token.

**Authentication:** JWT token (can be expired)
**Rate Limit:** 10 req/min

<Tabs>
<TabItem value="request" label="Request">

```bash
curl -X POST https://localhost:8443/api/v1/auth/refresh \
  -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
```

</TabItem>
<TabItem value="response-200" label="Response 200">

```json
{
  "accessToken": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "expiresIn": 43200,
  "scopes": ["read:catalog", "read:events"],
  "deviceId": "my-iphone-123"
}
```

</TabItem>
</Tabs>

### GET /api/v1/auth/setup-status

Checks if initial setup is required (no users exist).

**Authentication:** None

<Tabs>
<TabItem value="request" label="Request">

```bash
curl https://localhost:8443/api/v1/auth/setup-status
```

</TabItem>
<TabItem value="response" label="Response">

```json
{
  "setupRequired": true,
  "hasUsers": false
}
```

</TabItem>
</Tabs>

### POST /api/v1/auth/setup

Creates the first admin user. Only works if no users exist.

**Authentication:** None

<Tabs>
<TabItem value="request" label="Request">

```bash
curl -X POST https://localhost:8443/api/v1/auth/setup \
  -H "Content-Type: application/json" \
  -d '{
    "username": "admin",
    "password": "secure-password-here"
  }'
```

</TabItem>
<TabItem value="response-201" label="Response 201">

```json
{
  "message": "Setup completed successfully. Please log in.",
  "username": "admin"
}
```

</TabItem>
</Tabs>

### POST /api/v1/auth/login

Authenticates a user with username and password.

**Authentication:** None

<Tabs>
<TabItem value="request" label="Request">

```bash
curl -X POST https://localhost:8443/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "username": "admin",
    "password": "secure-password-here"
  }'
```

</TabItem>
<TabItem value="response-200" label="Response 200">

```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "user": {
    "id": 1,
    "username": "admin",
    "createdAt": "2025-01-15T10:30:00Z",
    "lastLogin": "2025-01-15T14:22:00Z",
    "isActive": true
  }
}
```

</TabItem>
</Tabs>

### GET /api/v1/auth/me

Returns the current authenticated user's information.

**Authentication:** JWT token

<Tabs>
<TabItem value="request" label="Request">

```bash
curl https://localhost:8443/api/v1/auth/me \
  -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
```

</TabItem>
<TabItem value="response-200" label="Response 200">

```json
{
  "user": {
    "id": 1,
    "username": "admin",
    "createdAt": "2025-01-15T10:30:00Z",
    "lastLogin": "2025-01-15T14:22:00Z",
    "isActive": true
  }
}
```

</TabItem>
</Tabs>

### POST /api/v1/auth/logout

Logs out the current user. Currently client-side only (discards token).

**Authentication:** JWT token

<Tabs>
<TabItem value="request" label="Request">

```bash
curl -X POST https://localhost:8443/api/v1/auth/logout \
  -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
```

</TabItem>
<TabItem value="response-200" label="Response 200">

```json
{
  "message": "Logged out successfully"
}
```

</TabItem>
</Tabs>

---

## Admin Endpoints

### GET /api/v1/admin/info

Returns Nexus instance information.

**Authentication:** IP-based (local) or JWT

<Tabs>
<TabItem value="request" label="Request">

```bash
curl https://localhost:8443/api/v1/admin/info
```

</TabItem>
<TabItem value="response" label="Response">

```json
{
  "version": "1.0.0",
  "nexusId": "nexus_abc123",
  "capabilities": ["discovery", "websocket", "containers"],
  "buildDate": "2025-10-13"
}
```

</TabItem>
</Tabs>

### GET /api/v1/stats

Returns aggregated system statistics.

**Authentication:** IP-based (local) or JWT

<Tabs>
<TabItem value="request" label="Request">

```bash
curl https://localhost:8443/api/v1/stats
```

</TabItem>
<TabItem value="response" label="Response">

```json
{
  "routes": {
    "value": 5,
    "trend": "5 active",
    "trendUp": true
  },
  "devices": {
    "value": 3,
    "trend": "2 online now",
    "trendUp": true
  },
  "discoveries": {
    "value": 2,
    "trend": "2 pending review",
    "trendUp": false
  },
  "requests": {
    "value": 1523,
    "trend": "1523 total",
    "trendUp": true
  }
}
```

</TabItem>
</Tabs>

### GET /api/v1/activity/recent

Returns recent activity events.

**Authentication:** IP-based (local) or JWT

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `limit` | query | 50 | Maximum events to return |
| `offset` | query | 0 | Number of events to skip |

<Tabs>
<TabItem value="request" label="Request">

```bash
curl "https://localhost:8443/api/v1/activity/recent?limit=10"
```

</TabItem>
<TabItem value="response-no-pagination-" label="Response (no pagination)">

```json
[
  {
    "id": "device_paired_dev-a1b2c3d4",
    "type": "device_paired",
    "icon": "Smartphone",
    "iconClass": "success",
    "message": "Device paired: John's iPhone",
    "timestamp": 1736949720000
  }
]
```

</TabItem>
<TabItem value="response-with-pagination-" label="Response (with pagination)">

```json
{
  "activities": [...],
  "total": 42,
  "limit": 10,
  "offset": 0
}
```

</TabItem>
</Tabs>

### GET /api/v1/audit-logs

Returns audit logs with optional filtering.

**Authentication:** JWT token (strict)

| Parameter | Type | Description |
|-----------|------|-------------|
| `action` | query | Filter by action type |
| `actor` | query | Filter by actor (device/user ID) |
| `limit` | query | Maximum logs to return (default: 100, max: 1000) |
| `offset` | query | Number of logs to skip |

<Tabs>
<TabItem value="request" label="Request">

```bash
curl "https://localhost:8443/api/v1/audit-logs?limit=50" \
  -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
```

</TabItem>
<TabItem value="response" label="Response">

```json
{
  "logs": [
    {
      "id": 1,
      "action": "device.paired",
      "actor": "dev-a1b2c3d4",
      "target": "dev-a1b2c3d4",
      "details": "{\"platform\":\"ios\"}",
      "ip": "192.168.1.100",
      "timestamp": "2025-01-15T10:30:00Z"
    }
  ],
  "limit": 50,
  "offset": 0,
  "count": 1
}
```

</TabItem>
</Tabs>

---

## Apps and Routes

### GET /api/v1/apps

Returns all registered applications with health status.

**Authentication:** IP-based (local) or JWT with `read:catalog` scope

<Tabs>
<TabItem value="request" label="Request">

```bash
curl https://localhost:8443/api/v1/apps
```

</TabItem>
<TabItem value="response" label="Response">

```json
[
  {
    "id": "grafana",
    "name": "Grafana",
    "icon": "chart-line",
    "tags": ["monitoring", "dashboards"],
    "endpoints": {
      "lan": "http://grafana:3000"
    },
    "proxyPath": "/apps/grafana/",
    "url": "https://192.168.1.100:8443/apps/grafana/",
    "faviconURL": "/api/v1/apps/grafana/favicon",
    "healthStatus": "healthy",
    "lastHealthCheck": "2025-01-15T14:22:00Z"
  }
]
```

</TabItem>
</Tabs>

### GET /api/v1/apps/\{appId\}/favicon

Returns the favicon for an application.

**Authentication:** IP-based (local) or JWT

<Tabs>
<TabItem value="request" label="Request">

```bash
curl https://localhost:8443/api/v1/apps/grafana/favicon -o favicon.ico
```

</TabItem>
</Tabs>

### GET /api/v1/routes

Returns all registered proxy routes.

**Authentication:** IP-based (local) or JWT

<Tabs>
<TabItem value="request" label="Request">

```bash
curl https://localhost:8443/api/v1/routes
```

</TabItem>
<TabItem value="response" label="Response">

```json
[
  {
    "routeId": "route_grafana",
    "appId": "grafana",
    "pathBase": "/apps/grafana/",
    "to": "http://grafana:3000",
    "stripPrefix": true,
    "websocket": true,
    "scopes": ["read:catalog"],
    "status": "ACTIVE",
    "healthInfo": {
      "status": "healthy",
      "lastCheck": "2025-01-15T14:22:00Z",
      "responseTime": 45
    }
  }
]
```

</TabItem>
</Tabs>

### PUT /api/v1/routes/\{routeId\}

Updates an existing route.

**Authentication:** IP-based (local) or JWT with `write:*` scope

<Tabs>
<TabItem value="request" label="Request">

```bash
curl -X PUT https://localhost:8443/api/v1/routes/route_grafana \
  -H "Content-Type: application/json" \
  -d '{
    "routeId": "route_grafana",
    "appId": "grafana",
    "pathBase": "/apps/grafana/",
    "to": "http://grafana:3000",
    "websocket": true,
    "icon": "chart-line"
  }'
```

</TabItem>
<TabItem value="response-200" label="Response 200">

```json
{
  "routeId": "route_grafana",
  "appId": "grafana",
  "pathBase": "/apps/grafana/",
  "to": "http://grafana:3000",
  "websocket": true
}
```

</TabItem>
</Tabs>

### DELETE /api/v1/routes/\{routeId\}

Deletes a route.

**Authentication:** IP-based (local) or JWT with `write:*` scope

<Tabs>
<TabItem value="request" label="Request">

```bash
curl -X DELETE https://localhost:8443/api/v1/routes/route_grafana
```

</TabItem>
<TabItem value="response" label="Response">

```
204 No Content
```

</TabItem>
</Tabs>

---

## Discovery

### GET /api/v1/discovery/proposals

Returns pending service discovery proposals.

**Authentication:** IP-based (local) or JWT

<Tabs>
<TabItem value="request" label="Request">

```bash
curl https://localhost:8443/api/v1/discovery/proposals
```

</TabItem>
<TabItem value="response" label="Response">

```json
[
  {
    "id": "proposal_abc123",
    "source": "docker",
    "detectedScheme": "http",
    "detectedHost": "grafana",
    "detectedPort": 3000,
    "confidence": 0.95,
    "suggestedApp": {
      "id": "grafana",
      "name": "Grafana",
      "icon": "chart-line"
    },
    "suggestedRoute": {
      "pathBase": "/apps/grafana/",
      "to": "http://grafana:3000"
    },
    "availablePorts": [
      {"port": 3000, "scheme": "http", "label": "Main UI"}
    ],
    "tags": ["monitoring", "grafana"],
    "lastSeen": "2025-01-15T14:22:00Z",
    "securityNotes": ["Service uses HTTP without TLS"]
  }
]
```

</TabItem>
</Tabs>

### POST /api/v1/discovery/proposals/\{proposalId\}/approve

Approves a discovery proposal, adding it to the catalog.

**Authentication:** IP-based (local) or JWT with `write:*` scope

<Tabs>
<TabItem value="request" label="Request">

```bash
curl -X POST https://localhost:8443/api/v1/discovery/proposals/proposal_abc123/approve \
  -H "Content-Type: application/json" \
  -d '{"port": 3000}'
```

</TabItem>
<TabItem value="response-200" label="Response 200">

```json
{
  "status": "approved",
  "id": "proposal_abc123",
  "app": {
    "id": "grafana",
    "name": "Grafana"
  },
  "route": {
    "pathBase": "/apps/grafana/",
    "to": "http://grafana:3000"
  }
}
```

</TabItem>
</Tabs>

### POST /api/v1/discovery/proposals/\{proposalId\}/dismiss

Dismisses a discovery proposal.

**Authentication:** IP-based (local) or JWT with `write:*` scope

<Tabs>
<TabItem value="request" label="Request">

```bash
curl -X POST https://localhost:8443/api/v1/discovery/proposals/proposal_abc123/dismiss
```

</TabItem>
<TabItem value="response-200" label="Response 200">

```json
{
  "status": "dismissed",
  "id": "proposal_abc123"
}
```

</TabItem>
</Tabs>

### POST /api/v1/discovery/rediscover

Triggers a fresh discovery scan by clearing dismissed proposals.

**Authentication:** IP-based (local) or JWT

<Tabs>
<TabItem value="request" label="Request">

```bash
curl -X POST https://localhost:8443/api/v1/discovery/rediscover
```

</TabItem>
<TabItem value="response-200" label="Response 200">

```json
{
  "status": "success",
  "message": "Rediscovery triggered. Discovery workers will scan for new services.",
  "dismissedCleared": 5,
  "activeCleared": 2
}
```

</TabItem>
</Tabs>

---

## Devices

### GET /api/v1/devices

Returns all paired devices.

**Authentication:** IP-based (local) or JWT

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `limit` | query | -1 | Maximum devices to return (-1 = no limit) |
| `offset` | query | 0 | Number of devices to skip |

<Tabs>
<TabItem value="request" label="Request">

```bash
curl https://localhost:8443/api/v1/devices
```

</TabItem>
<TabItem value="response-array-" label="Response (array)">

```json
[
  {
    "deviceId": "dev-a1b2c3d4",
    "deviceName": "John's iPhone",
    "scopes": ["read:catalog", "read:events"],
    "createdAt": "2025-01-15T10:30:00Z",
    "lastSeenAt": "2025-01-15T14:22:00Z"
  }
]
```

</TabItem>
<TabItem value="response-paginated-" label="Response (paginated)">

```json
{
  "devices": [...],
  "total": 42,
  "limit": 10,
  "offset": 0
}
```

</TabItem>
</Tabs>

### GET /api/v1/devices/\{deviceId\}

Returns details for a specific device.

**Authentication:** IP-based (local) or JWT

<Tabs>
<TabItem value="request" label="Request">

```bash
curl https://localhost:8443/api/v1/devices/dev-a1b2c3d4
```

</TabItem>
<TabItem value="response" label="Response">

```json
{
  "deviceId": "dev-a1b2c3d4",
  "deviceName": "John's iPhone",
  "scopes": ["read:catalog", "read:events"],
  "createdAt": "2025-01-15T10:30:00Z",
  "lastSeenAt": "2025-01-15T14:22:00Z"
}
```

</TabItem>
</Tabs>

### PATCH /api/v1/devices/\{deviceId\}

Updates device metadata.

**Authentication:** IP-based (local) or JWT

<Tabs>
<TabItem value="request" label="Request">

```bash
curl -X PATCH https://localhost:8443/api/v1/devices/dev-a1b2c3d4 \
  -H "Content-Type: application/json" \
  -d '{"deviceName": "John'\''s New iPhone"}'
```

</TabItem>
<TabItem value="response-200" label="Response 200">

```json
{
  "deviceId": "dev-a1b2c3d4",
  "deviceName": "John's New iPhone",
  "scopes": ["read:catalog", "read:events"],
  "createdAt": "2025-01-15T10:30:00Z",
  "lastSeenAt": "2025-01-15T14:22:00Z"
}
```

</TabItem>
</Tabs>

### DELETE /api/v1/devices/\{deviceId\}

Revokes device access. All existing tokens become invalid.

**Authentication:** IP-based (local) or JWT

<Tabs>
<TabItem value="request" label="Request">

```bash
curl -X DELETE https://localhost:8443/api/v1/devices/dev-a1b2c3d4
```

</TabItem>
<TabItem value="response-200" label="Response 200">

```json
{
  "status": "revoked",
  "deviceId": "dev-a1b2c3d4",
  "message": "Device access has been revoked. Existing tokens are now invalid.",
  "revokedAt": "now"
}
```

</TabItem>
</Tabs>

---

## API Keys

### GET /api/v1/apikeys

Returns all API keys (without the actual key values).

**Authentication:** IP-based (local) or JWT with `access:admin` scope

<Tabs>
<TabItem value="request" label="Request">

```bash
curl https://localhost:8443/api/v1/apikeys
```

</TabItem>
<TabItem value="response" label="Response">

```json
[
  {
    "id": "key_abc123def456",
    "name": "Production CI/CD Pipeline",
    "prefix": "nekzus_a1b",
    "scopes": ["read:catalog"],
    "expiresAt": "2026-01-15T00:00:00Z",
    "lastUsedAt": "2025-01-15T14:22:00Z",
    "createdAt": "2025-01-15T10:00:00Z",
    "createdBy": "dev-a1b2c3d4",
    "revokedAt": null
  }
]
```

</TabItem>
</Tabs>

### POST /api/v1/apikeys

Creates a new API key.

**Authentication:** IP-based (local) or JWT with `access:admin` scope

:::warning[One-Time Display]

The full API key is only returned once at creation. Store it securely.

:::


<Tabs>
<TabItem value="request" label="Request">

```bash
curl -X POST https://localhost:8443/api/v1/apikeys \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Production CI/CD Pipeline",
    "scopes": ["read:catalog", "read:events"],
    "expiresAt": "2026-01-15T00:00:00Z"
  }'
```

</TabItem>
<TabItem value="response-201" label="Response 201">

```json
{
  "id": "key_abc123def456",
  "name": "Production CI/CD Pipeline",
  "prefix": "nekzus_a1b",
  "key": "nekzus_a1b2c3d4e5f6789012345678901234567890abcdef1234567890abcdef123456",
  "scopes": ["read:catalog", "read:events"],
  "expiresAt": "2026-01-15T00:00:00Z",
  "createdAt": "2025-01-15T10:00:00Z",
  "createdBy": "dev-a1b2c3d4"
}
```

</TabItem>
</Tabs>

### GET /api/v1/apikeys/\{keyId\}

Returns details for a specific API key.

**Authentication:** IP-based (local) or JWT with `access:admin` scope

<Tabs>
<TabItem value="request" label="Request">

```bash
curl https://localhost:8443/api/v1/apikeys/key_abc123def456
```

</TabItem>
</Tabs>

### DELETE /api/v1/apikeys/\{keyId\}

Revokes or permanently deletes an API key.

**Authentication:** IP-based (local) or JWT with `access:admin` scope

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `permanent` | query | false | Permanently delete the key |

<Tabs>
<TabItem value="request-revoke-" label="Request (Revoke)">

```bash
curl -X DELETE https://localhost:8443/api/v1/apikeys/key_abc123def456
```

</TabItem>
<TabItem value="request-delete-" label="Request (Delete)">

```bash
curl -X DELETE "https://localhost:8443/api/v1/apikeys/key_abc123def456?permanent=true"
```

</TabItem>
<TabItem value="response" label="Response">

```json
{
  "message": "API key revoked successfully"
}
```

</TabItem>
</Tabs>

---

## Containers

Container management endpoints require Docker integration to be enabled.

:::note[Runtime Support]

Nekzus supports both Docker and Kubernetes runtimes. Use the `runtime` query parameter to specify the target runtime.

:::


### GET /api/v1/containers

Lists all containers from configured runtimes.

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

### GET /api/v1/containers/\{containerId\}

Returns detailed information about a container.

**Authentication:** IP-based (local) or JWT

<Tabs>
<TabItem value="request" label="Request">

```bash
curl https://localhost:8443/api/v1/containers/abc123def456
```

</TabItem>
</Tabs>

### POST /api/v1/containers/\{containerId\}/start

Starts a container asynchronously. Returns 202 Accepted immediately.

**Authentication:** IP-based (local) or JWT with `write:*` scope

<Tabs>
<TabItem value="request" label="Request">

```bash
curl -X POST https://localhost:8443/api/v1/containers/abc123def456/start
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

**WebSocket Notification:**
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

### POST /api/v1/containers/\{containerId\}/stop

Stops a container asynchronously.

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

### POST /api/v1/containers/\{containerId\}/restart

Restarts a container asynchronously.

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

### GET /api/v1/containers/\{containerId\}/stats

Returns resource usage statistics for a container.

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
    "available": 589012416
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

### GET /api/v1/containers/stats

Returns stats for all running containers.

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
      "memory": {"usage": 45.2, "used": 483729408, "limit": 1073741824, "available": 589012416},
      "network": {"rx": 1048576, "tx": 524288},
      "timestamp": 1701388800
    }
  ],
  "timestamp": 1701388800
}
```

</TabItem>
</Tabs>

### Bulk Operations

#### POST /api/v1/containers/start-all

Starts all stopped containers.

#### POST /api/v1/containers/stop-all

Stops all running containers.

#### POST /api/v1/containers/restart-all

Restarts all containers.

#### POST /api/v1/containers/batch

Performs operations on multiple containers.

<Tabs>
<TabItem value="request" label="Request">

```bash
curl -X POST https://localhost:8443/api/v1/containers/batch \
  -H "Content-Type: application/json" \
  -d '{
    "operation": "restart",
    "containerIds": ["abc123", "def456", "ghi789"]
  }'
```

</TabItem>
</Tabs>

---

## Toolbox

One-click Docker Compose service deployment.

### GET /api/v1/toolbox/services

Lists all available services in the toolbox catalog.

**Authentication:** IP-based (local) or JWT

| Parameter | Type | Description |
|-----------|------|-------------|
| `category` | query | Filter by category |

<Tabs>
<TabItem value="request" label="Request">

```bash
curl https://localhost:8443/api/v1/toolbox/services
```

</TabItem>
<TabItem value="response" label="Response">

```json
{
  "services": [
    {
      "id": "portainer",
      "name": "Portainer",
      "description": "Container management UI",
      "category": "management",
      "icon": "container",
      "difficulty": "beginner"
    }
  ],
  "count": 1
}
```

</TabItem>
</Tabs>

### GET /api/v1/toolbox/services/\{id\}

Returns details for a specific service template.

**Authentication:** IP-based (local) or JWT

### POST /api/v1/toolbox/deploy

Deploys a service from the toolbox.

**Authentication:** IP-based (local) or JWT

<Tabs>
<TabItem value="request" label="Request">

```bash
curl -X POST https://localhost:8443/api/v1/toolbox/deploy \
  -H "Content-Type: application/json" \
  -d '{
    "serviceId": "portainer",
    "serviceName": "my-portainer",
    "envVars": {
      "ADMIN_PASSWORD": "secure-password"
    },
    "customPort": 9000,
    "autoStart": true
  }'
```

</TabItem>
<TabItem value="response-202" label="Response 202">

```json
{
  "deployment_id": "deploy_1701388800000000",
  "status": "pending",
  "message": "Deployment 'my-portainer' initiated"
}
```

</TabItem>
</Tabs>

### GET /api/v1/toolbox/deployments

Lists all deployments.

**Authentication:** IP-based (local) or JWT

| Parameter | Type | Description |
|-----------|------|-------------|
| `status` | query | Filter by status |

### GET /api/v1/toolbox/deployments/\{id\}

Returns deployment status.

**Authentication:** IP-based (local) or JWT

### DELETE /api/v1/toolbox/deployments/\{id\}

Removes a deployment.

**Authentication:** IP-based (local) or JWT

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `removeVolumes` | query | false | Also remove associated volumes |

---

## Scripts

Script execution and workflow automation.

### GET /api/v1/scripts

Lists all registered scripts.

**Authentication:** IP-based (local) or JWT

| Parameter | Type | Description |
|-----------|------|-------------|
| `category` | query | Filter by category |

### GET /api/v1/scripts/available

Lists scripts found in the scripts directory that haven't been registered.

**Authentication:** IP-based (local) or JWT

### POST /api/v1/scripts

Registers a new script.

**Authentication:** IP-based (local) or JWT

<Tabs>
<TabItem value="request" label="Request">

```bash
curl -X POST https://localhost:8443/api/v1/scripts \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Backup Database",
    "description": "Creates a database backup",
    "category": "maintenance",
    "scriptPath": "/scripts/backup-db.sh",
    "timeoutSeconds": 300,
    "parameters": [
      {
        "name": "output_dir",
        "type": "string",
        "required": true,
        "default": "/backups"
      }
    ]
  }'
```

</TabItem>
</Tabs>

### GET /api/v1/scripts/\{id\}

Returns script details.

### PUT /api/v1/scripts/\{id\}

Updates a script.

### DELETE /api/v1/scripts/\{id\}

Deletes a script.

### POST /api/v1/scripts/\{id\}/execute

Executes a script.

**Authentication:** IP-based (local) or JWT

<Tabs>
<TabItem value="request" label="Request">

```bash
curl -X POST https://localhost:8443/api/v1/scripts/backup-database/execute \
  -H "Content-Type: application/json" \
  -d '{
    "parameters": {
      "output_dir": "/backups/daily"
    },
    "dryRun": false
  }'
```

</TabItem>
<TabItem value="response" label="Response">

```json
{
  "id": "exec_abc123",
  "scriptId": "backup-database",
  "status": "completed",
  "isDryRun": false,
  "triggeredBy": "dev-a1b2c3d4",
  "parameters": {"output_dir": "/backups/daily"},
  "exitCode": 0,
  "output": "Backup completed successfully",
  "createdAt": "2025-01-15T10:30:00Z",
  "startedAt": "2025-01-15T10:30:01Z",
  "endedAt": "2025-01-15T10:30:15Z"
}
```

</TabItem>
</Tabs>

### POST /api/v1/scripts/\{id\}/dry-run

Executes a script in dry-run mode.

### GET /api/v1/executions

Lists script executions.

| Parameter | Type | Description |
|-----------|------|-------------|
| `scriptId` | query | Filter by script ID |
| `status` | query | Filter by status |
| `limit` | query | Maximum results (default: 50, max: 100) |
| `offset` | query | Skip results |

### GET /api/v1/executions/\{id\}

Returns execution details.

### Workflows

#### GET /api/v1/workflows

Lists all workflows.

#### POST /api/v1/workflows

Creates a workflow.

#### GET /api/v1/workflows/\{id\}

Returns workflow details.

#### DELETE /api/v1/workflows/\{id\}

Deletes a workflow.

### Schedules

#### GET /api/v1/schedules

Lists all schedules.

#### POST /api/v1/schedules

Creates a schedule.

<Tabs>
<TabItem value="request" label="Request">

```bash
curl -X POST https://localhost:8443/api/v1/schedules \
  -H "Content-Type: application/json" \
  -d '{
    "scriptId": "backup-database",
    "cronExpression": "0 2 * * *",
    "parameters": {"output_dir": "/backups/nightly"},
    "enabled": true
  }'
```

</TabItem>
</Tabs>

#### GET /api/v1/schedules/\{id\}

Returns schedule details.

#### DELETE /api/v1/schedules/\{id\}

Deletes a schedule.

---

## Certificates

TLS certificate management.

### POST /api/v1/certificates/generate

Generates a new self-signed certificate.

**Authentication:** JWT token

<Tabs>
<TabItem value="request" label="Request">

```bash
curl -X POST https://localhost:8443/api/v1/certificates/generate \
  -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..." \
  -H "Content-Type: application/json" \
  -d '{
    "domains": ["localhost", "192.168.1.100", "nexus.local"],
    "provider": "self-signed"
  }'
```

</TabItem>
<TabItem value="response" label="Response">

```json
{
  "success": true,
  "certificate": {
    "domain": "localhost",
    "issuer": "Nekzus Self-Signed CA",
    "not_before": "2025-01-15T00:00:00Z",
    "not_after": "2026-01-15T00:00:00Z",
    "sans": ["localhost", "192.168.1.100", "nexus.local"],
    "fingerprint": "SHA256:abc123..."
  },
  "tls_upgraded": true
}
```

</TabItem>
</Tabs>

### GET /api/v1/certificates

Lists all certificates.

**Authentication:** JWT token

<Tabs>
<TabItem value="response" label="Response">

```json
{
  "certificates": [
    {
      "domain": "localhost",
      "issuer": "Nekzus Self-Signed CA",
      "not_before": "2025-01-15T00:00:00Z",
      "not_after": "2026-01-15T00:00:00Z",
      "sans": ["localhost", "192.168.1.100"],
      "fingerprint": "SHA256:abc123...",
      "expires_in_days": 365
    }
  ],
  "count": 1
}
```

</TabItem>
</Tabs>

### GET /api/v1/certificates/suggest

Suggests domains for certificate generation based on local network.

**Authentication:** JWT token

<Tabs>
<TabItem value="response" label="Response">

```json
{
  "suggestions": ["localhost", "macbook-pro.local", "192.168.1.100"],
  "count": 3
}
```

</TabItem>
</Tabs>

### GET /api/v1/certificates/\{domain\}

Returns certificate details for a domain.

### DELETE /api/v1/certificates/\{domain\}

Deletes a certificate.

---

## Backups

Backup and disaster recovery.

### GET /api/v1/backups

Lists all backups.

**Authentication:** IP-based (local) or JWT

### POST /api/v1/backups

Creates a new backup.

**Authentication:** IP-based (local) or JWT

<Tabs>
<TabItem value="request" label="Request">

```bash
curl -X POST https://localhost:8443/api/v1/backups \
  -H "Content-Type: application/json" \
  -d '{"description": "Pre-upgrade backup"}'
```

</TabItem>
</Tabs>

### GET /api/v1/backups/\{id\}

Returns backup details.

### DELETE /api/v1/backups/\{id\}

Deletes a backup.

### POST /api/v1/backups/\{id\}/restore

Restores from a backup.

<Tabs>
<TabItem value="request" label="Request">

```bash
curl -X POST https://localhost:8443/api/v1/backups/backup_123/restore \
  -H "Content-Type: application/json" \
  -d '{
    "restoreApps": true,
    "restoreRoutes": true,
    "restoreDevices": false
  }'
```

</TabItem>
</Tabs>

### GET /api/v1/backups/scheduler/status

Returns backup scheduler status.

### POST /api/v1/backups/scheduler/trigger

Triggers a scheduled backup manually.

---

## System

### GET /api/v1/system/resources

Returns system resource usage (CPU, RAM, disk).

**Authentication:** IP-based (local) or JWT

<Tabs>
<TabItem value="request" label="Request">

```bash
curl https://localhost:8443/api/v1/system/resources
```

</TabItem>
<TabItem value="response" label="Response">

```json
{
  "cpu": 12.5,
  "ram": 45.2,
  "ram_used": 483729408,
  "ram_total": 1073741824,
  "disk": 68.3,
  "disk_used": 107374182400,
  "disk_total": 157286400000,
  "storage_size": 1048576,
  "network": {
    "rx_bytes": 1048576,
    "tx_bytes": 524288
  }
}
```

</TabItem>
</Tabs>

### GET /api/v1/stats/quick

Returns quick stats optimized for mobile/widgets.

**Authentication:** JWT token (strict)

### GET /api/v1/services/\{appId\}/health

Returns health status for a specific service.

**Authentication:** IP-based (local) or JWT

<Tabs>
<TabItem value="request" label="Request">

```bash
curl https://localhost:8443/api/v1/services/grafana/health
```

</TabItem>
<TabItem value="response" label="Response">

```json
{
  "appId": "grafana",
  "status": "healthy",
  "lastCheck": "2025-01-15T14:22:00Z",
  "responseTime": 45,
  "statusCode": 200
}
```

</TabItem>
</Tabs>

---

## Webhooks

External notification endpoints.

### POST /api/v1/webhooks/activity

Creates an activity event via webhook.

**Authentication:** API key or JWT

<Tabs>
<TabItem value="request" label="Request">

```bash
curl -X POST https://localhost:8443/api/v1/webhooks/activity \
  -H "X-API-Key: nekzus_abc123..." \
  -H "Content-Type: application/json" \
  -d '{
    "message": "Deployment completed",
    "icon": "CheckCircle",
    "iconClass": "success",
    "details": "Version 2.0.0 deployed successfully",
    "deviceIds": ["dev-a1b2c3d4"]
  }'
```

</TabItem>
<TabItem value="response" label="Response">

```json
{
  "success": true,
  "eventId": "webhook-1701388800000000"
}
```

</TabItem>
</Tabs>

### POST /api/v1/webhooks/notify

Sends an arbitrary notification via WebSocket.

**Authentication:** API key or JWT

<Tabs>
<TabItem value="request" label="Request">

```bash
curl -X POST https://localhost:8443/api/v1/webhooks/notify \
  -H "X-API-Key: nekzus_abc123..." \
  -H "Content-Type: application/json" \
  -d '{
    "type": "custom_event",
    "data": {
      "title": "Build Complete",
      "status": "success"
    },
    "deviceIds": []
  }'
```

</TabItem>
<TabItem value="response" label="Response">

```json
{
  "success": true,
  "sent": true
}
```

</TabItem>
</Tabs>

---

## Federation

Peer-to-peer federation for multi-Nexus deployments.

### GET /api/v1/federation/peers

Lists all federation peers.

**Authentication:** IP-based (local) or JWT

### GET /api/v1/federation/peers/\{id\}

Returns peer details.

### DELETE /api/v1/federation/peers/\{id\}

Removes a peer from the federation.

### POST /api/v1/federation/sync

Triggers a catalog sync with all peers.

### GET /api/v1/federation/status

Returns federation status and health.

<Tabs>
<TabItem value="response" label="Response">

```json
{
  "enabled": true,
  "local_peer_id": "peer_abc123",
  "local_peer_name": "nexus-primary",
  "peer_count": 3,
  "peers_by_status": {
    "connected": 2,
    "disconnected": 1
  },
  "running": true
}
```

</TabItem>
</Tabs>

---

## WebSocket

Real-time bidirectional communication.

### GET /api/v1/ws

Establishes a WebSocket connection for real-time updates.

**Rate Limit:** 6 req/min, burst 3

#### Connection Flow

1. Client connects to `/api/v1/ws`
2. Client sends `auth` message with JWT token
3. Server responds with `auth_success` or `auth_failed`
4. Server sends `hello` message with connection info
5. Bidirectional messaging with ping/pong keepalive

#### Authentication Message

```json
{
  "type": "auth",
  "data": {
    "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
  }
}
```

#### Message Types

| Type | Direction | Description |
|------|-----------|-------------|
| `auth` | Client -> Server | Authentication request |
| `auth_success` | Server -> Client | Authentication succeeded |
| `auth_failed` | Server -> Client | Authentication failed |
| `hello` | Server -> Client | Connection established |
| `ping` / `pong` | Bidirectional | Keepalive |
| `discovery` | Server -> Client | Service discovery event |
| `config_reload` | Server -> Client | Configuration changed |
| `device_paired` | Server -> Client | New device paired |
| `device_revoked` | Server -> Client | Device access revoked |
| `health_change` | Server -> Client | Service health changed |
| `webhook` | Server -> Client | Webhook notification |
| `container.start.completed` | Server -> Client | Container started |
| `container.stop.completed` | Server -> Client | Container stopped |
| `container.restart.completed` | Server -> Client | Container restarted |
| `container.logs.data` | Server -> Client | Container log data |
| `notification_ack` | Client -> Server | Notification acknowledged |

#### Example: Subscribe to Container Logs

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

---

## Metrics

### GET /metrics

Prometheus metrics endpoint.

**Authentication:** None
**Rate Limit:** 30 req/min

Returns Prometheus-formatted metrics for monitoring.

```bash
curl https://localhost:8443/metrics
```

---

## Reverse Proxy

### /apps/\{appId\}/*

Proxies requests to registered applications.

**Authentication:** IP-based (local) or JWT (configurable per route)

The proxy supports:

- HTTP/HTTPS proxying
- WebSocket upgrades (when enabled on route)
- Server-Sent Events (SSE)
- Path prefix stripping
- Header forwarding
- Response caching (configurable)

Example:

```bash
# Access Grafana through the proxy
curl https://localhost:8443/apps/grafana/
```

---

## Session Cookies

Mobile webview session persistence.

### GET /api/v1/session-cookies

Lists stored session summaries (no cookie values).

**Authentication:** JWT token (strict)

### DELETE /api/v1/session-cookies

Clears all sessions for the device.

### DELETE /api/v1/session-cookies/\{appId\}

Clears sessions for a specific app.

---

## Export

Container configuration export for migration.

### GET /api/v1/containers/\{containerId\}/export/preview

Previews export configuration.

**Authentication:** IP-based (local) or JWT

### POST /api/v1/containers/\{containerId\}/export

Exports container configuration.

**Authentication:** IP-based (local) or JWT

### POST /api/v1/containers/batch/export/preview

Previews batch export configuration.

### POST /api/v1/containers/batch/export

Exports multiple container configurations.
