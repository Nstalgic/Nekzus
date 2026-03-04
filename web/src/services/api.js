/**
 * API Service
 *
 * Handles all HTTP requests to the Nekzus backend API.
 * All endpoints are relative URLs since the frontend is served
 * from the same server.
 *
 * Includes automatic JWT token injection and 401 handling.
 */

import { debug, errorDetails } from '../utils/debug';

/**
 * Base API configuration
 */
const API_BASE = '/api/v1';
const ADMIN_BASE = '/api/v1/admin';

/**
 * LocalStorage key for JWT token
 * @constant {string}
 */
const TOKEN_STORAGE_KEY = 'nekzus-token';

/**
 * Get authentication token from localStorage
 * @returns {string|null} JWT token or null
 */
function getAuthToken() {
  return localStorage.getItem(TOKEN_STORAGE_KEY);
}

/**
 * HTTP request helper with error handling and authentication
 * @param {string} url - API endpoint URL
 * @param {Object} options - Fetch options
 * @returns {Promise<Object>} Response data
 * @throws {Error} API error with status and message
 */
async function request(url, options = {}) {
  const method = options.method || 'GET';

  // Log API request when debug mode is enabled
  debug.api(method, url, options.body ? JSON.parse(options.body) : undefined);

  try {
    const token = getAuthToken();

    const response = await fetch(url, {
      ...options,
      headers: {
        'Content-Type': 'application/json',
        ...(token && { 'Authorization': `Bearer ${token}` }),
        ...options.headers,
      },
    });

    // Handle 401 Unauthorized - token expired or invalid
    if (response.status === 401) {
      debug.log('API', 'Unauthorized - clearing token and reloading');
      // Clear invalid token
      localStorage.removeItem(TOKEN_STORAGE_KEY);
      // Trigger page reload to show login screen
      window.location.reload();
      return;
    }

    // Handle 204 No Content
    if (response.status === 204) {
      debug.apiResponse(method, url, 204);
      return null;
    }

    // Parse JSON response
    const data = await response.json();

    // Log API response when debug mode is enabled
    debug.apiResponse(method, url, response.status, data);

    // Check for error status
    if (!response.ok) {
      // Handle both error formats: {message, code} and {error: {message, code}}
      const errorMessage = data.error?.message || data.message || 'API request failed';
      const errorCode = data.error?.code || data.code;
      const error = new Error(errorMessage);
      error.status = response.status;
      error.code = errorCode;
      error.response = { data };
      throw error;
    }

    return data;
  } catch (error) {
    // Log error with details when showErrorDetails is enabled
    debug.error('API', `${method} ${url} failed`, error);

    // Re-throw API errors
    if (error.status) {
      throw error;
    }

    // Network or parsing errors
    const networkError = new Error(
      errorDetails.isEnabled()
        ? 'Network error: ' + error.message
        : 'Network error'
    );
    networkError.status = 0;
    networkError.code = 'NETWORK_ERROR';
    networkError.originalError = error;
    throw networkError;
  }
}

/**
 * Health API
 */
export const healthAPI = {
  /**
   * Check server health
   * @returns {Promise<string>} "ok" if healthy
   */
  check: () => fetch('/healthz').then(res => res.text()),

  /**
   * Get detailed health info
   * @returns {Promise<Object>} Health information
   */
  detailed: () => request(`${API_BASE}/healthz`),
};

/**
 * Routes API
 */
export const routesAPI = {
  /**
   * List all routes
   * @returns {Promise<Array>} Array of routes
   */
  list: () => request(`${API_BASE}/routes`),

  /**
   * Get a specific route
   * @param {string} routeId - Route ID
   * @returns {Promise<Object>} Route object
   */
  get: (routeId) => request(`${API_BASE}/routes/${routeId}`),

  /**
   * Update a route
   * @param {string} routeId - Route ID
   * @param {Object} route - Updated route data
   * @returns {Promise<Object>} Updated route
   */
  update: (routeId, route) =>
    request(`${API_BASE}/routes/${routeId}`, {
      method: 'PATCH',
      body: JSON.stringify(route),
    }),

  /**
   * Delete a route
   * @param {string} routeId - Route ID
   * @returns {Promise<null>} No content
   */
  delete: (routeId) =>
    request(`${API_BASE}/routes/${routeId}`, {
      method: 'DELETE',
    }),
};

/**
 * Discovery API
 */
export const discoveryAPI = {
  /**
   * List all discovery proposals
   * @returns {Promise<Array>} Array of proposals
   */
  listProposals: () => request(`${API_BASE}/discovery/proposals`),

  /**
   * Approve a discovery proposal
   * @param {string} proposalId - Proposal ID
   * @param {Object} options - Optional parameters
   * @param {number} options.port - Optional port to use from availablePorts
   * @returns {Promise<Object>} Approval result
   */
  approveProposal: (proposalId, options = {}) =>
    request(`${API_BASE}/discovery/proposals/${proposalId}/approve`, {
      method: 'POST',
      headers: options.port ? { 'Content-Type': 'application/json' } : undefined,
      body: options.port ? JSON.stringify({ port: options.port }) : undefined,
    }),

  /**
   * Dismiss a discovery proposal
   * @param {string} proposalId - Proposal ID
   * @returns {Promise<Object>} Dismissal result
   */
  dismissProposal: (proposalId) =>
    request(`${API_BASE}/discovery/proposals/${proposalId}/dismiss`, {
      method: 'POST',
    }),

  /**
   * Trigger rediscovery scan
   * Clears dismissed and active proposals to allow fresh discovery
   * @returns {Promise<Object>} Rediscovery result with cleared counts
   */
  rediscover: () =>
    request(`${API_BASE}/discovery/rediscover`, {
      method: 'POST',
    }),
};

/**
 * Devices API
 */
export const devicesAPI = {
  /**
   * List all paired devices
   * @returns {Promise<Array>} Array of devices
   */
  list: () => request(`${ADMIN_BASE}/devices`),

  /**
   * Get a specific device
   * @param {string} deviceId - Device ID
   * @returns {Promise<Object>} Device object
   */
  get: (deviceId) => request(`${ADMIN_BASE}/devices/${deviceId}`),

  /**
   * Revoke a device
   * @param {string} deviceId - Device ID
   * @returns {Promise<null>} No content
   */
  revoke: (deviceId) =>
    request(`${ADMIN_BASE}/devices/${deviceId}`, {
      method: 'DELETE',
    }),

  /**
   * Update device metadata
   * @param {string} deviceId - Device ID
   * @param {Object} metadata - Updated metadata
   * @returns {Promise<Object>} Updated device
   */
  updateMetadata: (deviceId, metadata) =>
    request(`${ADMIN_BASE}/devices/${deviceId}`, {
      method: 'PATCH',
      body: JSON.stringify(metadata),
    }),
};

/**
 * Stats API
 */
export const statsAPI = {
  /**
   * Get system statistics
   * @returns {Promise<Object>} Stats object with routes, devices, discoveries, requests
   */
  get: () => request(`${API_BASE}/stats`),
};

/**
 * System API
 */
export const systemAPI = {
  /**
   * Get system resource usage
   * @returns {Promise<Object>} Resource metrics (CPU, RAM, disk, storage_size)
   * @example
   * {
   *   cpu: 15.2,          // CPU usage percentage
   *   ram: 45.8,          // RAM usage percentage
   *   disk: 62.3,         // Disk usage percentage
   *   storage_size: 1234567  // Database file size in bytes
   * }
   */
  getResources: () => request(`${API_BASE}/system/resources`),
};

/**
 * Activity API
 */
export const activityAPI = {
  /**
   * Get recent activity
   * @param {Object} options - Query options
   * @param {number} options.limit - Maximum number of events to return
   * @param {number} options.offset - Offset for pagination
   * @returns {Promise<Array|Object>} Array of events or paginated response
   */
  getRecent: ({ limit, offset } = {}) => {
    const params = new URLSearchParams();
    if (limit !== undefined) params.append('limit', limit);
    if (offset !== undefined) params.append('offset', offset);

    const queryString = params.toString();
    const url = `${API_BASE}/activity/recent${queryString ? `?${queryString}` : ''}`;

    return request(url);
  },
};

/**
 * Admin API
 */
export const adminAPI = {
  /**
   * Get Nexus instance information
   * @returns {Promise<Object>} Instance info (version, nexusId, capabilities)
   */
  getInfo: () => request(`${ADMIN_BASE}/info`),
};

/**
 * Auth API
 */
export const authAPI = {
  /**
   * Login with username and password
   * @param {string} username - Username
   * @param {string} password - Password
   * @returns {Promise<Object>} Login response with token and user
   */
  login: async (username, password) => {
    return request(`${API_BASE}/auth/login`, {
      method: 'POST',
      body: JSON.stringify({ username, password }),
    });
  },

  /**
   * Get current user information
   * @returns {Promise<Object>} User object
   */
  me: async () => {
    return request(`${API_BASE}/auth/me`);
  },

  /**
   * Logout current user
   * @returns {Promise<null>} No content
   */
  logout: async () => {
    return request(`${API_BASE}/auth/logout`, {
      method: 'POST',
    });
  },

  /**
   * Get QR code data for pairing
   * @returns {Promise<Object>} QR code payload
   */
  getQRCode: () => request(`${API_BASE}/auth/qr`),

  /**
   * Get QR code as PNG image URL
   * @returns {string} URL to QR code PNG
   */
  getQRCodeImageURL: () => `${API_BASE}/auth/qr?format=png`,
};

/**
 * Webhooks API
 */
export const webhooksAPI = {
  /**
   * Send activity webhook
   * @param {Object} payload - Activity webhook payload
   * @param {string} payload.message - Activity message
   * @param {string} [payload.icon] - Icon name
   * @param {string} [payload.iconClass] - Icon style class (success, warning, danger)
   * @param {string} [payload.details] - Additional details
   * @param {Array<string>} [payload.deviceIds] - Target device IDs (empty = broadcast)
   * @returns {Promise<Object>} Webhook response
   */
  sendActivity: (payload) =>
    request(`${API_BASE}/webhooks/activity`, {
      method: 'POST',
      body: JSON.stringify(payload),
    }),

  /**
   * Send notify webhook
   * @param {Object} payload - Notify webhook payload
   * @param {string} payload.type - Notification type
   * @param {Object} payload.data - Notification data (arbitrary JSON)
   * @param {Array<string>} [payload.deviceIds] - Target device IDs (empty = broadcast)
   * @returns {Promise<Object>} Webhook response
   */
  sendNotify: (payload) =>
    request(`${API_BASE}/webhooks/notify`, {
      method: 'POST',
      body: JSON.stringify(payload),
    }),
};

/**
 * Certificates API
 */
export const certificatesAPI = {
  /**
   * List all certificates
   * @returns {Promise<Object>} Object with certificates array and count
   * @example
   * {
   *   certificates: [{
   *     domain: "app.local",
   *     issuer: "self-signed",
   *     not_before: "2025-01-15T10:00:00Z",
   *     not_after: "2026-01-15T10:00:00Z",
   *     sans: ["app.local", "app2.local"],
   *     fingerprint: "sha256/AbCdEf123...",
   *     expires_in_days: 345.5
   *   }],
   *   count: 1
   * }
   */
  list: () => request(`${API_BASE}/certificates`),

  /**
   * Get a specific certificate by domain
   * @param {string} domain - Certificate domain
   * @returns {Promise<Object>} Certificate object
   */
  get: (domain) => request(`${API_BASE}/certificates/${encodeURIComponent(domain)}`),

  /**
   * Generate a new self-signed certificate
   * @param {Object} payload - Certificate generation request
   * @param {Array<string>} payload.domains - Array of domains for the certificate
   * @param {string} [payload.provider="self-signed"] - Certificate provider
   * @returns {Promise<Object>} Generated certificate
   * @example
   * certificatesAPI.generate({
   *   domains: ["app.local", "app2.local"],
   *   provider: "self-signed"
   * })
   */
  generate: (payload) =>
    request(`${API_BASE}/certificates/generate`, {
      method: 'POST',
      body: JSON.stringify(payload),
    }),

  /**
   * Delete a certificate by domain
   * @param {string} domain - Certificate domain
   * @returns {Promise<Object>} Deletion result
   */
  delete: (domain) =>
    request(`${API_BASE}/certificates/${encodeURIComponent(domain)}`, {
      method: 'DELETE',
    }),

  /**
   * Get suggested domains for certificate auto-configuration
   * @returns {Promise<Object>} Object with suggestions array and count
   * @example
   * {
   *   suggestions: ["localhost", "myhost", "myhost.local", "192.168.1.100"],
   *   count: 4
   * }
   */
  suggest: () => request(`${API_BASE}/certificates/suggest`),
};

/**
 * API Keys API
 */
export const apiKeysAPI = {
  /**
   * Create a new API key
   * @param {Object} payload - API key creation request
   * @param {string} payload.name - API key name
   * @param {Array<string>} payload.scopes - Array of scopes (e.g., ["write:*"])
   * @param {string} [payload.expiresAt] - Optional expiration date (ISO 8601)
   * @returns {Promise<Object>} Created API key with plaintext key (only returned once!)
   * @example
   * {
   *   id: "key_abc123",
   *   name: "Webhook Integration",
   *   prefix: "nekzus_abc",
   *   key: "nekzus_abc123def456...",  // Full key - only shown once!
   *   scopes: ["write:*"],
   *   createdAt: "2025-01-15T10:00:00Z",
   *   expiresAt: null
   * }
   */
  create: (payload) =>
    request(`${API_BASE}/apikeys`, {
      method: 'POST',
      body: JSON.stringify(payload),
    }),

  /**
   * List all API keys (without plaintext keys)
   * @returns {Promise<Array>} Array of API key objects
   */
  list: () => request(`${API_BASE}/apikeys`),

  /**
   * Get a specific API key (without plaintext key)
   * @param {string} keyId - API key ID
   * @returns {Promise<Object>} API key object
   */
  get: (keyId) => request(`${API_BASE}/apikeys/${keyId}`),

  /**
   * Revoke (soft delete) an API key
   * @param {string} keyId - API key ID
   * @returns {Promise<Object>} Revocation result
   */
  revoke: (keyId) =>
    request(`${API_BASE}/apikeys/${keyId}`, {
      method: 'DELETE',
    }),

  /**
   * Permanently delete an API key
   * @param {string} keyId - API key ID
   * @returns {Promise<Object>} Deletion result
   */
  delete: (keyId) =>
    request(`${API_BASE}/apikeys/${keyId}?permanent=true`, {
      method: 'DELETE',
    }),
};

/**
 * Containers API
 */
export const containersAPI = {
  /**
   * List all Docker containers
   * @returns {Promise<Array>} Array of container objects
   * @example
   * [{
   *   id: "abc123def456",
   *   name: "grafana",
   *   image: "grafana/grafana:latest",
   *   state: "running",
   *   status: "Up 2 hours",
   *   created: "2025-01-10T10:00:00Z",
   *   ports: [{
   *     IP: "0.0.0.0",
   *     PrivatePort: 3000,
   *     PublicPort: 3000,
   *     Type: "tcp"
   *   }],
   *   labels: { "com.docker.compose.service": "grafana" }
   * }]
   */
  list: () => request(`${API_BASE}/containers`),

  /**
   * Get detailed information about a specific container
   * @param {string} containerId - Container ID or name
   * @returns {Promise<Object>} Detailed container information
   * @example
   * {
   *   id: "abc123def456",
   *   name: "grafana",
   *   image: "grafana/grafana:latest",
   *   state: "running",
   *   config: {},
   *   network_settings: {},
   *   mounts: []
   * }
   */
  get: (containerId) => request(`${API_BASE}/containers/${containerId}`),

  /**
   * Start a stopped container
   * @param {string} containerId - Container ID or name
   * @returns {Promise<Object>} Start result
   */
  start: (containerId) =>
    request(`${API_BASE}/containers/${containerId}/start`, {
      method: 'POST',
    }),

  /**
   * Stop a running container
   * @param {string} containerId - Container ID or name
   * @param {number} [timeout=10] - Timeout in seconds before force kill
   * @returns {Promise<Object>} Stop result
   */
  stop: (containerId, timeout = 10) =>
    request(`${API_BASE}/containers/${containerId}/stop?timeout=${timeout}`, {
      method: 'POST',
    }),

  /**
   * Restart a container
   * @param {string} containerId - Container ID or name
   * @returns {Promise<Object>} Restart result
   */
  restart: (containerId) =>
    request(`${API_BASE}/containers/${containerId}/restart`, {
      method: 'POST',
    }),

  /**
   * Get container logs
   * @param {string} containerId - Container ID or name
   * @param {Object} options - Log options
   * @param {number} [options.tail=100] - Number of lines to show from end
   * @param {boolean} [options.follow=false] - Follow log output
   * @param {boolean} [options.timestamps=false] - Show timestamps
   * @returns {Promise<string>} Container logs as text
   */
  logs: (containerId, { tail = 100, follow = false, timestamps = false } = {}) => {
    const params = new URLSearchParams();
    params.append('tail', tail);
    if (follow) params.append('follow', 'true');
    if (timestamps) params.append('timestamps', 'true');

    return request(`${API_BASE}/containers/${containerId}/logs?${params.toString()}`);
  },

  /**
   * Get container stats (CPU, memory, network)
   * @param {string} containerId - Container ID or name
   * @returns {Promise<Object>} Container statistics
   * @example
   * {
   *   cpu_percent: 2.3,
   *   memory_usage: 234567890,
   *   memory_limit: 2147483648,
   *   memory_percent: 10.92,
   *   network_rx: 1234567,
   *   network_tx: 7654321
   * }
   */
  stats: (containerId) => request(`${API_BASE}/containers/${containerId}/stats`),
};

/**
 * Scripts API
 */
export const scriptsAPI = {
  /**
   * List all scripts
   * @returns {Promise<Array>} Array of script objects
   */
  list: () => request(`${API_BASE}/scripts`),

  /**
   * Get a specific script
   * @param {string} id - Script ID
   * @returns {Promise<Object>} Script object
   */
  get: (id) => request(`${API_BASE}/scripts/${id}`),

  /**
   * Get available scripts from the script directory
   * @returns {Promise<Array>} Array of available scripts
   */
  getAvailable: () => request(`${API_BASE}/scripts/available`),

  /**
   * Register a new script
   * @param {Object} script - Script registration payload
   * @param {string} script.name - Script name
   * @param {string} script.path - Path to script file
   * @param {string} [script.description] - Script description
   * @param {Array<Object>} [script.parameters] - Script parameters
   * @returns {Promise<Object>} Created script object
   */
  register: (script) =>
    request(`${API_BASE}/scripts`, {
      method: 'POST',
      body: JSON.stringify(script),
    }),

  /**
   * Update a script
   * @param {string} id - Script ID
   * @param {Object} script - Updated script data
   * @returns {Promise<Object>} Updated script object
   */
  update: (id, script) =>
    request(`${API_BASE}/scripts/${id}`, {
      method: 'PUT',
      body: JSON.stringify(script),
    }),

  /**
   * Delete a script
   * @param {string} id - Script ID
   * @returns {Promise<Object>} Deletion result
   */
  delete: (id) =>
    request(`${API_BASE}/scripts/${id}`, {
      method: 'DELETE',
    }),

  /**
   * Execute a script synchronously (waits for completion)
   * @param {string} id - Script ID
   * @param {Object} params - Script parameters
   * @returns {Promise<Object>} Execution result
   */
  execute: (id, params = {}) =>
    request(`${API_BASE}/scripts/${id}/execute`, {
      method: 'POST',
      body: JSON.stringify({ parameters: params }),
    }),

  /**
   * Execute a script asynchronously (returns immediately with execution ID)
   * @param {string} id - Script ID
   * @param {Object} params - Script parameters
   * @returns {Promise<Object>} Object containing executionId, status, and pollUrl
   */
  executeAsync: (id, params = {}) =>
    request(`${API_BASE}/scripts/${id}/execute?async=true`, {
      method: 'POST',
      body: JSON.stringify({ parameters: params }),
    }),

  /**
   * Dry-run a script (validate without executing)
   * @param {string} id - Script ID
   * @param {Object} params - Script parameters
   * @returns {Promise<Object>} Dry-run result
   */
  dryRun: (id, params = {}) =>
    request(`${API_BASE}/scripts/${id}/dry-run`, {
      method: 'POST',
      body: JSON.stringify({ parameters: params }),
    }),

  /**
   * List script executions
   * @param {Object} filters - Query filters
   * @param {string} [filters.scriptId] - Filter by script ID
   * @param {string} [filters.status] - Filter by status (pending, running, success, failed)
   * @param {number} [filters.limit] - Maximum number of results
   * @returns {Promise<Array>} Array of execution objects
   */
  listExecutions: (filters = {}) => {
    const params = new URLSearchParams();
    if (filters.scriptId) params.append('scriptId', filters.scriptId);
    if (filters.status) params.append('status', filters.status);
    if (filters.limit !== undefined) params.append('limit', filters.limit);

    const queryString = params.toString();
    const url = `${API_BASE}/executions${queryString ? `?${queryString}` : ''}`;

    return request(url);
  },

  /**
   * Get a specific execution
   * @param {string} id - Execution ID
   * @returns {Promise<Object>} Execution object
   */
  getExecution: (id) => request(`${API_BASE}/executions/${id}`),

  /**
   * List all workflows
   * @returns {Promise<Array>} Array of workflow objects
   */
  listWorkflows: () => request(`${API_BASE}/workflows`),

  /**
   * Get a specific workflow
   * @param {string} id - Workflow ID
   * @returns {Promise<Object>} Workflow object
   */
  getWorkflow: (id) => request(`${API_BASE}/workflows/${id}`),

  /**
   * Create a new workflow
   * @param {Object} workflow - Workflow creation payload
   * @param {string} workflow.name - Workflow name
   * @param {string} [workflow.description] - Workflow description
   * @param {Array<Object>} workflow.steps - Workflow steps
   * @returns {Promise<Object>} Created workflow object
   */
  createWorkflow: (workflow) =>
    request(`${API_BASE}/workflows`, {
      method: 'POST',
      body: JSON.stringify(workflow),
    }),

  /**
   * Delete a workflow
   * @param {string} id - Workflow ID
   * @returns {Promise<Object>} Deletion result
   */
  deleteWorkflow: (id) =>
    request(`${API_BASE}/workflows/${id}`, {
      method: 'DELETE',
    }),

  /**
   * List all schedules
   * @returns {Promise<Array>} Array of schedule objects
   */
  listSchedules: () => request(`${API_BASE}/schedules`),

  /**
   * Get a specific schedule
   * @param {string} id - Schedule ID
   * @returns {Promise<Object>} Schedule object
   */
  getSchedule: (id) => request(`${API_BASE}/schedules/${id}`),

  /**
   * Create a new schedule
   * @param {Object} schedule - Schedule creation payload
   * @param {string} schedule.name - Schedule name
   * @param {string} schedule.scriptId - Script ID to execute
   * @param {string} schedule.cron - Cron expression
   * @param {Object} [schedule.parameters] - Script parameters
   * @returns {Promise<Object>} Created schedule object
   */
  createSchedule: (schedule) =>
    request(`${API_BASE}/schedules`, {
      method: 'POST',
      body: JSON.stringify(schedule),
    }),

  /**
   * Delete a schedule
   * @param {string} id - Schedule ID
   * @returns {Promise<Object>} Deletion result
   */
  deleteSchedule: (id) =>
    request(`${API_BASE}/schedules/${id}`, {
      method: 'DELETE',
    }),
};

/**
 * Notifications API
 */
export const notificationsAPI = {
  /**
   * List notifications with optional filters
   * @param {Object} filters - Query filters
   * @param {string} [filters.status] - Filter by status (pending, delivered, failed, dismissed)
   * @param {string} [filters.deviceId] - Filter by device ID
   * @param {string} [filters.type] - Filter by notification type
   * @param {number} [filters.limit] - Maximum number of results
   * @param {number} [filters.offset] - Offset for pagination
   * @returns {Promise<Object>} Notification list result with total count
   */
  list: (filters = {}) => {
    const params = new URLSearchParams();
    if (filters.status) params.append('status', filters.status);
    if (filters.deviceId) params.append('device_id', filters.deviceId);
    if (filters.type) params.append('type', filters.type);
    if (filters.limit !== undefined) params.append('limit', filters.limit);
    if (filters.offset !== undefined) params.append('offset', filters.offset);

    const queryString = params.toString();
    const url = `${API_BASE}/notifications${queryString ? `?${queryString}` : ''}`;

    return request(url);
  },

  /**
   * Get notification queue statistics
   * @returns {Promise<Object>} Queue statistics
   */
  getStats: () => request(`${API_BASE}/notifications/stats`),

  /**
   * Get stale notifications grouped by device
   * @returns {Promise<Object>} Stale notification summaries
   */
  getStale: () => request(`${API_BASE}/notifications/stale`),

  /**
   * Dismiss a notification
   * @param {number} id - Notification ID
   * @returns {Promise<Object>} Dismissal result
   */
  dismiss: (id) =>
    request(`${API_BASE}/notifications/${id}`, {
      method: 'DELETE',
    }),

  /**
   * Dismiss all notifications for a device
   * @param {string} deviceId - Device ID
   * @returns {Promise<Object>} Dismissal result with count
   */
  dismissForDevice: (deviceId) =>
    request(`${API_BASE}/notifications/device/${deviceId}`, {
      method: 'DELETE',
    }),

  /**
   * Retry a failed notification
   * @param {number} id - Notification ID
   * @returns {Promise<Object>} Retry result
   */
  retry: (id) =>
    request(`${API_BASE}/notifications/${id}/retry`, {
      method: 'POST',
    }),

  /**
   * Bulk retry notifications
   * @param {Array<number>} ids - Array of notification IDs
   * @returns {Promise<Object>} Retry result with count
   */
  bulkRetry: (ids) =>
    request(`${API_BASE}/notifications/retry`, {
      method: 'POST',
      body: JSON.stringify({ ids }),
    }),

  /**
   * Clear all delivered notifications
   * @returns {Promise<Object>} Clear result with count
   */
  clearDelivered: () =>
    request(`${API_BASE}/notifications/delivered`, {
      method: 'DELETE',
    }),
};

/**
 * Federation API
 */
export const federationAPI = {
  /**
   * Get federation status
   * @returns {Promise<Object>} Federation status and config
   */
  status: () => request(`${API_BASE}/federation/status`),

  /**
   * List all connected peers
   * @returns {Promise<Array>} Array of peer objects
   */
  listPeers: () => request(`${API_BASE}/federation/peers`),

  /**
   * Get a specific peer
   * @param {string} peerId - Peer ID
   * @returns {Promise<Object>} Peer object
   */
  getPeer: (peerId) => request(`${API_BASE}/federation/peers/${peerId}`),

  /**
   * Get federated service catalog
   * @returns {Promise<Array>} Array of federated services
   */
  getCatalog: () => request(`${API_BASE}/federation/catalog`),

  /**
   * Trigger manual sync
   * @returns {Promise<Object>} Sync result
   */
  triggerSync: () =>
    request(`${API_BASE}/federation/sync`, {
      method: 'POST',
    }),

  /**
   * Get federation metrics
   * @returns {Promise<Object>} Federation metrics
   */
  getMetrics: () => request(`${API_BASE}/federation/metrics`),
};
