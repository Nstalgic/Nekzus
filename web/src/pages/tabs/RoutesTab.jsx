import { useState } from 'react';
import PropTypes from 'prop-types';
import { Plus, ExternalLink } from 'lucide-react';
import { useData } from '../../contexts/DataContext';
import { useSettings } from '../../contexts/SettingsContext';
import { Button } from '../../components/buttons';
import { Table } from '../../components/data-display';
import { EditRouteModal, ConfirmationModal } from '../../components/modals';
import { Badge } from '../../components/data-display';

/**
 * RoutesTab Component
 *
 * Complete routes management interface with:
 * - Route listing table with sorting and searching
 * - Add new route functionality
 * - Edit existing routes
 * - Delete routes with confirmation
 * - Real-time search and filtering
 *
 * @component
 * @returns {JSX.Element} Routes tab content
 */
export function RoutesTab() {
  const { routes, addRoute, updateRoute, deleteRoute, searchRoutes } = useData();
  const { settings } = useSettings();

  // Modal states
  const [editModalOpen, setEditModalOpen] = useState(false);
  const [deleteModalOpen, setDeleteModalOpen] = useState(false);
  const [selectedRoute, setSelectedRoute] = useState(null);

  // Handle add new route
  const handleAddRoute = () => {
    setSelectedRoute(null);
    setEditModalOpen(true);
  };

  // Handle edit route
  const handleEditRoute = (route) => {
    setSelectedRoute(route);
    setEditModalOpen(true);
  };

  // Handle delete route
  const handleDeleteRoute = (route) => {
    setSelectedRoute(route);

    // Check if confirmation is required
    if (settings.requireConfirmation) {
      setDeleteModalOpen(true);
    } else {
      // Directly delete without confirmation
      deleteRoute(route.routeId);
      setSelectedRoute(null);
    }
  };

  // Handle save (add or update)
  const handleSave = (formData) => {
    if (selectedRoute) {
      updateRoute(selectedRoute.routeId, formData);
    } else {
      addRoute(formData);
    }
    setEditModalOpen(false);
    setSelectedRoute(null);
  };

  // Handle confirm delete
  const handleConfirmDelete = () => {
    if (selectedRoute) {
      deleteRoute(selectedRoute.routeId);
    }
    setDeleteModalOpen(false);
    setSelectedRoute(null);
  };

  // Table column configuration
  const columns = [
    {
      key: 'appId',
      label: 'Application',
      sortable: true,
      render: (row) => <strong>{row.appId}</strong>
    },
    {
      key: 'pathBase',
      label: 'Path',
      sortable: true,
      render: (row) => (
        <a
          href={`${window.location.origin}${row.pathBase}`}
          target="_blank"
          rel="noopener noreferrer"
          className="route-path-link"
          title={`Open ${row.pathBase} in new tab`}
          style={{
            display: 'inline-flex',
            alignItems: 'center',
            gap: 'var(--spacing-xs)',
            color: 'var(--text-primary)',
            textDecoration: 'none',
            transition: 'color var(--transition-fast)',
          }}
          onMouseEnter={(e) => e.currentTarget.style.color = 'var(--accent-primary)'}
          onMouseLeave={(e) => e.currentTarget.style.color = 'var(--text-primary)'}
        >
          <code>{row.pathBase}</code>
          <ExternalLink size={14} style={{ opacity: 0.6 }} />
        </a>
      )
    },
    {
      key: 'to',
      label: 'Target',
      sortable: true,
      render: (row) => <span className="text-secondary">{row.to}</span>
    },
    {
      key: 'scopes',
      label: 'Scopes',
      sortable: false,
      render: (row) => (
        <div style={{ display: 'flex', gap: 'var(--spacing-xs)', flexWrap: 'wrap' }}>
          {Array.isArray(row.scopes) && row.scopes.length > 0 ? (
            row.scopes.map((scope) => (
              <Badge key={scope} variant="primary" size="sm">
                {scope}
              </Badge>
            ))
          ) : (
            <span className="text-secondary">-</span>
          )}
        </div>
      )
    },
    {
      key: 'status',
      label: 'Status',
      sortable: true,
      render: (row) => {
        // Default status to ACTIVE if not provided by backend
        const status = row.status || 'ACTIVE';
        return (
          <Badge
            variant={status === 'ACTIVE' ? 'success' : status === 'INACTIVE' ? 'default' : status === 'PENDING' ? 'warning' : 'error'}
            dot={true}
            filled={true}
            role="status"
          >
            {status === 'PENDING' && <span aria-hidden="true">⚠ </span>}
            {status === 'OFFLINE' && <span aria-hidden="true">✕ </span>}
            {status === 'ACTIVE' && <span className="sr-only">Status: </span>}
            {status}
          </Badge>
        );
      }
    }
  ];

  return (
    <div className="routes-tab">
      {/* Header with Add Route button */}
      <div className="tab-header">
        <Button
          variant="primary"
          onClick={handleAddRoute}
          icon={<Plus size={16} />}
        >
          Add Route
        </Button>
      </div>

      {/* Routes Table */}
      <div className="routes-table-container">
        <Table
          columns={columns}
          data={routes}
          onEdit={handleEditRoute}
          onDelete={handleDeleteRoute}
          searchable={true}
          sortable={true}
          defaultSortColumn="appId"
          defaultSortDirection="asc"
        />
      </div>

      {/* Edit/Add Route Modal */}
      <EditRouteModal
        isOpen={editModalOpen}
        onClose={() => {
          setEditModalOpen(false);
          setSelectedRoute(null);
        }}
        onSave={handleSave}
        route={selectedRoute}
      />

      {/* Delete Confirmation Modal */}
      <ConfirmationModal
        isOpen={deleteModalOpen}
        onClose={() => {
          setDeleteModalOpen(false);
          setSelectedRoute(null);
        }}
        onConfirm={handleConfirmDelete}
        title="DELETE ROUTE"
        message="WARNING: This will immediately stop all traffic to this endpoint. This action cannot be undone."
        details={
          selectedRoute ? (
            <>
              <div>
                <strong>Application:</strong>
                <span>{selectedRoute.appId}</span>
              </div>
              <div>
                <strong>Path:</strong>
                <code>{selectedRoute.pathBase}</code>
              </div>
              <div>
                <strong>Target:</strong>
                <span>{selectedRoute.to}</span>
              </div>
            </>
          ) : null
        }
        confirmText="DELETE ROUTE"
        cancelText="CANCEL"
        danger={true}
      />
    </div>
  );
}

RoutesTab.propTypes = {};

export default RoutesTab;
