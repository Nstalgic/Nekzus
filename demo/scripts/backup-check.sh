#!/bin/bash
# Demo Script: Backup Status Check
# Description: Check backup directory status (supports dry-run mode)
# Category: demo
set -euo pipefail

BACKUP_DIR="${BACKUP_DIR:-/app/data/backups}"
VERBOSE="${VERBOSE:-false}"

# Check for dry run mode
if [ "${DRY_RUN:-false}" = "true" ]; then
    echo "[DRY RUN] Would check backup directory: ${BACKUP_DIR}"
    echo "[DRY RUN] Verbose mode: ${VERBOSE}"
    echo "[DRY RUN] No actual operations performed"
    exit 0
fi

echo "=== Backup Status Check ==="
echo ""
echo "Backup Directory: ${BACKUP_DIR}"
echo ""

if [ -d "${BACKUP_DIR}" ]; then
    echo "Status: Directory exists"
    echo ""

    BACKUP_COUNT=$(find "${BACKUP_DIR}" -maxdepth 1 -name "*.db" -o -name "*.sql" -o -name "*.tar*" 2>/dev/null | wc -l || echo "0")
    echo "Backup files found: ${BACKUP_COUNT}"

    if [ "${VERBOSE}" = "true" ]; then
        echo ""
        echo "Files:"
        ls -lh "${BACKUP_DIR}" 2>/dev/null || echo "  (unable to list files)"
    fi

    # Get directory size
    DIR_SIZE=$(du -sh "${BACKUP_DIR}" 2>/dev/null | cut -f1 || echo "unknown")
    echo ""
    echo "Total size: ${DIR_SIZE}"
else
    echo "Status: Directory does not exist"
    echo ""
    echo "Recommendation: Enable backups in config or run initial backup"
fi

echo ""
echo "Timestamp: $(date -u +"%Y-%m-%dT%H:%M:%SZ")"
