/**
 * Routes Demo Page
 *
 * Demonstrates the complete Routes tab functionality:
 * - DataContext integration
 * - Table component with sorting and searching
 * - Add/Edit/Delete modals
 * - Full CRUD operations
 */

import { useState } from 'react';
import { Plus } from 'lucide-react';
import { useData } from '../contexts/DataContext';
import { Table } from '../components/data-display';
import { Badge } from '../components/data-display';
import { ConfirmationModal, EditRouteModal } from '../components/modals';

/**
 * RoutesDemo Component
 */
export default function RoutesDemo() {
  const { routes, addRoute, updateRoute, deleteRoute } = useData();

  // Modal states
  const [isEditModalOpen, setIsEditModalOpen] = useState(false);
  const [isDeleteModalOpen, setIsDeleteModalOpen] = useState(false);
  const [selectedRoute, setSelectedRoute] = useState(null);

  // Handle edit
  const handleEdit = (route) => {
    setSelectedRoute(route);
    setIsEditModalOpen(true);
  };

  // Handle add new
  const handleAddNew = () => {
    setSelectedRoute(null);
    setIsEditModalOpen(true);
  };

  // Handle save (add or update)
  const handleSave = (routeData) => {
    if (selectedRoute) {
      // Update existing route
      updateRoute(selectedRoute.id, routeData);
    } else {
      // Add new route
      addRoute(routeData);
    }
  };

  // Handle delete click
  const handleDeleteClick = (route) => {
    setSelectedRoute(route);
    setIsDeleteModalOpen(true);
  };

  // Handle delete confirm
  const handleDeleteConfirm = () => {
    if (selectedRoute) {
      deleteRoute(selectedRoute.id);
    }
  };

  // Table columns
  const columns = [
    {
      key: 'application',
      label: 'APPLICATION',
      sortable: true
    },
    {
      key: 'path',
      label: 'PATH',
      sortable: true
    },
    {
      key: 'target',
      label: 'TARGET',
      sortable: true
    },
    {
      key: 'scopes',
      label: 'SCOPES',
      sortable: false,
      render: (row) => (
        <div style={{ display: 'flex', gap: 'var(--spacing-2)', flexWrap: 'wrap' }}>
          {row.scopes.map(scope => (
            <Badge key={scope} variant="primary" size="sm">
              {scope}
            </Badge>
          ))}
        </div>
      )
    },
    {
      key: 'status',
      label: 'STATUS',
      sortable: true,
      render: (row) => {
        let variant = 'success';
        let icon = null;

        if (row.status === 'PENDING') {
          variant = 'warning';
          icon = '⏸';
        } else if (row.status === 'INACTIVE') {
          variant = 'error';
          icon = '✕';
        }

        return (
          <Badge variant={variant} dot filled>
            {icon && <span aria-hidden="true">{icon}</span>}
            {row.status}
          </Badge>
        );
      }
    }
  ];

  return (
    <div className="routes-demo">
      <div className="box">
        <div className="box-header" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <span>ROUTES MANAGEMENT</span>
          <button className="btn btn-success btn-sm" onClick={handleAddNew}>
            <Plus size={16} />
            ADD NEW ROUTE
          </button>
        </div>
        <div className="box-content">
          <Table
            columns={columns}
            data={routes}
            onEdit={handleEdit}
            onDelete={handleDeleteClick}
            searchable
            sortable
          />
        </div>
      </div>

      {/* Edit/Add Modal */}
      <EditRouteModal
        isOpen={isEditModalOpen}
        onClose={() => setIsEditModalOpen(false)}
        onSave={handleSave}
        route={selectedRoute}
      />

      {/* Delete Confirmation Modal */}
      <ConfirmationModal
        isOpen={isDeleteModalOpen}
        onClose={() => setIsDeleteModalOpen(false)}
        onConfirm={handleDeleteConfirm}
        title="Delete Route"
        message="Are you sure you want to delete this route? This action cannot be undone."
        details={
          selectedRoute && (
            <dl>
              <dt>Application:</dt>
              <dd>{selectedRoute.application}</dd>
              <dt>Path:</dt>
              <dd>{selectedRoute.path}</dd>
              <dt>Target:</dt>
              <dd>{selectedRoute.target}</dd>
            </dl>
          )
        }
        confirmText="DELETE"
        cancelText="CANCEL"
        danger
      />
    </div>
  );
}
