# Demo Scripts

Example scripts for testing the Nekzus scripts feature.

## Scripts

| Script | Description | Parameters |
|--------|-------------|------------|
| `hello.sh` | Simple hello world | `NAME` (default: World) |
| `system-info.sh` | Display system information | None |
| `backup-check.sh` | Check backup directory status | `BACKUP_DIR`, `VERBOSE` |
| `network-check.sh` | Test network connectivity | `TARGET`, `PORT`, `TIMEOUT` |
| `log-generator.sh` | Generate sample log entries | `COUNT`, `INTERVAL`, `LOG_LEVEL` |

## Usage in Demo

These scripts are mounted at `/app/demo-scripts` in the demo container and can be registered via the Scripts API.

### Register a Script

```bash
curl -X POST http://localhost:8080/api/v1/scripts \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Hello World",
    "scriptPath": "hello.sh",
    "category": "demo"
  }'
```

### Execute a Script

```bash
curl -X POST http://localhost:8080/api/v1/scripts/hello-world/execute \
  -H "Content-Type: application/json" \
  -d '{
    "parameters": {"NAME": "Nexus User"}
  }'
```

### List Available Scripts

```bash
curl http://localhost:8080/api/v1/scripts/available
```
