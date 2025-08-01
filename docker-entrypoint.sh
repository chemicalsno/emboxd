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
which playwright || go install github.com/playwright-community/playwright-go/cmd/playwright@latest

# Set Playwright browser path
export PLAYWRIGHT_BROWSERS_PATH=${PLAYWRIGHT_BROWSERS_PATH:-/root/.cache/ms-playwright}
echo "PLAYWRIGHT_BROWSERS_PATH=$PLAYWRIGHT_BROWSERS_PATH"

echo "Installing Firefox browser and drivers for Playwright..."
# Force reinstall the drivers to fix the common "please install the driver" error
if ! playwright install firefox --with-deps && playwright install; then
  echo "ERROR: Failed to install Firefox browser and drivers for Playwright"
  exit 1
fi

echo "Checking for Firefox installation..."
if ! ls -la $PLAYWRIGHT_BROWSERS_PATH 2>/dev/null; then
  echo "WARNING: No browser cache found at $PLAYWRIGHT_BROWSERS_PATH"
fi

# Verify the driver files exist
echo "Checking for Playwright drivers..."
if [[ ! -d "$PLAYWRIGHT_BROWSERS_PATH" ]]; then
  echo "ERROR: Playwright browser directory doesn't exist"
  mkdir -p $PLAYWRIGHT_BROWSERS_PATH
fi
ls -la $PLAYWRIGHT_BROWSERS_PATH

# Final startup
echo "=============== Starting EmBoxd ==============="
echo "Command: emboxd $@"
echo "Time: $(date)"
emboxd "$@" &
CHILD_PID=$!
wait $CHILD_PID