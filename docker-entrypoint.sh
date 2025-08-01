#!/bin/bash
set -e

# Handle signals properly
trap 'echo "Received SIGINT/SIGTERM - shutting down gracefully"; kill -TERM $CHILD_PID; exit' SIGINT SIGTERM

echo "=============== EmBoxd $(date) ==============="

# Ensure log directory exists
mkdir -p ${LOG_DIR:-/logs}

# Configure environment
echo "LOG_DIR=${LOG_DIR:-/logs}"
echo "TZ=${TZ:-UTC}"

# Set PATH to include Go binaries
export PATH=$PATH:/root/go/bin

# Verify Playwright installation
echo "=============== Playwright Setup ==============="
echo "Checking for playwright binary..."
which playwright || go install github.com/playwright-community/playwright-go/cmd/playwright@v0.4900.0

# Set Playwright browser path
export PLAYWRIGHT_BROWSERS_PATH=${PLAYWRIGHT_BROWSERS_PATH:-/root/.cache/ms-playwright}
echo "PLAYWRIGHT_BROWSERS_PATH=$PLAYWRIGHT_BROWSERS_PATH"

echo "Installing Firefox browser for Playwright..."
if ! playwright install firefox --with-deps; then
  echo "ERROR: Failed to install Firefox browser for Playwright"
  # Continue anyway - the directory might be pre-populated from a volume mount
fi

echo "Checking for Firefox installation..."
if ! ls -la $PLAYWRIGHT_BROWSERS_PATH 2>/dev/null; then
  echo "WARNING: No browser cache found at $PLAYWRIGHT_BROWSERS_PATH"
fi

# Final startup
echo "=============== Starting EmBoxd ==============="
echo "Command: emboxd $@"
echo "Time: $(date)"
emboxd "$@" &
CHILD_PID=$!
wait $CHILD_PID