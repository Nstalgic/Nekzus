import Tabs from "@theme/Tabs";
import TabItem from "@theme/TabItem";

# Scripts

The Scripts feature enables automated execution of shell scripts, Python scripts, and Go binaries directly from Nekzus. It provides a secure, managed environment for running custom automation tasks with parameter validation, execution tracking, workflows, and scheduled jobs.

---

## Overview

The Scripts system provides:

- **Script Registration** - Register and manage scripts from a designated directory
- **Parameterized Execution** - Define parameters with validation and default values
- **Dry Run Mode** - Test scripts without making actual changes
- **Execution History** - Track all script runs with output capture
- **Workflows** - Chain multiple scripts together with failure handling
- **Scheduled Jobs** - Run scripts on cron-based schedules
- **Output Management** - Capture and truncate large outputs automatically

:::info[Script Types]

Nekzus supports three script types: shell scripts (`.sh`, `.bash`), Python scripts (`.py`), and compiled Go binaries.

:::


---

## Configuration

Enable the Scripts feature in your `config.yaml`:

```yaml
scripts:
  enabled: true
  directory: "./scripts"           # Directory containing script files
  default_timeout: 300             # Default timeout in seconds (5 minutes)
  max_output_bytes: 10485760       # Maximum output size (10MB)
```

| Option | Description | Default |
|--------|-------------|---------|
| `enabled` | Enable/disable the Scripts feature | `false` |
| `directory` | Path to directory containing script files | `./scripts` |
| `default_timeout` | Default execution timeout in seconds | `300` (5 minutes) |
| `max_output_bytes` | Maximum output capture size in bytes | `10485760` (10MB) |

---

## Directory Structure

Create a `scripts` directory containing your automation scripts:

```
scripts/
├── backups/
│   ├── backup-database.sh
│   └── cleanup-old-backups.sh
├── maintenance/
│   ├── restart-services.sh
│   └── clear-cache.py
├── deployments/
│   └── deploy-app.sh
└── utilities/
    ├── health-check.sh
    └── generate-report.py
```

Scripts can be organized into subdirectories. The relative path becomes part of the script identifier.

---

## Creating Scripts

### Shell Scripts

Create executable shell scripts with proper shebang:

```bash
#!/bin/bash
# backup-database.sh - Backup PostgreSQL database

set -e

# Parameters are passed as environment variables
DB_HOST="${DB_HOST:-localhost}"
DB_NAME="${DB_NAME:?Database name required}"
DB_USER="${DB_USER:?Database user required}"
BACKUP_DIR="${BACKUP_DIR:-/backups}"

# Dry run support
if [ "$DRY_RUN" = "true" ]; then
    echo "[DRY RUN] Would backup $DB_NAME to $BACKUP_DIR"
    exit 0
fi

# Perform backup
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
BACKUP_FILE="$BACKUP_DIR/${DB_NAME}_${TIMESTAMP}.sql.gz"

echo "Backing up $DB_NAME to $BACKUP_FILE..."
pg_dump -h "$DB_HOST" -U "$DB_USER" "$DB_NAME" | gzip > "$BACKUP_FILE"

echo "Backup completed successfully: $BACKUP_FILE"
```

Make the script executable:

```bash
chmod +x scripts/backups/backup-database.sh
```

### Python Scripts

Python scripts are executed with `python3`:

```python
#!/usr/bin/env python3
"""generate-report.py - Generate system status report"""

import os
import json
from datetime import datetime

# Parameters from environment
OUTPUT_FORMAT = os.environ.get('OUTPUT_FORMAT', 'json')
INCLUDE_METRICS = os.environ.get('INCLUDE_METRICS', 'true').lower() == 'true'
DRY_RUN = os.environ.get('DRY_RUN', 'false').lower() == 'true'

def generate_report():
    report = {
        'timestamp': datetime.now().isoformat(),
        'status': 'healthy',
        'version': '1.0.0'
    }

    if INCLUDE_METRICS:
        report['metrics'] = {
            'cpu_usage': 45.2,
            'memory_usage': 62.1,
            'disk_usage': 38.5
        }

    return report

if __name__ == '__main__':
    if DRY_RUN:
        print(f"[DRY RUN] Would generate {OUTPUT_FORMAT} report")
    else:
        report = generate_report()
        if OUTPUT_FORMAT == 'json':
            print(json.dumps(report, indent=2))
        else:
            for key, value in report.items():
                print(f"{key}: {value}")
```

### Go Binaries

Compiled Go binaries are executed directly:

```go
// main.go
package main

import (
    "fmt"
    "os"
)

func main() {
    targetEnv := os.Getenv("TARGET_ENV")
    dryRun := os.Getenv("DRY_RUN") == "true"

    if dryRun {
        fmt.Printf("[DRY RUN] Would deploy to %s\n", targetEnv)
        return
    }

    fmt.Printf("Deploying to %s...\n", targetEnv)
    // Deployment logic here
}
```

Build and place in scripts directory:

```bash
go build -o scripts/deployments/deploy-app main.go
```

---

## Registering Scripts

Before scripts can be executed through the API, they must be registered with metadata.

### Browse Available Scripts

List unregistered scripts in the scripts directory:

<Tabs>
<TabItem value="api" label="API">

```bash
curl https://localhost:8443/api/v1/scripts/available
```

</TabItem>
<TabItem value="response" label="Response">

```json
{
  "available": [
    {
      "path": "backups/backup-database.sh",
      "name": "backup-database.sh",
      "scriptType": "shell",
      "size": 1024,
      "modTime": "2024-12-25T10:00:00Z"
    },
    {
      "path": "maintenance/clear-cache.py",
      "name": "clear-cache.py",
      "scriptType": "python",
      "size": 512,
      "modTime": "2024-12-25T09:30:00Z"
    }
  ],
  "count": 2
}
```

</TabItem>
</Tabs>

### Register a Script

Register a script with metadata and parameters:

```bash
curl -X POST https://localhost:8443/api/v1/scripts \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Database Backup",
    "description": "Backup PostgreSQL database with compression",
    "category": "backups",
    "scriptPath": "backups/backup-database.sh",
    "timeoutSeconds": 600,
    "parameters": [
      {
        "name": "DB_HOST",
        "label": "Database Host",
        "description": "PostgreSQL server hostname",
        "type": "text",
        "required": false,
        "default": "localhost"
      },
      {
        "name": "DB_NAME",
        "label": "Database Name",
        "description": "Name of the database to backup",
        "type": "text",
        "required": true
      },
      {
        "name": "DB_USER",
        "label": "Database User",
        "description": "PostgreSQL username",
        "type": "text",
        "required": true
      },
      {
        "name": "BACKUP_DIR",
        "label": "Backup Directory",
        "description": "Directory to store backup files",
        "type": "text",
        "required": false,
        "default": "/backups"
      }
    ],
    "environment": {
      "PGPASSWORD": "from-vault"
    },
    "allowedScopes": ["admin", "backup-operator"]
  }'
```

### Script Registration Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Human-readable display name |
| `description` | string | No | Description of what the script does |
| `category` | string | No | Category for grouping (default: `general`) |
| `scriptPath` | string | Yes | Relative path from scripts directory |
| `timeoutSeconds` | integer | No | Execution timeout (default: 300) |
| `parameters` | array | No | List of configurable parameters |
| `environment` | object | No | Static environment variables |
| `allowedScopes` | array | No | JWT scopes allowed to execute |
| `dryRunCommand` | string | No | Custom dry run flag/command |

### Parameter Types

| Type | Description | UI Rendering |
|------|-------------|--------------|
| `text` | Free-form text input | Text field |
| `password` | Sensitive text (masked) | Password field |
| `number` | Numeric input | Number field |
| `boolean` | True/false toggle | Checkbox |
| `select` | Dropdown selection | Select with `options` |

### Parameter Validation

Parameters support regex validation:

```json
{
  "name": "EMAIL",
  "label": "Notification Email",
  "type": "text",
  "required": true,
  "validation": "^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\\.[a-zA-Z]{2,}$"
}
```

---

## Script Execution

### Execute a Script

Run a registered script with parameters:

<Tabs>
<TabItem value="basic-execution" label="Basic Execution">

```bash
curl -X POST https://localhost:8443/api/v1/scripts/database-backup/execute \
  -H "Content-Type: application/json" \
  -d '{
    "parameters": {
      "DB_NAME": "production",
      "DB_USER": "backup_user"
    }
  }'
```

</TabItem>
<TabItem value="response" label="Response">

```json
{
  "id": "exec_abc123",
  "scriptId": "database-backup",
  "status": "completed",
  "isDryRun": false,
  "triggeredBy": "device:mobile-001",
  "triggeredIp": "192.168.1.100:54321",
  "parameters": {
    "DB_HOST": "localhost",
    "DB_NAME": "production",
    "DB_USER": "backup_user",
    "BACKUP_DIR": "/backups"
  },
  "output": "Backing up production to /backups/production_20241225_100000.sql.gz...\nBackup completed successfully: /backups/production_20241225_100000.sql.gz\n",
  "exitCode": 0,
  "startedAt": "2024-12-25T10:00:00Z",
  "completedAt": "2024-12-25T10:00:15Z",
  "createdAt": "2024-12-25T10:00:00Z"
}
```

</TabItem>
</Tabs>

### Dry Run Execution

Test a script without making changes:

<Tabs>
<TabItem value="dedicated-endpoint" label="Dedicated Endpoint">

```bash
curl -X POST https://localhost:8443/api/v1/scripts/database-backup/dry-run \
  -H "Content-Type: application/json" \
  -d '{
    "parameters": {
      "DB_NAME": "production",
      "DB_USER": "backup_user"
    }
  }'
```

</TabItem>
<TabItem value="via-execute-endpoint" label="Via Execute Endpoint">

```bash
curl -X POST https://localhost:8443/api/v1/scripts/database-backup/execute \
  -H "Content-Type: application/json" \
  -d '{
    "parameters": {
      "DB_NAME": "production",
      "DB_USER": "backup_user"
    },
    "dryRun": true
  }'
```

</TabItem>
</Tabs>

When `dryRun` is true, the script receives `DRY_RUN=true` as an environment variable. Scripts should check this variable and simulate actions without making real changes.

### Execution Timeouts

- **Per-Script Timeout**: Set via `timeoutSeconds` during registration
- **Global Default**: Configured via `default_timeout` in config
- **API Maximum**: 10 minutes for synchronous execution via API
- **Scheduler**: Uses per-script timeout

Scripts exceeding their timeout are terminated and marked with `timeout` status.

### Output Handling

Script output (stdout and stderr combined) is captured and stored:

- **Maximum Size**: Configurable via `max_output_bytes` (default: 10MB)
- **Truncation**: Large outputs are truncated with a message indicating the truncation point
- **Storage**: Output is persisted with the execution record

---

## Viewing Execution History

### List Executions

<Tabs>
<TabItem value="all-executions" label="All Executions">

```bash
curl "https://localhost:8443/api/v1/executions?limit=20&offset=0"
```

</TabItem>
<TabItem value="filter-by-script" label="Filter by Script">

```bash
curl "https://localhost:8443/api/v1/executions?scriptId=database-backup"
```

</TabItem>
<TabItem value="filter-by-status" label="Filter by Status">

```bash
curl "https://localhost:8443/api/v1/executions?status=failed"
```

</TabItem>
</Tabs>

### Get Execution Details

```bash
curl https://localhost:8443/api/v1/executions/exec_abc123
```

### Execution Statuses

| Status | Description |
|--------|-------------|
| `pending` | Execution created, waiting to start |
| `running` | Script currently executing |
| `completed` | Script finished successfully (exit code 0) |
| `failed` | Script exited with non-zero code |
| `timeout` | Script exceeded timeout and was terminated |
| `cancelled` | Execution was cancelled |

---

## Workflows

Workflows chain multiple scripts together for complex automation sequences.

### Create a Workflow

```bash
curl -X POST https://localhost:8443/api/v1/workflows \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Full System Backup",
    "description": "Complete backup of databases and files",
    "steps": [
      {
        "scriptId": "database-backup",
        "parameters": {
          "DB_NAME": "production"
        },
        "onFailure": "stop"
      },
      {
        "scriptId": "backup-files",
        "parameters": {
          "SOURCE_DIR": "/data"
        },
        "onFailure": "continue"
      },
      {
        "scriptId": "cleanup-old-backups",
        "parameters": {
          "RETENTION_DAYS": "30"
        },
        "onFailure": "stop"
      }
    ],
    "allowedScopes": ["admin"]
  }'
```

### Workflow Step Options

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `scriptId` | string | Yes | ID of the script to execute |
| `parameters` | object | No | Parameters for this step |
| `onFailure` | string | No | Action on failure: `stop` (default) or `continue` |

### Failure Handling

- **stop**: Halt workflow execution when this step fails
- **continue**: Log the failure and proceed to the next step

### List Workflows

```bash
curl https://localhost:8443/api/v1/workflows
```

### Get Workflow Details

```bash
curl https://localhost:8443/api/v1/workflows/full-system-backup
```

### Delete Workflow

```bash
curl -X DELETE https://localhost:8443/api/v1/workflows/full-system-backup
```

---

## Scheduled Jobs

Schedule scripts or workflows to run automatically using cron expressions.

### Create a Schedule

```bash
curl -X POST https://localhost:8443/api/v1/schedules \
  -H "Content-Type: application/json" \
  -d '{
    "scriptId": "database-backup",
    "cronExpression": "0 2 * * *",
    "parameters": {
      "DB_NAME": "production",
      "DB_USER": "backup_user"
    },
    "enabled": true
  }'
```

### Cron Expression Format

Nekzus uses standard 5-field cron expressions:

```
* * * * *
│ │ │ │ │
│ │ │ │ └── Day of week (0-6, Sunday = 0)
│ │ │ └──── Month (1-12)
│ │ └────── Day of month (1-31)
│ └──────── Hour (0-23)
└────────── Minute (0-59)
```

### Cron Examples

| Expression | Description |
|------------|-------------|
| `0 2 * * *` | Daily at 2:00 AM |
| `0 */4 * * *` | Every 4 hours |
| `30 1 * * 0` | Sundays at 1:30 AM |
| `0 0 1 * *` | First day of each month at midnight |
| `*/15 * * * *` | Every 15 minutes |
| `0 9-17 * * 1-5` | Hourly 9 AM - 5 PM on weekdays |

### Schedule Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `scriptId` | string | Either | Script to run (mutually exclusive with workflowId) |
| `workflowId` | string | Either | Workflow to run (mutually exclusive with scriptId) |
| `cronExpression` | string | Yes | 5-field cron expression |
| `parameters` | object | No | Default parameters for scheduled runs |
| `enabled` | boolean | Yes | Whether schedule is active |

### List Schedules

```bash
curl https://localhost:8443/api/v1/schedules
```

### Get Schedule Details

```bash
curl https://localhost:8443/api/v1/schedules/schedule_xyz789
```

Response includes `lastRunAt` and `nextRunAt` timestamps.

### Delete Schedule

```bash
curl -X DELETE https://localhost:8443/api/v1/schedules/schedule_xyz789
```

---

## API Reference

### Script Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/scripts` | List all registered scripts |
| `GET` | `/api/v1/scripts?category=backups` | Filter scripts by category |
| `GET` | `/api/v1/scripts/available` | List unregistered script files |
| `POST` | `/api/v1/scripts` | Register a new script |
| `GET` | `/api/v1/scripts/{id}` | Get script details |
| `PUT` | `/api/v1/scripts/{id}` | Update script registration |
| `DELETE` | `/api/v1/scripts/{id}` | Delete script registration |
| `POST` | `/api/v1/scripts/{id}/execute` | Execute a script |
| `POST` | `/api/v1/scripts/{id}/dry-run` | Dry run a script |

### Execution Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/executions` | List execution history |
| `GET` | `/api/v1/executions?scriptId={id}` | Filter by script |
| `GET` | `/api/v1/executions?status={status}` | Filter by status |
| `GET` | `/api/v1/executions/{id}` | Get execution details |

### Workflow Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/workflows` | List all workflows |
| `POST` | `/api/v1/workflows` | Create a new workflow |
| `GET` | `/api/v1/workflows/{id}` | Get workflow details |
| `DELETE` | `/api/v1/workflows/{id}` | Delete a workflow |

### Schedule Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/schedules` | List all schedules |
| `POST` | `/api/v1/schedules` | Create a new schedule |
| `GET` | `/api/v1/schedules/{id}` | Get schedule details |
| `DELETE` | `/api/v1/schedules/{id}` | Delete a schedule |

---

## Security Considerations

### Script Execution Security

:::danger[Security Warning]

Scripts execute with the same permissions as the Nekzus process. Only register trusted scripts from verified sources.

:::


**Best Practices:**

- **Limit Script Directory**: Only place audited, trusted scripts in the scripts directory
- **Use Dedicated User**: Run Nekzus with a dedicated user with minimal permissions
- **Avoid Hardcoded Secrets**: Pass sensitive values as parameters, not in script files
- **Validate Input**: Scripts should validate all input parameters
- **Use Dry Run**: Test scripts with dry run before production execution

### Environment Isolation

Scripts receive a clean environment with only:

- `PATH` - System PATH for command access
- `HOME` - Home directory for scripts that need it
- Script's static environment variables
- User-provided parameters
- `DRY_RUN` flag when applicable

### Access Control

- Scripts can specify `allowedScopes` to restrict who can execute them
- All executions are tracked with `triggeredBy` (device ID, scheduler, or API key)
- IP address of the triggering request is recorded

### Output Security

- Sensitive output should be avoided; consider redirecting to files
- Large outputs are automatically truncated to prevent storage issues
- Output is stored in the database and accessible via API

---

## Troubleshooting

### Script Not Found After Registration

**Symptom**: Execution fails with "Script file not found"

**Solutions**:

1. Verify the file exists at the correct path:
   ```bash
   ls -la scripts/backups/backup-database.sh
   ```

2. Check file permissions:
   ```bash
   chmod +x scripts/backups/backup-database.sh
   ```

3. Ensure the path matches the registered `scriptPath` exactly (case-sensitive)

### Script Execution Timeout

**Symptom**: Execution status is `timeout`

**Solutions**:

1. Increase script timeout during registration:
   ```bash
   curl -X PUT https://localhost:8443/api/v1/scripts/slow-script \
     -H "Content-Type: application/json" \
     -d '{"timeoutSeconds": 1800}'
   ```

2. Optimize script for faster execution

3. Break into smaller scripts chained via workflow

### Parameter Validation Failed

**Symptom**: Error "Parameter validation failed"

**Solutions**:

1. Check required parameters are provided
2. Verify values match validation regex patterns
3. Check parameter names are correct (case-sensitive)

### Script Fails Silently

**Symptom**: Exit code 0 but expected output not produced

**Solutions**:

1. Check for `set -e` in shell scripts to fail on errors
2. Verify environment variables are correctly passed
3. Check script output in execution details
4. Test script manually with same parameters

### Scheduler Not Running Scripts

**Symptom**: Scheduled scripts not executing at expected times

**Solutions**:

1. Verify schedule is enabled:
   ```bash
   curl https://localhost:8443/api/v1/schedules/schedule_id
   ```

2. Check `nextRunAt` timestamp is in the past

3. Verify cron expression is valid

4. Check Nekzus logs for scheduler errors:
   ```bash
   grep "script scheduler" /var/log/nekzus.log
   ```

### Python Script Import Errors

**Symptom**: Python script fails with ModuleNotFoundError

**Solutions**:

1. Install required packages in the Python environment used by Nekzus

2. Use virtual environment and specify path:
   ```bash
   #!/usr/bin/env /path/to/venv/bin/python3
   ```

3. Include dependencies in the script or use inline pip install

---

## Examples

### Complete Backup Automation

```yaml
# Register scripts
scripts:
  - name: "Backup PostgreSQL"
    scriptPath: "backups/pg-backup.sh"
    parameters:
      - name: DB_NAME
        required: true
      - name: RETENTION_DAYS
        default: "7"

  - name: "Upload to S3"
    scriptPath: "backups/s3-upload.sh"
    parameters:
      - name: BUCKET_NAME
        required: true
      - name: FILE_PATH
        required: true

# Create workflow
workflow:
  name: "Database Backup Pipeline"
  steps:
    - scriptId: backup-postgresql
      parameters:
        DB_NAME: production
      onFailure: stop
    - scriptId: upload-to-s3
      parameters:
        BUCKET_NAME: my-backups
      onFailure: stop

# Schedule nightly
schedule:
  workflowId: database-backup-pipeline
  cronExpression: "0 3 * * *"
  enabled: true
```

### Health Check with Notifications

```bash
#!/bin/bash
# health-check.sh

set -e

SERVICES="${SERVICES:-nginx,postgres,redis}"
WEBHOOK_URL="${WEBHOOK_URL:-}"

check_service() {
    local service=$1
    systemctl is-active --quiet "$service" && echo "OK" || echo "FAILED"
}

failed_services=""
for service in ${SERVICES//,/ }; do
    status=$(check_service "$service")
    echo "$service: $status"
    if [ "$status" = "FAILED" ]; then
        failed_services="$failed_services $service"
    fi
done

if [ -n "$failed_services" ] && [ -n "$WEBHOOK_URL" ]; then
    curl -X POST "$WEBHOOK_URL" \
      -H "Content-Type: application/json" \
      -d "{\"text\": \"Failed services:$failed_services\"}"
fi

[ -z "$failed_services" ] || exit 1
```

---

## Related Documentation

- [API Reference](../reference/api) - Complete API documentation
- [Configuration Reference](../reference/configuration) - Full configuration options
- [Troubleshooting](../guides/troubleshooting) - Troubleshooting guide
