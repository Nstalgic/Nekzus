/**
 * ScriptsTab Component
 *
 * Script management interface for automated tasks and workflows
 *
 * Features:
 * - Browse registered scripts
 * - Execute scripts with parameters
 * - View execution history
 * - Manage workflows
 * - Schedule jobs
 * - Monitor script status
 */

import { useState, useEffect, useMemo, useCallback } from 'react';
import PropTypes from 'prop-types';
import { RefreshCw, Plus, Play, FileText, GitBranch, Clock } from 'lucide-react';
import Badge from '../../components/data-display/Badge';
import { Table } from '../../components/data-display/Table';
import { useNotification } from '../../contexts/NotificationContext';
import { RegisterScriptModal } from '../../components/modals/RegisterScriptModal';
import { EditScriptModal } from '../../components/modals/EditScriptModal';
import { ExecutionOutputModal } from '../../components/modals/ExecutionOutputModal';
import { CreateWorkflowModal } from '../../components/modals/CreateWorkflowModal';
import { CreateScheduleModal } from '../../components/modals/CreateScheduleModal';
import { scriptsAPI } from '../../services/api';
import { websocketService, WS_MSG_TYPES } from '../../services/websocket';

/**
 * Format script type for display
 */
const formatScriptType = (type) => {
  const labels = {
    shell: 'Shell',
    python: 'Python',
    go_binary: 'Go'
  };
  return labels[type] || type;
};

/**
 * ScriptsTab Component
 */
export function ScriptsTab() {
  const { addNotification } = useNotification();

  // State for feature enabled
  const [featureEnabled, setFeatureEnabled] = useState(true);

  // State for scripts
  const [scripts, setScripts] = useState([]);
  const [scriptsLoading, setScriptsLoading] = useState(true);
  const [scriptsError, setScriptsError] = useState(null);

  // State for executions
  const [executions, setExecutions] = useState([]);
  const [executionsLoading, setExecutionsLoading] = useState(true);
  const [executionsError, setExecutionsError] = useState(null);

  // State for workflows
  const [workflows, setWorkflows] = useState([]);
  const [workflowsLoading, setWorkflowsLoading] = useState(true);
  const [workflowsError, setWorkflowsError] = useState(null);

  // State for schedules
  const [schedules, setSchedules] = useState([]);
  const [schedulesLoading, setSchedulesLoading] = useState(true);
  const [schedulesError, setSchedulesError] = useState(null);

  // State for UI
  const [selectedScript, setSelectedScript] = useState(null);
  const [selectedExecution, setSelectedExecution] = useState(null);
  const [executeModalOpen, setExecuteModalOpen] = useState(false);
  const [detailsModalOpen, setDetailsModalOpen] = useState(false);
  const [outputModalOpen, setOutputModalOpen] = useState(false);
  const [registerModalOpen, setRegisterModalOpen] = useState(false);
  const [editModalOpen, setEditModalOpen] = useState(false);
  const [workflowModalOpen, setWorkflowModalOpen] = useState(false);
  const [scheduleModalOpen, setScheduleModalOpen] = useState(false);
  const [filterCategory, setFilterCategory] = useState('all');
  const [filterStatus, setFilterStatus] = useState('all');
  const [viewMode, setViewMode] = useState('scripts'); // scripts | executions | workflows | schedules
  const [isRefreshing, setIsRefreshing] = useState(false);

  // Fetch scripts on mount
  useEffect(() => {
    fetchScripts();
  }, []);

  // Fetch executions on mount
  useEffect(() => {
    fetchExecutions();
  }, []);

  // Fetch workflows on mount
  useEffect(() => {
    fetchWorkflows();
  }, []);

  // Fetch schedules on mount
  useEffect(() => {
    fetchSchedules();
  }, []);

  // Listen for WebSocket execution events
  useEffect(() => {
    const handleExecutionStarted = (data) => {
      addNotification({
        severity: 'info',
        message: `Script "${data.scriptName || data.scriptId}" started executing`,
        strongText: 'STARTED:'
      });
      fetchExecutions();
    };

    const handleExecutionCompleted = (data) => {
      const exitCode = data.exitCode !== undefined ? ` (exit code: ${data.exitCode})` : '';
      addNotification({
        severity: 'success',
        message: `Script execution completed${exitCode}`,
        strongText: 'COMPLETED:'
      });
      fetchExecutions();
    };

    const handleExecutionFailed = (data) => {
      addNotification({
        severity: 'error',
        message: data.errorMessage || 'Script execution failed',
        strongText: 'FAILED:'
      });
      fetchExecutions();
    };

    websocketService.on(WS_MSG_TYPES.EXECUTION_STARTED, handleExecutionStarted);
    websocketService.on(WS_MSG_TYPES.EXECUTION_COMPLETED, handleExecutionCompleted);
    websocketService.on(WS_MSG_TYPES.EXECUTION_FAILED, handleExecutionFailed);

    return () => {
      websocketService.off(WS_MSG_TYPES.EXECUTION_STARTED, handleExecutionStarted);
      websocketService.off(WS_MSG_TYPES.EXECUTION_COMPLETED, handleExecutionCompleted);
      websocketService.off(WS_MSG_TYPES.EXECUTION_FAILED, handleExecutionFailed);
    };
  }, [addNotification]);

  // Fetch scripts from API
  const fetchScripts = async () => {
    try {
      setScriptsLoading(true);
      setScriptsError(null);
      const response = await scriptsAPI.list();
      // Check if feature is disabled
      if (response.enabled === false) {
        setFeatureEnabled(false);
        return;
      }
      setFeatureEnabled(true);
      setScripts(response.scripts || []);
    } catch (error) {
      console.error('Error fetching scripts:', error);
      setScriptsError(error.message);
    } finally {
      setScriptsLoading(false);
    }
  };

  // Fetch executions from API
  const fetchExecutions = async () => {
    try {
      setExecutionsLoading(true);
      setExecutionsError(null);
      const response = await scriptsAPI.listExecutions({ limit: 50 });
      setExecutions(response.executions || []);
    } catch (error) {
      console.error('Error fetching executions:', error);
      setExecutionsError(error.message);
    } finally {
      setExecutionsLoading(false);
    }
  };

  // Fetch workflows from API
  const fetchWorkflows = async () => {
    try {
      setWorkflowsLoading(true);
      setWorkflowsError(null);
      const response = await scriptsAPI.listWorkflows();
      setWorkflows(response.workflows || []);
    } catch (error) {
      console.error('Error fetching workflows:', error);
      setWorkflowsError(error.message);
    } finally {
      setWorkflowsLoading(false);
    }
  };

  // Fetch schedules from API
  const fetchSchedules = async () => {
    try {
      setSchedulesLoading(true);
      setSchedulesError(null);
      const response = await scriptsAPI.listSchedules();
      setSchedules(response.schedules || []);
    } catch (error) {
      console.error('Error fetching schedules:', error);
      setSchedulesError(error.message);
    } finally {
      setSchedulesLoading(false);
    }
  };

  // Handle refresh
  const handleRefresh = async () => {
    setIsRefreshing(true);
    try {
      switch (viewMode) {
        case 'scripts':
          await fetchScripts();
          break;
        case 'executions':
          await fetchExecutions();
          break;
        case 'workflows':
          await fetchWorkflows();
          break;
        case 'schedules':
          await fetchSchedules();
          break;
        default:
          break;
      }
    } finally {
      setTimeout(() => setIsRefreshing(false), 500);
    }
  };

  // Get unique categories from scripts
  const categories = useMemo(() => {
    const cats = new Set(scripts.map(s => s.category));
    return ['all', ...Array.from(cats)];
  }, [scripts]);

  // Filter scripts by category
  const filteredScripts = useMemo(() => {
    const filtered = filterCategory === 'all'
      ? scripts
      : scripts.filter(s => s.category === filterCategory);
    return [...filtered].sort((a, b) => a.name.localeCompare(b.name));
  }, [scripts, filterCategory]);

  // Filter executions by status
  const filteredExecutions = useMemo(() => {
    if (filterStatus === 'all') return executions;
    return executions.filter(e => e.status === filterStatus);
  }, [executions, filterStatus]);

  // Calculate stats
  const totalScripts = scripts.length;
  const runningExecutions = executions.filter(e => e.status === 'running').length;
  const scheduledJobs = schedules.length;

  // Handle execute script (async mode for better UX)
  const handleExecute = async (script) => {
    try {
      addNotification({
        severity: 'info',
        message: `Starting execution of ${script.name}...`,
        strongText: 'EXECUTING:'
      });

      // Use async execution for better mobile UX
      const response = await scriptsAPI.executeAsync(script.id, {});

      addNotification({
        severity: 'success',
        message: `Execution started. ID: ${response.executionId}`,
        strongText: 'QUEUED:'
      });

      // Refresh executions to show the new entry
      await fetchExecutions();
    } catch (error) {
      console.error('Error executing script:', error);
      addNotification({
        severity: 'error',
        message: error.message || 'Failed to execute script',
        strongText: 'ERROR:'
      });
    }
  };

  // Handle dry run
  const handleDryRun = async (script) => {
    try {
      addNotification({
        severity: 'info',
        message: `Running dry-run for ${script.name}...`,
        strongText: 'DRY RUN:'
      });

      const result = await scriptsAPI.dryRun(script.id, {});

      addNotification({
        severity: result.exitCode === 0 ? 'success' : 'warning',
        message: `Dry run completed with exit code ${result.exitCode}`,
        strongText: 'RESULT:'
      });
    } catch (error) {
      console.error('Error running dry run:', error);
      addNotification({
        severity: 'error',
        message: error.message || 'Dry run failed',
        strongText: 'ERROR:'
      });
    }
  };

  // Handle edit script
  const handleEdit = (script) => {
    setSelectedScript(script);
    setEditModalOpen(true);
  };

  // Handle update script
  const handleUpdateScript = async (scriptId, scriptData) => {
    try {
      await scriptsAPI.update(scriptId, scriptData);

      // Refresh scripts list
      await fetchScripts();

      addNotification({
        severity: 'success',
        message: `Script ${scriptData.name} updated successfully`,
        strongText: 'SUCCESS:'
      });
    } catch (error) {
      console.error('Error updating script:', error);
      addNotification({
        severity: 'error',
        message: error.message || 'Failed to update script',
        strongText: 'ERROR:'
      });
      throw error; // Re-throw so the modal can handle it
    }
  };

  // Handle delete script
  const handleDelete = async (script) => {
    try {
      await scriptsAPI.delete(script.id);
      await fetchScripts();
      addNotification({
        severity: 'success',
        message: `Script ${script.name} deleted`,
        strongText: 'DELETED:'
      });
    } catch (error) {
      console.error('Error deleting script:', error);
      addNotification({
        severity: 'error',
        message: error.message || 'Failed to delete script',
        strongText: 'ERROR:'
      });
    }
  };

  // Handle view execution
  const handleViewExecution = (execution) => {
    setSelectedExecution(execution);
    setOutputModalOpen(true);
  };

  // Refresh single execution (for auto-refresh while running)
  const handleRefreshExecution = async () => {
    if (!selectedExecution?.id) return;

    try {
      const response = await fetch(`/api/v1/executions/${selectedExecution.id}`);
      if (response.ok) {
        const updatedExecution = await response.json();
        setSelectedExecution(updatedExecution);

        // Also update in the list
        setExecutions(prev =>
          prev.map(e => e.id === updatedExecution.id ? updatedExecution : e)
        );
      }
    } catch (error) {
      console.error('Error refreshing execution:', error);
    }
  };

  // Handle register new script
  const handleRegisterScript = () => {
    setRegisterModalOpen(true);
  };

  // Handle create workflow
  const handleCreateWorkflow = async (workflowData) => {
    try {
      await scriptsAPI.createWorkflow(workflowData);
      await fetchWorkflows();
      addNotification({
        severity: 'success',
        message: `Workflow "${workflowData.name}" created successfully`,
        strongText: 'SUCCESS:'
      });
    } catch (error) {
      console.error('Error creating workflow:', error);
      addNotification({
        severity: 'error',
        message: error.message || 'Failed to create workflow',
        strongText: 'ERROR:'
      });
      throw error;
    }
  };

  // Handle create schedule
  const handleCreateSchedule = async (scheduleData) => {
    try {
      await scriptsAPI.createSchedule(scheduleData);
      await fetchSchedules();
      addNotification({
        severity: 'success',
        message: 'Schedule created successfully',
        strongText: 'SUCCESS:'
      });
    } catch (error) {
      console.error('Error creating schedule:', error);
      addNotification({
        severity: 'error',
        message: error.message || 'Failed to create schedule',
        strongText: 'ERROR:'
      });
      throw error;
    }
  };

  // Handle save new script
  const handleSaveScript = async (scriptData) => {
    try {
      await scriptsAPI.register(scriptData);

      // Refresh scripts list
      await fetchScripts();

      addNotification({
        severity: 'success',
        message: `Script ${scriptData.name} registered successfully`,
        strongText: 'SUCCESS:'
      });
    } catch (error) {
      console.error('Error registering script:', error);
      addNotification({
        severity: 'error',
        message: error.message || 'Failed to register script',
        strongText: 'ERROR:'
      });
    }
  };

  // Render status badge for executions
  const renderStatusBadge = (status) => {
    const variants = {
      running: 'info',
      completed: 'success',
      failed: 'error',
      cancelled: 'secondary'
    };
    return <Badge variant={variants[status] || 'secondary'} dot filled>{status.toUpperCase()}</Badge>;
  };

  // Execution table columns
  const executionColumns = [
    {
      key: 'scriptId',
      label: 'SCRIPT',
      sortable: true
    },
    {
      key: 'status',
      label: 'STATUS',
      sortable: true,
      render: (row) => renderStatusBadge(row.status)
    },
    {
      key: 'triggeredBy',
      label: 'TRIGGERED BY',
      sortable: true,
      render: (row) => row.triggeredBy || '-'
    },
    {
      key: 'startedAt',
      label: 'STARTED',
      sortable: true,
      render: (row) => row.startedAt ? new Date(row.startedAt).toLocaleString() : '-'
    },
    {
      key: 'duration',
      label: 'DURATION',
      sortable: false,
      render: (row) => {
        if (row.startedAt && row.completedAt) {
          const start = new Date(row.startedAt);
          const end = new Date(row.completedAt);
          const durationMs = end - start;
          if (durationMs < 1000) return `${durationMs}ms`;
          return `${(durationMs / 1000).toFixed(1)}s`;
        }
        return '-';
      }
    }
  ];

  // Workflow table columns
  const workflowColumns = [
    {
      key: 'name',
      label: 'WORKFLOW',
      sortable: true
    },
    {
      key: 'description',
      label: 'DESCRIPTION',
      sortable: false
    },
    {
      key: 'steps',
      label: 'STEPS',
      sortable: true,
      render: (row) => row.steps?.length || 0
    },
    {
      key: 'last_run',
      label: 'LAST RUN',
      sortable: true,
      render: (row) => row.last_run ? new Date(row.last_run).toLocaleString() : 'Never'
    }
  ];

  // Schedule table columns
  const scheduleColumns = [
    {
      key: 'id',
      label: 'JOB',
      sortable: true,
      render: (row) => {
        // Show a short version of the ID or derive name
        return row.id?.slice(0, 8) || '-';
      }
    },
    {
      key: 'target',
      label: 'TARGET',
      sortable: true,
      render: (row) => {
        if (row.scriptId) {
          const script = scripts.find(s => s.id === row.scriptId);
          return script ? script.name : row.scriptId;
        }
        if (row.workflowId) {
          const workflow = workflows.find(w => w.id === row.workflowId);
          return workflow ? `[WF] ${workflow.name}` : row.workflowId;
        }
        return '-';
      }
    },
    {
      key: 'cronExpression',
      label: 'SCHEDULE',
      sortable: false,
      render: (row) => <code>{row.cronExpression || '-'}</code>
    },
    {
      key: 'nextRunAt',
      label: 'NEXT RUN',
      sortable: true,
      render: (row) => row.nextRunAt ? new Date(row.nextRunAt).toLocaleString() : '-'
    },
    {
      key: 'enabled',
      label: 'STATUS',
      sortable: true,
      render: (row) => <Badge variant={row.enabled ? 'success' : 'secondary'}>{row.enabled ? 'ENABLED' : 'DISABLED'}</Badge>
    }
  ];

  // Render disabled state when scripts feature is not enabled
  if (!featureEnabled && !scriptsLoading) {
    return (
      <div className="scripts-tab scripts-disabled">
        <div className="disabled-overlay">
          <div className="disabled-content">
            <h3>Scripts Disabled</h3>
            <p className="text-secondary">
              The script execution system is not enabled on this server.
            </p>
            <p className="text-secondary" style={{ marginTop: 'var(--space-3)' }}>
              To enable, use one of the following:
            </p>
            <ul className="text-secondary" style={{ marginTop: 'var(--space-2)', textAlign: 'left', display: 'inline-block' }}>
              <li>Config file: <code>scripts.enabled: true</code></li>
              <li>Environment: <code>NEKZUS_SCRIPTS_ENABLED=true</code></li>
            </ul>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="scripts-tab">
      {/* Header */}
      <div className="tab-header">
        <div className="tab-header-left">
          {/* View Mode Toggle */}
          <div className="view-mode-toggle">
            <button
              className={`btn btn-sm ${viewMode === 'scripts' ? 'btn-primary' : 'btn-secondary'}`}
              onClick={() => setViewMode('scripts')}
            >
              <FileText size={16} />
              SCRIPTS ({totalScripts})
            </button>
            <button
              className={`btn btn-sm ${viewMode === 'executions' ? 'btn-primary' : 'btn-secondary'}`}
              onClick={() => setViewMode('executions')}
            >
              <Play size={16} />
              EXECUTIONS ({runningExecutions})
            </button>
            <button
              className={`btn btn-sm ${viewMode === 'workflows' ? 'btn-primary' : 'btn-secondary'}`}
              onClick={() => setViewMode('workflows')}
            >
              <GitBranch size={16} />
              WORKFLOWS ({workflows.length})
            </button>
            <button
              className={`btn btn-sm ${viewMode === 'schedules' ? 'btn-primary' : 'btn-secondary'}`}
              onClick={() => setViewMode('schedules')}
            >
              <Clock size={16} />
              SCHEDULES ({scheduledJobs})
            </button>
          </div>

          {/* Category Filter (only for scripts view) */}
          {viewMode === 'scripts' && (
            <div className="category-filter">
              {categories.map(cat => (
                <button
                  key={cat}
                  className={`btn btn-sm ${filterCategory === cat ? 'btn-primary' : 'btn-secondary'}`}
                  onClick={() => setFilterCategory(cat)}
                >
                  {cat.toUpperCase()}
                </button>
              ))}
            </div>
          )}

          {/* Status Filter (only for executions view) */}
          {viewMode === 'executions' && (
            <div className="filter-buttons">
              <button
                className={`btn btn-sm ${filterStatus === 'all' ? 'btn-primary' : 'btn-secondary'}`}
                onClick={() => setFilterStatus('all')}
              >
                ALL
              </button>
              <button
                className={`btn btn-sm ${filterStatus === 'running' ? 'btn-primary' : 'btn-secondary'}`}
                onClick={() => setFilterStatus('running')}
              >
                RUNNING
              </button>
              <button
                className={`btn btn-sm ${filterStatus === 'completed' ? 'btn-primary' : 'btn-secondary'}`}
                onClick={() => setFilterStatus('completed')}
              >
                COMPLETED
              </button>
              <button
                className={`btn btn-sm ${filterStatus === 'failed' ? 'btn-primary' : 'btn-secondary'}`}
                onClick={() => setFilterStatus('failed')}
              >
                FAILED
              </button>
            </div>
          )}
        </div>

        <div className="tab-header-right">
          {viewMode === 'scripts' && (
            <button
              className="btn btn-primary"
              onClick={handleRegisterScript}
              aria-label="Register new script"
            >
              <Plus size={16} />
              REGISTER SCRIPT
            </button>
          )}
          {viewMode === 'workflows' && (
            <button
              className="btn btn-primary"
              onClick={() => setWorkflowModalOpen(true)}
              aria-label="Create new workflow"
            >
              <Plus size={16} />
              CREATE WORKFLOW
            </button>
          )}
          {viewMode === 'schedules' && (
            <button
              className="btn btn-primary"
              onClick={() => setScheduleModalOpen(true)}
              aria-label="Create new schedule"
            >
              <Plus size={16} />
              CREATE SCHEDULE
            </button>
          )}
          <button
            className="btn btn-secondary"
            onClick={handleRefresh}
            disabled={isRefreshing}
            aria-label="Refresh"
          >
            <RefreshCw size={16} className={isRefreshing ? 'spinning' : ''} />
            REFRESH
          </button>
        </div>
      </div>

      {/* Scripts View */}
      {viewMode === 'scripts' && (
        <>
          {/* Loading State */}
          {scriptsLoading && !isRefreshing && (
            <div className="loading-state">
              <p>Loading scripts...</p>
            </div>
          )}

          {/* Error State */}
          {scriptsError && (
            <div className="error-state">
              <h3>Error Loading Scripts</h3>
              <p className="text-secondary">{scriptsError}</p>
              <button className="btn btn-primary" onClick={fetchScripts}>
                TRY AGAIN
              </button>
            </div>
          )}

          {/* Scripts Grid */}
          {!scriptsLoading && !scriptsError && (
            <>
              {filteredScripts.length > 0 ? (
                <div className="script-grid">
                  {filteredScripts.map((script) => (
                    <ScriptCard
                      key={script.id}
                      script={script}
                      onExecute={handleExecute}
                      onDryRun={handleDryRun}
                      onEdit={handleEdit}
                      onDelete={handleDelete}
                    />
                  ))}
                </div>
              ) : (
                <div className="empty-state">
                  <h3>No Scripts Found</h3>
                  <p className="text-secondary">
                    {filterCategory === 'all'
                      ? 'No scripts are currently registered.'
                      : `No scripts found in the ${filterCategory} category.`}
                  </p>
                  {filterCategory !== 'all' ? (
                    <button
                      className="btn btn-secondary"
                      onClick={() => setFilterCategory('all')}
                    >
                      SHOW ALL SCRIPTS
                    </button>
                  ) : (
                    <button
                      className="btn btn-primary"
                      onClick={handleRegisterScript}
                    >
                      REGISTER YOUR FIRST SCRIPT
                    </button>
                  )}
                </div>
              )}
            </>
          )}
        </>
      )}

      {/* Executions View */}
      {viewMode === 'executions' && (
        <>
          {/* Loading State */}
          {executionsLoading && !isRefreshing && (
            <div className="loading-state">
              <p>Loading executions...</p>
            </div>
          )}

          {/* Error State */}
          {executionsError && (
            <div className="error-state">
              <h3>Error Loading Executions</h3>
              <p className="text-secondary">{executionsError}</p>
              <button className="btn btn-primary" onClick={fetchExecutions}>
                TRY AGAIN
              </button>
            </div>
          )}

          {/* Executions Table */}
          {!executionsLoading && !executionsError && (
            <div className="executions-table">
              <Table
                columns={executionColumns}
                data={filteredExecutions}
                onEdit={handleViewExecution}
                editLabel="VIEW"
                searchable
                defaultSortColumn="scriptId"
                defaultSortDirection="asc"
                emptyMessage={
                  filterStatus === 'all'
                    ? 'No executions found.'
                    : `No ${filterStatus} executions found.`
                }
              />
            </div>
          )}
        </>
      )}

      {/* Workflows View */}
      {viewMode === 'workflows' && (
        <>
          {/* Loading State */}
          {workflowsLoading && !isRefreshing && (
            <div className="loading-state">
              <p>Loading workflows...</p>
            </div>
          )}

          {/* Error State */}
          {workflowsError && (
            <div className="error-state">
              <h3>Error Loading Workflows</h3>
              <p className="text-secondary">{workflowsError}</p>
              <button className="btn btn-primary" onClick={fetchWorkflows}>
                TRY AGAIN
              </button>
            </div>
          )}

          {/* Workflows Table */}
          {!workflowsLoading && !workflowsError && (
            <div className="workflows-table">
              <Table
                columns={workflowColumns}
                data={workflows}
                searchable
                defaultSortColumn="name"
                defaultSortDirection="asc"
                emptyMessage="No workflows defined."
              />
            </div>
          )}
        </>
      )}

      {/* Schedules View */}
      {viewMode === 'schedules' && (
        <>
          {/* Loading State */}
          {schedulesLoading && !isRefreshing && (
            <div className="loading-state">
              <p>Loading schedules...</p>
            </div>
          )}

          {/* Error State */}
          {schedulesError && (
            <div className="error-state">
              <h3>Error Loading Schedules</h3>
              <p className="text-secondary">{schedulesError}</p>
              <button className="btn btn-primary" onClick={fetchSchedules}>
                TRY AGAIN
              </button>
            </div>
          )}

          {/* Schedules Table */}
          {!schedulesLoading && !schedulesError && (
            <div className="schedules-table">
              <Table
                columns={scheduleColumns}
                data={schedules}
                searchable
                defaultSortColumn="name"
                defaultSortDirection="asc"
                emptyMessage="No scheduled jobs configured."
              />
            </div>
          )}
        </>
      )}

      {/* Register Script Modal */}
      <RegisterScriptModal
        isOpen={registerModalOpen}
        onClose={() => setRegisterModalOpen(false)}
        onSave={handleSaveScript}
      />

      {/* Edit Script Modal */}
      <EditScriptModal
        isOpen={editModalOpen}
        onClose={() => {
          setEditModalOpen(false);
          setSelectedScript(null);
        }}
        onSave={handleUpdateScript}
        script={selectedScript}
      />

      {/* Execution Output Modal */}
      <ExecutionOutputModal
        isOpen={outputModalOpen}
        onClose={() => {
          setOutputModalOpen(false);
          setSelectedExecution(null);
        }}
        execution={selectedExecution}
        onRefresh={handleRefreshExecution}
      />

      {/* Create Workflow Modal */}
      <CreateWorkflowModal
        isOpen={workflowModalOpen}
        onClose={() => setWorkflowModalOpen(false)}
        onSave={handleCreateWorkflow}
        scripts={scripts}
      />

      {/* Create Schedule Modal */}
      <CreateScheduleModal
        isOpen={scheduleModalOpen}
        onClose={() => setScheduleModalOpen(false)}
        onSave={handleCreateSchedule}
        scripts={scripts}
        workflows={workflows}
      />
    </div>
  );
}

/**
 * ScriptCard Component - Stub
 * Shows individual script information and actions
 */
function ScriptCard({ script, onExecute, onDryRun, onEdit, onDelete }) {
  return (
    <div className="script-card">
      <div className="script-card-header">
        <h3 className="script-card-title">{script.name}</h3>
        <Badge variant="primary">{formatScriptType(script.script_type)}</Badge>
      </div>
      <div className="script-card-body">
        <p className="script-card-description">{script.description}</p>
        <div className="script-card-meta">
          <span className="meta-item">
            <span className="meta-label">Category:</span>
            <span className="meta-value">{script.category}</span>
          </span>
          {script.last_run && (
            <span className="meta-item">
              <span className="meta-label">Last Run:</span>
              <span className="meta-value">{new Date(script.last_run).toLocaleString()}</span>
            </span>
          )}
        </div>
      </div>
      <div className="script-card-actions">
        <button
          className="btn btn-sm btn-primary"
          onClick={() => onExecute(script)}
          aria-label={`Execute ${script.name}`}
        >
          <Play size={14} />
          EXECUTE
        </button>
        <button
          className="btn btn-sm btn-secondary"
          onClick={() => onDryRun(script)}
          aria-label={`Dry run ${script.name}`}
        >
          DRY RUN
        </button>
        <button
          className="btn btn-sm btn-secondary"
          onClick={() => onEdit(script)}
          aria-label={`Edit ${script.name}`}
        >
          EDIT
        </button>
        <button
          className="btn btn-sm btn-error"
          onClick={() => onDelete(script)}
          aria-label={`Delete ${script.name}`}
        >
          DELETE
        </button>
      </div>
    </div>
  );
}

ScriptCard.propTypes = {
  script: PropTypes.shape({
    id: PropTypes.string.isRequired,
    name: PropTypes.string.isRequired,
    description: PropTypes.string,
    category: PropTypes.string.isRequired,
    script_type: PropTypes.string.isRequired,
    last_run: PropTypes.string
  }).isRequired,
  onExecute: PropTypes.func.isRequired,
  onDryRun: PropTypes.func.isRequired,
  onEdit: PropTypes.func.isRequired,
  onDelete: PropTypes.func.isRequired
};
