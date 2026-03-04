/**
 * FederationTab Component
 *
 * Federation management interface for peer-to-peer clustering.
 *
 * Features:
 * - View connected peers
 * - View federated service catalog
 * - Trigger manual sync
 * - Monitor federation health
 */

import { useState, useEffect, useCallback } from 'react';
import { PeerCard } from '../../components/cards';
import { Badge } from '../../components/data-display';
import { ConfirmationModal } from '../../components/modals';
import { federationAPI } from '../../services/api';
import { useNotification } from '../../contexts/NotificationContext';
import { getConfidenceCategory } from '../../utils/confidence';

/**
 * FederationTab Component
 */
export function FederationTab() {
  const { addNotification } = useNotification();

  // State
  const [federationStatus, setFederationStatus] = useState(null);
  const [peers, setPeers] = useState([]);
  const [catalog, setCatalog] = useState([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState(null);
  const [syncInProgress, setSyncInProgress] = useState(false);
  const [selectedPeer, setSelectedPeer] = useState(null);
  const [detailsModalOpen, setDetailsModalOpen] = useState(false);

  // Load federation data
  const loadData = useCallback(async () => {
    try {
      setError(null);

      // Load all federation data in parallel
      const [statusData, peersData, catalogData] = await Promise.all([
        federationAPI.status().catch(() => ({ enabled: false })),
        federationAPI.listPeers().catch(() => ({ peers: [] })),
        federationAPI.getCatalog().catch(() => ({ services: [] })),
      ]);

      setFederationStatus(statusData);
      setPeers(peersData.peers || []);
      setCatalog(catalogData.services || []);
    } catch (err) {
      setError(err.message || 'Failed to load federation data');
    } finally {
      setIsLoading(false);
    }
  }, []);

  // Initial load
  useEffect(() => {
    loadData();
  }, [loadData]);

  // Refresh data periodically
  useEffect(() => {
    const interval = setInterval(loadData, 30000); // Every 30 seconds
    return () => clearInterval(interval);
  }, [loadData]);

  // Handle manual sync
  const handleTriggerSync = async () => {
    try {
      setSyncInProgress(true);
      await federationAPI.triggerSync();
      addNotification({
        type: 'success',
        message: 'Federation sync triggered successfully',
      });
      // Reload data after sync
      await loadData();
    } catch (err) {
      addNotification({
        type: 'error',
        message: `Sync failed: ${err.message}`,
      });
    } finally {
      setSyncInProgress(false);
    }
  };

  // Handle view peer details
  const handleViewPeerDetails = (peer) => {
    setSelectedPeer(peer);
    setDetailsModalOpen(true);
  };

  // Calculate stats
  const onlinePeers = peers.filter(p => p.status === 'online').length;
  const totalServices = catalog.length;
  const localServices = catalog.filter(s => s.origin_peer_id === federationStatus?.local_peer_id).length;
  const remoteServices = totalServices - localServices;

  // Render loading state
  if (isLoading) {
    return (
      <div className="federation-tab">
        <div className="loading-state">
          <p>Loading federation data...</p>
        </div>
      </div>
    );
  }

  // Render disabled state
  if (!federationStatus?.enabled) {
    return (
      <div className="federation-tab">
        <div className="empty-state">
          <h3>Federation Disabled</h3>
          <p className="text-secondary">
            Federation is not enabled. Configure federation settings in your config file to enable peer-to-peer clustering.
          </p>
          <div className="code-block">
            <pre>{`federation:
  enabled: true
  local_peer_id: "nxs_yourserver"
  cluster_secret: "your-secret-key"
  # See docs for full configuration`}</pre>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="federation-tab">
      {/* Header */}
      <div className="tab-header">
        <div className="federation-status">
          <Badge variant="success" filled>FEDERATION ACTIVE</Badge>
          <span className="status-text">
            {onlinePeers} peer{onlinePeers !== 1 ? 's' : ''} connected
          </span>
        </div>
        <button
          className="btn btn-primary"
          onClick={handleTriggerSync}
          disabled={syncInProgress}
          aria-label="Trigger sync"
        >
          {syncInProgress ? 'SYNCING...' : 'SYNC NOW'}
        </button>
      </div>

      {/* Stats Row */}
      <div className="federation-stats">
        <div className="stat-box">
          <span className="stat-value">{peers.length}</span>
          <span className="stat-label">Total Peers</span>
        </div>
        <div className="stat-box">
          <span className="stat-value">{onlinePeers}</span>
          <span className="stat-label">Online</span>
        </div>
        <div className="stat-box">
          <span className="stat-value">{totalServices}</span>
          <span className="stat-label">Total Services</span>
        </div>
        <div className="stat-box">
          <span className="stat-value">{remoteServices}</span>
          <span className="stat-label">Remote Services</span>
        </div>
      </div>

      {/* Error display */}
      {error && (
        <div className="alert alert-error">
          <p>{error}</p>
          <button className="btn btn-sm" onClick={loadData}>Retry</button>
        </div>
      )}

      {/* Peers Section */}
      <section className="federation-section">
        <h3 className="section-title">Connected Peers</h3>
        {peers.length > 0 ? (
          <div className="peer-grid">
            {peers.map((peer) => (
              <PeerCard
                key={peer.id}
                peer={peer}
                onViewDetails={handleViewPeerDetails}
              />
            ))}
          </div>
        ) : (
          <div className="empty-state-sm">
            <p>No peers connected. Add bootstrap peers to your config or enable mDNS discovery.</p>
          </div>
        )}
      </section>

      {/* Federated Catalog Section */}
      <section className="federation-section">
        <h3 className="section-title">Federated Catalog</h3>
        {catalog.length > 0 ? (
          <div className="catalog-table">
            <table>
              <thead>
                <tr>
                  <th>Service</th>
                  <th>Origin Peer</th>
                  <th>Confidence</th>
                  <th>Last Seen</th>
                </tr>
              </thead>
              <tbody>
                {catalog.map((service) => (
                  <tr key={`${service.origin_peer_id}-${service.service_id}`}>
                    <td>
                      <div className="service-name">
                        {service.app?.icon && <span className="service-icon">{service.app.icon}</span>}
                        <span>{service.app?.name || service.service_id}</span>
                      </div>
                    </td>
                    <td>
                      <Badge
                        variant={service.origin_peer_id === federationStatus?.local_peer_id ? 'info' : 'default'}
                      >
                        {service.origin_peer_id === federationStatus?.local_peer_id ? 'LOCAL' : service.origin_peer_id}
                      </Badge>
                    </td>
                    <td>
                      <Badge variant={getConfidenceCategory(service.confidence || 1).variant} size="sm">
                        {getConfidenceCategory(service.confidence || 1).label}
                      </Badge>
                    </td>
                    <td className="text-secondary">
                      {service.last_seen ? new Date(service.last_seen).toLocaleString() : '-'}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <div className="empty-state-sm">
            <p>No federated services. Services will appear here when discovered on connected peers.</p>
          </div>
        )}
      </section>

      {/* Peer Details Modal */}
      <ConfirmationModal
        isOpen={detailsModalOpen}
        onClose={() => {
          setDetailsModalOpen(false);
          setSelectedPeer(null);
        }}
        onConfirm={() => {
          setDetailsModalOpen(false);
          setSelectedPeer(null);
        }}
        title={`Peer: ${selectedPeer?.name || selectedPeer?.id}`}
        message="Peer details"
        confirmText="CLOSE"
        hideCancel
        details={
          selectedPeer ? (
            <div className="peer-details">
              <div className="detail-row">
                <strong>Peer ID:</strong>
                <span>{selectedPeer.id}</span>
              </div>
              <div className="detail-row">
                <strong>Name:</strong>
                <span>{selectedPeer.name || '-'}</span>
              </div>
              <div className="detail-row">
                <strong>Status:</strong>
                <Badge variant={selectedPeer.status === 'online' ? 'success' : 'warning'}>
                  {selectedPeer.status?.toUpperCase()}
                </Badge>
              </div>
              <div className="detail-row">
                <strong>Address:</strong>
                <span>{selectedPeer.address || '-'}</span>
              </div>
              <div className="detail-row">
                <strong>Gossip Address:</strong>
                <span>{selectedPeer.gossip_addr || '-'}</span>
              </div>
              <div className="detail-row">
                <strong>Last Seen:</strong>
                <span>{selectedPeer.last_seen ? new Date(selectedPeer.last_seen).toLocaleString() : '-'}</span>
              </div>
              <div className="detail-row">
                <strong>Created:</strong>
                <span>{selectedPeer.created_at ? new Date(selectedPeer.created_at).toLocaleString() : '-'}</span>
              </div>
            </div>
          ) : null
        }
      />
    </div>
  );
}
