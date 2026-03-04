/**
 * Table Component - Advanced data table with sorting and searching
 *
 * Features:
 * - Column-based data display
 * - Sorting (click header to sort, show indicator)
 * - Searching (filter input at top)
 * - Responsive (card layout on mobile using data-label attributes)
 * - Row actions (Edit and Delete buttons)
 * - Empty state and loading state
 * - Keyboard navigation support
 */

import { useState, useMemo } from 'react';
import PropTypes from 'prop-types';
import { Search, ChevronUp, ChevronDown, Loader } from 'lucide-react';
import styles from './Table.module.css';

/**
 * Table Component
 *
 * @param {object} props - Component props
 * @param {Array} props.columns - Column definitions
 *   Each column: { key, label, sortable, render }
 * @param {Array} props.data - Data rows
 * @param {function} [props.onEdit] - Edit handler (receives row)
 * @param {function} [props.onDelete] - Delete handler (receives row)
 * @param {boolean} [props.searchable=false] - Enable search filter
 * @param {boolean} [props.sortable=true] - Enable sorting
 * @param {boolean} [props.loading=false] - Show loading state
 * @param {string} [props.emptyMessage='No data available'] - Empty state message
 * @param {string} [props.defaultSortColumn] - Default column to sort by
 * @param {string} [props.defaultSortDirection='asc'] - Default sort direction ('asc' or 'desc')
 *
 * @example
 * const columns = [
 *   { key: 'name', label: 'Name', sortable: true },
 *   { key: 'status', label: 'Status', render: (row) => <Badge>{row.status}</Badge> }
 * ];
 *
 * <Table
 *   columns={columns}
 *   data={routes}
 *   onEdit={handleEdit}
 *   onDelete={handleDelete}
 *   searchable
 *   defaultSortColumn="name"
 * />
 */
export function Table({
  columns,
  data,
  onEdit,
  onDelete,
  editLabel = 'EDIT',
  searchable = false,
  sortable = true,
  loading = false,
  emptyMessage = 'No data available',
  defaultSortColumn = null,
  defaultSortDirection = 'asc'
}) {
  const [searchQuery, setSearchQuery] = useState('');
  const [sortColumn, setSortColumn] = useState(defaultSortColumn);
  const [sortDirection, setSortDirection] = useState(defaultSortDirection);

  // Handle column sort
  const handleSort = (columnKey) => {
    if (!sortable) return;

    const column = columns.find(col => col.key === columnKey);
    if (!column || column.sortable === false) return;

    if (sortColumn === columnKey) {
      // Toggle direction
      setSortDirection(prev => prev === 'asc' ? 'desc' : 'asc');
    } else {
      // New column, default to ascending
      setSortColumn(columnKey);
      setSortDirection('asc');
    }
  };

  // Filter and sort data
  const processedData = useMemo(() => {
    let result = [...data];

    // Apply search filter
    if (searchQuery && searchQuery.trim()) {
      const query = searchQuery.toLowerCase();
      result = result.filter(row => {
        return columns.some(column => {
          const value = row[column.key];
          if (value === null || value === undefined) return false;

          // Handle arrays (like scopes)
          if (Array.isArray(value)) {
            return value.some(item =>
              String(item).toLowerCase().includes(query)
            );
          }

          // Handle strings and numbers
          return String(value).toLowerCase().includes(query);
        });
      });
    }

    // Apply sorting
    if (sortColumn) {
      result.sort((a, b) => {
        const aVal = a[sortColumn];
        const bVal = b[sortColumn];

        // Handle null/undefined
        if (aVal === null || aVal === undefined) return 1;
        if (bVal === null || bVal === undefined) return -1;

        // Handle arrays
        if (Array.isArray(aVal) && Array.isArray(bVal)) {
          const aStr = aVal.join(',');
          const bStr = bVal.join(',');
          return sortDirection === 'asc'
            ? aStr.localeCompare(bStr)
            : bStr.localeCompare(aStr);
        }

        // Handle strings
        if (typeof aVal === 'string' && typeof bVal === 'string') {
          return sortDirection === 'asc'
            ? aVal.localeCompare(bVal)
            : bVal.localeCompare(aVal);
        }

        // Handle numbers
        if (sortDirection === 'asc') {
          return aVal > bVal ? 1 : -1;
        }
        return aVal < bVal ? 1 : -1;
      });
    }

    return result;
  }, [data, columns, searchQuery, sortColumn, sortDirection]);

  // Render sort indicator
  const renderSortIndicator = (columnKey) => {
    if (!sortable) return null;

    const column = columns.find(col => col.key === columnKey);
    if (!column || column.sortable === false) return null;

    if (sortColumn === columnKey) {
      return sortDirection === 'asc' ? (
        <ChevronUp size={14} className={`${styles.sortIndicator} ${styles.active}`} />
      ) : (
        <ChevronDown size={14} className={`${styles.sortIndicator} ${styles.active}`} />
      );
    }

    return <ChevronDown size={14} className={styles.sortIndicator} />;
  };

  // Loading state
  if (loading) {
    return (
      <div className={styles.tableContainer}>
        {searchable && (
          <div className={styles.tableControls}>
            <div className={styles.searchWrapper}>
              <Search size={16} className={styles.searchIcon} />
              <input
                type="text"
                className={styles.searchInput}
                placeholder="Search..."
                disabled
              />
            </div>
          </div>
        )}
        <div className={styles.tableLoading}>
          <Loader size={32} className={styles.spin} />
          <p>Loading data...</p>
        </div>
      </div>
    );
  }

  // Empty state
  if (processedData.length === 0 && !searchQuery) {
    return (
      <div className={styles.tableContainer}>
        {searchable && (
          <div className={styles.tableControls}>
            <div className={styles.searchWrapper}>
              <Search size={16} className={styles.searchIcon} />
              <input
                type="text"
                className={styles.searchInput}
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                placeholder="Search..."
              />
            </div>
          </div>
        )}
        <div className={styles.tableEmpty}>
          <p>{emptyMessage}</p>
        </div>
      </div>
    );
  }

  return (
    <div className={styles.tableContainer}>
      {/* Search Controls */}
      {searchable && (
        <div className={styles.tableControls}>
          <div className={styles.searchWrapper}>
            <input
              type="text"
              className={`input ${styles.searchInput}`}
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              placeholder="Filter routes by application, path, or target..."
              aria-label="Search table"
            />
          </div>
        </div>
      )}

      {/* No results after search */}
      {processedData.length === 0 && searchQuery && (
        <div className={styles.tableEmpty}>
          <p>No results found for &quot;{searchQuery}&quot;</p>
          <button
            className="btn btn-secondary btn-sm"
            onClick={() => setSearchQuery('')}
          >
            Clear search
          </button>
        </div>
      )}

      {/* Table */}
      {processedData.length > 0 && (
        <div className={styles.tableWrapper}>
          <table className={styles.table} role="table">
            <thead>
              <tr>
                {columns.map(column => (
                  <th
                    key={column.key}
                    onClick={() => handleSort(column.key)}
                    className={column.sortable !== false && sortable ? styles.sortable : ''}
                    role="columnheader"
                    aria-sort={
                      sortColumn === column.key
                        ? sortDirection === 'asc'
                          ? 'ascending'
                          : 'descending'
                        : 'none'
                    }
                  >
                    <div className={styles.thContent}>
                      <span>{column.label}</span>
                      {renderSortIndicator(column.key)}
                    </div>
                  </th>
                ))}
                {(onEdit || onDelete) && (
                  <th className={styles.actionsColumn}>ACTIONS</th>
                )}
              </tr>
            </thead>
            <tbody>
              {processedData.map((row, index) => (
                <tr key={row.id || index}>
                  {columns.map(column => (
                    <td key={column.key} data-label={column.label}>
                      {column.render
                        ? column.render(row)
                        : row[column.key]}
                    </td>
                  ))}
                  {(onEdit || onDelete) && (
                    <td data-label="Actions" className={styles.actionsCell}>
                      <div className={styles.actionButtons}>
                        {onEdit && (
                          <button
                            className="btn btn-sm btn-secondary"
                            onClick={() => onEdit(row)}
                            aria-label={`${editLabel} ${row.application || 'item'}`}
                          >
                            {editLabel}
                          </button>
                        )}
                        {onDelete && (
                          <button
                            className="btn btn-sm btn-error"
                            onClick={() => onDelete(row)}
                            aria-label={`Delete ${row.application || 'item'}`}
                          >
                            DELETE
                          </button>
                        )}
                      </div>
                    </td>
                  )}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

Table.propTypes = {
  columns: PropTypes.arrayOf(
    PropTypes.shape({
      key: PropTypes.string.isRequired,
      label: PropTypes.string.isRequired,
      sortable: PropTypes.bool,
      render: PropTypes.func
    })
  ).isRequired,
  data: PropTypes.arrayOf(PropTypes.object).isRequired,
  onEdit: PropTypes.func,
  onDelete: PropTypes.func,
  editLabel: PropTypes.string,
  searchable: PropTypes.bool,
  sortable: PropTypes.bool,
  loading: PropTypes.bool,
  emptyMessage: PropTypes.string,
  defaultSortColumn: PropTypes.string,
  defaultSortDirection: PropTypes.oneOf(['asc', 'desc'])
};
