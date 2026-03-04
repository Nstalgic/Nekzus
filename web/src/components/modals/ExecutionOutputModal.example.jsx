/**
 * ExecutionOutputModal - Usage Examples
 *
 * This file demonstrates how to use the ExecutionOutputModal component
 * in different scenarios.
 */

import { useState } from 'react';
import { ExecutionOutputModal } from './ExecutionOutputModal';

/**
 * Example 1: Basic Usage with Completed Execution
 */
export function BasicExample() {
  const [isOpen, setIsOpen] = useState(false);

  const completedExecution = {
    id: 'exec-123',
    scriptName: 'backup-database.sh',
    status: 'completed',
    output: 'Starting database backup...\nConnecting to database...\nBackup completed successfully!\nFiles saved to /backups/db-2024-12-24.sql',
    exitCode: 0,
    triggeredBy: 'admin@example.com',
    startedAt: '2024-12-24T10:30:00Z',
    completedAt: '2024-12-24T10:30:45Z',
  };

  return (
    <>
      <button onClick={() => setIsOpen(true)}>View Execution Output</button>
      <ExecutionOutputModal
        isOpen={isOpen}
        onClose={() => setIsOpen(false)}
        execution={completedExecution}
      />
    </>
  );
}

/**
 * Example 2: Failed Execution with Error
 */
export function FailedExecutionExample() {
  const [isOpen, setIsOpen] = useState(false);

  const failedExecution = {
    id: 'exec-456',
    scriptName: 'deploy-app.sh',
    status: 'failed',
    output: 'Starting deployment...\nPulling latest code...\nError: Permission denied when accessing /var/www/app',
    error: 'Script exited with non-zero status code. Deployment failed due to insufficient permissions.',
    exitCode: 1,
    triggeredBy: 'scheduler',
    startedAt: '2024-12-24T11:00:00Z',
    completedAt: '2024-12-24T11:00:15Z',
  };

  return (
    <>
      <button onClick={() => setIsOpen(true)}>View Failed Execution</button>
      <ExecutionOutputModal
        isOpen={isOpen}
        onClose={() => setIsOpen(false)}
        execution={failedExecution}
      />
    </>
  );
}

/**
 * Example 3: Running Execution with Auto-Refresh
 */
export function RunningExecutionExample() {
  const [isOpen, setIsOpen] = useState(false);
  const [execution, setExecution] = useState({
    id: 'exec-789',
    scriptName: 'long-running-task.sh',
    status: 'running',
    output: 'Task started...\nProcessing item 1/100...\nProcessing item 2/100...',
    triggeredBy: 'user@example.com',
    startedAt: Date.now() / 1000 - 30, // Started 30 seconds ago
  });

  // Simulate fetching updated execution data
  const handleRefresh = async () => {
    console.log('Refreshing execution data...');
    // In real app, this would fetch from API
    await new Promise(resolve => setTimeout(resolve, 500));

    setExecution(prev => ({
      ...prev,
      output: prev.output + '\nProcessing item 3/100...',
    }));
  };

  return (
    <>
      <button onClick={() => setIsOpen(true)}>View Running Execution</button>
      <ExecutionOutputModal
        isOpen={isOpen}
        onClose={() => setIsOpen(false)}
        execution={execution}
        onRefresh={handleRefresh}
      />
    </>
  );
}

/**
 * Example 4: Execution with No Output
 */
export function NoOutputExample() {
  const [isOpen, setIsOpen] = useState(false);

  const noOutputExecution = {
    id: 'exec-999',
    scriptName: 'silent-script.sh',
    status: 'completed',
    output: '',
    exitCode: 0,
    triggeredBy: 'automation',
    startedAt: '2024-12-24T12:00:00Z',
    completedAt: '2024-12-24T12:00:01Z',
  };

  return (
    <>
      <button onClick={() => setIsOpen(true)}>View Silent Execution</button>
      <ExecutionOutputModal
        isOpen={isOpen}
        onClose={() => setIsOpen(false)}
        execution={noOutputExecution}
      />
    </>
  );
}

/**
 * Example 5: Integration with Scripts Tab
 *
 * This shows how to integrate the modal in a real scripts management page
 */
export function ScriptsIntegrationExample() {
  const [selectedExecution, setSelectedExecution] = useState(null);
  const [executions] = useState([
    {
      id: 'exec-1',
      scriptName: 'backup.sh',
      status: 'completed',
      output: 'Backup completed successfully',
      exitCode: 0,
      triggeredBy: 'admin',
      startedAt: '2024-12-24T09:00:00Z',
      completedAt: '2024-12-24T09:00:30Z',
    },
    {
      id: 'exec-2',
      scriptName: 'cleanup.sh',
      status: 'failed',
      output: 'Error during cleanup',
      error: 'Disk full',
      exitCode: 1,
      triggeredBy: 'scheduler',
      startedAt: '2024-12-24T10:00:00Z',
      completedAt: '2024-12-24T10:00:05Z',
    },
  ]);

  const handleRefreshExecution = async () => {
    // Fetch updated execution from API
    console.log('Refreshing execution:', selectedExecution.id);
  };

  return (
    <div>
      <h2>Recent Executions</h2>
      <div className="executions-list">
        {executions.map(exec => (
          <div key={exec.id} className="execution-card">
            <h3>{exec.scriptName}</h3>
            <span className={`badge badge-${exec.status}`}>{exec.status}</span>
            <button onClick={() => setSelectedExecution(exec)}>
              View Output
            </button>
          </div>
        ))}
      </div>

      <ExecutionOutputModal
        isOpen={!!selectedExecution}
        onClose={() => setSelectedExecution(null)}
        execution={selectedExecution}
        onRefresh={
          selectedExecution?.status === 'running'
            ? handleRefreshExecution
            : undefined
        }
      />
    </div>
  );
}

/**
 * Example 6: Timeout Execution
 */
export function TimeoutExecutionExample() {
  const [isOpen, setIsOpen] = useState(false);

  const timeoutExecution = {
    id: 'exec-timeout',
    scriptName: 'slow-process.sh',
    status: 'timeout',
    output: 'Starting process...\nProcessing data...\n[Process terminated due to timeout]',
    exitCode: 124,
    triggeredBy: 'scheduler',
    startedAt: '2024-12-24T14:00:00Z',
    completedAt: '2024-12-24T14:15:00Z',
  };

  return (
    <>
      <button onClick={() => setIsOpen(true)}>View Timeout Execution</button>
      <ExecutionOutputModal
        isOpen={isOpen}
        onClose={() => setIsOpen(false)}
        execution={timeoutExecution}
      />
    </>
  );
}
