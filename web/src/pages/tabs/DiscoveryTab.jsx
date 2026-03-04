/**
 * DiscoveryTab Component
 *
 * Service discovery management interface
 *
 * Features:
 * - Grid of DiscoveryCard components
 * - Bulk selection with "Select All" checkbox
 * - Bulk approve/reject actions
 * - Individual approve/reject per card
 * - Empty state when no discoveries
 * - Real-time discovery count
 */

import { useState, useMemo } from 'react';
import { RefreshCw } from 'lucide-react';
import { DiscoveryCard } from '../../components/cards';
import { ConfirmationModal } from '../../components/modals';
import { useData } from '../../contexts';
import { useSettings } from '../../contexts/SettingsContext';

/**
 * DiscoveryTab Component
 */
export function DiscoveryTab() {
  const { discoveries, approveDiscovery, rejectDiscovery, bulkApproveDiscoveries, bulkRejectDiscoveries, rediscover } = useData();
  const { settings } = useSettings();
  const [selectedIds, setSelectedIds] = useState(new Set());
  const [rejectModalOpen, setRejectModalOpen] = useState(false);
  const [bulkRejectModalOpen, setBulkRejectModalOpen] = useState(false);
  const [discoveryToReject, setDiscoveryToReject] = useState(null);
  const [isRediscovering, setIsRediscovering] = useState(false);

  // Calculate selection state
  const allSelected = useMemo(() => {
    return discoveries.length > 0 && selectedIds.size === discoveries.length;
  }, [discoveries.length, selectedIds.size]);

  const someSelected = useMemo(() => {
    return selectedIds.size > 0 && selectedIds.size < discoveries.length;
  }, [discoveries.length, selectedIds.size]);

  const hasSelections = selectedIds.size > 0;

  // Handle individual selection toggle
  const handleSelect = (discoveryId) => {
    setSelectedIds((prev) => {
      const newSet = new Set(prev);
      if (newSet.has(discoveryId)) {
        newSet.delete(discoveryId);
      } else {
        newSet.add(discoveryId);
      }
      return newSet;
    });
  };

  // Handle select all toggle
  const handleSelectAll = () => {
    if (allSelected) {
      setSelectedIds(new Set());
    } else {
      setSelectedIds(new Set(discoveries.map((d) => d.id)));
    }
  };

  // Handle individual approve
  const handleApprove = (discovery, selectedPort) => {
    const options = selectedPort ? { port: selectedPort } : {};
    approveDiscovery(discovery.id, options);
    // Remove from selection after action
    setSelectedIds((prev) => {
      const newSet = new Set(prev);
      newSet.delete(discovery.id);
      return newSet;
    });
  };

  // Handle individual reject
  const handleReject = (discovery) => {
    // Check if confirmation is required
    if (settings.requireConfirmationForRejections) {
      setDiscoveryToReject(discovery);
      setRejectModalOpen(true);
    } else {
      // Directly reject without confirmation
      rejectDiscovery(discovery.id);
      // Remove from selection after action
      setSelectedIds((prev) => {
        const newSet = new Set(prev);
        newSet.delete(discovery.id);
        return newSet;
      });
    }
  };

  // Handle confirm individual reject
  const handleConfirmReject = () => {
    if (discoveryToReject) {
      rejectDiscovery(discoveryToReject.id);
      // Remove from selection after action
      setSelectedIds((prev) => {
        const newSet = new Set(prev);
        newSet.delete(discoveryToReject.id);
        return newSet;
      });
    }
    setRejectModalOpen(false);
    setDiscoveryToReject(null);
  };

  // Handle bulk approve
  const handleBulkApprove = () => {
    const idsArray = Array.from(selectedIds);
    bulkApproveDiscoveries(idsArray);
    setSelectedIds(new Set());
  };

  // Handle bulk reject
  const handleBulkReject = () => {
    // Check if confirmation is required
    if (settings.requireConfirmationForRejections) {
      setBulkRejectModalOpen(true);
    } else {
      // Directly reject without confirmation
      const idsArray = Array.from(selectedIds);
      bulkRejectDiscoveries(idsArray);
      setSelectedIds(new Set());
    }
  };

  // Handle confirm bulk reject
  const handleConfirmBulkReject = () => {
    const idsArray = Array.from(selectedIds);
    bulkRejectDiscoveries(idsArray);
    setSelectedIds(new Set());
    setBulkRejectModalOpen(false);
  };

  // Handle rediscover
  const handleRediscover = async () => {
    setIsRediscovering(true);
    try {
      await rediscover();
      // Clear selections after rediscovery
      setSelectedIds(new Set());
    } catch (error) {
      console.error('Rediscovery failed:', error);
    } finally {
      setIsRediscovering(false);
    }
  };

  return (
    <div className="discovery-tab">

      {/* Bulk Actions Toolbar */}
      {discoveries.length > 0 && (
        <div className="bulk-actions">
          <label className="checkbox-label">
            <input
              type="checkbox"
              className="checkbox"
              id="selectAllDiscoveries"
              checked={allSelected}
              ref={(input) => {
                if (input) input.indeterminate = someSelected;
              }}
              onChange={handleSelectAll}
              aria-label="Select all discoveries"
            />
            <span>Select All</span>
          </label>
          <button
            type="button"
            className="btn btn-secondary btn-sm"
            onClick={handleBulkReject}
            disabled={!hasSelections}
            aria-label={`Reject ${selectedIds.size} selected discoveries`}
          >
            REJECT SELECTED
          </button>
          <button
            type="button"
            className="btn btn-success btn-sm"
            onClick={handleBulkApprove}
            disabled={!hasSelections}
            aria-label={`Approve ${selectedIds.size} selected discoveries`}
          >
            APPROVE SELECTED
          </button>
          <div style={{ flex: 1 }} />
          <button
            type="button"
            className="btn btn-primary btn-sm"
            onClick={handleRediscover}
            disabled={isRediscovering}
            aria-label="Trigger rediscovery scan"
            title="Clear dismissed proposals and scan for new services"
          >
            <RefreshCw size={14} className={isRediscovering ? 'spinning' : ''} />
            {isRediscovering ? ' SCANNING...' : ' REDISCOVER'}
          </button>
        </div>
      )}

      {/* Discovery Cards Grid */}
      {discoveries.length > 0 ? (
        <div className="discovery-grid">
          {discoveries.map((discovery) => (
            <DiscoveryCard
              key={discovery.id}
              discovery={{
                ...discovery,
                selected: selectedIds.has(discovery.id)
              }}
              onSelect={handleSelect}
              onApprove={handleApprove}
              onReject={handleReject}
            />
          ))}
        </div>
      ) : (
        <div className="empty-state">
          <h3>No Pending Discoveries</h3>
          <p className="text-secondary">
            All discovered services have been processed. New services will appear here when detected.
          </p>
          <button
            type="button"
            className="btn btn-primary btn-sm"
            onClick={handleRediscover}
            disabled={isRediscovering}
            aria-label="Trigger rediscovery scan"
            style={{ marginTop: 'var(--space-4)' }}
          >
            <RefreshCw size={16} className={isRediscovering ? 'spinning' : ''} />
            {isRediscovering ? ' SCANNING...' : ' REDISCOVER SERVICES'}
          </button>
        </div>
      )}

      {/* Individual Reject Confirmation Modal */}
      <ConfirmationModal
        isOpen={rejectModalOpen}
        onClose={() => {
          setRejectModalOpen(false);
          setDiscoveryToReject(null);
        }}
        onConfirm={handleConfirmReject}
        title="REJECT DISCOVERY"
        message={`Are you sure you want to reject this discovered service?`}
        details={
          discoveryToReject ? (
            <div className="confirmation-details">
              <p><strong>Service:</strong> {discoveryToReject.name}</p>
              <p><strong>Type:</strong> {discoveryToReject.type}</p>
              <p><strong>Address:</strong> {discoveryToReject.address}:{discoveryToReject.port}</p>
            </div>
          ) : null
        }
        confirmText="REJECT"
        cancelText="CANCEL"
      />

      {/* Bulk Reject Confirmation Modal */}
      <ConfirmationModal
        isOpen={bulkRejectModalOpen}
        onClose={() => setBulkRejectModalOpen(false)}
        onConfirm={handleConfirmBulkReject}
        title="REJECT SELECTED DISCOVERIES"
        message={`Are you sure you want to reject ${selectedIds.size} selected ${selectedIds.size === 1 ? 'discovery' : 'discoveries'}?`}
        details={
          <div className="confirmation-details">
            <p>This action will permanently reject the selected discovered services.</p>
          </div>
        }
        confirmText="REJECT ALL"
        cancelText="CANCEL"
      />
    </div>
  );
}
