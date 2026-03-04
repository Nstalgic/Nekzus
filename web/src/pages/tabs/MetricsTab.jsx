/**
 * MetricsTab Component
 *
 * Prometheus metrics dashboard for Nekzus
 *
 * Features:
 * - Real-time metrics display from /api/v1/metrics/dashboard
 * - Category-based organization (HTTP, Proxy, WebSocket, Auth, etc.)
 * - Visual stat cards with terminal aesthetic
 * - Auto-refresh capability
 */

import { useState, useEffect, useMemo, useCallback } from 'react';
import { RefreshCw, Activity, TrendingUp, TrendingDown, Minus } from 'lucide-react';
import { Box } from '../../components/boxes';
import { Badge } from '../../components/data-display';
import { useSettings } from '../../contexts';

/**
 * Format bytes to human readable
 */
const formatBytes = (bytes) => {
  if (!bytes || bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(2))} ${sizes[i]}`;
};

/**
 * Format duration to human readable
 */
const formatDuration = (seconds) => {
  if (!seconds || seconds === 0) return '0s';
  if (seconds < 60) return `${seconds.toFixed(1)}s`;
  if (seconds < 3600) return `${(seconds / 60).toFixed(1)}m`;
  return `${(seconds / 3600).toFixed(1)}h`;
};

/**
 * Format number with K/M/B suffixes
 */
const formatNumber = (num) => {
  if (!num && num !== 0) return '0';
  if (num >= 1000000000) return `${(num / 1000000000).toFixed(1)}B`;
  if (num >= 1000000) return `${(num / 1000000).toFixed(1)}M`;
  if (num >= 1000) return `${(num / 1000).toFixed(1)}K`;
  return num.toString();
};

/**
 * Metric Card Component
 */
const MetricCard = ({ label, value, unit, trend, variant = 'default' }) => {
  const getTrendIcon = () => {
    if (trend === 'up') return <TrendingUp size={12} />;
    if (trend === 'down') return <TrendingDown size={12} />;
    return <Minus size={12} />;
  };

  const getTrendColor = () => {
    if (variant === 'error' || variant === 'warning') {
      return trend === 'up' ? 'var(--color-error)' : 'var(--color-success)';
    }
    return trend === 'up' ? 'var(--color-success)' : trend === 'down' ? 'var(--color-error)' : 'var(--text-secondary)';
  };

  return (
    <div className="metric-card">
      <div className="metric-label">{label}</div>
      <div className="metric-value-wrapper">
        <span className="metric-value" style={{ color: variant !== 'default' ? `var(--color-${variant})` : undefined }}>
          {value}
        </span>
        {unit && <span className="metric-unit">{unit}</span>}
      </div>
      {trend && (
        <div className="metric-trend" style={{ color: getTrendColor() }}>
          {getTrendIcon()}
        </div>
      )}
    </div>
  );
};

/**
 * Stat Row Component
 */
const StatRow = ({ label, value, badge }) => (
  <div className="stat-row">
    <span className="stat-label">{label}</span>
    <span className="stat-value">
      {badge ? badge : value}
    </span>
  </div>
);

/**
 * Progress Meter Component
 */
const ProgressMeter = ({ label, value, max, unit = '%', variant = 'primary' }) => {
  const percentage = max > 0 ? (value / max) * 100 : 0;

  return (
    <div className="progress-meter">
      <div className="progress-meter-header">
        <span className="progress-meter-label">{label}</span>
        <span className="progress-meter-value">
          {value.toFixed(1)}{unit}
        </span>
      </div>
      <div className="progress-meter-bar">
        <div
          className={`progress-meter-fill progress-meter-fill-${variant}`}
          style={{ width: `${Math.min(percentage, 100)}%` }}
        />
      </div>
    </div>
  );
};

/**
 * MetricsTab Component
 */
export function MetricsTab() {
  const { settings } = useSettings();
  const [metrics, setMetrics] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [lastUpdate, setLastUpdate] = useState(null);
  const [autoRefresh, setAutoRefresh] = useState(true);

  const fetchMetrics = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const response = await fetch('/api/v1/metrics/dashboard');
      if (!response.ok) {
        throw new Error(`Failed to fetch metrics: ${response.status}`);
      }
      const data = await response.json();
      setMetrics(data);
      setLastUpdate(new Date());
    } catch (err) {
      console.error('Failed to fetch metrics:', err);
      setError(err.message);
    } finally {
      setLoading(false);
    }
  }, []);

  // Initial fetch
  useEffect(() => {
    fetchMetrics();
  }, [fetchMetrics]);

  // Auto-refresh
  useEffect(() => {
    if (!autoRefresh) return;

    const interval = setInterval(() => {
      fetchMetrics();
    }, (settings?.refreshInterval || 30) * 1000);

    return () => clearInterval(interval);
  }, [autoRefresh, settings?.refreshInterval, fetchMetrics]);

  // Derived metrics
  const derivedMetrics = useMemo(() => {
    if (!metrics) return null;

    const http = metrics.http || {};
    const proxy = metrics.proxy || {};
    const websocket = metrics.websocket || {};
    const auth = metrics.auth || {};

    return {
      http: {
        errorRate: http.totalRequests > 0 ? http.errorRate || 0 : 0,
      },
      proxy: {
        errorRate: proxy.totalRequests > 0
          ? ((proxy.upstreamErrors || 0) / proxy.totalRequests * 100).toFixed(1)
          : 0,
      },
      websocket: {
        errorRate: websocket.totalConnections > 0 ? 0 : 0,
      },
      auth: {
        jwtSuccessRate: auth.jwtValidations > 0
          ? (((auth.jwtValidations - auth.jwtFailures) / auth.jwtValidations) * 100).toFixed(1)
          : 100,
        pairingSuccessRate: (auth.pairingSuccess + auth.pairingFailure) > 0
          ? ((auth.pairingSuccess / (auth.pairingSuccess + auth.pairingFailure)) * 100).toFixed(1)
          : 100,
      },
    };
  }, [metrics]);

  if (loading && !metrics) {
    return (
      <div className="metrics-tab">
        <div className="metrics-loading">
          <Activity size={32} className="spinning" />
          <p>Loading metrics...</p>
        </div>
      </div>
    );
  }

  if (error && !metrics) {
    return (
      <div className="metrics-tab">
        <div className="metrics-loading">
          <p>Error loading metrics: {error}</p>
          <button className="btn btn-primary" onClick={fetchMetrics}>
            Retry
          </button>
        </div>
      </div>
    );
  }

  const http = metrics?.http || {};
  const proxy = metrics?.proxy || {};
  const websocket = metrics?.websocket || {};
  const auth = metrics?.auth || {};
  const discovery = metrics?.discovery || {};
  const health = metrics?.health || {};
  const system = metrics?.system || {};

  return (
    <div className="metrics-tab">
      {/* Header Controls */}
      <div className="metrics-header">
        <div className="metrics-header-info">
          {lastUpdate && (
            <span className="text-secondary">
              Last updated: {lastUpdate.toLocaleTimeString()}
            </span>
          )}
        </div>
        <div className="metrics-header-actions">
          <label className="checkbox-label">
            <input
              type="checkbox"
              className="checkbox"
              checked={autoRefresh}
              onChange={(e) => setAutoRefresh(e.target.checked)}
            />
            <span>Auto-refresh</span>
          </label>
          <button
            className="btn btn-primary btn-sm"
            onClick={fetchMetrics}
            disabled={loading}
            aria-label="Refresh metrics"
          >
            <RefreshCw size={14} className={loading ? 'spinning' : ''} />
            REFRESH
          </button>
        </div>
      </div>

      {/* HTTP Metrics */}
      <Box title="HTTP METRICS">
        <div className="metrics-grid">
          <MetricCard
            label="Total Requests"
            value={formatNumber(http.totalRequests || 0)}
            variant="primary"
          />
          <MetricCard
            label="In Flight"
            value={formatNumber(http.inFlightRequests || 0)}
            variant="info"
          />
          <MetricCard
            label="Avg Latency"
            value={(http.avgLatencyMs || 0).toFixed(1)}
            unit="ms"
            variant="default"
          />
          <MetricCard
            label="Error Rate"
            value={(derivedMetrics?.http?.errorRate || 0).toFixed(1)}
            unit="%"
            variant={parseFloat(derivedMetrics?.http?.errorRate || 0) > 5 ? 'error' : 'success'}
          />
        </div>

        {/* Compact grid for secondary stats */}
        <div className="metrics-compact-grid">
          {/* Latency Percentiles */}
          <div className="metrics-compact-section">
            <h4 className="metrics-compact-title">Latency Percentiles</h4>
            <div className="metrics-compact-stats">
              <div className="compact-stat">
                <span className="compact-stat-label">p50</span>
                <span className="compact-stat-value">{(http.p50LatencyMs || 0).toFixed(1)}ms</span>
              </div>
              <div className="compact-stat">
                <span className="compact-stat-label">p95</span>
                <span className="compact-stat-value">{(http.p95LatencyMs || 0).toFixed(1)}ms</span>
              </div>
              <div className="compact-stat">
                <span className="compact-stat-label">p99</span>
                <span className="compact-stat-value">{(http.p99LatencyMs || 0).toFixed(1)}ms</span>
              </div>
            </div>
          </div>

          {/* By Status Code */}
          <div className="metrics-compact-section">
            <h4 className="metrics-compact-title">By Status Code</h4>
            <div className="metrics-compact-stats">
              {Object.entries(http.byStatus || {}).map(([status, count]) => (
                <div key={status} className="compact-stat">
                  <span className="compact-stat-label">{status}</span>
                  <Badge
                    variant={status.startsWith('2') ? 'success' : status.startsWith('4') ? 'warning' : status.startsWith('5') ? 'error' : 'info'}
                    size="sm"
                  >
                    {formatNumber(count)}
                  </Badge>
                </div>
              ))}
            </div>
          </div>

          {/* By Method */}
          <div className="metrics-compact-section">
            <h4 className="metrics-compact-title">By Method</h4>
            <div className="metrics-compact-stats">
              {Object.entries(http.byMethod || {}).map(([method, count]) => (
                <div key={method} className="compact-stat">
                  <span className="compact-stat-label">{method}</span>
                  <span className="compact-stat-value">{formatNumber(count)}</span>
                </div>
              ))}
            </div>
          </div>
        </div>
      </Box>

      {/* Proxy Metrics */}
      <Box title="PROXY METRICS">
        <div className="metrics-grid">
          <MetricCard
            label="Total Requests"
            value={formatNumber(proxy.totalRequests || 0)}
            variant="primary"
          />
          <MetricCard
            label="Active Sessions"
            value={formatNumber(proxy.activeSessions || 0)}
            variant="info"
          />
          <MetricCard
            label="Bytes In/Out"
            value={`${formatBytes(proxy.bytesIn || 0)} / ${formatBytes(proxy.bytesOut || 0)}`}
            variant="default"
          />
          <MetricCard
            label="Upstream Errors"
            value={formatNumber(proxy.upstreamErrors || 0)}
            variant={(proxy.upstreamErrors || 0) > 10 ? 'error' : 'success'}
          />
        </div>

        {proxy.byApp && proxy.byApp.length > 0 && (
          <div className="metrics-section">
            <h4 className="metrics-section-title">By Application</h4>
            <div className="app-metrics-list">
              {proxy.byApp.map((app) => (
                <div key={app.appId} className="app-metric-item">
                  <div className="app-metric-header">
                    <span className="app-metric-name">{app.appId}</span>
                    <span className="app-metric-count">{formatNumber(app.requests)} req</span>
                  </div>
                  <div className="app-metric-stats">
                    <span className="text-secondary">{app.errors} errors</span>
                    <span className="text-secondary">{formatBytes(app.bytesIn + app.bytesOut)} transferred</span>
                  </div>
                </div>
              ))}
            </div>
          </div>
        )}
      </Box>

      {/* WebSocket Metrics */}
      <Box title="WEBSOCKET METRICS">
        <div className="metrics-grid">
          <MetricCard
            label="Active Connections"
            value={formatNumber(websocket.activeConnections || 0)}
            variant="primary"
          />
          <MetricCard
            label="Total Connections"
            value={formatNumber(websocket.totalConnections || 0)}
            variant="info"
          />
          <MetricCard
            label="Avg Duration"
            value={formatDuration(websocket.avgDurationSec || 0)}
            variant="default"
          />
          <MetricCard
            label="Messages"
            value={formatNumber(websocket.totalMessages || 0)}
            variant="info"
          />
        </div>

        <div className="metrics-section">
          <h4 className="metrics-section-title">Data Transfer</h4>
          <div className="stats-list">
            <StatRow label="Bytes In" value={formatBytes(websocket.totalBytesIn || 0)} />
            <StatRow label="Bytes Out" value={formatBytes(websocket.totalBytesOut || 0)} />
          </div>
        </div>
      </Box>

      {/* Three-column layout for smaller metric boxes */}
      <div className="three-column">
        {/* Auth Metrics */}
        <Box title="AUTH METRICS">
          <div className="metrics-section">
            <h4 className="metrics-section-title">JWT Validations</h4>
            <ProgressMeter
              label="Success Rate"
              value={parseFloat(derivedMetrics?.auth?.jwtSuccessRate || 100)}
              max={100}
              variant="success"
            />
            <div className="stats-list" style={{ marginTop: 'var(--spacing-md)' }}>
              <StatRow label="Validations" value={formatNumber(auth.jwtValidations || 0)} />
              <StatRow label="Failures" value={formatNumber(auth.jwtFailures || 0)} />
            </div>
          </div>

          <div className="metrics-section">
            <h4 className="metrics-section-title">Device Pairing</h4>
            <ProgressMeter
              label="Success Rate"
              value={parseFloat(derivedMetrics?.auth?.pairingSuccessRate || 100)}
              max={100}
              variant="success"
            />
            <div className="stats-list" style={{ marginTop: 'var(--spacing-md)' }}>
              <StatRow label="Success" value={formatNumber(auth.pairingSuccess || 0)} />
              <StatRow label="Failures" value={formatNumber(auth.pairingFailure || 0)} />
            </div>
          </div>

          <div className="metrics-section">
            <StatRow label="Token Refreshes" value={formatNumber(auth.tokenRefreshes || 0)} />
            <StatRow label="Local Auth Bypasses" value={formatNumber(auth.localAuthBypasses || 0)} />
            <StatRow
              label="Active Bootstrap"
              value={auth.activeBootstrap || 0}
              badge={<Badge variant="info" filled>{auth.activeBootstrap || 0}</Badge>}
            />
          </div>
        </Box>

        {/* Discovery Metrics */}
        <Box title="DISCOVERY">
          <div className="metrics-section">
            <div className="stats-list">
              <StatRow label="Total Scans" value={formatNumber(discovery.totalScans || 0)} />
              <StatRow label="Total Proposals" value={formatNumber(discovery.totalProposals || 0)} />
              <StatRow
                label="Pending"
                value={discovery.pendingProposals || 0}
                badge={
                  (discovery.pendingProposals || 0) > 0
                    ? <Badge variant="warning" filled>{discovery.pendingProposals}</Badge>
                    : <Badge variant="success">{discovery.pendingProposals || 0}</Badge>
                }
              />
              <StatRow label="Active Workers" value={discovery.activeWorkers || 0} />
            </div>
          </div>

          {discovery.bySource && discovery.bySource.length > 0 && (
            <div className="metrics-section">
              <h4 className="metrics-section-title">By Source</h4>
              <div className="stats-list">
                {discovery.bySource.map((src) => (
                  <StatRow
                    key={src.source}
                    label={src.source}
                    value={`${src.scans} scans / ${src.proposals} proposals`}
                  />
                ))}
              </div>
            </div>
          )}
        </Box>

        {/* Health & System Metrics */}
        <Box title="HEALTH & SYSTEM">
          <div className="metrics-section">
            <h4 className="metrics-section-title">Health Checks</h4>
            <ProgressMeter
              label="Uptime"
              value={health.uptimePercent || 100}
              max={100}
              variant="success"
            />
            <div className="stats-list" style={{ marginTop: 'var(--spacing-md)' }}>
              <StatRow label="Total Checks" value={formatNumber(health.totalChecks || 0)} />
              <StatRow label="Healthy" value={formatNumber(health.healthyChecks || 0)} />
            </div>
          </div>

          <div className="metrics-section">
            <h4 className="metrics-section-title">System</h4>
            <div className="stats-list">
              <StatRow label="Uptime" value={formatDuration(system.uptimeSeconds || 0)} />
              <StatRow label="Config Reloads" value={formatNumber(system.configReloads || 0)} />
              <StatRow
                label="Last Reload"
                badge={
                  <Badge variant={system.lastReloadStatus === 1 ? 'success' : system.lastReloadStatus === 2 ? 'error' : 'info'}>
                    {system.lastReloadStatus === 1 ? 'OK' : system.lastReloadStatus === 2 ? 'ERROR' : 'N/A'}
                  </Badge>
                }
              />
              <StatRow label="Certificates" value={system.certificatesTotal || 0} />
              <StatRow label="Notification Queue" value={system.notificationQueue || 0} />
            </div>
          </div>
        </Box>
      </div>
    </div>
  );
}

export default MetricsTab;
