/**
 * PairingModal Component
 *
 * Modal for pairing mobile devices via QR code.
 * Displays server URL, Nexus ID, and countdown timer.
 */

import { useState, useEffect, useCallback } from 'react';
import PropTypes from 'prop-types';
import { Modal } from './Modal';
import styles from './PairingModal.module.css';

/**
 * PairingModal Component
 *
 * @param {object} props - Component props
 * @param {boolean} props.isOpen - Whether modal is open
 * @param {function} props.onClose - Close callback
 * @param {function} props.onPair - Callback when device is paired (optional)
 * @param {string} props.serverUrl - Server URL for pairing
 * @param {string} props.qrCodeData - QR code data (ASCII art or placeholder)
 */
export function PairingModal({ isOpen, onClose, onPair, serverUrl, qrCodeData }) {
  const EXPIRATION_TIME = 5 * 60; // 5 minutes in seconds

  const [pairingData, setPairingData] = useState(null);
  const [qrImageUrl, setQrImageUrl] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const [remainingTime, setRemainingTime] = useState(EXPIRATION_TIME);
  const [isExpired, setIsExpired] = useState(false);
  const [isRedeemed, setIsRedeemed] = useState(false);
  const [isRefreshing, setIsRefreshing] = useState(false);

  // Fetch pairing data from API
  const fetchPairingData = useCallback(async () => {
    setLoading(true);
    setError(null);

    try {
      // Fetch JSON data for info display
      const response = await fetch('/api/v1/auth/qr');
      if (!response.ok) {
        throw new Error(`Failed to generate pairing code: ${response.statusText}`);
      }
      const data = await response.json();
      setPairingData(data);

      // Set QR code image URL (PNG format)
      setQrImageUrl(`/api/v1/auth/qr?format=png&t=${Date.now()}`);

      // Reset timer and status
      setRemainingTime(EXPIRATION_TIME);
      setIsExpired(false);
      setIsRedeemed(false);
    } catch (err) {
      console.error('Failed to fetch pairing data:', err);
      setError(err.message || 'Failed to generate pairing code');
    } finally {
      setLoading(false);
    }
  }, []);

  // Fetch pairing data when modal opens
  useEffect(() => {
    if (isOpen && !pairingData) {
      fetchPairingData();
    }
  }, [isOpen, pairingData, fetchPairingData]);

  // Countdown timer
  useEffect(() => {
    if (!isOpen || isExpired) return;

    const interval = setInterval(() => {
      setRemainingTime(prev => {
        if (prev <= 1) {
          setIsExpired(true);
          return 0;
        }
        return prev - 1;
      });
    }, 1000);

    return () => clearInterval(interval);
  }, [isOpen, isExpired]);

  // Reset timer when modal opens
  useEffect(() => {
    if (isOpen) {
      setRemainingTime(EXPIRATION_TIME);
      setIsExpired(false);
      setIsRedeemed(false);
    }
  }, [isOpen]);

  // Poll for code redemption status
  useEffect(() => {
    if (!isOpen || !pairingData?.code || isExpired || isRedeemed) return;

    const checkStatus = async () => {
      try {
        const response = await fetch(`/api/v1/auth/qr/status?code=${encodeURIComponent(pairingData.code)}`);
        if (response.ok) {
          const status = await response.json();
          if (status.redeemed) {
            setIsRedeemed(true);
            if (onPair) onPair();
          } else if (status.expired) {
            setIsExpired(true);
          }
        }
      } catch (err) {
        console.error('Failed to check pairing status:', err);
      }
    };

    // Poll every 2 seconds
    const interval = setInterval(checkStatus, 2000);

    return () => clearInterval(interval);
  }, [isOpen, pairingData?.code, isExpired, isRedeemed, onPair]);

  // Format time as MM:SS
  const formatTime = (seconds) => {
    const minutes = Math.floor(seconds / 60);
    const secs = seconds % 60;
    return `${minutes}:${secs.toString().padStart(2, '0')}`;
  };

  // Handle refresh button click
  const handleRefresh = async () => {
    setIsRefreshing(true);
    await fetchPairingData();
    setIsRefreshing(false);
  };

  // Handle close
  const handleClose = () => {
    setRemainingTime(EXPIRATION_TIME);
    setIsExpired(false);
    onClose();
  };

  return (
    <Modal isOpen={isOpen} onClose={handleClose} size="medium">
      <div className={styles.pairingModalCard}>
        {/* Header */}
        <div className={styles.pairingModalHeader}>
          <h3>DEVICE PAIRING</h3>
        </div>

        {/* Body */}
        <div className={styles.pairingModalBody}>
          {/* Loading State */}
          {loading && (
            <div className={styles.loadingState}>
              <div className="spinner">⟳</div>
              <p>Generating pairing code...</p>
            </div>
          )}

          {/* Error State */}
          {error && !loading && (
            <div className={styles.errorState}>
              <p className="error-message">{error}</p>
              <button className="btn btn-primary" onClick={handleRefresh}>
                TRY AGAIN
              </button>
            </div>
          )}

          {/* Success State */}
          {pairingData && !loading && !error && (
            <>
              {/* Instructions */}
              <div className={styles.pairingInstructions}>
                <p>Scan the QR code with the Nekzus mobile app to pair this device.</p>
              </div>

              {/* Info Box */}
              <div className={styles.pairingInfoBox}>
                <div className={styles.pairingInfoIcon}>
                  <span>✓</span>
                </div>
                <div className={styles.pairingInfoContent}>
                  <div className={styles.pairingInfoItem}>
                    <span className={styles.pairingInfoLabel}>Server:</span>
                    <span className={styles.pairingInfoValue}>{pairingData.qr?.u || serverUrl || window.location.origin}</span>
                  </div>
                </div>
              </div>

              {/* QR Code */}
              <div className={styles.pairingQrContainer}>
                <div className={styles.pairingQrBox}>
                  {qrImageUrl ? (
                    <img
                      src={qrImageUrl}
                      alt="Pairing QR Code"
                      className={styles.qrCodeImage}
                      style={{ maxWidth: '280px', maxHeight: '280px' }}
                    />
                  ) : (
                    <div className={styles.pairingQrPlaceholder}>
                      <pre className="qr-code-ascii">Loading QR Code...</pre>
                    </div>
                  )}
                </div>

                {/* Status */}
                <div className={styles.pairingQrStatus}>
                  <span className={styles.pairingStatusIndicator}>
                    <span className={`${styles.statusDot} ${isRedeemed ? styles.statusRedeemed : isExpired ? styles.statusExpired : styles.statusActive}`}></span>
                    <span className={`${styles.statusText} ${isRedeemed ? styles.redeemed : isExpired ? styles.expired : ''}`}>
                      {isRedeemed ? 'Paired Successfully' : isExpired ? 'QR Code Expired' : 'Active'}
                    </span>
                    {!isExpired && !isRedeemed && (
                      <span className={styles.statusTimer}>(expires in {formatTime(remainingTime)})</span>
                    )}
                  </span>
                </div>
              </div>
            </>
          )}
        </div>

        {/* Actions */}
        <div className={styles.pairingModalActions}>
          <button
            className="btn btn-secondary"
            onClick={handleClose}
            type="button"
          >
            CLOSE
          </button>
          <button
            className="btn btn-success"
            onClick={handleRefresh}
            disabled={isRefreshing}
            type="button"
          >
            <span className={styles.btnIcon}>⟳</span>
            {isRefreshing ? 'GENERATING...' : 'GENERATE NEW CODE'}
          </button>
        </div>
      </div>
    </Modal>
  );
}

PairingModal.propTypes = {
  isOpen: PropTypes.bool.isRequired,
  onClose: PropTypes.func.isRequired,
  onPair: PropTypes.func,
  serverUrl: PropTypes.string,
  qrCodeData: PropTypes.string
};
