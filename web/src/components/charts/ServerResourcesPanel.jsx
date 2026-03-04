import Box from '../boxes/Box';
import ResourceLineChart from './ResourceLineChart';
import { useData } from '../../contexts/DataContext';

/**
 * ServerResourcesPanel - Container for server resource monitoring charts
 *
 * Displays CPU, RAM, and Disk usage in a three-column grid layout.
 * Consumes data from DataContext for real-time updates.
 *
 * @example
 * ```jsx
 * <ServerResourcesPanel />
 * ```
 */
function ServerResourcesPanel() {
  const { resourceHistory, systemResources } = useData();

  return (
    <section
      className="component-section server-resources-section"
      data-testid="server-resources-panel"
    >
      <Box title="SERVER RESOURCES">
        <div
          className="three-column resource-charts-grid"
          data-testid="resource-charts-grid"
        >
          <ResourceLineChart
            label="CPU"
            data={resourceHistory.cpu}
            timestamps={resourceHistory.timestamps}
            currentValue={systemResources.cpu}
            height={100}
          />
          <ResourceLineChart
            label="RAM"
            data={resourceHistory.ram}
            timestamps={resourceHistory.timestamps}
            currentValue={systemResources.ram}
            height={100}
          />
          <ResourceLineChart
            label="DISK"
            data={resourceHistory.disk}
            timestamps={resourceHistory.timestamps}
            currentValue={systemResources.disk}
            height={100}
          />
        </div>
      </Box>
    </section>
  );
}

export default ServerResourcesPanel;
