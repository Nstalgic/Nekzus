/**
 * DataContext - Central data management for Nekzus Dashboard
 *
 * Manages routes, activities, devices, and discoveries with real API integration
 * Connects to WebSocket for real-time updates
 */

import { createContext, useContext, useState, useEffect, useCallback, useRef } from 'react';
import PropTypes from 'prop-types';
import {
  routesAPI,
  discoveryAPI,
  devicesAPI,
  activityAPI,
  statsAPI,
  healthAPI,
  containersAPI,
  systemAPI
} from '../services/api';
import { websocketService, WS_MSG_TYPES } from '../services/websocket';
import { useNotification } from './NotificationContext';

const DataContext = createContext(null);

/**
 * Format relative time (e.g., "2m ago", "1h ago")
 */
const getRelativeTime = (timestamp) => {
  const now = Date.now();
  const past = new Date(timestamp).getTime();
  const diffMs = now - past;
  const diffSec = Math.floor(diffMs / 1000);
  const diffMin = Math.floor(diffSec / 60);
  const diffHour = Math.floor(diffMin / 60);
  const diffDay = Math.floor(diffHour / 24);

  if (diffSec < 60) return 'Just now';
  if (diffMin < 60) return `${diffMin}m ago`;
  if (diffHour < 24) return `${diffHour}h ago`;
  return `${diffDay}d ago`;
};

/**
 * DataProvider Component
 */
export function DataProvider({ children }) {
  // Hooks
  const { addNotification } = useNotification();

  // State
  const [routes, setRoutes] = useState([]);
  const [discoveries, setDiscoveries] = useState([]);
  const [devices, setDevices] = useState([]);
  const [activities, setActivities] = useState([]);
  const [containers, setContainers] = useState([]);
  const [stats, setStats] = useState({
    routes: { value: 0, trend: 'Loading...', trendUp: false },
    devices: { value: 0, trend: 'Loading...', trendUp: false },
    discoveries: { value: 0, trend: 'Loading...', trendUp: false },
    requests: { value: 0, trend: 'Loading...', trendUp: false },
  });
  const [isLoading, setIsLoading] = useState(true);
  const [containersLoading, setContainersLoading] = useState(false);
  const [error, setError] = useState(null);
  const [containersError, setContainersError] = useState(null);
  const [wsConnected, setWsConnected] = useState(false);
  const [systemResources, setSystemResources] = useState({
    cpu: 0,
    ram: 0,
    ram_used: 0,
    ram_total: 0,
    disk: 0,
    disk_used: 0,
    disk_total: 0,
    storage_size: 0,
  });
  const [resourceHistory, setResourceHistory] = useState({
    cpu: [],
    ram: [],
    disk: [],
    timestamps: []
  });

  // Maximum number of history points to keep (15 minutes at 5-second intervals)
  const MAX_HISTORY_POINTS = 180;

  // Track if initial load is complete
  const initialLoadComplete = useRef(false);

  /**
   * Load all data from API
   */
  const loadData = useCallback(async () => {
    try {
      setIsLoading(true);
      setError(null);

      // Load all data in parallel
      const [routesData, discoveryData, devicesData, activitiesData, statsData] = await Promise.all([
        routesAPI.list().catch(err => {
          console.error('Failed to load routes:', err);
          return [];
        }),
        discoveryAPI.listProposals().catch(err => {
          console.error('Failed to load discoveries:', err);
          return [];
        }),
        devicesAPI.list().catch(err => {
          console.error('Failed to load devices:', err);
          return [];
        }),
        activityAPI.getRecent({ limit: 50 }).catch(err => {
          console.error('Failed to load activities:', err);
          return [];
        }),
        statsAPI.get().catch(err => {
          console.error('Failed to load stats:', err);
          return null;
        }),
      ]);

      // Update state
      setRoutes(routesData || []);
      setDiscoveries(discoveryData || []);
      setDevices(devicesData || []);

      // Handle activities response (could be array or paginated object)
      if (Array.isArray(activitiesData)) {
        setActivities(activitiesData);
      } else if (activitiesData && activitiesData.activities) {
        setActivities(activitiesData.activities);
      } else {
        setActivities([]);
      }

      // Update stats if available
      if (statsData) {
        setStats(statsData);
      }

      initialLoadComplete.current = true;
    } catch (err) {
      console.error('Failed to load data:', err);
      setError(err.message || 'Failed to load data');
    } finally {
      setIsLoading(false);
    }
  }, []);

  /**
   * Refresh specific data type
   */
  const refreshRoutes = useCallback(async () => {
    try {
      const data = await routesAPI.list();
      setRoutes(data || []);
    } catch (err) {
      console.error('Failed to refresh routes:', err);
    }
  }, []);

  const refreshDiscoveries = useCallback(async () => {
    try {
      const data = await discoveryAPI.listProposals();
      setDiscoveries(data || []);
    } catch (err) {
      console.error('Failed to refresh discoveries:', err);
    }
  }, []);

  const refreshDevices = useCallback(async () => {
    try {
      const data = await devicesAPI.list();
      setDevices(data || []);
    } catch (err) {
      console.error('Failed to refresh devices:', err);
    }
  }, []);

  const refreshActivities = useCallback(async () => {
    try {
      const data = await activityAPI.getRecent({ limit: 50 });
      if (Array.isArray(data)) {
        setActivities(data);
      } else if (data && data.activities) {
        setActivities(data.activities);
      }
    } catch (err) {
      console.error('Failed to refresh activities:', err);
    }
  }, []);

  const refreshStats = useCallback(async () => {
    try {
      const data = await statsAPI.get();
      if (data) {
        setStats(data);
      }
    } catch (err) {
      console.error('Failed to refresh stats:', err);
    }
  }, []);

  const refreshSystemResources = useCallback(async () => {
    try {
      const data = await systemAPI.getResources();
      if (data) {
        setSystemResources(data);

        // Accumulate history with rolling window
        setResourceHistory(prev => {
          const now = Date.now();
          return {
            cpu: [...prev.cpu, data.cpu].slice(-MAX_HISTORY_POINTS),
            ram: [...prev.ram, data.ram].slice(-MAX_HISTORY_POINTS),
            disk: [...prev.disk, data.disk].slice(-MAX_HISTORY_POINTS),
            timestamps: [...prev.timestamps, now].slice(-MAX_HISTORY_POINTS)
          };
        });
      }
    } catch (err) {
      console.error('Failed to refresh system resources:', err);
    }
  }, []);

  const refreshContainers = useCallback(async () => {
    try {
      setContainersLoading(true);
      setContainersError(null);
      const data = await containersAPI.list();

      // Fetch stats for running containers
      const containersWithStats = await Promise.all(
        (data || []).map(async (container) => {
          if (container.state === 'running') {
            try {
              const stats = await containersAPI.stats(container.id);
              return { ...container, stats };
            } catch (err) {
              console.warn(`Failed to fetch stats for container ${container.id}:`, err);
              return container;
            }
          }
          return container;
        })
      );

      setContainers(containersWithStats);
    } catch (err) {
      console.error('Failed to refresh containers:', err);
      setContainersError(err.message || 'Failed to load containers');
    } finally {
      setContainersLoading(false);
    }
  }, []);

  /**
   * Initialize data on mount
   */
  useEffect(() => {
    loadData();
  }, [loadData]);

  /**
   * Load containers separately (optional, not part of critical data)
   * Only run on mount - manual refresh via button
   */
  useEffect(() => {
    refreshContainers();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  /**
   * Poll system resources every 5 seconds
   */
  useEffect(() => {
    // Initial fetch
    refreshSystemResources();

    // Set up 5-second polling interval
    const intervalId = setInterval(() => {
      refreshSystemResources();
    }, 5000);

    // Cleanup on unmount
    return () => {
      clearInterval(intervalId);
    };
  }, [refreshSystemResources]);

  /**
   * Poll devices every 30 seconds to keep request counts and status fresh
   */
  useEffect(() => {
    const intervalId = setInterval(() => {
      refreshDevices();
    }, 30000);

    return () => {
      clearInterval(intervalId);
    };
  }, [refreshDevices]);

  /**
   * Setup WebSocket connection and listeners
   */
  useEffect(() => {
    // Connection state change handler
    websocketService.onConnectionChange = ({ connected, authenticated }) => {
      setWsConnected(connected && authenticated);
    };

    // Connect to WebSocket
    websocketService.connect();

    // Register event listeners for real-time updates
    websocketService.on(WS_MSG_TYPES.DISCOVERY, () => {
      console.log('WebSocket: New discovery detected');
      refreshDiscoveries();
      refreshStats();
    });

    websocketService.on(WS_MSG_TYPES.CONFIG_RELOAD, () => {
      console.log('WebSocket: Config reloaded');
      refreshRoutes();
      refreshStats();
    });

    websocketService.on(WS_MSG_TYPES.DEVICE_PAIRED, () => {
      console.log('WebSocket: Device paired');
      refreshDevices();
      refreshActivities();
      refreshStats();
    });

    websocketService.on(WS_MSG_TYPES.DEVICE_REVOKED, () => {
      console.log('WebSocket: Device revoked');
      refreshDevices();
      refreshActivities();
      refreshStats();
    });

    websocketService.on(WS_MSG_TYPES.DEVICE_STATUS, () => {
      console.log('WebSocket: Device status changed');
      refreshDevices();
    });

    websocketService.on(WS_MSG_TYPES.APP_REGISTERED, () => {
      console.log('WebSocket: App registered');
      refreshRoutes();
      refreshDiscoveries(); // Refresh discoveries to remove the approved proposal
      refreshActivities();
      refreshStats();
    });

    websocketService.on(WS_MSG_TYPES.PROPOSAL_DISMISSED, () => {
      console.log('WebSocket: Proposal dismissed');
      refreshDiscoveries();
      refreshActivities();
      refreshStats();
    });

    websocketService.on(WS_MSG_TYPES.HEALTH_CHANGE, () => {
      console.log('WebSocket: Health status changed');
      refreshRoutes();
    });

    websocketService.on(WS_MSG_TYPES.WEBHOOK, (data) => {
      console.log('WebSocket: Webhook activity received', data);
      // Add webhook activity to activities list in real-time
      if (data && data.type === 'webhook.activity') {
        setActivities(prev => [data, ...prev]);

        // Create notification in bell dropdown
        // Map iconClass to notification severity
        let severity = 'info';
        if (data.iconClass === 'success') {
          severity = 'success';
        } else if (data.iconClass === 'warning') {
          severity = 'warning';
        } else if (data.iconClass === 'danger') {
          severity = 'error';
        }

        addNotification({
          severity,
          message: data.message,
          strongText: data.details ? 'Webhook Activity' : undefined,
        });
      }
      // Also refresh to ensure consistency with backend
      refreshActivities();
    });

    // Cleanup on unmount
    return () => {
      websocketService.disconnect();
      websocketService.onConnectionChange = null;
    };
  }, [refreshRoutes, refreshDiscoveries, refreshDevices, refreshActivities, refreshStats, addNotification]);

  // ==================== ROUTE CRUD ====================

  /**
   * Delete a route
   */
  const deleteRoute = useCallback(async (id) => {
    try {
      await routesAPI.delete(id);
      // Update local state optimistically
      setRoutes(prev => prev.filter(r => r.routeId !== id));
      // Refresh to get updated data
      await refreshRoutes();
      await refreshStats();
      return true;
    } catch (err) {
      console.error('Failed to delete route:', err);
      throw err;
    }
  }, [refreshRoutes, refreshStats]);

  /**
   * Update an existing route
   */
  const updateRoute = useCallback(async (id, updates) => {
    try {
      const updatedRoute = await routesAPI.update(id, updates);
      // Update local state optimistically
      setRoutes(prev => prev.map(r => (r.routeId === id ? updatedRoute : r)));
      return updatedRoute;
    } catch (err) {
      console.error('Failed to update route:', err);
      throw err;
    }
  }, []);

  /**
   * Get a single route by ID
   */
  const getRoute = useCallback(
    (id) => {
      return routes.find(route => route.routeId === id);
    },
    [routes]
  );

  /**
   * Search and filter routes
   */
  const searchRoutes = useCallback(
    (query) => {
      if (!query || query.trim() === '') {
        return routes;
      }

      const lowercaseQuery = query.toLowerCase();

      return routes.filter(route => {
        return (
          route.appId?.toLowerCase().includes(lowercaseQuery) ||
          route.pathBase?.toLowerCase().includes(lowercaseQuery) ||
          route.to?.toLowerCase().includes(lowercaseQuery) ||
          route.scopes?.some(scope => scope.toLowerCase().includes(lowercaseQuery))
        );
      });
    },
    [routes]
  );

  // ==================== DISCOVERY OPERATIONS ====================

  /**
   * Approve a discovery (convert to route)
   * @param {string} id - Discovery ID
   * @param {Object} options - Optional parameters
   * @param {number} options.port - Optional port to use from availablePorts
   */
  const approveDiscovery = useCallback(
    async (id, options = {}) => {
      try {
        const result = await discoveryAPI.approveProposal(id, options);
        // Remove from local discoveries
        setDiscoveries(prev => prev.filter(d => d.id !== id));
        // Refresh routes and stats
        await refreshRoutes();
        await refreshActivities();
        await refreshStats();
        return result;
      } catch (err) {
        console.error('Failed to approve discovery:', err);
        throw err;
      }
    },
    [refreshRoutes, refreshActivities, refreshStats]
  );

  /**
   * Reject a discovery
   */
  const rejectDiscovery = useCallback(
    async (id) => {
      try {
        const result = await discoveryAPI.dismissProposal(id);
        // Remove from local discoveries
        setDiscoveries(prev => prev.filter(d => d.id !== id));
        // Refresh activities and stats
        await refreshActivities();
        await refreshStats();
        return result;
      } catch (err) {
        console.error('Failed to reject discovery:', err);
        throw err;
      }
    },
    [refreshActivities, refreshStats]
  );

  /**
   * Bulk approve discoveries
   */
  const bulkApproveDiscoveries = useCallback(
    async (ids) => {
      try {
        // Approve all in parallel
        await Promise.all(ids.map(id => discoveryAPI.approveProposal(id)));
        // Remove from local discoveries
        setDiscoveries(prev => prev.filter(d => !ids.includes(d.id)));
        // Refresh routes and stats
        await refreshRoutes();
        await refreshActivities();
        await refreshStats();
        return ids;
      } catch (err) {
        console.error('Failed to bulk approve discoveries:', err);
        throw err;
      }
    },
    [refreshRoutes, refreshActivities, refreshStats]
  );

  /**
   * Bulk reject discoveries
   */
  const bulkRejectDiscoveries = useCallback(
    async (ids) => {
      try {
        // Reject all in parallel
        await Promise.all(ids.map(id => discoveryAPI.dismissProposal(id)));
        // Remove from local discoveries
        setDiscoveries(prev => prev.filter(d => !ids.includes(d.id)));
        // Refresh activities and stats
        await refreshActivities();
        await refreshStats();
        return ids;
      } catch (err) {
        console.error('Failed to bulk reject discoveries:', err);
        throw err;
      }
    },
    [refreshActivities, refreshStats]
  );

  /**
   * Trigger rediscovery scan
   * Clears dismissed and active proposals to allow fresh discovery
   */
  const rediscover = useCallback(
    async () => {
      try {
        const result = await discoveryAPI.rediscover();
        // Refresh discoveries immediately to show cleared state
        await refreshDiscoveries();
        // Refresh activities and stats
        await refreshActivities();
        await refreshStats();
        return result;
      } catch (err) {
        console.error('Failed to trigger rediscovery:', err);
        throw err;
      }
    },
    [refreshDiscoveries, refreshActivities, refreshStats]
  );

  // ==================== DEVICE OPERATIONS ====================

  /**
   * Revoke a device (remove device access)
   */
  const revokeDevice = useCallback(
    async (id) => {
      try {
        await devicesAPI.revoke(id);
        // Remove from local devices
        setDevices(prev => prev.filter(d => d.id !== id));
        // Refresh activities and stats
        await refreshActivities();
        await refreshStats();
        return true;
      } catch (err) {
        console.error('Failed to revoke device:', err);
        throw err;
      }
    },
    [refreshActivities, refreshStats]
  );

  /**
   * Pair a new device (add device with access token)
   * Note: This is typically done via QR code pairing flow, not directly
   */
  const pairDevice = useCallback(
    async (deviceData) => {
      try {
        // In the real implementation, pairing happens via /api/v1/auth/pair endpoint
        // This function is here for compatibility but should rarely be called directly
        console.warn('pairDevice called - pairing should happen via QR code flow');

        // Refresh devices to pick up any new devices
        await refreshDevices();
        await refreshActivities();
        await refreshStats();

        return null;
      } catch (err) {
        console.error('Failed to pair device:', err);
        throw err;
      }
    },
    [refreshDevices, refreshActivities, refreshStats]
  );

  // ==================== CONTAINER OPERATIONS ====================

  /**
   * Start a container
   */
  const startContainer = useCallback(
    async (containerId) => {
      try {
        await containersAPI.start(containerId);
        // Update local state optimistically
        setContainers(prev =>
          prev.map(c => (c.id === containerId ? { ...c, state: 'running' } : c))
        );
        // Refresh to get updated data
        await refreshContainers();
        await refreshActivities();
        // Refresh routes to update health status
        await refreshRoutes();
        return true;
      } catch (err) {
        console.error('Failed to start container:', err);
        throw err;
      }
    },
    [refreshContainers, refreshActivities, refreshRoutes]
  );

  /**
   * Stop a container
   */
  const stopContainer = useCallback(
    async (containerId, timeout = 10) => {
      try {
        await containersAPI.stop(containerId, timeout);
        // Update local state optimistically
        setContainers(prev =>
          prev.map(c => (c.id === containerId ? { ...c, state: 'exited', stats: null } : c))
        );
        // Refresh to get updated data
        await refreshContainers();
        await refreshActivities();
        // Refresh routes to update health status
        await refreshRoutes();
        return true;
      } catch (err) {
        console.error('Failed to stop container:', err);
        throw err;
      }
    },
    [refreshContainers, refreshActivities, refreshRoutes]
  );

  /**
   * Restart a container
   */
  const restartContainer = useCallback(
    async (containerId) => {
      try {
        await containersAPI.restart(containerId);
        // Update local state optimistically
        setContainers(prev =>
          prev.map(c => (c.id === containerId ? { ...c, state: 'restarting' } : c))
        );
        // Refresh to get updated data after a short delay
        setTimeout(() => {
          refreshContainers();
          refreshRoutes(); // Also refresh routes after restart
        }, 1000);
        await refreshActivities();
        return true;
      } catch (err) {
        console.error('Failed to restart container:', err);
        throw err;
      }
    },
    [refreshContainers, refreshActivities, refreshRoutes]
  );

  // ==================== ACTIVITY OPERATIONS ====================

  /**
   * Get recent activities (limited)
   */
  const getRecentActivities = useCallback(
    (limit = 10) => {
      return activities.slice(0, limit);
    },
    [activities]
  );

  /**
   * Get activities by type
   */
  const getActivitiesByType = useCallback(
    (type) => {
      return activities.filter(activity => activity.type === type);
    },
    [activities]
  );

  /**
   * Format activity timestamp for display
   */
  const formatActivityTime = useCallback((timestamp) => {
    return getRelativeTime(timestamp);
  }, []);

  // ==================== STATISTICS ====================

  /**
   * Get route statistics
   */
  const getRouteStats = useCallback(() => {
    const total = routes.length;
    const active = routes.filter(r => r.status === 'ACTIVE' || !r.status).length;
    const byScope = routes.reduce((acc, route) => {
      if (route.scopes) {
        route.scopes.forEach(scope => {
          acc[scope] = (acc[scope] || 0) + 1;
        });
      }
      return acc;
    }, {});

    return {
      total,
      active,
      byScope,
    };
  }, [routes]);

  // Context value
  const value = {
    // State
    routes,
    discoveries,
    devices,
    activities,
    containers,
    stats,
    systemResources,
    resourceHistory,
    isLoading,
    containersLoading,
    error,
    containersError,
    wsConnected,

    // Refresh functions
    refreshRoutes,
    refreshDiscoveries,
    refreshDevices,
    refreshActivities,
    refreshStats,
    refreshSystemResources,
    refreshContainers,
    loadData,

    // Route CRUD
    updateRoute,
    deleteRoute,
    getRoute,
    searchRoutes,

    // Discovery operations
    approveDiscovery,
    rejectDiscovery,
    bulkApproveDiscoveries,
    bulkRejectDiscoveries,
    rediscover,

    // Device operations
    revokeDevice,
    pairDevice,

    // Container operations
    startContainer,
    stopContainer,
    restartContainer,

    // Activity operations
    getRecentActivities,
    getActivitiesByType,
    formatActivityTime,

    // Statistics
    getRouteStats,
  };

  return <DataContext.Provider value={value}>{children}</DataContext.Provider>;
}

DataProvider.propTypes = {
  children: PropTypes.node.isRequired,
};

/**
 * useData Hook
 *
 * Access data context in components
 *
 * @returns {object} Data context value
 * @throws {Error} If used outside DataProvider
 *
 * @example
 * const { routes, updateRoute, deleteRoute } = useData();
 */
export function useData() {
  const context = useContext(DataContext);

  if (!context) {
    throw new Error('useData must be used within a DataProvider');
  }

  return context;
}

export default DataContext;
