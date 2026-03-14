import Tabs from "@theme/Tabs";
import TabItem from "@theme/TabItem";

# Web Dashboard

Nekzus includes a modern, embedded React-based web dashboard for managing your API gateway, monitoring services, and configuring routes. The dashboard is compiled directly into the Go binary, requiring no separate frontend deployment.

---

## Overview

The web dashboard provides a terminal-inspired interface for:

- **System Monitoring** - Real-time CPU, memory, and disk usage with historical graphs
- **Route Management** - Create, edit, and delete proxy routes
- **Service Discovery** - Review and approve discovered services
- **Device Management** - Pair and manage connected devices
- **Container Operations** - Start, stop, restart, and export Docker containers
- **Settings & Customization** - 15 visual themes and extensive configuration options

:::tip[Quick Access]

After starting Nekzus, access the dashboard at:

- **Local**: `http://localhost:8080`
- **Production (TLS)**: `https://your-server:8443`

:::


---

## Dashboard Layout

The dashboard uses a three-section layout optimized for operational efficiency:

![Dashboard Layout](/dashboard-full.webp)

---

## Dashboard Pages

### Overview Panel

The Overview panel displays key metrics at a glance:

| Metric | Description |
|--------|-------------|
| **Active Routes** | Number of currently active proxy routes |
| **Paired Devices** | Total devices paired with this instance |
| **Pending Discoveries** | Services awaiting approval (highlighted if > 0) |
| **Total Requests** | Cumulative requests proxied through Nexus |

Click any metric to jump directly to its corresponding management tab.

### Recent Activity

Displays the last 6 system events in chronological order:

- Device pairing events
- Route configuration changes
- Discovery approvals/rejections
- Webhook notifications
- Container state changes

Timestamps show relative time (e.g., "2m ago", "1h ago").

### System Health

Real-time system status with visual indicators:

<Tabs>
<TabItem value="resource-meters" label="Resource Meters">


```
CPU  [========----] 65%
MEM  [==========--] 8.2/16.0 GB
DISK [============] 120.5/500.0 GB
```

</TabItem>
<TabItem value="status-badges" label="Status Badges">


| Component | Status Types |
|-----------|-------------|
| DATABASE | File size display |
| AUTHENTICATION | ONLINE / OFFLINE |
| DISCOVERY | IDLE / N PENDING |
| WEBSOCKET | CONNECTED / DISCONNECTED |
| ROUTES | N ACTIVE |
| DEVICES | N PAIRED |

</TabItem>
</Tabs>

---

## Management Console Tabs

### Routes Tab

Complete route management interface with full CRUD operations.

**Features:**

- Sortable table with search functionality
- Add new routes with the "Add Route" button
- Edit existing routes inline
- Delete routes with confirmation dialog
- External link icons to open routes in new tabs

**Table Columns:**

| Column | Description |
|--------|-------------|
| Application | Route identifier (app ID) |
| Path | URL path prefix (e.g., `/apps/grafana/`) |
| Target | Upstream service URL |
| Scopes | Required authentication scopes |
| Status | ACTIVE, INACTIVE, PENDING, or OFFLINE |

**Actions:**

- Click the edit icon to modify route settings
- Click the delete icon to remove a route (respects confirmation settings)

---

### Discovery Tab

Review and manage auto-discovered services from Docker, Kubernetes, or mDNS.

**Features:**

- Grid layout of discovery cards
- Bulk selection with "Select All" checkbox
- Bulk approve/reject actions
- Individual approve/reject per card
- Rediscover button to trigger a fresh scan
- Port selection for multi-port containers

**Card Information:**

Each discovery card displays:

- Service name and type (Docker, K8s, mDNS)
- Address and port(s)
- Confidence score
- Container/pod metadata
- Available actions

**Bulk Operations:**

```
[ ] Select All    [REJECT SELECTED]    [APPROVE SELECTED]    [REDISCOVER]
```

:::info[Confidence Scoring]

Discovery proposals include a confidence score based on:

- Label completeness
- Service mesh detection (Istio)
- Helm chart recognition
- Naming conventions

:::


---

### Devices Tab

Manage paired devices and their access permissions.

**Features:**

- Grid display of paired devices
- Device status indicators (online/offline)
- View detailed device information
- Revoke device access
- Pair new devices via QR code

**Device Card Information:**

| Field | Description |
|-------|-------------|
| Device Name | User-defined or auto-detected name |
| Platform | iOS, Android, macOS, Windows, etc. |
| Last Seen | Timestamp of last activity |
| Status | Online or Offline indicator |
| IP Address | Last known IP address |

**Pairing New Devices:**

1. Click "PAIR NEW DEVICE"
2. A QR code modal appears with:
   - Scannable QR code
   - Expiration countdown timer (5 minutes)
   - Bootstrap token (for manual entry)
3. Scan with the Nekzus mobile app
4. Device appears in the list after successful pairing

---

### Containers Tab

Docker and Kubernetes container management interface.

**Features:**

- Real-time container status and resource usage
- Start, stop, restart operations
- View container logs
- Inspect container details
- Export to Docker Compose format
- Batch export for multiple containers
- Filter by state (All, Running, Stopped)
- Filter by runtime (Docker, Kubernetes)

**Container Card Information:**

- Container name and image
- Current state with color-coded badge
- CPU and memory usage (for running containers)
- Port mappings
- Network information
- Runtime indicator (Docker/K8s)

**Export Options:**

<Tabs>
<TabItem value="single-container" label="Single Container">


Export individual containers with options:

- Include volumes
- Include networks
- Include environment variables
- Generate YAML preview

</TabItem>
<TabItem value="batch-export" label="Batch Export">


Select multiple containers for combined export:

- Single docker-compose.yml
- ZIP archive with separate files
- Shared network configuration

</TabItem>
</Tabs>

---

### Scripts Tab

:::note[Optional Feature]

The Scripts tab is enabled by default but can be disabled in Settings.

:::


Manage automated scripts and scheduled tasks.

**View Modes:**

| View | Description |
|------|-------------|
| Scripts | Browse and execute registered scripts |
| Executions | View execution history and status |
| Workflows | Multi-step script sequences |
| Schedules | Cron-based scheduled jobs |

**Script Actions:**

- Execute with parameters
- Dry run (validation only)
- Edit script configuration
- Delete script registration

---

### Metrics Tab

Prometheus-style metrics dashboard with real-time data.

**Metric Categories:**

<Tabs>
<TabItem value="http-metrics" label="HTTP Metrics">


- Total requests processed
- In-flight requests
- Average latency (ms)
- Error rate percentage
- Latency percentiles (p50, p95, p99)
- Requests by status code
- Requests by HTTP method

</TabItem>
<TabItem value="proxy-metrics" label="Proxy Metrics">


- Total proxy requests
- Active sessions
- Bytes transferred (in/out)
- Upstream errors
- Per-application breakdowns

</TabItem>
<TabItem value="websocket-metrics" label="WebSocket Metrics">


- Active connections
- Total connections lifetime
- Average connection duration
- Messages processed
- Data transfer volume

</TabItem>
<TabItem value="auth-metrics" label="Auth Metrics">


- JWT validation success rate
- Device pairing success rate
- Token refresh count
- Local auth bypasses
- Active bootstrap tokens

</TabItem>
<TabItem value="discovery-metrics" label="Discovery Metrics">


- Total scans performed
- Proposals generated
- Pending proposals
- Active workers
- Breakdown by source

</TabItem>
<TabItem value="system-metrics" label="System Metrics">


- Server uptime
- Config reload count
- Last reload status
- Certificate count
- Notification queue depth

</TabItem>
</Tabs>

**Auto-Refresh:**

Toggle auto-refresh on/off and manually refresh with the "REFRESH" button.

---

### Settings Tab

Comprehensive configuration interface organized into sections.

#### General Settings

| Setting | Default | Description |
|---------|---------|-------------|
| Dashboard Refresh Interval | 10s | How often to poll for updates |
| Timezone | UTC | Timestamp display timezone |
| Show Timestamp | true | Display timestamp in footer |
| Enable Toolbox | false | Show/hide Toolbox tab |
| Enable Scripts | true | Show/hide Scripts tab |
| Show Only Routed Containers | false | Filter containers by route presence |

#### Discovery Settings

| Setting | Default | Description |
|---------|---------|-------------|
| Auto-Approval Threshold | 100% | Confidence level for auto-approval |
| Notification Badge Threshold | 5 | Discovery count to show badge |
| Require Confirmation for Rejections | false | Prompt before rejecting |

#### Security Settings

| Setting | Default | Description |
|---------|---------|-------------|
| Session Timeout | 30 min | Idle timeout duration |
| Require Confirmation | true | Prompt for destructive actions |

#### Appearance Settings

| Setting | Options | Description |
|---------|---------|-------------|
| Terminal Theme | 15 themes | Visual theme selection |
| Font Size | Small, Medium, Large | UI font sizing |
| Show ASCII Logo | false | Display logo in Overview |
| Compact Mode | false | Reduce spacing |

#### Webhooks

Configure webhook endpoints for external integrations:

- **Webhook URL**: Auto-generated endpoint
- **Webhook Key**: API key for authentication
- Generate/regenerate keys
- Send test webhook

#### Notifications

Toggle notifications for:

- New discoveries
- Device offline alerts
- Certificate expiration warnings
- Route status changes
- System health alerts

Test notification buttons for each severity level.

#### Certificates

TLS certificate management:

- View installed certificates
- Check expiration dates
- Generate self-signed certificates
- Auto-detect local domains
- Delete certificates

#### Data Management

- Export settings to JSON
- Import settings from JSON
- Clear all local data

#### System Information

- Frontend version
- Backend version
- Connection status
- Webhook ID
- Local storage usage

#### Developer Options

- Debug mode (verbose logging)
- Show error details in UI
- Log WebSocket events
- Webhook testing tool

---

## Theme System

Nekzus includes 15 carefully crafted themes to match your preferences.

### Available Themes

| Theme | Description | Style |
|-------|-------------|-------|
| **Default (Slate Professional)** | Modern neutral palette | Dark |
| **Obsidian Dark** | Pure black OLED-friendly | Dark |
| **Nord Frost** | Nordic-inspired pastels | Dark |
| **Carbon Neutral** | IBM Carbon-inspired | Dark |
| **Classic Green** | Traditional terminal | Dark |
| **Cyan** | Teal and cyan palette | Dark |
| **Amber** | Warm amber monochrome | Dark |
| **Gruvbox** | Warm retro colors | Dark |
| **Gruvbox Light** | Light cream variant | Light |
| **Tokyo Night** | Deep blue with pastels | Dark |
| **Tokyo Night Storm** | Lighter Tokyo variant | Dark |
| **Catppuccin Mocha** | Warm pastel palette | Dark |
| **Pipboy** | Amber CRT with effects | Dark |
| **Pipboy Green** | Green CRT with effects | Dark |
| **Retro** | 80s vaporwave neon | Dark |

### CRT Effect Themes

The Pipboy themes include authentic CRT effects:

- Scanline overlay
- Screen flicker animation
- Radial glow from center
- Vignette (darkened edges)
- Screen curvature simulation
- Phosphor glow on text

:::tip[Performance Note]

CRT effects are optimized for modern browsers. On mobile devices, some effects are automatically disabled for better performance.

:::


### Changing Themes

<Tabs>
<TabItem value="settings-panel" label="Settings Panel">


1. Navigate to the **Settings** tab
2. Find the **Appearance** section
3. Select a theme from the dropdown

</TabItem>
<TabItem value="keyboard-shortcut" label="Keyboard Shortcut">


Press <kbd>Ctrl</kbd>+<kbd>K</kbd> (or <kbd>Cmd</kbd>+<kbd>K</kbd> on macOS) to open the theme switcher modal.

</TabItem>
</Tabs>

Themes are persisted in localStorage and apply immediately without page reload.

---

## Keyboard Shortcuts

| Shortcut | Action |
|----------|--------|
| <kbd>Ctrl</kbd>+<kbd>K</kbd> / <kbd>Cmd</kbd>+<kbd>K</kbd> | Open theme switcher |

---

## Real-Time Updates

The dashboard maintains a persistent WebSocket connection for real-time updates.

### Event Types

| Event | Action |
|-------|--------|
| `discovery` | Refreshes discovery proposals |
| `config_reload` | Refreshes routes and stats |
| `device_paired` | Refreshes devices and activity |
| `device_revoked` | Refreshes devices and activity |
| `app_registered` | Refreshes routes after approval |
| `proposal_dismissed` | Refreshes discoveries |
| `health_change` | Refreshes route health status |
| `webhook` | Adds activity and notification |

### Connection Status

The WebSocket connection status is displayed in the System Health panel:

- **CONNECTED** (green): Real-time updates active
- **DISCONNECTED** (red): Automatic reconnection in progress

The connection automatically reconnects on disconnection with exponential backoff.

---

## Mobile Responsiveness

The dashboard is fully responsive and works on all device sizes.

### Breakpoints

| Breakpoint | Layout Changes |
|------------|----------------|
| > 1024px | Three-column overview layout |
| 768px - 1024px | Single-column overview |
| < 768px | Stacked layout, touch-optimized |

### Mobile Optimizations

- Touch-friendly button sizes
- Collapsible sections
- Reduced CRT effects on mobile
- Optimized font sizing
- Scrollable tables

---

## Server Resources Panel

Real-time resource monitoring with historical graphs.

### Displayed Metrics

| Metric | Update Interval | History |
|--------|-----------------|---------|
| CPU Usage | 5 seconds | 15 minutes |
| Memory Usage | 5 seconds | 15 minutes |
| Disk Usage | 5 seconds | 15 minutes |

### Graph Features

- Line charts with smooth animations
- Rolling 15-minute window (180 data points)
- Percentage scale (0-100%)
- Timestamp-based X-axis
- Color-coded by metric type

---

## Authentication Flow

The dashboard implements a secure authentication flow:

```
1. Check Setup Status (/api/v1/auth/setup-status)
   |
   +---> Setup Required? ---> Show Setup Page
   |
   +---> Not Required? ---> Check Authentication
         |
         +---> Not Authenticated? ---> Show Login Page
         |
         +---> Authenticated? ---> Show Dashboard
```

### Login Page

- Username and password authentication
- Session persistence via JWT
- Error display for invalid credentials

### Setup Page

First-time setup for new installations:

- Create admin user
- Configure initial settings

---

## Performance Features

### Code Splitting

The dashboard uses React lazy loading for optimal initial load times:

- Tab content is loaded on demand
- Modal components are deferred
- Chart libraries are dynamically imported

### Caching

- API responses are cached where appropriate
- Theme preferences stored in localStorage
- Settings persisted locally

### Optimizations

- Memoized computations for filtered data
- Debounced search inputs
- Virtualized lists for large datasets
- WebSocket batching for high-frequency events

---

## Troubleshooting

<details>
<summary>Dashboard not loading</summary>


**Check 1**: Verify Nekzus is running:
```bash
curl http://localhost:8080/healthz
```

**Check 2**: Check browser console for JavaScript errors

**Check 3**: Clear browser cache and localStorage

</details>


<details>
<summary>WebSocket disconnecting frequently</summary>


**Check 1**: Ensure no proxy is interfering with WebSocket connections

**Check 2**: Check for network firewall rules blocking WebSocket

**Check 3**: Verify browser supports WebSocket protocol

</details>


<details>
<summary>Theme not applying correctly</summary>


**Check 1**: Clear localStorage:
```javascript
localStorage.removeItem('nxus-theme');
```

**Check 2**: Refresh the page

**Check 3**: Check for CSS conflicts with browser extensions

</details>


<details>
<summary>Real-time updates not working</summary>


**Check 1**: Look for "WEBSOCKET: DISCONNECTED" in System Health

**Check 2**: Check browser console for connection errors

**Check 3**: Verify WebSocket endpoint is accessible

</details>


---

## Related Documentation

<div className="card-grid">
  <div className="card">
    <h3>📡 API Reference</h3>
    <p>Complete API documentation for programmatic access.</p>
    <a href="../reference/api">→ API Reference</a>
  </div>
  <div className="card">
    <h3>🔍 Discovery Features</h3>
    <p>Learn about service discovery configuration.</p>
    <a href="../features/discovery">→ Discovery</a>
  </div>
  <div className="card">
    <h3>🧰 Toolbox</h3>
    <p>One-click service deployment catalog.</p>
    <a href="../features/toolbox">→ Toolbox</a>
  </div>
  <div className="card">
    <h3>⚙️ Configuration</h3>
    <p>Full configuration reference.</p>
    <a href="../reference/configuration">→ Configuration</a>
  </div>
</div>
