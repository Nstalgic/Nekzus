import Tabs from "@theme/Tabs";
import TabItem from "@theme/TabItem";

# Notifications

Nekzus provides a comprehensive notification system for pushing real-time alerts and messages to connected devices. The system supports both immediate delivery via WebSocket and queued delivery for offline devices.

---

## Overview

The notifications system enables:

- **Real-time delivery** - Instant WebSocket notifications to connected clients
- **Offline queuing** - Automatic queueing for devices that are not currently connected
- **Targeted delivery** - Send notifications to specific devices or broadcast to all
- **Activity persistence** - Activity notifications are stored in the activity log
- **Queue management** - Web UI and API for managing pending notifications
- **Retry mechanism** - Automatic and manual retry for failed deliveries

---

## Architecture

### Notification Flow

```d2
direction: down

webhook: Webhook API\n/webhooks/* {
  style.fill: "#7c3aed"
  style.font-color: "#ffffff"
}

router: Notification Router

devices: {
  grid-columns: 2
  online: Online Devices\n(WebSocket)
  offline: Offline Devices\n(Notification Queue)
}

delivery: {
  grid-columns: 2
  immediate: Immediate Delivery
  queued: Queued for Later Delivery
}

webhook -> router -> devices -> delivery
```

### Components

| Component | Description |
|-----------|-------------|
| Webhook Endpoints | HTTP endpoints for external integrations |
| WebSocket Manager | Manages real-time connections and message delivery |
| Notification Queue | SQLite-backed queue for offline device notifications |
| Activity Tracker | Persists activity events for the dashboard |

---

## Webhook Endpoints

### POST /api/v1/webhooks/activity

Creates an activity event that is displayed in the activity feed and optionally sent to devices.

**Authentication:** API Key or JWT

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `message` | string | Yes | The notification message |
| `icon` | string | No | Icon name (default: `"Bell"`) |
| `iconClass` | string | No | Icon style: `success`, `warning`, `danger`, `info` |
| `details` | string | No | Additional details |
| `deviceIds` | array | No | Target device IDs (empty = broadcast to all) |

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
    "details": "Version 2.0.0 deployed to production",
    "deviceIds": ["device-abc123"]
  }'
```

</TabItem>
<TabItem value="response-200" label="Response 200">

```json
{
  "success": true,
  "eventId": "webhook-1736949720000000"
}
```

</TabItem>
<TabItem value="response-400" label="Response 400">

```json
{
  "error": {
    "code": "INVALID_REQUEST",
    "message": "Missing required field: message"
  }
}
```

</TabItem>
</Tabs>

**Behavior:**

- Creates an `ActivityEvent` with type `webhook.activity`
- Persists the event to the activity tracker (visible in dashboard)
- Broadcasts via WebSocket to all connected clients (if no `deviceIds` specified)
- For targeted delivery, sends only to specified devices
- Queues notifications for offline devices (30-day TTL, 5 max retries)

### POST /api/v1/webhooks/notify

Sends an arbitrary notification payload via WebSocket without persisting to the activity log.

**Authentication:** API Key or JWT

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | No | Custom notification type |
| `data` | object | No | Arbitrary JSON payload |
| `deviceIds` | array | No | Target device IDs (empty = broadcast to all) |

<Tabs>
<TabItem value="request" label="Request">

```bash
curl -X POST https://localhost:8443/api/v1/webhooks/notify \
  -H "X-API-Key: nekzus_abc123..." \
  -H "Content-Type: application/json" \
  -d '{
    "type": "custom_alert",
    "data": {
      "alertType": "cpu_usage",
      "threshold": 90,
      "current": 95,
      "message": "CPU usage critical"
    },
    "deviceIds": ["device-abc123", "device-def456"]
  }'
```

</TabItem>
<TabItem value="response-200" label="Response 200">

```json
{
  "success": true,
  "sent": true
}
```

</TabItem>
</Tabs>

**Behavior:**

- Sends the payload directly via WebSocket (not persisted to activity log)
- Supports broadcast (empty `deviceIds`) or targeted delivery
- Queues notifications for offline devices when `deviceIds` is specified

---

## Notification Queue Management

The notification queue stores pending notifications for offline devices. These endpoints allow administrators to view, retry, and dismiss queued notifications.

### GET /api/v1/notifications

Lists notifications with optional filtering and pagination.

**Authentication:** IP-based (local) or JWT

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `status` | query | - | Filter: `pending`, `delivered`, `failed`, `dismissed` |
| `device_id` | query | - | Filter by device ID |
| `type` | query | - | Filter by notification type |
| `limit` | query | 50 | Max results (max: 200) |
| `offset` | query | 0 | Pagination offset |

<Tabs>
<TabItem value="request" label="Request">

```bash
curl "https://localhost:8443/api/v1/notifications?status=pending&limit=10"
```

</TabItem>
<TabItem value="response-200" label="Response 200">

```json
{
  "notifications": [
    {
      "id": 123,
      "deviceId": "device-abc123",
      "deviceName": "John's iPhone",
      "type": "webhook.activity",
      "status": "pending",
      "retryCount": 0,
      "maxRetries": 5,
      "createdAt": "2025-01-15T10:30:00Z",
      "expiresAt": "2025-02-14T10:30:00Z",
      "isStale": false
    }
  ],
  "total": 42,
  "limit": 10,
  "offset": 0
}
```

</TabItem>
</Tabs>

### GET /api/v1/notifications/stats

Returns queue statistics.

**Authentication:** IP-based (local) or JWT

<Tabs>
<TabItem value="request" label="Request">

```bash
curl https://localhost:8443/api/v1/notifications/stats
```

</TabItem>
<TabItem value="response-200" label="Response 200">

```json
{
  "totalPending": 15,
  "totalDelivered": 234,
  "totalFailed": 3,
  "staleCount": 2
}
```

</TabItem>
</Tabs>

### GET /api/v1/notifications/stale

Returns stale notifications grouped by device. A notification is considered stale if it has been pending for more than 24 hours.

**Authentication:** IP-based (local) or JWT

<Tabs>
<TabItem value="request" label="Request">

```bash
curl https://localhost:8443/api/v1/notifications/stale
```

</TabItem>
<TabItem value="response-200" label="Response 200">

```json
{
  "staleThresholdHours": 24,
  "devices": [
    {
      "deviceId": "device-abc123",
      "deviceName": "John's iPhone",
      "pendingCount": 5,
      "oldestNotification": "2025-01-14T10:30:00Z",
      "types": ["webhook.activity", "webhook.notify"]
    }
  ]
}
```

</TabItem>
</Tabs>

### DELETE /api/v1/notifications/\{id\}

Dismisses a single notification. The notification is marked as `dismissed` and will not be delivered.

**Authentication:** IP-based (local) or JWT

<Tabs>
<TabItem value="request" label="Request">

```bash
curl -X DELETE https://localhost:8443/api/v1/notifications/123
```

</TabItem>
<TabItem value="response-200" label="Response 200">

```json
{
  "status": "dismissed",
  "id": 123,
  "message": "Notification dismissed"
}
```

</TabItem>
<TabItem value="response-404" label="Response 404">

```json
{
  "error": {
    "code": "NOT_FOUND",
    "message": "Notification not found or already processed"
  }
}
```

</TabItem>
</Tabs>

### DELETE /api/v1/notifications/device/\{deviceId\}

Dismisses all pending notifications for a specific device.

**Authentication:** IP-based (local) or JWT

<Tabs>
<TabItem value="request" label="Request">

```bash
curl -X DELETE https://localhost:8443/api/v1/notifications/device/device-abc123
```

</TabItem>
<TabItem value="response-200" label="Response 200">

```json
{
  "status": "dismissed",
  "deviceId": "device-abc123",
  "count": 5,
  "message": "Notifications dismissed for device"
}
```

</TabItem>
</Tabs>

### POST /api/v1/notifications/\{id\}/retry

Resets a notification status to `pending` for retry delivery.

**Authentication:** IP-based (local) or JWT

<Tabs>
<TabItem value="request" label="Request">

```bash
curl -X POST https://localhost:8443/api/v1/notifications/123/retry
```

</TabItem>
<TabItem value="response-200" label="Response 200">

```json
{
  "status": "queued",
  "id": 123,
  "message": "Notification queued for retry"
}
```

</TabItem>
</Tabs>

### POST /api/v1/notifications/retry

Bulk retry multiple notifications.

**Authentication:** IP-based (local) or JWT

<Tabs>
<TabItem value="request" label="Request">

```bash
curl -X POST https://localhost:8443/api/v1/notifications/retry \
  -H "Content-Type: application/json" \
  -d '{"ids": [123, 124, 125]}'
```

</TabItem>
<TabItem value="response-200" label="Response 200">

```json
{
  "status": "queued",
  "count": 3,
  "message": "Notifications queued for retry"
}
```

</TabItem>
</Tabs>

### DELETE /api/v1/notifications/delivered

Clears all delivered notifications from the queue. This is useful for cleanup after confirming notifications were received.

**Authentication:** IP-based (local) or JWT

<Tabs>
<TabItem value="request" label="Request">

```bash
curl -X DELETE https://localhost:8443/api/v1/notifications/delivered
```

</TabItem>
<TabItem value="response-200" label="Response 200">

```json
{
  "status": "cleared",
  "count": 42,
  "message": "Delivered notifications cleared"
}
```

</TabItem>
</Tabs>

---

## WebSocket Message Types

Notifications are delivered via WebSocket using the following message structure:

### Activity Notification (from webhook/activity)

```json
{
  "type": "webhook",
  "data": {
    "id": "webhook-1736949720000000",
    "type": "webhook.activity",
    "icon": "CheckCircle",
    "iconClass": "success",
    "message": "Deployment completed",
    "details": "Version 2.0.0 deployed",
    "timestamp": 1736949720000
  },
  "timestamp": "2025-01-15T10:30:00Z"
}
```

### Custom Notification (from webhook/notify)

```json
{
  "type": "webhook",
  "data": {
    "type": "custom_alert",
    "data": {
      "alertType": "cpu_usage",
      "threshold": 90,
      "current": 95,
      "message": "CPU usage critical"
    },
    "deviceIds": ["device-abc123"]
  },
  "timestamp": "2025-01-15T10:30:00Z"
}
```

---

## Notification Statuses

| Status | Description |
|--------|-------------|
| `pending` | Awaiting delivery to device |
| `delivered` | Successfully delivered via WebSocket |
| `failed` | Delivery failed (will retry if retries remain) |
| `dismissed` | Manually dismissed by administrator |

---

## Queue Configuration

Notifications are queued with the following default settings:

| Setting | Value | Description |
|---------|-------|-------------|
| TTL | 30 days | Time-to-live for queued notifications |
| Max Retries | 5 | Maximum delivery attempts |
| Stale Threshold | 24 hours | Time after which pending notifications are marked stale |

---

## Frontend Integration

### NotificationsTab Component

The web dashboard includes a `NotificationsTab` component that provides:

- **Statistics overview** - Pending, delivered, and failed counts
- **Stale warning** - Alert when devices have notifications pending > 24 hours
- **Status filtering** - Filter by pending, delivered, failed, or dismissed
- **Bulk operations** - Retry all failed notifications
- **Individual actions** - Retry or dismiss specific notifications

### Real-time Updates

The frontend subscribes to WebSocket events to receive notifications in real-time:

```javascript
// Example: Listening for webhook notifications
websocket.onmessage = (event) => {
  const message = JSON.parse(event.data);
  if (message.type === 'webhook') {
    // Handle activity event
    if (message.data.type === 'webhook.activity') {
      showActivityNotification(message.data);
    }
    // Handle custom notification
    else if (message.data.type) {
      handleCustomNotification(message.data);
    }
  }
};
```

---

## Icon Reference

The `icon` field supports icon names from the Lucide icon library:

| Icon | Use Case |
|------|----------|
| `Bell` | Default notifications |
| `CheckCircle` | Success messages |
| `AlertTriangle` | Warnings |
| `AlertCircle` | Errors |
| `Info` | Informational |
| `Target` | Targeted actions |
| `Radio` | Broadcast messages |
| `Database` | Storage/data events |
| `Refresh` | Update notifications |

### Icon Classes

The `iconClass` field controls the color styling:

| Class | Color | Use Case |
|-------|-------|----------|
| `success` | Green | Successful operations |
| `warning` | Yellow/Orange | Warnings, attention needed |
| `danger` | Red | Errors, critical alerts |
| `info` | Blue | Informational messages |
| (none) | Default | Neutral notifications |

---

## Integration Examples

### CI/CD Pipeline Notification

Send deployment notifications from your CI/CD pipeline:

```bash
# In your deployment script
curl -X POST https://nexus.local:8443/api/v1/webhooks/activity \
  -H "X-API-Key: ${NEKZUS_API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "message": "Deployment to production completed",
    "icon": "CheckCircle",
    "iconClass": "success",
    "details": "Commit: '"${GIT_SHA}"' | Duration: '"${DEPLOY_TIME}"'s"
  }'
```

### Monitoring Alert Integration

Forward alerts from your monitoring system:

```bash
# Prometheus Alertmanager webhook receiver
curl -X POST https://nexus.local:8443/api/v1/webhooks/notify \
  -H "X-API-Key: ${NEKZUS_API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "type": "prometheus_alert",
    "data": {
      "alertname": "HighCPUUsage",
      "severity": "warning",
      "instance": "server-01",
      "value": 95,
      "summary": "CPU usage is above 90%"
    }
  }'
```

### Backup Completion Notification

Notify specific devices when backups complete:

```bash
# Send to specific admin devices
curl -X POST https://nexus.local:8443/api/v1/webhooks/activity \
  -H "X-API-Key: ${NEKZUS_API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "message": "Daily backup completed successfully",
    "icon": "Database",
    "iconClass": "success",
    "details": "Size: 2.4GB | Duration: 45s",
    "deviceIds": ["admin-device-1", "admin-device-2"]
  }'
```

### JavaScript/Node.js Example

```javascript
const axios = require('axios');

async function sendNotification(message, options = {}) {
  const payload = {
    message,
    icon: options.icon || 'Bell',
    iconClass: options.iconClass || '',
    details: options.details || '',
    deviceIds: options.deviceIds || []
  };

  try {
    const response = await axios.post(
      'https://nexus.local:8443/api/v1/webhooks/activity',
      payload,
      {
        headers: {
          'X-API-Key': process.env.NEKZUS_API_KEY,
          'Content-Type': 'application/json'
        }
      }
    );
    console.log('Notification sent:', response.data.eventId);
    return response.data;
  } catch (error) {
    console.error('Failed to send notification:', error.message);
    throw error;
  }
}

// Usage
sendNotification('Build completed', {
  icon: 'CheckCircle',
  iconClass: 'success',
  details: 'Build #1234 finished in 5m 32s'
});
```

### Python Example

```python
import requests
import os

def send_notification(message, icon='Bell', icon_class='', details='', device_ids=None):
    """Send a notification via Nekzus webhook."""
    payload = {
        'message': message,
        'icon': icon,
        'iconClass': icon_class,
        'details': details,
        'deviceIds': device_ids or []
    }

    response = requests.post(
        'https://nexus.local:8443/api/v1/webhooks/activity',
        json=payload,
        headers={
            'X-API-Key': os.environ['NEKZUS_API_KEY'],
            'Content-Type': 'application/json'
        },
        verify=False  # Set to True in production with proper certs
    )

    response.raise_for_status()
    return response.json()

# Usage
result = send_notification(
    message='Task completed',
    icon='CheckCircle',
    icon_class='success',
    details='Processed 1,000 records in 30 seconds'
)
print(f"Event ID: {result['eventId']}")
```

### Go Example

```go
package main

import (
    "bytes"
    "encoding/json"
    "fmt"
    "net/http"
    "os"
)

type ActivityPayload struct {
    Message   string   `json:"message"`
    Icon      string   `json:"icon,omitempty"`
    IconClass string   `json:"iconClass,omitempty"`
    Details   string   `json:"details,omitempty"`
    DeviceIDs []string `json:"deviceIds,omitempty"`
}

func sendNotification(payload ActivityPayload) error {
    body, err := json.Marshal(payload)
    if err != nil {
        return err
    }

    req, err := http.NewRequest(
        "POST",
        "https://nexus.local:8443/api/v1/webhooks/activity",
        bytes.NewReader(body),
    )
    if err != nil {
        return err
    }

    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("X-API-Key", os.Getenv("NEKZUS_API_KEY"))

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("unexpected status: %d", resp.StatusCode)
    }

    return nil
}

func main() {
    err := sendNotification(ActivityPayload{
        Message:   "Service restarted",
        Icon:      "Refresh",
        IconClass: "info",
        Details:   "All services are now running",
    })
    if err != nil {
        fmt.Printf("Error: %v\n", err)
    }
}
```

---

## Troubleshooting

### Notifications Not Received

1. **Check WebSocket connection** - Ensure the device is connected via WebSocket
2. **Verify device ID** - Confirm the `deviceIds` array contains valid device IDs
3. **Check queue** - Use `/api/v1/notifications` to see if notification is queued
4. **Review logs** - Check server logs for delivery errors

### Stale Notifications

Stale notifications indicate devices that have been offline for extended periods:

1. Check if the device is actually offline
2. Consider dismissing notifications for inactive devices
3. Review device activity in the Devices tab

### Queue Growing

If the notification queue keeps growing:

1. Identify offline devices with `/api/v1/notifications/stale`
2. Dismiss notifications for devices that are no longer active
3. Consider revoking inactive device access
4. Review TTL settings if notifications expire too slowly

### API Key Authentication Fails

1. Verify API key is not revoked
2. Check key has `write:*` scope
3. Ensure key is sent in `X-API-Key` header
4. Confirm key format: `nekzus_<64-character-hex>`

---

## Related Documentation

- [API Reference](../reference/api) - Complete API documentation
- [WebSocket Guide](../reference/api#websocket) - WebSocket connection details
- [API Keys](../reference/api#api-keys) - Creating and managing API keys
- [Activity](../reference/api#admin-endpoints) - Activity log endpoints
