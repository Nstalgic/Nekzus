#!/bin/bash
# Demo Script: System Information
# Description: Displays system information for the host
# Category: demo
set -euo pipefail

echo "=== System Information ==="
echo ""
echo "Hostname:     $(hostname 2>/dev/null || echo 'unknown')"
echo "OS:           $(uname -s)"
echo "Architecture: $(uname -m)"
echo "Kernel:       $(uname -r)"
echo ""
echo "=== Memory ==="
if command -v free &> /dev/null; then
    free -h 2>/dev/null || echo "Memory info not available"
elif [ -f /proc/meminfo ]; then
    grep -E '^(MemTotal|MemFree|MemAvailable):' /proc/meminfo
else
    echo "Memory info not available"
fi
echo ""
echo "=== Disk Usage ==="
df -h / 2>/dev/null | tail -1 || echo "Disk info not available"
echo ""
echo "=== Uptime ==="
uptime 2>/dev/null || echo "Uptime not available"
echo ""
echo "Timestamp: $(date -u +"%Y-%m-%dT%H:%M:%SZ")"
