#!/bin/bash
# Demo Script: Network Connectivity Check
# Description: Check network connectivity to services
# Category: demo
set -euo pipefail

TARGET="${TARGET:-localhost}"
PORT="${PORT:-8080}"
TIMEOUT="${TIMEOUT:-5}"

echo "=== Network Connectivity Check ==="
echo ""
echo "Target:  ${TARGET}"
echo "Port:    ${PORT}"
echo "Timeout: ${TIMEOUT}s"
echo ""

# Check if nc (netcat) is available
if command -v nc &> /dev/null; then
    echo "Testing TCP connection..."
    if nc -z -w "${TIMEOUT}" "${TARGET}" "${PORT}" 2>/dev/null; then
        echo "Result: SUCCESS - Port ${PORT} is reachable"
    else
        echo "Result: FAILED - Cannot connect to ${TARGET}:${PORT}"
        exit 1
    fi
elif command -v timeout &> /dev/null; then
    echo "Testing with timeout command..."
    if timeout "${TIMEOUT}" bash -c "echo >/dev/tcp/${TARGET}/${PORT}" 2>/dev/null; then
        echo "Result: SUCCESS - Port ${PORT} is reachable"
    else
        echo "Result: FAILED - Cannot connect to ${TARGET}:${PORT}"
        exit 1
    fi
else
    echo "Warning: nc (netcat) not available, trying wget..."
    if command -v wget &> /dev/null; then
        if wget -q --spider --timeout="${TIMEOUT}" "http://${TARGET}:${PORT}/" 2>/dev/null; then
            echo "Result: SUCCESS - HTTP endpoint is reachable"
        else
            echo "Result: FAILED or no HTTP response"
            exit 1
        fi
    else
        echo "Error: No suitable network testing tool available"
        exit 1
    fi
fi

echo ""
echo "Timestamp: $(date -u +"%Y-%m-%dT%H:%M:%SZ")"
