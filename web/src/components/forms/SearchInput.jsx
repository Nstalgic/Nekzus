import React from 'react';
import PropTypes from 'prop-types';
import { Search } from 'lucide-react';

/**
 * SearchInput Component
 *
 * A search/filter input field with search icon.
 *
 * @component
 * @example
 * ```jsx
 * <SearchInput
 *   value={searchTerm}
 *   onChange={(e) => setSearchTerm(e.target.value)}
 *   placeholder="Search routes..."
 * />
 * ```
 */
const SearchInput = ({
  value,
  onChange,
  placeholder = 'Search...',
}) => {
  return (
    <div style={{ position: 'relative', width: '100%' }}>
      <input
        type="text"
        className="input search-input"
        value={value}
        onChange={onChange}
        placeholder={placeholder}
        aria-label={placeholder}
        style={{ paddingLeft: '32px' }}
      />
      <Search
        size={14}
        style={{
          position: 'absolute',
          left: '10px',
          top: '50%',
          transform: 'translateY(-50%)',
          color: 'var(--text-secondary)',
          pointerEvents: 'none',
        }}
      />
    </div>
  );
};

SearchInput.propTypes = {
  /** Current search value */
  value: PropTypes.string.isRequired,
  /** Change handler function */
  onChange: PropTypes.func.isRequired,
  /** Placeholder text */
  placeholder: PropTypes.string,
};

export default SearchInput;
