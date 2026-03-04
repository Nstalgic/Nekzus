package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nstalgic/nekzus/internal/scripts"
)

// Script CRUD operations

// SaveScript saves or updates a script in the database.
func (s *Store) SaveScript(script *scripts.Script) error {
	parametersJSON, err := json.Marshal(script.Parameters)
	if err != nil {
		return fmt.Errorf("failed to marshal parameters: %w", err)
	}

	environmentJSON, err := json.Marshal(script.Environment)
	if err != nil {
		return fmt.Errorf("failed to marshal environment: %w", err)
	}

	allowedScopesJSON, err := json.Marshal(script.AllowedScopes)
	if err != nil {
		return fmt.Errorf("failed to marshal allowed_scopes: %w", err)
	}

	query := `
		INSERT INTO scripts (id, name, description, category, script_path, script_type,
			timeout_seconds, parameters, environment, allowed_scopes, dry_run_command,
			created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			description = excluded.description,
			category = excluded.category,
			script_path = excluded.script_path,
			script_type = excluded.script_type,
			timeout_seconds = excluded.timeout_seconds,
			parameters = excluded.parameters,
			environment = excluded.environment,
			allowed_scopes = excluded.allowed_scopes,
			dry_run_command = excluded.dry_run_command,
			updated_at = CURRENT_TIMESTAMP
	`

	_, err = s.db.Exec(query,
		script.ID, script.Name, script.Description, script.Category,
		script.ScriptPath, string(script.ScriptType), script.TimeoutSeconds,
		string(parametersJSON), string(environmentJSON), string(allowedScopesJSON),
		script.DryRunCommand, script.CreatedBy, script.CreatedAt, script.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to save script: %w", err)
	}

	return nil
}

// GetScript retrieves a script by ID.
func (s *Store) GetScript(id string) (*scripts.Script, error) {
	query := `
		SELECT id, name, description, category, script_path, script_type,
			timeout_seconds, parameters, environment, allowed_scopes, dry_run_command,
			created_by, created_at, updated_at
		FROM scripts WHERE id = ?
	`

	var script scripts.Script
	var parametersJSON, environmentJSON, allowedScopesJSON string
	var scriptType string
	var description, createdBy sql.NullString

	err := s.db.QueryRow(query, id).Scan(
		&script.ID, &script.Name, &description, &script.Category,
		&script.ScriptPath, &scriptType, &script.TimeoutSeconds,
		&parametersJSON, &environmentJSON, &allowedScopesJSON,
		&script.DryRunCommand, &createdBy, &script.CreatedAt, &script.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get script: %w", err)
	}

	script.ScriptType = scripts.ScriptType(scriptType)
	if description.Valid {
		script.Description = description.String
	}
	if createdBy.Valid {
		script.CreatedBy = createdBy.String
	}

	if err := json.Unmarshal([]byte(parametersJSON), &script.Parameters); err != nil {
		return nil, fmt.Errorf("failed to unmarshal parameters: %w", err)
	}

	if environmentJSON != "" && environmentJSON != "null" {
		if err := json.Unmarshal([]byte(environmentJSON), &script.Environment); err != nil {
			return nil, fmt.Errorf("failed to unmarshal environment: %w", err)
		}
	}

	if err := json.Unmarshal([]byte(allowedScopesJSON), &script.AllowedScopes); err != nil {
		return nil, fmt.Errorf("failed to unmarshal allowed_scopes: %w", err)
	}

	return &script, nil
}

// ListScripts retrieves all scripts.
func (s *Store) ListScripts() ([]*scripts.Script, error) {
	query := `
		SELECT id, name, description, category, script_path, script_type,
			timeout_seconds, parameters, environment, allowed_scopes, dry_run_command,
			created_by, created_at, updated_at
		FROM scripts ORDER BY name
	`

	return s.queryScripts(query)
}

// ListScriptsByCategory retrieves scripts filtered by category.
func (s *Store) ListScriptsByCategory(category string) ([]*scripts.Script, error) {
	query := `
		SELECT id, name, description, category, script_path, script_type,
			timeout_seconds, parameters, environment, allowed_scopes, dry_run_command,
			created_by, created_at, updated_at
		FROM scripts WHERE category = ? ORDER BY name
	`

	return s.queryScripts(query, category)
}

// queryScripts is a helper for querying multiple scripts.
func (s *Store) queryScripts(query string, args ...interface{}) ([]*scripts.Script, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query scripts: %w", err)
	}
	defer rows.Close()

	var result []*scripts.Script
	for rows.Next() {
		var script scripts.Script
		var parametersJSON, environmentJSON, allowedScopesJSON string
		var scriptType string
		var description, createdBy sql.NullString

		if err := rows.Scan(
			&script.ID, &script.Name, &description, &script.Category,
			&script.ScriptPath, &scriptType, &script.TimeoutSeconds,
			&parametersJSON, &environmentJSON, &allowedScopesJSON,
			&script.DryRunCommand, &createdBy, &script.CreatedAt, &script.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan script: %w", err)
		}

		script.ScriptType = scripts.ScriptType(scriptType)
		if description.Valid {
			script.Description = description.String
		}
		if createdBy.Valid {
			script.CreatedBy = createdBy.String
		}

		if err := json.Unmarshal([]byte(parametersJSON), &script.Parameters); err != nil {
			return nil, fmt.Errorf("failed to unmarshal parameters: %w", err)
		}

		if environmentJSON != "" && environmentJSON != "null" {
			if err := json.Unmarshal([]byte(environmentJSON), &script.Environment); err != nil {
				return nil, fmt.Errorf("failed to unmarshal environment: %w", err)
			}
		}

		if err := json.Unmarshal([]byte(allowedScopesJSON), &script.AllowedScopes); err != nil {
			return nil, fmt.Errorf("failed to unmarshal allowed_scopes: %w", err)
		}

		result = append(result, &script)
	}

	return result, nil
}

// DeleteScript deletes a script by ID.
func (s *Store) DeleteScript(id string) error {
	_, err := s.db.Exec("DELETE FROM scripts WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete script: %w", err)
	}
	return nil
}

// Execution CRUD operations

// SaveExecution saves a script execution record.
func (s *Store) SaveExecution(execution *scripts.Execution) error {
	parametersJSON, err := json.Marshal(execution.Parameters)
	if err != nil {
		return fmt.Errorf("failed to marshal parameters: %w", err)
	}

	query := `
		INSERT INTO script_executions (id, script_id, workflow_id, workflow_ex_id, status,
			is_dry_run, triggered_by, triggered_ip, parameters, output, exit_code,
			error_message, started_at, completed_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			status = excluded.status,
			output = excluded.output,
			exit_code = excluded.exit_code,
			error_message = excluded.error_message,
			started_at = excluded.started_at,
			completed_at = excluded.completed_at
	`

	_, err = s.db.Exec(query,
		execution.ID, execution.ScriptID, nullString(execution.WorkflowID),
		nullString(execution.WorkflowExID), string(execution.Status),
		execution.IsDryRun, execution.TriggeredBy, nullString(execution.TriggeredIP),
		string(parametersJSON), nullString(execution.Output), execution.ExitCode,
		nullString(execution.ErrorMessage), nullTime(execution.StartedAt),
		nullTime(execution.CompletedAt), execution.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to save execution: %w", err)
	}

	return nil
}

// GetExecution retrieves an execution by ID.
func (s *Store) GetExecution(id string) (*scripts.Execution, error) {
	query := `
		SELECT id, script_id, workflow_id, workflow_ex_id, status, is_dry_run,
			triggered_by, triggered_ip, parameters, output, exit_code,
			error_message, started_at, completed_at, created_at
		FROM script_executions WHERE id = ?
	`

	var exec scripts.Execution
	var status string
	var parametersJSON string
	var workflowID, workflowExID, triggeredIP, output, errorMessage sql.NullString
	var exitCode sql.NullInt64
	var startedAt, completedAt sql.NullTime

	err := s.db.QueryRow(query, id).Scan(
		&exec.ID, &exec.ScriptID, &workflowID, &workflowExID, &status,
		&exec.IsDryRun, &exec.TriggeredBy, &triggeredIP, &parametersJSON,
		&output, &exitCode, &errorMessage, &startedAt, &completedAt, &exec.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get execution: %w", err)
	}

	exec.Status = scripts.ExecutionStatus(status)
	if workflowID.Valid {
		exec.WorkflowID = workflowID.String
	}
	if workflowExID.Valid {
		exec.WorkflowExID = workflowExID.String
	}
	if triggeredIP.Valid {
		exec.TriggeredIP = triggeredIP.String
	}
	if output.Valid {
		exec.Output = output.String
	}
	if errorMessage.Valid {
		exec.ErrorMessage = errorMessage.String
	}
	if exitCode.Valid {
		code := int(exitCode.Int64)
		exec.ExitCode = &code
	}
	if startedAt.Valid {
		exec.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		exec.CompletedAt = &completedAt.Time
	}

	if err := json.Unmarshal([]byte(parametersJSON), &exec.Parameters); err != nil {
		return nil, fmt.Errorf("failed to unmarshal parameters: %w", err)
	}

	return &exec, nil
}

// UpdateExecutionStatus updates the status of an execution.
func (s *Store) UpdateExecutionStatus(id string, status scripts.ExecutionStatus, startedAt, completedAt *time.Time, exitCode *int, output *string, errorMessage string) error {
	query := `
		UPDATE script_executions SET
			status = ?,
			started_at = COALESCE(?, started_at),
			completed_at = COALESCE(?, completed_at),
			exit_code = COALESCE(?, exit_code),
			output = COALESCE(?, output),
			error_message = COALESCE(?, error_message)
		WHERE id = ?
	`

	_, err := s.db.Exec(query, string(status), nullTime(startedAt), nullTime(completedAt),
		nullInt(exitCode), nullStringPtr(output), nullString(errorMessage), id)
	if err != nil {
		return fmt.Errorf("failed to update execution status: %w", err)
	}

	return nil
}

// ListExecutions lists executions with optional filters.
func (s *Store) ListExecutions(scriptID, status string, limit, offset int) ([]*scripts.Execution, error) {
	query := `
		SELECT id, script_id, workflow_id, workflow_ex_id, status, is_dry_run,
			triggered_by, triggered_ip, parameters, output, exit_code,
			error_message, started_at, completed_at, created_at
		FROM script_executions
		WHERE (? = '' OR script_id = ?)
		AND (? = '' OR status = ?)
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`

	rows, err := s.db.Query(query, scriptID, scriptID, status, status, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query executions: %w", err)
	}
	defer rows.Close()

	var result []*scripts.Execution
	for rows.Next() {
		var exec scripts.Execution
		var statusStr string
		var parametersJSON string
		var workflowID, workflowExID, triggeredIP, output, errorMessage sql.NullString
		var exitCode sql.NullInt64
		var startedAt, completedAt sql.NullTime

		if err := rows.Scan(
			&exec.ID, &exec.ScriptID, &workflowID, &workflowExID, &statusStr,
			&exec.IsDryRun, &exec.TriggeredBy, &triggeredIP, &parametersJSON,
			&output, &exitCode, &errorMessage, &startedAt, &completedAt, &exec.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan execution: %w", err)
		}

		exec.Status = scripts.ExecutionStatus(statusStr)
		if workflowID.Valid {
			exec.WorkflowID = workflowID.String
		}
		if workflowExID.Valid {
			exec.WorkflowExID = workflowExID.String
		}
		if triggeredIP.Valid {
			exec.TriggeredIP = triggeredIP.String
		}
		if output.Valid {
			exec.Output = output.String
		}
		if errorMessage.Valid {
			exec.ErrorMessage = errorMessage.String
		}
		if exitCode.Valid {
			code := int(exitCode.Int64)
			exec.ExitCode = &code
		}
		if startedAt.Valid {
			exec.StartedAt = &startedAt.Time
		}
		if completedAt.Valid {
			exec.CompletedAt = &completedAt.Time
		}

		if err := json.Unmarshal([]byte(parametersJSON), &exec.Parameters); err != nil {
			return nil, fmt.Errorf("failed to unmarshal parameters: %w", err)
		}

		result = append(result, &exec)
	}

	return result, nil
}

// Workflow CRUD operations

// SaveWorkflow saves or updates a workflow.
func (s *Store) SaveWorkflow(workflow *scripts.Workflow) error {
	stepsJSON, err := json.Marshal(workflow.Steps)
	if err != nil {
		return fmt.Errorf("failed to marshal steps: %w", err)
	}

	allowedScopesJSON, err := json.Marshal(workflow.AllowedScopes)
	if err != nil {
		return fmt.Errorf("failed to marshal allowed_scopes: %w", err)
	}

	query := `
		INSERT INTO workflows (id, name, description, steps, allowed_scopes,
			created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			description = excluded.description,
			steps = excluded.steps,
			allowed_scopes = excluded.allowed_scopes,
			updated_at = CURRENT_TIMESTAMP
	`

	_, err = s.db.Exec(query,
		workflow.ID, workflow.Name, workflow.Description,
		string(stepsJSON), string(allowedScopesJSON),
		workflow.CreatedBy, workflow.CreatedAt, workflow.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to save workflow: %w", err)
	}

	return nil
}

// GetWorkflow retrieves a workflow by ID.
func (s *Store) GetWorkflow(id string) (*scripts.Workflow, error) {
	query := `
		SELECT id, name, description, steps, allowed_scopes, created_by, created_at, updated_at
		FROM workflows WHERE id = ?
	`

	var workflow scripts.Workflow
	var stepsJSON, allowedScopesJSON string
	var description, createdBy sql.NullString

	err := s.db.QueryRow(query, id).Scan(
		&workflow.ID, &workflow.Name, &description, &stepsJSON,
		&allowedScopesJSON, &createdBy, &workflow.CreatedAt, &workflow.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow: %w", err)
	}

	if description.Valid {
		workflow.Description = description.String
	}
	if createdBy.Valid {
		workflow.CreatedBy = createdBy.String
	}

	if err := json.Unmarshal([]byte(stepsJSON), &workflow.Steps); err != nil {
		return nil, fmt.Errorf("failed to unmarshal steps: %w", err)
	}

	if err := json.Unmarshal([]byte(allowedScopesJSON), &workflow.AllowedScopes); err != nil {
		return nil, fmt.Errorf("failed to unmarshal allowed_scopes: %w", err)
	}

	return &workflow, nil
}

// ListWorkflows retrieves all workflows.
func (s *Store) ListWorkflows() ([]*scripts.Workflow, error) {
	query := `
		SELECT id, name, description, steps, allowed_scopes, created_by, created_at, updated_at
		FROM workflows ORDER BY name
	`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query workflows: %w", err)
	}
	defer rows.Close()

	var result []*scripts.Workflow
	for rows.Next() {
		var workflow scripts.Workflow
		var stepsJSON, allowedScopesJSON string
		var description, createdBy sql.NullString

		if err := rows.Scan(
			&workflow.ID, &workflow.Name, &description, &stepsJSON,
			&allowedScopesJSON, &createdBy, &workflow.CreatedAt, &workflow.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan workflow: %w", err)
		}

		if description.Valid {
			workflow.Description = description.String
		}
		if createdBy.Valid {
			workflow.CreatedBy = createdBy.String
		}

		if err := json.Unmarshal([]byte(stepsJSON), &workflow.Steps); err != nil {
			return nil, fmt.Errorf("failed to unmarshal steps: %w", err)
		}

		if err := json.Unmarshal([]byte(allowedScopesJSON), &workflow.AllowedScopes); err != nil {
			return nil, fmt.Errorf("failed to unmarshal allowed_scopes: %w", err)
		}

		result = append(result, &workflow)
	}

	return result, nil
}

// DeleteWorkflow deletes a workflow by ID.
func (s *Store) DeleteWorkflow(id string) error {
	_, err := s.db.Exec("DELETE FROM workflows WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete workflow: %w", err)
	}
	return nil
}

// Workflow Execution operations

// SaveWorkflowExecution saves a workflow execution record.
func (s *Store) SaveWorkflowExecution(wfExec *scripts.WorkflowExecution) error {
	query := `
		INSERT INTO workflow_executions (id, workflow_id, status, current_step,
			triggered_by, started_at, completed_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			status = excluded.status,
			current_step = excluded.current_step,
			started_at = excluded.started_at,
			completed_at = excluded.completed_at
	`

	_, err := s.db.Exec(query,
		wfExec.ID, wfExec.WorkflowID, string(wfExec.Status), wfExec.CurrentStep,
		wfExec.TriggeredBy, nullTime(wfExec.StartedAt), nullTime(wfExec.CompletedAt),
		wfExec.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to save workflow execution: %w", err)
	}

	return nil
}

// GetWorkflowExecution retrieves a workflow execution by ID.
func (s *Store) GetWorkflowExecution(id string) (*scripts.WorkflowExecution, error) {
	query := `
		SELECT id, workflow_id, status, current_step, triggered_by,
			started_at, completed_at, created_at
		FROM workflow_executions WHERE id = ?
	`

	var wfExec scripts.WorkflowExecution
	var status string
	var startedAt, completedAt sql.NullTime

	err := s.db.QueryRow(query, id).Scan(
		&wfExec.ID, &wfExec.WorkflowID, &status, &wfExec.CurrentStep,
		&wfExec.TriggeredBy, &startedAt, &completedAt, &wfExec.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow execution: %w", err)
	}

	wfExec.Status = scripts.ExecutionStatus(status)
	if startedAt.Valid {
		wfExec.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		wfExec.CompletedAt = &completedAt.Time
	}

	return &wfExec, nil
}

// UpdateWorkflowExecutionStep updates the current step and status of a workflow execution.
func (s *Store) UpdateWorkflowExecutionStep(id string, step int, status scripts.ExecutionStatus) error {
	query := `UPDATE workflow_executions SET current_step = ?, status = ? WHERE id = ?`

	_, err := s.db.Exec(query, step, string(status), id)
	if err != nil {
		return fmt.Errorf("failed to update workflow execution step: %w", err)
	}

	return nil
}

// Schedule CRUD operations

// SaveSchedule saves or updates a schedule.
func (s *Store) SaveSchedule(schedule *scripts.Schedule) error {
	parametersJSON, err := json.Marshal(schedule.Parameters)
	if err != nil {
		return fmt.Errorf("failed to marshal parameters: %w", err)
	}

	query := `
		INSERT INTO script_schedules (id, script_id, workflow_id, cron_expression,
			parameters, enabled, last_run_at, next_run_at, created_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			script_id = excluded.script_id,
			workflow_id = excluded.workflow_id,
			cron_expression = excluded.cron_expression,
			parameters = excluded.parameters,
			enabled = excluded.enabled,
			last_run_at = excluded.last_run_at,
			next_run_at = excluded.next_run_at
	`

	_, err = s.db.Exec(query,
		schedule.ID, nullString(schedule.ScriptID), nullString(schedule.WorkflowID),
		schedule.CronExpression, string(parametersJSON), schedule.Enabled,
		nullTime(schedule.LastRunAt), nullTime(schedule.NextRunAt),
		schedule.CreatedBy, schedule.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to save schedule: %w", err)
	}

	return nil
}

// GetSchedule retrieves a schedule by ID.
func (s *Store) GetSchedule(id string) (*scripts.Schedule, error) {
	query := `
		SELECT id, script_id, workflow_id, cron_expression, parameters, enabled,
			last_run_at, next_run_at, created_by, created_at
		FROM script_schedules WHERE id = ?
	`

	var schedule scripts.Schedule
	var parametersJSON string
	var scriptID, workflowID, createdBy sql.NullString
	var lastRunAt, nextRunAt sql.NullTime

	err := s.db.QueryRow(query, id).Scan(
		&schedule.ID, &scriptID, &workflowID, &schedule.CronExpression,
		&parametersJSON, &schedule.Enabled, &lastRunAt, &nextRunAt,
		&createdBy, &schedule.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get schedule: %w", err)
	}

	if scriptID.Valid {
		schedule.ScriptID = scriptID.String
	}
	if workflowID.Valid {
		schedule.WorkflowID = workflowID.String
	}
	if createdBy.Valid {
		schedule.CreatedBy = createdBy.String
	}
	if lastRunAt.Valid {
		schedule.LastRunAt = &lastRunAt.Time
	}
	if nextRunAt.Valid {
		schedule.NextRunAt = &nextRunAt.Time
	}

	if parametersJSON != "" && parametersJSON != "null" {
		if err := json.Unmarshal([]byte(parametersJSON), &schedule.Parameters); err != nil {
			return nil, fmt.Errorf("failed to unmarshal parameters: %w", err)
		}
	}

	return &schedule, nil
}

// ListSchedules retrieves all schedules.
func (s *Store) ListSchedules() ([]*scripts.Schedule, error) {
	query := `
		SELECT id, script_id, workflow_id, cron_expression, parameters, enabled,
			last_run_at, next_run_at, created_by, created_at
		FROM script_schedules ORDER BY created_at
	`

	return s.querySchedules(query)
}

// ListEnabledSchedules retrieves all enabled schedules.
func (s *Store) ListEnabledSchedules() ([]*scripts.Schedule, error) {
	query := `
		SELECT id, script_id, workflow_id, cron_expression, parameters, enabled,
			last_run_at, next_run_at, created_by, created_at
		FROM script_schedules WHERE enabled = 1 ORDER BY next_run_at
	`

	return s.querySchedules(query)
}

// querySchedules is a helper for querying multiple schedules.
func (s *Store) querySchedules(query string, args ...interface{}) ([]*scripts.Schedule, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query schedules: %w", err)
	}
	defer rows.Close()

	var result []*scripts.Schedule
	for rows.Next() {
		var schedule scripts.Schedule
		var parametersJSON string
		var scriptID, workflowID, createdBy sql.NullString
		var lastRunAt, nextRunAt sql.NullTime

		if err := rows.Scan(
			&schedule.ID, &scriptID, &workflowID, &schedule.CronExpression,
			&parametersJSON, &schedule.Enabled, &lastRunAt, &nextRunAt,
			&createdBy, &schedule.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan schedule: %w", err)
		}

		if scriptID.Valid {
			schedule.ScriptID = scriptID.String
		}
		if workflowID.Valid {
			schedule.WorkflowID = workflowID.String
		}
		if createdBy.Valid {
			schedule.CreatedBy = createdBy.String
		}
		if lastRunAt.Valid {
			schedule.LastRunAt = &lastRunAt.Time
		}
		if nextRunAt.Valid {
			schedule.NextRunAt = &nextRunAt.Time
		}

		if parametersJSON != "" && parametersJSON != "null" {
			if err := json.Unmarshal([]byte(parametersJSON), &schedule.Parameters); err != nil {
				return nil, fmt.Errorf("failed to unmarshal parameters: %w", err)
			}
		}

		result = append(result, &schedule)
	}

	return result, nil
}

// UpdateScheduleLastRun updates the last run and next run times.
func (s *Store) UpdateScheduleLastRun(id string, lastRun, nextRun time.Time) error {
	query := `UPDATE script_schedules SET last_run_at = ?, next_run_at = ? WHERE id = ?`

	_, err := s.db.Exec(query, lastRun, nextRun, id)
	if err != nil {
		return fmt.Errorf("failed to update schedule last run: %w", err)
	}

	return nil
}

// DeleteSchedule deletes a schedule by ID.
func (s *Store) DeleteSchedule(id string) error {
	_, err := s.db.Exec("DELETE FROM script_schedules WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete schedule: %w", err)
	}
	return nil
}

// Helper functions for nullable fields

func nullString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func nullStringPtr(s *string) interface{} {
	if s == nil || *s == "" {
		return nil
	}
	return *s
}

func nullTime(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return *t
}

func nullInt(i *int) interface{} {
	if i == nil {
		return nil
	}
	return *i
}
