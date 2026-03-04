/**
 * ContainersTab Component
 *
 * Container management interface for Docker and Kubernetes
 *
 * Features:
 * - Grid of ContainerCard components
 * - Container actions (start, stop, restart)
 * - View detailed container information (inspect)
 * - View container logs
 * - Real-time container stats
 * - Refresh containers list
 * - Filter by state (running/stopped/all)
 * - Filter by runtime (all/docker/kubernetes)
 * - Multi-select for batch export
 */

import { useState, useMemo } from 'react';
import { RefreshCw, CheckSquare, Square } from 'lucide-react';
import { ContainerCard } from '../../components/cards/ContainerCard';
import {
  ContainerDetailsModal,
  ContainerLogsModal,
  ContainerExportModal,
  BatchExportModal,
  ConfirmationModal
} from '../../components/modals';
import { useData } from '../../contexts';
import { useSettings } from '../../contexts/SettingsContext';

/**
 * Sanitize a string to match backend appId format.
 * Replaces non-alphanumeric characters with underscores.
 * This mirrors the Go sanitize() function in internal/discovery/discovery.go
 */
function sanitizeAppId(str) {
  if (!str) return '';
  return str.replace(/[^a-zA-Z0-9]/g, '_').toLowerCase();
}

/**
 * ContainersTab Component
 */
export function ContainersTab() {
  const {
    containers,
    containersLoading,
    containersError,
    startContainer,
    stopContainer,
    restartContainer,
    refreshContainers,
    routes
  } = useData();
  const { settings } = useSettings();

  const [selectedContainer, setSelectedContainer] = useState(null);
  const [detailsModalOpen, setDetailsModalOpen] = useState(false);
  const [logsModalOpen, setLogsModalOpen] = useState(false);
  const [exportModalOpen, setExportModalOpen] = useState(false);
  const [batchExportModalOpen, setBatchExportModalOpen] = useState(false);
  const [confirmModalOpen, setConfirmModalOpen] = useState(false);
  const [actionType, setActionType] = useState(null);
  const [filterState, setFilterState] = useState('all'); // all, running, stopped
  const [isRefreshing, setIsRefreshing] = useState(false);
  const [filterRuntime, setFilterRuntime] = useState('all'); // all, docker, kubernetes

  // Multi-select state
  const [selectionMode, setSelectionMode] = useState(false);
  const [selectedContainers, setSelectedContainers] = useState(new Set());

  // Calculate container stats
  const runningContainers = containers.filter(c => c.state === 'running').length;
  const stoppedContainers = containers.filter(c => c.state === 'exited' || c.state === 'stopped').length;
  const dockerContainers = containers.filter(c => !c.runtime || c.runtime === 'docker').length;
  const k8sContainers = containers.filter(c => c.runtime === 'kubernetes').length;
  const hasMultipleRuntimes = dockerContainers > 0 && k8sContainers > 0;

  // Build lookup sets for route matching
  // Primary: container IDs from routes (most reliable)
  // Secondary: appIds for routes without containerId
  const { routedContainerIds, routedAppIds } = useMemo(() => {
    const containerIds = new Set();
    const appIds = new Set();

    routes.forEach(route => {
      if (route.containerId) {
        // Container IDs can be full (64 char) or short (12 char)
        // Store both the full ID and short version for matching
        containerIds.add(route.containerId.toLowerCase());
      }
      if (route.appId) {
        appIds.add(route.appId.toLowerCase());
      }
    });

    return { routedContainerIds: containerIds, routedAppIds: appIds };
  }, [routes]);

  // Filter containers by state, runtime, and routes
  const filteredContainers = containers.filter(container => {
    // State filter
    if (filterState === 'running' && container.state !== 'running') return false;
    if (filterState === 'stopped' && container.state !== 'exited' && container.state !== 'stopped') return false;

    // Runtime filter
    if (filterRuntime === 'docker' && container.runtime === 'kubernetes') return false;
    if (filterRuntime === 'kubernetes' && container.runtime !== 'kubernetes') return false;

    // Route filter - only apply when showOnlyRoutedContainers is enabled
    if (settings.showOnlyRoutedContainers) {
      const containerId = (container.id || '').toLowerCase();

      // Primary: match by container ID (most reliable)
      // Check if route's containerId matches container's full or short ID
      const hasMatchingContainerId = Array.from(routedContainerIds).some(routeContainerId =>
        containerId.startsWith(routeContainerId) || routeContainerId.startsWith(containerId)
      );

      if (hasMatchingContainerId) {
        return true;
      }

      // Secondary: check nekzus.app.id label
      const labelAppId = container.labels?.['nekzus.app.id'];
      if (labelAppId && routedAppIds.has(labelAppId.toLowerCase())) {
        return true;
      }

      // Tertiary: sanitize container name to match appId
      const containerName = (container.name || '').replace(/^\//, '');
      const sanitizedName = sanitizeAppId(containerName);
      if (routedAppIds.has(sanitizedName)) {
        return true;
      }

      // No match found - filter out this container
      return false;
    }

    return true;
  });

  // Handle refresh
  const handleRefresh = async () => {
    setIsRefreshing(true);
    try {
      await refreshContainers();
    } finally {
      // Small delay for visual feedback
      setTimeout(() => setIsRefreshing(false), 500);
    }
  };

  // Handle inspect container
  const handleInspect = (container) => {
    setSelectedContainer(container);
    setDetailsModalOpen(true);
  };

  // Handle view logs
  const handleLogs = (container) => {
    setSelectedContainer(container);
    setLogsModalOpen(true);
  };

  // Handle export container
  const handleExport = (container) => {
    setSelectedContainer(container);
    setExportModalOpen(true);
  };

  // Export container config to compose format
  const exportContainerConfig = async (containerId, options) => {
    console.log('[Export] Starting export for container:', containerId, 'with options:', options);

    const response = await fetch(`/api/v1/containers/${containerId}/export`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json'
      },
      body: JSON.stringify(options)
    });

    console.log('[Export] Response status:', response.status, response.statusText);

    if (!response.ok) {
      const text = await response.text();
      console.error('[Export] Error response:', text);
      let error;
      try {
        error = JSON.parse(text);
      } catch {
        throw new Error(text || 'Failed to export container');
      }
      throw new Error(error.message || 'Failed to export container');
    }

    const data = await response.json();
    console.log('[Export] Success, filename:', data.filename, 'warnings:', data.warnings?.length || 0);
    return data;
  };

  // Preview container export (generates YAML without downloading)
  const previewContainerExport = async (containerId, options) => {
    console.log('[Preview] Starting preview for container:', containerId, 'with options:', options);

    const response = await fetch(`/api/v1/containers/${containerId}/export/preview`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json'
      },
      body: JSON.stringify(options)
    });

    console.log('[Preview] Response status:', response.status, response.statusText);

    if (!response.ok) {
      const text = await response.text();
      console.error('[Preview] Error response:', text);
      let error;
      try {
        error = JSON.parse(text);
      } catch {
        throw new Error(text || 'Failed to generate preview');
      }
      throw new Error(error.message || 'Failed to generate preview');
    }

    const data = await response.json();
    console.log('[Preview] Success');
    return data;
  };

  // Batch export containers config to compose format
  const batchExportContainerConfig = async (containerIds, options) => {
    console.log('[BatchExport] Starting batch export for containers:', containerIds, 'with options:', options);

    // Build URL with format query parameter if ZIP format requested
    const { format, ...bodyOptions } = options;
    let url = '/api/v1/containers/batch/export';
    if (format === 'zip') {
      url += '?format=zip';
    }

    const response = await fetch(url, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({
        container_ids: containerIds,
        ...bodyOptions
      })
    });

    console.log('[BatchExport] Response status:', response.status, response.statusText);

    if (!response.ok && response.status !== 206) {
      const text = await response.text();
      console.error('[BatchExport] Error response:', text);
      let error;
      try {
        error = JSON.parse(text);
      } catch {
        throw new Error(text || 'Failed to batch export containers');
      }
      throw new Error(error.message || 'Failed to batch export containers');
    }

    // Return blob for ZIP format, JSON for default
    if (format === 'zip') {
      const blob = await response.blob();
      console.log('[BatchExport] ZIP success, size:', blob.size);
      return blob;
    }

    const data = await response.json();
    console.log('[BatchExport] Success, filename:', data.filename, 'warnings:', data.warnings?.length || 0);
    return data;
  };

  // Preview batch export (generates YAML without downloading)
  const previewBatchExport = async (containerIds, options) => {
    console.log('[BatchPreview] Starting preview for containers:', containerIds, 'with options:', options);

    const response = await fetch('/api/v1/containers/batch/export/preview', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({
        container_ids: containerIds,
        ...options
      })
    });

    console.log('[BatchPreview] Response status:', response.status, response.statusText);

    if (!response.ok && response.status !== 206) {
      const text = await response.text();
      console.error('[BatchPreview] Error response:', text);
      let error;
      try {
        error = JSON.parse(text);
      } catch {
        throw new Error(text || 'Failed to generate batch preview');
      }
      throw new Error(error.message || 'Failed to generate batch preview');
    }

    const data = await response.json();
    console.log('[BatchPreview] Success');
    return data;
  };

  // Toggle selection mode
  const toggleSelectionMode = () => {
    if (selectionMode) {
      // Exiting selection mode - clear selections
      setSelectedContainers(new Set());
    }
    setSelectionMode(!selectionMode);
  };

  // Handle container selection
  const handleContainerSelect = (container, isSelected) => {
    setSelectedContainers(prev => {
      const next = new Set(prev);
      if (isSelected) {
        next.add(container.id);
      } else {
        next.delete(container.id);
      }
      return next;
    });
  };

  // Select all filtered containers
  const selectAllContainers = () => {
    const allIds = new Set(filteredContainers.map(c => c.id));
    setSelectedContainers(allIds);
  };

  // Clear all selections
  const clearSelection = () => {
    setSelectedContainers(new Set());
  };

  // Open batch export modal
  const handleBatchExport = () => {
    if (selectedContainers.size > 0) {
      setBatchExportModalOpen(true);
    }
  };

  // Get selected container objects for batch export modal
  const getSelectedContainerObjects = () => {
    return containers.filter(c => selectedContainers.has(c.id));
  };

  // Handle start container
  const handleStart = async (container) => {
    if (settings.requireConfirmation) {
      setSelectedContainer(container);
      setActionType('start');
      setConfirmModalOpen(true);
    } else {
      await startContainer(container.id);
    }
  };

  // Handle stop container
  const handleStop = async (container) => {
    if (settings.requireConfirmation) {
      setSelectedContainer(container);
      setActionType('stop');
      setConfirmModalOpen(true);
    } else {
      await stopContainer(container.id);
    }
  };

  // Handle restart container
  const handleRestart = async (container) => {
    if (settings.requireConfirmation) {
      setSelectedContainer(container);
      setActionType('restart');
      setConfirmModalOpen(true);
    } else {
      await restartContainer(container.id);
    }
  };

  // Handle confirm action
  const handleConfirmAction = async () => {
    if (!selectedContainer || !actionType) return;

    try {
      switch (actionType) {
        case 'start':
          await startContainer(selectedContainer.id);
          break;
        case 'stop':
          await stopContainer(selectedContainer.id);
          break;
        case 'restart':
          await restartContainer(selectedContainer.id);
          break;
        default:
          break;
      }
    } finally {
      setConfirmModalOpen(false);
      setSelectedContainer(null);
      setActionType(null);
    }
  };

  // Get confirmation modal content
  const getConfirmationContent = () => {
    if (!selectedContainer || !actionType) return {};

    const displayName = selectedContainer.name?.replace(/^\//, '') || 'Unnamed Container';

    const titles = {
      start: 'Start Container',
      stop: 'Stop Container',
      restart: 'Restart Container'
    };

    const messages = {
      start: `Are you sure you want to start ${displayName}?`,
      stop: `Are you sure you want to stop ${displayName}?`,
      restart: `Are you sure you want to restart ${displayName}?`
    };

    return {
      title: titles[actionType] || 'Confirm Action',
      message: messages[actionType] || 'Are you sure?'
    };
  };

  return (
    <div className="containers-tab">
      {/* Header */}
      <div className="tab-header">
        <div className="tab-header-left">
          <div className="container-stats">
            <span className="stat-item">
              <span className="stat-label">Total:</span>
              <span className="stat-value">{containers.length}</span>
            </span>
            <span className="stat-item">
              <span className="stat-label">Running:</span>
              <span className="stat-value stat-success">{runningContainers}</span>
            </span>
            <span className="stat-item">
              <span className="stat-label">Stopped:</span>
              <span className="stat-value stat-secondary">{stoppedContainers}</span>
            </span>
            {hasMultipleRuntimes && (
              <>
                <span className="stat-divider">|</span>
                <span className="stat-item">
                  <span className="stat-label">Docker:</span>
                  <span className="stat-value">{dockerContainers}</span>
                </span>
                <span className="stat-item">
                  <span className="stat-label">K8s:</span>
                  <span className="stat-value">{k8sContainers}</span>
                </span>
              </>
            )}
          </div>

          {/* Filter Buttons */}
          <div className="filter-buttons">
            <button
              className={`btn btn-sm ${filterState === 'all' ? 'btn-primary' : 'btn-secondary'}`}
              onClick={() => setFilterState('all')}
            >
              ALL
            </button>
            <button
              className={`btn btn-sm ${filterState === 'running' ? 'btn-primary' : 'btn-secondary'}`}
              onClick={() => setFilterState('running')}
            >
              RUNNING
            </button>
            <button
              className={`btn btn-sm ${filterState === 'stopped' ? 'btn-primary' : 'btn-secondary'}`}
              onClick={() => setFilterState('stopped')}
            >
              STOPPED
            </button>
          </div>

          {/* Runtime Filter Buttons (only show when multiple runtimes exist) */}
          {hasMultipleRuntimes && (
            <div className="filter-buttons runtime-filter">
              <button
                className={`btn btn-sm ${filterRuntime === 'all' ? 'btn-primary' : 'btn-secondary'}`}
                onClick={() => setFilterRuntime('all')}
              >
                ALL RUNTIMES
              </button>
              <button
                className={`btn btn-sm ${filterRuntime === 'docker' ? 'btn-primary' : 'btn-secondary'}`}
                onClick={() => setFilterRuntime('docker')}
              >
                DOCKER
              </button>
              <button
                className={`btn btn-sm ${filterRuntime === 'kubernetes' ? 'btn-primary' : 'btn-secondary'}`}
                onClick={() => setFilterRuntime('kubernetes')}
              >
                K8S
              </button>
            </div>
          )}
        </div>

        <div className="tab-header-right">
          {/* Selection Mode Controls */}
          {selectionMode ? (
            <>
              <span className="selection-count">
                {selectedContainers.size} selected
              </span>
              <button
                className="btn btn-secondary btn-sm"
                onClick={selectedContainers.size === filteredContainers.length ? clearSelection : selectAllContainers}
                aria-label={selectedContainers.size === filteredContainers.length ? 'Deselect all' : 'Select all'}
              >
                {selectedContainers.size === filteredContainers.length ? (
                  <><Square size={14} /> DESELECT ALL</>
                ) : (
                  <><CheckSquare size={14} /> SELECT ALL</>
                )}
              </button>
              <button
                className="btn btn-primary btn-sm"
                onClick={handleBatchExport}
                disabled={selectedContainers.size === 0}
                aria-label="Export selected containers"
              >
                EXPORT {selectedContainers.size > 0 ? `(${selectedContainers.size})` : ''}
              </button>
              <button
                className="btn btn-secondary btn-sm"
                onClick={toggleSelectionMode}
                aria-label="Cancel selection"
              >
                CANCEL
              </button>
            </>
          ) : (
            <>
              <button
                className="btn btn-secondary"
                onClick={toggleSelectionMode}
                disabled={containers.length === 0}
                aria-label="Enter selection mode for batch export"
              >
                <CheckSquare size={16} />
                SELECT
              </button>
              <button
                className="btn btn-secondary"
                onClick={handleRefresh}
                disabled={isRefreshing || containersLoading}
                aria-label="Refresh containers"
              >
                <RefreshCw size={16} className={isRefreshing ? 'spinning' : ''} />
                REFRESH
              </button>
            </>
          )}
        </div>
      </div>

      {/* Loading State */}
      {containersLoading && !isRefreshing && (
        <div className="loading-state">
          <p>Loading containers...</p>
        </div>
      )}

      {/* Error State */}
      {containersError && (
        <div className="error-state">
          <h3>Error Loading Containers</h3>
          <p className="text-secondary">{containersError}</p>
          <button className="btn btn-primary" onClick={handleRefresh}>
            TRY AGAIN
          </button>
        </div>
      )}

      {/* Container Cards Grid */}
      {!containersLoading && !containersError && (
        <>
          {filteredContainers.length > 0 ? (
            <div className="container-grid">
              {filteredContainers.map((container) => (
                <ContainerCard
                  key={container.id}
                  container={container}
                  stats={container.stats}
                  onStart={handleStart}
                  onStop={handleStop}
                  onRestart={handleRestart}
                  onInspect={handleInspect}
                  onLogs={handleLogs}
                  onExport={handleExport}
                  selectable={selectionMode}
                  selected={selectedContainers.has(container.id)}
                  onSelect={handleContainerSelect}
                />
              ))}
            </div>
          ) : (
            <div className="empty-state">
              <h3>No Containers Found</h3>
              <p className="text-secondary">
                {settings.showOnlyRoutedContainers
                  ? 'No containers with configured routes found.'
                  : filterState === 'all' && filterRuntime === 'all'
                    ? 'No containers are currently available.'
                    : filterRuntime !== 'all'
                      ? `No ${filterState === 'all' ? '' : filterState + ' '}${filterRuntime === 'kubernetes' ? 'Kubernetes' : 'Docker'} containers found.`
                      : `No ${filterState} containers found.`}
              </p>
              {(filterState !== 'all' || filterRuntime !== 'all') && (
                <button
                  className="btn btn-secondary"
                  onClick={() => {
                    setFilterState('all');
                    setFilterRuntime('all');
                  }}
                >
                  SHOW ALL CONTAINERS
                </button>
              )}
            </div>
          )}
        </>
      )}

      {/* Container Details Modal */}
      <ContainerDetailsModal
        isOpen={detailsModalOpen}
        onClose={() => {
          setDetailsModalOpen(false);
          setSelectedContainer(null);
        }}
        container={selectedContainer}
      />

      {/* Container Logs Modal */}
      <ContainerLogsModal
        isOpen={logsModalOpen}
        onClose={() => {
          setLogsModalOpen(false);
          setSelectedContainer(null);
        }}
        container={selectedContainer}
      />

      {/* Container Export Modal */}
      <ContainerExportModal
        isOpen={exportModalOpen}
        onClose={() => {
          setExportModalOpen(false);
          setSelectedContainer(null);
        }}
        container={selectedContainer}
        onExport={exportContainerConfig}
        onPreview={previewContainerExport}
      />

      {/* Batch Export Modal */}
      <BatchExportModal
        isOpen={batchExportModalOpen}
        onClose={() => {
          setBatchExportModalOpen(false);
        }}
        containers={getSelectedContainerObjects()}
        onExport={batchExportContainerConfig}
        onPreview={previewBatchExport}
      />

      {/* Confirmation Modal */}
      <ConfirmationModal
        isOpen={confirmModalOpen}
        onClose={() => {
          setConfirmModalOpen(false);
          setSelectedContainer(null);
          setActionType(null);
        }}
        onConfirm={handleConfirmAction}
        title={getConfirmationContent().title}
        message={getConfirmationContent().message}
        details={
          selectedContainer ? (
            <div className="confirmation-details">
              <div className="confirmation-details-item">
                <span className="confirmation-details-label">Container:</span>
                <span className="confirmation-details-value">
                  {selectedContainer.name?.replace(/^\//, '') || 'Unnamed'}
                </span>
              </div>
              <div className="confirmation-details-item">
                <span className="confirmation-details-label">Image:</span>
                <span className="confirmation-details-value">{selectedContainer.image}</span>
              </div>
              <div className="confirmation-details-item">
                <span className="confirmation-details-label">Runtime:</span>
                <span className="confirmation-details-value">
                  {selectedContainer.runtime === 'kubernetes' ? 'Kubernetes' : 'Docker'}
                </span>
              </div>
              {selectedContainer.runtime === 'kubernetes' && selectedContainer.namespace && (
                <div className="confirmation-details-item">
                  <span className="confirmation-details-label">Namespace:</span>
                  <span className="confirmation-details-value">{selectedContainer.namespace}</span>
                </div>
              )}
              <div className="confirmation-details-item">
                <span className="confirmation-details-label">State:</span>
                <span className="confirmation-details-value">{selectedContainer.state}</span>
              </div>
              {actionType === 'stop' && (
                <p className="confirmation-warning">
                  Stopping this container may interrupt running services.
                </p>
              )}
            </div>
          ) : null
        }
        danger={actionType === 'stop'}
      />
    </div>
  );
}
