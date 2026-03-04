# Terminal Dashboard Components

This directory contains React components for the Nekzus terminal-style dashboard UI.

## Directory Structure

```
components/
├── boxes/              # Container and layout components
│   ├── Box.jsx         # Primary WTFUtil-style container
│   ├── Card.jsx        # Generic card container
│   ├── ThreeColumnGrid.jsx  # 3-column responsive grid
│   ├── TwoColumnGrid.jsx    # 2-column responsive grid
│   └── index.js        # Barrel exports
│
├── navigation/         # Tab navigation components
│   ├── Tabs.jsx        # Tab navigation wrapper
│   ├── TabItem.jsx     # Individual tab button
│   ├── TabContent.jsx  # Tab panel content
│   ├── TabBadge.jsx    # Notification badge
│   └── index.js        # Barrel exports
│
└── README.md          # This file
```

---

## Box Components

### Box

Primary container with a header positioned at the top edge (WTFUtil style).

**Props:**
- `title` (string, required) - Header text
- `children` (node, required) - Content inside the box
- `className` (string, optional) - Additional CSS classes

**Usage:**
```jsx
import { Box } from './components/boxes';

<Box title="OVERVIEW">
  <div className="overview-list">
    <div className="overview-item">
      <span className="overview-label">ACTIVE ROUTES</span>
      <span className="overview-value">42</span>
    </div>
  </div>
</Box>
```

---

### Card

Generic bordered container without positioned header.

**Props:**
- `children` (node, required) - Content inside the card
- `className` (string, optional) - Additional CSS classes

**Usage:**
```jsx
import { Card } from './components/boxes';

<Card className="discovery-card">
  <div className="discovery-card-header">
    <h3>Grafana</h3>
  </div>
  <div className="discovery-card-body">
    <p>Discovered service on port 3000</p>
  </div>
</Card>
```

---

### ThreeColumnGrid

Responsive 3-column grid layout. Automatically stacks to 1 column on mobile.

**Props:**
- `children` (node, required) - Grid items
- `className` (string, optional) - Additional CSS classes

**Usage:**
```jsx
import { Box, ThreeColumnGrid } from './components/boxes';

<ThreeColumnGrid>
  <Box title="OVERVIEW">
    {/* Overview content */}
  </Box>
  <Box title="RECENT ACTIVITY">
    {/* Activity list */}
  </Box>
  <Box title="SYSTEM HEALTH">
    {/* Health metrics */}
  </Box>
</ThreeColumnGrid>
```

**Responsive Behavior:**
- Desktop (>1024px): 3 columns
- Mobile (≤1024px): 1 column (stacked)

---

### TwoColumnGrid

Responsive 2-column grid layout.

**Props:**
- `children` (node, required) - Grid items
- `className` (string, optional) - Additional CSS classes

**Usage:**
```jsx
import { Box, TwoColumnGrid } from './components/boxes';

<TwoColumnGrid>
  <Box title="REQUEST">
    <form>{/* Request form */}</form>
  </Box>
  <Box title="RESPONSE">
    <pre>{/* Response output */}</pre>
  </Box>
</TwoColumnGrid>
```

---

## Navigation Components

### Tabs

Tab navigation wrapper that manages active state.

**Props:**
- `tabs` (array, required) - Array of tab objects
  - `id` (string) - Unique tab identifier
  - `label` (string) - Tab label text
  - `badge` (number, optional) - Badge count
  - `badgeSeverity` (string, optional) - 'warning' | 'error' | 'info'
- `activeTab` (string, required) - Currently active tab ID
- `onChange` (function, required) - Callback when tab changes: `(tabId) => void`
- `ariaLabel` (string, optional) - ARIA label for tablist

**Usage:**
```jsx
import { useState } from 'react';
import { Tabs } from './components/navigation';

const MyComponent = () => {
  const [activeTab, setActiveTab] = useState('routes');

  const tabs = [
    { id: 'routes', label: 'ROUTES' },
    { id: 'discovery', label: 'DISCOVERY', badge: 7, badgeSeverity: 'warning' },
    { id: 'devices', label: 'DEVICES' },
    { id: 'settings', label: 'SETTINGS' },
  ];

  return (
    <Tabs
      tabs={tabs}
      activeTab={activeTab}
      onChange={setActiveTab}
      ariaLabel="Management console tabs"
    />
  );
};
```

---

### TabItem

Individual tab button (typically used internally by Tabs component).

**Props:**
- `id` (string, required) - Tab ID
- `label` (string, required) - Tab label text
- `badge` (number, optional) - Badge count
- `badgeSeverity` (string, optional) - 'warning' | 'error' | 'info'
- `active` (boolean, required) - Active state
- `onClick` (function, required) - Click handler

**Usage:**
```jsx
import { TabItem } from './components/navigation';

<TabItem
  id="discovery"
  label="DISCOVERY"
  badge={7}
  badgeSeverity="warning"
  active={activeTab === 'discovery'}
  onClick={(e) => handleTabClick(e, 'discovery')}
/>
```

---

### TabContent

Tab panel content wrapper.

**Props:**
- `id` (string, required) - Panel ID (matches tab's aria-controls)
- `active` (boolean, required) - Whether panel is visible
- `children` (node, required) - Panel content
- `className` (string, optional) - Additional CSS classes

**Usage:**
```jsx
import { TabContent } from './components/navigation';

<TabContent id="routes" active={activeTab === 'routes'}>
  <table className="table">
    {/* Table content */}
  </table>
</TabContent>

<TabContent id="discovery" active={activeTab === 'discovery'}>
  <div className="discovery-grid">
    {/* Discovery cards */}
  </div>
</TabContent>
```

---

### TabBadge

Notification badge for tabs (typically used internally by TabItem).

**Props:**
- `count` (number, required) - Badge count
- `severity` (string, optional) - 'warning' | 'error' | 'info'

**Usage:**
```jsx
import { TabBadge } from './components/navigation';

<TabBadge count={7} severity="warning" />
```

**Severity Colors:**
- `warning` - Amber/yellow
- `error` - Red
- `info` - Cyan (default)

---

## Complete Example

Here's a complete example combining all components:

```jsx
import { useState } from 'react';
import { Box, ThreeColumnGrid } from './components/boxes';
import { Tabs, TabContent } from './components/navigation';

const Dashboard = () => {
  const [activeTab, setActiveTab] = useState('overview');

  const tabs = [
    { id: 'overview', label: 'OVERVIEW' },
    { id: 'routes', label: 'ROUTES' },
    { id: 'discovery', label: 'DISCOVERY', badge: 7, badgeSeverity: 'warning' },
    { id: 'devices', label: 'DEVICES', badge: 128 },
    { id: 'settings', label: 'SETTINGS' },
  ];

  return (
    <div className="container">
      {/* Top Section - Three Column Grid */}
      <section className="component-section">
        <ThreeColumnGrid>
          <Box title="OVERVIEW">
            <div className="overview-list">
              <div className="overview-item">
                <span className="overview-label">ACTIVE ROUTES</span>
                <span className="overview-value">42</span>
              </div>
              <div className="overview-item">
                <span className="overview-label">PAIRED DEVICES</span>
                <span className="overview-value">128</span>
              </div>
            </div>
          </Box>

          <Box title="RECENT ACTIVITY">
            <div className="activity-list">
              <div className="activity-item">
                <span className="activity-text">New route registered</span>
                <span className="activity-time">2m ago</span>
              </div>
            </div>
          </Box>

          <Box title="SYSTEM HEALTH">
            <div className="health-list">
              <div className="health-item">
                <span className="health-label">AUTHENTICATION</span>
                <span className="badge badge-success badge-dot badge-filled">
                  ONLINE
                </span>
              </div>
            </div>
          </Box>
        </ThreeColumnGrid>
      </section>

      {/* Management Console - Tabbed Interface */}
      <section className="component-section">
        <Box title="MANAGEMENT CONSOLE">
          <Tabs
            tabs={tabs}
            activeTab={activeTab}
            onChange={setActiveTab}
            ariaLabel="Management console tabs"
          />

          <TabContent id="routes" active={activeTab === 'routes'}>
            <table className="table">
              {/* Routes table */}
            </table>
          </TabContent>

          <TabContent id="discovery" active={activeTab === 'discovery'}>
            <div className="discovery-grid">
              {/* Discovery cards */}
            </div>
          </TabContent>

          <TabContent id="devices" active={activeTab === 'devices'}>
            <div className="device-grid">
              {/* Device cards */}
            </div>
          </TabContent>

          <TabContent id="settings" active={activeTab === 'settings'}>
            <div className="settings-grid">
              {/* Settings forms */}
            </div>
          </TabContent>
        </Box>
      </section>
    </div>
  );
};

export default Dashboard;
```

---

## Accessibility Features

All components include ARIA attributes for screen readers:

- **Tabs**: `role="tablist"`, `role="tab"`, `aria-selected`, `aria-controls`
- **TabContent**: `role="tabpanel"`, `aria-labelledby`
- **TabBadge**: `aria-hidden="true"` (decorative)
- **TabItem**: Dynamic `aria-label` with badge context

---

## CSS Classes Reference

### Box Component Classes
- `.box` - Main container
- `.box-header` - Header text (positioned at top edge)
- `.box-content` - Content area

### Grid Classes
- `.three-column` - 3-column grid layout
- `.two-column` - 2-column grid layout

### Tab Classes
- `.tabs` - Tab navigation container
- `.tab-item` - Individual tab button
- `.tab-item.active` - Active tab state
- `.tab-label` - Tab label text
- `.tab-badge` - Notification badge
- `.tab-content` - Tab panel container
- `.tab-content.active` - Visible tab panel

### Badge Severity Attributes
- `[data-severity="warning"]` - Amber badge
- `[data-severity="error"]` - Red badge
- `[data-severity="info"]` - Cyan badge

---

## Theme Support

All components work with the 9 terminal themes:
- Classic Green (default)
- Amber
- Cyan
- Gruvbox
- Gruvbox Light
- Tokyo Night
- Tokyo Night Storm
- Catppuccin Mocha
- Pipboy (with CRT effects)

Components automatically inherit theme colors through CSS custom properties defined in `styles.css`.

---

## Responsive Design

- **ThreeColumnGrid**: 3 columns → 1 column at ≤1024px
- **TwoColumnGrid**: 2 columns → 1 column at ≤768px
- **Tabs**: Horizontal scroll on mobile if tabs overflow
- **Box**: Full width on mobile with adjusted padding

---

## Next Steps

For additional components, see:
- `./forms/` - Form input components (coming soon)
- `./modals/` - Modal dialog components (coming soon)
- `./alerts/` - Alert and notification components (coming soon)
- `./badges/` - Badge and status components (coming soon)

---

## Notes

- All components use PropTypes for type checking
- Components follow React 19 best practices
- CSS classes match the exact structure from `dashboard.html`
- JSDoc comments provide inline documentation
- Barrel exports (`index.js`) allow clean imports
