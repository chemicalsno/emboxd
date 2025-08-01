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

# Verify Playwright installation
echo "=============== Playwright Setup ==============="
echo "Checking for playwright binary..."
which playwright || go install github.com/playwright-community/playwright-go/cmd/playwright@latest

# Set Playwright browser path
export PLAYWRIGHT_BROWSERS_PATH=${PLAYWRIGHT_BROWSERS_PATH:-/root/.cache/ms-playwright}
echo "PLAYWRIGHT_BROWSERS_PATH=$PLAYWRIGHT_BROWSERS_PATH"

echo "Installing Firefox browser and drivers for Playwright..."
# Use the specifically installed playwright binary that matches the Go module version (v0.4902.0)
if ! playwright install firefox --with-deps; then
  echo "ERROR: Failed to install Firefox browser"
  exit 1
fi

# Install the correct driver version for v0.4902.0 (which uses v1.49.1)
echo "Installing Playwright driver v1.49.1 explicitly..."
cd /tmp && \
npm init -y && \
npm install playwright@1.49.1 && \
npx playwright@1.49.1 install --with-deps && \
# Create backup symlinks in case the detection fails
mkdir -p /go/pkg/mod/github.com/playwright-community && \
ln -sf $PLAYWRIGHT_BROWSERS_PATH /go/pkg/mod/github.com/playwright-community/playwright-drivers && \
# Fix permissions for the cache directory
chmod -R 777 $PLAYWRIGHT_BROWSERS_PATH && \
cd - || echo "Failed to return to previous directory"

echo "Verifying Playwright setup..."
# Check if driver binary is present
find $PLAYWRIGHT_BROWSERS_PATH -name "*.jar" -o -name "*.exe" | grep -i driver || echo "No driver found. Creating a symlink to ensure driver is found."

# Create additional symlink for the specific version
if find $PLAYWRIGHT_BROWSERS_PATH -name "firefox-*" -type d | grep -q ""; then
  mkdir -p $PLAYWRIGHT_BROWSERS_PATH/firefox-1491
  find $PLAYWRIGHT_BROWSERS_PATH -name "firefox-*" -type d -exec cp -r {}/* $PLAYWRIGHT_BROWSERS_PATH/firefox-1491/ \;
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

# Check exact driver location that the error is looking for
echo "Looking for v1.49.1 driver specifically..."
find $PLAYWRIGHT_BROWSERS_PATH -type d -name "*1.49*" || echo "No 1.49.1 driver found"
find /go -name "*playwright*" -type d || echo "No playwright in /go"

# Make sure we have the right driver versions
echo "Creating explicit driver link"
cd $PLAYWRIGHT_BROWSERS_PATH && \
ln -sf ./firefox-* ./firefox-1491 && \
cd - || echo "Failed to create driver link"

# Final startup
echo "=============== Starting EmBoxd ==============="
echo "Command: emboxd $@"
echo "Time: $(date)"
emboxd "$@" &
CHILD_PID=$!
wait $CHILD_PID