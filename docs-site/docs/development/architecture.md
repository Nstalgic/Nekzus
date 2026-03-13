# Architecture Overview

This document provides a comprehensive overview of the Nekzus architecture for developers. It covers system components, data flow, extension points, and key design decisions.

---

## System Overview

Nekzus is a secure API gateway and reverse proxy designed for local network services. It provides auto-discovery, JWT authentication, WebSocket support, and a React-based management dashboard.

### High-Level Architecture

```d2
direction: down

clients: Clients {
  mobile: Mobile Apps\n(iOS/Android)
  browser: Web Browser
}

discovery: Discovery Sources {
  docker: Docker/K8s Containers\nmDNS Services
}

nekzus: NEKZUS {
  style.fill: "#7c3aed"
  style.font-color: "#ffffff"
}

core: Core Components {
  proxy: Proxy\n(HTTP/WS)
  storage: Storage\n(SQLite)
  dashboard: React Dashboard
}

upstream: Your Services\n(Grafana, Home Assistant, Jellyfin, etc.)

clients.mobile -> nekzus: HTTPS + JWT
clients.browser -> nekzus: HTTPS
discovery.docker -> nekzus: Discovery Labels
nekzus ->core.proxy
nekzus ->core.storage
nekzus ->core.dashboard
core.proxy -> upstream
```

### Component Interaction Diagram

```d2
direction: down

app: Application Layer {
  grid-rows: 1
  style.fill: "#e0e7ff"
  main: cmd/nekzus/\nmain.go
  handlers: HTTP Handlers\n(Auth, Routes)
  websocket: WebSocket Mgr\n(Real-time)
}

service: Service Layer {
  grid-rows: 1
  style.fill: "#ddd6fe"
  auth: Auth Manager\n(JWT/Scopes)
  discovery: Discovery Mgr\n(Docker/K8s/mDNS)
  toolbox: Toolbox Mgr\n(Compose Deploy)
}

infra: Infrastructure Layer {
  grid-rows: 1
  style.fill: "#c4b5fd"
  router: Route Registry\n(Radix Tree)
  proxy: Proxy/Cache\n(HTTP/WS)
  config: Config Mgr\n(Hot Reload)
}

persist: Persistence Layer {
  grid-rows: 1
  style.fill: "#a78bfa"
  sqlite: SQLite Storage\n(Apps, Routes, Devices, Proposals, Certificates, Deployments) {
    style.fill: "#7c3aed"
    style.font-color: "#ffffff"
  }
}

app -> service
service -> infra
infra -> persist
```

---

## Core Components

### cmd/nekzus/ - Main Application

The main application package contains the entry point, HTTP handlers, and route configuration.

#### Key Files

| File | Purpose |
|------|---------|
| `main.go` | Application initialization, server lifecycle, signal handling |
| `handlers.go` | Core HTTP handlers (health checks, utility functions) |
| `auth_handlers.go` | Authentication endpoints (login, logout, pairing) |
| `device_handlers.go` | Device management (list, revoke, details) |
| `qr_handlers.go` | QR code pairing flow |
| `apikey_handlers.go` | API key management |
| `stats_handlers.go` | Dashboard statistics |
| `webhooks.go` | Webhook registration and delivery |
| `federation_handlers.go` | P2P federation endpoints |
| `static.go` | Static file serving for React frontend |

#### Application Structure

```go
// Application holds the main application state
type Application struct {
    // Configuration
    config        types.ServerConfig
    configPath    string
    configWatcher *config.Watcher

    // Registries
    services *ServiceRegistry    // Auth, Discovery, Toolbox, Certs
    limiters *RateLimiterRegistry // Per-endpoint rate limiters
    managers *ManagerRegistry    // WebSocket, Router, Activity, Peers
    handlers *HandlerRegistry    // HTTP handlers
    jobs     *JobRegistry        // Background jobs

    // Core Infrastructure
    storage      *storage.Store
    metrics      *metrics.Metrics
    proxyCache   *proxy.Cache
    dockerClient *client.Client
    httpServer   *http.Server
}
```

#### Registry Pattern

The application uses registries to organize related components:

```d2
grid-columns: 3

services: ServiceRegistry {
  grid-columns: 1
  style.fill: "#e0e7ff"
  Auth
  Discovery
  Toolbox
  Scripts
  Certs
  SessionCookies
}

managers: ManagerRegistry {
  grid-columns: 1
  style.fill: "#ddd6fe"
  WebSocket
  Router
  Activity
  Backup
  Peers
}

handlers: HandlerRegistry {
  grid-columns: 1
  style.fill: "#c4b5fd"
  Auth
  Container
  Toolbox
  System
  Stats
  ServiceHealth
  Backup
}
```

---

### internal/auth/ - Authentication

The auth package handles JWT token management, bootstrap tokens, and device authentication.

#### Architecture

```d2
direction: right

auth: Auth Manager {
  style.fill: "#7c3aed"
  style.font-color: "#ffffff"
}

components: {
  grid-columns: 1
  jwt: JWT Sign/Parse
  bootstrap: Bootstrap Token Store
  revocation: Revocation List
}

auth -> components
```

#### Key Components

**Manager** (`jwt.go`): Core authentication manager

```go
type Manager struct {
    jwtSecret  []byte           // HS256 signing key
    issuer     string           // Token issuer (default: nekzus)
    audience   string           // Token audience (default: nekzus-mobile)
    bootstrap  *BootstrapStore  // Short-lived pairing tokens
    revocation *RevocationList  // Revoked tokens/devices
}
```

**Scopes** (`scopes.go`): Permission system

| Scope | Description |
|-------|-------------|
| `read:catalog` | View app catalog |
| `read:events` | View activity events |
| `access:mobile` | Mobile app access |
| `access:*` | Wildcard access to proxied services |
| `read:*` | Read all resources |
| `write:*` | Write all resources |

**Bootstrap Tokens**: Used for QR code pairing flow

- 5-minute expiry by default
- Single-use tokens
- Rate-limited to prevent brute force

---

### internal/config/ - Configuration

The config package handles YAML configuration loading, validation, and hot reload.

#### Configuration Flow

```d2
direction: right

yaml: config.yaml {
  shape: document
}
load: config.Load()
defaults: SetDefaults()
env: ApplyEnvOverrides()
config: types.ServerConfig {
  style.fill: "#7c3aed"
  style.font-color: "#ffffff"
}

yaml -> load -> defaults -> env -> config
```

#### Hot Reload

The config watcher monitors the configuration file for changes:

```go
type Watcher struct {
    configPath    string
    currentConfig types.ServerConfig
    fsWatcher     *fsnotify.Watcher  // File system watcher
    handlers      []ReloadHandler     // Reload callbacks
}
```

**Reloadable Settings:**

- Routes and apps
- Bootstrap tokens
- Discovery intervals
- Health check settings
- Metrics endpoint toggle

**Non-Reloadable Settings** (require restart):

- Server address (`server.addr`)
- TLS certificates
- JWT secret
- Database path

---

### internal/discovery/ - Service Discovery

The discovery package auto-discovers services from Docker, Kubernetes, and mDNS.

#### Discovery Architecture

```d2
direction: down

manager: Discovery Manager {
  style.fill: "#7c3aed"
  style.font-color: "#ffffff"
}

workers: {
  grid-columns: 3
  docker: Docker Worker
  k8s: K8s Worker
  mdns: mDNS Worker
}

submit: SubmitProposal()
store: ProposalStore (App)
events: EventBus (WebSocket)

manager -> workers: RegisterWorker()
workers -> submit
submit -> store -> events
```

#### Discovery Worker Interface

```go
type DiscoveryWorker interface {
    Name() string
    Start(ctx context.Context) error
    Stop() error
}
```

#### Docker Discovery

Discovers containers with `nekzus.enable=true` label:

```yaml
labels:
  nekzus.enable: "true"
  nekzus.app.id: "myapp"
  nekzus.app.name: "My Application"
  nekzus.route.path: "/apps/myapp/"
```

#### Kubernetes Discovery

Discovers pods with annotations:

```yaml
annotations:
  nekzus/enable: "true"
  nekzus/app-id: "myapp"
```

#### mDNS Discovery

Scans for services advertising via mDNS/Bonjour (e.g., `_http._tcp`).

---

### internal/proxy/ - Reverse Proxy

The proxy package handles HTTP and WebSocket proxying with caching.

#### Proxy Architecture

```d2
direction: right

request: Incoming Request
router: Route Registry\n(Radix Tree)
cache: Proxy Cache\n(httputil.Proxy)

proxies: {
  grid-columns: 1
  http: HTTP Proxy
  ws: WebSocket Proxy
}

upstream: Upstream Service {
  style.fill: "#7c3aed"
  style.font-color: "#ffffff"
}

request -> router -> cache -> proxies -> upstream
```

#### Proxy Cache

Caches `httputil.ReverseProxy` instances per route:

```go
type Cache struct {
    proxies sync.Map  // map[string]*httputil.ReverseProxy
}
```

#### WebSocket Proxy

RFC 6455 compliant WebSocket proxying:

```go
type WebSocketProxy struct {
    Target          string
    BufferSize      int           // Default: 32KB
    DialTimeout     time.Duration // Default: 10s
    InsecureSkipVerify bool
}
```

Features:

- Bidirectional tunneling via connection hijacking
- TLS support (ws:// and wss://)
- Header forwarding (Origin, Cookie, X-Forwarded-*)
- Buffer pooling to reduce GC pressure

---

### internal/storage/ - SQLite Persistence

The storage package provides SQLite-based persistence with WAL mode for better concurrency.

#### Database Schema

```
+------------------+       +------------------+
|      apps        |       |     routes       |
+------------------+       +------------------+
| id (PK)          |<------| route_id (PK)    |
| name             |       | app_id (FK)      |
| icon             |       | path_base        |
| tags (JSON)      |       | target_url       |
| endpoints (JSON) |       | scopes (JSON)    |
+------------------+       | websocket        |
                           | strip_prefix     |
                           +------------------+

+------------------+       +------------------+
|    devices       |       |   proposals      |
+------------------+       +------------------+
| device_id (PK)   |       | id (PK)          |
| device_name      |       | source           |
| platform         |       | detected_host    |
| scopes (JSON)    |       | detected_port    |
| last_seen        |       | suggested_app    |
+------------------+       | suggested_route  |
                           +------------------+

+------------------+       +------------------+
|  certificates    |       | toolbox_deploy   |
+------------------+       +------------------+
| id (PK)          |       | id (PK)          |
| domain           |       | service_id       |
| certificate_pem  |       | container_id     |
| private_key_pem  |       | status           |
| not_after        |       | env_vars (JSON)  |
+------------------+       +------------------+
```

#### Repository Interfaces

```go
type DeviceRepository interface {
    SaveDevice(deviceID, deviceName, platform, platformVersion string, scopes []string) error
    GetDevice(deviceID string) (*DeviceInfo, error)
    ListDevices() ([]DeviceInfo, error)
    DeleteDevice(deviceID string) error
    UpdateDeviceLastSeen(deviceID string) error
}

type RouteRepository interface {
    SaveRoute(route types.Route) error
    GetRoute(routeID string) (*types.Route, error)
    ListRoutes() ([]types.Route, error)
    DeleteRoute(routeID string) error
}
```

#### SQLite Configuration

```go
// WAL mode for better concurrency
db.Exec("PRAGMA journal_mode=WAL")
db.Exec("PRAGMA foreign_keys=ON")
db.Exec("PRAGMA busy_timeout=5000")

// Connection pool
db.SetMaxOpenConns(10)
db.SetMaxIdleConns(5)
```

---

### internal/toolbox/ - Docker Compose Deployment

The toolbox package manages one-click service deployments using Docker Compose.

#### Toolbox Architecture

```d2
direction: right

manager: Toolbox Manager {
  style.fill: "#7c3aed"
  style.font-color: "#ffffff"
}

sources: {
  grid-columns: 1
  compose: Compose Files
  templates: Service Templates
}

deployer: Deployer
docker: Docker API

manager -> sources -> deployer -> docker
```

#### Service Template Structure

Service templates are loaded from Docker Compose files with special labels:

```yaml
services:
  myservice:
    image: vendor/myservice:latest
    labels:
      nekzus.toolbox.name: "My Service"
      nekzus.toolbox.category: "productivity"
      nekzus.toolbox.description: "Service description"
      nekzus.toolbox.icon: "Package"
```

#### Environment Variable Extraction

Variables are automatically extracted from Compose files:

```go
// Pattern: ${VAR:-default} or ${VAR:?error}
varPattern := regexp.MustCompile(`\$\{([A-Z_][A-Z0-9_]*)(:-([^}]*))?\}`)
```

---

### internal/middleware/ - HTTP Middleware

The middleware package provides HTTP middleware for authentication, rate limiting, and more.

#### Middleware Chain

```d2
direction: right

request: Request
reqid: RequestID
recovery: Recovery
cors: CORS
rate: RateLimit
body: BodyLimit
ip: IPAuth
jwt: JWT Auth
apikey: API Key Auth
handler: Handler {
  style.fill: "#7c3aed"
  style.font-color: "#ffffff"
}

request -> reqid -> recovery -> cors -> rate -> body -> ip -> jwt -> apikey -> handler
```

#### Rate Limiter

Uses token bucket algorithm with RFC 6585 headers:

```go
func RateLimit(limiter *ratelimit.Limiter) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            clientIP := httputil.ExtractClientIP(r)
            state := limiter.GetState(clientIP)

            // RFC 6585 headers
            w.Header().Set("RateLimit-Limit", strconv.Itoa(state.Limit))
            w.Header().Set("RateLimit-Remaining", strconv.Itoa(state.Remaining))
            w.Header().Set("RateLimit-Reset", strconv.FormatInt(state.ResetAt, 10))

            if !limiter.Allow(clientIP) {
                http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
                return
            }
            next.ServeHTTP(w, r)
        })
    }
}
```

---

### internal/types/ - Shared Types

The types package contains shared type definitions used across packages.

#### Key Types

```go
// ServerConfig holds the complete application configuration
type ServerConfig struct {
    Server       ServerSettings
    Auth         AuthSettings
    Bootstrap    BootstrapSettings
    Storage      StorageSettings
    Discovery    DiscoverySettings
    HealthChecks HealthChecksConfig
    Metrics      MetricsConfig
    Toolbox      ToolboxConfig
    Routes       []Route
    Apps         []App
}

// App represents a discoverable application
type App struct {
    ID           string
    Name         string
    Icon         string
    Tags         []string
    Endpoints    map[string]string
    HealthStatus string
}

// Route defines a reverse proxy route
type Route struct {
    RouteID     string
    AppID       string
    PathBase    string
    To          string
    Scopes      []string
    Websocket   bool
    StripPrefix bool
}

// Proposal represents a discovered service awaiting approval
type Proposal struct {
    ID             string
    Source         string  // docker, kubernetes, mdns
    DetectedScheme string
    DetectedHost   string
    DetectedPort   int
    Confidence     float64
    SuggestedApp   App
    SuggestedRoute Route
}
```

---

### web/ - React Frontend

The React frontend provides a terminal-themed dashboard for managing Nekzus.

#### Frontend Architecture

```d2
direction: right

app: App.jsx {
  style.fill: "#7c3aed"
  style.font-color: "#ffffff"
}

middle: {
  grid-columns: 1
  context: Context Providers
  pages: Pages
}

components: Components

app -> middle -> components
```

#### Context Providers

```d2
direction: right

providers: {
  grid-columns: 1
  theme: ThemeProvider
  settings: SettingsProvider
  auth: AuthProvider
}

middle: {
  grid-columns: 1
  notify: NotificationProvider
  data: DataProvider
}

application: Application {
  style.fill: "#7c3aed"
  style.font-color: "#ffffff"
}

providers -> middle -> application
```

**ThemeProvider** (`ThemeContext.jsx`): Manages 8 visual themes

**SettingsProvider**: Application settings persistence

**AuthProvider** (`AuthContext.jsx`): JWT authentication state

```javascript
const AuthContext = {
    user: Object,           // Current user
    token: String,          // JWT token
    isAuthenticated: Bool,  // Auth status
    isLoading: Bool,        // Loading state
    login: Function,        // Login handler
    logout: Function,       // Logout handler
    checkAuth: Function,    // Validate token
}
```

**DataProvider** (`DataContext.jsx`): Central data management

```javascript
const DataContext = {
    // State
    routes: Array,
    discoveries: Array,
    devices: Array,
    activities: Array,
    containers: Array,
    stats: Object,
    wsConnected: Boolean,

    // CRUD Operations
    updateRoute: Function,
    deleteRoute: Function,
    approveDiscovery: Function,
    rejectDiscovery: Function,
    revokeDevice: Function,
}
```

#### Component Structure

```
web/src/
+-- components/
|   +-- buttons/       # Button, ButtonGroup
|   +-- cards/         # DeviceCard, ServiceCard, ContainerCard
|   +-- charts/        # ResourceLineChart, ServerResourcesPanel
|   +-- data-display/  # Badge, HealthItem, ActivityList
|   +-- forms/         # Input, Select, Checkbox, ToggleSwitch
|   +-- layout/        # Container, TerminalHeader, TerminalFooter
|   +-- modals/        # Modal, PairingModal, ConfirmationModal
|   +-- navigation/    # Tabs, TabItem, TabContent
|   +-- notifications/ # ToastContainer, NotificationBell
|   +-- utility/       # ThemeSwitcher, ASCIILogo
+-- contexts/          # React contexts
+-- pages/             # Page components
+-- services/          # API and WebSocket services
+-- styles/            # CSS (base, themes, app)
```

---

## Request Flow

### HTTP Request Flow

```d2
direction: right

client: Client Request
tls: TLS Termination
middleware: Middleware Chain\n(RequestID, Recovery,\nRateLimit, Auth)
router: Route Matching
handler: Handler / Proxy
response: Response {
  style.fill: "#7c3aed"
  style.font-color: "#ffffff"
}

client -> tls -> middleware -> router -> handler -> response
```

### Proxy Request Flow

```
1. Request: GET /apps/grafana/api/dashboards

2. Route Lookup (Radix Tree)
   - Match: /apps/grafana/ -> http://grafana:3000

3. Path Processing
   - Original: /apps/grafana/api/dashboards
   - Strip prefix: /api/dashboards
   - Upstream: http://grafana:3000/api/dashboards

4. Header Processing
   - Add X-Forwarded-For, X-Real-IP
   - Remove hop-by-hop headers
   - Strip Authorization header

5. Proxy Request
   - Forward to upstream
   - Stream response

6. Response Processing
   - Rewrite HTML (if enabled)
   - Rewrite cookie paths
```

---

## Discovery Architecture

### Discovery Flow

```d2
direction: right

poll: 1. Workers {
  grid-columns: 1
  docker: Docker
  k8s: K8s
  mdns: mDNS
}

manager: 2. Manager\nDedup + Process

storage: 3. Storage {
  grid-columns: 1
  sqlite: SQLite
  ws: WebSocket
}

dashboard: 4. Dashboard
router: 5. Router {
  style.fill: "#7c3aed"
  style.font-color: "#ffffff"
}

poll -> manager -> storage -> dashboard -> router
```

### Proposal Lifecycle

```d2
direction: right

start: {
  shape: circle
  style.fill: "#000"
}
discovered: Discovered
pending: Pending {
  style.fill: "#fef3c7"
}
approved: Approved {
  style.fill: "#d1fae5"
}
route: RouteCreated {
  style.fill: "#7c3aed"
  style.font-color: "#ffffff"
}
dismissed: Dismissed {
  style.fill: "#fee2e2"
}
ignored: Ignored {
  style.fill: "#e5e7eb"
}

start -> discovered -> pending
pending -> approved -> route: Route Created
pending -> dismissed -> ignored
```

---

## Authentication Flow

### QR Code Pairing Flow (2-Step)

```d2
direction: right

step1: Step 1 {
  grid-columns: 1
  qr: GET /api/v1/auth/qr
  payload: QR contains:\nURL + Code only
  scan: Mobile scans QR
}

step2: Step 2 {
  grid-columns: 1
  redeem: POST /api/v1/pair\n{code}
  config: Returns config:\nSPKI pins, bootstrap token
  validate: Validate cert pins
  pair: POST /pair\n{token, device}
}

jwt: JWT Token {
  style.fill: "#7c3aed"
  style.font-color: "#ffffff"
}

step1 -> step2 -> jwt
```

### JWT Token Structure

```json
{
  "iss": "nekzus",
  "aud": "nekzus-mobile",
  "sub": "device_abc123",
  "scopes": ["read:catalog", "read:events", "access:*"],
  "iat": 1703520000,
  "exp": 1706112000
}
```

### Token Validation Flow

```
1. Extract token from Authorization header
2. Verify signature (HS256)
3. Check expiration
4. Verify issuer and audience
5. Check revocation list
6. Check device revocation
7. Extract scopes for authorization
```

---

## Data Storage

### Database Migrations

Migrations run automatically on startup:

```go
func (s *Store) migrate() error {
    migrations := []string{
        // Apps table
        `CREATE TABLE IF NOT EXISTS apps (...)`,
        // Routes table
        `CREATE TABLE IF NOT EXISTS routes (...)`,
        // Devices table
        `CREATE TABLE IF NOT EXISTS devices (...)`,
        // ... more tables
    }

    for _, migration := range migrations {
        if _, err := s.db.Exec(migration); err != nil {
            return err
        }
    }
    return nil
}
```

### Schema Overview

| Table | Purpose | Key Fields |
|-------|---------|------------|
| `apps` | Application catalog | id, name, icon, tags |
| `routes` | Proxy routes | route_id, app_id, path_base, to |
| `devices` | Paired devices | device_id, name, platform, scopes |
| `proposals` | Discovery proposals | id, source, suggested_app, suggested_route |
| `certificates` | TLS certificates | domain, certificate_pem, not_after |
| `api_keys` | API key storage | id, key_hash, scopes, expires_at |
| `toolbox_deployments` | Deployed services | id, service_id, container_id, status |
| `service_health` | Health check state | app_id, status, consecutive_failures |
| `activity_events` | Activity feed | event_id, type, message, timestamp |
| `audit_logs` | Security audit trail | action, actor_id, target_id, success |

---

## Frontend Architecture

### State Management

```d2
direction: down

auth: AuthContext\nUser authentication state {
  style.fill: "#7c3aed"
  style.font-color: "#ffffff"
}

data: DataContext\nApplication data (routes, devices, etc.)

services: {
  grid-columns: 2
  api: API Service
  ws: WebSocket Service
}

auth -> data -> services
```

### WebSocket Integration

```javascript
// WebSocket message types
const WS_MSG_TYPES = {
    DISCOVERY: 'discovery',
    CONFIG_RELOAD: 'config_reload',
    DEVICE_PAIRED: 'device_paired',
    DEVICE_REVOKED: 'device_revoked',
    HEALTH_CHANGE: 'health_change',
    WEBHOOK: 'webhook',
}

// Connection with auto-reconnect
websocketService.connect();
websocketService.on(WS_MSG_TYPES.DISCOVERY, () => {
    refreshDiscoveries();
});
```

### CSS Architecture

Three-layer CSS architecture:

1. **base.css**: Design tokens (colors, spacing, typography)
2. **themes.css**: Theme-specific overrides
3. **app.css**: Component styles

```css
/* Design tokens in base.css */
:root {
  --bg-primary: #0a0c10;
  --text-primary: #f8fafc;
  --accent-primary: #00ff88;
  --space-4: 16px;
}

/* Component using tokens */
.card {
  background: var(--bg-primary);
  color: var(--text-primary);
  padding: var(--space-4);
}
```

---

## Extension Points

### Adding a New Discovery Source

1. **Implement the DiscoveryWorker interface:**

```go
// internal/discovery/myprotocol.go
type MyProtocolWorker struct {
    manager     DiscoverySubmitter
    interval    time.Duration
    ctx         context.Context
    cancel      context.CancelFunc
}

func (w *MyProtocolWorker) Name() string {
    return "myprotocol"
}

func (w *MyProtocolWorker) Start(ctx context.Context) error {
    w.ctx, w.cancel = context.WithCancel(ctx)

    ticker := time.NewTicker(w.interval)
    for {
        select {
        case <-ctx.Done():
            return nil
        case <-ticker.C:
            w.scan()
        }
    }
}

func (w *MyProtocolWorker) scan() {
    // Discover services
    services := discoverServices()

    for _, svc := range services {
        proposal := &types.Proposal{
            ID:             generateProposalID("myprotocol", svc.Host, svc.Port),
            Source:         "myprotocol",
            DetectedScheme: "http",
            DetectedHost:   svc.Host,
            DetectedPort:   svc.Port,
            Confidence:     0.8,
            SuggestedApp:   buildApp(svc),
            SuggestedRoute: buildRoute(svc),
        }
        w.manager.SubmitProposal(proposal)
    }
}

func (w *MyProtocolWorker) Stop() error {
    w.cancel()
    return nil
}
```

2. **Register the worker in main.go:**

```go
func (app *Application) setupDiscovery() error {
    // ... existing workers ...

    if app.config.Discovery.MyProtocol.Enabled {
        worker := discovery.NewMyProtocolWorker(
            app.services.Discovery,
            app.config.Discovery.MyProtocol.Interval,
        )
        app.services.Discovery.RegisterWorker(worker)
    }
}
```

### Adding New Middleware

1. **Create the middleware:**

```go
// internal/middleware/mymiddleware.go
func MyMiddleware(config MyConfig) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // Pre-processing
            ctx := context.WithValue(r.Context(), myKey, myValue)
            r = r.WithContext(ctx)

            // Call next handler
            next.ServeHTTP(w, r)

            // Post-processing (if needed)
        })
    }
}
```

2. **Add to the middleware chain in the route builder.**

### Adding API Endpoints

1. **Create the handler:**

```go
// internal/handlers/myhandler.go
type MyHandler struct {
    store   *storage.Store
    metrics *metrics.Metrics
}

func NewMyHandler(store *storage.Store, metrics *metrics.Metrics) *MyHandler {
    return &MyHandler{store: store, metrics: metrics}
}

func (h *MyHandler) HandleList(w http.ResponseWriter, r *http.Request) {
    items, err := h.store.ListItems()
    if err != nil {
        apperrors.WriteJSON(w, apperrors.Wrap(err, "LIST_FAILED", "Failed to list items", 500))
        return
    }
    json.NewEncoder(w).Encode(items)
}
```

2. **Register routes in the route builder.**

### Adding Frontend Features

1. **Create the API service:**

```javascript
// web/src/services/api/myResource.js
export const myResourceAPI = {
    list: async () => {
        const response = await fetch('/api/v1/my-resource', {
            headers: getAuthHeaders(),
        });
        return response.json();
    },
    create: async (data) => {
        const response = await fetch('/api/v1/my-resource', {
            method: 'POST',
            headers: getAuthHeaders(),
            body: JSON.stringify(data),
        });
        return response.json();
    },
};
```

2. **Add to DataContext:**

```javascript
// web/src/contexts/DataContext.jsx
export function DataProvider({ children }) {
    const [myResources, setMyResources] = useState([]);

    const refreshMyResources = useCallback(async () => {
        const data = await myResourceAPI.list();
        setMyResources(data);
    }, []);

    // Add to WebSocket listeners
    websocketService.on('my_resource_updated', refreshMyResources);

    const value = {
        myResources,
        refreshMyResources,
        // ... other values
    };
}
```

3. **Create the component:**

```javascript
// web/src/components/MyResourceList.jsx
import { useData } from '../contexts';

export function MyResourceList() {
    const { myResources, refreshMyResources } = useData();

    return (
        <div className="my-resource-list">
            {myResources.map(item => (
                <div key={item.id}>{item.name}</div>
            ))}
        </div>
    );
}
```

---

## Best Practices

### Error Handling

Use the structured errors package:

```go
import apperrors "github.com/nstalgic/nekzus/internal/errors"

// Create structured error
return apperrors.New("ERROR_CODE", "User message", http.StatusBadRequest)

// Wrap existing error
return apperrors.Wrap(err, "ERROR_CODE", "User message", http.StatusInternalServerError)

// Write JSON error response
apperrors.WriteJSON(w, err)
```

### Metrics Recording

Record operations for observability:

```go
// HTTP requests
app.metrics.RecordHTTPRequest(method, path, status, duration, reqSize, respSize)

// Authentication events
app.metrics.RecordAuthPairing("success", platform, duration)

// Proxy requests
app.metrics.RecordProxyRequest(appID, status, duration)
```

### Storage Operations

Handle optional storage gracefully:

```go
// Always check if storage is available
if app.storage != nil {
    device, err := app.storage.GetDevice(deviceID)
    if err != nil {
        // Handle error
    }
}

// Async updates for non-critical operations
go func() {
    if err := app.storage.UpdateDeviceLastSeen(deviceID); err != nil {
        log.Printf("Warning: %v", err)
    }
}()
```

### Testing

Follow TDD practices:

```bash
# Run all tests with race detector
go test -race ./...

# Run short tests only
go test -race ./... -short

# Run specific package
go test -race ./internal/proxy/... -v
```

---

## Related Documentation

- [Testing Guide](testing) - Testing practices and conventions
- [Contributing Guide](contributing) - How to contribute to the project
- [API Reference](../reference/api) - API endpoint documentation
