# Data Display Components

Terminal-style components for displaying metrics, activities, and progress indicators with WTFUtil aesthetic.

## Components

### Badge

Status/tag badge component with multiple variants and styles.

**Props:**
- `variant` - Color variant: `'primary'|'secondary'|'success'|'error'|'warning'|'info'` (default: `'primary'`)
- `dot` - Show dot indicator before text (default: `false`)
- `filled` - Filled background style (default: `false`)
- `size` - Badge size: `'sm'|'default'` (default: `'default'`)
- `children` - Badge content (required)

**Examples:**

```jsx
import { Badge } from '../components/data-display';

// Basic badge
<Badge variant="success">ONLINE</Badge>

// Filled badge with dot
<Badge variant="success" dot filled>ACTIVE</Badge>

// Small badge
<Badge variant="primary" size="sm">READ</Badge>

// Warning badge
<Badge variant="warning" filled>HIGH</Badge>
```

**CSS Classes:**
- `.badge` - Base badge styles
- `.badge-{variant}` - Variant colors
- `.badge-dot` - Adds dot indicator
- `.badge-filled` - Filled background
- `.badge-sm` - Small size

---

### OverviewItem

Single metric display with optional clickable value.

**Props:**
- `label` - Metric label (required)
- `value` - Metric value (required)
- `link` - Make value clickable (default: `false`)
- `urgent` - Urgent state with pulse animation (default: `false`)
- `onClick` - Click handler for link

**Examples:**

```jsx
import { OverviewItem } from '../components/data-display';

// Basic metric
<OverviewItem label="Active Routes" value="23" />

// Clickable metric
<OverviewItem
  label="Pending"
  value="5"
  link
  onClick={() => navigate('/pending')}
/>

// Urgent metric
<OverviewItem
  label="Alerts"
  value="3"
  link
  urgent
  onClick={handleAlertsClick}
/>
```

**CSS Classes:**
- `.overview-item` - Container
- `.overview-label` - Label text
- `.overview-value` - Value text
- `.overview-link` - Clickable value
- `.urgent` - Urgent state with pulse

---

### OverviewList

Container for multiple overview metrics.

**Props:**
- `metrics` - Array of metric objects (required)
  - Each metric: `{ id?, label, value, link?, urgent?, onClick? }`

**Examples:**

```jsx
import { OverviewList } from '../components/data-display';

const metrics = [
  { id: '1', label: 'Active Routes', value: '23' },
  { id: '2', label: 'Paired Devices', value: '12' },
  {
    id: '3',
    label: 'Pending Discoveries',
    value: '5',
    link: true,
    urgent: true,
    onClick: () => navigate('/discoveries')
  },
  { id: '4', label: 'Total Requests', value: '1.2K' }
];

<OverviewList metrics={metrics} />
```

**CSS Classes:**
- `.overview-list` - Container with flex layout

---

### ActivityItem

Single activity entry with text and timestamp.

**Props:**
- `text` - Activity description (required)
- `time` - Timestamp or relative time (required)

**Examples:**

```jsx
import { ActivityItem } from '../components/data-display';

<ActivityItem text="New route registered" time="2m ago" />
<ActivityItem text="Device paired: iPhone 13" time="5m ago" />
```

**CSS Classes:**
- `.activity-item` - Container with flex layout
- `.activity-text` - Activity description
- `.activity-time` - Timestamp

---

### ActivityList

Scrollable activity feed container.

**Props:**
- `activities` - Array of activity objects (required)
  - Each activity: `{ id, text, time }`
- `maxHeight` - Maximum height before scrolling (default: `'300px'`)

**Examples:**

```jsx
import { ActivityList } from '../components/data-display';

const activities = [
  { id: '1', text: 'New route registered', time: '2m ago' },
  { id: '2', text: 'Device paired: iPhone 13', time: '5m ago' },
  { id: '3', text: 'Discovery completed', time: '10m ago' },
  { id: '4', text: 'Certificate renewed', time: '1h ago' }
];

<ActivityList activities={activities} />

// Custom max height
<ActivityList activities={activities} maxHeight="400px" />
```

**CSS Classes:**
- `.activity-list` - Scrollable container

---

### ProgressBar

ASCII-style progress indicator using block characters.

**Props:**
- `current` - Current progress value (required)
- `max` - Maximum value (required)
- `label` - Progress label
- `blocks` - Number of block characters (default: `20`)
- `showPercentage` - Show percentage text (default: `true`)

**Examples:**

```jsx
import { ProgressBar } from '../components/data-display';

// Basic progress bar
<ProgressBar current={38} max={100} label="Sync Progress" />

// Without percentage
<ProgressBar current={15} max={50} label="Upload" showPercentage={false} />

// Custom block count
<ProgressBar current={750} max={1000} label="Storage" blocks={30} />
```

**Visualization:**
```
Sync Progress
████████░░░░░░░░░░░░
38 / 100 (38.0%)
```

**CSS Classes:**
- `.ascii-progress-bar` - Container
- `.ascii-progress-fill` - Fill container
- `.ascii-progress-blocks` - Block characters
- `.ascii-progress-text` - Label and percentage text

**Block Characters:**
- `█` - Filled block
- `░` - Empty block

---

## Usage Examples

### Dashboard Overview Section

```jsx
import { OverviewList, ActivityList } from '../components/data-display';

function DashboardOverview() {
  const metrics = [
    { id: '1', label: 'Active Routes', value: '23' },
    { id: '2', label: 'Paired Devices', value: '12' },
    {
      id: '3',
      label: 'Pending Discoveries',
      value: '5',
      link: true,
      urgent: true,
      onClick: () => navigate('/discoveries')
    }
  ];

  const activities = [
    { id: '1', text: 'New route registered', time: '2m ago' },
    { id: '2', text: 'Device paired: iPhone 13', time: '5m ago' }
  ];

  return (
    <div className="dashboard-grid">
      <div className="box">
        <div className="box-header">OVERVIEW</div>
        <div className="box-content">
          <OverviewList metrics={metrics} />
        </div>
      </div>

      <div className="box">
        <div className="box-header">RECENT ACTIVITY</div>
        <div className="box-content">
          <ActivityList activities={activities} />
        </div>
      </div>
    </div>
  );
}
```

### Status Cards with Badges

```jsx
import { Badge } from '../components/data-display';

function ServiceCard({ name, status, version }) {
  return (
    <div className="device-card">
      <div className="device-card-header">
        <span>{name}</span>
        <Badge variant={status === 'online' ? 'success' : 'error'} dot filled>
          {status.toUpperCase()}
        </Badge>
      </div>
      <div className="device-card-body">
        Version: <Badge variant="info" filled size="sm">{version}</Badge>
      </div>
    </div>
  );
}
```

### Progress Tracking

```jsx
import { ProgressBar } from '../components/data-display';

function SyncStatus({ syncData }) {
  return (
    <div className="box">
      <div className="box-header">SYNC STATUS</div>
      <div className="box-content">
        <ProgressBar
          current={syncData.synced}
          max={syncData.total}
          label="Services Synced"
        />
      </div>
    </div>
  );
}
```

---

## Accessibility

All components include proper ARIA attributes:

- **Badge**: Semantic markup, screen reader friendly
- **OverviewItem**: Proper link semantics, keyboard navigation
- **ActivityItem**: Clear text structure
- **ProgressBar**: `aria-hidden` on visual blocks, separate screen reader text

## Styling

All components use the terminal CSS framework classes from `styles.css`. They are unstyled components that rely on global styles for the WTFUtil aesthetic.

**Required CSS Variables:**
- `--text-primary`, `--text-secondary`, `--text-tertiary`
- `--color-success`, `--color-error`, `--color-warning`, `--color-info`
- `--accent-cyan`
- `--border-dim`
- `--spacing-xs`, `--spacing-sm`, `--spacing-md`
- `--transition-fast`
