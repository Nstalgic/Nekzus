/**
 * WebhookTesterModal Component
 *
 * Modal for testing webhook endpoints with custom payloads
 *
 * Features:
 * - Tab-based interface for activity/notify webhooks
 * - Device targeting (broadcast or specific devices)
 * - Real-time payload preview
 * - Success/error feedback via notifications
 * - Terminal aesthetic matching Nekzus design system
 */

import { useState, useEffect } from 'react';
import PropTypes from 'prop-types';
import { Modal } from './Modal';
import { devicesAPI, webhooksAPI } from '../../services/api';
import { useNotification } from '../../contexts/NotificationContext';
import styles from './WebhookTesterModal.module.css';

const WEBHOOK_TYPES = {
  ACTIVITY: 'activity',
  NOTIFY: 'notify',
};

const STYLE_OPTIONS = [
  { value: '', label: 'Default' },
  { value: 'success', label: 'Success' },
  { value: 'warning', label: 'Warning' },
  { value: 'danger', label: 'Danger' },
];

/**
 * WebhookTesterModal Component
 *
 * @param {object} props - Component props
 * @param {boolean} props.isOpen - Modal open state
 * @param {function} props.onClose - Close callback
 */
export function WebhookTesterModal({ isOpen, onClose }) {
  const { addNotification } = useNotification();
  const [activeTab, setActiveTab] = useState(WEBHOOK_TYPES.ACTIVITY);
  const [sending, setSending] = useState(false);
  const [devices, setDevices] = useState([]);
  const [loadingDevices, setLoadingDevices] = useState(false);

  // Activity webhook state
  const [message, setMessage] = useState('');
  const [iconClass, setIconClass] = useState('');
  const [details, setDetails] = useState('');

  // Notify webhook state
  const [notifyType, setNotifyType] = useState('custom_notification');
  const [notifyData, setNotifyData] = useState('{\n  "title": "Test Notification",\n  "body": "This is a test message from webhook",\n  "subtitle": "Optional subtitle",\n  "example": "data",\n  "value": 123\n}');

  // Device targeting
  const [targetMode, setTargetMode] = useState('broadcast'); // 'broadcast' or 'specific'
  const [selectedDeviceIds, setSelectedDeviceIds] = useState([]);

  // Load devices when modal opens
  useEffect(() => {
    if (isOpen) {
      loadDevices();
    }
  }, [isOpen]);

  const loadDevices = async () => {
    setLoadingDevices(true);
    try {
      const deviceList = await devicesAPI.list();
      setDevices(deviceList || []);
    } catch (error) {
      console.error('Failed to load devices:', error);
      addNotification({
        severity: 'error',
        message: 'Failed to load devices',
      });
      setDevices([]);
    } finally {
      setLoadingDevices(false);
    }
  };

  const handleDeviceToggle = (deviceId) => {
    setSelectedDeviceIds(prev => {
      if (prev.includes(deviceId)) {
        return prev.filter(id => id !== deviceId);
      } else {
        return [...prev, deviceId];
      }
    });
  };

  const handleSendWebhook = async () => {
    // Validation
    if (activeTab === WEBHOOK_TYPES.ACTIVITY && !message.trim()) {
      addNotification({
        severity: 'error',
        message: 'Message is required for activity webhooks',
      });
      return;
    }

    if (targetMode === 'specific' && selectedDeviceIds.length === 0) {
      addNotification({
        severity: 'error',
        message: 'Please select at least one device',
      });
      return;
    }

    // Build payload
    let payload;
    const deviceIds = targetMode === 'broadcast' ? [] : selectedDeviceIds;

    if (activeTab === WEBHOOK_TYPES.ACTIVITY) {
      payload = {
        message: message.trim(),
        iconClass: iconClass || undefined,
        details: details.trim() || undefined,
        deviceIds,
      };
    } else {
      // Notify webhook
      try {
        const parsedData = JSON.parse(notifyData);
        payload = {
          type: notifyType,
          data: parsedData,
          deviceIds,
        };
      } catch (error) {
        addNotification({
          severity: 'error',
          message: 'Invalid JSON in notify data',
        });
        return;
      }
    }

    // Send webhook
    setSending(true);
    try {
      if (activeTab === WEBHOOK_TYPES.ACTIVITY) {
        await webhooksAPI.sendActivity(payload);
        addNotification({
          severity: 'success',
          message: 'Activity webhook sent successfully',
        });
      } else {
        await webhooksAPI.sendNotify(payload);
        addNotification({
          severity: 'success',
          message: 'Notify webhook sent successfully',
        });
      }

      // Reset form on success
      resetForm();
    } catch (error) {
      console.error('Failed to send webhook:', error);
      addNotification({
        severity: 'error',
        message: `Failed to send webhook: ${error.message}`,
      });
    } finally {
      setSending(false);
    }
  };

  const resetForm = () => {
    setMessage('');
    setIconClass('');
    setDetails('');
    setNotifyType('custom_notification');
    setNotifyData('{\n  "title": "Test Notification",\n  "body": "This is a test message from webhook",\n  "subtitle": "Optional subtitle",\n  "example": "data",\n  "value": 123\n}');
    setTargetMode('broadcast');
    setSelectedDeviceIds([]);
  };

  const getPreviewPayload = () => {
    if (activeTab === WEBHOOK_TYPES.ACTIVITY) {
      const payload = {
        message: message || '(required)',
        iconClass: iconClass || undefined,
        details: details || undefined,
      };
      if (targetMode === 'specific') {
        payload.deviceIds = selectedDeviceIds;
      }
      // Remove undefined values for cleaner preview
      return Object.fromEntries(Object.entries(payload).filter(([_, v]) => v !== undefined));
    } else {
      try {
        const payload = {
          type: notifyType,
          data: JSON.parse(notifyData),
        };
        if (targetMode === 'specific') {
          payload.deviceIds = selectedDeviceIds;
        }
        return payload;
      } catch {
        return { error: 'Invalid JSON' };
      }
    }
  };

  // Reset state when modal closes
  const handleClose = () => {
    resetForm();
    setActiveTab(WEBHOOK_TYPES.ACTIVITY);
    onClose();
  };

  if (!isOpen) return null;

  return (
    <Modal isOpen={isOpen} onClose={handleClose} size="large">
      {/* Header */}
      <div className={styles.modalHeader}>
        <h2 className={styles.modalTitle}>WEBHOOK TESTER</h2>
        <button
          className={styles.modalCloseButton}
          onClick={handleClose}
          aria-label="Close modal"
          type="button"
        >
          ×
        </button>
      </div>

      {/* Body */}
      <div className={styles.modalBody}>
        <p className={styles.description}>
          Send test webhooks to mobile devices and WebSocket clients
        </p>

        {/* Tabs */}
        <div className={styles.tabContainer}>
          <button
            type="button"
            onClick={() => setActiveTab(WEBHOOK_TYPES.ACTIVITY)}
            className={`${styles.tab} ${activeTab === WEBHOOK_TYPES.ACTIVITY ? styles.active : ''}`}
          >
            ACTIVITY WEBHOOK
          </button>
          <button
            type="button"
            onClick={() => setActiveTab(WEBHOOK_TYPES.NOTIFY)}
            className={`${styles.tab} ${activeTab === WEBHOOK_TYPES.NOTIFY ? styles.active : ''}`}
          >
            NOTIFY WEBHOOK
          </button>
        </div>

        {/* Activity Webhook Form */}
        {activeTab === WEBHOOK_TYPES.ACTIVITY && (
          <div className={styles.formContainer}>
            {/* Message */}
            <div className={styles.formGroup}>
              <label htmlFor="message" className={styles.label}>
                MESSAGE <span className={styles.required}>*</span>
              </label>
              <input
                id="message"
                type="text"
                className="input"
                placeholder="Test notification message"
                value={message}
                onChange={(e) => setMessage(e.target.value)}
              />
            </div>

            {/* Style */}
            <div className={styles.formGroup}>
              <label htmlFor="iconClass" className={styles.label}>
                STYLE
              </label>
              <select
                id="iconClass"
                className="input"
                value={iconClass}
                onChange={(e) => setIconClass(e.target.value)}
              >
                {STYLE_OPTIONS.map(opt => (
                  <option key={opt.value} value={opt.value}>{opt.label}</option>
                ))}
              </select>
            </div>

            {/* Details */}
            <div className={styles.formGroup}>
              <label htmlFor="details" className={styles.label}>
                DETAILS (OPTIONAL)
              </label>
              <textarea
                id="details"
                className="input"
                placeholder="Additional details or context"
                rows={2}
                value={details}
                onChange={(e) => setDetails(e.target.value)}
              />
            </div>
          </div>
        )}

        {/* Notify Webhook Form */}
        {activeTab === WEBHOOK_TYPES.NOTIFY && (
          <div className={styles.formContainer}>
            {/* Type */}
            <div className={styles.formGroup}>
              <label htmlFor="notifyType" className={styles.label}>
                NOTIFICATION TYPE
              </label>
              <input
                id="notifyType"
                type="text"
                className="input"
                placeholder="custom_notification"
                value={notifyType}
                onChange={(e) => setNotifyType(e.target.value)}
              />
            </div>

            {/* JSON Data */}
            <div className={styles.formGroup}>
              <label htmlFor="notifyData" className={styles.label}>
                PAYLOAD DATA (JSON)
              </label>
              <textarea
                id="notifyData"
                className="input"
                placeholder='{"key": "value"}'
                rows={6}
                value={notifyData}
                onChange={(e) => setNotifyData(e.target.value)}
              />
            </div>
          </div>
        )}

        {/* Device Targeting Section */}
        <div className={styles.sectionDivider}>
          <h4 className={styles.sectionHeader}>
            TARGET DEVICES
          </h4>

          <div className={styles.targetOptions}>
            {/* Broadcast Option */}
            <label className={styles.radioLabel}>
              <input
                type="radio"
                name="targetMode"
                checked={targetMode === 'broadcast'}
                onChange={() => setTargetMode('broadcast')}
              />
              <span className={styles.radioText}>
                Broadcast to all devices
              </span>
            </label>

            {/* Specific Devices Option */}
            <label className={styles.radioLabel}>
              <input
                type="radio"
                name="targetMode"
                checked={targetMode === 'specific'}
                onChange={() => setTargetMode('specific')}
              />
              <span className={styles.radioText}>
                Target specific devices
              </span>
            </label>

            {/* Device List */}
            {targetMode === 'specific' && (
              <div className={styles.deviceList}>
                {loadingDevices ? (
                  <p className={styles.emptyState}>
                    Loading devices...
                  </p>
                ) : devices.length === 0 ? (
                  <p className={styles.emptyState}>
                    No paired devices found. Pair a device first.
                  </p>
                ) : (
                  <div className={styles.deviceItems}>
                    {devices.map(device => (
                      <label
                        key={device.id}
                        className={styles.deviceItem}
                      >
                        <input
                          type="checkbox"
                          checked={selectedDeviceIds.includes(device.id)}
                          onChange={() => handleDeviceToggle(device.id)}
                        />
                        <span className={styles.deviceName}>
                          {device.name || device.id}
                          <span className={styles.deviceId}>
                            {device.id}
                          </span>
                        </span>
                      </label>
                    ))}
                  </div>
                )}
              </div>
            )}
          </div>
        </div>

        {/* Payload Preview Section */}
        <div className={styles.previewContainer}>
          <h4 className={styles.sectionHeader}>
            PAYLOAD PREVIEW
          </h4>
          <pre className={styles.preview}>
            {JSON.stringify(getPreviewPayload(), null, 2)}
          </pre>
        </div>
      </div>

      {/* Footer */}
      <div className={styles.modalFooter}>
        <button
          className="btn btn-secondary"
          onClick={handleClose}
          disabled={sending}
          type="button"
        >
          CANCEL
        </button>
        <button
          className="btn btn-primary"
          onClick={handleSendWebhook}
          disabled={sending || (activeTab === WEBHOOK_TYPES.ACTIVITY && !message.trim())}
          type="button"
        >
          {sending ? 'SENDING...' : 'SEND WEBHOOK'}
        </button>
      </div>
    </Modal>
  );
}

WebhookTesterModal.propTypes = {
  isOpen: PropTypes.bool.isRequired,
  onClose: PropTypes.func.isRequired,
};
