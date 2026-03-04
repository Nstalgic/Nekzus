#!/bin/bash
# Demo Script: Log Generator
# Description: Generate sample log entries for testing log streaming
# Category: demo
set -euo pipefail

COUNT="${COUNT:-10}"
INTERVAL="${INTERVAL:-1}"
LOG_LEVEL="${LOG_LEVEL:-INFO}"

echo "=== Log Generator ==="
echo "Generating ${COUNT} log entries at ${INTERVAL}s intervals"
echo ""

for i in $(seq 1 "${COUNT}"); do
    TIMESTAMP=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

    # Vary log levels for realistic output
    case $((i % 5)) in
        0) LEVEL="ERROR" ;;
        1) LEVEL="WARN" ;;
        2) LEVEL="DEBUG" ;;
        *) LEVEL="${LOG_LEVEL}" ;;
    esac

    # Generate realistic log messages
    case $((i % 4)) in
        0) MSG="Request processed successfully id=${i} latency=$((RANDOM % 100 + 10))ms" ;;
        1) MSG="Connection established client=user-$((RANDOM % 100))" ;;
        2) MSG="Cache hit rate=$((RANDOM % 30 + 70))%" ;;
        3) MSG="Health check passed services=3/3" ;;
    esac

    echo "${TIMESTAMP} [${LEVEL}] ${MSG}"

    if [ "${i}" -lt "${COUNT}" ]; then
        sleep "${INTERVAL}"
    fi
done

echo ""
echo "Log generation complete"
