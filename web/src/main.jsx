import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'

// CSS imports in correct order
import './styles/tokens.css'
import './styles/base.css'
import './styles/themes.css'
import './styles/utilities.css'
import './styles/animations.css'
import './styles/accessibility.css'
import './styles/responsive.css'

// Layout Component Styles
import './styles/components/layout/container.css'
import './styles/components/layout/header.css'
import './styles/components/layout/content.css'
import './styles/components/layout/footer.css'
import './styles/components/layout/alerts.css'
import './styles/components/notifications.css'

// Box/Card Component Styles
import './styles/components/boxes/box.css'
import './styles/components/boxes/card.css'
import './styles/components/boxes/grids.css'
import './styles/components/boxes/discovery-card.css'
import './styles/components/boxes/device-card.css'
import './styles/components/boxes/container-card.css'
import './styles/components/boxes/confirmation-card.css'
import './styles/components/boxes/pairing-modal.css'
import './styles/components/boxes/service-card.css'
import './styles/components/boxes/deployment-card.css'

// Navigation Component Styles
import './styles/components/navigation/tabs.css'
import './styles/components/navigation/tab-content.css'
import './styles/components/navigation/nav.css'

// Tab Pages Styles
import './styles/components/tabs/routes-tab.css'
import './styles/components/tabs/discovery-tab.css'
import './styles/components/tabs/devices-tab.css'
import './styles/components/tabs/containers-tab.css'
import './styles/components/tabs/toolbox-tab.css'
import './styles/components/tabs/settings-tab.css'
import './styles/components/tabs/metrics-tab.css'
import './styles/components/tabs/scripts-tab.css'
import './styles/components/tabs/notifications-tab.css'
import './styles/components/tabs/federation-tab.css'

// Form Component Styles
import './styles/components/forms/form.css'
import './styles/components/forms/input.css'
import './styles/components/forms/dropdown.css'
import './styles/components/forms/checkbox.css'
import './styles/components/forms/radio.css'
import './styles/components/forms/toggle.css'
import './styles/components/forms/textarea.css'

// Button Component Styles
import './styles/components/buttons/button.css'
import './styles/components/buttons/button-group.css'
import './styles/components/buttons/bulk-actions.css'

// Data Display Component Styles
import './styles/components/data-display/table.css'
import './styles/components/data-display/overview.css'
import './styles/components/data-display/activity.css'
import './styles/components/data-display/health.css'
import './styles/components/data-display/progress.css'
import './styles/components/data-display/badge.css'
import './styles/components/data-display/tag.css'
import './styles/components/data-display/empty-state.css'

// Chart Component Styles
import './styles/components/charts/charts.css'

// Modal Component Styles
import './styles/components/modals/container-modals.css'
import './styles/components/modals/container-logs-modal.css'

// Testing Component Styles
import './styles/components/testing/tester.css'
import './styles/components/testing/response-display.css'

// Settings Component Styles
import './styles/components/settings/settings.css'
import './styles/components/settings/data-management.css'

// Typography Component Styles
import './styles/components/typography/typography.css'
import './styles/components/typography/code.css'

import App from './App.jsx'

createRoot(document.getElementById('root')).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
