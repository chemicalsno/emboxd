#!/bin/bash
set -e

# Create logs directory inside container if needed
mkdir -p ${LOG_DIR:-/logs}
chmod 755 ${LOG_DIR:-/logs}

echo "Starting emboxd with LOG_DIR=${LOG_DIR:-/logs}"

# Execute the original entrypoint
exec emboxd "$@"