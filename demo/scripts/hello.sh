#!/bin/bash
# Demo Script: Hello World
# Description: Simple script to demonstrate basic script execution
# Category: demo
set -euo pipefail

NAME="${NAME:-World}"
echo "Hello, ${NAME}!"
echo "Script executed at: $(date -u +"%Y-%m-%dT%H:%M:%SZ")"
echo "Running on: $(uname -s) $(uname -m)"
