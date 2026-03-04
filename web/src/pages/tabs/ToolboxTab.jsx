/**
 * ToolboxTab Component
 *
 * Docker toolbox interface for one-click service deployment
 *
 * Features:
 * - Browse service catalog
 * - Filter services by category
 * - Deploy services with configuration
 * - View active deployments
 * - Monitor deployment status
 * - Remove deployments
 */

import { useState, useEffect, useMemo } from 'react';
import { RefreshCw, Store, Package } from 'lucide-react';
import { ServiceCard } from '../../components/cards/ServiceCard';
import { DeploymentCard } from '../../components/cards/DeploymentCard';
import {
  ServiceDetailsModal,
  DeployServiceModal,
  ConfirmationModal
} from '../../components/modals';
import { useNotification } from '../../contexts/NotificationContext';

/**
 * ToolboxTab Component
 */
export function ToolboxTab() {
  const { addNotification } = useNotification();

  // State for feature enabled
  const [featureEnabled, setFeatureEnabled] = useState(true);

  // State for services
  const [services, setServices] = useState([]);
  const [servicesLoading, setServicesLoading] = useState(true);
  const [servicesError, setServicesError] = useState(null);

  // State for deployments
  const [deployments, setDeployments] = useState([]);
  const [deploymentsLoading, setDeploymentsLoading] = useState(true);
  const [deploymentsError, setDeploymentsError] = useState(null);

  // State for UI
  const [selectedService, setSelectedService] = useState(null);
  const [selectedDeployment, setSelectedDeployment] = useState(null);
  const [detailsModalOpen, setDetailsModalOpen] = useState(false);
  const [deployModalOpen, setDeployModalOpen] = useState(false);
  const [removeModalOpen, setRemoveModalOpen] = useState(false);
  const [filterCategory, setFilterCategory] = useState('all');
  const [viewMode, setViewMode] = useState('services'); // services | deployments
  const [isRefreshing, setIsRefreshing] = useState(false);

  // Fetch services on mount
  useEffect(() => {
    fetchServices();
  }, []);

  // Fetch deployments on mount
  useEffect(() => {
    fetchDeployments();
  }, []);

  // Fetch services from API
  const fetchServices = async () => {
    try {
      setServicesLoading(true);
      setServicesError(null);
      const response = await fetch('/api/v1/toolbox/services');
      if (!response.ok) {
        throw new Error(`Failed to load services: ${response.statusText}`);
      }
      const data = await response.json();
      // Check if feature is disabled
      if (data.enabled === false) {
        setFeatureEnabled(false);
        return;
      }
      setFeatureEnabled(true);
      setServices(data.services || []);
    } catch (error) {
      console.error('Error fetching services:', error);
      setServicesError(error.message);
    } finally {
      setServicesLoading(false);
    }
  };

  // Fetch deployments from API
  const fetchDeployments = async () => {
    try {
      setDeploymentsLoading(true);
      setDeploymentsError(null);
      const response = await fetch('/api/v1/toolbox/deployments');
      if (!response.ok) {
        throw new Error(`Failed to load deployments: ${response.statusText}`);
      }
      const data = await response.json();
      setDeployments(data.deployments || []);
    } catch (error) {
      console.error('Error fetching deployments:', error);
      setDeploymentsError(error.message);
    } finally {
      setDeploymentsLoading(false);
    }
  };

  // Handle refresh
  const handleRefresh = async () => {
    setIsRefreshing(true);
    try {
      if (viewMode === 'services') {
        await fetchServices();
      } else {
        await fetchDeployments();
      }
    } finally {
      // Small delay for visual feedback
      setTimeout(() => setIsRefreshing(false), 500);
    }
  };

  // Get unique categories from services
  const categories = useMemo(() => {
    const cats = new Set(services.map(s => s.category));
    return ['all', ...Array.from(cats)];
  }, [services]);

  // Filter and sort services by category
  const filteredServices = useMemo(() => {
    const filtered = filterCategory === 'all'
      ? services
      : services.filter(s => s.category === filterCategory);
    // Sort alphabetically by name for consistent ordering
    return [...filtered].sort((a, b) => a.name.localeCompare(b.name));
  }, [services, filterCategory]);

  // Calculate stats
  const totalServices = services.length;
  const totalDeployments = deployments.length;
  const runningDeployments = deployments.filter(d => d.status === 'running').length;

  // Handle view service details
  const handleViewDetails = (service) => {
    setSelectedService(service);
    setDetailsModalOpen(true);
  };

  // Handle deploy service
  const handleDeployService = (service) => {
    setSelectedService(service);
    setDeployModalOpen(true);
  };

  // Poll deployment status until it completes or fails
  const pollDeploymentStatus = async (deploymentId, serviceName, maxAttempts = 60) => {
    const pollInterval = 2000; // 2 seconds between polls

    for (let attempt = 0; attempt < maxAttempts; attempt++) {
      try {
        const response = await fetch(`/api/v1/toolbox/deployments/${deploymentId}`);
        if (!response.ok) {
          console.error('Failed to fetch deployment status');
          break;
        }

        const deployment = await response.json();

        if (deployment.status === 'deployed') {
          addNotification({
            severity: 'success',
            message: `${serviceName} deployed successfully!`,
            strongText: 'DEPLOYED:'
          });
          await fetchDeployments();
          return;
        }

        if (deployment.status === 'failed') {
          addNotification({
            severity: 'error',
            message: `${serviceName} deployment failed: ${deployment.error_message || 'Unknown error'}`,
            strongText: 'FAILED:'
          });
          await fetchDeployments();
          return;
        }

        // Still deploying, wait and poll again
        await new Promise(resolve => setTimeout(resolve, pollInterval));
      } catch (error) {
        console.error('Error polling deployment status:', error);
        break;
      }
    }

    // Timeout or error - refresh anyway
    await fetchDeployments();
  };

  // Handle confirm deployment
  const handleConfirmDeploy = async (deploymentConfig) => {
    // Show deploying notification
    addNotification({
      severity: 'info',
      message: `Deploying ${deploymentConfig.service_name}...`,
      strongText: 'DEPLOYING:'
    });

    try {
      const response = await fetch('/api/v1/toolbox/deploy', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify(deploymentConfig),
      });

      if (!response.ok) {
        throw new Error(`Deployment failed: ${response.statusText}`);
      }

      const result = await response.json();
      console.log('Deployment initiated:', result);

      // Close modal immediately
      setDeployModalOpen(false);
      setSelectedService(null);

      // Refresh deployments list to show pending deployment
      await fetchDeployments();

      // Poll for actual deployment completion in background
      pollDeploymentStatus(result.deployment_id, deploymentConfig.service_name);

    } catch (error) {
      console.error('Error deploying service:', error);

      // Show error notification
      addNotification({
        severity: 'error',
        message: `Failed to deploy ${deploymentConfig.service_name}: ${error.message}`,
        strongText: 'ERROR:'
      });

      throw error; // Re-throw to be handled by modal
    }
  };

  // Handle remove deployment
  const handleRemoveDeployment = (deployment) => {
    setSelectedDeployment(deployment);
    setRemoveModalOpen(true);
  };

  // Handle confirm remove
  const handleConfirmRemove = async () => {
    if (!selectedDeployment) return;

    const serviceName = selectedDeployment.service_name;

    try {
      const response = await fetch(`/api/v1/toolbox/deployments/${selectedDeployment.id}`, {
        method: 'DELETE',
      });

      if (!response.ok) {
        throw new Error(`Failed to remove deployment: ${response.statusText}`);
      }

      // Show success notification
      addNotification({
        severity: 'success',
        message: `${serviceName} removed successfully`,
        strongText: 'REMOVED:'
      });

      // Refresh deployments list
      await fetchDeployments();
    } catch (error) {
      console.error('Error removing deployment:', error);

      // Show error notification
      addNotification({
        severity: 'error',
        message: `Failed to remove ${serviceName}: ${error.message}`,
        strongText: 'ERROR:'
      });
    } finally {
      setRemoveModalOpen(false);
      setSelectedDeployment(null);
    }
  };

  // Render disabled state when toolbox is not enabled
  if (!featureEnabled && !servicesLoading) {
    return (
      <div className="toolbox-tab toolbox-disabled">
        <div className="disabled-overlay">
          <div className="disabled-content">
            <h3>Toolbox Disabled</h3>
            <p className="text-secondary">
              The service toolbox is not enabled on this server.
            </p>
            <p className="text-secondary" style={{ marginTop: 'var(--space-3)' }}>
              To enable, use one of the following:
            </p>
            <ul className="text-secondary" style={{ marginTop: 'var(--space-2)', textAlign: 'left', display: 'inline-block' }}>
              <li>Config file: <code>toolbox.enabled: true</code></li>
              <li>Environment: <code>NEKZUS_TOOLBOX_ENABLED=true</code></li>
            </ul>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="toolbox-tab">
      {/* Header */}
      <div className="tab-header">
        <div className="tab-header-left">
          {/* View Mode Toggle */}
          <div className="view-mode-toggle">
            <button
              className={`btn btn-sm ${viewMode === 'services' ? 'btn-primary' : 'btn-secondary'}`}
              onClick={() => setViewMode('services')}
            >
              <Store size={16} />
              SERVICES ({totalServices})
            </button>
            <button
              className={`btn btn-sm ${viewMode === 'deployments' ? 'btn-primary' : 'btn-secondary'}`}
              onClick={() => setViewMode('deployments')}
            >
              <Package size={16} />
              DEPLOYMENTS ({totalDeployments})
            </button>
          </div>

          {/* Category Filter (only for services view) */}
          {viewMode === 'services' && (
            <div className="category-filter">
              {categories.map(cat => (
                <button
                  key={cat}
                  className={`btn btn-sm ${filterCategory === cat ? 'btn-primary' : 'btn-secondary'}`}
                  onClick={() => setFilterCategory(cat)}
                >
                  {cat.toUpperCase()}
                </button>
              ))}
            </div>
          )}
        </div>

        <div className="tab-header-right">
          <button
            className="btn btn-secondary"
            onClick={handleRefresh}
            disabled={isRefreshing || servicesLoading || deploymentsLoading}
            aria-label="Refresh"
          >
            <RefreshCw size={16} className={isRefreshing ? 'spinning' : ''} />
            REFRESH
          </button>
        </div>
      </div>

      {/* Services View */}
      {viewMode === 'services' && (
        <>
          {/* Loading State */}
          {servicesLoading && !isRefreshing && (
            <div className="loading-state">
              <p>Loading services...</p>
            </div>
          )}

          {/* Error State */}
          {servicesError && (
            <div className="error-state">
              <h3>Error Loading Services</h3>
              <p className="text-secondary">{servicesError}</p>
              <button className="btn btn-primary" onClick={fetchServices}>
                TRY AGAIN
              </button>
            </div>
          )}

          {/* Services Grid */}
          {!servicesLoading && !servicesError && (
            <>
              {filteredServices.length > 0 ? (
                <div className="service-grid">
                  {filteredServices.map((service) => (
                    <ServiceCard
                      key={service.id}
                      service={service}
                      onViewDetails={handleViewDetails}
                      onDeploy={handleDeployService}
                    />
                  ))}
                </div>
              ) : (
                <div className="empty-state">
                  <h3>No Services Found</h3>
                  <p className="text-secondary">
                    {filterCategory === 'all'
                      ? 'No services are currently available in the toolbox.'
                      : `No services found in the ${filterCategory} category.`}
                  </p>
                  {filterCategory !== 'all' && (
                    <button
                      className="btn btn-secondary"
                      onClick={() => setFilterCategory('all')}
                    >
                      SHOW ALL SERVICES
                    </button>
                  )}
                </div>
              )}
            </>
          )}
        </>
      )}

      {/* Deployments View */}
      {viewMode === 'deployments' && (
        <>
          {/* Loading State */}
          {deploymentsLoading && !isRefreshing && (
            <div className="loading-state">
              <p>Loading deployments...</p>
            </div>
          )}

          {/* Error State */}
          {deploymentsError && (
            <div className="error-state">
              <h3>Error Loading Deployments</h3>
              <p className="text-secondary">{deploymentsError}</p>
              <button className="btn btn-primary" onClick={fetchDeployments}>
                TRY AGAIN
              </button>
            </div>
          )}

          {/* Deployments Grid */}
          {!deploymentsLoading && !deploymentsError && (
            <>
              {deployments.length > 0 ? (
                <div className="deployment-grid">
                  {deployments.map((deployment) => (
                    <DeploymentCard
                      key={deployment.id}
                      deployment={deployment}
                      onRemove={handleRemoveDeployment}
                    />
                  ))}
                </div>
              ) : (
                <div className="empty-state">
                  <h3>No Deployments</h3>
                  <p className="text-secondary">
                    You haven't deployed any services yet. Switch to the Services view to deploy.
                  </p>
                  <button
                    className="btn btn-primary"
                    onClick={() => setViewMode('services')}
                  >
                    BROWSE SERVICES
                  </button>
                </div>
              )}
            </>
          )}
        </>
      )}

      {/* Service Details Modal */}
      <ServiceDetailsModal
        isOpen={detailsModalOpen}
        onClose={() => {
          setDetailsModalOpen(false);
          setSelectedService(null);
        }}
        service={selectedService}
        onDeploy={handleDeployService}
      />

      {/* Deploy Service Modal */}
      <DeployServiceModal
        isOpen={deployModalOpen}
        onClose={() => {
          setDeployModalOpen(false);
          setSelectedService(null);
        }}
        service={selectedService}
        onDeploy={handleConfirmDeploy}
      />

      {/* Remove Deployment Confirmation Modal */}
      <ConfirmationModal
        isOpen={removeModalOpen}
        onClose={() => {
          setRemoveModalOpen(false);
          setSelectedDeployment(null);
        }}
        onConfirm={handleConfirmRemove}
        title="Remove Deployment"
        message={`Are you sure you want to remove the deployment ${selectedDeployment?.service_name}?`}
        details={
          selectedDeployment ? (
            <div className="confirmation-details">
              <div className="confirmation-details-item">
                <span className="confirmation-details-label">Service:</span>
                <span className="confirmation-details-value">
                  {selectedDeployment.service_name}
                </span>
              </div>
              <div className="confirmation-details-item">
                <span className="confirmation-details-label">Container ID:</span>
                <span className="confirmation-details-value">
                  <code>{selectedDeployment.container_id?.substring(0, 12) || 'N/A'}</code>
                </span>
              </div>
              <div className="confirmation-details-item">
                <span className="confirmation-details-label">Status:</span>
                <span className="confirmation-details-value">{selectedDeployment.status}</span>
              </div>
              <p className="confirmation-warning">
                This will stop and remove the container. This action cannot be undone.
              </p>
            </div>
          ) : null
        }
        danger={true}
      />
    </div>
  );
}
