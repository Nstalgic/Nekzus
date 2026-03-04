/**
 * Mock Data for Nekzus Terminal Dashboard
 *
 * Provides realistic test data for routes, activities, and health metrics
 */

/**
 * Routes Data (42 routes)
 * Variety of applications, paths, targets, scopes, and statuses
 */
export const routes = [
  // Core API Routes
  {
    id: 'route-001',
    application: 'Grafana',
    path: '/grafana',
    target: 'http://grafana:3000',
    scopes: ['READ'],
    status: 'ACTIVE',
    requiresAuth: true,
    healthCheck: 'http://grafana:3000/api/health',
    createdAt: '2025-01-05T10:30:00Z',
    updatedAt: '2025-01-05T10:30:00Z'
  },
  {
    id: 'route-002',
    application: 'Prometheus',
    path: '/prometheus',
    target: 'http://prometheus:9090',
    scopes: ['READ', 'ADMIN'],
    status: 'ACTIVE',
    requiresAuth: true,
    healthCheck: 'http://prometheus:9090/-/healthy',
    createdAt: '2025-01-04T14:20:00Z',
    updatedAt: '2025-01-06T09:15:00Z'
  },
  {
    id: 'route-003',
    application: 'Portainer',
    path: '/portainer',
    target: 'http://portainer:9000',
    scopes: ['READ', 'WRITE', 'ADMIN'],
    status: 'ACTIVE',
    requiresAuth: true,
    healthCheck: null,
    createdAt: '2025-01-03T08:45:00Z',
    updatedAt: '2025-01-03T08:45:00Z'
  },
  {
    id: 'route-004',
    application: 'Kubernetes Dashboard',
    path: '/k8s-dashboard',
    target: 'https://kubernetes-dashboard.kube-system.svc',
    scopes: ['READ', 'WRITE', 'ADMIN'],
    status: 'ACTIVE',
    requiresAuth: true,
    healthCheck: null,
    createdAt: '2025-01-02T11:00:00Z',
    updatedAt: '2025-01-02T11:00:00Z'
  },
  {
    id: 'route-005',
    application: 'User Service',
    path: '/api/v1/users',
    target: 'http://user-service:3001',
    scopes: ['READ', 'WRITE'],
    status: 'ACTIVE',
    requiresAuth: true,
    healthCheck: 'http://user-service:3001/health',
    createdAt: '2025-01-01T09:00:00Z',
    updatedAt: '2025-01-07T15:30:00Z'
  },
  {
    id: 'route-006',
    application: 'Auth Service',
    path: '/api/v1/auth',
    target: 'http://auth-service:3002',
    scopes: ['AUTH'],
    status: 'ACTIVE',
    requiresAuth: false,
    healthCheck: 'http://auth-service:3002/healthz',
    createdAt: '2025-01-01T09:05:00Z',
    updatedAt: '2025-01-01T09:05:00Z'
  },
  {
    id: 'route-007',
    application: 'Payment Service',
    path: '/api/v1/payments',
    target: 'http://payment-service:3003',
    scopes: ['PAYMENT', 'WRITE'],
    status: 'ACTIVE',
    requiresAuth: true,
    healthCheck: null,
    createdAt: '2025-01-01T09:10:00Z',
    updatedAt: '2025-01-05T12:00:00Z'
  },
  {
    id: 'route-008',
    application: 'Analytics Service',
    path: '/api/v1/analytics',
    target: 'http://analytics-service:3004',
    scopes: ['READ', 'ANALYTICS'],
    status: 'PENDING',
    requiresAuth: true,
    healthCheck: 'http://analytics-service:3004/status',
    createdAt: '2025-01-07T14:20:00Z',
    updatedAt: '2025-01-07T14:20:00Z'
  },
  {
    id: 'route-009',
    application: 'Notification Service',
    path: '/api/v1/notifications',
    target: 'http://notification-service:3005',
    scopes: ['NOTIFY'],
    status: 'INACTIVE',
    requiresAuth: true,
    healthCheck: null,
    createdAt: '2025-01-01T09:15:00Z',
    updatedAt: '2025-01-06T18:00:00Z'
  },
  {
    id: 'route-010',
    application: 'File Storage',
    path: '/api/v1/files',
    target: 'http://minio:9000',
    scopes: ['READ', 'WRITE'],
    status: 'ACTIVE',
    requiresAuth: true,
    healthCheck: 'http://minio:9000/minio/health/live',
    createdAt: '2025-01-02T10:30:00Z',
    updatedAt: '2025-01-02T10:30:00Z'
  },
  {
    id: 'route-011',
    application: 'Redis Cache',
    path: '/api/v1/cache',
    target: 'http://redis:6379',
    scopes: ['READ', 'WRITE', 'ADMIN'],
    status: 'ACTIVE',
    requiresAuth: true,
    healthCheck: null,
    createdAt: '2025-01-01T10:00:00Z',
    updatedAt: '2025-01-01T10:00:00Z'
  },
  {
    id: 'route-012',
    application: 'RabbitMQ',
    path: '/rabbitmq',
    target: 'http://rabbitmq:15672',
    scopes: ['READ', 'WRITE', 'ADMIN'],
    status: 'ACTIVE',
    requiresAuth: true,
    healthCheck: 'http://rabbitmq:15672/api/health/checks/alarms',
    createdAt: '2025-01-03T11:15:00Z',
    updatedAt: '2025-01-03T11:15:00Z'
  },
  {
    id: 'route-013',
    application: 'Elasticsearch',
    path: '/elasticsearch',
    target: 'http://elasticsearch:9200',
    scopes: ['READ', 'WRITE'],
    status: 'ACTIVE',
    requiresAuth: true,
    healthCheck: 'http://elasticsearch:9200/_cluster/health',
    createdAt: '2025-01-04T09:30:00Z',
    updatedAt: '2025-01-04T09:30:00Z'
  },
  {
    id: 'route-014',
    application: 'Kibana',
    path: '/kibana',
    target: 'http://kibana:5601',
    scopes: ['READ'],
    status: 'ACTIVE',
    requiresAuth: true,
    healthCheck: 'http://kibana:5601/api/status',
    createdAt: '2025-01-04T09:35:00Z',
    updatedAt: '2025-01-04T09:35:00Z'
  },
  {
    id: 'route-015',
    application: 'Jaeger',
    path: '/jaeger',
    target: 'http://jaeger:16686',
    scopes: ['READ'],
    status: 'ACTIVE',
    requiresAuth: true,
    healthCheck: null,
    createdAt: '2025-01-05T08:00:00Z',
    updatedAt: '2025-01-05T08:00:00Z'
  },
  {
    id: 'route-016',
    application: 'Consul',
    path: '/consul',
    target: 'http://consul:8500',
    scopes: ['READ', 'WRITE', 'ADMIN'],
    status: 'ACTIVE',
    requiresAuth: true,
    healthCheck: 'http://consul:8500/v1/health/service/consul',
    createdAt: '2025-01-06T10:20:00Z',
    updatedAt: '2025-01-06T10:20:00Z'
  },
  {
    id: 'route-017',
    application: 'Vault',
    path: '/vault',
    target: 'https://vault:8200',
    scopes: ['READ', 'WRITE', 'ADMIN'],
    status: 'ACTIVE',
    requiresAuth: true,
    healthCheck: 'https://vault:8200/v1/sys/health',
    createdAt: '2025-01-06T10:25:00Z',
    updatedAt: '2025-01-06T10:25:00Z'
  },
  {
    id: 'route-018',
    application: 'Jenkins',
    path: '/jenkins',
    target: 'http://jenkins:8080',
    scopes: ['READ', 'WRITE', 'ADMIN'],
    status: 'ACTIVE',
    requiresAuth: true,
    healthCheck: null,
    createdAt: '2025-01-02T14:00:00Z',
    updatedAt: '2025-01-02T14:00:00Z'
  },
  {
    id: 'route-019',
    application: 'GitLab',
    path: '/gitlab',
    target: 'http://gitlab:80',
    scopes: ['READ', 'WRITE'],
    status: 'ACTIVE',
    requiresAuth: true,
    healthCheck: 'http://gitlab:80/-/health',
    createdAt: '2025-01-03T15:30:00Z',
    updatedAt: '2025-01-03T15:30:00Z'
  },
  {
    id: 'route-020',
    application: 'SonarQube',
    path: '/sonarqube',
    target: 'http://sonarqube:9000',
    scopes: ['READ'],
    status: 'ACTIVE',
    requiresAuth: true,
    healthCheck: 'http://sonarqube:9000/api/system/status',
    createdAt: '2025-01-04T16:00:00Z',
    updatedAt: '2025-01-04T16:00:00Z'
  },
  {
    id: 'route-021',
    application: 'Nexus Repository',
    path: '/nexus',
    target: 'http://nexus:8081',
    scopes: ['READ', 'WRITE'],
    status: 'ACTIVE',
    requiresAuth: true,
    healthCheck: null,
    createdAt: '2025-01-05T09:00:00Z',
    updatedAt: '2025-01-05T09:00:00Z'
  },
  {
    id: 'route-022',
    application: 'Harbor Registry',
    path: '/harbor',
    target: 'https://harbor:443',
    scopes: ['READ', 'WRITE', 'ADMIN'],
    status: 'ACTIVE',
    requiresAuth: true,
    healthCheck: 'https://harbor:443/api/v2.0/health',
    createdAt: '2025-01-06T11:00:00Z',
    updatedAt: '2025-01-06T11:00:00Z'
  },
  {
    id: 'route-023',
    application: 'ArgoCD',
    path: '/argocd',
    target: 'http://argocd-server:80',
    scopes: ['READ', 'WRITE', 'ADMIN'],
    status: 'ACTIVE',
    requiresAuth: true,
    healthCheck: null,
    createdAt: '2025-01-07T08:30:00Z',
    updatedAt: '2025-01-07T08:30:00Z'
  },
  {
    id: 'route-024',
    application: 'Backstage',
    path: '/backstage',
    target: 'http://backstage:7007',
    scopes: ['READ'],
    status: 'ACTIVE',
    requiresAuth: true,
    healthCheck: null,
    createdAt: '2025-01-07T09:00:00Z',
    updatedAt: '2025-01-07T09:00:00Z'
  },
  {
    id: 'route-025',
    application: 'WordPress',
    path: '/blog',
    target: 'http://wordpress:80',
    scopes: ['PUBLIC'],
    status: 'ACTIVE',
    requiresAuth: false,
    healthCheck: null,
    createdAt: '2025-01-01T12:00:00Z',
    updatedAt: '2025-01-01T12:00:00Z'
  },
  {
    id: 'route-026',
    application: 'Ghost Blog',
    path: '/ghost',
    target: 'http://ghost:2368',
    scopes: ['READ', 'WRITE'],
    status: 'ACTIVE',
    requiresAuth: true,
    healthCheck: null,
    createdAt: '2025-01-02T13:00:00Z',
    updatedAt: '2025-01-02T13:00:00Z'
  },
  {
    id: 'route-027',
    application: 'Matomo Analytics',
    path: '/matomo',
    target: 'http://matomo:80',
    scopes: ['READ', 'ANALYTICS'],
    status: 'ACTIVE',
    requiresAuth: true,
    healthCheck: null,
    createdAt: '2025-01-03T14:00:00Z',
    updatedAt: '2025-01-03T14:00:00Z'
  },
  {
    id: 'route-028',
    application: 'Netdata',
    path: '/netdata',
    target: 'http://netdata:19999',
    scopes: ['READ'],
    status: 'ACTIVE',
    requiresAuth: true,
    healthCheck: null,
    createdAt: '2025-01-04T15:00:00Z',
    updatedAt: '2025-01-04T15:00:00Z'
  },
  {
    id: 'route-029',
    application: 'Uptime Kuma',
    path: '/uptime',
    target: 'http://uptime-kuma:3001',
    scopes: ['READ'],
    status: 'ACTIVE',
    requiresAuth: true,
    healthCheck: null,
    createdAt: '2025-01-05T16:00:00Z',
    updatedAt: '2025-01-05T16:00:00Z'
  },
  {
    id: 'route-030',
    application: 'Pi-hole',
    path: '/pihole',
    target: 'http://pihole:80',
    scopes: ['READ', 'WRITE', 'ADMIN'],
    status: 'ACTIVE',
    requiresAuth: true,
    healthCheck: null,
    createdAt: '2025-01-01T13:00:00Z',
    updatedAt: '2025-01-01T13:00:00Z'
  },
  {
    id: 'route-031',
    application: 'Traefik',
    path: '/traefik',
    target: 'http://traefik:8080',
    scopes: ['READ', 'ADMIN'],
    status: 'ACTIVE',
    requiresAuth: true,
    healthCheck: 'http://traefik:8080/ping',
    createdAt: '2025-01-02T14:30:00Z',
    updatedAt: '2025-01-02T14:30:00Z'
  },
  {
    id: 'route-032',
    application: 'Nginx Proxy Manager',
    path: '/nginx-proxy',
    target: 'http://nginx-proxy:81',
    scopes: ['READ', 'WRITE', 'ADMIN'],
    status: 'ACTIVE',
    requiresAuth: true,
    healthCheck: null,
    createdAt: '2025-01-03T15:00:00Z',
    updatedAt: '2025-01-03T15:00:00Z'
  },
  {
    id: 'route-033',
    application: 'Minio Console',
    path: '/minio-console',
    target: 'http://minio:9001',
    scopes: ['READ', 'WRITE', 'ADMIN'],
    status: 'ACTIVE',
    requiresAuth: true,
    healthCheck: null,
    createdAt: '2025-01-04T16:30:00Z',
    updatedAt: '2025-01-04T16:30:00Z'
  },
  {
    id: 'route-034',
    application: 'PostgreSQL Admin',
    path: '/pgadmin',
    target: 'http://pgadmin:80',
    scopes: ['READ', 'WRITE', 'ADMIN'],
    status: 'ACTIVE',
    requiresAuth: true,
    healthCheck: null,
    createdAt: '2025-01-05T17:00:00Z',
    updatedAt: '2025-01-05T17:00:00Z'
  },
  {
    id: 'route-035',
    application: 'MongoDB Express',
    path: '/mongo-express',
    target: 'http://mongo-express:8081',
    scopes: ['READ', 'WRITE', 'ADMIN'],
    status: 'PENDING',
    requiresAuth: true,
    healthCheck: null,
    createdAt: '2025-01-07T10:00:00Z',
    updatedAt: '2025-01-07T10:00:00Z'
  },
  {
    id: 'route-036',
    application: 'Adminer',
    path: '/adminer',
    target: 'http://adminer:8080',
    scopes: ['READ', 'WRITE', 'ADMIN'],
    status: 'ACTIVE',
    requiresAuth: true,
    healthCheck: null,
    createdAt: '2025-01-06T12:00:00Z',
    updatedAt: '2025-01-06T12:00:00Z'
  },
  {
    id: 'route-037',
    application: 'Code Server',
    path: '/code',
    target: 'http://code-server:8443',
    scopes: ['READ', 'WRITE'],
    status: 'ACTIVE',
    requiresAuth: true,
    healthCheck: null,
    createdAt: '2025-01-02T15:30:00Z',
    updatedAt: '2025-01-02T15:30:00Z'
  },
  {
    id: 'route-038',
    application: 'Jupyter Notebook',
    path: '/jupyter',
    target: 'http://jupyter:8888',
    scopes: ['READ', 'WRITE'],
    status: 'ACTIVE',
    requiresAuth: true,
    healthCheck: null,
    createdAt: '2025-01-03T16:00:00Z',
    updatedAt: '2025-01-03T16:00:00Z'
  },
  {
    id: 'route-039',
    application: 'Heimdall Dashboard',
    path: '/heimdall',
    target: 'http://heimdall:80',
    scopes: ['READ'],
    status: 'ACTIVE',
    requiresAuth: true,
    healthCheck: null,
    createdAt: '2025-01-04T17:00:00Z',
    updatedAt: '2025-01-04T17:00:00Z'
  },
  {
    id: 'route-040',
    application: 'Home Assistant',
    path: '/homeassistant',
    target: 'http://homeassistant:8123',
    scopes: ['READ', 'WRITE'],
    status: 'INACTIVE',
    requiresAuth: true,
    healthCheck: 'http://homeassistant:8123/api/',
    createdAt: '2025-01-01T14:00:00Z',
    updatedAt: '2025-01-06T20:00:00Z'
  },
  {
    id: 'route-041',
    application: 'Plex Media Server',
    path: '/plex',
    target: 'http://plex:32400',
    scopes: ['READ'],
    status: 'ACTIVE',
    requiresAuth: true,
    healthCheck: null,
    createdAt: '2025-01-05T18:00:00Z',
    updatedAt: '2025-01-05T18:00:00Z'
  },
  {
    id: 'route-042',
    application: 'Jellyfin',
    path: '/jellyfin',
    target: 'http://jellyfin:8096',
    scopes: ['READ'],
    status: 'ACTIVE',
    requiresAuth: false,
    healthCheck: 'http://jellyfin:8096/health',
    createdAt: '2025-01-06T19:00:00Z',
    updatedAt: '2025-01-06T19:00:00Z'
  }
];

/**
 * Activities Data (Recent system activities)
 */
export const activities = [
  {
    id: 'activity-001',
    type: 'route_added',
    message: 'New route registered: /api/v1/users',
    timestamp: new Date(Date.now() - 2 * 60 * 1000).toISOString(), // 2m ago
    severity: 'info',
    metadata: { routeId: 'route-005' }
  },
  {
    id: 'activity-002',
    type: 'device_paired',
    message: 'Device authenticated: device-4523',
    timestamp: new Date(Date.now() - 5 * 60 * 1000).toISOString(), // 5m ago
    severity: 'success',
    metadata: { deviceId: 'device-4523' }
  },
  {
    id: 'activity-003',
    type: 'discovery_started',
    message: 'Discovery worker started: worker-03',
    timestamp: new Date(Date.now() - 12 * 60 * 1000).toISOString(), // 12m ago
    severity: 'info',
    metadata: { workerId: 'worker-03' }
  },
  {
    id: 'activity-004',
    type: 'certificate_renewed',
    message: 'Certificate renewed: api.example.com',
    timestamp: new Date(Date.now() - 25 * 60 * 1000).toISOString(), // 25m ago
    severity: 'success',
    metadata: { domain: 'api.example.com' }
  },
  {
    id: 'activity-005',
    type: 'route_updated',
    message: 'Route updated: /api/v1/auth',
    timestamp: new Date(Date.now() - 60 * 60 * 1000).toISOString(), // 1h ago
    severity: 'info',
    metadata: { routeId: 'route-006' }
  },
  {
    id: 'activity-006',
    type: 'cache_cleared',
    message: 'Proxy cache cleared: 2,341 entries',
    timestamp: new Date(Date.now() - 2 * 60 * 60 * 1000).toISOString(), // 2h ago
    severity: 'warning',
    metadata: { entriesCleared: 2341 }
  },
  {
    id: 'activity-007',
    type: 'route_deleted',
    message: 'Route deleted: /api/v1/legacy',
    timestamp: new Date(Date.now() - 3 * 60 * 60 * 1000).toISOString(), // 3h ago
    severity: 'warning',
    metadata: { routeId: 'route-deleted-001' }
  },
  {
    id: 'activity-008',
    type: 'discovery_approved',
    message: 'Discovery approved: Grafana Dashboard',
    timestamp: new Date(Date.now() - 4 * 60 * 60 * 1000).toISOString(), // 4h ago
    severity: 'success',
    metadata: { discoveryId: 'discovery-001' }
  },
  {
    id: 'activity-009',
    type: 'discovery_rejected',
    message: 'Discovery rejected: Unknown Service',
    timestamp: new Date(Date.now() - 5 * 60 * 60 * 1000).toISOString(), // 5h ago
    severity: 'warning',
    metadata: { discoveryId: 'discovery-002' }
  },
  {
    id: 'activity-010',
    type: 'device_revoked',
    message: 'Device access revoked: old-tablet-xyz',
    timestamp: new Date(Date.now() - 6 * 60 * 60 * 1000).toISOString(), // 6h ago
    severity: 'error',
    metadata: { deviceId: 'device-xyz' }
  },
  {
    id: 'activity-011',
    type: 'config_updated',
    message: 'Configuration reloaded successfully',
    timestamp: new Date(Date.now() - 8 * 60 * 60 * 1000).toISOString(), // 8h ago
    severity: 'info',
    metadata: {}
  },
  {
    id: 'activity-012',
    type: 'health_check_failed',
    message: 'Health check failed: Notification Service',
    timestamp: new Date(Date.now() - 10 * 60 * 60 * 1000).toISOString(), // 10h ago
    severity: 'error',
    metadata: { routeId: 'route-009' }
  }
];

/**
 * Health Metrics
 */
export const healthMetrics = {
  apiGateway: {
    status: 'ONLINE',
    uptime: '15d 6h 32m',
    lastCheck: new Date().toISOString()
  },
  database: {
    status: 'OPERATIONAL',
    uptime: '15d 6h 32m',
    lastCheck: new Date().toISOString()
  },
  discoveryWorker: {
    status: 'RUNNING',
    workers: 5,
    lastCheck: new Date().toISOString()
  },
  memory: {
    current: 6.2, // GB
    max: 16, // GB
    percentage: 38,
    status: 'OK'
  },
  connections: {
    active: 142,
    total: 200,
    status: 'OK'
  },
  requestRate: {
    current: 847, // requests per minute
    average: 650,
    status: 'OK'
  },
  errorRate: {
    current: 0.5, // percentage
    threshold: 5,
    status: 'OK'
  },
  certificates: {
    total: 12,
    expiring: 3,
    expired: 0,
    status: 'WARNING'
  },
  proxyCache: {
    entries: 1234,
    hitRate: 87.3, // percentage
    status: 'OK'
  },
  storage: {
    status: 'OPERATIONAL',
    used: 45.2, // GB
    total: 100, // GB
    percentage: 45.2
  }
};

/**
 * Service Discoveries - Pending services discovered but not yet approved
 */
export const discoveries = [
  {
    id: 'discovery-001',
    name: 'Test API Service',
    source: 'docker',
    target: 'http://test-api-service:3000',
    path: '/apps/api/',
    confidence: 100,
    tags: ['test', 'api', 'json'],
    security: {
      hasWarning: true,
      message: 'Upstream uses HTTP (unencrypted)'
    },
    details: {
      source: 'docker',
      target: 'http://test-api-service:3000',
      path: '/apps/api/',
      security: 'Discovered via Docker API, JWT required'
    },
    discoveredAt: new Date(Date.now() - 2 * 60 * 60 * 1000).toISOString(), // 2h ago
    selected: false
  },
  {
    id: 'discovery-002',
    name: 'User Management API',
    source: 'docker',
    target: 'http://user-service:8080',
    path: '/users/',
    confidence: 95,
    tags: ['users', 'auth'],
    security: {
      hasWarning: false
    },
    details: {
      source: 'docker',
      target: 'http://user-service:8080',
      path: '/users/',
      security: 'Discovered via Docker API, JWT required'
    },
    discoveredAt: new Date(Date.now() - 3 * 60 * 60 * 1000).toISOString(), // 3h ago
    selected: false
  },
  {
    id: 'discovery-003',
    name: 'Analytics Dashboard',
    source: 'mdns',
    target: 'http://analytics.local:5000',
    path: '/analytics/',
    confidence: 85,
    tags: ['analytics', 'dashboard', 'metrics'],
    security: {
      hasWarning: true,
      message: 'mDNS discovery, verify target before approval'
    },
    details: {
      source: 'mdns',
      target: 'http://analytics.local:5000',
      path: '/analytics/',
      security: 'Discovered via mDNS, certificate pinning recommended'
    },
    discoveredAt: new Date(Date.now() - 5 * 60 * 60 * 1000).toISOString(), // 5h ago
    selected: false
  },
  {
    id: 'discovery-004',
    name: 'Payment Gateway',
    source: 'kubernetes',
    target: 'https://payment-gateway.default.svc.cluster.local',
    path: '/payments/',
    confidence: 98,
    tags: ['payments', 'pci', 'secure'],
    security: {
      hasWarning: false
    },
    details: {
      source: 'kubernetes',
      target: 'https://payment-gateway.default.svc.cluster.local',
      path: '/payments/',
      security: 'Discovered via K8s API, TLS enabled, JWT required'
    },
    discoveredAt: new Date(Date.now() - 1 * 60 * 60 * 1000).toISOString(), // 1h ago
    selected: false
  },
  {
    id: 'discovery-005',
    name: 'Notification Service',
    source: 'docker',
    target: 'http://notifications:9000',
    path: '/notify/',
    confidence: 90,
    tags: ['notifications', 'webhooks', 'email'],
    security: {
      hasWarning: true,
      message: 'Rate limiting recommended for webhook endpoints'
    },
    details: {
      source: 'docker',
      target: 'http://notifications:9000',
      path: '/notify/',
      security: 'Discovered via Docker API, implement rate limiting'
    },
    discoveredAt: new Date(Date.now() - 30 * 60 * 1000).toISOString(), // 30m ago
    selected: false
  },
  {
    id: 'discovery-006',
    name: 'Legacy Database API',
    source: 'mdns',
    target: 'http://legacy-db.local:3306',
    path: '/db/',
    confidence: 65,
    tags: ['database', 'legacy', 'mysql'],
    security: {
      hasWarning: true,
      message: 'Low confidence detection, manual verification required'
    },
    details: {
      source: 'mdns',
      target: 'http://legacy-db.local:3306',
      path: '/db/',
      security: 'Discovered via mDNS, low confidence - verify manually'
    },
    discoveredAt: new Date(Date.now() - 12 * 60 * 60 * 1000).toISOString(), // 12h ago
    selected: false
  },
  {
    id: 'discovery-007',
    name: 'ML Inference Service',
    source: 'kubernetes',
    target: 'http://ml-inference.ml-namespace.svc.cluster.local:8000',
    path: '/ml/inference/',
    confidence: 92,
    tags: ['ml', 'ai', 'inference', 'python'],
    security: {
      hasWarning: false
    },
    details: {
      source: 'kubernetes',
      target: 'http://ml-inference.ml-namespace.svc.cluster.local:8000',
      path: '/ml/inference/',
      security: 'Discovered via K8s API, namespace isolated, JWT required'
    },
    discoveredAt: new Date(Date.now() - 45 * 60 * 1000).toISOString(), // 45m ago
    selected: false
  }
];

/**
 * Paired Devices - Devices that have been paired and granted access
 */
export const devices = [
  {
    id: 'device-001',
    name: 'iPhone 15 Pro',
    platform: 'iOS',
    platformVersion: '17.2',
    status: 'online',
    lastSeen: new Date(Date.now() - 2 * 60 * 1000).toISOString(), // 2m ago
    pairedAt: new Date(Date.now() - 7 * 24 * 60 * 60 * 1000).toISOString(), // 7 days ago
    requestsToday: 342,
    totalRequests: 15847,
    ipAddress: '192.168.1.105',
    userAgent: 'Nekzus-iOS/1.2.0'
  },
  {
    id: 'device-002',
    name: 'MacBook Pro',
    platform: 'macOS',
    platformVersion: '14.2',
    status: 'online',
    lastSeen: new Date(Date.now() - 5 * 60 * 1000).toISOString(), // 5m ago
    pairedAt: new Date(Date.now() - 30 * 24 * 60 * 60 * 1000).toISOString(), // 30 days ago
    requestsToday: 892,
    totalRequests: 28456,
    ipAddress: '192.168.1.102',
    userAgent: 'Nekzus-macOS/2.1.0'
  },
  {
    id: 'device-003',
    name: 'Galaxy S24 Ultra',
    platform: 'Android',
    platformVersion: '14',
    status: 'online',
    lastSeen: new Date(Date.now() - 1 * 60 * 1000).toISOString(), // 1m ago
    pairedAt: new Date(Date.now() - 14 * 24 * 60 * 60 * 1000).toISOString(), // 14 days ago
    requestsToday: 156,
    totalRequests: 4523,
    ipAddress: '192.168.1.108',
    userAgent: 'Nekzus-Android/1.5.2'
  },
  {
    id: 'device-004',
    name: 'iPad Air',
    platform: 'iOS',
    platformVersion: '17.1',
    status: 'offline',
    lastSeen: new Date(Date.now() - 3 * 60 * 60 * 1000).toISOString(), // 3h ago
    pairedAt: new Date(Date.now() - 45 * 24 * 60 * 60 * 1000).toISOString(), // 45 days ago
    requestsToday: 0,
    totalRequests: 12034,
    ipAddress: '192.168.1.110',
    userAgent: 'Nekzus-iOS/1.1.8'
  },
  {
    id: 'device-005',
    name: 'Work Laptop',
    platform: 'Windows',
    platformVersion: '11',
    status: 'online',
    lastSeen: new Date(Date.now() - 10 * 60 * 1000).toISOString(), // 10m ago
    pairedAt: new Date(Date.now() - 60 * 24 * 60 * 60 * 1000).toISOString(), // 60 days ago
    requestsToday: 1245,
    totalRequests: 67891,
    ipAddress: '192.168.1.115',
    userAgent: 'Nekzus-Windows/2.0.5'
  },
  {
    id: 'device-006',
    name: 'Pixel 8 Pro',
    platform: 'Android',
    platformVersion: '14',
    status: 'offline',
    lastSeen: new Date(Date.now() - 12 * 60 * 60 * 1000).toISOString(), // 12h ago
    pairedAt: new Date(Date.now() - 21 * 24 * 60 * 60 * 1000).toISOString(), // 21 days ago
    requestsToday: 0,
    totalRequests: 8765,
    ipAddress: '192.168.1.120',
    userAgent: 'Nekzus-Android/1.5.0'
  },
  {
    id: 'device-007',
    name: 'Chrome Browser',
    platform: 'Web',
    platformVersion: 'Chrome 120',
    status: 'online',
    lastSeen: new Date(Date.now() - 30 * 1000).toISOString(), // 30s ago
    pairedAt: new Date(Date.now() - 5 * 24 * 60 * 60 * 1000).toISOString(), // 5 days ago
    requestsToday: 89,
    totalRequests: 456,
    ipAddress: '192.168.1.102',
    userAgent: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)'
  },
  {
    id: 'device-008',
    name: 'Home Server',
    platform: 'Linux',
    platformVersion: 'Ubuntu 22.04',
    status: 'online',
    lastSeen: new Date(Date.now() - 15 * 1000).toISOString(), // 15s ago
    pairedAt: new Date(Date.now() - 90 * 24 * 60 * 60 * 1000).toISOString(), // 90 days ago
    requestsToday: 2456,
    totalRequests: 234567,
    ipAddress: '192.168.1.50',
    userAgent: 'Nekzus-CLI/3.0.1'
  },
  {
    id: 'device-009',
    name: 'Old Phone',
    platform: 'Android',
    platformVersion: '12',
    status: 'offline',
    lastSeen: new Date(Date.now() - 7 * 24 * 60 * 60 * 1000).toISOString(), // 7 days ago
    pairedAt: new Date(Date.now() - 180 * 24 * 60 * 60 * 1000).toISOString(), // 180 days ago
    requestsToday: 0,
    totalRequests: 3456,
    ipAddress: '192.168.1.125',
    userAgent: 'Nekzus-Android/1.2.0'
  },
  {
    id: 'device-010',
    name: 'Test Device',
    platform: 'iOS',
    platformVersion: '17.0',
    status: 'online',
    lastSeen: new Date(Date.now() - 45 * 1000).toISOString(), // 45s ago
    pairedAt: new Date(Date.now() - 1 * 60 * 60 * 1000).toISOString(), // 1h ago
    requestsToday: 23,
    totalRequests: 23,
    ipAddress: '192.168.1.130',
    userAgent: 'Nekzus-iOS/1.3.0-beta'
  }
];

/**
 * System Information
 */
export const systemInfo = {
  version: '1.0.0',
  build: 'a1b2c3d',
  startTime: new Date(Date.now() - 15 * 24 * 60 * 60 * 1000).toISOString(), // 15 days ago
  hostname: 'nekzus-01',
  platform: 'linux/amd64',
  goVersion: '1.25.0'
};
