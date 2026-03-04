# Button Components

Terminal-style button components with WTFUtil aesthetic.

## Components

### Button

Main button component with multiple variants, sizes, and states.

**Props:**
- `variant` - Button style: `'primary'|'secondary'|'success'|'error'` (default: `'primary'`)
- `size` - Button size: `'sm'|'default'` (default: `'default'`)
- `loading` - Loading state with spinner (default: `false`)
- `disabled` - Disabled state (default: `false`)
- `onClick` - Click handler function
- `children` - Button content (required)
- `type` - HTML button type: `'button'|'submit'|'reset'` (default: `'button'`)

**Examples:**

```jsx
import { Button } from '../components/buttons';

// Primary button
<Button variant="primary" onClick={handleExecute}>
  Execute
</Button>

// Success button
<Button variant="success" onClick={handleStart}>
  Start
</Button>

// Error/Danger button
<Button variant="error" onClick={handleStop}>
  Stop
</Button>

// Secondary button
<Button variant="secondary" onClick={handleCancel}>
  Cancel
</Button>

// Small button
<Button variant="primary" size="sm" onClick={handleEdit}>
  Edit
</Button>

// Loading state
<Button variant="primary" loading>
  Processing
</Button>

// Disabled button
<Button variant="primary" disabled>
  Unavailable
</Button>

// Submit button for forms
<Button variant="success" type="submit">
  Submit
</Button>
```

**CSS Classes:**
- `.btn` - Base button styles
- `.btn-primary` - Primary variant
- `.btn-secondary` - Secondary variant
- `.btn-success` - Success variant
- `.btn-error` - Error/danger variant
- `.btn-sm` - Small size
- `.loading` - Loading state

**States:**
- **Default**: Border with transparent background
- **Hover**: Filled background with inverted colors
- **Active**: Slight transform on click
- **Focus**: Outline for keyboard navigation
- **Loading**: Shows `[PROCESSING...]` text
- **Disabled**: Reduced opacity, no interactions

---

### ButtonGroup

Groups multiple buttons together with proper spacing.

**Props:**
- `children` - Button elements (required)

**Examples:**

```jsx
import { Button, ButtonGroup } from '../components/buttons';

// Action buttons
<ButtonGroup>
  <Button variant="primary">Execute</Button>
  <Button variant="secondary">Cancel</Button>
</ButtonGroup>

// Approval buttons
<ButtonGroup>
  <Button variant="secondary">Reject</Button>
  <Button variant="success">Approve</Button>
</ButtonGroup>

// Card actions
<ButtonGroup>
  <Button variant="secondary" size="sm">View</Button>
  <Button variant="error" size="sm">Revoke</Button>
</ButtonGroup>
```

**CSS Classes:**
- `.button-group` - Flex container with gap

---

## Usage Examples

### Card Actions

```jsx
import { Button, ButtonGroup } from '../components/buttons';

function DeviceCard({ device, onView, onRevoke }) {
  return (
    <div className="device-card">
      <div className="device-card-header">
        <span>{device.name}</span>
      </div>
      <div className="device-card-body">
        <p>{device.platform} • Last seen: {device.lastSeen}</p>
      </div>
      <div className="device-card-actions">
        <ButtonGroup>
          <Button variant="secondary" size="sm" onClick={() => onView(device)}>
            View Details
          </Button>
          <Button variant="error" size="sm" onClick={() => onRevoke(device)}>
            Revoke
          </Button>
        </ButtonGroup>
      </div>
    </div>
  );
}
```

### Discovery Approval

```jsx
import { Button, ButtonGroup } from '../components/buttons';

function DiscoveryCard({ discovery, onApprove, onReject }) {
  return (
    <div className="discovery-card">
      <div className="discovery-card-header">
        <span className="discovery-card-title">{discovery.name}</span>
      </div>
      <div className="discovery-card-body">
        <p>{discovery.url}</p>
      </div>
      <div className="discovery-card-actions">
        <ButtonGroup>
          <Button variant="secondary" onClick={() => onReject(discovery)}>
            Reject
          </Button>
          <Button variant="success" onClick={() => onApprove(discovery)}>
            ✓ Approve
          </Button>
        </ButtonGroup>
      </div>
    </div>
  );
}
```

### Form Submission

```jsx
import { Button, ButtonGroup } from '../components/buttons';

function RouteForm({ onSubmit, onCancel, isSubmitting }) {
  return (
    <form onSubmit={onSubmit}>
      {/* Form fields */}

      <ButtonGroup>
        <Button
          variant="secondary"
          onClick={onCancel}
          disabled={isSubmitting}
        >
          Cancel
        </Button>
        <Button
          variant="success"
          type="submit"
          loading={isSubmitting}
        >
          Save Route
        </Button>
      </ButtonGroup>
    </form>
  );
}
```

### Confirmation Dialog

```jsx
import { Button, ButtonGroup } from '../components/buttons';

function ConfirmationCard({ message, onConfirm, onCancel, isProcessing }) {
  return (
    <div className="confirmation-card">
      <div className="confirmation-card-body">
        <p>{message}</p>
      </div>
      <div className="confirmation-card-actions">
        <ButtonGroup>
          <Button
            variant="secondary"
            onClick={onCancel}
            disabled={isProcessing}
          >
            Cancel
          </Button>
          <Button
            variant="success"
            onClick={onConfirm}
            loading={isProcessing}
          >
            Confirm
          </Button>
        </ButtonGroup>
      </div>
    </div>
  );
}
```

### Toolbar Actions

```jsx
import { Button, ButtonGroup } from '../components/buttons';

function DiscoveryToolbar({ selectedCount, onApprove, onReject }) {
  return (
    <div className="discovery-toolbar">
      <span>
        {selectedCount} selected
      </span>
      <ButtonGroup>
        <Button
          variant="secondary"
          size="sm"
          disabled={selectedCount === 0}
          onClick={onReject}
        >
          Reject Selected
        </Button>
        <Button
          variant="success"
          size="sm"
          disabled={selectedCount === 0}
          onClick={onApprove}
        >
          ✓ Approve Selected
        </Button>
      </ButtonGroup>
    </div>
  );
}
```

### Settings Actions

```jsx
import { Button } from '../components/buttons';

function SettingsPanel() {
  const [exporting, setExporting] = useState(false);

  const handleExport = async () => {
    setExporting(true);
    try {
      await exportSettings();
    } finally {
      setExporting(false);
    }
  };

  return (
    <div className="settings-item">
      <div>
        <h3>Export Settings</h3>
        <p className="text-secondary">Download your settings as a JSON file</p>
      </div>
      <Button
        variant="primary"
        onClick={handleExport}
        loading={exporting}
      >
        Export
      </Button>
    </div>
  );
}
```

---

## Accessibility

All button components include proper ARIA attributes:

- **Button**:
  - Native `<button>` element for keyboard/screen reader support
  - `aria-busy` when loading
  - `aria-disabled` when disabled or loading
  - Proper focus indicators (`:focus-visible`)
  - Keyboard support (Enter/Space)

- **ButtonGroup**:
  - Semantic grouping with proper spacing
  - Maintains button tab order

---

## Styling

Buttons use the terminal CSS framework classes from `styles.css`:

**Color Variants:**
- Primary: `--text-primary`
- Secondary: `--text-secondary`
- Success: `--color-success`
- Error: `--color-error`

**Hover Effect:**
All buttons invert on hover (border color becomes background)

**Loading State:**
Shows `[PROCESSING...]` text with the button's color variant

**Focus State:**
2px white outline with offset for keyboard navigation

**Required CSS Variables:**
- `--font-mono`
- `--spacing-xs`, `--spacing-md`
- `--border-width`, `--border-color`
- `--bg-primary`
- `--text-primary`, `--text-secondary`, `--text-tertiary`, `--text-white`
- `--color-success`, `--color-error`
- `--transition-fast`

---

## Button Variants Visual Guide

```
┌─────────────────────────────────────┐
│ [  PRIMARY  ]  Border: --text-primary  │
│ [SECONDARY ]  Border: --text-secondary │
│ [ SUCCESS  ]  Border: --color-success  │
│ [  ERROR   ]  Border: --color-error    │
│ [ DISABLED ]  Opacity: 0.5, No hover   │
└─────────────────────────────────────┘

Hover State:
┌─────────────────────────────────────┐
│ ▓▓ PRIMARY ▓▓  Filled background     │
└─────────────────────────────────────┘

Loading State:
┌─────────────────────────────────────┐
│ [PROCESSING...]                      │
└─────────────────────────────────────┘
```

---

## Best Practices

1. **Use semantic variants:**
   - `primary` for main actions
   - `secondary` for cancel/back actions
   - `success` for confirmations/approvals
   - `error` for destructive actions

2. **Always provide loading state for async actions:**
   ```jsx
   <Button variant="success" loading={isSubmitting}>
     Save
   </Button>
   ```

3. **Disable buttons when action is unavailable:**
   ```jsx
   <Button variant="primary" disabled={!isValid}>
     Submit
   </Button>
   ```

4. **Group related actions:**
   ```jsx
   <ButtonGroup>
     <Button variant="secondary">Cancel</Button>
     <Button variant="success">Confirm</Button>
   </ButtonGroup>
   ```

5. **Use small size for compact spaces:**
   ```jsx
   <Button variant="secondary" size="sm">Edit</Button>
   ```
