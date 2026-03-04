/**
 * DevicesTab Component
 *
 * Device management interface
 *
 * Features:
 * - Grid of DeviceCard components
 * - Pair new device button
 * - View device details
 * - Revoke device access with confirmation
 * - Filter by status (online/offline)
 * - Real-time device count
 */

import { useState } from 'react';
import { DeviceCard } from '../../components/cards';
import { ConfirmationModal, PairingModal, DeviceDetailsModal } from '../../components/modals';
import { useData } from '../../contexts';
import { useSettings } from '../../contexts/SettingsContext';

/**
 * DevicesTab Component
 */
export function DevicesTab() {
  const { devices, revokeDevice, pairDevice } = useData();
  const { settings } = useSettings();
  const [selectedDevice, setSelectedDevice] = useState(null);
  const [revokeModalOpen, setRevokeModalOpen] = useState(false);
  const [detailsModalOpen, setDetailsModalOpen] = useState(false);
  const [pairingModalOpen, setPairingModalOpen] = useState(false);

  // Calculate device stats
  const onlineDevices = devices.filter(d => d.status === 'online').length;
  const offlineDevices = devices.filter(d => d.status === 'offline').length;

  // Handle pair new device
  const handlePairDevice = () => {
    setPairingModalOpen(true);
  };

  // Handle successful pairing
  const handlePairSuccess = (newDevice) => {
    if (pairDevice) {
      pairDevice(newDevice);
    }
  };

  // Handle view device details
  const handleViewDetails = (device) => {
    setSelectedDevice(device);
    setDetailsModalOpen(true);
  };

  // Handle revoke device
  const handleRevokeDevice = (device) => {
    setSelectedDevice(device);

    // Check if confirmation is required
    if (settings.requireConfirmation) {
      setRevokeModalOpen(true);
    } else {
      // Directly revoke without confirmation
      revokeDevice(device.id);
      setSelectedDevice(null);
    }
  };

  // Handle confirm revoke
  const handleConfirmRevoke = () => {
    if (selectedDevice) {
      revokeDevice(selectedDevice.id);
    }
    setRevokeModalOpen(false);
    setSelectedDevice(null);
  };

  return (
    <div className="devices-tab">
      {/* Header */}
      <div className="tab-header">
        <button
          className="btn btn-success"
          onClick={handlePairDevice}
          aria-label="Pair new device"
        >
          PAIR NEW DEVICE
        </button>
      </div>

      {/* Device Cards Grid */}
      {devices.length > 0 ? (
        <div className="device-grid">
          {devices.map((device) => (
            <DeviceCard
              key={device.id}
              device={device}
              onViewDetails={handleViewDetails}
              onRevoke={handleRevokeDevice}
            />
          ))}
        </div>
      ) : (
        <div className="empty-state">
          <h3>No Paired Devices</h3>
          <p className="text-secondary">
            No devices have been paired yet. Click "Pair New Device" to get started.
          </p>
        </div>
      )}

      {/* Pairing Modal */}
      <PairingModal
        isOpen={pairingModalOpen}
        onClose={() => setPairingModalOpen(false)}
        onPair={handlePairSuccess}
      />

      {/* Device Details Modal */}
      <DeviceDetailsModal
        isOpen={detailsModalOpen}
        onClose={() => {
          setDetailsModalOpen(false);
          setSelectedDevice(null);
        }}
        device={selectedDevice}
        onRevoke={handleRevokeDevice}
      />

      {/* Revoke Confirmation Modal */}
      <ConfirmationModal
        isOpen={revokeModalOpen}
        onClose={() => {
          setRevokeModalOpen(false);
          setSelectedDevice(null);
        }}
        onConfirm={handleConfirmRevoke}
        title="Revoke Device Access"
        message={`Are you sure you want to revoke access for ${selectedDevice?.name}?`}
        details={
          selectedDevice ? (
            <div className="confirmation-details">
              <div className="detail-row">
                <strong>Device:</strong> <span>{selectedDevice.name}</span>
              </div>
              <div className="detail-row">
                <strong>Platform:</strong> <span>{selectedDevice.platform} {selectedDevice.platformVersion}</span>
              </div>
              <div className="detail-row">
                <strong>Status:</strong> <span>{selectedDevice.status}</span>
              </div>
              <div className="detail-row">
                <strong>Last Seen:</strong> <span>{new Date(selectedDevice.lastSeen).toLocaleString()}</span>
              </div>
              <p style={{ marginTop: 'var(--spacing-md)', color: 'var(--color-error)' }}>
                This device will immediately lose access to all routes and services.
              </p>
            </div>
          ) : null
        }
        danger={true}
      />
    </div>
  );
}
