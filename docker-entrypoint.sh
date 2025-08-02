#!/bin/bash
set -e

# Handle signals properly
trap 'echo "Received SIGINT/SIGTERM - shutting down gracefully"; kill -TERM $CHILD_PID; exit' SIGINT SIGTERM

echo "=============== EmBoxd $(date) ==============="

# Ensure log directory exists and is writable
LOG_DIR=${LOG_DIR:-/logs}
mkdir -p $LOG_DIR
touch $LOG_DIR/emboxd.log
chmod -R 755 $LOG_DIR
chmod 644 $LOG_DIR/emboxd.log
echo "Log directory: $LOG_DIR"
ls -la $LOG_DIR

# Configure environment
echo "LOG_DIR=${LOG_DIR:-/logs}"
echo "TZ=${TZ:-UTC}"

# Set PATH to include Go binaries
export PATH=$PATH:/root/go/bin

# Set Playwright browser path
export PLAYWRIGHT_BROWSERS_PATH=${PLAYWRIGHT_BROWSERS_PATH:-/root/.cache/ms-playwright}
echo "PLAYWRIGHT_BROWSERS_PATH=$PLAYWRIGHT_BROWSERS_PATH"

# Verify Playwright setup
echo "=============== Playwright Setup ==============="
echo "Checking for Playwright installation..."

# Verify the driver files exist
if [[ ! -d "$PLAYWRIGHT_BROWSERS_PATH" ]]; then
  echo "ERROR: Playwright browser directory doesn't exist"
  mkdir -p $PLAYWRIGHT_BROWSERS_PATH
fi

# Quick verification of Firefox installation
echo "Verifying Firefox installation..."
if ! ls -la $PLAYWRIGHT_BROWSERS_PATH/firefox-* 2>/dev/null; then
  echo "WARNING: Firefox installation not found at expected location"
  # We don't exit here as the Dockerfile should have installed it correctly
fi

# Verify symlinks for compatibility
echo "Verifying Playwright symlinks..."
# Ensure the playwright-drivers symlink exists
if [[ ! -L "/go/pkg/mod/github.com/playwright-community/playwright-drivers" ]]; then
  echo "Creating playwright-drivers symlink"
  mkdir -p /go/pkg/mod/github.com/playwright-community
  ln -sf $PLAYWRIGHT_BROWSERS_PATH /go/pkg/mod/github.com/playwright-community/playwright-drivers
fi

# Ensure the firefox-1491 directory exists
if [[ ! -d "$PLAYWRIGHT_BROWSERS_PATH/firefox-1491" ]]; then
  echo "Creating firefox-1491 directory"
  if firefox_dir=$(find $PLAYWRIGHT_BROWSERS_PATH -name "firefox-*" -type d | head -1); then
    mkdir -p $PLAYWRIGHT_BROWSERS_PATH/firefox-1491
    cp -r $firefox_dir/* $PLAYWRIGHT_BROWSERS_PATH/firefox-1491/
    ln -sf ./firefox-* $PLAYWRIGHT_BROWSERS_PATH/firefox-1491
  else
    echo "WARNING: No Firefox installation found to link"
  fi
fi

# Final startup
echo "=============== Starting EmBoxd ==============="
echo "Command: emboxd $@"
echo "Time: $(date)"
emboxd "$@" &
CHILD_PID=$!
wait $CHILD_PID