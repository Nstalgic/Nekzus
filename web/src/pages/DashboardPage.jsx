import { useState, useMemo, useEffect } from 'react';
import { Box, ThreeColumnGrid } from '../components/boxes';
import { ServerResourcesPanel } from '../components/charts';
import { Tabs, TabContent } from '../components/navigation';
import {
  OverviewList,
  ActivityList,
  HealthList,
  ProgressBar,
} from '../components/data-display';
import { RoutesTab, DiscoveryTab, DevicesTab, SettingsTab, ContainersTab, ToolboxTab, ScriptsTab, MetricsTab, NotificationsTab, FederationTab } from './tabs';
import { useData, useSettings } from '../contexts';
import { useDebug } from '../utils/debug';

/**
 * DashboardPage Component
 *
 * Complete working dashboard demonstrating all terminal UI components.
 * Features three-column overview section, tabbed management console,
 * and comprehensive system monitoring.
 *
 * Layout structure:
 * 1. Three-column overview (metrics, activity, health)
 * 2. Management console with tabs (routes, discovery, devices, containers, toolbox, scripts, settings)
 *
 * @component
 * @returns {JSX.Element} Complete dashboard page
 *
 * @example
 * <DashboardPage />
 */
function DashboardPage() {
  const [activeTab, setActiveTab] = useState('routes');
  const { routes, discoveries, devices, activities, containers, stats, systemResources, error, wsConnected } = useData();
  const { settings } = useSettings();
  const dbg = useDebug('DashboardPage');

  // Debug: Log component mount/unmount
  useEffect(() => {
    dbg.mount();
    return () => dbg.unmount();
  }, []);

  // Debug: Log data changes
  useEffect(() => {
    dbg.log('Data updated', {
      routes: routes.length,
      discoveries: discoveries.length,
      devices: devices.length,
      containers: containers.length,
      wsConnected,
    });
  }, [routes.length, discoveries.length, devices.length, containers.length, wsConnected]);

  // Wrap setActiveTab to add debug logging
  const handleTabChange = (tabId) => {
    dbg.log('Tab changed', { from: activeTab, to: tabId });
    setActiveTab(tabId);
  };

  // Overview metrics with real data
  const overviewMetrics = [
    {
      id: 'routes',
      label: 'ACTIVE ROUTES',
      value: String(routes.filter(r => r.status === 'ACTIVE').length),
      link: true,
      onClick: () => handleTabChange('routes'),
    },
    {
      id: 'devices',
      label: 'PAIRED DEVICES',
      value: String(devices.length),
      link: true,
      onClick: () => handleTabChange('devices'),
    },
    {
      id: 'discoveries',
      label: 'PENDING DISCOVERIES',
      value: String(discoveries.length),
      link: true,
      urgent: discoveries.length > 0,
      onClick: () => handleTabChange('discovery'),
    },
    {
      id: 'requests',
      label: 'TOTAL REQUESTS',
      value: String(stats?.requests?.value || 0),
    },
  ];

  // Format activities for display (showing last 6)
  const recentActivities = activities.slice(0, 6).map(activity => {
    const now = new Date();
    const diff = now - activity.timestamp;
    const minutes = Math.floor(diff / 60000);
    const hours = Math.floor(diff / 3600000);
    const days = Math.floor(diff / 86400000);

    let time;
    if (minutes < 1) time = 'just now';
    else if (minutes < 60) time = `${minutes}m ago`;
    else if (hours < 24) time = `${hours}h ago`;
    else time = `${days}d ago`;

    return {
      id: activity.id,
      text: activity.message,
      time: time,
    };
  });

  // Format bytes to human-readable format
  const formatBytes = (bytes) => {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
  };

  // Format bytes with consistent unit for used/total display
  const formatBytesWithUnit = (used, total) => {
    if (total === 0) return '0/0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    // Use the unit based on total size
    const i = Math.floor(Math.log(total) / Math.log(k));
    const usedFormatted = parseFloat((used / Math.pow(k, i)).toFixed(1));
    const totalFormatted = parseFloat((total / Math.pow(k, i)).toFixed(1));
    return `${usedFormatted}/${totalFormatted} ${sizes[i]}`;
  };

  // Real-time health status data from API
  const healthItems = useMemo(() => {
    const items = [];

    // STORAGE SIZE - Database file size
    items.push({
      id: 'storage',
      label: 'DATABASE',
      badge: {
        variant: 'info',
        filled: true,
        children: formatBytes(systemResources.storage_size),
      },
    });

    // AUTHENTICATION - Based on API connectivity
    items.push({
      id: 'auth',
      label: 'AUTHENTICATION',
      badge: {
        variant: error ? 'error' : 'success',
        dot: true,
        filled: true,
        children: error ? 'OFFLINE' : 'ONLINE',
      },
    });

    // DISCOVERY - Number of pending proposals
    const discoveryCount = discoveries.length;
    items.push({
      id: 'discovery',
      label: 'DISCOVERY',
      badge: {
        variant: discoveryCount > 0 ? 'warning' : 'info',
        filled: true,
        children: discoveryCount > 0 ? `${discoveryCount} PENDING` : 'IDLE',
      },
    });

    // CONNECTIONS - WebSocket connection status
    items.push({
      id: 'connections',
      label: 'WEBSOCKET',
      badge: {
        variant: wsConnected ? 'success' : 'error',
        dot: true,
        filled: true,
        children: wsConnected ? 'CONNECTED' : 'DISCONNECTED',
      },
    });

    // ROUTES - Number of active routes
    const routesCount = routes.length;
    items.push({
      id: 'routes',
      label: 'ROUTES',
      badge: {
        variant: routesCount > 0 ? 'success' : 'warning',
        filled: true,
        children: `${routesCount} ACTIVE`,
      },
    });

    // DEVICES - Number of paired devices
    const devicesCount = devices.length;
    items.push({
      id: 'devices',
      label: 'DEVICES',
      badge: {
        variant: devicesCount > 0 ? 'success' : 'info',
        filled: true,
        children: `${devicesCount} PAIRED`,
      },
    });

    // FEDERATION - Show if federation is enabled
    if (settings.enableFederation) {
      items.push({
        id: 'federation',
        label: 'FEDERATION',
        badge: {
          variant: 'info',
          filled: true,
          children: 'ENABLED',
        },
        link: true,
        onClick: () => handleTabChange('federation'),
      });
    }

    return items;
  }, [error, discoveries.length, wsConnected, routes.length, devices.length, systemResources, settings.enableFederation]);

  // Tab configuration (conditionally include Toolbox based on settings)
  const tabs = useMemo(() => {
    const baseTabs = [
      { id: 'routes', label: 'ROUTES' },
      {
        id: 'discovery',
        label: 'DISCOVERY',
        badge: discoveries.length > 0 ? discoveries.length : undefined,
        badgeSeverity: discoveries.length > 0 ? 'warning' : undefined
      },
      { id: 'devices', label: 'DEVICES' },
      { id: 'containers', label: 'CONTAINERS' },
    ];

    // Add Toolbox tab if enabled in settings
    if (settings.enableToolbox) {
      baseTabs.push({ id: 'toolbox', label: 'TOOLBOX' });
    }

    // Add Scripts tab if enabled in settings
    if (settings.enableScripts) {
      baseTabs.push({ id: 'scripts', label: 'SCRIPTS' });
    }

    // Add Metrics tab
    baseTabs.push({ id: 'metrics', label: 'METRICS' });

    // Add Notifications tab
    baseTabs.push({ id: 'notifications', label: 'NOTIFICATIONS' });

    // Add Federation tab if enabled
    if (settings.enableFederation) {
      baseTabs.push({ id: 'federation', label: 'FEDERATION' });
    }

    // Settings tab is always last
    baseTabs.push({ id: 'settings', label: 'SETTINGS' });

    return baseTabs;
  }, [discoveries.length, settings.enableToolbox, settings.enableScripts, settings.enableFederation]);

  return (
    <>
      {/* Three-column overview section */}
      <section className="component-section">
        <ThreeColumnGrid>
          {/* Column 1: Overview with ASCII logo */}
          <Box title="OVERVIEW">
            <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
              <OverviewList metrics={overviewMetrics} />

            </div>
          </Box>

          {/* Column 2: Recent Activity */}
          <Box title="RECENT ACTIVITY">
            <ActivityList activities={recentActivities} />
          </Box>

          {/* Column 3: System Health */}
          <Box title="SYSTEM HEALTH">
            {/* Resource metrics row - CPU, Memory, Disk side by side */}
            <div className="resource-metrics-row">
              <div className="resource-metric">
                <span className="resource-label">CPU {Math.round(systemResources.cpu)}%</span>
                <ProgressBar
                  current={Math.round(systemResources.cpu)}
                  max={100}
                  showPercentage={false}
                  blocks={15}
                />
              </div>
              <div className="resource-metric">
                <span className="resource-label">
                  MEM {formatBytesWithUnit(systemResources.ram_used, systemResources.ram_total)}
                </span>
                <ProgressBar
                  current={Math.round(systemResources.ram)}
                  max={100}
                  showPercentage={false}
                  blocks={15}
                />
              </div>
              <div className="resource-metric">
                <span className="resource-label">
                  DISK {formatBytesWithUnit(systemResources.disk_used, systemResources.disk_total)}
                </span>
                <ProgressBar
                  current={Math.round(systemResources.disk)}
                  max={100}
                  showPercentage={false}
                  blocks={15}
                />
              </div>
            </div>
            <HealthList items={healthItems} />
          </Box>
        </ThreeColumnGrid>
      </section>

      {/* Server Resources Line Graphs */}
      <ServerResourcesPanel />

      {/* Management Console with Tabs */}
      <section className="component-section">
        <Box title="MANAGEMENT CONSOLE">
          <Tabs
            tabs={tabs}
            activeTab={activeTab}
            onChange={handleTabChange}
            aria-label="Management console navigation"
          />

          {/* Tab Content Panels */}
          <TabContent id="routes" active={activeTab === 'routes'}>
            <RoutesTab />
          </TabContent>

          <TabContent id="discovery" active={activeTab === 'discovery'}>
            <DiscoveryTab />
          </TabContent>

          <TabContent id="devices" active={activeTab === 'devices'}>
            <DevicesTab />
          </TabContent>

          <TabContent id="containers" active={activeTab === 'containers'}>
            <ContainersTab />
          </TabContent>

          {settings.enableToolbox && (
            <TabContent id="toolbox" active={activeTab === 'toolbox'}>
              <ToolboxTab />
            </TabContent>
          )}

          {settings.enableScripts && (
            <TabContent id="scripts" active={activeTab === 'scripts'}>
              <ScriptsTab />
            </TabContent>
          )}

          <TabContent id="metrics" active={activeTab === 'metrics'}>
            <MetricsTab />
          </TabContent>

          <TabContent id="notifications" active={activeTab === 'notifications'}>
            <NotificationsTab />
          </TabContent>

          {settings.enableFederation && (
            <TabContent id="federation" active={activeTab === 'federation'}>
              <FederationTab />
            </TabContent>
          )}

          <TabContent id="settings" active={activeTab === 'settings'}>
            <SettingsTab />
          </TabContent>
        </Box>
      </section>
    </>
  );
}

export default DashboardPage;
