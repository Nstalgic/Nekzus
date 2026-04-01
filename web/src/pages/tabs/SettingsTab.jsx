/**
 * SettingsTab Component
 *
 * Complete settings interface matching dashboard.html layout
 *
 * Features:
 * - 3x2 grid layout (6 setting boxes)
 * - General, Discovery, Security (top row)
 * - Appearance, Webhooks, Notifications (bottom row)
 * - Custom dropdown components matching dashboard.html
 * - Save and Reset functionality
 * - Certificate management section
 */

import { useState, useEffect } from 'react';
import { Save, RotateCcw, Trash2, Shield, Plus, Wand2 } from 'lucide-react';
import { Box } from '../../components/boxes';
import { Badge } from '../../components/data-display';
import { ThemeSwitcher } from '../../components/utility';
import CustomDropdown from '../../components/forms/CustomDropdown';
import { ConfirmationModal } from '../../components/modals/ConfirmationModal';
import { useSettings } from '../../contexts';
import { useNotification } from '../../contexts/NotificationContext';
import { WebhookTesterModal } from '../../components/modals/WebhookTesterModal';
import { certificatesAPI, apiKeysAPI } from '../../services/api';

/**
 * Copy text to clipboard
 * @param {string} text - Text to copy
 * @returns {Promise<boolean>} Success status
 */
const copyToClipboard = async (text) => {
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text);
      return true;
    } else {
      // Fallback for older browsers or non-HTTPS
      const textArea = document.createElement('textarea');
      textArea.value = text;
      textArea.style.position = 'fixed';
      textArea.style.left = '-999999px';
      document.body.appendChild(textArea);
      textArea.focus();
      textArea.select();
      const successful = document.execCommand('copy');
      document.body.removeChild(textArea);
      return successful;
    }
  } catch (err) {
    console.error('Failed to copy:', err);
    return false;
  }
};

/**
 * Format date for certificate display
 * @param {string} dateString - ISO date string
 * @returns {string} Formatted date (YYYY-MM-DD)
 */
const formatDate = (dateString) => {
  if (!dateString) return 'N/A';
  const date = new Date(dateString);
  return date.toISOString().split('T')[0];
};

/**
 * Get expiry status color
 * @param {number} daysRemaining - Days until expiry
 * @returns {string} Color variable name
 */
const getExpiryColor = (daysRemaining) => {
  if (daysRemaining < 7) return 'var(--color-error)';
  if (daysRemaining < 30) return 'var(--color-warning)';
  return 'var(--color-success)';
};

/**
 * Validate domain for local development
 * @param {string} domain - Domain to validate
 * @returns {boolean} True if valid local domain
 */
const isLocalDomain = (domain) => {
  const trimmed = domain.trim().toLowerCase();
  if (!trimmed) return false;

  // Allow .local, .lan, localhost
  if (trimmed.endsWith('.local') || trimmed.endsWith('.lan') || trimmed === 'localhost') {
    return true;
  }

  // Allow IP addresses
  const ipv4Regex = /^(\d{1,3}\.){3}\d{1,3}$/;
  if (ipv4Regex.test(trimmed)) {
    return true;
  }

  return false;
};

/**
 * SettingsTab Component
 */
export function SettingsTab() {
  const { settings, updateSetting, resetSettings, defaults } = useSettings();
  const { addNotification } = useNotification();
  const [hasChanges, setHasChanges] = useState(false);
  const [localSettings, setLocalSettings] = useState(settings);
  const [copyFeedback, setCopyFeedback] = useState({ url: false, key: false });
  const [webhookTestStatus, setWebhookTestStatus] = useState(null); // 'success' | 'error' | null
  const [isWebhookTesterOpen, setIsWebhookTesterOpen] = useState(false);

  // Sync localSettings when context settings change (e.g., after reset or external update)
  useEffect(() => {
    if (!hasChanges) {
      setLocalSettings(settings);
    }
  }, [settings, hasChanges]);

  // Certificate state
  const [certificates, setCertificates] = useState([]);
  const [loadingCerts, setLoadingCerts] = useState(false);
  const [generatingCert, setGeneratingCert] = useState(false);
  const [deletingCert, setDeletingCert] = useState(null);
  const [showGenerateForm, setShowGenerateForm] = useState(false);
  const [newDomains, setNewDomains] = useState('');
  const [certToDelete, setCertToDelete] = useState(null);

  // Load certificates on mount
  useEffect(() => {
    loadCertificates();
  }, []);

  // Validate and clean up stale webhook keys on mount
  useEffect(() => {
    const validateWebhookKey = async () => {
      // If there's a webhook key but no backend key ID, it's an old client-generated key
      if (settings.webhookKey && !settings.webhookKeyId) {
        console.log('Clearing old client-side generated webhook key');
        updateSetting('webhookKey', '');
        updateSetting('webhookKeyId', '');
        setLocalSettings((prev) => ({ ...prev, webhookKey: '' }));
        return;
      }

      // If there's a key ID, validate it still exists in the database
      if (settings.webhookKeyId) {
        try {
          await apiKeysAPI.get(settings.webhookKeyId);
          // Key exists, nothing to do
        } catch (error) {
          // Key doesn't exist in database (fresh setup or database reset)
          console.log('Webhook key not found in database, clearing cached key');
          updateSetting('webhookKey', '');
          updateSetting('webhookKeyId', '');
          setLocalSettings((prev) => ({ ...prev, webhookKey: '' }));
        }
      }
    };

    validateWebhookKey();
  }, []); // Only run once on mount

  // Load certificates from API
  const loadCertificates = async () => {
    setLoadingCerts(true);
    try {
      const data = await certificatesAPI.list();
      setCertificates(data.certificates || []);
    } catch (error) {
      console.error('Failed to load certificates:', error);
      addNotification({
        severity: 'error',
        message: `Failed to load certificates: ${error.message}`,
        strongText: 'ERROR:'
      });
    } finally {
      setLoadingCerts(false);
    }
  };

  // Handle generate certificate
  const handleGenerateCertificate = async () => {
    // Parse domains from input
    const domains = newDomains
      .split(',')
      .map(d => d.trim())
      .filter(d => d.length > 0);

    // Validate domains
    if (domains.length === 0) {
      addNotification({
        severity: 'error',
        message: 'Please enter at least one domain',
        strongText: 'VALIDATION ERROR:'
      });
      return;
    }

    // Validate all domains are local
    const invalidDomains = domains.filter(d => !isLocalDomain(d));
    if (invalidDomains.length > 0) {
      addNotification({
        severity: 'error',
        message: `Invalid domains: ${invalidDomains.join(', ')}. Use .local, .lan, localhost, or IP addresses.`,
        strongText: 'VALIDATION ERROR:'
      });
      return;
    }

    setGeneratingCert(true);
    try {
      const response = await certificatesAPI.generate({
        domains,
        provider: 'self-signed'
      });

      // Check if TLS was upgraded (server switched from HTTP to HTTPS)
      if (response.tls_upgraded) {
        addNotification({
          severity: 'success',
          message: `Certificate generated! Server upgraded to HTTPS. Redirecting...`,
          strongText: 'TLS ENABLED:'
        });

        // Give user a moment to see the message, then redirect to HTTPS
        setTimeout(() => {
          const httpsUrl = window.location.href.replace('http://', 'https://');
          window.location.href = httpsUrl;
        }, 1500);
        return;
      }

      addNotification({
        severity: 'success',
        message: `Certificate generated for ${domains.join(', ')}`,
        strongText: 'SUCCESS:'
      });

      // Reset form and reload certificates
      setNewDomains('');
      setShowGenerateForm(false);
      await loadCertificates();
    } catch (error) {
      console.error('Failed to generate certificate:', error);
      addNotification({
        severity: 'error',
        message: `Failed to generate certificate: ${error.message}`,
        strongText: 'ERROR:'
      });
    } finally {
      setGeneratingCert(false);
    }
  };

  // Handle auto-config certificate generation
  const handleAutoConfig = async () => {
    setGeneratingCert(true);
    try {
      // Fetch suggested domains from the server
      let response;
      try {
        response = await certificatesAPI.suggest();
      } catch (suggestError) {
        console.error('Failed to fetch suggestions:', suggestError);
        addNotification({
          severity: 'error',
          message: `Failed to detect local domains: ${suggestError.message}`,
          strongText: 'ERROR:'
        });
        return;
      }

      const suggestions = response.suggestions || [];

      if (suggestions.length === 0) {
        addNotification({
          severity: 'warning',
          message: 'No local domains detected',
          strongText: 'AUTO CONFIG:'
        });
        return;
      }

      // Generate certificate with all suggested domains
      let generateResponse;
      try {
        generateResponse = await certificatesAPI.generate({
          domains: suggestions,
          provider: 'self-signed'
        });
      } catch (generateError) {
        console.error('Failed to generate certificate:', generateError);
        addNotification({
          severity: 'error',
          message: `Failed to generate certificate: ${generateError.message}`,
          strongText: 'ERROR:'
        });
        return;
      }

      // Check if TLS was upgraded (server switched from HTTP to HTTPS)
      if (generateResponse.tls_upgraded) {
        addNotification({
          severity: 'success',
          message: `Certificate generated! Server upgraded to HTTPS. Redirecting...`,
          strongText: 'TLS ENABLED:'
        });

        // Give user a moment to see the message, then redirect to HTTPS
        setTimeout(() => {
          const httpsUrl = window.location.href.replace('http://', 'https://');
          window.location.href = httpsUrl;
        }, 1500);
        return;
      }

      addNotification({
        severity: 'success',
        message: `Certificate generated for ${suggestions.join(', ')}`,
        strongText: 'AUTO CONFIG:'
      });

      // Reset form and reload certificates
      setNewDomains('');
      setShowGenerateForm(false);
      await loadCertificates();
    } catch (error) {
      console.error('Failed to auto-configure certificate:', error);
      addNotification({
        severity: 'error',
        message: `Auto-config failed: ${error.message}`,
        strongText: 'ERROR:'
      });
    } finally {
      setGeneratingCert(false);
    }
  };

  // Handle delete certificate (with confirmation if enabled)
  const handleDeleteCertificate = (domain) => {
    if (localSettings.requireConfirmation) {
      setCertToDelete(domain);
    } else {
      confirmDeleteCertificate(domain);
    }
  };

  // Confirm delete certificate
  const confirmDeleteCertificate = async (domain) => {
    const domainToDelete = domain || certToDelete;
    if (!domainToDelete) return;

    setDeletingCert(domainToDelete);
    try {
      await certificatesAPI.delete(domainToDelete);

      addNotification({
        severity: 'success',
        message: `Certificate for ${domainToDelete} deleted`,
        strongText: 'SUCCESS:'
      });

      await loadCertificates();
    } catch (error) {
      console.error('Failed to delete certificate:', error);
      addNotification({
        severity: 'error',
        message: `Failed to delete certificate: ${error.message}`,
        strongText: 'ERROR:'
      });
    } finally {
      setDeletingCert(null);
      setCertToDelete(null);
    }
  };

  // Handle setting change
  const handleChange = (key, value) => {
    setLocalSettings((prev) => ({ ...prev, [key]: value }));
    setHasChanges(true);
  };

  // Handle save
  const handleSave = () => {
    updateSetting(localSettings);
    setHasChanges(false);

    // Show success notification
    addNotification({
      severity: 'success',
      message: 'Settings saved successfully!',
      strongText: 'SAVED:'
    });
  };

  // Handle reset
  const handleReset = () => {
    if (confirm('Are you sure you want to reset all settings to defaults?')) {
      resetSettings();
      setLocalSettings(defaults); // Use defaults, not stale settings value
      setHasChanges(false);

      // Show reset notification
      addNotification({
        severity: 'info',
        message: 'All settings reset to defaults.',
        strongText: 'RESET:'
      });
    }
  };

  // Generate webhook URLs
  const getWebhookActivityUrl = () => {
    return `${window.location.origin}/api/v1/webhooks/activity`;
  };

  const getWebhookNotifyUrl = () => {
    return `${window.location.origin}/api/v1/webhooks/notify`;
  };

  // For display/copy purposes, show the activity endpoint
  const getWebhookUrl = () => {
    return getWebhookActivityUrl();
  };

  // Handle copy webhook URL
  const handleCopyUrl = async () => {
    const success = await copyToClipboard(getWebhookUrl());
    if (success) {
      setCopyFeedback(prev => ({ ...prev, url: true }));
      setTimeout(() => setCopyFeedback(prev => ({ ...prev, url: false })), 2000);
    }
  };

  // Handle copy webhook key
  const handleCopyKey = async () => {
    const success = await copyToClipboard(localSettings.webhookKey);
    if (success) {
      setCopyFeedback(prev => ({ ...prev, key: true }));
      setTimeout(() => setCopyFeedback(prev => ({ ...prev, key: false })), 2000);
    }
  };

  // Handle generate webhook key
  const handleGenerateKey = async () => {
    if (localSettings.webhookKey) {
      // Warn if regenerating existing key
      if (!confirm('Generating a new key will invalidate the existing key. Continue?')) {
        return;
      }

      // Revoke old key if it exists
      if (settings.webhookKeyId) {
        try {
          await apiKeysAPI.revoke(settings.webhookKeyId);
        } catch (error) {
          console.warn('Failed to revoke old key:', error);
        }
      }
    }

    try {
      // Create API key via backend
      const response = await apiKeysAPI.create({
        name: 'Webhook API Key',
        scopes: ['write:*'],
      });

      // Store the full key (only returned once!)
      const newKey = response.key;
      handleChange('webhookKey', newKey);

      // Also store the API key ID for future reference
      updateSetting('webhookKeyId', response.id);

      // Show success notification with warning
      addNotification({
        severity: 'success',
        message: 'API key generated successfully! This is the only time you will see the full key. Make sure to copy it now.',
        strongText: 'SUCCESS:',
      });

      // Auto-copy to clipboard
      await copyToClipboard(newKey);
      setCopyFeedback(prev => ({ ...prev, key: true }));
      setTimeout(() => setCopyFeedback(prev => ({ ...prev, key: false })), 3000);

    } catch (error) {
      console.error('Failed to generate API key:', error);
      addNotification({
        severity: 'error',
        message: `Failed to generate API key: ${error.message}`,
        strongText: 'ERROR:',
      });
    }
  };

  // Handle send test webhook
  const handleSendTestWebhook = async () => {
    setWebhookTestStatus('loading');

    // Show notification that test is starting
    addNotification({
      severity: 'info',
      message: 'Sending test webhook broadcast...'
    });

    try {
      // Send test activity webhook
      const response = await fetch(getWebhookActivityUrl(), {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          message: 'Test webhook broadcast from settings',
          icon: 'Send',
          iconClass: 'success',
          details: `Sent at ${new Date().toLocaleTimeString()}`,
          // deviceIds: [] // Empty = broadcast to all
        }),
      });

      if (!response.ok) {
        const errorText = await response.text();
        throw new Error(`HTTP ${response.status}: ${errorText}`);
      }

      const result = await response.json();

      // Success response
      setWebhookTestStatus('success');
      setTimeout(() => setWebhookTestStatus(null), 3000);

      // Show success notification
      addNotification({
        severity: 'success',
        message: 'Test webhook broadcast sent successfully!',
        strongText: 'SUCCESS:',
        link: {
          text: 'View activity',
          href: '#activity'
        }
      });

      console.log('Webhook test result:', result);
    } catch (error) {
      console.error('Webhook test failed:', error);
      setWebhookTestStatus('error');
      setTimeout(() => setWebhookTestStatus(null), 3000);

      // Show error notification
      addNotification({
        severity: 'error',
        message: `Webhook test failed: ${error.message}`,
        strongText: 'ERROR:',
        link: {
          text: 'Check settings',
          href: '#webhooks'
        }
      });
    }
  };

  // Handle export settings
  const handleExportSettings = () => {
    const dataStr = JSON.stringify(localSettings, null, 2);
    const dataBlob = new Blob([dataStr], { type: 'application/json' });
    const url = URL.createObjectURL(dataBlob);
    const link = document.createElement('a');
    link.href = url;
    link.download = 'nekzus-settings.json';
    link.click();
    URL.revokeObjectURL(url);
  };

  // Handle import settings
  const handleImportSettings = () => {
    const input = document.createElement('input');
    input.type = 'file';
    input.accept = 'application/json';
    input.onchange = (e) => {
      const file = e.target.files[0];
      if (file) {
        const reader = new FileReader();
        reader.onload = (event) => {
          try {
            const imported = JSON.parse(event.target.result);
            setLocalSettings(imported);
            setHasChanges(true);
            alert('Settings imported successfully! Click "Save Changes" to apply.');
          } catch (error) {
            alert('Failed to import settings: Invalid JSON file');
          }
        };
        reader.readAsText(file);
      }
    };
    input.click();
  };

  // Handle clear all data
  const handleClearAllData = () => {
    if (confirm('Are you sure you want to clear all local data? This will reset all settings and reload the page.')) {
      localStorage.clear();
      window.location.reload();
    }
  };

  // Handle webhook tester
  const handleOpenWebhookTester = () => {
    setIsWebhookTesterOpen(true);
  };

  const handleCloseWebhookTester = () => {
    setIsWebhookTesterOpen(false);
  };

  // Calculate local storage usage
  const getStorageUsage = () => {
    let total = 0;
    for (let key in localStorage) {
      if (localStorage.hasOwnProperty(key)) {
        total += localStorage[key].length + key.length;
      }
    }
    // Convert to KB
    const kb = (total / 1024).toFixed(2);
    return `${kb} KB`;
  };

  // Dropdown options
  const refreshIntervalOptions = [
    { value: '5', label: '5 seconds' },
    { value: '10', label: '10 seconds' },
    { value: '30', label: '30 seconds' },
    { value: '60', label: '60 seconds' },
    { value: '0', label: 'Manual only' },
  ];

  const timezoneOptions = [
    { value: 'UTC', label: 'UTC' },
    { value: 'America/New_York', label: 'America/New_York' },
    { value: 'America/Los_Angeles', label: 'America/Los_Angeles' },
    { value: 'America/Chicago', label: 'America/Chicago' },
    { value: 'Europe/London', label: 'Europe/London' },
    { value: 'Europe/Paris', label: 'Europe/Paris' },
    { value: 'Asia/Tokyo', label: 'Asia/Tokyo' },
    { value: 'Asia/Shanghai', label: 'Asia/Shanghai' },
  ];

  const autoApprovalOptions = [
    { value: '100', label: 'Disabled' },
    { value: '90', label: 'Verified only' },
    { value: '70', label: 'Detected or higher' },
  ];

  const badgeThresholdOptions = [
    { value: '1', label: 'Show at 1+' },
    { value: '3', label: 'Show at 3+' },
    { value: '5', label: 'Show at 5+' },
    { value: '10', label: 'Show at 10+' },
    { value: '0', label: 'Never show' },
  ];

  const sessionTimeoutOptions = [
    { value: '15', label: '15 minutes' },
    { value: '30', label: '30 minutes' },
    { value: '60', label: '1 hour' },
    { value: '120', label: '2 hours' },
    { value: '0', label: 'Never' },
  ];

  const fontSizeOptions = [
    { value: 'small', label: 'Small (12px)' },
    { value: 'medium', label: 'Medium (14px)' },
    { value: 'large', label: 'Large (16px)' },
  ];

  return (
    <div className="settings-tab">
      {/* Save/Reset Buttons */}
      <div className="settings-actions">
        <button
          className="btn btn-secondary"
          onClick={handleReset}
          aria-label="Reset all settings to defaults"
        >
          <RotateCcw size={16} />
          RESET TO DEFAULTS
        </button>
        <button
          className="btn btn-success"
          onClick={handleSave}
          disabled={!hasChanges}
          aria-label="Save settings"
        >
          <Save size={16} />
          SAVE CHANGES
        </button>
      </div>

      {/* Settings Grid - 3x2 layout */}
      <div className="settings-grid">
        {/* Row 1: GENERAL */}
        <Box title="GENERAL">
          <form className="settings-form">
            <div className="form-group">
              <label>DASHBOARD REFRESH INTERVAL</label>
              <CustomDropdown
                id="refreshIntervalDropdown"
                options={refreshIntervalOptions}
                value={String(localSettings.refreshInterval)}
                onChange={(val) => handleChange('refreshInterval', Number(val))}
              />
            </div>

            <div className="form-group">
              <label>TIMEZONE</label>
              <CustomDropdown
                id="timezoneDropdown"
                options={timezoneOptions}
                value={localSettings.timezone}
                onChange={(val) => handleChange('timezone', val)}
              />
            </div>

            <div className="form-group">
              <label className="checkbox-label">
                <input
                  type="checkbox"
                  className="checkbox"
                  checked={localSettings.showTimestamp}
                  onChange={(e) => handleChange('showTimestamp', e.target.checked)}
                />
                <span>Show timestamp in footer</span>
              </label>
            </div>

            <div className="form-group">
              <label className="checkbox-label">
                <input
                  type="checkbox"
                  className="checkbox"
                  checked={localSettings.enableToolbox}
                  onChange={(e) => handleChange('enableToolbox', e.target.checked)}
                />
                <span>Enable Toolbox tab</span>
              </label>
            </div>

            <div className="form-group">
              <label className="checkbox-label">
                <input
                  type="checkbox"
                  className="checkbox"
                  checked={localSettings.enableScripts}
                  onChange={(e) => handleChange('enableScripts', e.target.checked)}
                />
                <span>Enable Scripts tab</span>
              </label>
            </div>

            <div className="form-group">
              <label className="checkbox-label">
                <input
                  type="checkbox"
                  className="checkbox"
                  checked={localSettings.enableFederation}
                  onChange={(e) => handleChange('enableFederation', e.target.checked)}
                />
                <span>Enable Federation tab</span>
              </label>
            </div>

            <div className="form-group">
              <label className="checkbox-label">
                <input
                  type="checkbox"
                  className="checkbox"
                  checked={localSettings.showOnlyRoutedContainers}
                  onChange={(e) => handleChange('showOnlyRoutedContainers', e.target.checked)}
                />
                <span>Show only containers with routes</span>
              </label>
            </div>
          </form>
        </Box>

        {/* Row 1: DISCOVERY */}
        <Box title="DISCOVERY">
          <form className="settings-form">
            <div className="form-group">
              <label>AUTO-APPROVAL THRESHOLD</label>
              <CustomDropdown
                id="autoApprovalDropdown"
                options={autoApprovalOptions}
                value={String(localSettings.autoApprovalThreshold)}
                onChange={(val) => handleChange('autoApprovalThreshold', Number(val))}
              />
            </div>

            <div className="form-group">
              <label>NOTIFICATION BADGE THRESHOLD</label>
              <CustomDropdown
                id="badgeThresholdDropdown"
                options={badgeThresholdOptions}
                value={String(localSettings.notificationBadgeThreshold)}
                onChange={(val) => handleChange('notificationBadgeThreshold', Number(val))}
              />
            </div>

            <div className="form-group">
              <label className="checkbox-label">
                <input
                  type="checkbox"
                  className="checkbox"
                  checked={localSettings.requireConfirmationForRejections}
                  onChange={(e) => handleChange('requireConfirmationForRejections', e.target.checked)}
                />
                <span>Require confirmation for rejections</span>
              </label>
            </div>
          </form>
        </Box>

        {/* Row 1: SECURITY */}
        <Box title="SECURITY">
          <form className="settings-form">
            <div className="form-group">
              <label>SESSION TIMEOUT</label>
              <CustomDropdown
                id="sessionTimeoutDropdown"
                options={sessionTimeoutOptions}
                value={String(localSettings.sessionTimeout)}
                onChange={(val) => handleChange('sessionTimeout', Number(val))}
              />
            </div>

            <div className="form-group">
              <label className="checkbox-label">
                <input
                  type="checkbox"
                  className="checkbox"
                  checked={localSettings.requireConfirmation}
                  onChange={(e) => handleChange('requireConfirmation', e.target.checked)}
                />
                <span>Require confirmation for destructive actions</span>
              </label>
            </div>
          </form>
        </Box>

        {/* Row 2: APPEARANCE */}
        <Box title="APPEARANCE">
          <form className="settings-form">
            <div className="form-group">
              <label>TERMINAL THEME</label>
              <ThemeSwitcher />
            </div>

            <div className="form-group">
              <label>FONT SIZE</label>
              <CustomDropdown
                id="fontSizeDropdown"
                options={fontSizeOptions}
                value={localSettings.fontSize}
                onChange={(val) => handleChange('fontSize', val)}
              />
            </div>

            <div className="form-group">
              <label className="checkbox-label">
                <input
                  type="checkbox"
                  className="checkbox"
                  id="compactModeToggle"
                  checked={localSettings.compactMode}
                  onChange={(e) => handleChange('compactMode', e.target.checked)}
                />
                <span>Compact mode (reduced spacing)</span>
              </label>
            </div>
          </form>
        </Box>

        {/* Row 2: WEBHOOKS */}
        <Box title="WEBHOOKS">
          <form className="settings-form">
            {/* Webhook URL */}
            <div className="form-group">
              <label>WEBHOOK URL</label>
              <div style={{ display: 'flex', gap: 'var(--space-2)' }}>
                <input
                  type="text"
                  className="input"
                  value={getWebhookUrl()}
                  readOnly
                  style={{ flex: 1 }}
                />
                <button
                  type="button"
                  className="btn btn-secondary"
                  onClick={handleCopyUrl}
                  title="Copy webhook URL"
                  style={{ minWidth: '80px' }}
                >
                  {copyFeedback.url ? 'COPIED' : 'COPY'}
                </button>
              </div>
            </div>

            {/* Webhook Key */}
            <div className="form-group">
              <label>WEBHOOK KEY</label>
              <div style={{ display: 'flex', gap: 'var(--space-2)' }}>
                <input
                  type="password"
                  className="input"
                  value={localSettings.webhookKey || ''}
                  readOnly
                  placeholder="No key generated yet"
                  style={{ flex: 1 }}
                />
                {localSettings.webhookKey && (
                  <button
                    type="button"
                    className="btn btn-secondary"
                    onClick={handleCopyKey}
                    title="Copy webhook key"
                    style={{ minWidth: '80px' }}
                  >
                    {copyFeedback.key ? 'COPIED' : 'COPY'}
                  </button>
                )}
              </div>
            </div>

            {/* Generate/Regenerate Key Button */}
            <div className="form-group">
              <button
                type="button"
                className="btn btn-primary"
                onClick={handleGenerateKey}
                style={{ width: '100%' }}
              >
                {localSettings.webhookKey ? 'REGENERATE' : 'GENERATE KEY'}
              </button>
              {localSettings.webhookKey && (
                <p className="text-secondary" style={{ fontSize: '11px', marginTop: 'var(--space-2)' }}>
                  Warning: Regenerating will revoke the existing key
                </p>
              )}
            </div>

            {/* Send Test Webhook */}
            <div className="form-group">
              <button
                type="button"
                className={`btn ${webhookTestStatus === 'success' ? 'btn-success' : webhookTestStatus === 'error' ? 'btn-error' : 'btn-secondary'}`}
                onClick={handleSendTestWebhook}
                disabled={webhookTestStatus === 'loading'}
                style={{ width: '100%' }}
              >
                {webhookTestStatus === 'loading' && <span>SENDING...</span>}
                {webhookTestStatus === 'success' && <span>TEST SENT</span>}
                {webhookTestStatus === 'error' && <span>FAILED</span>}
                {!webhookTestStatus && <span>SEND TEST WEBHOOK</span>}
              </button>
            </div>
          </form>
        </Box>

        {/* Row 2: NOTIFICATIONS */}
        <Box title="NOTIFICATIONS">
          <form className="settings-form">
            <div className="form-group">
              <label className="checkbox-label">
                <input
                  type="checkbox"
                  className="checkbox"
                  checked={localSettings.notifyNewDiscoveries}
                  onChange={(e) => handleChange('notifyNewDiscoveries', e.target.checked)}
                />
                <span>New discoveries</span>
              </label>
            </div>

            <div className="form-group">
              <label className="checkbox-label">
                <input
                  type="checkbox"
                  className="checkbox"
                  checked={localSettings.notifyDeviceOffline}
                  onChange={(e) => handleChange('notifyDeviceOffline', e.target.checked)}
                />
                <span>Device offline alerts</span>
              </label>
            </div>

            <div className="form-group">
              <label className="checkbox-label">
                <input
                  type="checkbox"
                  className="checkbox"
                  checked={localSettings.notifyCertificateExpiry}
                  onChange={(e) => handleChange('notifyCertificateExpiry', e.target.checked)}
                />
                <span>Certificate expiration warnings</span>
              </label>
            </div>

            <div className="form-group">
              <label className="checkbox-label">
                <input
                  type="checkbox"
                  className="checkbox"
                  checked={localSettings.notifyRouteStatusChange}
                  onChange={(e) => handleChange('notifyRouteStatusChange', e.target.checked)}
                />
                <span>Route status changes</span>
              </label>
            </div>

            <div className="form-group">
              <label className="checkbox-label">
                <input
                  type="checkbox"
                  className="checkbox"
                  checked={localSettings.notifySystemHealth}
                  onChange={(e) => handleChange('notifySystemHealth', e.target.checked)}
                />
                <span>System health alerts</span>
              </label>
            </div>

            {/* Test notification buttons */}
            <div className="form-group" style={{ borderTop: '1px solid var(--border)', paddingTop: 'var(--space-4)', marginTop: 'var(--space-4)' }}>
              <label style={{ marginBottom: 'var(--space-2)', display: 'block', color: 'var(--text-tertiary)', fontSize: 'var(--text-xs)' }}>
                TEST NOTIFICATIONS
              </label>
              <div style={{ display: 'flex', gap: 'var(--space-2)', flexWrap: 'wrap' }}>
                <button
                  type="button"
                  className="btn btn-small btn-secondary"
                  onClick={() => addNotification({
                    severity: 'info',
                    message: 'This is a test info notification'
                  })}
                >
                  INFO
                </button>
                <button
                  type="button"
                  className="btn btn-small btn-success"
                  onClick={() => addNotification({
                    severity: 'success',
                    message: 'Settings saved successfully!',
                    strongText: 'SUCCESS:'
                  })}
                >
                  SUCCESS
                </button>
                <button
                  type="button"
                  className="btn btn-small"
                  style={{ borderColor: 'var(--warning)', color: 'var(--warning)' }}
                  onClick={() => addNotification({
                    severity: 'warning',
                    message: '3 certificates expiring within 7 days.',
                    strongText: 'WARNING:',
                    link: {
                      text: 'View details',
                      href: '#certificates'
                    }
                  })}
                >
                  WARNING
                </button>
                <button
                  type="button"
                  className="btn btn-small btn-danger"
                  onClick={() => addNotification({
                    severity: 'error',
                    message: 'Failed to connect to discovery service',
                    strongText: 'ERROR:',
                    link: {
                      text: 'Troubleshoot',
                      href: '#discovery'
                    }
                  })}
                >
                  ERROR
                </button>
              </div>
            </div>
          </form>
        </Box>
      </div>

      {/* Certificates Section */}
      <Box title="CERTIFICATES" style={{ marginTop: 'var(--spacing-lg)' }}>
        <p className="text-secondary" style={{ marginBottom: 'var(--spacing-lg)', fontSize: '13px' }}>
          Manage TLS certificates for secure service communication
        </p>

        {/* Loading State */}
        {loadingCerts && (
          <div style={{
            padding: 'var(--spacing-lg)',
            textAlign: 'center',
            color: 'var(--text-secondary)'
          }}>
            Loading certificates...
          </div>
        )}

        {/* Certificate List */}
        {!loadingCerts && certificates.length > 0 && (
          <div style={{ marginBottom: 'var(--spacing-lg)' }}>
            {certificates.map((cert) => (
              <div
                key={cert.domain}
                style={{
                  border: '2px solid var(--border-color)',
                  borderRadius: '0',
                  padding: 'var(--spacing-md)',
                  marginBottom: 'var(--spacing-md)',
                  backgroundColor: 'var(--bg-tertiary)'
                }}
              >
                {/* Domain and Expiry Info */}
                <div style={{
                  display: 'grid',
                  gridTemplateColumns: '1fr auto',
                  gap: 'var(--spacing-md)',
                  marginBottom: 'var(--spacing-sm)',
                  alignItems: 'start'
                }}>
                  <div>
                    <div style={{
                      fontFamily: 'var(--font-mono)',
                      fontSize: '14px',
                      fontWeight: 'bold',
                      color: 'var(--text-primary)',
                      marginBottom: 'var(--spacing-xs)'
                    }}>
                      {cert.domain.toUpperCase()}
                    </div>
                    <div style={{
                      fontSize: '12px',
                      color: 'var(--text-secondary)',
                      marginBottom: 'var(--spacing-xs)'
                    }}>
                      Issuer: {cert.issuer}
                    </div>
                  </div>
                  <div style={{ textAlign: 'right' }}>
                    <div style={{
                      fontSize: '12px',
                      color: 'var(--text-secondary)',
                      marginBottom: 'var(--spacing-xs)'
                    }}>
                      Expires: {formatDate(cert.not_after)}
                    </div>
                    <div style={{
                      fontSize: '12px',
                      fontWeight: 'bold',
                      color: getExpiryColor(cert.expires_in_days)
                    }}>
                      {cert.expires_in_days < 0 ? 'EXPIRED' :
                       cert.expires_in_days < 1 ? 'EXPIRES TODAY' :
                       `${Math.floor(cert.expires_in_days)} days remaining`}
                      {cert.expires_in_days > 0 && cert.expires_in_days < 7 && ' - EXPIRING SOON'}
                    </div>
                  </div>
                </div>

                {/* Fingerprint and Actions */}
                <div style={{
                  display: 'flex',
                  justifyContent: 'space-between',
                  alignItems: 'center',
                  borderTop: '1px solid var(--border-dim)',
                  paddingTop: 'var(--spacing-sm)',
                  gap: 'var(--spacing-md)'
                }}>
                  <div style={{
                    fontFamily: 'var(--font-mono)',
                    fontSize: '11px',
                    color: 'var(--text-tertiary)',
                    wordBreak: 'break-all',
                    flex: 1
                  }}>
                    {cert.fingerprint ?
                      `Fingerprint: ${cert.fingerprint.substring(0, 24)}...` :
                      'No fingerprint available'}
                  </div>
                  <button
                    className="btn btn-error btn-small"
                    onClick={() => handleDeleteCertificate(cert.domain)}
                    disabled={deletingCert === cert.domain}
                    style={{ flexShrink: 0 }}
                  >
                    {deletingCert === cert.domain ? (
                      'DELETING...'
                    ) : (
                      <>
                        <Trash2 size={14} />
                        DELETE
                      </>
                    )}
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}

        {/* Empty State */}
        {!loadingCerts && certificates.length === 0 && !showGenerateForm && (
          <div style={{
            padding: 'var(--spacing-lg)',
            textAlign: 'center',
            border: '2px dashed var(--border-dim)',
            borderRadius: '0',
            marginBottom: 'var(--spacing-lg)'
          }}>
            <Shield size={32} style={{
              color: 'var(--text-tertiary)',
              marginBottom: 'var(--spacing-sm)'
            }} />
            <p className="text-secondary" style={{ marginBottom: 'var(--spacing-md)' }}>
              No certificates configured
            </p>
            <button
              className="btn btn-primary"
              onClick={() => setShowGenerateForm(true)}
            >
              <Plus size={16} />
              GENERATE CERTIFICATE
            </button>
          </div>
        )}

        {/* Generate Certificate Form */}
        {showGenerateForm && (
          <div style={{
            border: '2px solid var(--accent-cyan)',
            borderRadius: '0',
            padding: 'var(--spacing-md)',
            marginBottom: 'var(--spacing-lg)',
            backgroundColor: 'var(--bg-secondary)'
          }}>
            <h4 style={{
              marginBottom: 'var(--spacing-md)',
              color: 'var(--text-primary)'
            }}>
              GENERATE NEW CERTIFICATE
            </h4>

            {/* Auto Config Option */}
            <div style={{
              padding: 'var(--spacing-md)',
              marginBottom: 'var(--spacing-md)',
              backgroundColor: 'var(--bg-tertiary)',
              border: '1px solid var(--border-color)'
            }}>
              <div style={{
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'center',
                marginBottom: 'var(--spacing-xs)'
              }}>
                <span style={{ fontWeight: 'bold', color: 'var(--text-primary)' }}>
                  AUTO CONFIG
                </span>
                <button
                  className="btn btn-primary"
                  onClick={handleAutoConfig}
                  disabled={generatingCert}
                  style={{ minWidth: '140px' }}
                >
                  {generatingCert ? (
                    'GENERATING...'
                  ) : (
                    <>
                      <Wand2 size={16} />
                      AUTO GENERATE
                    </>
                  )}
                </button>
              </div>
              <p style={{
                fontSize: '11px',
                color: 'var(--text-tertiary)',
                margin: 0
              }}>
                Automatically detect hostname and local IP addresses
              </p>
            </div>

            {/* Divider */}
            <div style={{
              display: 'flex',
              alignItems: 'center',
              margin: 'var(--spacing-md) 0',
              color: 'var(--text-tertiary)',
              fontSize: '12px'
            }}>
              <div style={{ flex: 1, height: '1px', backgroundColor: 'var(--border-color)' }} />
              <span style={{ padding: '0 var(--spacing-sm)' }}>OR ENTER MANUALLY</span>
              <div style={{ flex: 1, height: '1px', backgroundColor: 'var(--border-color)' }} />
            </div>

            <div className="form-group">
              <label style={{
                display: 'block',
                marginBottom: 'var(--spacing-xs)',
                fontSize: '12px',
                color: 'var(--text-secondary)'
              }}>
                DOMAINS (comma-separated)
              </label>
              <input
                type="text"
                className="input"
                placeholder="app.local, service.lan, 192.168.1.100"
                value={newDomains}
                onChange={(e) => setNewDomains(e.target.value)}
                disabled={generatingCert}
                style={{
                  width: '100%',
                  marginBottom: 'var(--spacing-sm)',
                  fontFamily: 'var(--font-mono)'
                }}
              />
              <p style={{
                fontSize: '11px',
                color: 'var(--text-tertiary)',
                marginBottom: 'var(--spacing-md)'
              }}>
                Use .local or .lan domains (e.g., app.local, service.lan) or IP addresses
              </p>
              <div style={{ display: 'flex', gap: 'var(--spacing-sm)' }}>
                <button
                  className="btn btn-secondary"
                  onClick={() => {
                    setShowGenerateForm(false);
                    setNewDomains('');
                  }}
                  disabled={generatingCert}
                  style={{ flex: 1 }}
                >
                  CANCEL
                </button>
                <button
                  className="btn btn-primary"
                  onClick={handleGenerateCertificate}
                  disabled={generatingCert || !newDomains.trim()}
                  style={{ flex: 1 }}
                >
                  {generatingCert ? (
                    'GENERATING...'
                  ) : (
                    <>
                      <Shield size={16} />
                      GENERATE
                    </>
                  )}
                </button>
              </div>
            </div>
          </div>
        )}

        {/* Generate Button (when not in form mode and has certs) */}
        {!loadingCerts && certificates.length > 0 && !showGenerateForm && (
          <button
            className="btn btn-primary"
            onClick={() => setShowGenerateForm(true)}
            style={{ width: '100%' }}
          >
            <Plus size={16} />
            GENERATE NEW CERTIFICATE
          </button>
        )}
      </Box>

      {/* Data Management Section */}
      <Box title="DATA MANAGEMENT" style={{ marginTop: 'var(--spacing-lg)' }}>
        <p className="text-secondary" style={{ marginBottom: 'var(--spacing-lg)' }}>
          Export, import, and manage your data
        </p>

        {/* Export Settings */}
        <div className="data-management-item">
          <div className="data-management-info">
            <h4 style={{ marginBottom: 'var(--spacing-xs)' }}>Export Settings</h4>
            <p className="text-secondary">Download your settings as a JSON file</p>
          </div>
          <button className="btn btn-primary" onClick={handleExportSettings}>
            EXPORT
          </button>
        </div>

        {/* Import Settings */}
        <div className="data-management-item">
          <div className="data-management-info">
            <h4 style={{ marginBottom: 'var(--spacing-xs)' }}>Import Settings</h4>
            <p className="text-secondary">Restore settings from a JSON file</p>
          </div>
          <button className="btn btn-primary" onClick={handleImportSettings}>
            IMPORT
          </button>
        </div>

        {/* Clear All Data */}
        <div className="data-management-item">
          <div className="data-management-info">
            <h4 style={{ marginBottom: 'var(--spacing-xs)' }}>Clear All Data</h4>
            <p className="text-secondary">
              Remove all local data and reset to defaults (requires reload)
            </p>
          </div>
          <button className="btn btn-error" onClick={handleClearAllData}>
            CLEAR DATA
          </button>
        </div>
      </Box>

      {/* System Information Section */}
      <Box title="SYSTEM INFORMATION" style={{ marginTop: 'var(--spacing-lg)' }}>
        <p className="text-secondary" style={{ marginBottom: 'var(--spacing-lg)' }}>
          View system details and status
        </p>

        {/* Frontend Version */}
        <div className="system-info-item">
          <div className="system-info-left">
            <h4 style={{ marginBottom: 'var(--spacing-xs)' }}>Frontend Version</h4>
            <p className="text-secondary">Web dashboard version</p>
          </div>
          <div className="system-info-right">
            <code className="system-info-value">{__APP_VERSION__}</code>
          </div>
        </div>

        {/* Backend Version */}
        <div className="system-info-item">
          <div className="system-info-left">
            <h4 style={{ marginBottom: 'var(--spacing-xs)' }}>Backend Version</h4>
            <p className="text-secondary">Nekzus server version</p>
          </div>
          <div className="system-info-right">
            <code className="system-info-value">{__APP_VERSION__}</code>
          </div>
        </div>

        {/* Connection Status */}
        <div className="system-info-item">
          <div className="system-info-left">
            <h4 style={{ marginBottom: 'var(--spacing-xs)' }}>Connection Status</h4>
            <p className="text-secondary">WebSocket connection state</p>
          </div>
          <div className="system-info-right">
            <Badge variant="success">● Connected</Badge>
          </div>
        </div>

        {/* Webhook ID */}
        <div className="system-info-item">
          <div className="system-info-left">
            <h4 style={{ marginBottom: 'var(--spacing-xs)' }}>Webhook ID</h4>
            <p className="text-secondary">Unique identifier for webhooks</p>
          </div>
          <div className="system-info-right">
            <code className="system-info-value" style={{ color: 'var(--color-accent)' }}>
              {localSettings.webhookId}
            </code>
          </div>
        </div>

        {/* Local Storage Usage */}
        <div className="system-info-item">
          <div className="system-info-left">
            <h4 style={{ marginBottom: 'var(--spacing-xs)' }}>Local Storage Usage</h4>
            <p className="text-secondary">Space used for cached data</p>
          </div>
          <div className="system-info-right">
            <code className="system-info-value">{getStorageUsage()}</code>
          </div>
        </div>
      </Box>

      {/* Developer Options Section */}
      <Box title="DEVELOPER OPTIONS" style={{ marginTop: 'var(--spacing-lg)' }}>
        <p className="text-secondary" style={{ marginBottom: 'var(--spacing-lg)', fontSize: '13px' }}>
          Advanced settings for developers
        </p>

        {/* Debug Mode */}
        <div className="system-info-item">
          <div className="system-info-left">
            <h4 style={{ marginBottom: 'var(--spacing-xs)' }}>Debug Mode</h4>
            <p className="text-secondary">Enable verbose logging in console</p>
          </div>
          <div className="system-info-right">
            <label className="toggle-switch">
              <input
                type="checkbox"
                checked={localSettings.debugMode}
                onChange={(e) => handleChange('debugMode', e.target.checked)}
              />
              <span className="toggle-slider"></span>
            </label>
          </div>
        </div>

        {/* Show Error Details */}
        <div className="system-info-item">
          <div className="system-info-left">
            <h4 style={{ marginBottom: 'var(--spacing-xs)' }}>Show Error Details</h4>
            <p className="text-secondary">Display detailed error information in UI</p>
          </div>
          <div className="system-info-right">
            <label className="toggle-switch">
              <input
                type="checkbox"
                checked={localSettings.showErrorDetails}
                onChange={(e) => handleChange('showErrorDetails', e.target.checked)}
              />
              <span className="toggle-slider"></span>
            </label>
          </div>
        </div>

        {/* Log WebSocket Events */}
        <div className="system-info-item">
          <div className="system-info-left">
            <h4 style={{ marginBottom: 'var(--spacing-xs)' }}>Log WebSocket Events</h4>
            <p className="text-secondary">Log all WebSocket messages to console</p>
          </div>
          <div className="system-info-right">
            <label className="toggle-switch">
              <input
                type="checkbox"
                checked={localSettings.logWebSocketEvents}
                onChange={(e) => handleChange('logWebSocketEvents', e.target.checked)}
              />
              <span className="toggle-slider"></span>
            </label>
          </div>
        </div>

        {/* Webhook Testing */}
        <div className="system-info-item" style={{ borderBottom: 'none' }}>
          <div className="system-info-left">
            <h4 style={{ marginBottom: 'var(--spacing-xs)' }}>Webhook Testing</h4>
            <p className="text-secondary">Send test webhooks to mobile devices</p>
          </div>
          <div className="system-info-right">
            <button className="btn btn-primary" onClick={handleOpenWebhookTester}>
              OPEN TESTER
            </button>
          </div>
        </div>
      </Box>

      {/* Webhook Tester Modal */}
      <WebhookTesterModal
        isOpen={isWebhookTesterOpen}
        onClose={handleCloseWebhookTester}
      />

      {/* Certificate Delete Confirmation Modal */}
      <ConfirmationModal
        isOpen={!!certToDelete}
        onClose={() => setCertToDelete(null)}
        onConfirm={() => confirmDeleteCertificate()}
        title="DELETE CERTIFICATE"
        message={`Are you sure you want to delete the certificate for ${certToDelete}?`}
        details={
          <div style={{
            fontFamily: 'var(--font-mono)',
            fontSize: '12px',
            color: 'var(--text-secondary)'
          }}>
            This action cannot be undone. You will need to generate a new certificate.
          </div>
        }
        confirmText="DELETE"
        cancelText="CANCEL"
        danger={true}
      />
    </div>
  );
}
